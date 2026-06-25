package social

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

func newInboxStore(t *testing.T) *meta.InboxStore {
	t.Helper()
	db, err := meta.Open(filepath.Join(t.TempDir(), "meta.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return meta.NewInboxStore(db)
}

func TestStaticSourceFetch(t *testing.T) {
	src := &StaticSource{
		PlatformName: "weibo",
		Snapshots:    []Metrics{{PublicationID: "p1", Views: 10}},
	}
	if src.Platform() != "weibo" {
		t.Fatalf("platform = %q", src.Platform())
	}
	got, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(got) != 1 || got[0].Views != 10 {
		t.Fatalf("unexpected snapshots %+v", got)
	}
}

func TestStaticSourceDefaultPlatform(t *testing.T) {
	if (&StaticSource{}).Platform() != "static" {
		t.Fatal("empty platform should default to static")
	}
}

func TestToInboxItem(t *testing.T) {
	fetched := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	it := toInboxItem("douyin", Metrics{
		PublicationID: "vid42",
		Title:         "我的视频",
		URL:           "https://example.com/v/42",
		Views:         1200,
		Comments:      34,
		Followers:     5,
		FetchedAt:     fetched,
	})
	if it.Source != meta.InboxSourceMisc {
		t.Fatalf("source = %q, want misc", it.Source)
	}
	if it.Title != "我的视频" {
		t.Fatalf("title = %q", it.Title)
	}
	if it.URL != "https://example.com/v/42" {
		t.Fatalf("url = %q", it.URL)
	}
	if it.Summary != "views 1200 · comments 34 · followers 5" {
		t.Fatalf("summary = %q", it.Summary)
	}
	if !it.CreatedAt.Equal(fetched) {
		t.Fatalf("createdAt = %v, want %v", it.CreatedAt, fetched)
	}
	if !hasTag(it.Tags, "social-feedback") || !hasTag(it.Tags, "douyin") {
		t.Fatalf("tags = %v", it.Tags)
	}
}

func TestToInboxItemFallbacks(t *testing.T) {
	it := toInboxItem("", Metrics{PublicationID: "p9"})
	if it.Title != "p9" {
		t.Fatalf("empty title should fall back to publication id, got %q", it.Title)
	}
	if it.CreatedAt.IsZero() {
		t.Fatal("zero FetchedAt should be filled with now")
	}
	if hasTag(it.Tags, "") {
		t.Fatalf("empty platform should not become a tag: %v", it.Tags)
	}
}

func TestReflowIntoInbox(t *testing.T) {
	store := newInboxStore(t)
	src := &StaticSource{
		PlatformName: "weibo",
		Snapshots: []Metrics{
			{PublicationID: "p1", Title: "帖子一", Views: 100, Comments: 5, Followers: 2},
			{PublicationID: "p2", Title: "帖子二", Views: 50},
		},
	}

	n, err := Reflow(store, src)
	if err != nil {
		t.Fatalf("reflow: %v", err)
	}
	if n != 2 {
		t.Fatalf("captured %d, want 2", n)
	}

	items, err := store.List(false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("inbox has %d items, want 2", len(items))
	}
	for _, it := range items {
		if it.Source != meta.InboxSourceMisc {
			t.Fatalf("reflowed item source = %q, want misc", it.Source)
		}
		if !hasTag(it.Tags, "social-feedback") || !hasTag(it.Tags, "weibo") {
			t.Fatalf("reflowed item tags = %v", it.Tags)
		}
		if it.Status != meta.InboxStatusUnread {
			t.Fatalf("reflowed item status = %q, want unread", it.Status)
		}
	}
}

func TestBridgeName(t *testing.T) {
	b := NewBridge(&StaticSource{PlatformName: "xhs"})
	if b.Name() != "social:xhs" {
		t.Fatalf("name = %q", b.Name())
	}
}

type errSource struct{}

func (errSource) Platform() string                   { return "boom" }
func (errSource) Fetch(context.Context) ([]Metrics, error) {
	return nil, errors.New("api down")
}

func TestReflowPropagatesFetchError(t *testing.T) {
	store := newInboxStore(t)
	if _, err := Reflow(store, errSource{}); err == nil {
		t.Fatal("expected fetch error to propagate")
	}
}

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}
