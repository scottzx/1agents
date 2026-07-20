package supervisor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/config"
)

func writeSkillsSource(t *testing.T, root string) string {
	t.Helper()
	dir := filepath.Join(root, "skills-src")
	if err := os.MkdirAll(filepath.Join(dir, "skill_manager"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("fastapi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestIsSkillsSource(t *testing.T) {
	tmp := t.TempDir()
	if isSkillsSource(tmp) {
		t.Fatal("empty dir should not be skills source")
	}
	src := writeSkillsSource(t, tmp)
	if !isSkillsSource(src) {
		t.Fatalf("expected valid source at %s", src)
	}
}

func TestResolveSkillsDirPrefersConfig(t *testing.T) {
	tmp := t.TempDir()
	src := writeSkillsSource(t, tmp)
	// decoy that auto-discovery would also find
	decoyRoot := filepath.Join(tmp, "cwd")
	if err := os.MkdirAll(filepath.Join(decoyRoot, "modules", "1skills", "skill_manager"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(decoyRoot, "modules", "1skills", "requirements.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.SkillsSourceDir = src
	s := NewSkills(cfg)

	got := s.resolveSkillsDir(decoyRoot)
	abs, _ := filepath.Abs(src)
	if got != abs && got != src {
		t.Fatalf("resolveSkillsDir = %q, want config path %q", got, abs)
	}
}

func TestResolveSkillsDirRejectsInvalidConfig(t *testing.T) {
	tmp := t.TempDir()
	// valid source only under modules/1skills for discovery fallback
	src := filepath.Join(tmp, "modules", "1skills")
	if err := os.MkdirAll(filepath.Join(src, "skill_manager"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "requirements.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.SkillsSourceDir = filepath.Join(tmp, "not-a-skills-pkg")
	s := NewSkills(cfg)

	got := s.resolveSkillsDir(tmp)
	if got != src {
		// may be absolute
		abs, _ := filepath.Abs(src)
		if got != abs {
			t.Fatalf("resolveSkillsDir = %q, want discovered %q", got, abs)
		}
	}
}

func TestResolveSkillsDirEnvOverride(t *testing.T) {
	tmp := t.TempDir()
	src := writeSkillsSource(t, tmp)
	t.Setenv("ONEAGENTS_SKILLS_DIR", src)

	s := NewSkills(config.Default())
	got := s.resolveSkillsDir(tmp)
	abs, _ := filepath.Abs(src)
	if got != abs && got != src {
		t.Fatalf("resolveSkillsDir = %q, want env path %q", got, abs)
	}
}

func TestFindSkillsSourceNpmLayout(t *testing.T) {
	tmp := t.TempDir()
	// Simulate .../node_modules/@1agents/skills under a search root
	pkg := filepath.Join(tmp, "node_modules", "@1agents", "skills")
	if err := os.MkdirAll(filepath.Join(pkg, "skill_manager"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "requirements.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := findSkillsSource(tmp)
	abs, _ := filepath.Abs(pkg)
	if got != abs && got != pkg {
		t.Fatalf("findSkillsSource = %q, want %q", got, abs)
	}
}
