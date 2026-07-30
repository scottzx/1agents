package supervisor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/scottzx/1Agents/backend/internal/config"
)

func TestNewHarnessKitTokenIsRandomAndStrong(t *testing.T) {
	first, err := newHarnessKitToken()
	if err != nil {
		t.Fatal(err)
	}
	second, err := newHarnessKitToken()
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("generated tokens must differ")
	}
	if len(first) < 40 || len(second) < 40 {
		t.Fatalf("tokens are unexpectedly short: %d, %d", len(first), len(second))
	}
}

func TestSecretRedactingWriterHandlesSplitWrites(t *testing.T) {
	var destination bytes.Buffer
	writer := newSecretRedactingWriter(&destination, "split-secret")
	for _, chunk := range []string{"before spl", "it-sec", "ret after\nsecond split-secret"} {
		if _, err := writer.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Flush(); err != nil {
		t.Fatal(err)
	}
	got := destination.String()
	if strings.Contains(got, "split-secret") {
		t.Fatalf("secret leaked: %q", got)
	}
	if got != "before [REDACTED] after\nsecond [REDACTED]" {
		t.Fatalf("redacted output = %q", got)
	}
}

func TestAllocateHarnessKitPort(t *testing.T) {
	port, err := allocateHarnessKitPort("127.0.0.1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if port < 1 {
		t.Fatalf("port = %d", port)
	}

	if _, err := allocateHarnessKitPort("0.0.0.0", 0); err == nil {
		t.Fatal("non-loopback supervised host must be rejected")
	}
}

func TestHarnessKitSupervisorDegradesWhenBinaryMissing(t *testing.T) {
	cfg := config.Default()
	cfg.HarnessKitBinaryPath = filepath.Join(t.TempDir(), "missing-hk")
	cfg.MaxRestarts = 1
	sup := NewHarnessKit(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sup.Start(ctx)
	select {
	case <-sup.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("supervisor did not finish after discovery failure")
	}

	status := sup.Status()
	if status.State != "degraded" || status.Ready {
		t.Fatalf("status = %+v", status)
	}
	if status.LastError != "binary_not_found" || strings.Contains(status.LastError, cfg.HarnessKitBinaryPath) {
		t.Fatalf("public error is not safely classified: %q", status.LastError)
	}
	if _, _, ready := sup.Endpoint(); ready {
		t.Fatal("degraded supervisor exposed a ready endpoint")
	}
}

func TestHarnessKitSupervisorLifecycleReadinessAndTokenRotation(t *testing.T) {
	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, "crash-once"), []byte("1"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.HarnessKitBinaryPath = os.Args[0]
	cfg.HarnessKitDataDir = dataDir
	cfg.HarnessKitPort = 0
	cfg.RestartDelay = 10 * time.Millisecond
	cfg.MaxRestarts = 3

	sup := NewHarnessKit(cfg)
	sup.readinessTimeout = 5 * time.Second
	sup.commandFactory = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		helperArgs := append([]string{"-test.run=TestHarnessKitHelperProcess", "--"}, args...)
		return exec.CommandContext(ctx, os.Args[0], helperArgs...)
	}

	ctx, cancel := context.WithCancel(context.Background())
	sup.Start(ctx)
	waitForHarnessKitState(t, sup, "ready", 8*time.Second)

	endpoint, token, ready := sup.Endpoint()
	if !ready || endpoint == "" || token == "" {
		t.Fatalf("endpoint = %q token-empty=%v ready=%v", endpoint, token == "", ready)
	}
	if sup.Status().RestartCount != 1 {
		t.Fatalf("restartCount = %d, want 1 after crash-once", sup.Status().RestartCount)
	}

	resp, err := http.Get(endpoint + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated health status = %d", resp.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodGet, endpoint+"/api/health", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || string(body) != "ok" {
		t.Fatalf("authenticated health = %d %q", resp.StatusCode, string(body))
	}

	tokenLog, err := os.ReadFile(filepath.Join(dataDir, "helper-tokens"))
	if err != nil {
		t.Fatal(err)
	}
	tokens := strings.Fields(string(tokenLog))
	if len(tokens) != 2 {
		t.Fatalf("helper starts = %d, tokens = %q", len(tokens), string(tokenLog))
	}
	if tokens[0] == tokens[1] {
		t.Fatal("token was not rotated after child restart")
	}

	statusJSON, err := json.Marshal(sup.Status())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(statusJSON), token) || strings.Contains(string(statusJSON), endpoint) {
		t.Fatalf("public status leaked credentials: %s", statusJSON)
	}

	cancel()
	select {
	case <-sup.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("supervisor did not stop after cancellation")
	}
	if got := sup.Status().State; got != "stopped" {
		t.Fatalf("final state = %q", got)
	}
}

func waitForHarnessKitState(t *testing.T, sup *HarnessKitSupervisor, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if sup.Status().State == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("state = %+v, want %q", sup.Status(), want)
}

// TestHarnessKitHelperProcess is re-executed as a subprocess by the lifecycle
// test. Arguments after "--" intentionally mirror the real hk serve contract.
func TestHarnessKitHelperProcess(t *testing.T) {
	separator := -1
	for i, arg := range os.Args {
		if arg == "--" {
			separator = i
			break
		}
	}
	if separator < 0 {
		return
	}
	args := os.Args[separator+1:]
	if len(args) == 0 || args[0] != "serve" {
		os.Exit(2)
	}

	values := make(map[string]string)
	for i := 1; i+1 < len(args); i += 2 {
		values[args[i]] = args[i+1]
	}
	host := values["--host"]
	port, err := strconv.Atoi(values["--port"])
	token := os.Getenv("HARNESSKIT_TOKEN")
	if err != nil || host != "127.0.0.1" || port < 1 || token == "" ||
		values["--data-dir"] == "" || values["--token"] != "" {
		os.Exit(3)
	}

	tokenFile := filepath.Join(values["--data-dir"], "helper-tokens")
	f, err := os.OpenFile(tokenFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		os.Exit(4)
	}
	_, _ = fmt.Fprintln(f, token)
	_ = f.Close()

	crashOnce := filepath.Join(values["--data-dir"], "crash-once")
	if _, err := os.Stat(crashOnce); err == nil {
		_ = os.Remove(crashOnce)
		os.Exit(17)
	}

	server := &http.Server{
		Addr: host + ":" + strconv.Itoa(port),
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/health" {
				http.NotFound(w, r)
				return
			}
			if r.Header.Get("Authorization") != "Bearer "+token {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			_, _ = io.WriteString(w, "ok")
		}),
	}
	if err := server.ListenAndServe(); err != nil {
		os.Exit(5)
	}
}
