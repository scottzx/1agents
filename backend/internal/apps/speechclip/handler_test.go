package speechclip

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestHandler returns a Handler with a nil taskAPI — safe for handlers that
// don't dispatch tasks (HandleTimeline, HandleAssetFile).
func newTestHandler() *Handler { return &Handler{} }

// ── HandleTimeline ────────────────────────────────────────────────────────────

func TestHandleTimeline_GetMissing(t *testing.T) {
	ws := t.TempDir()
	h := newTestHandler()
	q := url.Values{"workspacePath": {ws}}
	r := httptest.NewRequest(http.MethodGet, "/api/speech_clip/timeline?"+q.Encode(), nil)
	rw := httptest.NewRecorder()
	h.HandleTimeline(rw, r)
	if rw.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", rw.Code, rw.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rw.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	tl, exists := resp["timeline"]
	if !exists {
		t.Fatal("response missing 'timeline' key")
	}
	if tl != nil {
		t.Fatalf("expected timeline:null for missing file, got %v", tl)
	}
}

func TestHandleTimeline_PostValid(t *testing.T) {
	ws := t.TempDir()
	h := newTestHandler()

	body, _ := json.Marshal(map[string]any{
		"workspacePath": ws,
		"timeline":      validTimelineFixture,
	})
	r := httptest.NewRequest(http.MethodPost, "/api/speech_clip/timeline", strings.NewReader(string(body)))
	rw := httptest.NewRecorder()
	h.HandleTimeline(rw, r)
	if rw.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", rw.Code, rw.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rw.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["ok"] != true {
		t.Fatalf("expected ok:true, got %v", resp)
	}
	if resp["timeline"] == nil {
		t.Fatal("expected timeline in response")
	}

	// Verify the file was written.
	if _, err := os.Stat(filepath.Join(appDir(ws), "timelines", "main.json")); err != nil {
		t.Fatalf("timelines/main.json not written: %v", err)
	}
}

func TestHandleTimeline_GetAfterPost(t *testing.T) {
	ws := t.TempDir()
	h := newTestHandler()

	// POST first
	body, _ := json.Marshal(map[string]any{"workspacePath": ws, "timeline": validTimelineFixture})
	r := httptest.NewRequest(http.MethodPost, "/api/speech_clip/timeline", strings.NewReader(string(body)))
	h.HandleTimeline(httptest.NewRecorder(), r)

	// GET should return the saved timeline
	q := url.Values{"workspacePath": {ws}}
	r2 := httptest.NewRequest(http.MethodGet, "/api/speech_clip/timeline?"+q.Encode(), nil)
	rw2 := httptest.NewRecorder()
	h.HandleTimeline(rw2, r2)
	if rw2.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", rw2.Code, rw2.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rw2.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["timeline"] == nil {
		t.Fatal("expected non-null timeline after save")
	}
}

func TestHandleTimeline_PostInvalidSchema(t *testing.T) {
	ws := t.TempDir()
	h := newTestHandler()

	// assetId empty → ValidateTimeline returns error
	body, _ := json.Marshal(map[string]any{
		"workspacePath": ws,
		"timeline": map[string]any{
			"version": 1, "id": "main", "assetId": "",
			"clips": []any{map[string]any{"startMs": 0, "endMs": 3000, "text": "x", "sourceSentenceIds": []int{0}}},
		},
	})
	r := httptest.NewRequest(http.MethodPost, "/api/speech_clip/timeline", strings.NewReader(string(body)))
	rw := httptest.NewRecorder()
	h.HandleTimeline(rw, r)
	if rw.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d: %s", rw.Code, rw.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rw.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["code"] != "invalid_schema" {
		t.Fatalf("expected code=invalid_schema, got %v", resp["code"])
	}
	if resp["error"] == "" || resp["error"] == nil {
		t.Fatal("expected non-empty error message")
	}
}

// ── HandleAssetFile ───────────────────────────────────────────────────────────

func TestHandleAssetFile_NotFound(t *testing.T) {
	ws := t.TempDir()
	h := newTestHandler()
	q := url.Values{"workspacePath": {ws}, "assetId": {"a99"}}
	r := httptest.NewRequest(http.MethodGet, "/api/speech_clip/assets/file?"+q.Encode(), nil)
	rw := httptest.NewRecorder()
	h.HandleAssetFile(rw, r)
	if rw.Code != http.StatusNotFound {
		t.Fatalf("expected 404 got %d: %s", rw.Code, rw.Body.String())
	}
}

func TestHandleAssetFile_ServeFile(t *testing.T) {
	ws := t.TempDir()
	h := newTestHandler()

	proj := &Project{Assets: []Asset{{ID: "a01", File: "a01.mp3", Label: "test", AddedAt: "2026-01-01T00:00:00Z"}}}
	if err := h.saveProject(ws, proj); err != nil {
		t.Fatal(err)
	}
	assetsDir := filepath.Join(appDir(ws), "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetsDir, "a01.mp3"), []byte("audio data"), 0o644); err != nil {
		t.Fatal(err)
	}

	q := url.Values{"workspacePath": {ws}, "assetId": {"a01"}}
	r := httptest.NewRequest(http.MethodGet, "/api/speech_clip/assets/file?"+q.Encode(), nil)
	rw := httptest.NewRecorder()
	h.HandleAssetFile(rw, r)
	if rw.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", rw.Code, rw.Body.String())
	}
}

func TestHandleAssetFile_PathTraversal(t *testing.T) {
	ws := t.TempDir()
	h := newTestHandler()

	// Manually write a project.json with a malicious file path.
	proj := &Project{Assets: []Asset{{ID: "a01", File: "../../../etc/passwd", Label: "evil", AddedAt: "2026-01-01T00:00:00Z"}}}
	if err := h.saveProject(ws, proj); err != nil {
		t.Fatal(err)
	}

	q := url.Values{"workspacePath": {ws}, "assetId": {"a01"}}
	r := httptest.NewRequest(http.MethodGet, "/api/speech_clip/assets/file?"+q.Encode(), nil)
	rw := httptest.NewRecorder()
	h.HandleAssetFile(rw, r)
	if rw.Code != http.StatusForbidden {
		t.Fatalf("expected 403 got %d: %s", rw.Code, rw.Body.String())
	}
}
