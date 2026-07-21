package agent

import (
	"sync"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	// Each test gets its own ONEAGENTS_HOME, hence its own meta.db
	// (meta.OpenDefault caches per resolved path).
	t.Setenv("ONEAGENTS_HOME", t.TempDir())
	s, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return s
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
	all, err := s.ListByWorkspace("ws1", false)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("List returned %d records, want 1", len(all))
	}

	// List with no match
	none, err := s.ListByWorkspace("ws2", false)
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

func TestStoreListSortedByLastEventAt(t *testing.T) {
	s := newTestStore(t)
	// Three sessions; later Touch on "a" must promote it to the front
	// (sidebar sorts by last assistant-text activity, newest first).
	for _, id := range []string{"a", "b", "c"} {
		if err := s.Add(ChatSessionRecord{
			ID:          id,
			WorkspaceID: "ws",
			AgentType:   AgentTypeClaudecode,
			CcProject:   "p",
			CcSessionID: id,
			SessionKey:  "k:" + id,
		}); err != nil {
			t.Fatalf("Add %s: %v", id, err)
		}
	}
	all, err := s.ListByWorkspace("ws", false)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("got %d, want 3", len(all))
	}
	// Add seeds LastEventAt = CreatedAt; newest create wins initially.
	if all[0].CreatedAt.IsZero() || all[0].LastEventAt.IsZero() {
		t.Fatalf("CreatedAt/LastEventAt not set by Add: %+v", all[0])
	}

	if err := s.Touch("a"); err != nil {
		t.Fatalf("Touch a: %v", err)
	}
	all, _ = s.ListByWorkspace("ws", false)
	if all[0].ID != "a" {
		t.Fatalf("after Touch(a), want a first, got order: %s, %s, %s", all[0].ID, all[1].ID, all[2].ID)
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
	all, _ := s.ListByWorkspace("ws", false)
	if len(all) != n {
		t.Fatalf("got %d records after concurrent add, want %d", len(all), n)
	}
}
