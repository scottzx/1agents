package supervisor

import (
	"context"
	"log"
	"os"
	"os/exec"
	"path/filepath"
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
	cmd := exec.CommandContext(ctx, "npx", "tsx", "bridge-server.js")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "ACPX_PORT=38082")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err == nil {
		defer stdin.Close()
	}

	s.mu.Lock()
	s.cmd = cmd
	s.mu.Unlock()

	log.Printf("[acpx-sup] exec: npx tsx bridge-server.js (Dir: %s)", dir)
	err = cmd.Run()

	if ctx.Err() != nil {
		return nil
	}
	return err
}

func (s *AcpxSupervisor) stopProcess() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cmd != nil && s.cmd.Process != nil {
		log.Println("[acpx-sup] Sending SIGINT to acpx-server...")
		_ = s.cmd.Process.Signal(os.Interrupt)
	}
}
