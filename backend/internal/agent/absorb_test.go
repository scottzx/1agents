package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeUpstream lays out a fake submodule SKILL.md under modules/<source>/<rel>.
func writeUpstream(t *testing.T, modulesDir, source, rel, content string) {
	t.Helper()
	p := filepath.Join(modulesDir, source, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func testCfg(t *testing.T) AbsorbConfig {
	t.Helper()
	base := t.TempDir()
	return AbsorbConfig{
		ModulesDir: filepath.Join(base, "modules"),
		SkillsDir:  filepath.Join(base, "skills"),
		RolesDir:   filepath.Join(base, "roles"),
		LedgerPath: filepath.Join(base, ".absorbed.json"),
	}
}

func TestAbsorbSkillAddsProvenance(t *testing.T) {
	cfg := testCfg(t)
	writeUpstream(t, cfg.ModulesDir, "superpowers", "skills/brainstorming/SKILL.md",
		"---\nname: brainstorming\ndescription: turn ideas into designs\n---\n# Body\nhello\n")

	manifest := []AbsorbEntry{{Source: "superpowers", SrcPath: "skills/brainstorming/SKILL.md", Kind: AbsorbSkill}}
	results, err := Absorb(cfg, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Action != "written" {
		t.Fatalf("expected one written result, got %+v", results)
	}

	dst := filepath.Join(cfg.SkillsDir, "brainstorming", "SKILL.md")
	raw, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	for _, want := range []string{"name: brainstorming", "source: superpowers", "license: MIT", "# Body", "hello"} {
		if !strings.Contains(got, want) {
			t.Errorf("absorbed skill missing %q\n---\n%s", want, got)
		}
	}

	// The transformed output must re-parse as a valid skill.
	if _, err := parseSkillMarkdown(raw); err != nil {
		t.Fatalf("absorbed skill does not re-parse: %v", err)
	}
}

func TestAbsorbRoleSplitsFromGstack(t *testing.T) {
	cfg := testCfg(t)
	// gstack-style frontmatter: scalar fields our role parser tolerates plus
	// extra keys (triggers, allowed-tools) that must be ignored, not fail.
	writeUpstream(t, cfg.ModulesDir, "gstack", "cso/SKILL.md",
		"---\nname: cso\ndescription: Chief Security Officer mode\nallowed-tools:\n  - Bash\n  - Read\ntriggers:\n  - security audit\n---\n# Prompt body\naudit things\n")

	manifest := []AbsorbEntry{{Source: "gstack", SrcPath: "cso/SKILL.md", Kind: AbsorbRole}}
	results, err := Absorb(cfg, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Action != "written" || results[0].Kind != AbsorbRole {
		t.Fatalf("unexpected result %+v", results[0])
	}

	dst := filepath.Join(cfg.RolesDir, "cso.md")
	raw, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	for _, want := range []string{"name: cso", "engine: claude-code", "source: gstack", "license: MIT", "audit things"} {
		if !strings.Contains(got, want) {
			t.Errorf("absorbed role missing %q\n---\n%s", want, got)
		}
	}
	// Must re-parse as a role.
	if _, err := parseRoleMarkdown(raw); err != nil {
		t.Fatalf("absorbed role does not re-parse: %v", err)
	}
}

func TestAbsorbIncrementalSkipsUnchanged(t *testing.T) {
	cfg := testCfg(t)
	writeUpstream(t, cfg.ModulesDir, "superpowers", "skills/x/SKILL.md",
		"---\nname: x\ndescription: d\n---\nbody\n")
	manifest := []AbsorbEntry{{Source: "superpowers", SrcPath: "skills/x/SKILL.md", Kind: AbsorbSkill}}

	first, err := Absorb(cfg, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if first[0].Action != "written" {
		t.Fatalf("first run should write, got %q", first[0].Action)
	}

	// Second run with no upstream change → skipped.
	second, err := Absorb(cfg, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if second[0].Action != "skipped" {
		t.Fatalf("unchanged second run should skip, got %q", second[0].Action)
	}

	// Change upstream → written again.
	writeUpstream(t, cfg.ModulesDir, "superpowers", "skills/x/SKILL.md",
		"---\nname: x\ndescription: d\n---\nbody changed\n")
	third, err := Absorb(cfg, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if third[0].Action != "written" {
		t.Fatalf("changed upstream should rewrite, got %q", third[0].Action)
	}
}

func TestAbsorbLedgerRecordsSourceAndHash(t *testing.T) {
	cfg := testCfg(t)
	writeUpstream(t, cfg.ModulesDir, "superpowers", "skills/y/SKILL.md",
		"---\nname: y\ndescription: d\n---\nbody\n")
	if _, err := Absorb(cfg, []AbsorbEntry{{Source: "superpowers", SrcPath: "skills/y/SKILL.md", Kind: AbsorbSkill}}); err != nil {
		t.Fatal(err)
	}
	ledger, err := loadLedger(cfg.LedgerPath)
	if err != nil {
		t.Fatal(err)
	}
	rec, ok := ledger["y"]
	if !ok {
		t.Fatal("ledger missing entry y")
	}
	if rec.Source != "superpowers" || rec.ContentHash == "" || rec.Kind != "skill" {
		t.Fatalf("bad ledger record %+v", rec)
	}

	names, err := AbsorbedNames(cfg.LedgerPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "y" {
		t.Fatalf("unexpected absorbed names %v", names)
	}
}
