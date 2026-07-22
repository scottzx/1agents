package ccconnect

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/BurntSushi/toml"
	_ "github.com/chenhg5/cc-connect"
	"github.com/chenhg5/cc-connect/config"
	"github.com/chenhg5/cc-connect/core"

	"github.com/scottzx/1Agents/backend/internal/agent"

	// Blank-import all agents and platforms plugins from cc-connect
	_ "github.com/chenhg5/cc-connect/agent/acp"
	_ "github.com/chenhg5/cc-connect/agent/claudecode"
	_ "github.com/chenhg5/cc-connect/agent/codex"
	_ "github.com/chenhg5/cc-connect/agent/cursor"
	_ "github.com/chenhg5/cc-connect/agent/devin"
	_ "github.com/chenhg5/cc-connect/agent/gemini"
	_ "github.com/chenhg5/cc-connect/agent/iflow"
	_ "github.com/chenhg5/cc-connect/agent/kimi"
	_ "github.com/chenhg5/cc-connect/agent/opencode"
	_ "github.com/chenhg5/cc-connect/agent/pi"
	_ "github.com/chenhg5/cc-connect/agent/qoder"
	_ "github.com/chenhg5/cc-connect/agent/tmux"
	_ "github.com/chenhg5/cc-connect/platform/dingtalk"
	_ "github.com/chenhg5/cc-connect/platform/discord"
	_ "github.com/chenhg5/cc-connect/platform/feishu"
	_ "github.com/chenhg5/cc-connect/platform/line"
	_ "github.com/chenhg5/cc-connect/platform/max"
	_ "github.com/chenhg5/cc-connect/platform/qq"
	_ "github.com/chenhg5/cc-connect/platform/qqbot"
	_ "github.com/chenhg5/cc-connect/platform/slack"
	_ "github.com/chenhg5/cc-connect/platform/telegram"
	_ "github.com/chenhg5/cc-connect/platform/wecom"
	_ "github.com/chenhg5/cc-connect/platform/weibo"
	_ "github.com/chenhg5/cc-connect/platform/weixin"
	_ "github.com/chenhg5/cc-connect/platform/wps-xiezuo"
	_ "github.com/chenhg5/cc-connect/web"

	"github.com/scottzx/1Agents/backend/internal/workspace"
)

type dummyBridgePlatform struct{}

func (p *dummyBridgePlatform) Name() string                            { return "bridge" }
func (p *dummyBridgePlatform) Start(handler core.MessageHandler) error { return nil }
func (p *dummyBridgePlatform) Reply(ctx context.Context, replyCtx any, content string) error {
	return nil
}
func (p *dummyBridgePlatform) Send(ctx context.Context, replyCtx any, content string) error {
	return nil
}
func (p *dummyBridgePlatform) Stop() error { return nil }

func init() {
	core.RegisterPlatform("bridge", func(opts map[string]any) (core.Platform, error) {
		return &dummyBridgePlatform{}, nil
	})
}

var (
	ManagementPort  int
	ManagementToken string
	BridgePort      int
	BridgeToken     string

	// sharedTasksStore is the task store instance owned by the HTTP server's
	// scheduler. The IM notifier (#129) must write back through the SAME
	// instance so its per-workspace Mutate lock serializes against the
	// scheduler's; a second NewTasksStore() would have independent locks and
	// risk lost updates. Set by SetTasksStore before/around Start.
	sharedTasksStore *agent.TasksStore
)

// SetTasksStore hands the IM notifier the server's shared task store so its
// approve/reject write-backs serialize with the scheduler. Call before Start.
func SetTasksStore(store *agent.TasksStore) { sharedTasksStore = store }

const defaultResetOnIdleMins = 0

type initialModelRefreshStarter interface {
	StartInitialModelRefresh()
}

type providerWiringResult struct {
	explicitProviderRequested bool
	activeProviderApplied     bool
	canStartInitialRefresh    bool
}

// Start boots the cc-connect supervisor, dynamic port allocator, configuration synchronization,
// and engine listeners.
func Start(ctx context.Context, isDesktop bool) {
	log.Println("[ccconnect] Starting cc-connect integration runner...")

	baseMgmtPort := 39820
	baseBridgePort := 39810

	var err error
	ManagementPort, err = findFreePort(baseMgmtPort)
	if err != nil {
		log.Printf("[ccconnect] Error finding management port, fallback to %d: %v", baseMgmtPort, err)
		ManagementPort = baseMgmtPort
	}

	BridgePort, err = findFreePort(baseBridgePort)
	if err != nil {
		log.Printf("[ccconnect] Error finding bridge port, fallback to %d: %v", baseBridgePort, err)
		BridgePort = baseBridgePort
	}

	// 1agents runs its embedded cc-connect against a PRIVATE config under
	// ~/.1agents/im_channels — never the shared ~/.cc-connect a globally
	// installed cc-connect owns. This decouples the two so 1agents' project
	// sync can no longer clobber a user's standalone cc-connect setup.
	ccDir := ccConfigDir()
	if err := os.MkdirAll(ccDir, 0o755); err != nil {
		log.Printf("[ccconnect] Error creating %s: %v", ccDir, err)
	}

	configPath := ccConfigPath()
	config.ConfigPath = configPath

	// One-time migration: if we have no private config yet but a legacy shared
	// one exists (1agents used ~/.cc-connect before decoupling), copy it over so
	// the user's existing channels carry across. The legacy file is left intact.
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if legacy := legacyCCConfigPath(); legacy != "" {
			if data, rerr := os.ReadFile(legacy); rerr == nil {
				if werr := os.WriteFile(configPath, data, 0o644); werr != nil {
					log.Printf("[ccconnect] migrate legacy config %s → %s failed: %v", legacy, configPath, werr)
				} else {
					log.Printf("[ccconnect] migrated legacy config %s → %s", legacy, configPath)
				}
			}
		}
	}

	enabledTrue := true

	// ── 1. Synchronously bootstrap config & tokens to avoid first-launch race condition ──
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		ManagementToken = core.GenerateToken(16)
		BridgeToken = core.GenerateToken(16)

		defaultTOML := fmt.Sprintf(`# cc-connect config bootstrapped by 1agents IDE

language = "zh"

[log]
level = "info"

[management]
enabled = true
port = %d
token = "%s"
cors_origins = ["*"]

[bridge]
enabled = true
port = %d
token = "%s"
insecure = true
`, ManagementPort, ManagementToken, BridgePort, BridgeToken)

		if err := os.WriteFile(configPath, []byte(defaultTOML), 0o644); err != nil {
			log.Printf("[ccconnect] Error writing bootstrapped config: %v", err)
		} else {
			log.Printf("[ccconnect] Bootstrapped default cc-connect config at %s", configPath)
		}
	}

	cfgSync := &config.Config{}
	if _, err := toml.DecodeFile(configPath, cfgSync); err == nil {
		if cfgSync.Management.Token != "" {
			ManagementToken = cfgSync.Management.Token
		} else {
			ManagementToken = core.GenerateToken(16)
		}
		if cfgSync.Bridge.Token != "" {
			BridgeToken = cfgSync.Bridge.Token
		} else {
			BridgeToken = core.GenerateToken(16)
		}
		syncAllProvidersToCCSwitch(cfgSync.Providers)
	} else {
		log.Printf("[ccconnect] Error decoding config TOML synchronously: %v", err)
		if ManagementToken == "" {
			ManagementToken = core.GenerateToken(16)
		}
		if BridgeToken == "" {
			BridgeToken = core.GenerateToken(16)
		}
	}

	// Boot cc-connect core engines & servers in a background supervisor loop
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[ccconnect] Recovered from panic in engine startup: %v", r)
			}
		}()

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			runCtx, runCancel := context.WithCancel(ctx)

			if _, err := os.Stat(configPath); os.IsNotExist(err) {
				ManagementToken = core.GenerateToken(16)
				BridgeToken = core.GenerateToken(16)

				defaultTOML := fmt.Sprintf(`# cc-connect config bootstrapped by 1agents IDE

language = "zh"

[log]
level = "info"

[management]
enabled = true
port = %d
token = "%s"
cors_origins = ["*"]

[bridge]
enabled = true
port = %d
token = "%s"
insecure = true
`, ManagementPort, ManagementToken, BridgePort, BridgeToken)

				if err := os.WriteFile(configPath, []byte(defaultTOML), 0o644); err != nil {
					log.Printf("[ccconnect] Error writing bootstrapped config: %v", err)
				} else {
					log.Printf("[ccconnect] Bootstrapped default cc-connect config at %s", configPath)
				}
			}

			cfg := &config.Config{}
			if _, err := toml.DecodeFile(configPath, cfg); err == nil {
				syncAllProvidersToCCSwitch(cfg.Providers)
				// #277 Phase 4: one-shot fold of legacy `X__<agent>` projects that
				// share a work_dir into a single de-suffixed project + per-channel
				// agent bindings. Idempotent: a no-op once already migrated.
				if migrated, changed := MigrateLegacyAgentSuffixProjects(cfg.Projects); changed {
					cfg.Projects = migrated
					log.Printf("[ccconnect] Migrated legacy __<agent> projects by path → %d project(s)", len(cfg.Projects))
				}
			} else {
				log.Printf("[ccconnect] Error decoding config TOML (%s): %v", configPath, err)
			}

			// Always ensure management and bridge are properly configured in memory
			if cfg.Management.Token == "" {
				ManagementToken = core.GenerateToken(16)
				cfg.Management.Token = ManagementToken
			} else {
				ManagementToken = cfg.Management.Token
			}
			cfg.Management.Enabled = &enabledTrue
			cfg.Management.Port = ManagementPort
			cfg.Management.CORSOrigins = []string{"*"} // Ensure absolute access from any client/iframe origin

			if cfg.Bridge.Token == "" {
				BridgeToken = core.GenerateToken(16)
				cfg.Bridge.Token = BridgeToken
			} else {
				BridgeToken = cfg.Bridge.Token
			}
			cfg.Bridge.Enabled = &enabledTrue
			cfg.Bridge.Port = BridgePort
			cfg.Bridge.Insecure = &enabledTrue // Allow local connections

			// Sync Workspaces configurations as Projects
			wsHandler := workspace.NewHandler()
			wsCfg, err := wsHandler.LoadWorkspacesConfig()
			if err != nil {
				log.Printf("[ccconnect] Error loading workspaces config: %v", err)
			} else {
				// One-way sync only: meta.db workspaces → cc-connect projects,
				// matched by work_dir PATH (#277). meta.db is the single source of
				// truth for the project SET; the config owns each project's channels
				// + per-channel agent bindings.
				//
				// The old two-way "watchdog" (importing config projects BACK into the
				// workspace registry to force the two stores equal) is gone: it kept
				// resurrecting deleted workspaces and fought manual edits. Now project
				// creation/deletion flows one way (workspace CRUD → config, delete
				// removes by path), and cc-connect Web edits to channels/agents are
				// preserved because reconcileProjectsByPath keeps matched projects'
				// platforms verbatim (only refreshing work_dir + repairing a
				// placeholder name). Projects whose path is no longer a registered
				// workspace are dropped.
				if len(wsCfg.Workspaces) > 0 {
					cfg.Projects = reconcileProjectsByPath(cfg.Projects, wsCfg.Workspaces)
				}
			}

			// 3. Fallback: if project list is empty, inject a default placeholder project to pass CC-Connect's validation.
			if len(cfg.Projects) == 0 {
				homeDir, _ := os.UserHomeDir()
				tempDir := filepath.Join(homeDir, "temp")
				_ = os.MkdirAll(tempDir, 0755)
				cfg.Projects = []config.ProjectConfig{
					{
						Name: "temp",
						Agent: config.AgentConfig{
							Type: "claudecode",
							Options: map[string]any{
								"work_dir": tempDir,
								"mode":     "default",
							},
						},
						Platforms: []config.PlatformConfig{
							{
								Type: "bridge",
							},
						},
					},
				}
			}

			// Write the merged configuration back to disk
			if err := saveConfig(cfg, configPath); err != nil {
				log.Printf("[ccconnect] Error saving config back to disk: %v", err)
			}

			// Now load and validate the fully populated configuration officially
			finalCfg, err := config.Load(configPath)
			if err != nil {
				log.Printf("[ccconnect] Error loading final validated config: %v", err)
				runCancel()
				// Block until a workspace is added (new RestartCh signal) or the
				// daemon shuts down. This prevents a tight 1s retry loop flooding
				// logs when the config is invalid (e.g., no projects configured).
				select {
				case <-ctx.Done():
					return
				case <-core.RestartCh:
					log.Println("[ccconnect] Config changed, retrying engine start...")
					time.Sleep(300 * time.Millisecond)
				}
				continue
			}

			log.Printf("[ccconnect] Active Management Port: %d", ManagementPort)
			log.Printf("[ccconnect] Active Bridge Port: %d", BridgePort)

			// Run the engines and servers synchronously in this background loop,
			// blocking until reload/restart is requested or context is cancelled.
			shouldRestart := runEngine(runCtx, finalCfg, configPath)
			runCancel()

			if !shouldRestart {
				return // Context was cancelled or clean exit, do not restart
			}

			log.Println("[ccconnect] CC-Connect engines restarting/reloading in-process...")
			time.Sleep(300 * time.Millisecond) // Short delay to let sockets clean up
		}
	}()
}

func findFreePort(startPort int) (int, error) {
	for port := startPort; port < startPort+100; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			ln.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("no free ports found starting at %d", startPort)
}

func saveConfig(cfg *config.Config, path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return toml.NewEncoder(file).Encode(cfg)
}

func runEngine(ctx context.Context, cfg *config.Config, configPath string) bool {
	// Setup log levels
	logLevel := slog.LevelInfo
	switch strings.ToLower(cfg.Log.Level) {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})))

	if len(cfg.Projects) == 0 {
		slog.Info("no projects configured in cc-connect, running management servers only")
	}

	engines := make([]*core.Engine, 0, len(cfg.Projects))
	effectiveWorkDirs := make([]string, 0, len(cfg.Projects))
	// bootedProjects[i] is the project that produced engines[i]. Kept in lockstep
	// with the slices above so a project skipped on agent-creation failure never
	// shifts the index mapping (see the continue below). Downstream registration
	// must index this slice, never cfg.Projects, which still holds skipped entries.
	bootedProjects := make([]config.ProjectConfig, 0, len(cfg.Projects))

	for _, proj := range cfg.Projects {
		if proj.RunAsUser != "" {
			if proj.Agent.Options == nil {
				proj.Agent.Options = map[string]any{}
			}
			proj.Agent.Options["run_as_user"] = proj.RunAsUser
			if len(proj.RunAsEnv) > 0 {
				proj.Agent.Options["run_as_env"] = proj.RunAsEnv
			}
		}
		agent, err := core.CreateAgent(proj.Agent.Type, buildAgentOptions(cfg.DataDir, proj))
		if err != nil {
			slog.Error("failed to create agent", "project", proj.Name, "error", err)
			continue
		}

		providerWiring := wireAgentProviders(agent, proj.Agent)

		var platforms []core.Platform
		// channelAgents maps a platform's Name() to a channel-level agent
		// override (config.PlatformConfig.Agent, #277). Only populated when a
		// channel binds a different agent than the project default; left empty
		// for existing single-agent projects so they behave exactly as before.
		channelAgents := make(map[core.Platform]core.Agent)
		for _, pc := range proj.Platforms {
			opts := make(map[string]any, len(pc.Options)+2)
			for k, v := range pc.Options {
				opts[k] = v
			}
			opts["cc_data_dir"] = cfg.DataDir
			opts["cc_project"] = proj.Name
			p, err := core.CreatePlatform(pc.Type, opts)
			if err != nil {
				slog.Error("failed to create platform", "project", proj.Name, "type", pc.Type, "error", err)
				continue
			}
			platforms = append(platforms, p)

			// Channel-level agent binding: when this channel overrides the
			// project agent with a different type, build a separate agent
			// instance so the channel can run concurrently alongside the project
			// default and other channels (same work_dir, different agent).
			chAgentCfg := config.ResolvePlatformAgent(proj, pc)
			if pc.Agent == nil || strings.EqualFold(chAgentCfg.Type, proj.Agent.Type) {
				continue
			}
			chAgent, err := core.CreateAgent(chAgentCfg.Type, buildChannelAgentOptions(cfg.DataDir, proj, chAgentCfg))
			if err != nil {
				slog.Error("failed to create channel agent", "project", proj.Name, "channel", p.Name(), "agent", chAgentCfg.Type, "error", err)
				continue
			}
			wireAgentProviders(chAgent, chAgentCfg)
			channelAgents[p] = chAgent
		}

		workDir, _ := proj.Agent.Options["work_dir"].(string)
		projectState := core.NewProjectStateStore(projectStatePath(cfg.DataDir, proj.Name))
		effectiveWorkDir := applyProjectStateOverride(proj.Name, agent, workDir, projectState)
		startInitialRefreshIfReady(agent, providerWiring)
		sessionFile := sessionStorePath(cfg.DataDir, proj.Name, effectiveWorkDir)

		var lang core.Language
		switch cfg.Language {
		case "zh", "chinese":
			lang = core.LangChinese
		case "zh-TW", "zh_TW", "zhtw":
			lang = core.LangTraditionalChinese
		case "ja", "japanese":
			lang = core.LangJapanese
		case "es", "spanish":
			lang = core.LangSpanish
		case "en", "english":
			lang = core.LangEnglish
		default:
			lang = core.LangAuto
		}

		engine := core.NewEngine(proj.Name, agent, platforms, sessionFile, lang)
		for p, chAgent := range channelAgents {
			engine.SetChannelAgent(p, chAgent)
			slog.Info("channel-level agent bound", "project", proj.Name, "channel", p.Name(), "agent", chAgent.Name())
		}
		_, _, _, _, _, showCtx, showFooter, _ := config.EffectiveDisplay(cfg, &proj)
		engine.SetShowContextIndicator(showCtx)
		engine.SetReplyFooterEnabled(showFooter)
		engine.SetAttachmentSendEnabled(cfg.AttachmentSend != "off")
		engine.SetFilterExternalSessions(proj.FilterExternalSessions != nil && *proj.FilterExternalSessions)
		engine.SetBaseWorkDir(workDir)
		engine.SetProjectStateStore(projectState)
		engine.SetDataDir(cfg.DataDir)

		// Reload configuration setups
		capturedEngine := engine
		capturedProjName := proj.Name
		engine.SetConfigReloadFunc(func() (*core.ConfigReloadResult, error) {
			return reloadConfig(configPath, capturedProjName, capturedEngine)
		})

		capturedProj := proj
		engine.SetProviderSaveFunc(func(providerName string) error {
			err := config.SaveActiveProvider(capturedProjName, providerName)
			if err == nil {
				appType := ""
				switch capturedProj.Agent.Type {
				case "claudecode":
					appType = "claude"
				case "codex":
					appType = "codex"
				case "gemini":
					appType = "gemini"
				}
				if appType != "" && providerName != "" {
					go runCCSwitchSwitchCommand(appType, sanitizeID(providerName))
				}
			}
			return err
		})

		engines = append(engines, engine)
		effectiveWorkDirs = append(effectiveWorkDirs, effectiveWorkDir)
		bootedProjects = append(bootedProjects, proj)
	}

	cronStore, err := core.NewCronStore(cfg.DataDir)
	if err != nil {
		slog.Warn("cron store unavailable", "error", err)
	}
	var cronSched *core.CronScheduler
	if cronStore != nil {
		cronSched = core.NewCronScheduler(cronStore)
		if cfg.Cron.Silent != nil && *cfg.Cron.Silent {
			cronSched.SetDefaultSilent(true)
		}
		if cfg.Cron.SessionMode != "" {
			cronSched.SetDefaultSessionMode(cfg.Cron.SessionMode)
		}
		for i, e := range engines {
			cronSched.RegisterEngine(bootedProjects[i].Name, e)
			e.SetCronScheduler(cronSched)
		}
	}

	heartbeatSched := core.NewHeartbeatScheduler(cfg.DataDir)
	for i := range engines {
		proj := bootedProjects[i]
		hbCfg := buildHeartbeatConfig(proj.Heartbeat)
		if hbCfg.Enabled {
			heartbeatSched.Register(proj.Name, hbCfg, engines[i], effectiveWorkDirs[i])
		}
		engines[i].SetHeartbeatScheduler(heartbeatSched)
	}

	var startErrors []error
	for _, e := range engines {
		if err := e.Start(); err != nil {
			slog.Warn("engine start partially failed", "error", err)
			startErrors = append(startErrors, err)
		}
	}
	if len(startErrors) > 0 && len(startErrors) == len(engines) {
		slog.Error("all engines failed to start")
	}

	if cronSched != nil {
		if err := cronSched.Start(); err != nil {
			slog.Error("cron scheduler start failed", "error", err)
		}
	}

	heartbeatSched.Start()

	// Start local Unix socket API server
	var apiSrv *core.APIServer
	if apiSrvInstance, err := core.NewAPIServer(cfg.DataDir); err != nil {
		slog.Error("failed to create cc-connect Unix socket API server", "error", err)
	} else {
		apiSrv = apiSrvInstance
		for i, e := range engines {
			apiSrv.RegisterEngine(bootedProjects[i].Name, e)
		}
		if cronSched != nil {
			apiSrv.SetCronScheduler(cronSched)
		}
		apiSrv.Start()
	}

	// Start bridge server
	var bridgeSrv *core.BridgeServer
	if cfg.Bridge.Enabled != nil && *cfg.Bridge.Enabled {
		port := cfg.Bridge.Port
		if port <= 0 {
			port = 9810
		}
		path := cfg.Bridge.Path
		if path == "" {
			path = "/bridge/ws"
		}
		insecure := cfg.Bridge.Insecure != nil && *cfg.Bridge.Insecure
		if insecure {
			bridgeSrv = core.NewBridgeServerInsecure(port, cfg.Bridge.Token, path, cfg.Bridge.CORSOrigins)
		} else {
			bridgeSrv = core.NewBridgeServer(port, cfg.Bridge.Token, path, cfg.Bridge.CORSOrigins)
		}
		if bridgeSrv != nil {
			for i, e := range engines {
				bp := bridgeSrv.NewPlatform(bootedProjects[i].Name)
				bridgeSrv.RegisterEngine(bootedProjects[i].Name, e, bp)
				e.AddPlatform(bp)
			}
			bridgeSrv.Start()

			// #129: wire the task-state IM notifier over this bridge so
			// blocked/failed/pending_review tasks push an approve/reject card
			// and the user's tap writes back to the task store.
			if n := newTaskNotifier(bridgeSrv, sharedTasksStore); n != nil {
				agent.SetTaskNotifier(n)
			}
		}
	}

	// Start management API server
	var mgmtSrv *core.ManagementServer
	if cfg.Management.Enabled != nil && *cfg.Management.Enabled {
		port := cfg.Management.Port
		if port <= 0 {
			port = 9820
		}
		mgmtSrv = core.NewManagementServer(port, cfg.Management.Token, cfg.Management.CORSOrigins)
		for i, e := range engines {
			mgmtSrv.RegisterEngine(bootedProjects[i].Name, e)
		}
		if cronSched != nil {
			mgmtSrv.SetCronScheduler(cronSched)
		}
		mgmtSrv.SetHeartbeatScheduler(heartbeatSched)
		// Serve the host-verified creatable agent list (installed + drivable),
		// intersected with cc-connect's plugin registry, so the dashboard never
		// offers an agent that would brick the engine (issue #24).
		mgmtSrv.SetListAgents(func() []core.CreatableAgentInfo {
			creatable := agent.DefaultCatalog().CreatableAgents(core.ListRegisteredAgents())
			out := make([]core.CreatableAgentInfo, 0, len(creatable))
			for _, a := range creatable {
				out = append(out, core.CreatableAgentInfo{
					Type:        a.Type,
					Label:       a.Label,
					CcTransport: a.CcTransport,
					Command:     a.Command,
				})
			}
			return out
		})
		if bridgeSrv != nil {
			mgmtSrv.SetBridgeServer(bridgeSrv)
		}
		// requestCCReload hot-reloads the cc-connect engines (non-blocking) so a
		// config write applied via the management API takes effect without the
		// Web UI asking the user to restart. Mirrors the RestartCh trigger that
		// SetChannelAgentBinding / workspace CRUD already use. This is what lets
		// "add channel / scan QR / change channel agent" auto-apply.
		requestCCReload := func() {
			select {
			case core.RestartCh <- core.RestartRequest{}:
			default:
			}
		}
		mgmtSrv.SetSetupFeishuSave(func(req core.FeishuSetupSaveRequest) error {
			platType := req.PlatformType
			if platType == "" {
				platType = "feishu"
			}
			_, err := config.EnsureProjectWithFeishuPlatform(config.EnsureProjectWithFeishuOptions{
				ProjectName:  req.ProjectName,
				PlatformType: platType,
				WorkDir:      req.WorkDir,
				AgentType:    req.AgentType,
			})
			if err != nil {
				return fmt.Errorf("ensure project: %w", err)
			}
			if _, err = config.SaveFeishuPlatformCredentials(config.FeishuCredentialUpdateOptions{
				ProjectName:       req.ProjectName,
				PlatformType:      platType,
				AppID:             req.AppID,
				AppSecret:         req.AppSecret,
				OwnerOpenID:       req.OwnerOpenID,
				SetAllowFromEmpty: true,
			}); err != nil {
				return err
			}
			requestCCReload()
			return nil
		})
		mgmtSrv.SetSetupWeixinSave(func(req core.WeixinSetupSaveRequest) error {
			_, err := config.EnsureProjectWithWeixinPlatform(config.EnsureProjectWithWeixinOptions{
				ProjectName: req.ProjectName,
				WorkDir:     req.WorkDir,
				AgentType:   req.AgentType,
			})
			if err != nil {
				return fmt.Errorf("ensure project: %w", err)
			}
			if _, err = config.SaveWeixinPlatformCredentials(config.WeixinCredentialUpdateOptions{
				ProjectName:       req.ProjectName,
				Token:             req.Token,
				BaseURL:           req.BaseURL,
				AccountID:         req.IlinkBotID,
				ScannedUserID:     req.IlinkUserID,
				SetAllowFromEmpty: true,
			}); err != nil {
				return err
			}
			requestCCReload()
			return nil
		})
		mgmtSrv.SetAddPlatformToProject(func(projectName, platType string, opts map[string]any, workDir, agentType string) error {
			if opts == nil {
				opts = map[string]any{}
			}
			// Auto-fill the required "command" for ACP-driven agents from the
			// detected binary path, so the project never bricks the engine on a
			// missing path (issue #24). Only fills when absent; Devin etc. that
			// derive their own default are unaffected.
			if cmd, _ := opts["command"].(string); strings.TrimSpace(cmd) == "" {
				if detected := agent.DefaultCatalog().CommandForACPAgent(agentType); detected != "" {
					opts["command"] = detected
				}
			}
			if err := config.AddPlatformToProject(projectName, config.PlatformConfig{Type: platType, Options: opts}, workDir, agentType); err != nil {
				return err
			}
			requestCCReload()
			return nil
		})
		mgmtSrv.SetRemoveProject(config.RemoveProject)
		// Per-channel agent binding for the cc-connect Web UI: writes the
		// channel's [projects.platforms.agent] override (by index) and hot-reloads
		// the engine. Reuses the same path the (now-removed) 1agents panel used.
		mgmtSrv.SetSetChannelAgent(SetChannelAgentBinding)
		mgmtSrv.SetSaveProjectSettings(func(name string, u core.ProjectSettingsUpdate) error {
			return config.SaveProjectSettings(name, config.ProjectSettingsUpdate{
				Language:             u.Language,
				AdminFrom:            u.AdminFrom,
				DisabledCommands:     u.DisabledCommands,
				WorkDir:              u.WorkDir,
				Mode:                 u.Mode,
				AgentType:            u.AgentType,
				ShowContextIndicator: u.ShowContextIndicator,
				ReplyFooter:          u.ReplyFooter,
				InjectSender:         u.InjectSender,
				PlatformAllowFrom:    u.PlatformAllowFrom,
			})
		})
		mgmtSrv.SetGetProjectConfig(config.GetProjectConfigDetails)
		mgmtSrv.SetSaveProviderRefs(config.SaveProviderRefs)
		mgmtSrv.SetConfigFilePath(configPath)
		mgmtSrv.SetGetGlobalSettings(config.GetGlobalSettings)
		mgmtSrv.SetSaveGlobalSettings(func(updates map[string]any) error {
			u := config.GlobalSettingsUpdate{}
			if v, ok := updates["language"].(string); ok {
				u.Language = &v
			}
			if v, ok := updates["attachment_send"].(string); ok {
				u.AttachmentSend = &v
			}
			if v, ok := updates["log_level"].(string); ok {
				u.LogLevel = &v
			}
			if v, ok := updates["idle_timeout_mins"].(float64); ok {
				iv := int(v)
				u.IdleTimeoutMins = &iv
			}
			if v, ok := updates["thinking_messages"].(bool); ok {
				u.ThinkingMessages = &v
			}
			if v, ok := updates["thinking_max_len"].(float64); ok {
				iv := int(v)
				u.ThinkingMaxLen = &iv
			}
			if v, ok := updates["tool_messages"].(bool); ok {
				u.ToolMessages = &v
			}
			if v, ok := updates["tool_max_len"].(float64); ok {
				iv := int(v)
				u.ToolMaxLen = &iv
			}
			if v, ok := updates["stream_preview_enabled"].(bool); ok {
				u.StreamPreviewOn = &v
			}
			if v, ok := updates["stream_preview_interval_ms"].(float64); ok {
				iv := int(v)
				u.StreamPreviewIntMs = &iv
			}
			if v, ok := updates["rate_limit_max_messages"].(float64); ok {
				iv := int(v)
				u.RateLimitMax = &iv
			}
			if v, ok := updates["rate_limit_window_secs"].(float64); ok {
				iv := int(v)
				u.RateLimitWindow = &iv
			}
			return config.SaveGlobalSettings(u)
		})
		mgmtSrv.SetListGlobalProviders(func() ([]core.GlobalProviderInfo, error) {
			providers, err := config.ListGlobalProviders()
			if err != nil {
				return nil, err
			}
			out := make([]core.GlobalProviderInfo, len(providers))
			for i, p := range providers {
				out[i] = configProviderToGlobal(p)
			}
			return out, nil
		})
		mgmtSrv.SetAddGlobalProvider(func(info core.GlobalProviderInfo) error {
			p := globalProviderToConfig(info)
			err := config.AddGlobalProvider(p)
			if err == nil {
				syncProviderToCCSwitch(p)
			}
			return err
		})
		mgmtSrv.SetUpdateGlobalProvider(func(name string, info core.GlobalProviderInfo) error {
			p := globalProviderToConfig(info)
			err := config.UpdateGlobalProvider(name, p)
			if err == nil {
				syncProviderToCCSwitch(p)
			}
			return err
		})
		mgmtSrv.SetRemoveGlobalProvider(func(name string) error {
			err := config.RemoveGlobalProvider(name)
			if err == nil {
				deleteProviderFromCCSwitch(name)
			}
			return err
		})
		mgmtSrv.SetFetchPresets(core.FetchProviderPresets)
		mgmtSrv.SetFetchSkillPresets(core.FetchSkillPresets)
		if cfg.ProviderPresetsURL != "" {
			core.SetPresetsURL(cfg.ProviderPresetsURL)
		}
		mgmtSrv.SetListCCSwitchProviders(listCCSwitchProvidersForWeb)
		mgmtSrv.SetGetCCSwitchSettings(getCCSwitchSettingsForWeb)
		mgmtSrv.SetSaveCCSwitchSettings(saveCCSwitchSettingsForWeb)
		mgmtSrv.SetSwitchCCSwitchProvider(switchCCSwitchProviderForWeb)
		mgmtSrv.Start()
	}

	slog.Info("cc-connect is running inside 1Agent", "projects", len(engines))

	if notify := core.ConsumeRestartNotify(cfg.DataDir); notify != nil {
		slog.Info("post-restart: sending success notification", "platform", notify.Platform, "session", notify.SessionKey)
		for _, e := range engines {
			e.SendRestartNotification(notify.Platform, notify.SessionKey)
		}
	}

	// Block until context is done or restart requested, then stop servers
	var restartReq *core.RestartRequest
	select {
	case <-ctx.Done():
	case req := <-core.RestartCh:
		restartReq = &req
		slog.Info("restart requested via cc-connect management API", "session", req.SessionKey, "platform", req.Platform)
	}

	if restartReq != nil {
		// Allow the HTTP server to flush the successful response back to the client
		time.Sleep(300 * time.Millisecond)
	}

	slog.Info("shutting down cc-connect engines...")
	if apiSrv != nil {
		apiSrv.Stop()
	}
	if mgmtSrv != nil {
		mgmtSrv.Stop()
	}
	if bridgeSrv != nil {
		bridgeSrv.Stop()
	}
	heartbeatSched.Stop()
	if cronSched != nil {
		cronSched.Stop()
	}
	for _, e := range engines {
		e.Stop()
	}

	if restartReq != nil {
		if err := core.SaveRestartNotify(cfg.DataDir, *restartReq); err != nil {
			slog.Error("restart: save notify failed", "error", err)
		}
		return true
	}
	return false
}

// ── In-Process Helper Functions (Copied/Adapted from main.go) ────────────────

func buildAgentOptions(dataDir string, proj config.ProjectConfig) map[string]any {
	opts := make(map[string]any, len(proj.Agent.Options)+2)
	for k, v := range proj.Agent.Options {
		opts[k] = v
	}
	opts["cc_data_dir"] = dataDir
	opts["cc_project"] = proj.Name
	return opts
}

// buildChannelAgentOptions builds the agent options for a channel-level agent
// override (#277). The channel's own [projects.platforms.agent.options] win, but
// it inherits the project's work_dir and run_as_* isolation when unset, so a
// channel agent runs in the same workspace directory as the project default.
func buildChannelAgentOptions(dataDir string, proj config.ProjectConfig, agentCfg config.AgentConfig) map[string]any {
	opts := make(map[string]any, len(agentCfg.Options)+4)
	for k, v := range agentCfg.Options {
		opts[k] = v
	}
	if _, ok := opts["work_dir"]; !ok {
		if wd, ok := proj.Agent.Options["work_dir"].(string); ok && wd != "" {
			opts["work_dir"] = wd
		}
	}
	if proj.RunAsUser != "" {
		if _, ok := opts["run_as_user"]; !ok {
			opts["run_as_user"] = proj.RunAsUser
		}
		if len(proj.RunAsEnv) > 0 {
			if _, ok := opts["run_as_env"]; !ok {
				opts["run_as_env"] = proj.RunAsEnv
			}
		}
	}
	opts["cc_data_dir"] = dataDir
	opts["cc_project"] = proj.Name
	return opts
}

func wireAgentProviders(agent core.Agent, agentCfg config.AgentConfig) providerWiringResult {
	result := providerWiringResult{canStartInitialRefresh: true}
	active, _ := agentCfg.Options["provider"].(string)
	result.explicitProviderRequested = active != ""

	ps, ok := agent.(core.ProviderSwitcher)
	if !ok || len(agentCfg.Providers) == 0 {
		return result
	}

	providers := make([]core.ProviderConfig, len(agentCfg.Providers))
	for i, p := range agentCfg.Providers {
		providers[i] = configProviderToCore(p)
	}
	ps.SetProviders(providers)
	if result.explicitProviderRequested {
		result.activeProviderApplied = ps.SetActiveProvider(active)
		result.canStartInitialRefresh = result.activeProviderApplied
	}
	return result
}

func configProviderToCore(p config.ProviderConfig) core.ProviderConfig {
	c := core.ProviderConfig{
		Name: p.Name, APIKey: p.APIKey, BaseURL: p.BaseURL,
		Model: p.Model, Models: convertProviderModels(p.Models),
		Thinking: p.Thinking, Env: p.Env,
	}
	if p.Codex != nil {
		c.CodexWireAPI = p.Codex.WireAPI
		c.CodexHTTPHeaders = p.Codex.HTTPHeaders
	}
	return c
}

func convertProviderModels(ms []config.ProviderModelConfig) []core.ModelOption {
	if len(ms) == 0 {
		return nil
	}
	opts := make([]core.ModelOption, len(ms))
	for i, m := range ms {
		opts[i] = core.ModelOption{Name: m.Model, Alias: m.Alias}
	}
	return opts
}

func startInitialRefreshIfReady(agent core.Agent, result providerWiringResult) {
	if !result.canStartInitialRefresh {
		return
	}
	if starter, ok := agent.(initialModelRefreshStarter); ok {
		starter.StartInitialModelRefresh()
	}
}

func projectStatePath(dataDir, projectName string) string {
	replacer := strings.NewReplacer(
		"\\", "_",
		"/", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	name := strings.TrimSpace(projectName)
	name = replacer.Replace(name)
	if name == "" {
		name = "project"
	}
	return filepath.Join(dataDir, "projects", name+".state.json")
}

func applyProjectStateOverride(projectName string, agent core.Agent, configuredWorkDir string, store *core.ProjectStateStore) string {
	effectiveWorkDir := configuredWorkDir
	if store == nil {
		return effectiveWorkDir
	}

	switcher, ok := agent.(core.WorkDirSwitcher)
	if !ok {
		return effectiveWorkDir
	}

	override := store.WorkDirOverride()
	if override == "" {
		return effectiveWorkDir
	}
	if abs, err := filepath.Abs(override); err == nil {
		override = abs
	}

	info, err := os.Stat(override)
	if err != nil || !info.IsDir() {
		slog.Warn("project_state: ignoring invalid work_dir override", "project", projectName, "work_dir", override)
		return effectiveWorkDir
	}

	switcher.SetWorkDir(override)
	slog.Info("project_state: applied work_dir override", "project", projectName, "work_dir", override)
	return override
}

func sessionStorePath(dataDir, name, workDir string) string {
	var filename string
	if workDir == "" {
		filename = name + ".json"
	} else {
		abs, err := filepath.Abs(workDir)
		if err != nil {
			abs = workDir
		}
		h := sha256.Sum256([]byte(abs))
		short := hex.EncodeToString(h[:4])
		filename = fmt.Sprintf("%s_%s.json", name, short)
	}

	for _, legacy := range []string{
		filepath.Join(dataDir, filename),
		filepath.Join(dataDir, strings.TrimSuffix(filename, ".json")+".sessions.json"),
	} {
		if _, err := os.Stat(legacy); err == nil {
			slog.Info("session: using legacy file in dataDir", "path", legacy)
			return legacy
		}
	}

	return filepath.Join(dataDir, "sessions", filename)
}

func buildHeartbeatConfig(hc config.HeartbeatConfig) core.HeartbeatConfig {
	cfg := core.HeartbeatConfig{
		IntervalMins: 30,
		OnlyWhenIdle: true,
		Silent:       true,
		TimeoutMins:  30,
		SessionKey:   hc.SessionKey,
		Prompt:       hc.Prompt,
	}
	if hc.Enabled != nil {
		cfg.Enabled = *hc.Enabled
	}
	if hc.IntervalMins != nil {
		cfg.IntervalMins = *hc.IntervalMins
	}
	if hc.OnlyWhenIdle != nil {
		cfg.OnlyWhenIdle = *hc.OnlyWhenIdle
	}
	if hc.Silent != nil {
		cfg.Silent = *hc.Silent
	}
	if hc.TimeoutMins != nil {
		cfg.TimeoutMins = *hc.TimeoutMins
	}
	return cfg
}

func derefInt(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

func resolveResetOnIdle(configured *int) (time.Duration, bool) {
	if configured != nil {
		return time.Duration(*configured) * time.Minute, false
	}
	return time.Duration(defaultResetOnIdleMins) * time.Minute, true
}

func reloadConfig(configPath, projName string, engine *core.Engine) (*core.ConfigReloadResult, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("reload config: %w", err)
	}
	syncAllProvidersToCCSwitch(cfg.Providers)

	result := &core.ConfigReloadResult{}

	var proj *config.ProjectConfig
	for i := range cfg.Projects {
		if cfg.Projects[i].Name == projName {
			proj = &cfg.Projects[i]
			break
		}
	}
	if proj == nil {
		return nil, fmt.Errorf("project %q not found in config", projName)
	}

	mode, tm, tool, tmlen, toollen, showCtx, showFooter, _ := config.EffectiveDisplay(cfg, proj)
	engine.SetDisplayConfig(core.DisplayCfg{
		Mode:             mode,
		CardMode:         config.EffectiveCardMode(cfg, proj),
		ThinkingMessages: tm,
		ThinkingMaxLen:   tmlen,
		ToolMaxLen:       toollen,
		ToolMessages:     tool,
	})
	result.DisplayUpdated = true

	engine.SetShowContextIndicator(showCtx)
	engine.SetReplyFooterEnabled(showFooter)

	if proj.AutoCompress.Enabled != nil && *proj.AutoCompress.Enabled {
		minGap := 30 * time.Minute
		if proj.AutoCompress.MinGapMins != nil {
			minGap = time.Duration(*proj.AutoCompress.MinGapMins) * time.Minute
		}
		maxTokens := derefInt(proj.AutoCompress.MaxTokens)
		if maxTokens <= 0 {
			maxTokens = 12000
		}
		engine.SetAutoCompressConfig(true, maxTokens, minGap)
	} else {
		engine.SetAutoCompressConfig(false, 0, 0)
	}
	resetIdle, defaulted := resolveResetOnIdle(proj.ResetOnIdleMins)
	engine.SetResetOnIdle(resetIdle)
	if defaulted {
		slog.Info("project: reset_on_idle_mins not set, applying default", "project", proj.Name)
	}

	if cfg.InstantReply.Enabled != nil && *cfg.InstantReply.Enabled {
		engine.SetInstantReply(core.InstantReplyCfg{
			Enabled: true,
			Content: cfg.InstantReply.Content,
		})
	} else {
		engine.SetInstantReply(core.InstantReplyCfg{})
	}

	engine.SetInjectSender(proj.InjectSender != nil && *proj.InjectSender)
	engine.SetAttachmentSendEnabled(cfg.AttachmentSend != "off")

	return result, nil
}

func configProviderToGlobal(p config.ProviderConfig) core.GlobalProviderInfo {
	info := core.GlobalProviderInfo{
		ID:          p.ID,
		Name:        p.Name,
		APIKey:      p.APIKey,
		BaseURL:     p.BaseURL,
		Model:       p.Model,
		Thinking:    p.Thinking,
		Env:         p.Env,
		AgentTypes:  p.AgentTypes,
		Endpoints:   p.Endpoints,
		AgentModels: p.AgentModels,
	}
	for _, m := range p.Models {
		info.Models = append(info.Models, struct {
			Model string `json:"model"`
			Alias string `json:"alias,omitempty"`
		}{Model: m.Model, Alias: m.Alias})
	}
	if len(p.AgentModelLists) > 0 {
		info.AgentModelLists = make(map[string][]core.GlobalModelEntry, len(p.AgentModelLists))
		for at, ml := range p.AgentModelLists {
			entries := make([]core.GlobalModelEntry, len(ml))
			for i, m := range ml {
				entries[i] = core.GlobalModelEntry{Model: m.Model, Alias: m.Alias}
			}
			info.AgentModelLists[at] = entries
		}
	}
	if p.Codex != nil {
		info.Codex = &core.GlobalCodexConfig{
			WireAPI:     p.Codex.WireAPI,
			HTTPHeaders: p.Codex.HTTPHeaders,
		}
	}
	return info
}

func globalProviderToConfig(info core.GlobalProviderInfo) config.ProviderConfig {
	p := config.ProviderConfig{
		ID:          info.ID,
		Name:        info.Name,
		APIKey:      info.APIKey,
		BaseURL:     info.BaseURL,
		Model:       info.Model,
		Thinking:    info.Thinking,
		Env:         info.Env,
		AgentTypes:  info.AgentTypes,
		Endpoints:   info.Endpoints,
		AgentModels: info.AgentModels,
	}
	for _, m := range info.Models {
		p.Models = append(p.Models, config.ProviderModelConfig{Model: m.Model, Alias: m.Alias})
	}
	if len(info.AgentModelLists) > 0 {
		p.AgentModelLists = make(map[string][]config.ProviderModelConfig, len(info.AgentModelLists))
		for at, ml := range info.AgentModelLists {
			entries := make([]config.ProviderModelConfig, len(ml))
			for i, m := range ml {
				entries[i] = config.ProviderModelConfig{Model: m.Model, Alias: m.Alias}
			}
			p.AgentModelLists[at] = entries
		}
	}
	if info.Codex != nil {
		p.Codex = &config.CodexProviderConfig{
			WireAPI:     info.Codex.WireAPI,
			HTTPHeaders: info.Codex.HTTPHeaders,
		}
	}
	return p
}

type ccSwitchRow struct {
	ID             string `json:"id"`
	AppType        string `json:"app_type"`
	Name           string `json:"name"`
	SettingsConfig string `json:"settings_config"`
	IsCurrent      int    `json:"is_current"`
}

func queryCCSwitchDB(dbPath, appTypeFilter string) ([]ccSwitchRow, error) {
	db, err := sql.Open("sqlite", dbPath+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open cc-switch db: %w", err)
	}
	defer db.Close()

	query := "SELECT id, app_type, name, settings_config, is_current FROM providers"
	var args []any
	if appTypeFilter != "" {
		query += " WHERE app_type = ?"
		args = append(args, appTypeFilter)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query cc-switch db: %w", err)
	}
	defer rows.Close()

	var result []ccSwitchRow
	for rows.Next() {
		var r ccSwitchRow
		if err := rows.Scan(&r.ID, &r.AppType, &r.Name, &r.SettingsConfig, &r.IsCurrent); err != nil {
			continue
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func convertCCSwitchProvider(row ccSwitchRow) (config.ProviderConfig, error) {
	var sc map[string]any
	if err := json.Unmarshal([]byte(row.SettingsConfig), &sc); err != nil {
		return config.ProviderConfig{}, fmt.Errorf("invalid settings_config JSON: %w", err)
	}

	p := config.ProviderConfig{
		Name: strings.ToLower(strings.ReplaceAll(strings.TrimSpace(row.Name), " ", "-")),
	}

	switch row.AppType {
	case "claude":
		return convertClaudeProvider(p, sc)
	case "codex":
		return convertCodexProvider(p, sc)
	default:
		return config.ProviderConfig{}, fmt.Errorf("unsupported app_type %q (only claude and codex are supported)", row.AppType)
	}
}

func convertClaudeProvider(p config.ProviderConfig, sc map[string]any) (config.ProviderConfig, error) {
	env, _ := sc["env"].(map[string]any)
	if env == nil {
		return p, fmt.Errorf("no env in settings_config")
	}

	if key, ok := env["ANTHROPIC_AUTH_TOKEN"].(string); ok && key != "" {
		p.APIKey = key
	}
	if url, ok := env["ANTHROPIC_BASE_URL"].(string); ok && url != "" {
		p.BaseURL = url
	}
	if model, ok := env["ANTHROPIC_MODEL"].(string); ok && model != "" {
		p.Model = model
	}

	extra := make(map[string]string)
	known := map[string]bool{"ANTHROPIC_AUTH_TOKEN": true, "ANTHROPIC_BASE_URL": true, "ANTHROPIC_MODEL": true}
	for k, v := range env {
		if !known[k] {
			if s, ok := v.(string); ok && s != "" {
				extra[k] = s
			}
		}
	}
	if len(extra) > 0 {
		p.Env = extra
	}

	if p.APIKey == "" && len(p.Env) == 0 {
		return p, fmt.Errorf("no API key or env found")
	}
	return p, nil
}

func convertCodexProvider(p config.ProviderConfig, sc map[string]any) (config.ProviderConfig, error) {
	if auth, ok := sc["auth"].(map[string]any); ok {
		if key, ok := auth["OPENAI_API_KEY"].(string); ok && key != "" {
			p.APIKey = key
		}
	}

	if cfgStr, ok := sc["config"].(string); ok && cfgStr != "" {
		p.BaseURL, p.Model = parseCodexConfigTOML(cfgStr)
	}

	if p.APIKey == "" {
		return p, fmt.Errorf("no OPENAI_API_KEY found")
	}
	return p, nil
}

func parseCodexConfigTOML(cfgStr string) (baseURL, model string) {
	for _, line := range strings.Split(cfgStr, "\n") {
		line = strings.TrimSpace(line)
		if k, v, ok := parseTOMLKV(line); ok {
			switch k {
			case "base_url":
				if baseURL == "" {
					baseURL = v
				}
			case "model":
				if model == "" {
					model = v
				}
			}
		}
	}
	return
}

func parseTOMLKV(line string) (key, value string, ok bool) {
	idx := strings.Index(line, "=")
	if idx < 0 || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
		return "", "", false
	}
	key = strings.TrimSpace(line[:idx])
	value = strings.TrimSpace(line[idx+1:])
	value = strings.Trim(value, "\"'")
	return key, value, true
}

func findCCSwitchDB() string {
	for _, p := range ccSwitchDBCandidates() {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func ccSwitchDBCandidates() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	candidates := []string{
		filepath.Join(home, ".cc-switch", "cc-switch.db"),
	}

	switch runtime.GOOS {
	case "linux":
		dataHome := os.Getenv("XDG_DATA_HOME")
		if dataHome == "" {
			dataHome = filepath.Join(home, ".local", "share")
		}
		candidates = append(candidates, filepath.Join(dataHome, "cc-switch", "cc-switch.db"))
	case "darwin":
		candidates = append(candidates, filepath.Join(home, "Library", "Application Support", "cc-switch", "cc-switch.db"))
	}

	return candidates
}

func listCCSwitchProvidersForWeb() ([]core.CCSwitchProviderInfo, error) {
	dbPath := findCCSwitchDB()
	if dbPath == "" {
		return nil, fmt.Errorf("cc-switch database not found")
	}

	rows, err := queryCCSwitchDB(dbPath, "")
	if err != nil {
		return nil, err
	}

	result := make([]core.CCSwitchProviderInfo, 0, len(rows))
	for _, row := range rows {
		p, err := convertCCSwitchProvider(row)
		if err != nil {
			continue
		}
		result = append(result, core.CCSwitchProviderInfo{
			ID:        row.ID,
			Name:      p.Name,
			AppType:   row.AppType,
			APIKey:    p.APIKey,
			BaseURL:   p.BaseURL,
			Model:     p.Model,
			IsCurrent: row.IsCurrent == 1,
		})
	}
	return result, nil
}

func sanitizeID(name string) string {
	id := strings.ToLower(name)
	id = strings.ReplaceAll(id, " ", "-")
	var sb strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func syncProviderToCCSwitch(p config.ProviderConfig) {
	dbPath := findCCSwitchDB()
	if dbPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return
		}
		dir := filepath.Join(home, ".cc-switch")
		_ = os.MkdirAll(dir, 0755)
		dbPath = filepath.Join(dir, "cc-switch.db")
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Printf("[ccconnect] sync to cc-switch failed to open db: %v", err)
		return
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS providers (
		id TEXT NOT NULL,
		app_type TEXT NOT NULL,
		name TEXT NOT NULL,
		settings_config TEXT NOT NULL,
		website_url TEXT,
		category TEXT,
		created_at INTEGER,
		sort_index INTEGER,
		notes TEXT,
		icon TEXT,
		icon_color TEXT,
		meta TEXT NOT NULL DEFAULT '{}',
		is_current BOOLEAN NOT NULL DEFAULT 0,
		in_failover_queue BOOLEAN NOT NULL DEFAULT 0,
		PRIMARY KEY (id, app_type)
	)`)
	if err != nil {
		log.Printf("[ccconnect] sync to cc-switch failed to create table: %v", err)
		return
	}

	id := p.ID
	if id == "" {
		id = sanitizeID(p.Name)
	}
	if id == "" {
		return
	}

	var targetApps []string
	if len(p.AgentTypes) > 0 {
		for _, at := range p.AgentTypes {
			switch at {
			case "claudecode":
				targetApps = append(targetApps, "claude")
			case "codex":
				targetApps = append(targetApps, "codex")
			case "gemini":
				targetApps = append(targetApps, "gemini")
			}
		}
	} else {
		targetApps = []string{"claude", "codex", "gemini"}
	}

	for _, app := range targetApps {
		var settingsMap map[string]any
		switch app {
		case "claude":
			env := map[string]string{
				"ANTHROPIC_BASE_URL":             p.BaseURL,
				"ANTHROPIC_AUTH_TOKEN":           p.APIKey,
				"ANTHROPIC_MODEL":                p.Model,
				"ANTHROPIC_DEFAULT_HAIKU_MODEL":  p.Model,
				"ANTHROPIC_DEFAULT_SONNET_MODEL": p.Model,
				"ANTHROPIC_DEFAULT_OPUS_MODEL":   p.Model,
			}
			for k, v := range p.Env {
				env[k] = v
			}
			settingsMap = map[string]any{
				"env": env,
			}
		case "codex":
			settingsMap = map[string]any{
				"auth": map[string]string{
					"OPENAI_API_KEY": p.APIKey,
				},
				"config": fmt.Sprintf("base_url = %q\nmodel = %q\n", p.BaseURL, p.Model),
			}
		case "gemini":
			settingsMap = map[string]any{
				"env": map[string]string{
					"GOOGLE_GEMINI_BASE_URL": p.BaseURL,
					"GEMINI_API_KEY":         p.APIKey,
					"GEMINI_MODEL":           p.Model,
				},
			}
		default:
			settingsMap = map[string]any{
				"api_key":  p.APIKey,
				"base_url": p.BaseURL,
				"model":    p.Model,
			}
		}

		scBytes, err := json.Marshal(settingsMap)
		if err != nil {
			continue
		}
		scStr := string(scBytes)

		query := `INSERT INTO providers (id, app_type, name, settings_config, meta)
			VALUES (?, ?, ?, ?, '{"commonConfigEnabled":true}')
			ON CONFLICT(id, app_type) DO UPDATE SET
				name = excluded.name,
				settings_config = excluded.settings_config,
				meta = excluded.meta`
		_, err = db.Exec(query, id, app, p.Name, scStr)
		if err != nil {
			log.Printf("[ccconnect] sync provider %s for app %s to cc-switch failed: %v", p.Name, app, err)
		}
	}
}

func deleteProviderFromCCSwitch(name string) {
	id := sanitizeID(name)
	dbPath := findCCSwitchDB()
	if dbPath == "" {
		return
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return
	}
	defer db.Close()

	_, _ = db.Exec("DELETE FROM providers WHERE id = ?", id)
}

func syncAllProvidersToCCSwitch(providers []config.ProviderConfig) {
	for _, p := range providers {
		syncProviderToCCSwitch(p)
	}
}

func runCCSwitchSwitchCommand(appType, providerID string) {
	binPath, err := exec.LookPath("cc-switch")
	if err != nil {
		binPath = "./build/cc-switch"
		if _, err := os.Stat(binPath); err != nil {
			log.Printf("[ccconnect] cc-switch binary not found in PATH or build/")
			return
		}
	}

	log.Printf("[ccconnect] executing %s --app %s provider switch %s", binPath, appType, providerID)
	cmd := exec.Command(binPath, "--app", appType, "provider", "switch", providerID)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Printf("[ccconnect] cc-switch switch command failed: %v", err)
	} else {
		log.Printf("[ccconnect] cc-switch switch command executed successfully")
	}
}

func getCCSwitchSettingsForWeb() (map[string]string, error) {
	dbPath := findCCSwitchDB()
	if dbPath == "" {
		return nil, fmt.Errorf("cc-switch database not found")
	}

	db, err := sql.Open("sqlite", dbPath+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open cc-switch db: %w", err)
	}
	defer db.Close()

	rows, err := db.Query("SELECT key, value FROM settings WHERE key LIKE 'common_config_%'")
	if err != nil {
		return nil, fmt.Errorf("query cc-switch settings: %w", err)
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var key, val string
		if err := rows.Scan(&key, &val); err == nil {
			result[key] = val
		}
	}
	return result, nil
}

func saveCCSwitchSettingsForWeb(updates map[string]string) error {
	dbPath := findCCSwitchDB()
	if dbPath == "" {
		return fmt.Errorf("cc-switch database not found")
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("open cc-switch db: %w", err)
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for k, v := range updates {
		_, err := tx.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value`, k, v)
		if err != nil {
			return fmt.Errorf("update settings key %s: %w", k, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Trigger live reload by calling the switch command for each updated appType
	// if there's a currently active provider for that app.
	for k := range updates {
		if strings.HasPrefix(k, "common_config_") {
			appType := strings.TrimPrefix(k, "common_config_")
			var currentProviderID string
			err := db.QueryRow("SELECT id FROM providers WHERE app_type = ? AND is_current = 1 LIMIT 1", appType).Scan(&currentProviderID)
			if err == nil && currentProviderID != "" {
				go runCCSwitchSwitchCommand(appType, currentProviderID)
			}
		}
	}

	return nil
}

func switchCCSwitchProviderForWeb(appType, providerID string) error {
	go runCCSwitchSwitchCommand(appType, providerID)
	return nil
}

// CCProjectSlug turns a workspace name into a cc-connect-safe project name.
// Since #277 the agent type is no longer encoded as a __<agent> suffix — the
// project name equals the (sanitized) workspace name and the agent type lives
// in the per-channel [projects.platforms.agent] binding.
// CCProjectName returns the canonical cc-connect project name for a workspace.
// A name that slugs losslessly (already ascii-safe, e.g. "Coze") is used as-is;
// ANY lossy slug falls back to the workspace id. The old rule only caught fully
// non-ASCII names (slug == "ws"), so a mixed name like "办公2" produced the
// degenerate "_2" and the panel 404'd on /projects/_2. Workspace ids are ascii
// (assistant badge / hex / "default") and badge folders are named by id, so the
// id fallback lines up with both the register path (workspace.
// registerWorkspaceProject) and the reconciler's dir-name slug — all three
// address the same project name. Callers addressing a workspace's cc-connect
// project (e.g. the panel route) MUST use this, not CCProjectSlug(name) alone.
func CCProjectName(workspaceName, workspaceID string) string {
	projName := CCProjectSlug(workspaceName)
	if projName != workspaceName {
		projName = CCProjectSlug(workspaceID)
	}
	return projName
}

// hasASCIIAlnum reports whether s contains at least one ASCII letter or digit.
// A name without one (e.g. "_") is a degenerate slug placeholder, not a real
// project name.
func hasASCIIAlnum(s string) bool {
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return true
		}
	}
	return false
}

func CCProjectSlug(workspaceName string) string {
	var sb strings.Builder
	inInvalidSeq := false
	hasAlnum := false
	for _, r := range workspaceName {
		isAlnum := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
		isValid := isAlnum || r == '_' || r == '-'
		if isValid {
			sb.WriteRune(r)
			inInvalidSeq = false
			if isAlnum {
				hasAlnum = true
			}
		} else {
			if !inInvalidSeq {
				sb.WriteRune('_')
				inInvalidSeq = true
			}
		}
	}
	slug := sb.String()
	if len(slug) > 32 {
		slug = slug[:32]
	}
	// A name with no ASCII-alphanumeric content (e.g. the built-in "对话"
	// workspace) collapses to just "_"; treat it as empty so callers fall back
	// to the workspace id instead of minting a "_" project. A "_" project name
	// round-trips into an empty workspace id on re-import (see the import loop),
	// which poisons /api/agent/sessions (workspace_id="").
	if !hasAlnum {
		slug = "ws"
	}
	return slug
}

// reconcileProjectsByPath rebuilds the cc-connect project list from the 1agents
// workspace registry, matching by work_dir PATH rather than name (#277):
//
//   - one workspace ⇄ one project; the project name equals the (sanitized)
//     workspace name, with no __<agent> suffix;
//   - an existing project at the same path keeps its agent + channel bindings
//     (only work_dir is refreshed and a bridge channel ensured), so a channel
//     bound to e.g. codex survives a resync — its name is left untouched to
//     avoid orphaning session/state files;
//   - projects without a work_dir (the temp placeholder, platform-only configs)
//     are preserved verbatim;
//   - workspace-backed projects whose path is no longer registered are dropped,
//     preserving the existing delete-on-workspace-removal semantics.
func reconcileProjectsByPath(projects []config.ProjectConfig, workspaces []workspace.Workspace) []config.ProjectConfig {
	projByPath := make(map[string]*config.ProjectConfig, len(projects))
	for i := range projects {
		if wd, _ := projects[i].Agent.Options["work_dir"].(string); wd != "" {
			projByPath[normalizePath(wd)] = &projects[i]
		}
	}

	var out []config.ProjectConfig
	for _, ws := range workspaces {
		if ws.Path == "" {
			continue
		}
		projName := CCProjectName(ws.Name, ws.ID)

		if p, ok := projByPath[normalizePath(ws.Path)]; ok {
			if p.Agent.Options == nil {
				p.Agent.Options = make(map[string]any)
			}
			p.Agent.Options["work_dir"] = ws.Path

			hasBridge := false
			for _, plat := range p.Platforms {
				if plat.Type == "bridge" {
					hasBridge = true
					break
				}
			}
			if !hasBridge {
				p.Platforms = append(p.Platforms, config.PlatformConfig{Type: "bridge"})
			}
			// Repair a degenerate placeholder name (e.g. "_", minted before the
			// CCProjectSlug fix for a non-ASCII workspace such as the default
			// "对话"): rename it to the canonical slug so the project stays
			// consistent with its workspace. Names with any alnum char are a real
			// (possibly user/channel-chosen) name and are left untouched to avoid
			// orphaning session/state files.
			if !hasASCIIAlnum(p.Name) && p.Name != projName {
				log.Printf("[ccconnect] Renaming placeholder project %q → %q (path %s)", p.Name, projName, ws.Path)
				p.Name = projName
			}
			out = append(out, *p)
		} else {
			out = append(out, config.ProjectConfig{
				Name: projName,
				Agent: config.AgentConfig{
					Type: "claudecode",
					Options: map[string]any{
						"work_dir": ws.Path,
						"mode":     "default",
					},
				},
				Platforms: []config.PlatformConfig{{Type: "bridge"}},
			})
		}
	}

	// Preserve projects with no work_dir (placeholder / platform-only). Those
	// with a work_dir were either rebuilt above (path owned) or are orphans for
	// a removed workspace and are intentionally dropped.
	for i := range projects {
		if wd, _ := projects[i].Agent.Options["work_dir"].(string); wd == "" {
			out = append(out, projects[i])
		}
	}
	return out
}

// normalizePath canonicalizes a filesystem path for identity comparison between
// 1agents workspaces and cc-connect project work_dirs (#277 sync-by-path). It
// resolves to an absolute, cleaned path; an empty input stays empty.
func normalizePath(p string) string {
	if p == "" {
		return ""
	}
	out := filepath.Clean(p)
	if abs, err := filepath.Abs(p); err == nil {
		out = filepath.Clean(abs)
	}
	// macOS and Windows filesystems are case-insensitive by default, so paths
	// differing only in case (e.g. /Users/scott/Coze vs …/coze) are the SAME
	// directory. Case-fold the comparison KEY on those platforms so they dedup
	// to one project; Linux stays case-sensitive. Only the key is folded — the
	// stored ws.Path / work_dir keep their real on-disk case.
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		out = strings.ToLower(out)
	}
	return out
}
