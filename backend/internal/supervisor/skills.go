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

// SkillsSupervisor manages the lifecycle of the 1skills python FastAPI server.
//
// Launch priority (first match wins):
//  1. skill-manager binary next to 1agents executable (release mode, PyInstaller bundle)
//  2. modules/1skills/.venv/bin/python (development mode, source checkout)
//
// In development mode the supervisor auto-bootstraps the virtual environment
// and installs dependencies when it is missing.
// In release mode the self-contained binary is used directly.
// In both cases the supervisor restarts the process if it exits unexpectedly.
type SkillsSupervisor struct {
	cfg          *config.Config
	cmd          *exec.Cmd
	mu           sync.Mutex
	restartCount int
	done         chan struct{}
}

// NewSkills creates a new SkillsSupervisor with the given configuration.
func NewSkills(cfg *config.Config) *SkillsSupervisor {
	return &SkillsSupervisor{
		cfg:  cfg,
		done: make(chan struct{}),
	}
}

// Start launches the supervision loop in a background goroutine.
func (s *SkillsSupervisor) Start(ctx context.Context) {
	go s.supervisionLoop(ctx)
}

// Done returns a channel closed when the supervisor has fully stopped.
func (s *SkillsSupervisor) Done() <-chan struct{} {
	return s.done
}

// launchMode describes how the supervisor will run 1skills.
type launchMode int

const (
	launchModeBinary launchMode = iota // release: standalone skill-manager binary
	launchModeVenv                     // dev: .venv python + skill_manager module
)

// resolveRuntime decides which launch mode to use and returns the executable
// path together with the working directory that should be used.
//
// Search order:
//  1. skill-manager binary in the same directory as the running 1agents executable
//  2. skill-manager binary in ./bin/ (relative to CWD – matches release layout)
//  3. .venv python inside modules/1skills (development checkout)
func (s *SkillsSupervisor) resolveRuntime(cwd string) (mode launchMode, execPath string, skillsDir string) {
	// Find modules/1skills by traversing upwards from cwd
	foundDir := ""
	dir := cwd
	for {
		candidate := filepath.Join(dir, "modules", "1skills")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			foundDir = candidate
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir { // reached root
			break
		}
		dir = parent
	}

	if foundDir != "" {
		skillsDir = foundDir
	} else {
		skillsDir = filepath.Join(cwd, "modules", "1skills")
	}

	// 1. Next to the running executable (release layout: bin/1agents, bin/skill-manager)
	if selfExe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(selfExe), "skill-manager")
		if isExecutable(candidate) {
			log.Printf("[skills-sup] Found skill-manager binary next to executable: %s", candidate)
			return launchModeBinary, candidate, skillsDir
		}
	}

	// 2. ./bin/skill-manager relative to CWD
	candidate := filepath.Join(cwd, "bin", "skill-manager")
	if isExecutable(candidate) {
		log.Printf("[skills-sup] Found skill-manager binary in bin/: %s", candidate)
		return launchModeBinary, candidate, skillsDir
	}

	// 3. Development .venv
	venvPython := filepath.Join(skillsDir, ".venv", "bin", "python")
	log.Printf("[skills-sup] skill-manager binary not found, falling back to dev mode (.venv): %s", venvPython)
	return launchModeVenv, venvPython, skillsDir
}

// isExecutable returns true if path exists and is a regular executable file.
func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular() && info.Mode()&0o111 != 0
}

// supervisionLoop manages the check, bootstrap, and process watch cycle.
func (s *SkillsSupervisor) supervisionLoop(ctx context.Context) {
	defer close(s.done)

	cwd, err := os.Getwd()
	if err != nil {
		log.Printf("[skills-sup] Failed to determine working directory: %v", err)
		return
	}

	mode, execPath, skillsDir := s.resolveRuntime(cwd)
	log.Printf("[skills-sup] Launch mode: %v, exec: %s, skillsDir: %s", mode, execPath, skillsDir)

	// Dev-mode only: bootstrap venv if missing
	if mode == launchModeVenv {
		if !isExecutable(execPath) {
			log.Println("[skills-sup] Virtual environment not found. Attempting to bootstrap 1skills...")
			if err := s.bootstrap(ctx, skillsDir); err != nil {
				log.Printf("[skills-sup] Bootstrapping failed: %v. Server will not be started.", err)
				return
			}
			log.Println("[skills-sup] Bootstrapping completed successfully.")
		}
	}

	for {
		select {
		case <-ctx.Done():
			log.Println("[skills-sup] Shutdown requested, stopping 1skills.")
			s.stopProcess()
			return
		default:
		}

		if s.restartCount >= s.cfg.MaxRestarts {
			log.Printf("[skills-sup] FATAL: 1skills has restarted %d times consecutively. Giving up.", s.restartCount)
			return
		}

		log.Printf("[skills-sup] Starting 1skills microservice (attempt %d)...", s.restartCount+1)
		if err := s.startProcess(ctx, mode, skillsDir, execPath); err != nil {
			log.Printf("[skills-sup] 1skills exited with error: %v", err)
		} else {
			log.Println("[skills-sup] 1skills exited cleanly.")
		}

		if ctx.Err() != nil {
			log.Println("[skills-sup] Context cancelled after process exit, stopping supervisor.")
			return
		}

		s.mu.Lock()
		s.restartCount++
		count := s.restartCount
		s.mu.Unlock()

		log.Printf("[skills-sup] Restarting 1skills in %v... (%d/%d)",
			s.cfg.RestartDelay, count, s.cfg.MaxRestarts)

		select {
		case <-ctx.Done():
			log.Println("[skills-sup] Shutdown during restart wait, stopping.")
			return
		case <-time.After(s.cfg.RestartDelay):
		}
	}
}

// bootstrap initializes the virtual environment and installs dependencies (dev mode only).
func (s *SkillsSupervisor) bootstrap(ctx context.Context, dir string) error {
	pythonBin := s.cfg.SkillsBinaryPath
	if pythonBin == "" {
		pythonBin = "python3"
	}

	log.Printf("[skills-sup] Creating venv using %s in %s...", pythonBin, dir)
	cmdVenv := exec.CommandContext(ctx, pythonBin, "-m", "venv", ".venv")
	cmdVenv.Dir = dir
	cmdVenv.Stdout = os.Stdout
	cmdVenv.Stderr = os.Stderr
	if err := cmdVenv.Run(); err != nil {
		return err
	}

	pipPath := filepath.Join(dir, ".venv", "bin", "pip")
	log.Printf("[skills-sup] Installing requirements via %s...", pipPath)
	cmdPip := exec.CommandContext(ctx, pipPath, "install", "-r", "requirements.txt")
	cmdPip.Dir = dir
	cmdPip.Stdout = os.Stdout
	cmdPip.Stderr = os.Stderr
	return cmdPip.Run()
}

// startProcess runs the 1skills service and blocks until it exits.
// In binary mode the skill-manager executable is invoked directly.
// In venv mode the .venv python interpreter is invoked with -m skill_manager.
func (s *SkillsSupervisor) startProcess(ctx context.Context, mode launchMode, dir string, execPath string) error {
	port := s.portFrom(s.cfg.SkillsAddr)

	var cmd *exec.Cmd
	switch mode {
	case launchModeBinary:
		// skill-manager serve --host 127.0.0.1 --port <port> --no-open-browser
		cmd = exec.CommandContext(ctx, execPath,
			"serve",
			"--host", "127.0.0.1",
			"--port", port,
			"--no-open-browser",
		)
		// The binary is self-contained; no specific working directory is required.
	default: // launchModeVenv
		// python -m skill_manager serve --host 127.0.0.1 --port <port> --no-open-browser
		cmd = exec.CommandContext(ctx, execPath,
			"-m", "skill_manager", "serve",
			"--host", "127.0.0.1",
			"--port", port,
			"--no-open-browser",
		)
		cmd.Dir = dir
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	s.mu.Lock()
	s.cmd = cmd
	s.mu.Unlock()

	log.Printf("[skills-sup] exec: %s %s (Dir: %q)", execPath, strings.Join(cmd.Args[1:], " "), cmd.Dir)
	err := cmd.Run()

	if ctx.Err() != nil {
		return nil
	}
	return err
}

// stopProcess sends SIGINT or SIGKILL to stop the python process.
func (s *SkillsSupervisor) stopProcess() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cmd != nil && s.cmd.Process != nil {
		log.Println("[skills-sup] Sending SIGINT to 1skills...")
		_ = s.cmd.Process.Signal(os.Interrupt)
	}
}

// portFrom extracts the port number from an address string (e.g. "127.0.0.1:8000" -> "8000")
func (s *SkillsSupervisor) portFrom(addr string) string {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[i+1:]
		}
	}
	return addr
}
