package ccconnect

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenhg5/cc-connect/config"
	"github.com/chenhg5/cc-connect/core"
)

// seedChannelConfig writes a config with one project that has a claudecode
// default agent and three channels: two inheriting the default, one already
// bound to codex.
func seedChannelConfig(t *testing.T) (configPath, workDir string) {
	t.Helper()
	dir := t.TempDir()
	workDir = filepath.Join(dir, "ws")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("mkdir workdir: %v", err)
	}
	codex := config.AgentConfig{Type: "codex", Options: map[string]any{"work_dir": workDir}}
	cfg := &config.Config{
		Projects: []config.ProjectConfig{{
			Name:  "myws",
			Agent: config.AgentConfig{Type: "claudecode", Options: map[string]any{"work_dir": workDir}},
			Platforms: []config.PlatformConfig{
				{Type: "bridge"},
				{Type: "telegram", Options: map[string]any{"token": "x"}},
				{Type: "feishu", Agent: &codex},
			},
		}},
	}
	configPath = filepath.Join(dir, "config.toml")
	if err := saveConfig(cfg, configPath); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	config.ConfigPath = configPath
	t.Cleanup(func() { config.ConfigPath = "" })
	return configPath, workDir
}

func TestGetProjectChannels(t *testing.T) {
	seedChannelConfig(t)

	pc, err := GetProjectChannels("myws")
	if err != nil {
		t.Fatalf("GetProjectChannels: %v", err)
	}
	if pc.DefaultAgent != "claudecode" {
		t.Errorf("DefaultAgent = %q, want claudecode", pc.DefaultAgent)
	}
	if len(pc.Channels) != 3 {
		t.Fatalf("got %d channels, want 3", len(pc.Channels))
	}
	// bridge + telegram inherit claudecode; feishu is bound to codex.
	if !pc.Channels[0].Inherited || pc.Channels[0].Agent != "claudecode" {
		t.Errorf("bridge channel = %+v, want inherited claudecode", pc.Channels[0])
	}
	if !pc.Channels[1].Inherited || pc.Channels[1].Agent != "claudecode" {
		t.Errorf("telegram channel = %+v, want inherited claudecode", pc.Channels[1])
	}
	if pc.Channels[2].Inherited || pc.Channels[2].Agent != "codex" {
		t.Errorf("feishu channel = %+v, want bound codex", pc.Channels[2])
	}
	if pc.Channels[2].Type != "feishu" {
		t.Errorf("channel 2 type = %q, want feishu", pc.Channels[2].Type)
	}
}

func TestSetChannelAgentBinding(t *testing.T) {
	// Drain any pending restart signal the write triggers so the buffered
	// channel never blocks across subtests.
	drain := func() {
		select {
		case <-core.RestartCh:
		default:
		}
	}

	configPath, workDir := seedChannelConfig(t)

	// Bind the telegram channel (index 1) to codex.
	if err := SetChannelAgentBinding("myws", 1, "codex"); err != nil {
		t.Fatalf("bind codex: %v", err)
	}
	drain()

	pc, err := GetProjectChannels("myws")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if pc.Channels[1].Inherited || pc.Channels[1].Agent != "codex" {
		t.Errorf("telegram after bind = %+v, want bound codex", pc.Channels[1])
	}
	// The channel agent should inherit the project work_dir.
	if pc.Channels[1].WorkDir != workDir {
		t.Errorf("telegram channel work_dir = %q, want %q", pc.Channels[1].WorkDir, workDir)
	}

	// Persisted to disk as a [projects.platforms.agent] block.
	raw, _ := os.ReadFile(configPath)
	if !strings.Contains(string(raw), "codex") {
		t.Errorf("config not persisted with codex binding: %s", raw)
	}

	// Clearing the binding (empty agent) re-inherits the default.
	if err := SetChannelAgentBinding("myws", 1, ""); err != nil {
		t.Fatalf("clear binding: %v", err)
	}
	drain()
	pc, _ = GetProjectChannels("myws")
	if !pc.Channels[1].Inherited || pc.Channels[1].Agent != "claudecode" {
		t.Errorf("telegram after clear = %+v, want inherited claudecode", pc.Channels[1])
	}

	// Setting to the project default also clears the override.
	if err := SetChannelAgentBinding("myws", 0, "claudecode"); err != nil {
		t.Fatalf("set to default: %v", err)
	}
	drain()
	pc, _ = GetProjectChannels("myws")
	if !pc.Channels[0].Inherited {
		t.Errorf("bridge after set-to-default = %+v, want inherited", pc.Channels[0])
	}

	// Out-of-range index is an error.
	if err := SetChannelAgentBinding("myws", 99, "codex"); err == nil {
		t.Error("expected error for out-of-range channel index")
	}
	// Unknown project is an error.
	if err := SetChannelAgentBinding("nope", 0, "codex"); err == nil {
		t.Error("expected error for unknown project")
	}
}

func TestChannelsHandler(t *testing.T) {
	seedChannelConfig(t)
	drain := func() {
		select {
		case <-core.RestartCh:
		default:
		}
	}

	// GET without project → 400.
	rec := httptest.NewRecorder()
	ChannelsHandler(rec, httptest.NewRequest(http.MethodGet, "/api/cc-connect/channels", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("GET no project = %d, want 400", rec.Code)
	}

	// GET with project → 200 + channels.
	rec = httptest.NewRecorder()
	ChannelsHandler(rec, httptest.NewRequest(http.MethodGet, "/api/cc-connect/channels?project=myws", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var got ProjectChannels
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode GET: %v", err)
	}
	if len(got.Channels) != 3 {
		t.Errorf("GET returned %d channels, want 3", len(got.Channels))
	}

	// POST bind → 200, telegram now codex.
	rec = httptest.NewRecorder()
	body := strings.NewReader(`{"project":"myws","index":1,"agent":"codex"}`)
	ChannelsHandler(rec, httptest.NewRequest(http.MethodPost, "/api/cc-connect/channels", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	drain()
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Channels[1].Agent != "codex" {
		t.Errorf("POST response telegram = %+v, want codex", got.Channels[1])
	}

	// POST missing index → 400.
	rec = httptest.NewRecorder()
	ChannelsHandler(rec, httptest.NewRequest(http.MethodPost, "/api/cc-connect/channels", strings.NewReader(`{"project":"myws"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST no index = %d, want 400", rec.Code)
	}

	// PUT → 405.
	rec = httptest.NewRecorder()
	ChannelsHandler(rec, httptest.NewRequest(http.MethodPut, "/api/cc-connect/channels", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT = %d, want 405", rec.Code)
	}
}
