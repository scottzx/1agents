package meta

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveGrokSessionTitle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cwd := "/tmp/my-project"
	sid := "019f-test-session-id"
	enc := url.PathEscape(cwd)
	dir := filepath.Join(home, ".grok", "sessions", enc, sid)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	summary := `{"generated_title":"Fix the login bug","session_summary":"Fix the login bug","info":{"id":"` + sid + `","cwd":"` + cwd + `"}}`
	if err := os.WriteFile(filepath.Join(dir, "summary.json"), []byte(summary), 0o644); err != nil {
		t.Fatalf("write summary: %v", err)
	}

	// Direct path via workspace cwd.
	title, err := ResolveGrokSessionTitle(cwd, sid)
	if err != nil {
		t.Fatalf("ResolveGrokSessionTitle: %v", err)
	}
	if title != "Fix the login bug" {
		t.Fatalf("title = %q, want %q", title, "Fix the login bug")
	}

	// Walk fallback when workspace path is empty / unknown.
	title, err = ResolveGrokSessionTitle("", sid)
	if err != nil {
		t.Fatalf("walk ResolveGrokSessionTitle: %v", err)
	}
	if title != "Fix the login bug" {
		t.Fatalf("walk title = %q, want %q", title, "Fix the login bug")
	}

	// Unknown session id → empty.
	title, err = ResolveGrokSessionTitle(cwd, "does-not-exist")
	if err != nil {
		t.Fatalf("missing session: %v", err)
	}
	if title != "" {
		t.Fatalf("missing session title = %q, want empty", title)
	}
}

func TestResolveGrokSessionTitleFallsBackToSessionSummary(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cwd := "/tmp/other"
	sid := "019f-summary-only"
	enc := url.PathEscape(cwd)
	dir := filepath.Join(home, ".grok", "sessions", enc, sid)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// generated_title missing; session_summary present.
	summary := `{"session_summary":"Kanban board refactor"}`
	if err := os.WriteFile(filepath.Join(dir, "summary.json"), []byte(summary), 0o644); err != nil {
		t.Fatalf("write summary: %v", err)
	}

	title, err := ResolveGrokSessionTitle(cwd, sid)
	if err != nil {
		t.Fatalf("ResolveGrokSessionTitle: %v", err)
	}
	if title != "Kanban board refactor" {
		t.Fatalf("title = %q, want session_summary", title)
	}
}

func TestResolveGrokSessionTitleEmptyInputs(t *testing.T) {
	title, err := ResolveGrokSessionTitle("/tmp", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if title != "" {
		t.Fatalf("empty session id should return empty title, got %q", title)
	}
}
