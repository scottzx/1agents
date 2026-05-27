package tunnel

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"sync"
	"syscall"
	"time"
)

// TunnelSupervisor manages the lifecycle of the active on-demand cloudflared process.
type TunnelSupervisor struct {
	mu          sync.Mutex
	cmd         *exec.Cmd
	isActive    bool
	publicURL   string
	activeToken string

	idleTimeout time.Duration
	lastAccess  time.Time
	stopMonitor chan struct{}
}

// Global instance to allow access from HTTP server handlers.
var DefaultSupervisor = &TunnelSupervisor{}

// GenerateRandomToken generates a cryptographically secure 32-character hex token.
func GenerateRandomToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// SetIdleTimeout configures the inactivity timeout before the tunnel auto-stops.
func (s *TunnelSupervisor) SetIdleTimeout(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.idleTimeout = d
}

// RecordAccess updates the last-access timestamp to prevent idle timeout.
func (s *TunnelSupervisor) RecordAccess() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.isActive {
		s.lastAccess = time.Now()
	}
}

// Start launches the cloudflared quick tunnel subprocess.
// It blocks until the transient trycloudflare.com URL is successfully extracted, or a timeout occurs.
func (s *TunnelSupervisor) Start(localPort string) (string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isActive {
		return s.publicURL, s.activeToken, nil
	}

	// 1. Ensure cloudflared binary is installed
	binaryPath, err := EnsureBinary()
	if err != nil {
		return "", "", fmt.Errorf("failed to ensure cloudflared binary: %w", err)
	}

	// 2. Generate a secure session token
	token := GenerateRandomToken()

	// 3. Configure command to tunnel local HTTP gateway
	localURL := fmt.Sprintf("http://127.0.0.1:%s", localPort)
	args := []string{"tunnel", "--url", localURL}

	cmd := exec.Command(binaryPath, args...)

	// Cloudflared streams connection status and URL into stderr
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return "", "", fmt.Errorf("failed to create stderr pipe for cloudflared: %w", err)
	}

	log.Printf("[tunnel] Launching: %s %v", binaryPath, args)
	if err := cmd.Start(); err != nil {
		return "", "", fmt.Errorf("failed to start cloudflared process: %w", err)
	}

	// Channel to receive the parsed public URL
	urlChan := make(chan string, 1)
	errChan := make(chan error, 1)

	// Regex to extract trycloudflare public tunnel URLs
	cfURLRegex := regexp.MustCompile(`https://[a-zA-Z0-9\-]+\.trycloudflare\.com`)

	// Scan stderr in real-time
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		var extractedURL string
		for scanner.Scan() {
			line := scanner.Text()
			// Forward lines to main daemon stdout for visibility/debugging
			log.Printf("[cloudflared-output] %s", line)

			if matches := cfURLRegex.FindStringSubmatch(line); len(matches) > 0 {
				extractedURL = matches[0]
				urlChan <- extractedURL
				break
			}
		}

		// Keep scanning in background to prevent pipe block after extraction
		for scanner.Scan() {
			_ = scanner.Text()
		}

		if err := scanner.Err(); err != nil {
			errChan <- err
		}
	}()

	// Wait for extraction or a timeout (15s)
	select {
	case url := <-urlChan:
		s.cmd = cmd
		s.isActive = true
		s.publicURL = url
		s.activeToken = token
		s.lastAccess = time.Now()
		log.Printf("[tunnel] Cloudflare tunnel established successfully: %s", url)
		log.Printf("[tunnel] Active dynamic session token: %s", token)

		// Start idle monitor if timeout is configured
		if s.idleTimeout > 0 {
			s.stopMonitor = make(chan struct{})
			go s.idleMonitor(s.stopMonitor)
		}
		return url, token, nil

	case err := <-errChan:
		_ = cmd.Process.Kill()
		return "", "", fmt.Errorf("scanner error during tunnel start: %w", err)

	case <-time.After(15 * time.Second):
		log.Println("[tunnel] Timeout waiting for Cloudflare tunnel URL extraction. Terminating process.")
		_ = cmd.Process.Kill()
		return "", "", fmt.Errorf("timeout waiting for tunnel to establish (network issue or DNS throttle)")
	}
}

// Stop terminates the running cloudflared process gracefully.
func (s *TunnelSupervisor) Stop() error {
	s.mu.Lock()

	if !s.isActive {
		s.mu.Unlock()
		return nil
	}

	// Signal idle monitor to exit
	if s.stopMonitor != nil {
		close(s.stopMonitor)
		s.stopMonitor = nil
	}

	s.mu.Unlock()

	log.Println("[tunnel] Shutting down Cloudflare tunnel subprocess...")

	// Kill subprocess (no lock held during wait to avoid deadlock)
	var err error
	if s.cmd != nil && s.cmd.Process != nil {
		// Send SIGINT for clean disconnection on the edge, fallback to SIGKILL if it hangs
		if runtimeOS := s.cmd.Process.Signal(syscall.SIGINT); runtimeOS != nil {
			_ = s.cmd.Process.Kill()
		}

		// Wait in a separate goroutine to release system process resource
		done := make(chan error, 1)
		go func() {
			done <- s.cmd.Wait()
		}()

		select {
		case err = <-done:
			log.Println("[tunnel] cloudflared subprocess exited.")
		case <-time.After(3 * time.Second):
			log.Println("[tunnel] cloudflared didn't exit in time, forcing kill.")
			_ = s.cmd.Process.Kill()
		}
	}

	s.mu.Lock()
	s.cmd = nil
	s.isActive = false
	s.publicURL = ""
	s.activeToken = ""
	s.mu.Unlock()

	log.Println("[tunnel] Public tunnel closed and session token revoked.")
	return err
}

// GetStatus returns the current tunnel state, transient URL, and active session token.
func (s *TunnelSupervisor) GetStatus() (bool, string, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isActive, s.publicURL, s.activeToken
}

// idleMonitor periodically checks if the tunnel has been idle too long and auto-stops it.
func (s *TunnelSupervisor) idleMonitor(stop chan struct{}) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			s.mu.Lock()
			if !s.isActive {
				s.mu.Unlock()
				return
			}
			idle := time.Since(s.lastAccess)
			timeout := s.idleTimeout
			s.mu.Unlock()

			if idle > timeout {
				log.Printf("[tunnel] Idle timeout reached (%.0f min). Auto-stopping tunnel.", idle.Minutes())
				_ = s.Stop()
				return
			}
		}
	}
}

// PortFrom extracts the port string from an "addr:port" string.
func PortFrom(addr string) string {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[i+1:]
		}
	}
	return addr
}
