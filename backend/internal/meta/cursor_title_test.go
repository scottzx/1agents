package meta

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveCursorSessionTitle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	sid := "fc438acb-32cd-43ed-bc26-8803c3ec7d6f"
	dir := filepath.Join(home, ".cursor", "acp-sessions", sid)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "meta.json"),
		[]byte(`{"schemaVersion":1,"cwd":"/tmp/proj","title":"Check Cursor AutoTitle"}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	title, err := ResolveCursorSessionTitle(sid)
	if err != nil {
		t.Fatalf("ResolveCursorSessionTitle: %v", err)
	}
	if title != "Check Cursor AutoTitle" {
		t.Fatalf("title = %q, want Check Cursor AutoTitle", title)
	}

	title, err = ResolveCursorSessionTitle("missing-id")
	if err != nil {
		t.Fatalf("missing: %v", err)
	}
	if title != "" {
		t.Fatalf("missing title = %q, want empty", title)
	}
}

func TestResolveCursorSessionTitleEmptyInputs(t *testing.T) {
	title, err := ResolveCursorSessionTitle("")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if title != "" {
		t.Fatalf("empty session id should return empty title, got %q", title)
	}
}

func TestResolveCursorSessionTitleEmptyTitle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	sid := "no-title-yet"
	dir := filepath.Join(home, ".cursor", "acp-sessions", sid)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "meta.json"),
		[]byte(`{"schemaVersion":1,"cwd":"/tmp/proj"}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	title, err := ResolveCursorSessionTitle(sid)
	if err != nil {
		t.Fatalf("ResolveCursorSessionTitle: %v", err)
	}
	if title != "" {
		t.Fatalf("empty title field = %q, want empty", title)
	}
}
