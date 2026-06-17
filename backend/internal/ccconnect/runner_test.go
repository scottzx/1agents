package ccconnect

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chenhg5/cc-connect/config"
)

// TestRunEngineSkipsBadProjectAndServesAPI is the issue #24 regression test.
//
// A project whose agent cannot be created (the generic "acp" type without the
// required "command" option) must be skipped — the engine and the management
// API must stay up for the remaining projects, instead of the boot loop
// panicking with an index-out-of-range and taking the whole management port
// down (502 for every project). It also asserts Part B: GET /api/v1/agents
// returns the host-curated list, which never offers the brick-prone "acp".
func TestRunEngineSkipsBadProjectAndServesAPI(t *testing.T) {
	dataDir := t.TempDir()

	port, err := findFreePort(41820)
	if err != nil {
		t.Fatalf("findFreePort: %v", err)
	}

	enabled, disabled := true, false
	cfg := &config.Config{DataDir: dataDir}
	cfg.Projects = []config.ProjectConfig{
		// Good: constructs fine (no binary probe at New).
		{Name: "good", Agent: config.AgentConfig{Type: "claudecode", Options: map[string]any{"work_dir": dataDir}}},
		// Bad: acp.New fails ("command" is required) → must be skipped, not fatal.
		{Name: "bad_acp", Agent: config.AgentConfig{Type: "acp", Options: map[string]any{"work_dir": dataDir}}},
	}
	cfg.Management.Enabled = &enabled
	cfg.Management.Port = port
	cfg.Management.Token = "" // empty token disables auth for the test
	cfg.Bridge.Enabled = &disabled

	// add-platform persists through config.AddPlatformToProject, which reads and
	// writes the global config.ConfigPath file — seed it so the create-via-API
	// call below has a real file to mutate.
	configPath := filepath.Join(dataDir, "config.toml")
	if err := saveConfig(cfg, configPath); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	config.ConfigPath = configPath

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan bool, 1)
	go func() { done <- runEngine(ctx, cfg, configPath) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Log("runEngine did not return within 5s after cancel")
		}
	}()

	base := fmt.Sprintf("http://127.0.0.1:%d/api/v1", port)

	// Poll /status until the management server is listening. If the boot loop
	// panicked (the bug), mgmtSrv.Start() is never reached and this never
	// succeeds → the test fails, which is the regression signal.
	if !waitForOK(base+"/status", 8*time.Second) {
		t.Fatal("management API never became reachable — boot loop likely panicked (issue #24 regression)")
	}

	// Part A: the surviving "good" project is served; the bad one was skipped.
	projects := httpGet(t, base+"/projects")
	if !strings.Contains(projects, "good") {
		t.Errorf("/projects missing surviving project %q: %s", "good", projects)
	}
	if strings.Contains(projects, "bad_acp") {
		t.Errorf("/projects must not contain the skipped bad project: %s", projects)
	}

	// Part B: /agents is curated — the generic, command-requiring "acp" is gone.
	var agentsResp struct {
		Agents []string `json:"agents"`
	}
	if err := json.Unmarshal([]byte(httpGet(t, base+"/agents")), &agentsResp); err != nil {
		t.Fatalf("decode /agents: %v", err)
	}
	for _, a := range agentsResp.Agents {
		if a == "acp" || a == "tmux" {
			t.Errorf("/agents exposed un-creatable agent %q (should be curated out): %v", a, agentsResp.Agents)
		}
	}

	// Create-via-API path: POST add-platform for a brand-new acp project with no
	// "command". The API call itself must succeed (201) — it only writes config;
	// the engine no longer bricks on the bad project (Part A), so this is safe.
	body, _ := json.Marshal(map[string]any{
		"type":       "bridge",
		"agent_type": "acp",
		"work_dir":   dataDir,
	})
	resp, err := http.Post(base+"/projects/created__acp/add-platform", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST add-platform: %v", err)
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("add-platform returned %d (want 201): %s", resp.StatusCode, rb)
	}

	// The new project must be persisted to the config file.
	saved, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read back config: %v", err)
	}
	if !strings.Contains(string(saved), "created__acp") {
		t.Errorf("created project not persisted to config: %s", saved)
	}
}

func waitForOK(url string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func httpGet(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d", url, resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	return string(body)
}
