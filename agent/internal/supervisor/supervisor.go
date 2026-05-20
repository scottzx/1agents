package supervisor

import (
	"context"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/scottzx/remote-agents/agent/internal/config"
)

// Supervisor manages the lifecycle of the ttyd child process.
// It starts ttyd on a localhost-only port and automatically restarts it
// if it exits unexpectedly, up to MaxRestarts consecutive times.
type Supervisor struct {
	cfg          *config.Config
	cmd          *exec.Cmd
	mu           sync.Mutex
	restartCount int
	done         chan struct{}
}

// New creates a new Supervisor with the given configuration.
func New(cfg *config.Config) *Supervisor {
	return &Supervisor{
		cfg:  cfg,
		done: make(chan struct{}),
	}
}

// Start launches the supervision loop in a background goroutine.
// The loop runs until ctx is cancelled.
func (s *Supervisor) Start(ctx context.Context) {
	go s.supervisionLoop(ctx)
}

// Done returns a channel that is closed when the supervisor has fully stopped
// (including sending the termination signal to ttyd).
func (s *Supervisor) Done() <-chan struct{} {
	return s.done
}

// ResetRestartCount resets the consecutive restart counter.
// Call this after the process has been stable for a while to avoid hitting
// MaxRestarts due to infrequent but repeated crashes.
func (s *Supervisor) ResetRestartCount() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restartCount = 0
}

// supervisionLoop is the core watch-and-restart loop.
func (s *Supervisor) supervisionLoop(ctx context.Context) {
	defer close(s.done)

	for {
		// Check if we've been asked to shut down before attempting a start.
		select {
		case <-ctx.Done():
			log.Println("[supervisor] Shutdown requested, stopping.")
			s.stopProcess()
			return
		default:
		}

		// Honour the maximum restart cap.
		if s.restartCount >= s.cfg.MaxRestarts {
			log.Printf("[supervisor] FATAL: ttyd has restarted %d times consecutively. Giving up.", s.restartCount)
			return
		}

		// Start the child process and block until it exits.
		log.Printf("[supervisor] Starting ttyd (attempt %d)...", s.restartCount+1)
		if err := s.startProcess(ctx); err != nil {
			log.Printf("[supervisor] ttyd exited with error: %v", err)
		} else {
			log.Println("[supervisor] ttyd exited cleanly.")
		}

		// If the exit was caused by context cancellation, stop the loop.
		if ctx.Err() != nil {
			log.Println("[supervisor] Context cancelled after process exit, stopping supervisor.")
			return
		}

		s.mu.Lock()
		s.restartCount++
		count := s.restartCount
		s.mu.Unlock()

		log.Printf("[supervisor] Restarting ttyd in %v... (%d/%d)",
			s.cfg.RestartDelay, count, s.cfg.MaxRestarts)

		// Wait for the restart delay, but bail early on shutdown.
		select {
		case <-ctx.Done():
			log.Println("[supervisor] Shutdown during restart wait, stopping.")
			return
		case <-time.After(s.cfg.RestartDelay):
		}
	}
}

// startProcess builds and runs the ttyd command, blocking until it exits.
// ttyd is forced to listen only on the loopback address so that all
// external traffic must flow through the Go gateway.
func (s *Supervisor) startProcess(ctx context.Context) error {
	// Force ttyd onto the loopback interface and the configured port.
	// The port number is extracted from TtydAddr (e.g. "127.0.0.1:7681" → "7681").
	port := portFrom(s.cfg.TtydAddr)
	args := []string{
		"-p", port,      // port
		"-i", "127.0.0.1", // bind to loopback only
	}
	args = append(args, s.cfg.TtydArgs...)

	cmd := exec.CommandContext(ctx, s.cfg.TtydBinaryPath, args...)

	// Mirror ttyd stdout/stderr into our own logs for easy debugging.
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	s.mu.Lock()
	s.cmd = cmd
	s.mu.Unlock()

	log.Printf("[supervisor] exec: %s %v", s.cfg.TtydBinaryPath, args)
	err := cmd.Run()

	// Ignore errors that are simply a result of ctx cancellation.
	if ctx.Err() != nil {
		return nil
	}
	return err
}

// stopProcess sends SIGINT to the ttyd process for a graceful shutdown.
func (s *Supervisor) stopProcess() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cmd != nil && s.cmd.Process != nil {
		log.Println("[supervisor] Sending SIGINT to ttyd...")
		_ = s.cmd.Process.Signal(os.Interrupt)
	}
}

// portFrom extracts the port string from an "addr:port" string.
func portFrom(addr string) string {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[i+1:]
		}
	}
	return addr
}
