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

func writeCursorMeta(t *testing.T, home, sid, title string) {
	t.Helper()
	dir := filepath.Join(home, ".cursor", "acp-sessions", sid)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir cursor: %v", err)
	}
	body, _ := json.Marshal(map[string]any{"schemaVersion": 1, "title": title})
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), body, 0o644); err != nil {
		t.Fatalf("write cursor meta: %v", err)
	}
}

func TestResolveAcpSessionTitleCursor(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	sid := "cursor-acp-id"
	writeCursorMeta(t, home, sid, "Check Cursor AutoTitle")

	got := resolveAcpSessionTitle("/tmp/proj", sid, "新建会话")
	if got != "Check Cursor AutoTitle" {
		t.Fatalf("resolveAcpSessionTitle = %q, want Cursor meta.json title", got)
	}
}

func TestResolveAcpSessionTitleClaudePreferredOverCursor(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cwd := "/Users/test/mixed-proj"
	sid := "shared-cursor-id"
	slug := getProjectSlug(cwd)

	claudeDir := filepath.Join(home, ".claude", "projects", slug)
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("mkdir claude: %v", err)
	}
	if err := os.WriteFile(filepath.Join(claudeDir, sid+".jsonl"),
		[]byte(`{"aiTitle":"Claude Wins"}`+"\n"), 0o644); err != nil {
		t.Fatalf("write jsonl: %v", err)
	}
	writeCursorMeta(t, home, sid, "Cursor Loses")

	got := resolveAcpSessionTitle(cwd, sid, "新建会话")
	if got != "Claude Wins" {
		t.Fatalf("got %q, want Claude preferred over Cursor", got)
	}
}

func TestApplyAcpAutoTitleDefaultUnnamed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	s := newTestStore(t)

	sid := "cursor-turn-1"
	writeCursorMeta(t, home, sid, "Promote Subpane State")
	if err := s.Add(ChatSessionRecord{
		ID: "chat-1", WorkspaceID: "ws", AgentType: "cursor",
		Name: "新建会话", AcpSessionID: sid,
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	applyAcpAutoTitle(s, "chat-1", "/tmp/proj")

	got, ok, err := s.Get("chat-1")
	if err != nil || !ok {
		t.Fatalf("Get: ok=%v err=%v", ok, err)
	}
	if got.Name != "Promote Subpane State" {
		t.Fatalf("name = %q, want Promote Subpane State", got.Name)
	}
	if got.UserNamed {
		t.Fatalf("ApplyAutoTitle must leave user_named=0")
	}

	writeCursorMeta(t, home, sid, "Later Refined Title")
	applyAcpAutoTitle(s, "chat-1", "/tmp/proj")
	got, _, _ = s.Get("chat-1")
	if got.Name != "Promote Subpane State" {
		t.Fatalf("second turn overwrote non-default name: %q", got.Name)
	}
}

func TestApplyAcpAutoTitleSkipsUserNamed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	s := newTestStore(t)

	sid := "cursor-user-1"
	writeCursorMeta(t, home, sid, "AI should not win")
	if err := s.Add(ChatSessionRecord{
		ID: "chat-user", WorkspaceID: "ws", AgentType: "cursor",
		Name: "新建会话", AcpSessionID: sid,
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := s.UpdateName("chat-user", "我手改的标题"); err != nil {
		t.Fatalf("UpdateName: %v", err)
	}

	applyAcpAutoTitle(s, "chat-user", "/tmp/proj")

	got, _, _ := s.Get("chat-user")
	if got.Name != "我手改的标题" {
		t.Fatalf("user name overwritten: %q", got.Name)
	}
	if !got.UserNamed {
		t.Fatalf("user_named should stay 1")
	}
}

func TestMaybeSurfaceAutoTitleCursor(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	s := newTestStore(t)

	sid := "cursor-list-1"
	writeCursorMeta(t, home, sid, "Check Cursor AutoTitle")
	rec := ChatSessionRecord{
		ID: "chat-list", WorkspaceID: "ws", AgentType: "cursor",
		Name: "新建会话", AcpSessionID: sid,
	}
	if err := s.Add(rec); err != nil {
		t.Fatalf("Add: %v", err)
	}

	maybeSurfaceAutoTitle(s, &rec, "/tmp/proj")
	if rec.Name != "Check Cursor AutoTitle" {
		t.Fatalf("list/get surface = %q, want Check Cursor AutoTitle", rec.Name)
	}
	if rec.UserNamed {
		t.Fatalf("surface must not flip user_named")
	}
}
