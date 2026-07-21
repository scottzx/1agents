package supervisor

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/scottzx/1Agents/backend/internal/config"
)

type AcpxSupervisor struct {
	cfg          *config.Config
	cmd          *exec.Cmd
	mu           sync.Mutex
	restartCount int
	done         chan struct{}
}

func NewAcpx(cfg *config.Config) *AcpxSupervisor {
	return &AcpxSupervisor{
		cfg:  cfg,
		done: make(chan struct{}),
	}
}

func (s *AcpxSupervisor) Start(ctx context.Context) {
	go s.supervisionLoop(ctx)
}

func (s *AcpxSupervisor) Done() <-chan struct{} {
	return s.done
}

func (s *AcpxSupervisor) supervisionLoop(ctx context.Context) {
	defer close(s.done)

	cwd, err := os.Getwd()
	if err != nil {
		log.Printf("[acpx-sup] Failed to determine working directory: %v", err)
		return
	}

	// Resolve bridge entry once; if missing, do not thrash chdir into non-existent paths.
	bridge, err := resolveAcpBridge(cwd)
	if err != nil {
		log.Printf("[acpx-sup] FATAL: cannot locate ACP bridge-server: %v", err)
		log.Printf("[acpx-sup] Install @1agents/acp-bridge (dependency of @1agents/cli), or run from a monorepo checkout with modules/1acp.")
		return
	}
	log.Printf("[acpx-sup] Using ACP bridge: %s (dir=%s, via=%s)", bridge.script, bridge.workDir, bridge.via)

	for {
		select {
		case <-ctx.Done():
			log.Println("[acpx-sup] Shutdown requested, stopping acpx-server.")
			s.stopProcess()
			return
		default:
		}

		if s.restartCount >= s.cfg.MaxRestarts {
			log.Printf("[acpx-sup] FATAL: acpx-server has restarted %d times consecutively. Giving up.", s.restartCount)
			return
		}

		log.Printf("[acpx-sup] Starting acpx-server microservice (attempt %d)...", s.restartCount+1)
		if err := s.startProcess(ctx, bridge); err != nil {
			log.Printf("[acpx-sup] acpx-server exited with error: %v", err)
		} else {
			log.Println("[acpx-sup] acpx-server exited cleanly.")
		}

		if ctx.Err() != nil {
			log.Println("[acpx-sup] Context cancelled after process exit, stopping supervisor.")
			return
		}

		s.mu.Lock()
		s.restartCount++
		count := s.restartCount
		s.mu.Unlock()

		log.Printf("[acpx-sup] Restarting acpx-server in %v... (%d/%d)",
			s.cfg.RestartDelay, count, s.cfg.MaxRestarts)

		select {
		case <-ctx.Done():
			log.Println("[acpx-sup] Shutdown during restart wait, stopping.")
			return
		case <-time.After(s.cfg.RestartDelay):
		}
	}
}

type acpBridge struct {
	script  string // absolute path to bridge-server.mjs / bridge-server.js
	workDir string // directory for cmd.Dir (must exist)
	via     string // how it was resolved (for logs)
	useTsx  bool   // monorepo dev: run via npx tsx
}

func (s *AcpxSupervisor) startProcess(ctx context.Context, bridge acpBridge) error {
	acpxPort := "38082"
	if v := os.Getenv("ACPX_PORT"); v != "" {
		acpxPort = v
	}

	var cmd *exec.Cmd
	var label string
	if bridge.useTsx {
		cmd = exec.CommandContext(ctx, "npx", "tsx", filepath.Base(bridge.script))
		cmd.Dir = bridge.workDir
		label = fmt.Sprintf("npx tsx %s (dir=%s, %s)", filepath.Base(bridge.script), bridge.workDir, bridge.via)
	} else {
		// Production: plain node on packaged bridge-server.mjs (imports acpx/runtime).
		cmd = exec.CommandContext(ctx, "node", bridge.script)
		cmd.Dir = bridge.workDir
		label = fmt.Sprintf("node %s (dir=%s, %s)", bridge.script, bridge.workDir, bridge.via)
	}

	cmd.Env = os.Environ()
	if home, err := os.UserHomeDir(); err != nil {
		log.Printf("[acpx-sup] Unable to resolve user home for agent PATH: %v", err)
	} else {
		cmd.Env = acpxEnvironment(cmd.Env, home)
	}
	cmd.Env = append(cmd.Env, "ACPX_PORT="+acpxPort)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err == nil {
		defer stdin.Close()
	}

	s.mu.Lock()
	s.cmd = cmd
	s.mu.Unlock()

	log.Printf("[acpx-sup] exec: %s", label)
	err = cmd.Run()

	if ctx.Err() != nil {
		return nil
	}
	return err
}

// resolveAcpBridge finds the WebSocket bridge entry for production npm installs
// or monorepo development.
//
// Order:
//  1. node_modules/@1agents/acp-bridge/bridge-server.mjs (local / nested under @1agents/cli)
//  2. npm global root: $(npm root -g)/@1agents/acp-bridge/...
//  3. monorepo modules/1acp/bridge-server.js (dev, via tsx)
func resolveAcpBridge(cwd string) (acpBridge, error) {
	// 1) Walk cwd / parents and executable parents for node_modules/@1agents/acp-bridge
	for _, start := range acpSearchRoots(cwd) {
		for _, rel := range []string{
			filepath.Join("node_modules", "@1agents", "acp-bridge", "bridge-server.mjs"),
			filepath.Join("node_modules", "@1agents", "cli", "node_modules", "@1agents", "acp-bridge", "bridge-server.mjs"),
			filepath.Join("node_modules", "@1agents", "acp-bridge", "bridge-server.js"),
		} {
			candidate := filepath.Join(start, rel)
			if fileExists(candidate) {
				abs, _ := filepath.Abs(candidate)
				return acpBridge{
					script:  abs,
					workDir: filepath.Dir(abs),
					via:     "node_modules/@1agents/acp-bridge",
					useTsx:  false,
				}, nil
			}
		}
	}

	// 2) Global npm root
	if root := npmGlobalRoot(); root != "" {
		for _, rel := range []string{
			filepath.Join(root, "@1agents", "acp-bridge", "bridge-server.mjs"),
			filepath.Join(root, "@1agents", "cli", "node_modules", "@1agents", "acp-bridge", "bridge-server.mjs"),
		} {
			if fileExists(rel) {
				abs, _ := filepath.Abs(rel)
				return acpBridge{
					script:  abs,
					workDir: filepath.Dir(abs),
					via:     "npm root -g",
					useTsx:  false,
				}, nil
			}
		}
	}

	// 3) Dev: modules/1acp in monorepo
	for _, start := range acpSearchRoots(cwd) {
		devBridge := filepath.Join(start, "modules", "1acp", "bridge-server.js")
		if fileExists(devBridge) {
			abs, _ := filepath.Abs(devBridge)
			return acpBridge{
				script:  abs,
				workDir: filepath.Dir(abs),
				via:     "modules/1acp (dev)",
				useTsx:  true,
			}, nil
		}
	}

	return acpBridge{}, fmt.Errorf(
		"@1agents/acp-bridge not found (looked under node_modules and npm root -g); " +
			"also no modules/1acp/bridge-server.js for dev. " +
			"Reinstall: npm i -g @1agents/cli (pulls @1agents/acp-bridge)",
	)
}

func acpSearchRoots(cwd string) []string {
	var roots []string
	seen := map[string]struct{}{}
	add := func(p string) {
		if p == "" {
			return
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			abs = p
		}
		if _, ok := seen[abs]; ok {
			return
		}
		seen[abs] = struct{}{}
		roots = append(roots, abs)
	}
	add(cwd)
	dir := cwd
	for i := 0; i < 8; i++ {
		add(dir)
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if selfExe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(selfExe)
		add(exeDir)
		// e.g. .../node_modules/@1agents/core-linux-x64/bin -> walk up
		d := exeDir
		for i := 0; i < 8; i++ {
			add(d)
			parent := filepath.Dir(d)
			if parent == d {
				break
			}
			d = parent
		}
	}
	return roots
}

func npmGlobalRoot() string {
	cmd := exec.Command("npm", "root", "-g")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

func (s *AcpxSupervisor) stopProcess() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cmd != nil && s.cmd.Process != nil {
		log.Println("[acpx-sup] Sending SIGINT to acpx-server...")
		_ = s.cmd.Process.Signal(os.Interrupt)
	}
}

func acpxEnvironment(base []string, home string) []string {
	pathValue := ""
	for _, entry := range base {
		name, value, ok := strings.Cut(entry, "=")
		if ok && name == "PATH" {
			pathValue = value
			break
		}
	}

	paths := []string{
		filepath.Join(home, ".local", "bin"),
		filepath.Join(home, ".grok", "bin"),
	}
	if pathValue != "" {
		paths = append(paths, strings.Split(pathValue, string(os.PathListSeparator))...)
	}

	seen := make(map[string]struct{}, len(paths))
	unique := paths[:0]
	for _, path := range paths {
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		unique = append(unique, path)
	}

	return mergeEnvironment(base, map[string]string{
		"PATH": strings.Join(unique, string(os.PathListSeparator)),
	})
}
