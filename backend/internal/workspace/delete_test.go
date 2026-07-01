package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chenhg5/cc-connect/config"
)

// TestRemoveCCProjectsByPath is the resurrection regression test: deleting a
// workspace must purge its cc-connect project by work_dir PATH, even when the
// project name has drifted (slug, legacy __<agent> suffix, or "_"). Otherwise
// the startup import loop re-creates the workspace on the next restart.
func TestRemoveCCProjectsByPath(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "projects", "demo")
	other := filepath.Join(dir, "projects", "keep")

	cfgToml := `
[bridge]
token = "keep"

[[projects]]
name = "demo__claudecode"
[projects.agent]
type = "claudecode"
[projects.agent.options]
work_dir = "` + target + `"
[[projects.platforms]]
type = "bridge"

[[projects]]
name = "_"
[projects.agent]
type = "claudecode"
[projects.agent.options]
work_dir = "` + target + `/"
[[projects.platforms]]
type = "bridge"

[[projects]]
name = "keep"
[projects.agent]
type = "claudecode"
[projects.agent.options]
work_dir = "` + other + `"
[[projects.platforms]]
type = "bridge"
`
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(cfgToml), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	orig := config.ConfigPath
	config.ConfigPath = path
	t.Cleanup(func() { config.ConfigPath = orig })

	// Both projects pointing at target (by path, incl. a trailing-slash variant
	// and a drifted name) must be removed; the unrelated project must survive.
	if removed := removeCCProjectsByPath(target); removed != 2 {
		t.Fatalf("removeCCProjectsByPath = %d, want 2", removed)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(cfg.Projects) != 1 || cfg.Projects[0].Name != "keep" {
		t.Fatalf("after remove, projects = %+v; want only \"keep\"", cfg.Projects)
	}
}
