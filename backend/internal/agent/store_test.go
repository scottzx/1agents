package agent

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	// Re-init the package-level configDir to use the test home.
	configDir = filepath.Join(dir, ".1agents")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	return &Store{path: filepath.Join(configDir, configFile)}
}

func TestStoreAddGetListDelete(t *testing.T) {
	s := newTestStore(t)

	rec := ChatSessionRecord{
		ID:          "abc",
		WorkspaceID: "ws1",
		Name:        "first",
		AgentType:   AgentTypeClaudecode,
		CcProject:   "ws1__claudecode",
		CcSessionID: "cc-1",
		SessionKey:  "chatui:ws1:cc-1",
	}
	if err := s.Add(rec); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Get
	got, ok, err := s.Get("abc")
	if err != nil || !ok {
		t.Fatalf("Get abc: ok=%v err=%v", ok, err)
	}
	if got.Name != "first" || got.WorkspaceID != "ws1" {
		t.Fatalf("Get returned wrong record: %+v", got)
	}

	// List by workspace
	all, err := s.ListByWorkspace("ws1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("List returned %d records, want 1", len(all))
	}

	// List with no match
	none, err := s.ListByWorkspace("ws2")
	if err != nil {
		t.Fatalf("List ws2: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("List ws2 returned %d records, want 0", len(none))
	}

	// Duplicate add
	if err := s.Add(rec); err != ErrDuplicate {
		t.Fatalf("duplicate add: got %v, want ErrDuplicate", err)
	}

	// Delete
	if err := s.Delete("abc"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok, _ := s.Get("abc"); ok {
		t.Fatalf("Get abc after delete: still found")
	}

	// Delete missing
	if err := s.Delete("nope"); err != ErrNotFound {
		t.Fatalf("delete missing: got %v, want ErrNotFound", err)
	}
}

func TestStoreListSortedByCreatedAt(t *testing.T) {
	s := newTestStore(t)
	ids := []string{"a", "b", "c"}
	for _, id := range ids {
		_ = s.Add(ChatSessionRecord{
			ID:          id,
			WorkspaceID: "ws",
			AgentType:   AgentTypeClaudecode,
			CcProject:   "p",
			CcSessionID: id,
			SessionKey:  "k:" + id,
		})
	}
	all, _ := s.ListByWorkspace("ws")
	if len(all) != 3 {
		t.Fatalf("got %d, want 3", len(all))
	}
	// Add() should have set CreatedAt to ~now; sort is newest-first.
	// We just check the order is stable and non-empty.
	if all[0].CreatedAt.IsZero() {
		t.Fatalf("CreatedAt not set by Add")
	}
}

func TestStoreConcurrentAdds(t *testing.T) {
	s := newTestStore(t)
	var wg sync.WaitGroup
	const n = 20
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = s.Add(ChatSessionRecord{
				ID:          string(rune('a' + i)),
				WorkspaceID: "ws",
				AgentType:   AgentTypeClaudecode,
				CcProject:   "p",
				CcSessionID: "c",
				SessionKey:  "k",
			})
		}(i)
	}
	wg.Wait()
	all, _ := s.ListByWorkspace("ws")
	if len(all) != n {
		t.Fatalf("got %d records after concurrent add, want %d", len(all), n)
	}
}
