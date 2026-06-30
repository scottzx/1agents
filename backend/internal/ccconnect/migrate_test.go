package ccconnect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chenhg5/cc-connect/config"
)

// findProject returns the project with the given name, or nil.
func findProject(projs []config.ProjectConfig, name string) *config.ProjectConfig {
	for i := range projs {
		if projs[i].Name == name {
			return &projs[i]
		}
	}
	return nil
}

// channelAgentType returns the effective agent type for a channel: its own
// binding if set, else the project default.
func channelAgentType(p *config.ProjectConfig, idx int) string {
	if p.Platforms[idx].Agent != nil {
		return p.Platforms[idx].Agent.Type
	}
	return p.Agent.Type
}

func TestMigrateLegacyAgentSuffixProjects_FoldSamePath(t *testing.T) {
	wd := "/tmp/projX"
	in := []config.ProjectConfig{
		{
			Name:  "X__claudecode",
			Agent: config.AgentConfig{Type: "claudecode", Options: map[string]any{"work_dir": wd}},
			Platforms: []config.PlatformConfig{
				{Type: "bridge"},
				{Type: "feishu", Options: map[string]any{"app_id": "a"}},
			},
		},
		{
			Name:  "X__codex",
			Agent: config.AgentConfig{Type: "codex", Options: map[string]any{"work_dir": wd}},
			Platforms: []config.PlatformConfig{
				{Type: "telegram", Options: map[string]any{"token": "t"}},
			},
		},
	}

	out, changed := MigrateLegacyAgentSuffixProjects(in)
	if !changed {
		t.Fatal("expected changed=true for legacy suffixed projects")
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 folded project, got %d: %+v", len(out), out)
	}

	p := findProject(out, "X")
	if p == nil {
		t.Fatalf("expected folded project named X, got names %v", projectNames(out))
	}
	if p.Agent.Type != "claudecode" {
		t.Errorf("default agent = %q, want claudecode (first member)", p.Agent.Type)
	}
	if len(p.Platforms) != 3 {
		t.Fatalf("expected 3 channels (2 from claudecode + 1 from codex), got %d", len(p.Platforms))
	}

	// Channels from X__claudecode inherit the default (no per-channel binding).
	if channelAgentType(p, 0) != "claudecode" || p.Platforms[0].Agent != nil {
		t.Errorf("channel 0 (bridge) = %+v, want inherited claudecode", p.Platforms[0])
	}
	if channelAgentType(p, 1) != "claudecode" || p.Platforms[1].Agent != nil {
		t.Errorf("channel 1 (feishu) = %+v, want inherited claudecode", p.Platforms[1])
	}
	// The codex channel must carry an explicit binding to codex + the work_dir.
	if p.Platforms[2].Type != "telegram" {
		t.Errorf("channel 2 type = %q, want telegram", p.Platforms[2].Type)
	}
	if p.Platforms[2].Agent == nil || p.Platforms[2].Agent.Type != "codex" {
		t.Fatalf("channel 2 = %+v, want bound codex", p.Platforms[2])
	}
	if got, _ := p.Platforms[2].Agent.Options["work_dir"].(string); got != wd {
		t.Errorf("codex channel work_dir = %q, want %q", got, wd)
	}
}

func TestMigrateLegacyAgentSuffixProjects_Idempotent(t *testing.T) {
	wd := "/tmp/projY"
	in := []config.ProjectConfig{
		{
			Name:  "Y__claudecode",
			Agent: config.AgentConfig{Type: "claudecode", Options: map[string]any{"work_dir": wd}},
			Platforms: []config.PlatformConfig{
				{Type: "bridge"},
			},
		},
		{
			Name:  "Y__codex",
			Agent: config.AgentConfig{Type: "codex", Options: map[string]any{"work_dir": wd}},
			Platforms: []config.PlatformConfig{
				{Type: "feishu", Options: map[string]any{"app_id": "a"}},
			},
		},
	}

	out1, changed1 := MigrateLegacyAgentSuffixProjects(in)
	if !changed1 {
		t.Fatal("first run: expected changed=true")
	}

	out2, changed2 := MigrateLegacyAgentSuffixProjects(out1)
	if changed2 {
		t.Fatalf("second run: expected changed=false (idempotent), got out=%+v", out2)
	}
	if len(out2) != len(out1) {
		t.Errorf("idempotent run altered project count: %d → %d", len(out1), len(out2))
	}
}

func TestMigrateLegacyAgentSuffixProjects_NoSuffixNoOp(t *testing.T) {
	in := []config.ProjectConfig{
		{
			Name:      "alpha",
			Agent:     config.AgentConfig{Type: "claudecode", Options: map[string]any{"work_dir": "/tmp/a"}},
			Platforms: []config.PlatformConfig{{Type: "bridge"}},
		},
		{
			Name:      "beta",
			Agent:     config.AgentConfig{Type: "codex", Options: map[string]any{"work_dir": "/tmp/b"}},
			Platforms: []config.PlatformConfig{{Type: "telegram"}},
		},
	}
	_, changed := MigrateLegacyAgentSuffixProjects(in)
	if changed {
		t.Error("expected changed=false for already-clean config")
	}
}

func TestMigrateLegacyAgentSuffixProjects_PreservesPlaceholder(t *testing.T) {
	wd := "/tmp/projZ"
	in := []config.ProjectConfig{
		{
			Name:      "Z__claudecode",
			Agent:     config.AgentConfig{Type: "claudecode", Options: map[string]any{"work_dir": wd}},
			Platforms: []config.PlatformConfig{{Type: "bridge"}},
		},
		{
			// Placeholder with no work_dir — must survive verbatim.
			Name:      "temp",
			Agent:     config.AgentConfig{Type: "claudecode", Options: map[string]any{}},
			Platforms: []config.PlatformConfig{{Type: "bridge"}},
		},
	}
	out, changed := MigrateLegacyAgentSuffixProjects(in)
	if !changed {
		t.Fatal("expected changed=true (Z has a suffix)")
	}
	if findProject(out, "Z") == nil {
		t.Errorf("expected de-suffixed Z, got %v", projectNames(out))
	}
	if findProject(out, "temp") == nil {
		t.Errorf("placeholder 'temp' must be preserved, got %v", projectNames(out))
	}
}

func TestMigrateConfigFile_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	wd := filepath.Join(dir, "ws")
	if err := os.MkdirAll(wd, 0o755); err != nil {
		t.Fatalf("mkdir workdir: %v", err)
	}
	cfg := &config.Config{
		Projects: []config.ProjectConfig{
			{
				Name:      "ws__claudecode",
				Agent:     config.AgentConfig{Type: "claudecode", Options: map[string]any{"work_dir": wd}},
				Platforms: []config.PlatformConfig{{Type: "bridge"}},
			},
			{
				Name:      "ws__codex",
				Agent:     config.AgentConfig{Type: "codex", Options: map[string]any{"work_dir": wd}},
				Platforms: []config.PlatformConfig{{Type: "feishu", Options: map[string]any{"app_id": "a"}}},
			},
		},
	}
	path := filepath.Join(dir, "config.toml")
	if err := saveConfig(cfg, path); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	changed, err := MigrateConfigFile(path)
	if err != nil {
		t.Fatalf("MigrateConfigFile: %v", err)
	}
	if !changed {
		t.Fatal("expected file to be changed")
	}

	// Reload from disk and assert the folded shape persisted.
	reloaded, err := loadConfigForChannels(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(reloaded.Projects) != 1 {
		t.Fatalf("expected 1 project on disk, got %d: %v", len(reloaded.Projects), projectNames(reloaded.Projects))
	}
	p := findProject(reloaded.Projects, "ws")
	if p == nil {
		t.Fatalf("expected folded project ws, got %v", projectNames(reloaded.Projects))
	}
	if len(p.Platforms) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(p.Platforms))
	}
	// Second run on the now-migrated file is a no-op.
	changed2, err := MigrateConfigFile(path)
	if err != nil {
		t.Fatalf("second MigrateConfigFile: %v", err)
	}
	if changed2 {
		t.Error("expected second run to be a no-op (idempotent)")
	}
}
