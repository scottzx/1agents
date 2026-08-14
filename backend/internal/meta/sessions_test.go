package meta

import (
	"path/filepath"
	"testing"
)

// TestSessionUserNamedFlag covers #94: UpdateName must flip user_named to 1 so
// the list/get endpoint stops overwriting the user's title with the AI title
// auto-resolution. The default for new sessions (created via Add) is 0, so the
// legacy "auto AI title" behavior is preserved for sessions the user has not
// touched.
func TestSessionUserNamedFlag(t *testing.T) {
	s := NewSessionStore(newTestDB(t))

	rec := ChatSessionRecord{
		ID:          "abc",
		WorkspaceID: "ws1",
		Name:        "新建会话",
		AgentType:   "claudecode",
	}
	if err := s.Add(rec); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// New sessions default to user_named = false (AI title still applies).
	got, ok, err := s.Get("abc")
	if err != nil || !ok {
		t.Fatalf("Get abc: ok=%v err=%v", ok, err)
	}
	if got.UserNamed {
		t.Fatalf("newly-added session should have user_named=false, got true")
	}

	// User renames the session — user_named flips to 1.
	if err := s.UpdateName("abc", "我的项目会话"); err != nil {
		t.Fatalf("UpdateName: %v", err)
	}
	got, _, _ = s.Get("abc")
	if got.Name != "我的项目会话" {
		t.Fatalf("UpdateName failed: name=%q", got.Name)
	}
	if !got.UserNamed {
		t.Fatalf("after UpdateName, user_named should be true")
	}

	// ClearUserNamed resets the flag (reserved for a future reset endpoint).
	if err := s.ClearUserNamed("abc"); err != nil {
		t.Fatalf("ClearUserNamed: %v", err)
	}
	got, _, _ = s.Get("abc")
	if got.UserNamed {
		t.Fatalf("after ClearUserNamed, user_named should be false")
	}
	if got.Name != "我的项目会话" {
		t.Fatalf("ClearUserNamed must not change name, got %q", got.Name)
	}
}

func TestApplyAutoTitleOnlyDefaultAndUnnamed(t *testing.T) {
	s := NewSessionStore(newTestDB(t))

	if err := s.Add(ChatSessionRecord{
		ID: "auto", WorkspaceID: "ws", AgentType: "cursor", Name: "新建会话",
	}); err != nil {
		t.Fatalf("Add auto: %v", err)
	}
	applied, err := s.ApplyAutoTitle("auto", "Check Cursor AutoTitle")
	if err != nil {
		t.Fatalf("ApplyAutoTitle: %v", err)
	}
	if !applied {
		t.Fatalf("expected auto title to apply")
	}
	got, _, _ := s.Get("auto")
	if got.Name != "Check Cursor AutoTitle" {
		t.Fatalf("name = %q, want Check Cursor AutoTitle", got.Name)
	}
	if got.UserNamed {
		t.Fatalf("ApplyAutoTitle must not set user_named")
	}

	// Second apply is a no-op: name is no longer a default placeholder.
	applied, err = s.ApplyAutoTitle("auto", "Later Refined Title")
	if err != nil {
		t.Fatalf("second ApplyAutoTitle: %v", err)
	}
	if applied {
		t.Fatalf("non-default name must not be overwritten")
	}
	got, _, _ = s.Get("auto")
	if got.Name != "Check Cursor AutoTitle" {
		t.Fatalf("name mutated after second apply: %q", got.Name)
	}

	if err := s.Add(ChatSessionRecord{
		ID: "user", WorkspaceID: "ws", AgentType: "cursor", Name: "新建会话",
	}); err != nil {
		t.Fatalf("Add user: %v", err)
	}
	if err := s.UpdateName("user", "我的项目会话"); err != nil {
		t.Fatalf("UpdateName: %v", err)
	}
	applied, err = s.ApplyAutoTitle("user", "AI should not win")
	if err != nil {
		t.Fatalf("ApplyAutoTitle user: %v", err)
	}
	if applied {
		t.Fatalf("user_named session must not take an auto title")
	}
	got, _, _ = s.Get("user")
	if got.Name != "我的项目会话" {
		t.Fatalf("user name overwritten: %q", got.Name)
	}
}

func TestApplyAutoTitleEmptyNoop(t *testing.T) {
	s := NewSessionStore(newTestDB(t))
	if err := s.Add(ChatSessionRecord{
		ID: "empty", WorkspaceID: "ws", AgentType: "cursor", Name: "新建会话",
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	applied, err := s.ApplyAutoTitle("empty", "   ")
	if err != nil || applied {
		t.Fatalf("empty title applied=%v err=%v", applied, err)
	}
	got, _, _ := s.Get("empty")
	if got.Name != "新建会话" {
		t.Fatalf("name = %q, want 新建会话", got.Name)
	}
}

// TestSessionListByWorkspacePropagatesUserNamed makes sure ListByWorkspace
// surfaces user_named on every row (it would otherwise be silently dropped if
// the scan failed to populate it).
func TestSessionListByWorkspacePropagatesUserNamed(t *testing.T) {
	s := NewSessionStore(newTestDB(t))

	if err := s.Add(ChatSessionRecord{
		ID: "a", WorkspaceID: "ws", AgentType: "claudecode", Name: "auto",
	}); err != nil {
		t.Fatalf("Add a: %v", err)
	}
	if err := s.Add(ChatSessionRecord{
		ID: "b", WorkspaceID: "ws", AgentType: "claudecode", Name: "user",
	}); err != nil {
		t.Fatalf("Add b: %v", err)
	}
	if err := s.UpdateName("b", "我的项目会话"); err != nil {
		t.Fatalf("UpdateName b: %v", err)
	}

	all, err := s.ListByWorkspace("ws", false)
	if err != nil {
		t.Fatalf("ListByWorkspace: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("got %d records, want 2", len(all))
	}
	var auto, user ChatSessionRecord
	for _, r := range all {
		switch r.ID {
		case "a":
			auto = r
		case "b":
			user = r
		}
	}
	if auto.UserNamed {
		t.Fatalf("session a should have user_named=false (auto title still applies)")
	}
	if !user.UserNamed {
		t.Fatalf("session b should have user_named=true after UpdateName")
	}
}

// TestSessionSchemaV21UserNamedColumn is the migration idempotency check for
// #94: opening a DB at the pre-v21 schema and reopening a DB already at v21
// must both leave user_version at the current schemaVersion and yield a
// working user_named column (no "duplicate column name" or "no such column"
// errors on the second open).
func TestSessionSchemaV21UserNamedColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.db")

	// First open: fresh DB, schemaVersion advances and the column lands.
	db1, err := Open(path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	s1 := NewSessionStore(db1)
	if err := s1.Add(ChatSessionRecord{
		ID: "x", WorkspaceID: "ws", AgentType: "claudecode", Name: "ok",
	}); err != nil {
		t.Fatalf("Add x: %v", err)
	}
	if err := s1.UpdateName("x", "我的项目会话"); err != nil {
		t.Fatalf("UpdateName x: %v", err)
	}
	db1.Close()

	// Second open: schema is already at the latest, ensureSessionsColumns must
	// be a no-op (no "duplicate column name" error).
	db2, err := Open(path)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer db2.Close()

	got, ok, err := NewSessionStore(db2).Get("x")
	if err != nil || !ok {
		t.Fatalf("Get x after reopen: ok=%v err=%v", ok, err)
	}
	if !got.UserNamed || got.Name != "我的项目会话" {
		t.Fatalf("user_named/name not preserved across reopen: %+v", got)
	}
}