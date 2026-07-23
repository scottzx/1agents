package meta

import (
	"errors"
	"testing"
)

func newTestInboxStore(t *testing.T) *InboxStore {
	return NewInboxStore(newTestDB(t))
}

func TestInboxCaptureDefaults(t *testing.T) {
	s := newTestInboxStore(t)
	item, err := s.Capture(InboxItem{Title: "ship it", Content: "an idea"})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if item.ID == "" {
		t.Fatal("expected generated id")
	}
	if item.Source != InboxSourceManual {
		// empty source normalizes to misc; manual must be explicit
		if item.Source != InboxSourceMisc {
			t.Fatalf("unexpected source %q", item.Source)
		}
	}
	if item.Status != InboxStatusUnread {
		t.Fatalf("status = %q, want unread", item.Status)
	}
	if item.CreatedAt.IsZero() || item.UpdatedAt.IsZero() {
		t.Fatal("timestamps not set")
	}
}

func TestInboxSourceNormalization(t *testing.T) {
	s := newTestInboxStore(t)
	cases := map[string]string{
		"manual":      InboxSourceManual,
		"agent":       InboxSourceAgent,
		"function":    InboxSourceFunction,
		"data_source": InboxSourceDataSource,
		"im":          InboxSourceIM,
		"rss":         InboxSourceRSS,
		"unknown":     InboxSourceMisc,
	}
	for in, want := range cases {
		got, err := s.Capture(InboxItem{Source: in, Title: "x"})
		if err != nil {
			t.Fatalf("capture %q: %v", in, err)
		}
		if got.Source != want {
			t.Errorf("source %q normalized to %q, want %q", in, got.Source, want)
		}
	}
	// Capture treats empty source as manual; Deliver normalizes empty to misc.
	cap, err := s.Capture(InboxItem{Title: "manual default source"})
	if err != nil {
		t.Fatalf("capture empty source: %v", err)
	}
	if cap.Source != InboxSourceManual {
		t.Errorf("Capture empty source = %q, want manual", cap.Source)
	}
	del, err := s.Deliver(InboxItem{WorkspaceID: "ws", Title: "deliver empty source", Source: ""})
	if err != nil {
		t.Fatalf("deliver empty source: %v", err)
	}
	if del.Source != InboxSourceMisc {
		t.Errorf("Deliver empty source = %q, want misc", del.Source)
	}
}

// TestInboxDeliverListByWorkspaceIsolation: A deliver → B list sees only that
// mail; A list does not (acceptance criterion #204).
func TestInboxDeliverListByWorkspaceIsolation(t *testing.T) {
	s := newTestInboxStore(t)
	delivered, err := s.Deliver(InboxItem{
		WorkspaceID:     "ws-b",
		Source:          InboxSourceAgent,
		FromWorkspaceID: "ws-a",
		FromRef:         "collector",
		Title:           "handoff for B",
		Content:         "please triage",
		Payload:         []byte(`{"score":1}`),
	})
	if err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if delivered.WorkspaceID != "ws-b" || delivered.FromWorkspaceID != "ws-a" {
		t.Fatalf("envelope fields: workspace=%q from=%q", delivered.WorkspaceID, delivered.FromWorkspaceID)
	}
	if string(delivered.Payload) != `{"score":1}` {
		t.Fatalf("payload = %s", delivered.Payload)
	}

	// Noise in A must not leak into B's list.
	if _, err := s.Deliver(InboxItem{
		WorkspaceID: "ws-a",
		Source:      InboxSourceFunction,
		Title:       "only for A",
	}); err != nil {
		t.Fatalf("deliver to A: %v", err)
	}

	listB, err := s.ListByWorkspace("ws-b", false)
	if err != nil {
		t.Fatalf("list B: %v", err)
	}
	if len(listB) != 1 || listB[0].ID != delivered.ID {
		t.Fatalf("B list = %+v, want only delivered id %s", listB, delivered.ID)
	}

	listA, err := s.ListByWorkspace("ws-a", false)
	if err != nil {
		t.Fatalf("list A: %v", err)
	}
	if len(listA) != 1 || listA[0].Title != "only for A" {
		t.Fatalf("A list = %+v, want only A's item", listA)
	}
	for _, it := range listA {
		if it.ID == delivered.ID {
			t.Fatal("A must not see B's delivered item")
		}
	}

	unreadB, err := s.UnreadCountByWorkspace("ws-b")
	if err != nil || unreadB != 1 {
		t.Fatalf("unread B = %d err=%v, want 1", unreadB, err)
	}
}

func TestInboxDeliverRequiresWorkspace(t *testing.T) {
	s := newTestInboxStore(t)
	if _, err := s.Deliver(InboxItem{Title: "no workspace"}); err == nil {
		t.Fatal("Deliver without workspaceId should fail")
	}
}

// TestInboxLegacyBackfillWorkspaceID: rows written without workspace_id (or
// empty after column add) are backfilled to the builtin default assistant on
// Open / ensureInboxItemsColumns.
func TestInboxLegacyBackfillWorkspaceID(t *testing.T) {
	db := newTestDB(t)
	// Simulate a pre-#202 row: blank workspace_id (column exists with DEFAULT '').
	id := "legacy-inbox-1"
	if _, err := db.sql.Exec(`
		INSERT INTO inbox_items (id, workspace_id, source, title, content, url, summary, tags, status, created_at, updated_at)
		VALUES (?, '', 'manual', 'old global item', '', '', '', '[]', 'unread', ?, ?)`,
		id, "2020-01-01T00:00:00Z", "2020-01-01T00:00:00Z"); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}
	if err := db.ensureInboxItemsColumns(); err != nil {
		t.Fatalf("ensure/backfill: %v", err)
	}
	s := NewInboxStore(db)
	got, ok, err := s.Get(id)
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if got.WorkspaceID != DefaultInboxWorkspaceID {
		t.Fatalf("workspace_id = %q, want %q after backfill", got.WorkspaceID, DefaultInboxWorkspaceID)
	}
}

func TestInboxCaptureDefaultsWorkspace(t *testing.T) {
	s := newTestInboxStore(t)
	item, err := s.Capture(InboxItem{Title: "no ws"})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if item.WorkspaceID != DefaultInboxWorkspaceID {
		t.Fatalf("workspace_id = %q, want default", item.WorkspaceID)
	}
}

func TestInboxListExcludesArchivedAndCountsUnread(t *testing.T) {
	s := newTestInboxStore(t)
	a, _ := s.Capture(InboxItem{Source: "manual", Title: "first"})
	_, _ = s.Capture(InboxItem{Source: "im", Title: "second"})

	unread, err := s.UnreadCount()
	if err != nil {
		t.Fatalf("unread count: %v", err)
	}
	if unread != 2 {
		t.Fatalf("unread = %d, want 2", unread)
	}

	if _, err := s.SetStatus(a.ID, InboxStatusArchived); err != nil {
		t.Fatalf("archive: %v", err)
	}

	active, err := s.List(false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("active list = %d, want 1 (archived hidden)", len(active))
	}

	all, err := s.List(true)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("list-all = %d, want 2 (archived retained)", len(all))
	}

	unread, _ = s.UnreadCount()
	if unread != 1 {
		t.Fatalf("unread after archive = %d, want 1", unread)
	}
}

func TestInboxListNewestFirst(t *testing.T) {
	s := newTestInboxStore(t)
	first, _ := s.Capture(InboxItem{Source: "manual", Title: "old"})
	// Force a later timestamp on the second row.
	second, _ := s.Capture(InboxItem{Source: "manual", Title: "new"})
	if _, err := s.db.sql.Exec(`UPDATE inbox_items SET created_at = ? WHERE id = ?`,
		"2099-01-01T00:00:00Z", second.ID); err != nil {
		t.Fatalf("bump time: %v", err)
	}
	list, err := s.List(false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if list[0].ID != second.ID || list[1].ID != first.ID {
		t.Fatalf("not newest-first: got %s,%s", list[0].Title, list[1].Title)
	}
}

func TestInboxSetStatusUnknownID(t *testing.T) {
	s := newTestInboxStore(t)
	if _, err := s.SetStatus("nope", InboxStatusArchived); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestInboxSetStatusInvalidStatus(t *testing.T) {
	s := newTestInboxStore(t)
	it, _ := s.Capture(InboxItem{Source: "manual", Title: "x"})
	if _, err := s.SetStatus(it.ID, "bogus"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound for invalid status", err)
	}
}

func TestInboxRoundtripTags(t *testing.T) {
	s := newTestInboxStore(t)
	it, _ := s.Capture(InboxItem{Source: "rss", Title: "tagged", Tags: []string{"news", "ai"}})
	got, ok, err := s.Get(it.ID)
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "news" || got.Tags[1] != "ai" {
		t.Fatalf("tags roundtrip failed: %v", got.Tags)
	}
}

// fakeSource exercises the injectable InboxSource seam.
type fakeSource struct {
	items   []InboxItem
	drained bool
}

func (f *fakeSource) Name() string { return "fake" }
func (f *fakeSource) Drain() ([]InboxItem, error) {
	if f.drained {
		return nil, nil
	}
	f.drained = true
	return f.items, nil
}

func TestInboxIngestFromSource(t *testing.T) {
	s := newTestInboxStore(t)
	src := &fakeSource{items: []InboxItem{
		{Source: "im", Title: "msg1"},
		{Source: "im", Content: "msg2 body"},
		{Source: "im"}, // empty title+content → skipped
	}}
	n, err := s.IngestFrom(src)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if n != 2 {
		t.Fatalf("ingested %d, want 2 (empty item skipped)", n)
	}
	list, _ := s.List(false)
	if len(list) != 2 {
		t.Fatalf("list = %d, want 2", len(list))
	}
}
