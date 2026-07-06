package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func writeAgentFile(t *testing.T, ws, name, content string) {
	t.Helper()
	dir := filepath.Join(ws, ".claude", "agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const reviewerAgent = `---
name: code-reviewer
description: Sharp and pragmatic reviewer
tools: Read, Grep
model: sonnet
---
You are a senior code reviewer. Be thorough but not pedantic.`

func TestResolveAgentPersona_PrimaryFromTeam(t *testing.T) {
	ws := t.TempDir()
	writeAgentFile(t, ws, "code-reviewer.md", reviewerAgent)
	if err := WriteTeam(ws, Team{Primary: "code-reviewer.md", Members: []string{"code-reviewer.md"}}); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveAgentPersona(ws, "")
	if err != nil {
		t.Fatal(err)
	}
	want := "You are a senior code reviewer. Be thorough but not pedantic."
	if got != want {
		t.Fatalf("persona body:\n got %q\nwant %q", got, want)
	}
}

func TestResolveAgentPersona_ExplicitRefOverridesPrimary(t *testing.T) {
	ws := t.TempDir()
	writeAgentFile(t, ws, "code-reviewer.md", reviewerAgent)
	writeAgentFile(t, ws, "architect.md", "---\nname: architect\n---\nYou design systems first.")
	if err := WriteTeam(ws, Team{Primary: "code-reviewer.md"}); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveAgentPersona(ws, "architect.md")
	if err != nil {
		t.Fatal(err)
	}
	if got != "You design systems first." {
		t.Fatalf("explicit ref should win: got %q", got)
	}
}

func TestResolveAgentPersona_EmptyPrimaryNoSoul_NoInjection(t *testing.T) {
	ws := t.TempDir()
	// team.json exists with an empty primary (project-level "no personality").
	if err := WriteTeam(ws, Team{Primary: "", Members: nil}); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveAgentPersona(ws, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("empty primary + no SOUL.md should inject nothing, got %q", got)
	}
}

func TestResolveAgentPersona_LegacySoulFallback(t *testing.T) {
	ws := t.TempDir()
	// No team.json at all; a pre-team-model assistant only has SOUL.md.
	if err := writeWorkspaceSoul(ws, "I am the legacy persona."); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveAgentPersona(ws, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "I am the legacy persona." {
		t.Fatalf("expected SOUL.md fallback, got %q", got)
	}
}

func TestResolveAgentPersona_NoFrontmatter(t *testing.T) {
	ws := t.TempDir()
	writeAgentFile(t, ws, "plain.md", "Just a body, no frontmatter.")
	if err := WriteTeam(ws, Team{Primary: "plain.md"}); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveAgentPersona(ws, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "Just a body, no frontmatter." {
		t.Fatalf("got %q", got)
	}
}

func TestReadTeam_MissingIsZero(t *testing.T) {
	ws := t.TempDir()
	tm, err := ReadTeam(ws)
	if err != nil {
		t.Fatal(err)
	}
	if tm.Primary != "" || len(tm.Members) != 0 {
		t.Fatalf("missing manifest should be zero Team, got %+v", tm)
	}
}
