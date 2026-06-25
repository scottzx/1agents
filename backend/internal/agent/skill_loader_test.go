package agent

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadSkillsBuiltin: the embedded builtin skill resolves with no user/project
// dirs present.
func TestLoadSkillsBuiltin(t *testing.T) {
	t.Setenv("ONEAGENTS_HOME", t.TempDir()) // isolate from the real ~/.1agents
	reg := LoadSkills("")
	sk, ok := reg.Resolve("deep-research")
	if !ok {
		t.Fatal("builtin deep-research skill not resolved")
	}
	if sk.Source != "builtin" {
		t.Errorf("Source = %q, want builtin", sk.Source)
	}
	if sk.Body == "" {
		t.Error("skill body is empty")
	}
}

// TestSkillLayerOverride: a user-level skill of the same name overrides the
// builtin; deleting it falls back to the builtin.
func TestSkillLayerOverride(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ONEAGENTS_HOME", home)

	skillDir := filepath.Join(home, ".1agents", "skills", "deep-research")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	custom := "---\nname: deep-research\ndescription: custom\n---\noverridden body"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := LoadSkills("")
	sk, _ := reg.Resolve("deep-research")
	if sk.Source != "user" || sk.Body != "overridden body" {
		t.Fatalf("override failed: Source=%q Body=%q", sk.Source, sk.Body)
	}

	// Remove the user copy → falls back to builtin.
	os.RemoveAll(filepath.Join(home, ".1agents", "skills", "deep-research"))
	reg = LoadSkills("")
	sk, _ = reg.Resolve("deep-research")
	if sk.Source != "builtin" {
		t.Errorf("after delete, Source = %q, want builtin", sk.Source)
	}
}

// TestProjectSkillLayer: a project-level skill loads and overrides user/builtin.
func TestProjectSkillLayer(t *testing.T) {
	t.Setenv("ONEAGENTS_HOME", t.TempDir())
	ws := t.TempDir()
	skillDir := filepath.Join(ws, ".1agents", "skills", "proj-only")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	doc := "---\nname: proj-only\n---\nproject skill"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := LoadSkills(ws)
	sk, ok := reg.Resolve("proj-only")
	if !ok || sk.Source != "project" {
		t.Fatalf("project skill not resolved: ok=%v src=%q", ok, sk)
	}
}

func TestParseSkillMarkdownErrors(t *testing.T) {
	cases := map[string][]byte{
		"no frontmatter": []byte("just a body"),
		"missing name":   []byte("---\ndescription: x\n---\nbody"),
	}
	for name, raw := range cases {
		if _, err := parseSkillMarkdown(raw); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}
