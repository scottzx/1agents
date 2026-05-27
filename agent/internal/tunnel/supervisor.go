package tunnel

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"sort"
	"sync"
	"syscall"
	"time"
)

// TunnelInfo holds the public state of one active tunnel.
type TunnelInfo struct {
	Port  string `json:"port"`
	URL   string `json:"url"`
	Token string `json:"token"`
	Link  string `json:"link"`
}

type tunnelInstance struct {
	cmd         *exec.Cmd
	publicURL   string
	token       string
	lastAccess  time.Time
	stopMonitor chan struct{}
}

// TunnelSupervisor manages the lifecycle of on-demand cloudflared tunnel processes.
// Multiple tunnels can run concurrently, each exposing a different local port.
type TunnelSupervisor struct {
	mu          sync.Mutex
	tunnels     map[string]*tunnelInstance // keyed by local port string, e.g. "8080"
	idleTimeout time.Duration
}

// Global instance to allow access from HTTP server handlers.
var DefaultSupervisor = &TunnelSupervisor{
	tunnels: make(map[string]*tunnelInstance),
}

// GenerateRandomToken generates a cryptographically secure 32-character hex token.
func GenerateRandomToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// SetIdleTimeout configures the inactivity timeout before a tunnel auto-stops.
func (s *TunnelSupervisor) SetIdleTimeout(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.idleTimeout = d
}

// RecordAccess updates the last-access timestamp for all active tunnels.
func (s *TunnelSupervisor) RecordAccess() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, inst := range s.tunnels {
		inst.lastAccess = time.Now()
	}
}

// Start launches a cloudflared quick tunnel for the given local port.
// Returns the existing tunnel info if one is already active for that port.
func (s *TunnelSupervisor) Start(localPort string) (string, string, error) {
	s.mu.Lock()
	if inst, ok := s.tunnels[localPort]; ok {
		url, token := inst.publicURL, inst.token
		s.mu.Unlock()
		return url, token, nil
	}
	s.mu.Unlock()

	binaryPath, err := EnsureBinary()
	if err != nil {
		return "", "", fmt.Errorf("failed to ensure cloudflared binary: %w", err)
	}

	token := GenerateRandomToken()
	localURL := fmt.Sprintf("http://127.0.0.1:%s", localPort)
	args := []string{"tunnel", "--url", localURL}

	cmd := exec.Command(binaryPath, args...)
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return "", "", fmt.Errorf("failed to create stderr pipe for cloudflared: %w", err)
	}

	log.Printf("[tunnel:%s] Launching: %s %v", localPort, binaryPath, args)
	if err := cmd.Start(); err != nil {
		return "", "", fmt.Errorf("failed to start cloudflared process: %w", err)
	}

	urlChan := make(chan string, 1)
	errChan := make(chan error, 1)
	cfURLRegex := regexp.MustCompile(`https://[a-zA-Z0-9\-]+\.trycloudflare\.com`)

	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		var extractedURL string
		for scanner.Scan() {
			line := scanner.Text()
			log.Printf("[cloudflared:%s] %s", localPort, line)
			if matches := cfURLRegex.FindStringSubmatch(line); len(matches) > 0 {
				extractedURL = matches[0]
				urlChan <- extractedURL
				break
			}
		}
		for scanner.Scan() {
			_ = scanner.Text()
		}
		if err := scanner.Err(); err != nil {
			errChan <- err
		}
	}()

	select {
	case url := <-urlChan:
		inst := &tunnelInstance{
			cmd:        cmd,
			publicURL:  url,
			token:      token,
			lastAccess: time.Now(),
		}

		s.mu.Lock()
		s.tunnels[localPort] = inst
		timeout := s.idleTimeout
		s.mu.Unlock()

		log.Printf("[tunnel:%s] Cloudflare tunnel established: %s", localPort, url)

		if timeout > 0 {
			inst.stopMonitor = make(chan struct{})
			go s.idleMonitor(localPort, inst, inst.stopMonitor)
		}
		return url, token, nil

	case err := <-errChan:
		_ = cmd.Process.Kill()
		return "", "", fmt.Errorf("scanner error during tunnel start: %w", err)

	case <-time.After(15 * time.Second):
		log.Printf("[tunnel:%s] Timeout waiting for Cloudflare tunnel URL.", localPort)
		_ = cmd.Process.Kill()
		return "", "", fmt.Errorf("timeout waiting for tunnel to establish")
	}
}

// Stop terminates the cloudflared tunnel for the given local port.
func (s *TunnelSupervisor) Stop(localPort string) error {
	s.mu.Lock()
	inst, ok := s.tunnels[localPort]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("no active tunnel for port %s", localPort)
	}
	delete(s.tunnels, localPort)
	s.mu.Unlock()

	return s.killInstance(localPort, inst)
}

// StopAll terminates every active tunnel.
func (s *TunnelSupervisor) StopAll() []string {
	s.mu.Lock()
	ports := make([]string, 0, len(s.tunnels))
	instances := make(map[string]*tunnelInstance, len(s.tunnels))
	for p, inst := range s.tunnels {
		ports = append(ports, p)
		instances[p] = inst
	}
	s.tunnels = make(map[string]*tunnelInstance)
	s.mu.Unlock()

	sort.Strings(ports)
	for _, p := range ports {
		_ = s.killInstance(p, instances[p])
	}
	return ports
}

// ListAll returns info for every active tunnel, sorted by port.
func (s *TunnelSupervisor) ListAll() []TunnelInfo {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]TunnelInfo, 0, len(s.tunnels))
	for port, inst := range s.tunnels {
		result = append(result, TunnelInfo{
			Port:  port,
			URL:   inst.publicURL,
			Token: inst.token,
			Link:  fmt.Sprintf("%s/?token=%s", inst.publicURL, inst.token),
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Port < result[j].Port })
	return result
}

// ValidateToken checks whether the given token matches any active tunnel.
func (s *TunnelSupervisor) ValidateToken(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, inst := range s.tunnels {
		if inst.token == token {
			inst.lastAccess = time.Now()
			return true
		}
	}
	return false
}

// HasAnyActive returns true if at least one tunnel is running.
func (s *TunnelSupervisor) HasAnyActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.tunnels) > 0
}

// killInstance terminates a single cloudflared process.
func (s *TunnelSupervisor) killInstance(port string, inst *tunnelInstance) error {
	if inst.stopMonitor != nil {
		close(inst.stopMonitor)
	}

	log.Printf("[tunnel:%s] Shutting down Cloudflare tunnel...", port)

	if inst.cmd != nil && inst.cmd.Process != nil {
		_ = inst.cmd.Process.Signal(syscall.SIGINT)

		done := make(chan error, 1)
		go func() { done <- inst.cmd.Wait() }()

		select {
		case <-done:
			log.Printf("[tunnel:%s] cloudflared subprocess exited.", port)
		case <-time.After(3 * time.Second):
			log.Printf("[tunnel:%s] cloudflared didn't exit in time, forcing kill.", port)
			_ = inst.cmd.Process.Kill()
		}
	}

	log.Printf("[tunnel:%s] Tunnel closed, token revoked.", port)
	return nil
}

// idleMonitor periodically checks if a tunnel has been idle too long.
func (s *TunnelSupervisor) idleMonitor(port string, inst *tunnelInstance, stop chan struct{}) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			s.mu.Lock()
			timeout := s.idleTimeout
			_, stillTracked := s.tunnels[port]
			s.mu.Unlock()

			if !stillTracked {
				return
			}

			if time.Since(inst.lastAccess) > timeout {
				log.Printf("[tunnel:%s] Idle timeout reached (%.0f min). Auto-stopping.", port, time.Since(inst.lastAccess).Minutes())
				s.mu.Lock()
				delete(s.tunnels, port)
				s.mu.Unlock()
				_ = s.killInstance(port, inst)
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
