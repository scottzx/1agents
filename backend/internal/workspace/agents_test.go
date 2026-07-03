package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

// seedSharedAgent writes a single <name>.md agent file into the shared store
// rooted at home (~/.1agents/skill-manager/agents/<name>.md).
func seedSharedAgent(t *testing.T, home, file, content string) {
	t.Helper()
	dir := filepath.Join(home, ".1agents", "skill-manager", "agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", file, err)
	}
}

func TestSyncAgentsToWorkspace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ONEAGENTS_HOME", home)

	seedSharedAgent(t, home, "alpha.md", "# alpha")
	seedSharedAgent(t, home, "beta.md", "# beta")

	ws := t.TempDir()
	// "shared:alpha.md" exercises ref normalization; "missing.md" must be skipped
	// without failing the whole call.
	synced, err := syncAgentsToWorkspace(ws, []string{"shared:alpha.md", "beta.md", "missing.md"})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(synced) != 2 {
		t.Fatalf("expected 2 synced, got %v", synced)
	}

	// Real copies land under <ws>/.claude/agents as single files.
	for _, file := range []string{"alpha.md", "beta.md"} {
		p := filepath.Join(ws, ".claude", "agents", file)
		if fi, err := os.Lstat(p); err != nil {
			t.Errorf("expected real file %s: %v", file, err)
		} else if fi.Mode()&os.ModeSymlink != 0 {
			t.Errorf("%s should be a real file, not a symlink", file)
		} else if fi.IsDir() {
			t.Errorf("%s should be a file, not a directory", file)
		}
	}

	// <ws>/.agents/agents is a single relative dir symlink into the .claude store.
	agentsLink := filepath.Join(ws, ".agents", "agents")
	fi, err := os.Lstat(agentsLink)
	if err != nil {
		t.Fatalf("expected .agents/agents symlink: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf(".agents/agents should be a symlink")
	}
	if tgt, _ := os.Readlink(agentsLink); tgt != filepath.Join("..", ".claude", "agents") {
		t.Errorf(".agents/agents target = %q", tgt)
	}
	for _, file := range []string{"alpha.md", "beta.md"} {
		if _, err := os.Stat(filepath.Join(agentsLink, file)); err != nil {
			t.Errorf("%s not reachable via .agents/agents: %v", file, err)
		}
	}

	// A second sync is idempotent: no error, store copy stays put.
	if _, err := syncAgentsToWorkspace(ws, []string{"alpha.md"}); err != nil {
		t.Fatalf("re-sync: %v", err)
	}
}

func TestWorkspaceAgentFile(t *testing.T) {
	ws := t.TempDir()
	dir := filepath.Join(ws, ".claude", "agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "alpha.md"), []byte("# alpha"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Scoped ref resolves to the same on-disk file.
	got, err := workspaceAgentFile(ws, "shared:alpha.md")
	if err != nil {
		t.Fatalf("workspaceAgentFile: %v", err)
	}
	if got != filepath.Join(dir, "alpha.md") {
		t.Errorf("got %q, want %q", got, filepath.Join(dir, "alpha.md"))
	}

	// Missing file and traversal refs are rejected.
	if _, err := workspaceAgentFile(ws, "missing.md"); err == nil {
		t.Error("expected error for missing agent file")
	}
	if _, err := workspaceAgentFile(ws, ".."); err == nil {
		t.Error("expected error for traversal ref")
	}
}

func TestNormalizeAgentRef(t *testing.T) {
	cases := map[string]string{
		"foo.md":           "foo.md",
		"shared:foo.md":    "foo.md",
		"centralized:x.md": "x.md",
		"  spaced.md  ":    "spaced.md",
		"../escape.md":     "escape.md",
		"a/b/c.md":         "c.md",
	}
	for in, want := range cases {
		if got := normalizeAgentRef(in); got != want {
			t.Errorf("normalizeAgentRef(%q) = %q, want %q", in, got, want)
		}
	}
}
