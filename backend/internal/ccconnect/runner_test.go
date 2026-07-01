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

	"github.com/scottzx/1Agents/backend/internal/workspace"
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

// TestReconcileProjectsByPath is the #277 sync-by-path regression test: project
// names lose the __<agent> suffix, projects map to workspaces by path, and an
// existing channel-bound agent at the same path survives a resync.
func TestReconcileProjectsByPath(t *testing.T) {
	codex := config.AgentConfig{Type: "codex", Options: map[string]any{"work_dir": "/repos/app"}}

	existing := []config.ProjectConfig{
		// Legacy suffixed project at /repos/app already has a codex-bound channel.
		{
			Name:  "app__claudecode",
			Agent: config.AgentConfig{Type: "claudecode", Options: map[string]any{"work_dir": "/repos/app"}},
			Platforms: []config.PlatformConfig{
				{Type: "bridge"},
				{Type: "feishu", Agent: &codex},
			},
		},
		// Orphan: no workspace points here anymore → must be dropped.
		{
			Name:  "gone__claudecode",
			Agent: config.AgentConfig{Type: "claudecode", Options: map[string]any{"work_dir": "/repos/gone"}},
		},
		// Degenerate placeholder name (the "_" the default "对话" workspace used
		// to slug to): must be repaired to the canonical id-derived name.
		{
			Name:      "_",
			Agent:     config.AgentConfig{Type: "claudecode", Options: map[string]any{"work_dir": "/repos/default"}},
			Platforms: []config.PlatformConfig{{Type: "bridge"}},
		},
		// Placeholder with no work_dir → must be preserved.
		{
			Name:      "temp",
			Agent:     config.AgentConfig{Type: "claudecode"},
			Platforms: []config.PlatformConfig{{Type: "bridge"}},
		},
	}
	workspaces := []workspace.Workspace{
		{ID: "app", Name: "app", Path: "/repos/app"},
		{ID: "newone", Name: "New One", Path: "/repos/new"},
		// Non-ASCII name (like the built-in default "对话"): slug is "ws" → falls
		// back to the id, and its existing "_" project must be renamed to it.
		{ID: "default", Name: "对话", Path: "/repos/default"},
	}

	out := reconcileProjectsByPath(existing, workspaces)

	byName := make(map[string]config.ProjectConfig)
	for _, p := range out {
		byName[p.Name] = p
	}

	// Existing project at /repos/app is matched by PATH; its codex channel
	// binding (and project name) survive the resync. Its legacy name is kept
	// as-is on purpose: renaming would orphan session/state files (the one-time
	// rename of legacy __<agent> projects is the Phase 4 migrator's job).
	app, ok := byName["app__claudecode"]
	if !ok {
		t.Fatalf("existing project at /repos/app was not preserved by path; got %v", projectNames(out))
	}
	foundCodex := false
	for _, pl := range app.Platforms {
		if pl.Type == "feishu" && pl.Agent != nil && pl.Agent.Type == "codex" {
			foundCodex = true
		}
	}
	if !foundCodex {
		t.Errorf("codex channel binding lost on resync: %+v", app.Platforms)
	}

	// New workspace → fresh project, name = sanitized workspace name (no suffix).
	if _, ok := byName["New_One"]; !ok {
		t.Errorf("new workspace did not produce project %q; got %v", "New_One", projectNames(out))
	}

	// Orphan dropped, placeholder preserved.
	if _, ok := byName["gone__claudecode"]; ok {
		t.Error("orphan project for removed workspace was not dropped")
	}
	if _, ok := byName["temp"]; !ok {
		t.Error("placeholder project (no work_dir) was not preserved")
	}

	// Degenerate "_" name repaired to the canonical id-derived name ("default"),
	// and the old "_" name is gone.
	if _, ok := byName["_"]; ok {
		t.Errorf("degenerate placeholder name %q was not repaired; got %v", "_", projectNames(out))
	}
	if _, ok := byName["default"]; !ok {
		t.Errorf("placeholder project was not renamed to %q; got %v", "default", projectNames(out))
	}
}

// TestCCProjectSlugNonASCIINoUnderscoreOnly guards the empty-workspace-id
// regression: a non-ASCII workspace name (like the built-in "对话" default)
// must not slug to a bare "_" (which re-imports as an empty workspace id and
// makes /api/agent/sessions?workspace_id= 400). It falls back to "ws" so the
// caller can substitute the workspace id instead.
func TestCCProjectSlugNonASCIINoUnderscoreOnly(t *testing.T) {
	cases := map[string]string{
		"对话":       "ws",
		"！！！":      "ws",
		"app":      "app",
		"New One":  "New_One",
		"混合mix":    "_mix",
		"a_b-c":    "a_b-c",
	}
	for in, want := range cases {
		if got := CCProjectSlug(in); got != want {
			t.Errorf("CCProjectSlug(%q) = %q; want %q", in, got, want)
		}
	}
	// sanitizeID must never resurrect an empty id from the slug fallback.
	if got := sanitizeID(CCProjectSlug("对话")); got == "" {
		t.Errorf("sanitizeID(CCProjectSlug(%q)) = %q; want non-empty", "对话", got)
	}
}

func projectNames(ps []config.ProjectConfig) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Name
	}
	return out
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
