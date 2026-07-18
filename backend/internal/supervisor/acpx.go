package supervisor

import (
	"context"
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

	// Resolve modules/1acp path
	dir := cwd
	foundDir := ""
	for {
		candidate := filepath.Join(dir, "modules", "1acp")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			foundDir = candidate
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if foundDir == "" {
		foundDir = filepath.Join(cwd, "modules", "1acp")
	}

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
		if err := s.startProcess(ctx, foundDir); err != nil {
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

func (s *AcpxSupervisor) startProcess(ctx context.Context, dir string) error {
	acpxPort := "38082"
	if v := os.Getenv("ACPX_PORT"); v != "" {
		acpxPort = v
	}

	// Prefer installed acpx package (npm registry dist) over submodule+tsx.
	cmd, label := resolveAcpxCommand(ctx, dir)
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

// resolveAcpxCommand prefers:
//  1. `acpx` on PATH (npm global / @1agents install)
//  2. node running acpx dist/cli.js from node_modules
//  3. legacy modules/1acp + npx tsx bridge-server.js (dev only)
func resolveAcpxCommand(ctx context.Context, modulesAcpDir string) (*exec.Cmd, string) {
	if p, err := exec.LookPath("acpx"); err == nil {
		// Many acpx builds expose CLI; bridge may still need bridge-server.
		// Prefer package root bridge when present, else run acpx as long-running helper is insufficient.
		// Fall through to node dist if bridge-server.js exists next to package.
		_ = p
	}

	// Walk for node_modules/acpx or node_modules/@1agents/acpx
	for _, name := range []string{
		"node_modules/acpx",
		"node_modules/@1agents/acpx",
	} {
		if root := findUp(modulesAcpDir, name); root != "" {
			if bridge := filepath.Join(root, "bridge-server.js"); fileExists(bridge) {
				cmd := exec.CommandContext(ctx, "node", bridge)
				cmd.Dir = root
				return cmd, "node " + bridge
			}
			if cli := filepath.Join(root, "dist", "cli.js"); fileExists(cli) {
				// No dedicated bridge entry: keep legacy bridge from modules if available.
				log.Printf("[acpx-sup] found %s but no bridge-server.js; trying modules/1acp fallback", root)
			}
		}
	}

	// Dev / submodule fallback
	bridge := filepath.Join(modulesAcpDir, "bridge-server.js")
	if fileExists(bridge) {
		cmd := exec.CommandContext(ctx, "npx", "tsx", "bridge-server.js")
		cmd.Dir = modulesAcpDir
		return cmd, "npx tsx bridge-server.js (modules/1acp)"
	}

	// Last resort: acpx on PATH with no bridge knowledge
	if p, err := exec.LookPath("acpx"); err == nil {
		cmd := exec.CommandContext(ctx, p, "--help")
		return cmd, p + " --help (no bridge-server found; ACP bridge may be unavailable)"
	}

	cmd := exec.CommandContext(ctx, "npx", "tsx", "bridge-server.js")
	cmd.Dir = modulesAcpDir
	return cmd, "npx tsx bridge-server.js (fallback)"
}

func findUp(start, rel string) string {
	dir := start
	for i := 0; i < 8; i++ {
		candidate := filepath.Join(dir, rel)
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			return candidate
		}
		// also search parent/node_modules paths when start is modules/1acp
		candidate = filepath.Join(dir, "..", rel)
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			abs, _ := filepath.Abs(candidate)
			return abs
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
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
