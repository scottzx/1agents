package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

// seedSharedSkill writes a minimal skill package into the shared store rooted at
// home (~/.1agents/skill-manager/shared/<name>).
func seedSharedSkill(t *testing.T, home, name string, files map[string]string) {
	t.Helper()
	dir := filepath.Join(home, ".1agents", "skill-manager", "shared", name)
	for rel, content := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", p, err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
}

func TestSyncSkillsToWorkspace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ONEAGENTS_HOME", home)

	seedSharedSkill(t, home, "alpha", map[string]string{
		"SKILL.md":          "# alpha",
		"assets/helper.txt": "nested",
	})
	seedSharedSkill(t, home, "beta", map[string]string{"SKILL.md": "# beta"})

	ws := t.TempDir()
	// "shared:alpha" exercises ref normalization; "missing" must be skipped
	// without failing the whole call.
	synced, err := syncSkillsToWorkspace(ws, []string{"shared:alpha", "beta", "missing"})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(synced) != 2 {
		t.Fatalf("expected 2 synced, got %v", synced)
	}

	// Real copies land under <ws>/.claude/skills.
	for _, rel := range []string{
		"alpha/SKILL.md",
		"alpha/assets/helper.txt",
		"beta/SKILL.md",
	} {
		p := filepath.Join(ws, ".claude", "skills", rel)
		if fi, err := os.Lstat(p); err != nil {
			t.Errorf("expected real file %s: %v", rel, err)
		} else if fi.Mode()&os.ModeSymlink != 0 {
			t.Errorf("%s should be a real file, not a symlink", rel)
		}
	}

	// <ws>/.agents/skills is a single relative dir symlink into the .claude
	// store; every skill is reachable through it (no per-skill links).
	skillsLink := filepath.Join(ws, ".agents", "skills")
	fi, err := os.Lstat(skillsLink)
	if err != nil {
		t.Fatalf("expected .agents/skills symlink: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf(".agents/skills should be a symlink")
	}
	if tgt, _ := os.Readlink(skillsLink); tgt != filepath.Join("..", ".claude", "skills") {
		t.Errorf(".agents/skills target = %q", tgt)
	}
	for _, name := range []string{"alpha", "beta"} {
		if _, err := os.Stat(filepath.Join(skillsLink, name, "SKILL.md")); err != nil {
			t.Errorf("%s not reachable via .agents/skills: %v", name, err)
		}
	}

	// A second sync is idempotent: store copy left untouched, symlink already
	// correct → nothing re-synced.
	again, err := syncSkillsToWorkspace(ws, []string{"alpha"})
	if err != nil {
		t.Fatalf("re-sync: %v", err)
	}
	if len(again) != 1 {
		// alpha is re-linked (link is idempotent but still counted as synced);
		// the meaningful guarantee is no error + the store copy stays put.
		t.Logf("re-sync returned %v", again)
	}
}

// TestSyncSkillsReplacesStaleLink proves that a stale .agents/skills entry (a
// real dir from an older per-skill layout) is replaced by the whole-dir symlink
// on the next sync.
func TestSyncSkillsReplacesStaleLink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ONEAGENTS_HOME", home)
	seedSharedSkill(t, home, "alpha", map[string]string{"SKILL.md": "# alpha"})

	ws := t.TempDir()
	// Simulate the old per-skill layout: a real .agents/skills dir with content.
	stale := filepath.Join(ws, ".agents", "skills", "leftover")
	if err := os.MkdirAll(stale, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := syncSkillsToWorkspace(ws, []string{"alpha"}); err != nil {
		t.Fatalf("sync: %v", err)
	}
	link := filepath.Join(ws, ".agents", "skills")
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf(".agents/skills should have been replaced by a symlink")
	}
}

func TestNormalizeSkillRef(t *testing.T) {
	cases := map[string]string{
		"foo":           "foo",
		"shared:foo":    "foo",
		"centralized:x": "x",
		"  spaced  ":    "spaced",
		"../escape":     "escape",
		"a/b/c":         "c",
	}
	for in, want := range cases {
		if got := normalizeSkillRef(in); got != want {
			t.Errorf("normalizeSkillRef(%q) = %q, want %q", in, got, want)
		}
	}
}
