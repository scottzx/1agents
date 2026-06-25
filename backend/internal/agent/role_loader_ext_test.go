package agent

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestRoleSkillBindingMissing: a role naming an unresolved skill loads but
// records it in MissingSkills (graceful degradation); a resolvable one doesn't.
func TestRoleSkillBindingMissing(t *testing.T) {
	t.Setenv("ONEAGENTS_HOME", t.TempDir())
	ws := t.TempDir()
	roleDir := filepath.Join(ws, ".1agents", "roles")
	if err := os.MkdirAll(roleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// deep-research resolves (builtin); ghost-skill does not.
	doc := "---\nname: researcher\nengine: claude-code\nskills: [deep-research, ghost-skill]\n---\nbody"
	if err := os.WriteFile(filepath.Join(roleDir, "researcher.md"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := LoadRoles(ws)
	tpl, ok := reg.Resolve("researcher")
	if !ok {
		t.Fatal("researcher role not resolved")
	}
	if want := []string{"ghost-skill"}; !reflect.DeepEqual(tpl.MissingSkills, want) {
		t.Errorf("MissingSkills = %v, want %v", tpl.MissingSkills, want)
	}
}

// TestProjectRolesDir: the canonical ".1agents/roles" project dir loads.
func TestProjectRolesDir(t *testing.T) {
	t.Setenv("ONEAGENTS_HOME", t.TempDir())
	ws := t.TempDir()
	roleDir := filepath.Join(ws, ".1agents", "roles")
	if err := os.MkdirAll(roleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	doc := "---\nname: pm\nengine: claude-code\n---\nproject pm override"
	if err := os.WriteFile(filepath.Join(roleDir, "pm.md"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := LoadRoles(ws)
	tpl, _ := reg.Resolve("pm")
	if tpl.Source != "project" || !strings.Contains(tpl.Prompt, "project pm override") {
		t.Errorf("project roles dir not applied: Source=%q Prompt=%q", tpl.Source, tpl.Prompt)
	}
}

// TestForkAndRestoreBuiltin: forking a builtin writes a user-level copy that
// shadows the builtin; restoring deletes it and falls back to the builtin.
func TestForkAndRestoreBuiltin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ONEAGENTS_HOME", home)

	// Before fork: pm is builtin.
	if tpl, _ := LoadRoles("").Resolve("pm"); tpl.Source != "builtin" {
		t.Fatalf("pre-fork pm Source = %q, want builtin", tpl.Source)
	}

	path, err := ForkRole("pm", "")
	if err != nil {
		t.Fatalf("ForkRole: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("fork file missing: %v", err)
	}

	// After fork: pm resolves to the user copy.
	if tpl, _ := LoadRoles("").Resolve("pm"); tpl.Source != "user" {
		t.Errorf("post-fork pm Source = %q, want user", tpl.Source)
	}

	// Restore deletes the fork → back to builtin.
	if err := RestoreRole("pm"); err != nil {
		t.Fatalf("RestoreRole: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("fork file should be gone, stat err = %v", err)
	}
	if tpl, _ := LoadRoles("").Resolve("pm"); tpl.Source != "builtin" {
		t.Errorf("post-restore pm Source = %q, want builtin", tpl.Source)
	}
}

// TestRestoreNoForkIsNoop: restoring when no fork exists is not an error.
func TestRestoreNoForkIsNoop(t *testing.T) {
	t.Setenv("ONEAGENTS_HOME", t.TempDir())
	if err := RestoreRole("pm"); err != nil {
		t.Errorf("RestoreRole no-fork: %v", err)
	}
}
