package retro

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/kwiki"
)

// seedStore ingests one retrospective (via Archive) plus one unrelated wiki
// page, so the handler's source/tag filter is exercised.
func seedStore(t *testing.T) *kwiki.Store {
	t.Helper()
	store, err := kwiki.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open kwiki: %v", err)
	}
	if _, err := Archive(store, sampleInput()); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if _, err := store.Ingest(kwiki.InboxItem{
		Title:  "无关页面",
		Text:   "这是一条普通的 inbox 知识，不是复盘。",
		Source: "manual",
		Domain: "work",
	}); err != nil {
		t.Fatalf("ingest non-retro: %v", err)
	}
	return store
}

func TestHandlerList(t *testing.T) {
	h := Handler(seedStore(t))
	req := httptest.NewRequest(http.MethodGet, "/api/retrospectives", nil)
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp struct {
		Items []Item `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("got %d retros, want 1 (non-retro page must be filtered out)", len(resp.Items))
	}
	it := resp.Items[0]
	if it.Title != "复盘：Remote Agent" {
		t.Errorf("title = %q", it.Title)
	}
	if it.Body == "" {
		t.Error("body is empty")
	}
}

func TestHandlerSingle(t *testing.T) {
	store := seedStore(t)
	h := Handler(store)

	// Find the retro slug from the list first.
	pages, _ := store.Pages()
	var slug string
	for _, p := range pages {
		if isRetro(p) {
			slug = p.Slug
		}
	}
	if slug == "" {
		t.Fatal("no retro slug found")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/retrospectives/"+slug, nil)
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var it Item
	if err := json.Unmarshal(rec.Body.Bytes(), &it); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if it.Slug != slug {
		t.Errorf("slug = %q, want %q", it.Slug, slug)
	}
}

func TestHandlerSingleNotFound(t *testing.T) {
	h := Handler(seedStore(t))
	req := httptest.NewRequest(http.MethodGet, "/api/retrospectives/does-not-exist", nil)
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestHandlerMethodNotAllowed(t *testing.T) {
	h := Handler(seedStore(t))
	req := httptest.NewRequest(http.MethodPost, "/api/retrospectives", nil)
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}
