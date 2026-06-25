package meta

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestProjectArchiveLifecycle covers the #141 archive/close/reopen store layer:
// EnsureProject seeds an active project, archive/close move it out of the active
// view with distinct status+reason while preserving the row, and reopen restores
// it.
func TestProjectArchiveLifecycle(t *testing.T) {
	db := newTestDB(t)
	if err := db.EnsureProject("ws1", "Proj", "/tmp/p1"); err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	// Fresh project is active with no archive metadata.
	p, ok, err := db.GetProject("ws1")
	if err != nil || !ok {
		t.Fatalf("GetProject: ok=%v err=%v", ok, err)
	}
	if p.Status != ProjectStatusActive || p.ArchiveReason != "" || p.ArchivedAt != nil {
		t.Fatalf("fresh project not active/clean: %+v", p)
	}

	// Archive (阶段性完成归档).
	if err := db.ArchiveProject("ws1", ProjectStatusArchived, ArchiveReasonCompleted, "shipped v1"); err != nil {
		t.Fatalf("ArchiveProject: %v", err)
	}
	p, _, _ = db.GetProject("ws1")
	if p.Status != ProjectStatusArchived || p.ArchiveReason != ArchiveReasonCompleted {
		t.Fatalf("archive: status/reason wrong: %+v", p)
	}
	if p.ArchiveNote != "shipped v1" || p.ArchivedAt == nil {
		t.Fatalf("archive: note/timestamp not recorded: %+v", p)
	}

	// Data is preserved — the row still lists (just not under active).
	if active, _ := db.ListProjectsByStatus(ProjectStatusActive); len(active) != 0 {
		t.Fatalf("archived project should not be active: %+v", active)
	}
	if archived, _ := db.ListProjectsByStatus(ProjectStatusArchived); len(archived) != 1 {
		t.Fatalf("archived project missing from archive view: %+v", archived)
	}
	if all, _ := db.ListProjects(); len(all) != 1 {
		t.Fatalf("ListProjects should keep the row: %+v", all)
	}

	// Reopen → back to active, archive metadata cleared.
	if err := db.ReopenProject("ws1"); err != nil {
		t.Fatalf("ReopenProject: %v", err)
	}
	p, _, _ = db.GetProject("ws1")
	if p.Status != ProjectStatusActive || p.ArchiveReason != "" || p.ArchiveNote != "" || p.ArchivedAt != nil {
		t.Fatalf("reopen did not clear archive metadata: %+v", p)
	}

	// Close (竞品出现砍掉) → killed status with superseded reason.
	if err := db.ArchiveProject("ws1", ProjectStatusKilled, ArchiveReasonSuperseded, "大厂已做"); err != nil {
		t.Fatalf("close (kill): %v", err)
	}
	p, _, _ = db.GetProject("ws1")
	if p.Status != ProjectStatusKilled || p.ArchiveReason != ArchiveReasonSuperseded {
		t.Fatalf("close: status/reason wrong: %+v", p)
	}
}

// EnsureProject must not clobber an archived project's status (renames in the
// workspace registry keep flowing in, but they shouldn't resurrect it).
func TestEnsureProjectPreservesArchived(t *testing.T) {
	db := newTestDB(t)
	if err := db.EnsureProject("ws1", "Proj", "/tmp/p1"); err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := db.ArchiveProject("ws1", ProjectStatusArchived, ArchiveReasonCompleted, ""); err != nil {
		t.Fatalf("ArchiveProject: %v", err)
	}
	// Re-ensure (e.g. on server restart / rename) must not flip it back to active.
	if err := db.EnsureProject("ws1", "Renamed", "/tmp/p1"); err != nil {
		t.Fatalf("EnsureProject again: %v", err)
	}
	p, _, _ := db.GetProject("ws1")
	if p.Status != ProjectStatusArchived {
		t.Fatalf("EnsureProject resurrected archived project: %+v", p)
	}
	if p.Name != "Renamed" {
		t.Fatalf("EnsureProject should still refresh the name: %+v", p)
	}
}

func TestArchiveProjectErrors(t *testing.T) {
	db := newTestDB(t)
	// Unknown id → ErrNotFound.
	if err := db.ArchiveProject("nope", ProjectStatusArchived, ArchiveReasonCompleted, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("archive unknown id: got %v, want ErrNotFound", err)
	}
	if err := db.ReopenProject("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("reopen unknown id: got %v, want ErrNotFound", err)
	}
	// Invalid target status (active is not an archive target).
	if err := db.EnsureProject("ws1", "Proj", "/tmp/p1"); err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := db.ArchiveProject("ws1", ProjectStatusActive, "", ""); err == nil {
		t.Fatalf("archive with status=active should error")
	}
}

func TestProjectActionHandler(t *testing.T) {
	db := newTestDB(t)
	if err := db.EnsureProject("ws1", "Proj", "/tmp/p1"); err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	h := ProjectActionHandler(db)

	do := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		rec := httptest.NewRecorder()
		h(rec, req)
		return rec
	}

	// archive with empty body → default reason "completed".
	rec := do(http.MethodPost, "/api/projects/ws1/archive", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("archive: code=%d body=%s", rec.Code, rec.Body.String())
	}
	var p Project
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("archive decode: %v", err)
	}
	if p.Status != ProjectStatusArchived || p.ArchiveReason != ArchiveReasonCompleted {
		t.Fatalf("archive default: %+v", p)
	}

	// close with explicit reason+note → killed/superseded.
	rec = do(http.MethodPost, "/api/projects/ws1/close", `{"reason":"superseded","note":"竞品"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("close: code=%d body=%s", rec.Code, rec.Body.String())
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &p)
	if p.Status != ProjectStatusKilled || p.ArchiveReason != ArchiveReasonSuperseded || p.ArchiveNote != "竞品" {
		t.Fatalf("close explicit: %+v", p)
	}

	// reopen → active.
	rec = do(http.MethodPost, "/api/projects/ws1/reopen", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("reopen: code=%d body=%s", rec.Code, rec.Body.String())
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &p)
	if p.Status != ProjectStatusActive {
		t.Fatalf("reopen: %+v", p)
	}

	// unknown id → 404; unknown action → 404; GET → 405.
	if rec := do(http.MethodPost, "/api/projects/missing/archive", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown id: code=%d", rec.Code)
	}
	if rec := do(http.MethodPost, "/api/projects/ws1/bogus", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown action: code=%d", rec.Code)
	}
	if rec := do(http.MethodGet, "/api/projects/ws1/archive", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET action: code=%d", rec.Code)
	}
}

// TestProjectsHandlerStatusFilter exercises the GET ?status= filter.
func TestProjectsHandlerStatusFilter(t *testing.T) {
	db := newTestDB(t)
	_ = db.EnsureProject("a", "A", "/tmp/a")
	_ = db.EnsureProject("b", "B", "/tmp/b")
	if err := db.ArchiveProject("b", ProjectStatusArchived, ArchiveReasonCompleted, ""); err != nil {
		t.Fatalf("ArchiveProject: %v", err)
	}
	h := ProjectsHandler(db)

	get := func(query string) []Project {
		req := httptest.NewRequest(http.MethodGet, "/api/projects"+query, nil)
		rec := httptest.NewRecorder()
		h(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: code=%d", query, rec.Code)
		}
		var ps []Project
		if err := json.Unmarshal(rec.Body.Bytes(), &ps); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return ps
	}

	if all := get(""); len(all) != 2 {
		t.Fatalf("no filter: want 2, got %d", len(all))
	}
	active := get("?status=active")
	if len(active) != 1 || active[0].ID != "a" {
		t.Fatalf("status=active: %+v", active)
	}
	archived := get("?status=archived")
	if len(archived) != 1 || archived[0].ID != "b" {
		t.Fatalf("status=archived: %+v", archived)
	}
}
