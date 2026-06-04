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
// It automatically bootstraps the virtual environment if it does not exist,
// and restarts the server if it exits unexpectedly.
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

// supervisionLoop manages the check, bootstrap, and process watch cycle.
func (s *SkillsSupervisor) supervisionLoop(ctx context.Context) {
	defer close(s.done)

	// Resolve skillsDir to an absolute path so it remains valid regardless of CWD changes.
	cwd, err := os.Getwd()
	if err != nil {
		log.Printf("[skills-sup] Failed to determine working directory: %v", err)
		return
	}
	skillsDir := filepath.Join(cwd, "modules", "1skills")
	venvPython := filepath.Join(skillsDir, ".venv", "bin", "python")
	log.Printf("[skills-sup] Using skillsDir: %s", skillsDir)

	// 1. Asynchronously bootstrap if .venv is missing
	if _, err := os.Stat(venvPython); os.IsNotExist(err) {
		log.Println("[skills-sup] Virtual environment not found. Attempting to bootstrap 1skills...")
		if err := s.bootstrap(ctx, skillsDir); err != nil {
			log.Printf("[skills-sup] Bootstrapping failed: %v. Server will not be started.", err)
			return
		}
		log.Println("[skills-sup] Bootstrapping completed successfully.")
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
		if err := s.startProcess(ctx, skillsDir, venvPython); err != nil {
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

// bootstrap initializes the virtual environment and installs dependencies.
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

// startProcess runs the 1skills FastAPI command and blocks until it exits.
func (s *SkillsSupervisor) startProcess(ctx context.Context, dir string, pythonPath string) error {
	port := s.portFrom(s.cfg.SkillsAddr)
	args := []string{
		"-m", "skill_manager", "serve",
		"--host", "127.0.0.1",
		"--port", port,
		"--no-open-browser",
	}

	cmd := exec.CommandContext(ctx, pythonPath, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	s.mu.Lock()
	s.cmd = cmd
	s.mu.Unlock()

	log.Printf("[skills-sup] exec: %s %s in Dir: %s", pythonPath, strings.Join(args, " "), dir)
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
