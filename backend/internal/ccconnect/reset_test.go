package ccconnect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/chenhg5/cc-connect/config"
)

// TestPurgeWorkspaceProjects verifies the reset purge drops only the
// workspace-backed [[projects]] (those carrying a work_dir) while preserving
// provider/model config and projects without a work_dir.
func TestPurgeWorkspaceProjects(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	const initial = `language = "zh"

[bridge]
enabled = true
port = 8081
token = "bridge-keep"

[[providers]]
name = "modelscope"
base_url = "https://api.modelscope.cn/v1"
api_key = "keep-me"

[[projects]]
name = "myproj__claudecode"
[projects.agent]
type = "claudecode"
[projects.agent.options]
work_dir = "/Users/me/work/myproj"
mode = "default"
[[projects.platforms]]
type = "bridge"

[[projects]]
name = "agent-only-config"
[projects.agent]
type = "claudecode"
provider_refs = ["modelscope"]
[projects.agent.options]
mode = "default"
`
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Point config.ConfigPath at our temp file (PurgeWorkspaceProjects prefers it).
	orig := config.ConfigPath
	config.ConfigPath = path
	t.Cleanup(func() { config.ConfigPath = orig })

	removed, err := PurgeWorkspaceProjects()
	if err != nil {
		t.Fatalf("PurgeWorkspaceProjects: %v", err)
	}
	if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}

	var cfg config.Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		t.Fatalf("re-decode: %v", err)
	}

	// The workspace project must be gone; the no-work_dir project must survive.
	if len(cfg.Projects) != 1 {
		t.Fatalf("projects after purge = %d, want 1 (%+v)", len(cfg.Projects), cfg.Projects)
	}
	if cfg.Projects[0].Name != "agent-only-config" {
		t.Errorf("surviving project = %q, want agent-only-config", cfg.Projects[0].Name)
	}
	if wd, _ := cfg.Projects[0].Agent.Options["work_dir"].(string); wd != "" {
		t.Errorf("surviving project unexpectedly has work_dir %q", wd)
	}

	// Provider config + bridge config preserved.
	if len(cfg.Providers) != 1 || cfg.Providers[0].Name != "modelscope" {
		t.Errorf("providers not preserved: %+v", cfg.Providers)
	}
	if cfg.Bridge.Token != "bridge-keep" {
		t.Errorf("bridge token not preserved: %q", cfg.Bridge.Token)
	}
}

// TestPurgeWorkspaceProjectsMissingFile confirms a missing config is a no-op.
func TestPurgeWorkspaceProjectsMissingFile(t *testing.T) {
	orig := config.ConfigPath
	config.ConfigPath = filepath.Join(t.TempDir(), "nope.toml")
	t.Cleanup(func() { config.ConfigPath = orig })

	removed, err := PurgeWorkspaceProjects()
	if err != nil {
		t.Fatalf("expected nil err for missing file, got %v", err)
	}
	if removed != 0 {
		t.Errorf("removed = %d, want 0", removed)
	}
}
