package agent

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/scottzx/1Agents/backend/internal/provider"
)

// TestRealProfileThrough1ACP is an opt-in smoke test for the complete
// Go resolver -> 1ACP bridge -> grok-build path. It uses the real provider
// file read-only, but gives the bridge an isolated HOME and state directory.
//
//	ONEAGENTS_REAL_PROFILE_E2E=1 go test ./internal/agent -run TestRealProfileThrough1ACP -v
//
// ONEAGENTS_REAL_PROFILE_ID may select a profile other than deepseek-build.
func TestRealProfileThrough1ACP(t *testing.T) {
	if os.Getenv("ONEAGENTS_REAL_PROFILE_E2E") != "1" {
		t.Skip("set ONEAGENTS_REAL_PROFILE_E2E=1 to run the real Provider Profile E2E")
	}
	if _, err := exec.LookPath("grok"); err != nil {
		t.Fatal("grok runtime is not installed")
	}
	profileID := strings.TrimSpace(os.Getenv("ONEAGENTS_REAL_PROFILE_ID"))
	if profileID == "" {
		profileID = provider.DeepSeekBuildProfileID
	}
	launch, _, err := resolveProfile(provider.NewStore(""), profileID)
	if err != nil {
		t.Fatal(err)
	}
	secret := launch.TransientCredentials["xai.api_key"]
	if secret == "" {
		t.Fatal("resolved profile has no xai.api_key credential")
	}

	bridgeDir := find1ACPDir(t)
	tsx := filepath.Join(bridgeDir, "node_modules", ".bin", "tsx")
	if _, err := os.Stat(tsx); err != nil {
		t.Fatalf("1ACP tsx runtime is unavailable: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	isolatedHome := t.TempDir()
	cmd := exec.CommandContext(ctx, tsx, "bridge-server.js")
	cmd.Dir = bridgeDir
	cmd.Env = append(withoutProcessEnv(os.Environ(), "HOME", "USERPROFILE", "ACPX_PORT"),
		"HOME="+isolatedHome,
		"USERPROFILE="+isolatedHome,
		fmt.Sprintf("ACPX_PORT=%d", port),
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var bridgeOutput bytes.Buffer
	cmd.Stdout = &bridgeOutput
	cmd.Stderr = &bridgeOutput
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = stdin.Close()
		cancel()
		_ = cmd.Wait()
	})

	endpoint := url.URL{Scheme: "ws", Host: fmt.Sprintf("127.0.0.1:%d", port)}
	var conn *websocket.Conn
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		conn, _, err = websocket.DefaultDialer.Dial(endpoint.String(), nil)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("connect 1ACP bridge: %v\n%s", err, redactLiveOutput(bridgeOutput.String(), secret))
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Minute))
	sessionID := "profile-live-e2e"
	if err := conn.WriteJSON(WsMessage{
		Action:        "ensure_session",
		SessionID:     sessionID,
		WorkspacePath: t.TempDir(),
		AgentType:     launch.RuntimeID,
		Launch:        launch,
	}); err != nil {
		t.Fatal(err)
	}
	waitForBridgeEvent(t, conn, "session_ready", secret)
	if err := conn.WriteJSON(WsMessage{Action: "prompt", SessionID: sessionID, Text: "Reply with OK only."}); err != nil {
		t.Fatal(err)
	}
	waitForBridgeEvent(t, conn, "done", secret)

	stateDir := filepath.Join(isolatedHome, ".1agents", "acpx-state")
	if err := filepath.WalkDir(stateDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if bytes.Contains(data, []byte(secret)) {
			return fmt.Errorf("credential leaked to %s", path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func find1ACPDir(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for dir := cwd; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "modules", "1acp", "bridge-server.js")
		if _, err := os.Stat(candidate); err == nil {
			return filepath.Dir(candidate)
		}
		if filepath.Dir(dir) == dir {
			t.Fatal("modules/1acp/bridge-server.js not found")
		}
	}
}

func waitForBridgeEvent(t *testing.T, conn *websocket.Conn, wanted, secret string) {
	t.Helper()
	for {
		var message WsMessage
		if err := conn.ReadJSON(&message); err != nil {
			t.Fatalf("read %s: %v", wanted, err)
		}
		if message.Event == "error" {
			t.Fatalf("1ACP error before %s: %s", wanted, redactLiveOutput(message.Message, secret))
		}
		if message.Event == wanted {
			return
		}
	}
}

func withoutProcessEnv(values []string, keys ...string) []string {
	prefixes := make([]string, len(keys))
	for i, key := range keys {
		prefixes[i] = key + "="
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		keep := true
		for _, prefix := range prefixes {
			if strings.HasPrefix(value, prefix) {
				keep = false
				break
			}
		}
		if keep {
			out = append(out, value)
		}
	}
	return out
}

func redactLiveOutput(output, secret string) string {
	if secret != "" {
		output = strings.ReplaceAll(output, secret, "[REDACTED]")
	}
	if len(output) > 4000 {
		return output[len(output)-4000:]
	}
	return output
}
