package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/workspace"
)

// workCaseRig wires a Handler over an isolated ONEAGENTS_HOME with one
// registered workspace, mirroring mutationAttributionRig.
func workCaseRig(t *testing.T) (*Handler, *meta.DB, string, string) {
	t.Helper()
	t.Setenv("ONEAGENTS_HOME", t.TempDir())
	wsID := "project-1"
	wsPath := t.TempDir()
	if err := workspace.NewHandler().SaveWorkspacesConfig(&workspace.WorkspacesConfig{
		Workspaces: []workspace.Workspace{
			{ID: wsID, Name: "Project 1", Path: wsPath, Status: "active"},
		},
	}); err != nil {
		t.Fatalf("SaveWorkspacesConfig: %v", err)
	}
	db, err := meta.OpenDefault()
	if err != nil {
		t.Fatalf("OpenDefault: %v", err)
	}
	if err := db.EnsureProject(wsID, "Project 1", wsPath); err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	store := meta.NewSessionStore(db)
	tasksStore := meta.NewTaskStore(db)
	scheduler := NewScheduler(tasksStore, func() ([]WorkspaceRef, error) { return nil, nil })
	h := NewHandler(store, tasksStore, nil, scheduler, NewCatalogStore(), "http://127.0.0.1:0")
	return h, db, wsID, wsPath
}

func doWorkCase(t *testing.T, h *Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == nil {
		req = httptest.NewRequest(method, path, nil)
	} else {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		req = httptest.NewRequest(method, path, strings.NewReader(string(raw)))
	}
	rr := httptest.NewRecorder()
	if strings.HasPrefix(path, "/api/agent/work-cases/") {
		h.HandleWorkCasesItem(rr, req)
	} else {
		h.HandleWorkCasesRoot(rr, req)
	}
	return rr
}

// createWorkCaseHTTP posts a new case and returns its decoded body.
func createWorkCaseHTTP(t *testing.T, h *Handler, wsID, title string) map[string]any {
	t.Helper()
	rr := doWorkCase(t, h, http.MethodPost, "/api/agent/work-cases", map[string]any{
		"workspace_id": wsID,
		"title":        title,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("create %s status %d: %s", title, rr.Code, rr.Body.String())
	}
	var out map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&out); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if out["id"] == "" || out["id"] == nil {
		t.Fatalf("create returned no id: %v", out)
	}
	return out
}

func TestWorkCaseHTTPCreateGetListDelete(t *testing.T) {
	h, _, wsID, _ := workCaseRig(t)

	// Missing fields / unknown workspace are rejected.
	if rr := doWorkCase(t, h, http.MethodPost, "/api/agent/work-cases", map[string]any{"workspace_id": wsID}); rr.Code != http.StatusBadRequest {
		t.Fatalf("create without title status %d", rr.Code)
	}
	if rr := doWorkCase(t, h, http.MethodPost, "/api/agent/work-cases", map[string]any{"workspace_id": "ghost", "title": "x"}); rr.Code != http.StatusNotFound {
		t.Fatalf("create unknown workspace status %d", rr.Code)
	}
	if rr := doWorkCase(t, h, http.MethodGet, "/api/agent/work-cases", nil); rr.Code != http.StatusBadRequest {
		t.Fatalf("list without workspace_id status %d", rr.Code)
	}

	created := createWorkCaseHTTP(t, h, wsID, "第一个 WorkCase")
	if created["status"] != "open" {
		t.Fatalf("created status=%v, want open", created["status"])
	}
	if v, _ := created["version"].(float64); v != 1 {
		t.Fatalf("created version=%v, want 1", created["version"])
	}
	if created["eventId"] == "" || created["eventId"] == nil {
		t.Fatalf("create response missing eventId: %v", created)
	}
	id := created["id"].(string)

	// GET item returns the case with its (empty) link list.
	rr := doWorkCase(t, h, http.MethodGet, "/api/agent/work-cases/"+id, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("get status %d", rr.Code)
	}
	var detail struct {
		meta.WorkCase
		Links []meta.CaseLink `json:"links"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if detail.Title != "第一个 WorkCase" || detail.Links == nil {
		t.Fatalf("detail mismatch: %+v", detail)
	}

	// GET list (all + status filter + bogus filter).
	rr = doWorkCase(t, h, http.MethodGet, "/api/agent/work-cases?workspace_id="+wsID, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("list status %d", rr.Code)
	}
	var listed []meta.WorkCase
	if err := json.NewDecoder(rr.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != id {
		t.Fatalf("list mismatch: %+v", listed)
	}
	rr = doWorkCase(t, h, http.MethodGet, "/api/agent/work-cases?workspace_id="+wsID+"&status=closed", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("filtered list status %d", rr.Code)
	}
	listed = nil
	if err := json.NewDecoder(rr.Body).Decode(&listed); err != nil {
		t.Fatalf("decode filtered list: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("closed filter returned %d cases", len(listed))
	}
	if rr := doWorkCase(t, h, http.MethodGet, "/api/agent/work-cases?workspace_id="+wsID+"&status=bogus", nil); rr.Code != http.StatusBadRequest {
		t.Fatalf("bogus status filter status %d", rr.Code)
	}

	// DELETE removes the case (subsequent GET 404s).
	rr = doWorkCase(t, h, http.MethodDelete, "/api/agent/work-cases/"+id+"?workspace_id="+wsID, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete status %d: %s", rr.Code, rr.Body.String())
	}
	if rr := doWorkCase(t, h, http.MethodGet, "/api/agent/work-cases/"+id, nil); rr.Code != http.StatusNotFound {
		t.Fatalf("get after delete status %d", rr.Code)
	}
	// Deleting through the wrong workspace is rejected.
	ghost := createWorkCaseHTTP(t, h, wsID, "scoped delete")
	if rr := doWorkCase(t, h, http.MethodDelete, "/api/agent/work-cases/"+ghost["id"].(string)+"?workspace_id=ghost", nil); rr.Code != http.StatusNotFound {
		t.Fatalf("delete via unknown workspace status %d", rr.Code)
	}
}

func TestWorkCaseHTTPPatchAndVersionConflict(t *testing.T) {
	h, db, wsID, _ := workCaseRig(t)
	created := createWorkCaseHTTP(t, h, wsID, "patch target")
	id := created["id"].(string)

	// currentPhase carries arbitrary application vocabulary — the kernel must
	// accept it without validation (应用投影，不编码领域规则).
	rr := doWorkCase(t, h, http.MethodPatch, "/api/agent/work-cases/"+id, map[string]any{
		"expectedVersion": 1,
		"title":           "renamed",
		"currentPhase":    "presales-qualification",
		"subjectRefs":     []string{"presales:opportunity:42"},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("patch status %d: %s", rr.Code, rr.Body.String())
	}
	var patched meta.WorkCase
	if err := json.NewDecoder(rr.Body).Decode(&patched); err != nil {
		t.Fatalf("decode patch: %v", err)
	}
	if patched.Title != "renamed" || patched.CurrentPhase != "presales-qualification" ||
		len(patched.SubjectRefs) != 1 || patched.Version != 2 {
		t.Fatalf("patch result mismatch: %+v", patched)
	}

	// Stale expectedVersion → 409; missing expectedVersion → 400.
	rr = doWorkCase(t, h, http.MethodPatch, "/api/agent/work-cases/"+id, map[string]any{
		"expectedVersion": 1,
		"title":           "conflict",
	})
	if rr.Code != http.StatusConflict {
		t.Fatalf("stale patch status %d, want 409: %s", rr.Code, rr.Body.String())
	}
	rr = doWorkCase(t, h, http.MethodPatch, "/api/agent/work-cases/"+id, map[string]any{"title": "x"})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("patch without version status %d, want 400", rr.Code)
	}
	// Malformed subject ref → 400 (structured domainref error).
	rr = doWorkCase(t, h, http.MethodPatch, "/api/agent/work-cases/"+id, map[string]any{
		"expectedVersion": 2,
		"primarySubject":  "not a ref",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad ref patch status %d, want 400", rr.Code)
	}
	// Unknown case → 404.
	if rr := doWorkCase(t, h, http.MethodPatch, "/api/agent/work-cases/deadbeef", map[string]any{"expectedVersion": 1}); rr.Code != http.StatusNotFound {
		t.Fatalf("patch unknown status %d", rr.Code)
	}
	// Stored row agrees.
	got, ok, err := meta.NewWorkCaseStore(db).Get(id)
	if err != nil || !ok || got.Title != "renamed" || got.Version != 2 {
		t.Fatalf("stored after patch: ok=%v err=%v case=%+v", ok, err, got)
	}
}

func TestWorkCaseHTTPLifecycleTransitions(t *testing.T) {
	h, _, wsID, _ := workCaseRig(t)
	created := createWorkCaseHTTP(t, h, wsID, "lifecycle")
	id := created["id"].(string)

	transition := func(status string, version int) *httptest.ResponseRecorder {
		return doWorkCase(t, h, http.MethodPost, "/api/agent/work-cases/"+id+"/transition", map[string]any{
			"status": status, "expectedVersion": version, "reason": "test",
		})
	}

	// open → suspended → open → closed are all legal.
	if rr := transition("suspended", 1); rr.Code != http.StatusOK {
		t.Fatalf("open→suspended status %d: %s", rr.Code, rr.Body.String())
	}
	if rr := transition("open", 2); rr.Code != http.StatusOK {
		t.Fatalf("suspended→open status %d: %s", rr.Code, rr.Body.String())
	}
	rr := transition("closed", 3)
	if rr.Code != http.StatusOK {
		t.Fatalf("open→closed status %d: %s", rr.Code, rr.Body.String())
	}
	var closed meta.WorkCase
	if err := json.NewDecoder(rr.Body).Decode(&closed); err != nil {
		t.Fatalf("decode closed: %v", err)
	}
	if closed.Status != meta.CaseStatusClosed || closed.CloseReason != "test" || closed.ClosedAt == nil || closed.Version != 4 {
		t.Fatalf("closed result mismatch: %+v", closed)
	}

	// 非法终态回退被拒绝: closed → open/suspended/closed/cancelled all fail.
	for _, to := range []string{"open", "suspended", "closed", "cancelled"} {
		if rr := transition(to, 4); rr.Code != http.StatusBadRequest {
			t.Fatalf("closed→%s status %d, want 400: %s", to, rr.Code, rr.Body.String())
		}
	}
	// Stale version on a live transition → 409.
	live := createWorkCaseHTTP(t, h, wsID, "live")
	rr = doWorkCase(t, h, http.MethodPost, "/api/agent/work-cases/"+live["id"].(string)+"/transition", map[string]any{
		"status": "suspended", "expectedVersion": 99,
	})
	if rr.Code != http.StatusConflict {
		t.Fatalf("stale transition status %d, want 409", rr.Code)
	}
	// Unknown status → 400; missing version → 400; unknown case → 404.
	if rr := doWorkCase(t, h, http.MethodPost, "/api/agent/work-cases/"+id+"/transition", map[string]any{"status": "won", "expectedVersion": 4}); rr.Code != http.StatusBadRequest {
		t.Fatalf("unknown status %d", rr.Code)
	}
	if rr := doWorkCase(t, h, http.MethodPost, "/api/agent/work-cases/"+live["id"].(string)+"/transition", map[string]any{"status": "open"}); rr.Code != http.StatusBadRequest {
		t.Fatalf("missing version %d", rr.Code)
	}
	if rr := doWorkCase(t, h, http.MethodPost, "/api/agent/work-cases/deadbeef/transition", map[string]any{"status": "open", "expectedVersion": 1}); rr.Code != http.StatusNotFound {
		t.Fatalf("unknown case transition %d", rr.Code)
	}
}

func TestWorkCaseHTTPLinksTasksRunsEvents(t *testing.T) {
	h, db, wsID, wsPath := workCaseRig(t)
	created := createWorkCaseHTTP(t, h, wsID, "linked case")
	id := created["id"].(string)

	// Seed a task + session in this workspace via the kernel stores.
	now := time.Now().UTC()
	if err := meta.NewTaskStore(db).Save(wsPath, &meta.TasksConfig{Tasks: []meta.Task{
		{ID: "task-1", Title: "T1", Status: meta.TaskStatusPending, Type: meta.ItemTypeTask, CreatedAt: now, UpdatedAt: now},
	}}); err != nil {
		t.Fatalf("Save task: %v", err)
	}
	if err := meta.NewSessionStore(db).Add(meta.ChatSessionRecord{ID: "sess-1", WorkspaceID: wsID, Name: "case session"}); err != nil {
		t.Fatalf("Add session: %v", err)
	}

	link := func(kind, target string, version int) *httptest.ResponseRecorder {
		return doWorkCase(t, h, http.MethodPost, "/api/agent/work-cases/"+id+"/links", map[string]any{
			"kind": kind, "targetId": target, "expectedVersion": version,
		})
	}

	// Link task, session and two artifact refs — one Case, many objects.
	if rr := link("task", "task-1", 1); rr.Code != http.StatusOK {
		t.Fatalf("link task status %d: %s", rr.Code, rr.Body.String())
	}
	if rr := link("session", "sess-1", 2); rr.Code != http.StatusOK {
		t.Fatalf("link session status %d: %s", rr.Code, rr.Body.String())
	}
	if rr := link("artifact", ".artifacts/app/brief.md", 3); rr.Code != http.StatusOK {
		t.Fatalf("link artifact status %d: %s", rr.Code, rr.Body.String())
	}
	if rr := link("artifact", ".artifacts/app/deck.pdf", 4); rr.Code != http.StatusOK {
		t.Fatalf("link artifact 2 status %d: %s", rr.Code, rr.Body.String())
	}
	// Duplicate → 409; unknown task → 404; unknown kind → 400; stale version → 409.
	if rr := link("task", "task-1", 5); rr.Code != http.StatusConflict {
		t.Fatalf("duplicate link status %d, want 409", rr.Code)
	}
	if rr := link("task", "ghost-task", 5); rr.Code != http.StatusNotFound {
		t.Fatalf("unknown task link status %d, want 404", rr.Code)
	}
	if rr := link("weird", "x", 5); rr.Code != http.StatusBadRequest {
		t.Fatalf("unknown kind status %d, want 400", rr.Code)
	}
	if rr := link("artifact", ".artifacts/app/x", 2); rr.Code != http.StatusConflict {
		t.Fatalf("stale link status %d, want 409", rr.Code)
	}

	// GET item surfaces the links.
	rr := doWorkCase(t, h, http.MethodGet, "/api/agent/work-cases/"+id, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("get status %d", rr.Code)
	}
	var detail struct {
		Version int             `json:"version"`
		Links   []meta.CaseLink `json:"links"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if len(detail.Links) != 4 || detail.Version != 5 {
		t.Fatalf("detail links=%d version=%d, want 4/5", len(detail.Links), detail.Version)
	}

	// Task 可按 Case 查询.
	rr = doWorkCase(t, h, http.MethodGet, "/api/agent/work-cases/"+id+"/tasks", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("tasks status %d", rr.Code)
	}
	var tasksPage struct {
		Items []meta.Task `json:"items"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&tasksPage); err != nil {
		t.Fatalf("decode tasks: %v", err)
	}
	if len(tasksPage.Items) != 1 || tasksPage.Items[0].ID != "task-1" {
		t.Fatalf("tasks by case: %+v", tasksPage.Items)
	}

	// TaskRun 可按 Case 查询.
	run, err := meta.NewTaskRunStore(db).Create(wsPath, meta.TaskRun{TaskID: "task-1", Kind: meta.TaskRunExecution})
	if err != nil {
		t.Fatalf("TaskRun create: %v", err)
	}
	if _, err := meta.NewTaskRunStore(db).Finish(run.ID, meta.TaskRunCompleted, nil, nil, &meta.ClosedBy{Kind: "manual_decision", Verdict: "accepted"}, ""); err != nil {
		t.Fatalf("TaskRun finish: %v", err)
	}
	rr = doWorkCase(t, h, http.MethodGet, "/api/agent/work-cases/"+id+"/runs", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("runs status %d", rr.Code)
	}
	var runsPage struct {
		Items []meta.TaskRun `json:"items"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&runsPage); err != nil {
		t.Fatalf("decode runs: %v", err)
	}
	if len(runsPage.Items) != 1 || runsPage.Items[0].ID != run.ID || runsPage.Items[0].Status != meta.TaskRunCompleted {
		t.Fatalf("runs by case: %+v", runsPage.Items)
	}

	// Unlink the task, then the tasks view is empty again.
	rr = doWorkCase(t, h, http.MethodDelete,
		"/api/agent/work-cases/"+id+"/links?kind=task&target_id=task-1&expected_version=5", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("unlink status %d: %s", rr.Code, rr.Body.String())
	}
	rr = doWorkCase(t, h, http.MethodGet, "/api/agent/work-cases/"+id+"/tasks", nil)
	tasksPage.Items = nil
	if err := json.NewDecoder(rr.Body).Decode(&tasksPage); err != nil {
		t.Fatalf("decode tasks after unlink: %v", err)
	}
	if len(tasksPage.Items) != 0 {
		t.Fatalf("tasks after unlink: %+v", tasksPage.Items)
	}
	// Unlinking a missing edge → 404; bad expected_version → 400.
	if rr := doWorkCase(t, h, http.MethodDelete, "/api/agent/work-cases/"+id+"/links?kind=task&target_id=task-1&expected_version=6", nil); rr.Code != http.StatusNotFound {
		t.Fatalf("double unlink status %d, want 404", rr.Code)
	}
	if rr := doWorkCase(t, h, http.MethodDelete, "/api/agent/work-cases/"+id+"/links?kind=task&target_id=task-1", nil); rr.Code != http.StatusBadRequest {
		t.Fatalf("unlink without version status %d, want 400", rr.Code)
	}

	// Event 关联可按 Case 查询: create + 4 links + 1 unlink = 6 events.
	rr = doWorkCase(t, h, http.MethodGet, "/api/agent/work-cases/"+id+"/events", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("events status %d", rr.Code)
	}
	var eventsPage meta.ProjectEventPage
	if err := json.NewDecoder(rr.Body).Decode(&eventsPage); err != nil {
		t.Fatalf("decode events: %v", err)
	}
	if len(eventsPage.Items) != 6 {
		t.Fatalf("events=%d, want 6: %+v", len(eventsPage.Items), eventsPage.Items)
	}
	for _, ev := range eventsPage.Items {
		if ev.TargetType != "work_case" || ev.TargetID != id {
			t.Fatalf("event not case-scoped: %+v", ev)
		}
	}
	// Unknown case → 404 on every sub-resource.
	for _, sub := range []string{"tasks", "runs", "events"} {
		if rr := doWorkCase(t, h, http.MethodGet, "/api/agent/work-cases/deadbeef/"+sub, nil); rr.Code != http.StatusNotFound {
			t.Fatalf("%s unknown case status %d", sub, rr.Code)
		}
	}
}
