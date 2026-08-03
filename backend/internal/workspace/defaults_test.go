package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureProjectGuideFiles(t *testing.T) {
	dir := t.TempDir()

	// Pre-create CLAUDE.md with custom content; it must be left untouched.
	existing := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(existing, []byte("KEEP ME"), 0o644); err != nil {
		t.Fatal(err)
	}

	ensureProjectGuideFiles(dir)

	// AGENTS.md was missing -> created with the template.
	agents, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("AGENTS.md not created: %v", err)
	}
	if string(agents) != agentsGuideTemplate {
		t.Errorf("AGENTS.md content mismatch")
	}

	// CLAUDE.md existed -> untouched.
	claude, err := os.ReadFile(existing)
	if err != nil {
		t.Fatal(err)
	}
	if string(claude) != "KEEP ME" {
		t.Errorf("CLAUDE.md was overwritten, got %q", string(claude))
	}
}

func TestEnsureProjectGuideFilesScaffoldDirs(t *testing.T) {
	dir := t.TempDir()

	// Pre-create .agents; it must be left untouched (no .gitkeep injected).
	existing := filepath.Join(dir, ".agents")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatal(err)
	}

	ensureProjectGuideFiles(dir)

	// .claude was missing -> created with a .gitkeep so git can track it.
	keep, err := os.Stat(filepath.Join(dir, ".claude", ".gitkeep"))
	if err != nil {
		t.Fatalf(".claude/.gitkeep not created: %v", err)
	}
	if keep.Size() != 0 {
		t.Errorf(".gitkeep should be empty, got %d bytes", keep.Size())
	}

	// .agents existed -> untouched.
	if _, err := os.Stat(filepath.Join(existing, ".gitkeep")); !os.IsNotExist(err) {
		t.Errorf("pre-existing .agents should be left untouched")
	}
}

func TestGitInitProject(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	ensureProjectGuideFiles(dir)

	gitInitProject(dir)

	// Repository exists with exactly one commit whose subject is "init".
	out, err := exec.Command("git", "-C", dir, "log", "--format=%s").CombinedOutput()
	if err != nil {
		t.Fatalf("git log failed: %v: %s", err, out)
	}
	subjects := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(subjects) != 1 || subjects[0] != "init" {
		t.Fatalf("expected a single 'init' commit, got %q", out)
	}

	// The scaffold files are tracked by the init commit.
	out, err = exec.Command("git", "-C", dir, "ls-files").CombinedOutput()
	if err != nil {
		t.Fatalf("git ls-files failed: %v: %s", err, out)
	}
	tracked := string(out)
	for _, want := range []string{"CLAUDE.md", "AGENTS.md", ".claude/.gitkeep", ".agents/.gitkeep"} {
		if !strings.Contains(tracked, want) {
			t.Errorf("init commit does not track %s; ls-files:\n%s", want, tracked)
		}
	}

	// A second call must leave the existing repository alone (no new commit).
	gitInitProject(dir)
	out, err = exec.Command("git", "-C", dir, "rev-list", "--count", "HEAD").CombinedOutput()
	if err != nil {
		t.Fatalf("git rev-list failed: %v: %s", err, out)
	}
	if n := strings.TrimSpace(string(out)); n != "1" {
		t.Errorf("expected 1 commit after re-run, got %s", n)
	}
}
