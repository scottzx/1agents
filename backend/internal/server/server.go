package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/scottzx/1Agents/backend/internal/agent"
	"github.com/scottzx/1Agents/backend/internal/appkit"
	"github.com/scottzx/1Agents/backend/internal/appregistry"
	"github.com/scottzx/1Agents/backend/internal/apps/speechclip"
	"github.com/scottzx/1Agents/backend/internal/auth"
	"github.com/scottzx/1Agents/backend/internal/ccconnect"
	"github.com/scottzx/1Agents/backend/internal/config"
	"github.com/scottzx/1Agents/backend/internal/contacts"
	ctxt "github.com/scottzx/1Agents/backend/internal/context"
	"github.com/scottzx/1Agents/backend/internal/data"
	"github.com/scottzx/1Agents/backend/internal/digest"
	"github.com/scottzx/1Agents/backend/internal/fs"
	"github.com/scottzx/1Agents/backend/internal/gateway"
	"github.com/scottzx/1Agents/backend/internal/git"
	"github.com/scottzx/1Agents/backend/internal/ingest"
	"github.com/scottzx/1Agents/backend/internal/localtoken"
	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/sources"
	"github.com/scottzx/1Agents/backend/internal/system"
	"github.com/scottzx/1Agents/backend/internal/taskapi"
	"github.com/scottzx/1Agents/backend/internal/templateregistry"
	"github.com/scottzx/1Agents/backend/internal/terminal"
	"github.com/scottzx/1Agents/backend/internal/tunnel"
	"github.com/scottzx/1Agents/backend/internal/workspace"
)

// NewRouter builds and returns the main HTTP request multiplexer.
//
// Route hierarchy (evaluated top-to-bottom):
//
//	/api/fs/*         → File system CRUD handlers (Go, local I/O)
//	/api/workspace/*  → Workspace CRUD handlers (Go, JSON file storage)
//	/api/agent/*      → 1agents-side chat session index (Go, JSON file storage)
//	/api/terminal/*   → Tmux terminal session management (create/list/kill/switch)
//	/api/system/*     → System management: version info, OTA self-update
//	/ws               → Reverse-proxy to ttyd WebSocket endpoint
//	/token            → Reverse-proxy to ttyd auth-token endpoint
//	/                 → Static file server (compiled frontend assets)
func NewRouter(cfg *config.Config) http.Handler {
	mux := http.NewServeMux()

	// ── File system API ──────────────────────────────────────────────────────
	fsHandler := fs.NewHandler(cfg.WorkDir)
	mux.HandleFunc("/api/fs/list", fsHandler.List)              // GET  ?path=.
	mux.HandleFunc("/api/fs/search", fsHandler.Search)          // GET  ?query=xxx&tag=all/doc/img/code
	mux.HandleFunc("/api/fs/read", fsHandler.Read)              // GET  ?path=./main.go
	mux.HandleFunc("/api/fs/view", fsHandler.View)              // GET  ?path=./page.html (serves with correct content-type)
	mux.HandleFunc("/api/fs/view/", fsHandler.View)             // GET  /api/fs/view/relative/path (prefix route for relative assets support)
	mux.HandleFunc("/api/fs/image", fsHandler.Image)            // GET  ?path=./image.png (returns base64 data URL, deprecated)
	mux.HandleFunc("/api/fs/image/", fsHandler.ImageStream)     // GET  /api/fs/image/relative/path (streams raw bytes; preferred)
	mux.HandleFunc("/api/fs/write", fsHandler.Write)            // POST ?path=./main.go
	mux.HandleFunc("/api/fs/upload", fsHandler.Upload)          // POST multipart/form-data (field "file") → saves to /tmp, returns {path,name}
	mux.HandleFunc("/api/fs/upload-to", fsHandler.UploadTo)     // POST multipart/form-data (field "file") → saves to specified path
	mux.HandleFunc("/api/fs/open-folder", fsHandler.OpenFolder) // POST ?path=... → opens folder in Finder/Explorer
	mux.HandleFunc("/api/fs/rename", fsHandler.Rename)          // POST ?oldPath=...&newPath=...
	mux.HandleFunc("/api/fs/mkdir", fsHandler.Mkdir)            // POST ?path=./newdir
	mux.HandleFunc("/api/fs/delete", fsHandler.Delete)          // DELETE ?path=./main.go

	// ── Workspace API ────────────────────────────────────────────────────────
	wsHandler := workspace.NewHandler(cfg.TmuxSession)
	wsHandler.SetSkillsAddr(cfg.SkillsAddr)
	if err := wsHandler.EnsureDefaultWorkspace(); err != nil {
		log.Printf("[server] ensure default workspace: %v", err)
	}
	mux.HandleFunc("/api/workspace/list", wsHandler.List)                                 // GET
	mux.HandleFunc("/api/workspace/create", wsHandler.Create)                             // POST
	mux.HandleFunc("/api/workspace/update", wsHandler.Update)                             // POST
	mux.HandleFunc("/api/workspace/skills", wsHandler.WorkspaceSkills)                    // GET ?id= — synced skills + drift status
	mux.HandleFunc("/api/workspace/push-skill", wsHandler.PushSkill)                      // POST {id, skillRef} — push edited copy back to 母体
	mux.HandleFunc("/api/workspace/pull-skill", wsHandler.PullSkill)                      // POST {id, skillRef} — pull latest from 母体 into workspace copy
	mux.HandleFunc("/api/workspace/available-skills", wsHandler.AvailableSkills)          // GET ?id= — 母体 skills the project can add
	mux.HandleFunc("/api/workspace/add-skill", wsHandler.AddSkill)                        // POST {id, skillRef} — materialize a 母体 skill into the project
	mux.HandleFunc("/api/workspace/remove-skill", wsHandler.RemoveSkill)                  // POST {id, skillRef} — delete a skill from the project only
	mux.HandleFunc("/api/workspace/agents", wsHandler.WorkspaceAgents)                    // GET ?id= — synced agents + drift status
	mux.HandleFunc("/api/workspace/push-agent", wsHandler.PushAgent)                      // POST {id, agentRef} — push edited copy back to 母体
	mux.HandleFunc("/api/workspace/soul", wsHandler.WorkspaceSoul)                        // GET ?id= / POST {id, content} — assistant persona SOUL.md
	mux.HandleFunc("/api/workspace/team", wsHandler.WorkspaceTeam)                        // GET ?id= / POST {id, primary} — agent-team manifest (primary + members)
	mux.HandleFunc("/api/workspace/available-agents", wsHandler.WorkspaceAvailableAgents) // GET ?id= — 母体 agents the project can add
	mux.HandleFunc("/api/workspace/add-agent", wsHandler.AddAgent)                        // POST {id, agentRef} — materialize a 母体 agent into the project
	mux.HandleFunc("/api/workspace/remove-agent", wsHandler.RemoveAgent)                  // POST {id, agentRef} — delete an agent from the project only
	mux.HandleFunc("/api/assistant/souls", wsHandler.ListSouls)                           // GET ?lang= — curated persona presets
	mux.HandleFunc("/api/workspace/reorder", wsHandler.Reorder)                           // POST
	mux.HandleFunc("/api/workspace/delete", wsHandler.Delete)                             // DELETE ?id=xxx
	mux.HandleFunc("/api/workspace/pick-directory", wsHandler.PickDirectory)              // POST — opens native folder picker
	mux.HandleFunc("/api/workspace/list-directories", wsHandler.ListDirectories)          // GET ?path=...
	mux.HandleFunc("/api/workspace/create-directory", wsHandler.CreateDirectory)          // POST
	mux.HandleFunc("/api/workspace/upload-avatar", wsHandler.UploadAvatar)                // POST multipart file → {url}
	mux.Handle("/avatars/", workspace.ServeAvatars())                                     // GET avatar files (embedded presets + uploads)

	// ── App registry API (Wave 2a, #330) ────────────────────────────────────
	mux.HandleFunc("/api/apps", appregistry.HandleList)  // GET → {apps:[...]}
	mux.HandleFunc("/api/apps/", appregistryItemHandler) // POST /{id}/enable|disable

	// ── Template registry API (Wave 2a, #329) ───────────────────────────────
	mux.HandleFunc("/api/templates", templateregistry.HandleList) // GET → {templates:[...]}

	// ── Project config API (Wave 2a, #327) ──────────────────────────────────
	mux.HandleFunc("/api/project/config", projectConfigHandler) // GET ?id=, PUT

	// ── Agent (chat session) index API ─────────────────────────────────────
	// 1agents-side metadata store. The actual conversation lives in
	// cc-connect; these endpoints only index a session created via the
	// cc-connect REST/WS so the sidebar can list it like a terminal session.
	agentStore, err := agent.NewStore()
	if err != nil {
		log.Printf("[server] agent store init failed: %v", err)
	} else {
		// One-time import of the legacy JSON stores into ~/.1agents/meta.db
		// (renames the source files to *.migrated; no-op afterwards).
		if db, dbErr := meta.OpenDefault(); dbErr == nil {
			// Unify the legacy workspaces_dir.json into the projects table first
			// (renames the json to *.migrated), then import the remaining legacy
			// stores (sessions + per-workspace tasks.json), which read the workspace
			// paths back from the projects table.
			if migErr := db.ImportLegacyWorkspaces(); migErr != nil {
				log.Printf("[server] workspace registry migration: %v", migErr)
			}
			if migErr := db.MigrateLegacy(); migErr != nil {
				log.Printf("[server] legacy metadata migration: %v", migErr)
			}
			mux.HandleFunc("/api/projects", meta.ProjectsHandler(db))       // GET, POST
			mux.HandleFunc("/api/search", meta.SearchHandler(db))           // GET ?q=xxx — 对话历史 quick search over tasks + sessions
			mux.HandleFunc("/api/projects/", meta.ProjectActionHandler(db)) // POST {id}/archive|close|reopen

			// Inbox 统一信息收口层 (#60): multi-source intake + archive.
			inboxStore := meta.NewInboxStore(db)
			mux.HandleFunc("/api/inbox", meta.InboxHandler(inboxStore))      // GET (?archived=1), POST capture
			mux.HandleFunc("/api/inbox/", meta.InboxItemHandler(inboxStore)) // POST /{id}/archive|read|unread
		}

		tasksStore, tsErr := agent.NewTasksStore()
		if tsErr != nil {
			log.Printf("[server] tasks store init failed: %v", tsErr)
		} else {
			// Share this store instance with the cc-connect IM notifier (#129)
			// so its approve/reject write-backs serialize with the scheduler's
			// per-workspace Mutate lock.
			ccconnect.SetTasksStore(tasksStore)

			acpxPort := acpxBridgePort()
			acpxClient := agent.NewAcpxClient(acpxPort)

			scheduler := agent.NewScheduler(tasksStore, func() ([]agent.WorkspaceRef, error) {
				wsHandler := workspace.NewHandler()
				wsCfg, err := wsHandler.LoadWorkspacesConfig()
				if err != nil {
					return nil, err
				}
				refs := make([]agent.WorkspaceRef, len(wsCfg.Workspaces))
				for i, ws := range wsCfg.Workspaces {
					refs[i] = agent.WorkspaceRef{ID: ws.ID, Name: ws.Name, Path: ws.Path}
				}
				return refs, nil
			})
			// Loopback base for the PM/verifier task-tool MCP subprocess to call
			// back into this daemon's HTTP API (always http on 127.0.0.1; the
			// internal-token bypass in authMiddleware accepts it). The verifier
			// runner posts verdicts here via submit_review, so the runner needs
			// it too — built before the runner is wired.
			_, selfPort, _ := net.SplitHostPort(cfg.ListenAddr)
			selfBaseURL := "http://127.0.0.1:" + selfPort

			// Headless executor/verifier: scheduler-triggered tasks run through
			// the 1acp bridge with no frontend involved (automation-first).
			scheduler.SetRunner(agent.NewTaskRunner(acpxPort, selfBaseURL, tasksStore, agentStore, scheduler))

			// ── North Task API + executor=function dispatch (Epic #317 内核).
			// tasksStore is *meta.TaskStore (agent.TasksStore alias) and
			// agent.Task == meta.Task, so the bridge is direct. RunInits runs any
			// installable app's startup hook against the live API — no apps are
			// compiled in yet, so it is a no-op until one is registered.
			taskAPI := taskapi.New(tasksStore)
			scheduler.SetFunctionRunner(func(task agent.Task, wsPath string) {
				taskapi.RunFunction(task, wsPath, tasksStore, taskAPI)
			})
			appkit.RunInits(taskAPI)

			// 口播剪辑 (speech_clip) app: project-scoped pipeline whose heavy steps
			// (transcribe/highlight) dispatch through the task kernel above. Importing
			// the package fires its init() (manifest + template + RegisterFunction);
			// here we wire its HTTP surface over the live task API.
			speechClipHandler := speechclip.NewHandler(taskAPI)
			mux.HandleFunc("/api/speech_clip/assets", speechClipHandler.HandleAssets)             // POST  import asset (server path)
			mux.HandleFunc("/api/speech_clip/assets/upload", speechClipHandler.HandleUpload)      // POST  upload recorded blob
			mux.HandleFunc("/api/speech_clip/transcribe", speechClipHandler.HandleTranscribe) // POST  dispatch transcribe task
			mux.HandleFunc("/api/speech_clip/highlights", speechClipHandler.HandleHighlights) // POST dispatch / GET rows
			mux.HandleFunc("/api/speech_clip/pick", speechClipHandler.HandlePick)             // POST toggle 金句
			mux.HandleFunc("/api/speech_clip/project", speechClipHandler.HandleProject)       // GET   project.json + status
			mux.HandleFunc("/api/speech_clip/transcript", speechClipHandler.HandleTranscript) // GET   sentence rows

			scheduler.Start(context.Background())

			// Probe installed agent CLIs once at startup; cached behind an
			// RWMutex and re-probable via /api/agent/catalog?refresh=1. Shared
			// process-wide with the cc-connect runner (which curates the
			// management API's creatable-agent list from the same probe).
			catalogStore := agent.DefaultCatalog()

			agentHandler := agent.NewHandler(agentStore, tasksStore, acpxClient, scheduler, catalogStore, selfBaseURL)
			mux.HandleFunc("/api/agent/agent-types", agentHandler.HandleAgentTypes)      // GET
			mux.HandleFunc("/api/agent/catalog", agentHandler.HandleAgentCatalog)        // GET (?refresh=1)
			mux.HandleFunc("/api/agent/sessions", agentHandler.HandleSessionsRoot)       // GET, POST
			mux.HandleFunc("/api/agent/sessions/", agentHandler.HandleSessionsItem)      // GET, DELETE /{id}
			mux.HandleFunc("/api/agent/project-items", agentHandler.HandleTasksRoot)           // GET, POST
			mux.HandleFunc("/api/agent/project-items/resolve", agentHandler.HandleTaskResolve) // GET ?project=&number= (more specific than the subtree below)
			mux.HandleFunc("/api/agent/project-items/", agentHandler.HandleTasksItem)          // DELETE /{id}
			mux.HandleFunc("/api/agent/agenda", agentHandler.HandleAgendaRoot)           // GET (cross-workspace agenda, #192)
			mux.HandleFunc("/api/agent/milestones", agentHandler.HandleMilestonesRoot)   // GET, POST
			mux.HandleFunc("/api/agent/milestones/", agentHandler.HandleMilestonesItem)  // PATCH, DELETE /{id}, POST /reorder
			mux.HandleFunc("/api/agent/discussions", agentHandler.HandleDiscussionsRoot) // POST — create a discussion thread (#189)
			mux.HandleFunc("/api/agent/discussions/", agentHandler.HandleDiscussionItem) // POST /{id}/cards, /{id}/conclude (#189)
			mux.HandleFunc("/api/agent/chat/ws", agentHandler.HandleChatWs)              // WebSocket upgrade & bridge
			mux.HandleFunc("/api/agent/dashboard", agentHandler.HandleDashboard)         // GET — cross-project cockpit aggregate (read-only)

			// Chat-digest: Feishu message sync (sync.db) + value-extraction
			// templates (meta.db v15) + single-batch analysis tasks (run by the
			// scheduler above). Self-wires its own stores; seeds presets and
			// starts a periodic re-sync of every known chat.
			var digestHandler *digest.Handler
			if dh, dErr := digest.NewHandlerDefault(); dErr != nil {
				log.Printf("[server] digest init failed: %v", dErr)
			} else {
				digestHandler = dh
				if err := digestHandler.Seed(); err != nil {
					log.Printf("[server] digest seed: %v", err)
				}
				// NOTE: the standalone 15-min ticker (StartPeriodicSync) is retired.
				// Feishu message sync now runs through the work-order scheduler via the
				// ingest feishu_message task (wired below) — a single scheduler, per
				// the "走工单系统的循环机制" directive.
				mux.HandleFunc("/api/digest/templates", digestHandler.HandleTemplates)     // GET, POST
				mux.HandleFunc("/api/digest/templates/", digestHandler.HandleTemplateItem) // PATCH, DELETE /{id}
				mux.HandleFunc("/api/digest/bindings", digestHandler.HandleBindings)       // GET ?session=, POST, DELETE
				mux.HandleFunc("/api/digest/sync", digestHandler.HandleSync)               // POST {chatId}
				mux.HandleFunc("/api/digest/analyze", digestHandler.HandleAnalyze)         // POST {chatId, workspace}
				mux.HandleFunc("/api/digest/messages", digestHandler.HandleMessages)       // GET ?session=
				// 飞书渠道配置 (Phase 2): browse groups, track/untrack, manual + auto
				// sync (reuses SyncChat watermark + message_id dedup).
				mux.HandleFunc("/api/digest/chats/available", digestHandler.HandleAvailableChats) // GET
				mux.HandleFunc("/api/digest/chats/tracked", digestHandler.HandleTrackedChats)     // GET, POST
				mux.HandleFunc("/api/digest/chats/tracked/", digestHandler.HandleTrackedChatItem) // DELETE, PATCH /{chatId}
				mux.HandleFunc("/api/digest/sync/all", digestHandler.HandleSyncAll)               // POST
				mux.HandleFunc("/api/digest/sync/config", digestHandler.HandleSyncConfig)         // GET, PUT
				mux.HandleFunc("/api/digest/status", digestHandler.HandleStatus)                  // GET
			}

			// 联系人聚合: a user-curated address book (meta.db v16) over channel
			// identities auto-discovered from synced Feishu messages (sync.db).
			// Self-wires its own stores from the default databases.
			if contactsHandler, cErr := contacts.NewHandlerDefault(); cErr != nil {
				log.Printf("[server] contacts init failed: %v", cErr)
			} else {
				mux.HandleFunc("/api/contacts", contactsHandler.HandleContacts)                // GET, POST
				mux.HandleFunc("/api/contacts/channels", contactsHandler.HandleChannels)       // GET ?contactId=&unlinked=1
				mux.HandleFunc("/api/contacts/channels/", contactsHandler.HandleChannelAction) // POST /{id}/link|unlink
				mux.HandleFunc("/api/contacts/discover", contactsHandler.HandleDiscover)       // POST
				mux.HandleFunc("/api/contacts/messages", contactsHandler.HandleMessages)       // GET ?contactId=|sessionId=
				mux.HandleFunc("/api/contacts/sessions", contactsHandler.HandleSessions)       // GET
				mux.HandleFunc("/api/contacts/companies", contactsHandler.HandleCompanies)     // GET tenant→company map
				mux.HandleFunc("/api/contacts/groups/", contactsHandler.HandleGroupMembers)    // GET /{sessionId}/members
				// Local macOS / iCloud channels (sibling syncers of Feishu): iCloud
				// contacts via CardDAV (user's Apple ID + app-specific password, stored
				// locally); iMessage via the local chat.db (needs Full Disk Access).
				// Exact paths outrank the /api/contacts/ prefix below.
				mux.HandleFunc("/api/contacts/icloud/credentials", contactsHandler.HandleICloudCredentials) // GET, POST, DELETE
				mux.HandleFunc("/api/contacts/icloud/sync", contactsHandler.HandleICloudSync)               // POST
				mux.HandleFunc("/api/contacts/imessage/sync", contactsHandler.HandleIMessageSync)           // POST
				// 渠道隐私/同意 + 爬取规则 (per-sub-module consent gate + rules).
				mux.HandleFunc("/api/channels/modules", contactsHandler.HandleChannelModules)     // GET
				mux.HandleFunc("/api/channels/modules/", contactsHandler.HandleChannelModuleItem) // POST/DELETE /{id}/consent, PUT /{id}/rules
				mux.HandleFunc("/api/contacts/", contactsHandler.HandleContactItem)               // PATCH, DELETE /{id}
			}

			// 数据源管理 (data-source management): read-only bronze layer
			// (source_records) — per-(source,kind) rollup for the overview cards and
			// a tabular record list for the 多维表格 detail view.
			if sourcesHandler, sErr := sources.NewHandlerDefault(); sErr != nil {
				log.Printf("[server] sources init failed: %v", sErr)
			} else {
				mux.HandleFunc("/api/sources/summary", sourcesHandler.HandleSummary) // GET
				mux.HandleFunc("/api/sources/records", sourcesHandler.HandleRecords) // GET ?source=&kind=&limit=
			}

			// 数据归一 (data normalization): read-only silver layer (data.db) — the
			// conformed, domain-oriented (联系人/消息/日历/待办) view that cuts across
			// sources. The bronze→silver transform runs on the ingest path; this is
			// the viewer. (POST /api/data/silver/run is registered with ingest below.)
			if dataHandler, dErr := data.NewHandlerDefault(); dErr != nil {
				log.Printf("[server] data (silver) init failed: %v", dErr)
			} else {
				mux.HandleFunc("/api/data/summary", dataHandler.HandleSummary) // GET
				mux.HandleFunc("/api/data/records", dataHandler.HandleRecords) // GET ?domain=&source=&limit=
				// 数据融合 (gold) 只读视图: 跨源归并后的联系人/消息/日历.
				mux.HandleFunc("/api/data/gold/summary", dataHandler.HandleGoldSummary) // GET
				mux.HandleFunc("/api/data/gold/records", dataHandler.HandleGoldRecords) // GET ?domain=&limit=
				// 提拔: 把一条融合待办转成任务 (agent 或 human), 回写 linked_task_id.
				dataHandler.SetSelfBaseURL(selfBaseURL)
				mux.HandleFunc("/api/data/gold/todos/promote", dataHandler.HandlePromoteTodo) // POST {id, workspaceId, assignee}
			}

			mux.HandleFunc("/api/project/local-config", handleProjectLocalConfig) // GET/PUT project-local config json
			mux.HandleFunc("/api/studio/save-assets", handleStudioSaveAssets)     // POST
			mux.HandleFunc("/api/studio/transcribe", handleStudioTranscribe)  // POST

			// 数据源摄取编排 (ingestion orchestration): CLI 生命周期探针 + 每表爬取
			// 配置 + 工单驱动的立刻/定时同步. Every pull runs as a work-order
			// function task through the scheduler above — this package never grows
			// its own ticker (user directive: 走工单系统的循环机制).
			var ingestHandler *ingest.Handler
			if ih, iErr := ingest.NewHandlerDefault(); iErr != nil {
				log.Printf("[server] ingest init failed: %v", iErr)
			} else {
				ingestHandler = ih
				// Declarative REST connectors: load ~/.1agents/connectors/*.yaml and
				// register each vendor + its REST descriptors BEFORE RegisterFunctions,
				// so the generic sync handler is wired per manifest vendor.
				manifests, mfErr := sources.LoadManifests()
				if mfErr != nil {
					log.Printf("[server] load connector manifests: %v", mfErr)
				}
				for _, m := range manifests {
					sources.RegisterManifest(m)
				}
				ingestHandler.RegisterFunctions()
				if wsPath, pErr := ingestHandler.ProvisionSystemWorkspace(); pErr != nil {
					log.Printf("[server] ingest system workspace: %v", pErr)
				} else {
					ingestHandler.SetDispatcher(ingest.NewDispatcher(taskAPI, tasksStore, wsPath))
				}
				if aErr := ingestHandler.SeedManifestAccounts(manifests); aErr != nil {
					log.Printf("[server] ingest seed manifest accounts: %v", aErr)
				}
				if sErr := ingestHandler.SeedManifestConfigs(manifests); sErr != nil {
					log.Printf("[server] ingest seed manifest configs: %v", sErr)
				}
				ingestHandler.RegisterManifestGovernance(manifests) // generic bronze→silver + viewer tables
				// Standalone governance DAGs (集成/治理解耦): cross-source entity steps.
				if gms, gErr := sources.LoadGovernanceManifests(); gErr != nil {
					log.Printf("[server] load governance manifests: %v", gErr)
				} else {
					ingestHandler.RegisterGovernanceManifests(gms)
				}
				mux.HandleFunc("/api/data/silver/run", ingestHandler.HandleRunSilver) // POST — 手动重新清洗 bronze→silver
				// 数据治理 DAG: 依赖关系 + 执行日志 + 按需重跑。
				mux.HandleFunc("/api/data/governance", ingestHandler.HandleGovernanceDAG)                 // GET — steps + nodes + edges
				mux.HandleFunc("/api/data/governance/runs", ingestHandler.HandleGovernanceRuns)           // GET ?step=&limit=
				mux.HandleFunc("/api/data/governance/run", ingestHandler.HandleGovernanceRun)             // POST ?step=&rebuild= — 重跑整个 DAG 或单步
				mux.HandleFunc("/api/data/governance/table", ingestHandler.HandleGovernanceTable)         // GET ?name=&limit= — 治理输出表下钻
				mux.HandleFunc("/api/data/governance/manifests", ingestHandler.HandleGovernanceManifests) // GET list, POST 热加治理规则/脚本
				mux.HandleFunc("/api/sources/cli/", ingestHandler.CLIHandler().HandleCLI)                 // GET /{tool}/status, POST /{tool}/recheck
				// 账号注册表 (源为中心): 厂家能力 + 每账号 CRUD.
				mux.HandleFunc("/api/sources/vendors", ingestHandler.HandleVendors)       // GET — vendor capability table
				mux.HandleFunc("/api/sources/accounts", ingestHandler.HandleAccounts)     // GET list, POST create
				mux.HandleFunc("/api/sources/accounts/", ingestHandler.HandleAccountItem) // DELETE /{id}
				// Per-source collection config / sync / history — the handlers parse the
				// {source} from the path (feishu / microsoft / google).
				mux.HandleFunc("/api/sources/feishu/collections", ingestHandler.HandleCollections)       // GET, PUT
				mux.HandleFunc("/api/sources/feishu/sync", ingestHandler.HandleSync)                     // POST {kind}
				mux.HandleFunc("/api/sources/feishu/history", ingestHandler.HandleHistory)               // GET
				mux.HandleFunc("/api/sources/feishu/schedules", ingestHandler.HandleSchedules)           // GET — 定时任务触发状态
				mux.HandleFunc("/api/sources/feishu/chats", ingestHandler.HandleChats)                   // GET — cached 群列表 (bronze) + tracked join
				mux.HandleFunc("/api/sources/feishu/chats/members", ingestHandler.HandleChatMembersSync) // POST {chatId} — manual 群成员 roster refresh
				mux.HandleFunc("/api/sources/microsoft/collections", ingestHandler.HandleCollections)
				mux.HandleFunc("/api/sources/microsoft/sync", ingestHandler.HandleSync)
				mux.HandleFunc("/api/sources/microsoft/history", ingestHandler.HandleHistory)
				mux.HandleFunc("/api/sources/microsoft/schedules", ingestHandler.HandleSchedules)
				// Microsoft Graph OAuth (PKCE) connect flow — region-aware (大陆/21Vianet).
				mux.HandleFunc("/api/sources/oauth/microsoft/config", ingestHandler.HandleMSOAuthConfig)         // GET ?region= / POST {region,clientId,tenant} — in-UI app registration
				mux.HandleFunc("/api/sources/oauth/microsoft/start", ingestHandler.HandleMSOAuthStart)           // POST {accountId} → {authUrl}
				mux.HandleFunc("/api/sources/oauth/microsoft/callback", ingestHandler.HandleMSOAuthCallback)     // GET  ?code&state (Azure redirect target)
				mux.HandleFunc("/api/sources/oauth/microsoft/status", ingestHandler.HandleMSOAuthStatus)         // GET  ?accountId
				mux.HandleFunc("/api/sources/oauth/microsoft/disconnect", ingestHandler.HandleMSOAuthDisconnect) // POST {accountId}
				mux.HandleFunc("/api/sources/google/collections", ingestHandler.HandleCollections)
				mux.HandleFunc("/api/sources/google/sync", ingestHandler.HandleSync)
				mux.HandleFunc("/api/sources/google/history", ingestHandler.HandleHistory)
				mux.HandleFunc("/api/sources/google/schedules", ingestHandler.HandleSchedules)
				mux.HandleFunc("/api/sources/agentmail/collections", ingestHandler.HandleCollections)
				mux.HandleFunc("/api/sources/agentmail/sync", ingestHandler.HandleSync)
				mux.HandleFunc("/api/sources/agentmail/history", ingestHandler.HandleHistory)
				mux.HandleFunc("/api/sources/agentmail/schedules", ingestHandler.HandleSchedules)
				// 自定义连接器: add/list manifests from the UI (hot-registered, no restart).
				mux.HandleFunc("/api/sources/connectors", ingestHandler.HandleConnectors) // GET list, POST add
				mux.HandleFunc("/api/sources/templates", ingestHandler.HandleTemplates)   // GET embedded templates, POST install
				// Manifest REST sources are served by ONE source-agnostic catch-all (built-in
				// vendors keep their explicit routes above, which win by longest-prefix match).
				// A hot-added vendor needs no new route: /api/sources/{vendor}/{action}.
				mux.HandleFunc("/api/sources/", ingestHandler.HandleManifestRoute)
				if err := ingestHandler.SeedLegacyAccounts(); err != nil {
					log.Printf("[server] ingest seed legacy accounts: %v", err)
				}
			}

			// Cross-wire ingest ⇄ digest: the feishu_message work-order task drives
			// the proven message → unified_messages sync (+ 二度联系人) so the existing
			// message/digest UI stays fresh while bronze holds the raw archive. Then
			// carry the legacy digest auto-sync setting into the new config and arm
			// the recurring tasks — done here (after the callback is set) so the first
			// message-sync run isn't a bronze-only no-op.
			if ingestHandler != nil && digestHandler != nil {
				ingestHandler.SetMessageSync(func(ctx context.Context) error {
					_, _, e := digestHandler.SyncTracked(ctx)
					return e
				})
				if mErr := ingestHandler.MigrateLegacyMessageSync(); mErr != nil {
					log.Printf("[server] ingest migrate legacy message sync: %v", mErr)
				}
				if rErr := ingestHandler.EnsureRecurringForEnabled(); rErr != nil {
					log.Printf("[server] ingest re-arm recurring: %v", rErr)
				}
			}

			// PMO 跨项目对话式需求分发层 (#61): dispatch a clarified requirement into
			// a target project's pool (and close the originating inbox item). Shares
			// the task store (#N numbering / write lock) and the inbox store (over
			// the same cached DB handle) so dispatching marks the source item read.
			if db, dbErr := meta.OpenDefault(); dbErr == nil {
				pmoStore := meta.NewPMOStore(tasksStore, meta.NewInboxStore(db))
				mux.HandleFunc("/api/pmo/dispatch", meta.PMODispatchHandler(pmoStore)) // GET targets, POST dispatch
			}
		}
	}

	mux.HandleFunc("/api/cc-connect/url", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var body struct {
			Workspace string `json:"workspace"`
			Theme     string `json:"theme"`
			Lang      string `json:"lang"`
			Path      string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		wsConfig, err := wsHandler.LoadWorkspacesConfig()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var foundWS *workspace.Workspace
		for i := range wsConfig.Workspaces {
			if wsConfig.Workspaces[i].ID == body.Workspace {
				foundWS = &wsConfig.Workspaces[i]
				break
			}
		}

		if foundWS == nil {
			http.Error(w, "workspace not found", http.StatusNotFound)
			return
		}

		redirectPath := ""
		if body.Path != "" {
			redirectPath = body.Path
		} else if foundWS.ChatChannel != "" {
			redirectPath = "/chat/" + foundWS.ChatChannel
		} else {
			// #277: project name = workspace name (no __<agent> suffix); the
			// agent type is carried per-channel, not in the project name.
			// Use the shared slug+id-fallback so a non-ASCII workspace name (the
			// default "对话" → "ws" → id "default") targets the SAME project
			// name reconcile created — otherwise the panel 404s on /projects/ws.
			projName := ccconnect.CCProjectName(foundWS.Name, foundWS.ID)
			redirectPath = "/projects/" + projName
		}

		// Normalize language codes from BCP-47 to CC-Connect codes
		normalLang := "zh"
		langLower := strings.ToLower(body.Lang)
		if strings.HasPrefix(langLower, "en") {
			normalLang = "en"
		} else if strings.HasPrefix(langLower, "zh-tw") || strings.HasPrefix(langLower, "zh-hk") {
			normalLang = "zh-TW"
		} else if strings.HasPrefix(langLower, "ja") {
			normalLang = "ja"
		} else if strings.HasPrefix(langLower, "es") {
			normalLang = "es"
		}

		url := fmt.Sprintf("/cc-connect/login?token=%s&redirect=%s&theme=%s&lang=%s",
			ccconnect.ManagementToken,
			url.QueryEscape(redirectPath),
			body.Theme,
			normalLang,
		)

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]string{"url": url})
	})

	// ── CC-Connect channel↔agent binding API (#277 Phase 2) ──────────────────
	// GET  ?project=<name>            → list channels + each channel's agent
	// POST {project,index,agent}      → bind/clear a channel's agent (hot reload)
	mux.HandleFunc("/api/cc-connect/channels", ccconnect.ChannelsHandler)
	// Incremental import from an external cc-connect config (default: the shared
	// ~/.cc-connect/config.toml) into 1agents, matched by work_dir path.
	mux.HandleFunc("/api/cc-connect/import", ccconnect.ImportHandler)

	// ── Git API ───────────────────────────────────────────────────────────────
	gitHandler := git.NewHandler(cfg.WorkDir)
	mux.HandleFunc("/api/git/status", gitHandler.Status)                  // GET
	mux.HandleFunc("/api/git/diff", gitHandler.Diff)                      // GET  ?file=<path>&staged=<bool>
	mux.HandleFunc("/api/git/stage", gitHandler.Stage)                    // POST ?file=<path> or ?all=true
	mux.HandleFunc("/api/git/unstage", gitHandler.Unstage)                // POST ?file=<path> or ?all=true
	mux.HandleFunc("/api/git/discard", gitHandler.Discard)                // POST ?file=<path>
	mux.HandleFunc("/api/git/ai-commit", gitHandler.AICommit)             // POST
	mux.HandleFunc("/api/git/commit", gitHandler.Commit)                  // POST {message:"…"}
	mux.HandleFunc("/api/git/log", gitHandler.Log)                        // GET  ?limit=20
	mux.HandleFunc("/api/git/branches", gitHandler.Branches)              // GET
	mux.HandleFunc("/api/git/checkout", gitHandler.Checkout)              // POST {branch:"…",create:bool}
	mux.HandleFunc("/api/git/push", gitHandler.Push)                      // POST
	mux.HandleFunc("/api/git/pull", gitHandler.Pull)                      // POST
	mux.HandleFunc("/api/git/fetch", gitHandler.Fetch)                    // POST
	mux.HandleFunc("/api/git/worktrees", gitHandler.Worktrees)            // GET
	mux.HandleFunc("/api/git/graph", gitHandler.Graph)                    // GET ?limit=100
	mux.HandleFunc("/api/git/commit-files", gitHandler.CommitFiles)       // GET ?hash=<hash>
	mux.HandleFunc("/api/git/commit-diff", gitHandler.CommitDiff)         // GET ?hash=<hash>&file=<path>
	mux.HandleFunc("/api/git/worktree-status", gitHandler.WorktreeStatus) // GET ?path=<path>
	mux.HandleFunc("/api/git/worktree-diff", gitHandler.WorktreeDiff)     // GET ?path=<path>&file=<file>

	// ── Workspace context API (switches fs + git roots at runtime) ─────────
	ctxHandler := ctxt.NewHandler(fsHandler, gitHandler)
	mux.HandleFunc("/api/context/set", ctxHandler.Set) // POST {"path":"..."}
	mux.HandleFunc("/api/context/get", ctxHandler.Get) // GET
	// ── Terminal API (tmux session management) ────────────────────────────────
	termHandler := terminal.NewHandler(cfg)
	// Create the hidden anchor window (tmux index 0, root shell) at boot so it
	// always exists before any user action and survives a backend restart.
	termHandler.EnsureStartupSession()
	mux.HandleFunc("/api/terminal/create", termHandler.Create) // POST {workspaceId, cwd}
	mux.HandleFunc("/api/terminal/list", termHandler.List)     // GET
	mux.HandleFunc("/api/terminal/kill", termHandler.Kill)     // POST {windowIndex}
	mux.HandleFunc("/api/terminal/switch", termHandler.Switch) // POST {windowIndex}
	mux.HandleFunc("/api/terminal/rename", termHandler.Rename) // POST {windowName, name}
	mux.HandleFunc("/api/terminal/mouse", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			termHandler.GetMouse(w, r)
		} else if r.Method == http.MethodPost {
			termHandler.SetMouse(w, r)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// ── ttyd reverse proxy ───────────────────────────────────────────────────
	// All WebSocket and HTTP traffic destined for ttyd is forwarded here.
	// The frontend should connect to ws://<host>/ws (not directly to ttyd).
	ttydProxy := gateway.NewTtydProxy(cfg.TtydAddr)
	mux.Handle("/ws", ttydProxy)    // terminal WebSocket stream
	mux.Handle("/token", ttydProxy) // ttyd auth token endpoint

	// ── CC-Connect reverse proxy ─────────────────────────────────────────────
	// Transparently reverse-proxies requests to the local CC-Connect management server
	// under the main HTTPS gateway, resolving LAN protocol security and Mixed Content.
	mux.Handle("/cc-connect/", gateway.NewCCConnectProxy(ccconnect.ManagementPort))
	mux.Handle("/assets/", gateway.NewCCConnectProxy(ccconnect.ManagementPort))
	mux.Handle("/api/v1/", gateway.NewCCConnectProxy(ccconnect.ManagementPort))

	// ── Bridge WebSocket proxy ──────────────────────────────────────────────
	// Proxies /bridge/ws to the CC-Connect bridge server (dynamic port).
	mux.Handle("/bridge/", gateway.NewBridgeProxy(ccconnect.BridgePort, ccconnect.BridgeToken))

	// ── 1skills reverse proxy ────────────────────────────────────────────────
	var skillsPort int
	if _, portStr, err := net.SplitHostPort(cfg.SkillsAddr); err == nil {
		fmt.Sscanf(portStr, "%d", &skillsPort)
	} else {
		skillsPort = 38085
	}
	mux.Handle("/1skills/", gateway.NewSkillsProxy(skillsPort))

	// ── Module embed scripts (custom elements) ──────────────────────────────
	// Self-contained ESM bundles produced by the submodule embed pipelines
	// (1skills: `yarn build:embed`, cc-connect: `npm run build:embed`).
	// The 1agents frontend loads them as ESM modules to register
	// <skills-panel> and <cc-connect-panel> custom elements, replacing
	// the iframe approach for non-terminal panels.
	//
	// The path inside dist-embed is fixed by the submodule's vite
	// library-mode config. We resolve the file at startup and 404 if the
	// submodule has not been built yet — a friendlier failure mode than
	// the route silently shadowing the static catch-all.
	mux.HandleFunc("/api/embed/skills-embed.js", serveEmbedScript([]string{
		"modules/1skills/dist-embed/skills-embed.js",
		"../modules/1skills/dist-embed/skills-embed.js",
	}))
	mux.HandleFunc("/api/embed/cc-connect-embed.js", serveEmbedScript([]string{
		"modules/cc-connect/web/dist-embed/cc-connect-embed.js",
		"../modules/cc-connect/web/dist-embed/cc-connect-embed.js",
	}))

	// ── 1skills API pass-through routes ──────────────────────────────────────
	// The 1skills frontend is built with VITE_API_BASE=/api, so its JS makes
	// requests to /api/skills, /api/mcp, /api/slash-commands, etc. directly on
	// the gateway host. These routes forward those calls to the Python backend
	// without stripping any prefix (the Python FastAPI handles /api/* natively).
	skillsAPIProxy := gateway.NewSkillsProxy(skillsPort)
	mux.Handle("/api/skills", skillsAPIProxy)
	mux.Handle("/api/skills/", skillsAPIProxy)
	mux.Handle("/api/agents", skillsAPIProxy)
	mux.Handle("/api/agents/", skillsAPIProxy)
	mux.Handle("/api/import/", skillsAPIProxy)
	mux.Handle("/api/mcp/", skillsAPIProxy)
	mux.Handle("/api/slash-commands", skillsAPIProxy)
	mux.Handle("/api/slash-commands/", skillsAPIProxy)
	mux.Handle("/api/marketplace/", skillsAPIProxy)
	mux.Handle("/api/scan/", skillsAPIProxy)
	mux.Handle("/api/settings", skillsAPIProxy)
	mux.Handle("/api/settings/", skillsAPIProxy)
	mux.Handle("/api/health", skillsAPIProxy)

	// ── Tunnel API (on-demand multi-port tunnel control) ─────────────────────
	tunnelAuth := func(r *http.Request) bool {
		authHeader := r.Header.Get("Authorization")
		expectedAuth := "Bearer " + ccconnect.ManagementToken
		return authHeader == expectedAuth || r.URL.Query().Get("token") == ccconnect.ManagementToken
	}

	resolvePort := func(r *http.Request) string {
		if p := r.URL.Query().Get("port"); p != "" {
			return p
		}
		return tunnel.PortFrom(cfg.ListenAddr)
	}

	resolveTimeout := func(r *http.Request) int {
		t := r.URL.Query().Get("timeout")
		if t == "" {
			return 0
		}
		var mins int
		fmt.Sscanf(t, "%d", &mins)
		return mins
	}

	mux.HandleFunc("/api/tunnel/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !tunnelAuth(r) {
			http.Error(w, "unauthorized control command", http.StatusUnauthorized)
			return
		}

		port := resolvePort(r)
		timeout := resolveTimeout(r)
		publicURL, token, err := tunnel.DefaultSupervisor.Start(port, timeout)
		if err != nil {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"port":  port,
			"url":   publicURL,
			"token": token,
			"link":  fmt.Sprintf("%s/?token=%s", publicURL, token),
		})
	})

	mux.HandleFunc("/api/tunnel/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !tunnelAuth(r) {
			http.Error(w, "unauthorized control command", http.StatusUnauthorized)
			return
		}

		port := r.URL.Query().Get("port")
		if port == "" {
			http.Error(w, "port parameter is required to stop a specific tunnel", http.StatusBadRequest)
			return
		}

		if err := tunnel.DefaultSupervisor.Stop(port); err != nil {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "stopped", "port": port})
	})

	mux.HandleFunc("/api/tunnel/stop-all", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !tunnelAuth(r) {
			http.Error(w, "unauthorized control command", http.StatusUnauthorized)
			return
		}

		stopped := tunnel.DefaultSupervisor.StopAll()

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":        "all_stopped",
			"stopped_ports": stopped,
		})
	})

	mux.HandleFunc("/api/tunnel/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		tunnels := tunnel.DefaultSupervisor.ListAll()

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"active":  len(tunnels) > 0,
			"tunnels": tunnels,
		})
	})

	// ── System management API (version check + OTA update) ──────────────────
	sysHandler := system.NewHandler()
	mux.HandleFunc("/api/system/version", sysHandler.Version)                     // GET  — current & latest version, has_update flag
	mux.HandleFunc("/api/system/update", sysHandler.Update)                       // POST — trigger OTA update (non-blocking, returns 202)
	mux.HandleFunc("/api/system/update/status", sysHandler.UpdateStatus)          // GET  — real-time update progress log
	mux.HandleFunc(system.ManifestPath, sysHandler.Manifest)                      // GET  — frontend OTA manifest (proxied from GitHub Releases)
	mux.HandleFunc("/api/system/happy/status", sysHandler.HappyStatus)            // GET  — happy daemon status + machine credentials
	mux.HandleFunc("/api/system/happy/daemon/start", sysHandler.HappyDaemonStart) // POST — start happy daemon
	mux.HandleFunc("/api/system/happy/daemon/stop", sysHandler.HappyDaemonStop)   // POST — stop happy daemon
	mux.HandleFunc("/api/system/happy/pair/start", sysHandler.HappyPairStart)     // POST — begin account-level pairing, returns pairing code
	mux.HandleFunc("/api/system/happy/pair/status", sysHandler.HappyPairStatus)   // GET  — pairing progress (pending/authorized/error)
	// 重置本地数据: wipe App data (meta.db/sync.db tables + knowledge/scratch files +
	// workspace-backed cc-connect projects), keep relay pairing identity (~/.happy +
	// relay-creds.json) and provider/model config, re-seed default workspace.
	mux.HandleFunc("/api/system/reset", sysHandler.ResetHandler(wsHandler.EnsureDefaultWorkspace, ccconnect.PurgeWorkspaceProjects)) // POST — reset local data

	// ── Relay client credentials (issue #109) ───────────────────────────────
	mux.HandleFunc("/api/relay/credentials", sysHandler.RelayCredentialsHandler) // GET/POST/DELETE — persist relay account master key

	// ── Device registry + heartbeat + discovery (issue #110) ─────────────────
	mux.HandleFunc("/api/devices", sysHandler.DevicesHandler)            // GET/POST/DELETE — list/register/remove devices
	mux.HandleFunc("/api/devices/heartbeat", sysHandler.DeviceHeartbeat) // POST — refresh a device's lastSeen
	mux.HandleFunc("/api/devices/refresh", sysHandler.DevicesRefresh)    // POST — Tailscale scan + full refresh

	// ── Device proxy routing layer (issue #111) ──────────────────────────────
	// /api/proxy/{deviceId}/... forwards HTTP + WebSocket to a target device
	// resolved via the #110 registry (direct Tailscale/LAN connection). The
	// trailing-slash subtree does not collide with the exact "/api/proxy" web
	// iframe proxy registered below.
	mux.HandleFunc("/api/proxy/", sysHandler.DeviceProxyHandler)

	// ── Access Token API ─────────────────────────────────────────────────────
	mux.HandleFunc("/api/access/status", handleAccessStatus)
	mux.HandleFunc("/api/access/generate", handleAccessGenerate)
	mux.HandleFunc("/api/access/verify", handleAccessVerify)
	mux.HandleFunc("/api/access/revoke", handleAccessRevoke)

	// ── Proxy API ────────────────────────────────────────────────────────────
	mux.HandleFunc("/api/proxy", handleProxy)

	// ── Static frontend assets + task permalink deep links ───────────────────
	// This catch-all must be registered last so it does not shadow the routes
	// above. frontend/dist must contain an index.html for SPA-style navigation.
	//
	// GitHub-style task URLs (/{project}/tasks/{number}) have no file on disk,
	// so the catch-all serves the SPA index for them and lets the frontend
	// resolve the reference (switch project + open the task). A wildcard
	// ServeMux pattern can't express this — a top-level "/{project}/tasks/..."
	// collides with every "/prefix/" subtree route (e.g. /cc-connect/) and
	// panics at registration. So the check lives here in the catch-all, which
	// only sees paths no more-specific route already claimed.
	staticFS := http.FileServer(http.Dir(cfg.StaticDir))
	taskPermalinkRe := regexp.MustCompile(`^/[^/]+/tasks/\d+/?$`)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && taskPermalinkRe.MatchString(r.URL.Path) {
			http.ServeFile(w, r, filepath.Join(cfg.StaticDir, "index.html"))
			return
		}
		// Big-screen build target (issue #120): the standalone dashboard bundle
		// is emitted as dashboard.html. Serve it at the clean /dashboard path so
		// the big screen has a stable URL distinct from the SPA root.
		if r.Method == http.MethodGet && (r.URL.Path == "/dashboard" || r.URL.Path == "/dashboard/") {
			http.ServeFile(w, r, filepath.Join(cfg.StaticDir, "dashboard.html"))
			return
		}
		staticFS.ServeHTTP(w, r)
	})

	return authMiddleware(mux, cfg)
}

// authMiddleware enforces authentication in two layers:
//
//  1. Tunnel auth — when any Cloudflare tunnel is active, the ephemeral session
//     token is required.
//  2. Access token auth — when the user has generated a persistent access token
//     file, all non-localhost requests must present it. Localhost always bypasses.
func authMiddleware(next http.Handler, cfg *config.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ── Layer 0: internal loopback bearer ───────────────────────────────
		// Loopback helper subprocesses (e.g. the `1agents project-items` MCP server
		// the AI Project Manager session spawns) present the process-scoped
		// internal token. Accept it only from localhost so the bypass can never
		// be reached over the tunnel, then skip both auth layers below.
		if isLocalhost(r) {
			if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
				if strings.TrimPrefix(authHeader, "Bearer ") == localtoken.Token {
					next.ServeHTTP(w, r)
					return
				}
			}
		}

		// ── Layer 1: Tunnel session auth ────────────────────────────────────
		if tunnel.DefaultSupervisor.HasAnyActive() {
			// Bypass tunnel auth for tunnel control APIs
			if !strings.HasPrefix(r.URL.Path, "/api/tunnel/") {
				authenticated := false
				var matchedToken string

				checkToken := func(tok string) bool {
					if tok != "" && tunnel.DefaultSupervisor.ValidateToken(tok) {
						matchedToken = tok
						return true
					}
					return false
				}

				if tokenParam := r.URL.Query().Get("token"); tokenParam != "" {
					if checkToken(tokenParam) {
						authenticated = true
						http.SetCookie(w, &http.Cookie{
							Name:     "ra_session_token",
							Value:    matchedToken,
							Path:     "/",
							HttpOnly: true,
							Secure:   true,
							SameSite: http.SameSiteLaxMode,
						})
					}
				}

				if !authenticated {
					authHeader := r.Header.Get("Authorization")
					if strings.HasPrefix(authHeader, "Bearer ") {
						if checkToken(strings.TrimPrefix(authHeader, "Bearer ")) {
							authenticated = true
						}
					}
				}

				if !authenticated {
					if cookie, err := r.Cookie("ra_session_token"); err == nil {
						if checkToken(cookie.Value) {
							authenticated = true
						}
					}
				}

				if !authenticated {
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					w.WriteHeader(http.StatusUnauthorized)
					_, _ = w.Write([]byte(`{"error": "Unauthorized: Ephemeral session token required. Please scan the authorized QR code or click the secure link."}`))
					return
				}
			}
		}

		// ── Layer 2: Access token auth ─────────────────────────────────────
		if !auth.TokenExists() {
			next.ServeHTTP(w, r)
			return
		}

		// Access token API endpoints manage their own auth
		if strings.HasPrefix(r.URL.Path, "/api/access/") {
			next.ServeHTTP(w, r)
			return
		}

		// Localhost always bypasses
		if isLocalhost(r) {
			next.ServeHTTP(w, r)
			return
		}

		storedToken, _ := auth.LoadToken()
		if storedToken == "" {
			next.ServeHTTP(w, r)
			return
		}

		accessAuthenticated := false

		// Mechanism A: ?access_token= query param
		if t := r.URL.Query().Get("access_token"); t != "" && t == storedToken {
			accessAuthenticated = true
		}

		// Mechanism B: Authorization: Bearer <token> (also checks access token)
		if !accessAuthenticated {
			authHeader := r.Header.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				if strings.TrimPrefix(authHeader, "Bearer ") == storedToken {
					accessAuthenticated = true
				}
			}
		}

		// Mechanism C: ra_access_token cookie
		if !accessAuthenticated {
			if cookie, err := r.Cookie("ra_access_token"); err == nil {
				if cookie.Value == storedToken {
					accessAuthenticated = true
				}
			}
		}

		if accessAuthenticated {
			// Refresh long-lived cookie
			http.SetCookie(w, &http.Cookie{
				Name:     "ra_access_token",
				Value:    storedToken,
				Path:     "/",
				HttpOnly: true,
				Secure:   r.TLS != nil,
				SameSite: http.SameSiteLaxMode,
				MaxAge:   365 * 24 * 3600,
			})
			next.ServeHTTP(w, r)
			return
		}

		// Not authenticated — reject API calls, let page requests through
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"access_token_required","message":"An access token is required for non-localhost access."}`))
			return
		}

		// Page request: let SPA load; it will call /api/access/status and show gate
		next.ServeHTTP(w, r)
	})
}

func isLocalhost(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return host == "127.0.0.1" || host == "::1"
}

// acpxBridgePort is the port the 1acp bridge-server listens on and the backend
// dials. Defaults to 38082, overridable via ACPX_PORT — the same env var the
// supervisor passes to the spawned bridge-server — so an isolated second
// instance can run without colliding with a primary one.
func acpxBridgePort() int {
	if v := os.Getenv("ACPX_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			return p
		}
	}
	return 38082
}

// ── Access Token Handlers ───────────────────────────────────────────────────────

func handleAccessStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	required := auth.TokenExists()
	authenticated := true

	if required && !isLocalhost(r) {
		storedToken, _ := auth.LoadToken()
		if storedToken != "" {
			authenticated = false

			if t := r.URL.Query().Get("access_token"); t != "" && t == storedToken {
				authenticated = true
			}
			if !authenticated {
				authHeader := r.Header.Get("Authorization")
				if strings.HasPrefix(authHeader, "Bearer ") {
					if strings.TrimPrefix(authHeader, "Bearer ") == storedToken {
						authenticated = true
					}
				}
			}
			if !authenticated {
				if cookie, err := r.Cookie("ra_access_token"); err == nil {
					if cookie.Value == storedToken {
						authenticated = true
					}
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]bool{
		"required":      required,
		"authenticated": authenticated,
	})
}

func handleAccessGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !isLocalhost(r) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "Token generation is only allowed from localhost."})
		return
	}

	token := tunnel.GenerateRandomToken()
	if err := auth.SaveToken(token); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]string{
		"token":   token,
		"message": "Access token generated. Save it now — it will not be shown again.",
	})
}

func handleAccessVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": "Invalid request body."})
		return
	}

	if body.Token == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": "Token is required."})
		return
	}

	storedToken, err := auth.LoadToken()
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}

	if body.Token != storedToken {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": "无效的访问令牌。"})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "ra_access_token",
		Value:    storedToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   365 * 24 * 3600,
	})

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}

func handleAccessRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Allow localhost or already-authenticated callers
	allowed := isLocalhost(r)
	if !allowed {
		storedToken, _ := auth.LoadToken()
		if storedToken != "" {
			if cookie, err := r.Cookie("ra_access_token"); err == nil && cookie.Value == storedToken {
				allowed = true
			}
			if !allowed {
				authHeader := r.Header.Get("Authorization")
				if strings.HasPrefix(authHeader, "Bearer ") && strings.TrimPrefix(authHeader, "Bearer ") == storedToken {
					allowed = true
				}
			}
		}
	}

	if !allowed {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "Token revocation requires localhost or authenticated access."})
		return
	}

	if err := auth.DeleteToken(); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "message": "Access token revoked."})
}

// handleProxy acts as a reverse proxy that fetches external websites, strips
// X-Frame-Options & Content-Security-Policy headers, and injects `<base>` and link-rewriting scripts.
func handleProxy(w http.ResponseWriter, r *http.Request) {
	targetURL := r.URL.Query().Get("url")
	if targetURL == "" {
		http.Error(w, "Missing url parameter", http.StatusBadRequest)
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), "GET", targetURL, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Forward standard headers
	req.Header.Set("User-Agent", r.Header.Get("User-Agent"))
	req.Header.Set("Accept", r.Header.Get("Accept"))
	req.Header.Set("Accept-Language", r.Header.Get("Accept-Language"))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Read response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	contentType := resp.Header.Get("Content-Type")
	// If it's HTML, inject our base href and click interceptor scripts!
	if strings.Contains(strings.ToLower(contentType), "text/html") {
		htmlStr := string(bodyBytes)

		// 1. Inject <base href="..."> right after the opening <head> tag
		headIdx := strings.Index(strings.ToLower(htmlStr), "<head>")
		if headIdx != -1 {
			insertPos := headIdx + len("<head>")

			// Inject `<base>` tag and click interceptor script
			actualURL := resp.Request.URL.String()
			baseTag := `<base href="` + actualURL + `">`
			scriptTag := `
<script>
(function() {
  function getOriginalUrl(url) {
    try {
      var urlObj = new URL(url || window.location.href);
      if (urlObj.pathname === '/api/proxy') {
        var target = urlObj.searchParams.get('url');
        if (target) return target;
      }
      return urlObj.href;
    } catch(e) {
      return url || window.location.href;
    }
  }

  function notifyParent(url) {
    try {
      var orig = getOriginalUrl(url);
      window.parent.postMessage({ type: 'iframe_navigate', url: orig }, '*');
    } catch(e) {}
  }

  // Notify parent of initial load
  notifyParent();

  // Notify parent on history popstate (e.g. back/forward)
  window.addEventListener('popstate', function() {
    notifyParent();
  });

  // Prevent links from redirecting the frame to non-proxied addresses
  document.addEventListener('click', function(e) {
    var target = e.target.closest('a');
    if (target && target.href) {
      e.preventDefault();
      // Route the absolute URL back through our proxy!
      window.location.href = window.location.origin + '/api/proxy?url=' + encodeURIComponent(target.href);
    }
  }, true);

  // Prevent form actions from escaping the proxy
  document.addEventListener('submit', function(e) {
    var target = e.target;
    if (target && target.action) {
      if (target.method.toLowerCase() === 'get') {
        e.preventDefault();
        try {
          var url = new URL(target.action);
          var formData = new FormData(target);
          for (var pair of formData.entries()) {
            url.searchParams.set(pair[0], pair[1]);
          }
          window.location.href = window.location.origin + '/api/proxy?url=' + encodeURIComponent(url.href);
        } catch(err) {
          // Fallback if URL parsing fails
        }
      }
    }
  }, true);

  // Rewrite History API state changes to same-origin to prevent SecurityError
  if (window.history) {
    var originalPushState = window.history.pushState;
    window.history.pushState = function(state, title, url) {
      try {
        if (url) {
          var resolvedUrl = new URL(url, document.baseURI).href;
          var proxiedUrl = window.location.origin + '/api/proxy?url=' + encodeURIComponent(resolvedUrl);
          originalPushState.apply(window.history, [state, title, proxiedUrl]);
          notifyParent(resolvedUrl);
        } else {
          originalPushState.apply(window.history, arguments);
          notifyParent();
        }
      } catch (e) {
        console.warn('Blocked pushState rewrite:', e);
      }
    };

    var originalReplaceState = window.history.replaceState;
    window.history.replaceState = function(state, title, url) {
      try {
        if (url) {
          var resolvedUrl = new URL(url, document.baseURI).href;
          var proxiedUrl = window.location.origin + '/api/proxy?url=' + encodeURIComponent(resolvedUrl);
          originalReplaceState.apply(window.history, [state, title, proxiedUrl]);
          notifyParent(resolvedUrl);
        } else {
          originalReplaceState.apply(window.history, arguments);
          notifyParent();
        }
      } catch (e) {
        console.warn('Blocked replaceState rewrite:', e);
      }
    };
  }

  // Intercept window.fetch to route relative/external data requests through proxy
  if (window.fetch) {
    var originalFetch = window.fetch;
    window.fetch = function(input, init) {
      try {
        var url;
        if (typeof input === 'string') {
          url = input;
        } else if (input instanceof URL) {
          url = input.href;
        } else if (input && input.url) {
          url = input.url;
        }

        if (url) {
          var resolvedUrl = new URL(url, document.baseURI).href;
          var proxyHost = window.location.host;
          var resolvedObj = new URL(resolvedUrl);
          if (resolvedObj.host !== proxyHost) {
            var proxiedUrl = window.location.origin + '/api/proxy?url=' + encodeURIComponent(resolvedUrl);
            if (typeof input === 'string') {
              input = proxiedUrl;
            } else if (input instanceof URL) {
              input = new URL(proxiedUrl);
            } else if (input instanceof Request) {
              input = new Request(proxiedUrl, input);
            } else if (input && input.url) {
              input = new Request(proxiedUrl, input);
            }
          }
        }
      } catch (e) {
        console.warn('Blocked fetch rewrite:', e);
      }
      return originalFetch.apply(this, arguments);
    };
  }

  // Intercept XMLHttpRequest to route relative/external data requests through proxy
  if (window.XMLHttpRequest) {
    var originalOpen = window.XMLHttpRequest.prototype.open;
    window.XMLHttpRequest.prototype.open = function(method, url, async, user, password) {
      try {
        if (url) {
          var resolvedUrl = new URL(url, document.baseURI).href;
          var proxyHost = window.location.host;
          var resolvedObj = new URL(resolvedUrl);
          if (resolvedObj.host !== proxyHost) {
            arguments[1] = window.location.origin + '/api/proxy?url=' + encodeURIComponent(resolvedUrl);
          }
        }
      } catch (e) {
        console.warn('Blocked XHR rewrite:', e);
      }
      return originalOpen.apply(this, arguments);
    };
  }
})();
</script>
`
			htmlStr = htmlStr[:insertPos] + baseTag + scriptTag + htmlStr[insertPos:]
			bodyBytes = []byte(htmlStr)
		}
	}

	// Copy headers, stripping security controls
	for k, v := range resp.Header {
		lowerK := strings.ToLower(k)
		if lowerK == "x-frame-options" || lowerK == "content-security-policy" || lowerK == "csp" {
			continue
		}
		for _, val := range v {
			w.Header().Add(k, val)
		}
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(bodyBytes)
}

// serveEmbedScript returns an http.HandlerFunc that serves a single
// submodule embed bundle. The handler resolves the file lazily on each
// request, so a submodule that is built *after* 1agents has started
// becomes available without a restart. The first existing path wins.
//
// If none of the candidates exist the handler returns 404 with a hint
// telling the operator how to produce the bundle. This is intentional:
// silently shadowing the static catch-all would make "iframe doesn't
// load" look like "module registration failed", which is much harder
// to diagnose.
// appregistryItemHandler routes /api/apps/{id}/enable and /api/apps/{id}/disable
// to the appropriate appregistry handler. The mux prefix route /api/apps/ catches
// both sub-paths; we dispatch by suffix.
func appregistryItemHandler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case strings.HasSuffix(path, "/enable"):
		appregistry.HandleEnable(w, r)
	case strings.HasSuffix(path, "/disable"):
		appregistry.HandleDisable(w, r)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

// projectConfigHandler routes GET /api/project/config and PUT /api/project/config.
func projectConfigHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		workspace.HandleProjectConfigGet(w, r)
	case http.MethodPut:
		workspace.HandleProjectConfigPut(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func serveEmbedScript(candidates []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Only allow GET — these are static assets; anything else is a bug.
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		for _, c := range candidates {
			abs, err := filepath.Abs(c)
			if err != nil {
				continue
			}
			if info, err := os.Stat(abs); err == nil && !info.IsDir() {
				w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
				w.Header().Set("Cache-Control", "no-cache")
				http.ServeFile(w, r, abs)
				return
			}
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w,
			"embed bundle not found; tried: %s\nbuild it with `yarn build:embed` (1skills) or `npm run build:embed` (cc-connect) inside the submodule",
			strings.Join(candidates, ", "),
		)
	}
}

// handleProjectLocalConfig reads/writes a project-local config blob at
// <workspacePath>/.1agents/project_config.json. Generic passthrough — the
// frontend owns the schema (e.g. hiddenTabs). Server-persisted and travels with
// the project directory, so it works cross-device without a DB.
func handleProjectLocalConfig(w http.ResponseWriter, r *http.Request) {
	ws := r.URL.Query().Get("workspacePath")
	if ws == "" {
		http.Error(w, "workspacePath required", http.StatusBadRequest)
		return
	}
	path := filepath.Join(ws, ".1agents", "project_config.json")

	switch r.Method {
	case http.MethodGet:
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{}"))
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var incoming map[string]any
		if err := json.Unmarshal(body, &incoming); err != nil {
			http.Error(w, "body must be a JSON object: "+err.Error(), http.StatusBadRequest)
			return
		}
		// Shallow-merge into the existing file so independent writers (e.g. tab
		// visibility vs project config) each own their own top-level keys without
		// clobbering the others.
		merged := map[string]any{}
		if existing, rerr := os.ReadFile(path); rerr == nil {
			_ = json.Unmarshal(existing, &merged)
		}
		for k, v := range incoming {
			merged[k] = v
		}
		out, err := json.MarshalIndent(merged, "", "  ")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(path, out, 0o644); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleStudioSaveAssets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID           string `json:"id"`
		WebcamBase64 string `json:"webcamBase64"`
		ScreenBase64 string `json:"screenBase64"`
		AudioBase64  string `json:"audioBase64"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	dir := filepath.Join(home, ".1agents", "studio", req.ID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	saveBase64File := func(filename, b64 string) error {
		if b64 == "" {
			return nil
		}
		data, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, filename), data, 0644)
	}

	if err := saveBase64File("webcam.webm", req.WebcamBase64); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := saveBase64File("screen.webm", req.ScreenBase64); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := saveBase64File("audio.webm", req.AudioBase64); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "path": dir})
}

func handleStudioTranscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID          string `json:"id"`
		AudioBase64 string `json:"audioBase64"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	res := map[string]any{
		"id":           req.ID,
		"createdAt":    1717171717,
		"duration":     60,
		"speakerCount": 1,
		"title":        "演示录制",
		"fullText":     "这是一次双录屏的演示录制。我们将在这里展示摄像头和网页的同步录制，并进行粗剪测试。",
		"summary":      "本次录制演示了摄像头与屏幕的双录功能。",
		"utterances": []map[string]any{
			{
				"speaker": "speaker_0",
				"start":   0.0,
				"end":     5.0,
				"text":    "这是一次双录屏的演示录制。",
			},
			{
				"speaker": "speaker_0",
				"start":   5.0,
				"end":     10.0,
				"text":    "我们将在这里展示摄像头和网页的同步录制，",
			},
			{
				"speaker": "speaker_0",
				"start":   10.0,
				"end":     15.0,
				"text":    "并进行粗剪测试。",
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}
