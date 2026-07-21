package agent

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAcpSessionTitleGrok(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cwd := "/Users/test/project"
	sid := "019f-grok-acp-id"
	enc := url.PathEscape(cwd)
	dir := filepath.Join(home, ".grok", "sessions", enc, sid)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body, _ := json.Marshal(map[string]any{
		"generated_title": "Implement session auto title",
		"session_summary": "Implement session auto title",
	})
	if err := os.WriteFile(filepath.Join(dir, "summary.json"), body, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := resolveAcpSessionTitle(cwd, sid, "新建会话")
	if got != "Implement session auto title" {
		t.Fatalf("resolveAcpSessionTitle = %q, want Grok generated_title", got)
	}

	// Default when neither Claude nor Grok has data.
	got = resolveAcpSessionTitle(cwd, "missing-id", "新建会话")
	if got != "新建会话" {
		t.Fatalf("missing title = %q, want default", got)
	}
}

func TestResolveAcpSessionTitleClaudePreferredOverGrok(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cwd := "/Users/test/claude-proj"
	sid := "shared-session-id"
	slug := getProjectSlug(cwd)

	// Claude jsonl with aiTitle.
	claudeDir := filepath.Join(home, ".claude", "projects", slug)
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("mkdir claude: %v", err)
	}
	jsonl := `{"type":"x"}` + "\n" + `{"aiTitle":"Claude AI Title","slug":"claude-slug"}` + "\n"
	if err := os.WriteFile(filepath.Join(claudeDir, sid+".jsonl"), []byte(jsonl), 0o644); err != nil {
		t.Fatalf("write jsonl: %v", err)
	}

	// Grok also has a title for the same id (should lose).
	enc := url.PathEscape(cwd)
	grokDir := filepath.Join(home, ".grok", "sessions", enc, sid)
	if err := os.MkdirAll(grokDir, 0o755); err != nil {
		t.Fatalf("mkdir grok: %v", err)
	}
	if err := os.WriteFile(filepath.Join(grokDir, "summary.json"),
		[]byte(`{"generated_title":"Grok Title"}`), 0o644); err != nil {
		t.Fatalf("write grok: %v", err)
	}

	got := resolveAcpSessionTitle(cwd, sid, "新建会话")
	if got != "Claude AI Title" {
		t.Fatalf("got %q, want Claude preferred over Grok", got)
	}
}
