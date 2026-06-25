package agent

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestImportSubagentScalarTools imports a Claude Code subagent whose tools field
// is a comma-separated string, and verifies it round-trips through the loader as
// an allow list with engine defaulted.
func TestImportSubagentScalarTools(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ONEAGENTS_HOME", home)

	cc := "---\n" +
		"name: code-reviewer\n" +
		"description: reviews code\n" +
		"tools: Read, Grep, Bash\n" +
		"---\n" +
		"You are a code reviewer."

	path, err := ImportSubagent([]byte(cc))
	if err != nil {
		t.Fatalf("ImportSubagent: %v", err)
	}
	if got := filepath.Join(home, ".1agents", "roles", "code-reviewer.md"); path != got {
		t.Errorf("path = %q, want %q", path, got)
	}

	// The imported file must reload via the loader in our schema.
	tpl, ok := LoadRoles("").Resolve("code-reviewer")
	if !ok {
		t.Fatal("imported role not resolved")
	}
	if tpl.Engine != "claude-code" {
		t.Errorf("Engine = %q, want claude-code (defaulted)", tpl.Engine)
	}
	if want := []string{"Read", "Grep", "Bash"}; !reflect.DeepEqual(tpl.Tools.Allow, want) {
		t.Errorf("Tools.Allow = %v, want %v", tpl.Tools.Allow, want)
	}
	if tpl.Prompt != "You are a code reviewer." {
		t.Errorf("Prompt = %q", tpl.Prompt)
	}
}

// TestImportSubagentStructuredTools: a file already in our schema imports
// unchanged.
func TestImportSubagentStructuredTools(t *testing.T) {
	t.Setenv("ONEAGENTS_HOME", t.TempDir())
	doc := "---\nname: planner\nengine: codex\ntools:\n  allow: [Read]\n  deny: [Bash]\n---\nplan things"
	if _, err := ImportSubagent([]byte(doc)); err != nil {
		t.Fatalf("ImportSubagent: %v", err)
	}
	tpl, _ := LoadRoles("").Resolve("planner")
	if tpl.Engine != "codex" {
		t.Errorf("Engine = %q, want codex", tpl.Engine)
	}
	if !reflect.DeepEqual(tpl.Tools.Allow, []string{"Read"}) || !reflect.DeepEqual(tpl.Tools.Deny, []string{"Bash"}) {
		t.Errorf("tools mismatch: %+v", tpl.Tools)
	}
}

// TestImportSkill imports a SKILL.md into the user skills dir and resolves it.
func TestImportSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ONEAGENTS_HOME", home)
	doc := "---\nname: brainstorm\ndescription: ideate\n---\nbrainstorm steps"
	path, err := ImportSkill([]byte(doc))
	if err != nil {
		t.Fatalf("ImportSkill: %v", err)
	}
	if got := filepath.Join(home, ".1agents", "skills", "brainstorm", "SKILL.md"); path != got {
		t.Errorf("path = %q, want %q", path, got)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("skill file missing: %v", err)
	}
	sk, ok := LoadSkills("").Resolve("brainstorm")
	if !ok || sk.Source != "user" {
		t.Fatalf("imported skill not resolved: ok=%v", ok)
	}
}

func TestImportRejectsInvalid(t *testing.T) {
	t.Setenv("ONEAGENTS_HOME", t.TempDir())
	if _, err := ImportSubagent([]byte("no frontmatter here")); err == nil {
		t.Error("expected error importing role without frontmatter")
	}
	if _, err := ImportSkill([]byte("---\ndescription: x\n---\nno name")); err == nil {
		t.Error("expected error importing skill without name")
	}
}
