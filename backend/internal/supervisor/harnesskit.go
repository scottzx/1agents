package supervisor

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/scottzx/1Agents/backend/internal/config"
)

const (
	harnessKitModeSupervised = "supervised"
	harnessKitModeExternal   = "external"
)

// HarnessKitStatus is the public, credential-free daemon state exposed by
// /api/harnesskit/status.
type HarnessKitStatus struct {
	Mode          string    `json:"mode"`
	State         string    `json:"state"`
	Ready         bool      `json:"ready"`
	Port          int       `json:"port,omitempty"`
	RestartCount  int       `json:"restartCount"`
	MaxRestarts   int       `json:"maxRestarts"`
	LastError     string    `json:"lastError,omitempty"`
	LastChangedAt time.Time `json:"lastChangedAt"`
}

type harnessKitCommandFactory func(context.Context, string, ...string) *exec.Cmd

// HarnessKitSupervisor owns the authenticated loopback hk serve process.
// Credentials remain private to the backend and rotate on every child start.
type HarnessKitSupervisor struct {
	cfg *config.Config

	mu       sync.RWMutex
	cmd      *exec.Cmd
	endpoint string
	token    string
	status   HarnessKitStatus
	done     chan struct{}

	commandFactory   harnessKitCommandFactory
	readinessClient  *http.Client
	readinessTimeout time.Duration
}

// NewHarnessKit creates a HarnessKit supervisor. Start is intentionally
// asynchronous so a missing extension daemon degrades only the Extensions
// surface and never prevents the rest of 1agents from starting.
func NewHarnessKit(cfg *config.Config) *HarnessKitSupervisor {
	mode := strings.ToLower(strings.TrimSpace(cfg.HarnessKitMode))
	if mode == "" {
		mode = harnessKitModeSupervised
	}
	return &HarnessKitSupervisor{
		cfg:  cfg,
		done: make(chan struct{}),
		status: HarnessKitStatus{
			Mode:          mode,
			State:         "starting",
			MaxRestarts:   cfg.MaxRestarts,
			LastChangedAt: time.Now().UTC(),
		},
		commandFactory: exec.CommandContext,
		readinessClient: &http.Client{
			Timeout: 750 * time.Millisecond,
		},
		readinessTimeout: 15 * time.Second,
	}
}

// Start launches the supervision loop.
func (s *HarnessKitSupervisor) Start(ctx context.Context) {
	go s.supervisionLoop(ctx)
}

// Done closes after the supervisor and any child process have stopped.
func (s *HarnessKitSupervisor) Done() <-chan struct{} {
	return s.done
}

// Status returns a credential-free point-in-time snapshot.
func (s *HarnessKitSupervisor) Status() HarnessKitStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

// Endpoint returns private proxy connection details only while the daemon is
// ready. The token must never be serialized into an HTTP response or log.
func (s *HarnessKitSupervisor) Endpoint() (baseURL, token string, ready bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.status.Ready || s.endpoint == "" || s.token == "" {
		return "", "", false
	}
	return s.endpoint, s.token, true
}

func (s *HarnessKitSupervisor) supervisionLoop(ctx context.Context) {
	defer close(s.done)

	switch s.status.Mode {
	case harnessKitModeSupervised:
		s.superviseChild(ctx)
	case harnessKitModeExternal:
		s.monitorExternal(ctx)
	default:
		s.setDegraded(fmt.Errorf("unsupported HarnessKit mode %q", s.status.Mode))
	}
}

func (s *HarnessKitSupervisor) superviseChild(ctx context.Context) {
	if s.cfg.HarnessKitHost != "127.0.0.1" {
		s.setDegraded(fmt.Errorf("supervised HarnessKit host must be 127.0.0.1"))
		return
	}

	binary, err := s.resolveBinary()
	if err != nil {
		s.setDegraded(err)
		return
	}

	maxRestarts := s.cfg.MaxRestarts
	if maxRestarts < 1 {
		maxRestarts = 1
	}

	for attempt := 0; attempt < maxRestarts; attempt++ {
		if ctx.Err() != nil {
			s.setStopped()
			return
		}

		err := s.runChild(ctx, binary)
		if ctx.Err() != nil {
			s.setStopped()
			return
		}

		s.mu.Lock()
		s.status.RestartCount = attempt + 1
		s.status.MaxRestarts = maxRestarts
		s.mu.Unlock()
		s.setDegraded(err)

		if attempt+1 >= maxRestarts {
			log.Printf("[harnesskit-sup] hk stopped after %d attempts: %v", maxRestarts, err)
			return
		}

		delay := s.cfg.RestartDelay
		if delay <= 0 {
			delay = time.Second
		}
		log.Printf("[harnesskit-sup] restarting hk in %s (%d/%d)", delay, attempt+1, maxRestarts)
		select {
		case <-ctx.Done():
			s.setStopped()
			return
		case <-time.After(delay):
		}
	}
}

func (s *HarnessKitSupervisor) runChild(ctx context.Context, binary string) error {
	port, err := allocateHarnessKitPort(s.cfg.HarnessKitHost, s.cfg.HarnessKitPort)
	if err != nil {
		return fmt.Errorf("allocate loopback port: %w", err)
	}
	token, err := newHarnessKitToken()
	if err != nil {
		return fmt.Errorf("generate daemon token: %w", err)
	}
	if err := os.MkdirAll(s.cfg.HarnessKitDataDir, 0o700); err != nil {
		return fmt.Errorf("create HarnessKit data directory: %w", err)
	}

	endpoint := "http://" + net.JoinHostPort(s.cfg.HarnessKitHost, strconv.Itoa(port))
	args := []string{
		"serve",
		"--host", s.cfg.HarnessKitHost,
		"--port", strconv.Itoa(port),
		"--data-dir", s.cfg.HarnessKitDataDir,
	}
	cmd := s.commandFactory(ctx, binary, args...)
	stdout := newSecretRedactingWriter(os.Stdout, token)
	stderr := newSecretRedactingWriter(os.Stderr, token)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = environmentWith("HARNESSKIT_TOKEN", token)

	s.mu.Lock()
	s.cmd = cmd
	s.endpoint = endpoint
	s.token = token
	s.status.State = "starting"
	s.status.Ready = false
	s.status.Port = port
	s.status.LastError = ""
	s.status.LastChangedAt = time.Now().UTC()
	s.mu.Unlock()

	log.Printf("[harnesskit-sup] starting %s serve --host %s --port %d --data-dir %s (token via environment)",
		binary, s.cfg.HarnessKitHost, port, s.cfg.HarnessKitDataDir)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start hk: %w", err)
	}

	waitCh := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		_ = stdout.Flush()
		_ = stderr.Flush()
		waitCh <- err
	}()

	readinessCtx, cancel := context.WithTimeout(ctx, s.readinessTimeout)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-readinessCtx.Done():
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			<-waitCh
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("hk readiness timeout after %s", s.readinessTimeout)
		case err := <-waitCh:
			if err == nil {
				return errors.New("hk exited before readiness")
			}
			return fmt.Errorf("hk exited before readiness: %w", err)
		case <-ticker.C:
			if s.healthReady(readinessCtx, endpoint, token) {
				s.setReady(port)
				log.Printf("[harnesskit-sup] hk ready on 127.0.0.1:%d", port)
				goto ready
			}
		}
	}

ready:
	select {
	case <-ctx.Done():
		s.stopProcess()
		<-waitCh
		return nil
	case err := <-waitCh:
		if err == nil {
			return errors.New("hk exited unexpectedly")
		}
		return fmt.Errorf("hk exited: %w", err)
	}
}

func (s *HarnessKitSupervisor) monitorExternal(ctx context.Context) {
	host := strings.TrimSpace(s.cfg.HarnessKitHost)
	if host == "" || s.cfg.HarnessKitPort < 1 {
		s.setDegraded(errors.New("external HarnessKit requires a host and port"))
		return
	}
	token := strings.TrimSpace(s.cfg.HarnessKitToken)
	if token == "" {
		token = strings.TrimSpace(os.Getenv("ONEAGENTS_HARNESSKIT_TOKEN"))
	}
	if token == "" {
		s.setDegraded(errors.New("external HarnessKit requires ONEAGENTS_HARNESSKIT_TOKEN"))
		return
	}
	endpoint := "http://" + net.JoinHostPort(host, strconv.Itoa(s.cfg.HarnessKitPort))

	s.mu.Lock()
	s.endpoint = endpoint
	s.token = token
	s.status.Port = s.cfg.HarnessKitPort
	s.mu.Unlock()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if s.healthReady(ctx, endpoint, token) {
			s.setReady(s.cfg.HarnessKitPort)
		} else {
			s.setDegraded(errors.New("external HarnessKit health check failed"))
		}
		select {
		case <-ctx.Done():
			s.setStopped()
			return
		case <-ticker.C:
		}
	}
}

func (s *HarnessKitSupervisor) healthReady(ctx context.Context, endpoint, token string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/api/health", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := s.readinessClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func (s *HarnessKitSupervisor) setReady(port int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status.State = "ready"
	s.status.Ready = true
	s.status.Port = port
	s.status.LastError = ""
	s.status.LastChangedAt = time.Now().UTC()
}

func (s *HarnessKitSupervisor) setDegraded(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status.State = "degraded"
	s.status.Ready = false
	if err != nil {
		s.status.LastError = publicHarnessKitError(err)
	}
	s.status.LastChangedAt = time.Now().UTC()
}

func (s *HarnessKitSupervisor) setStopped() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status.State = "stopped"
	s.status.Ready = false
	s.status.LastChangedAt = time.Now().UTC()
}

func (s *HarnessKitSupervisor) stopProcess() {
	s.mu.RLock()
	cmd := s.cmd
	s.mu.RUnlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Signal(os.Interrupt)
	}
}

func (s *HarnessKitSupervisor) resolveBinary() (string, error) {
	name := "hk"
	if runtime.GOOS == "windows" {
		name = "hk.exe"
	}

	var candidates []string
	if explicit := strings.TrimSpace(s.cfg.HarnessKitBinaryPath); explicit != "" {
		if !isExecutable(explicit) {
			return "", fmt.Errorf("configured HarnessKit binary is not executable")
		}
		absolute, err := filepath.Abs(explicit)
		if err == nil {
			return absolute, nil
		}
		return explicit, nil
	}
	if env := strings.TrimSpace(os.Getenv("ONEAGENTS_HARNESSKIT_BIN")); env != "" {
		if !isExecutable(env) {
			return "", fmt.Errorf("ONEAGENTS_HARNESSKIT_BIN is not executable")
		}
		absolute, err := filepath.Abs(env)
		if err == nil {
			return absolute, nil
		}
		return env, nil
	}
	if self, err := os.Executable(); err == nil {
		candidates = append(candidates,
			filepath.Join(filepath.Dir(self), name),
			filepath.Join(filepath.Dir(self), "bin", name),
		)
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(cwd, "bin", name),
			filepath.Join(cwd, "build", name),
			filepath.Join(cwd, "modules", "HarnessKit", "target", "release", name),
			filepath.Join(cwd, "modules", "HarnessKit", "target", "debug", name),
			filepath.Join(cwd, "..", "modules", "HarnessKit", "target", "release", name),
		)
	}

	for _, candidate := range candidates {
		if isExecutable(candidate) {
			absolute, err := filepath.Abs(candidate)
			if err == nil {
				return absolute, nil
			}
			return candidate, nil
		}
	}
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("HarnessKit binary not found; pass -harnesskit-bin or set ONEAGENTS_HARNESSKIT_BIN")
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	return runtime.GOOS == "windows" || info.Mode().Perm()&0o111 != 0
}

func allocateHarnessKitPort(host string, requested int) (int, error) {
	if host != "127.0.0.1" {
		return 0, fmt.Errorf("host %q is not supervised loopback", host)
	}
	address := net.JoinHostPort(host, "0")
	if requested > 0 {
		address = net.JoinHostPort(host, strconv.Itoa(requested))
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func newHarnessKitToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func environmentWith(key, value string) []string {
	prefix := key + "="
	environment := os.Environ()
	filtered := environment[:0]
	for _, entry := range environment {
		if !strings.HasPrefix(entry, prefix) {
			filtered = append(filtered, entry)
		}
	}
	return append(filtered, prefix+value)
}

func publicHarnessKitError(err error) string {
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "binary not found"), strings.Contains(message, "binary is not executable"):
		return "binary_not_found"
	case strings.Contains(message, "host must be"):
		return "invalid_supervised_host"
	case strings.Contains(message, "requires a host and port"):
		return "external_endpoint_required"
	case strings.Contains(message, "requires oneagents_harnesskit_token"):
		return "external_token_required"
	case strings.Contains(message, "readiness timeout"):
		return "readiness_timeout"
	case strings.Contains(message, "health check failed"):
		return "health_check_failed"
	case strings.Contains(message, "data directory"):
		return "data_directory_unavailable"
	case strings.Contains(message, "allocate loopback port"):
		return "port_unavailable"
	case strings.Contains(message, "exited"):
		return "process_exited"
	default:
		return "unavailable"
	}
}

// secretRedactingWriter prevents a daemon regression from writing the
// supervisor-generated token into the parent process logs. It retains the
// minimum suffix needed to detect a token split across Write calls.
type secretRedactingWriter struct {
	mu      sync.Mutex
	dst     io.Writer
	secret  []byte
	pending []byte
}

func newSecretRedactingWriter(dst io.Writer, secret string) *secretRedactingWriter {
	return &secretRedactingWriter{dst: dst, secret: []byte(secret)}
}

func (w *secretRedactingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending = append(w.pending, p...)
	if err := w.drain(false); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (w *secretRedactingWriter) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.drain(true)
}

func (w *secretRedactingWriter) drain(final bool) error {
	if len(w.secret) == 0 {
		_, err := w.dst.Write(w.pending)
		w.pending = w.pending[:0]
		return err
	}

	for {
		index := bytes.Index(w.pending, w.secret)
		if index < 0 {
			break
		}
		if _, err := w.dst.Write(w.pending[:index]); err != nil {
			return err
		}
		if _, err := w.dst.Write([]byte("[REDACTED]")); err != nil {
			return err
		}
		w.pending = w.pending[index+len(w.secret):]
	}

	writeLen := len(w.pending)
	if !final {
		writeLen -= len(w.secret) - 1
		if writeLen < 0 {
			writeLen = 0
		}
	}
	if writeLen > 0 {
		if _, err := w.dst.Write(w.pending[:writeLen]); err != nil {
			return err
		}
		w.pending = append(w.pending[:0], w.pending[writeLen:]...)
	}
	return nil
}
