package workspace

import (
	"os"
	"path/filepath"
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
