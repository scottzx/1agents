package agent

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestHandler(t *testing.T) (*Handler, *Store) {
	t.Helper()
	s := newTestStore(t)
	tasksStore, err := NewTasksStore()
	if err != nil {
		t.Fatalf("NewTasksStore: %v", err)
	}
	acpxClient := NewAcpxClient(38082)
	workspacesFn := func() ([]WorkspaceRef, error) {
		return []WorkspaceRef{}, nil
	}
	scheduler := NewScheduler(tasksStore, workspacesFn)
	return NewHandler(s, tasksStore, acpxClient, scheduler, NewCatalogStore(), "http://127.0.0.1:0"), s
}

func TestHandlerAgentTypes(t *testing.T) {
	h, _ := newTestHandler(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/agent/agent-types", nil)
	h.HandleAgentTypes(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rr.Code)
	}
	var got []string
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) == 0 {
		t.Fatalf("empty agent-types list")
	}
	if got[0] != AgentTypeClaudecode {
		t.Fatalf("first agent %q, want %q", got[0], AgentTypeClaudecode)
	}
}

func TestHandlerListRequiresWorkspaceID(t *testing.T) {
	h, _ := newTestHandler(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/agent/sessions", nil)
	h.HandleSessionsRoot(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rr.Code)
	}
}

func TestHandlerCreateListGetDeleteRoundTrip(t *testing.T) {
	h, _ := newTestHandler(t)

	// Create
	body := IndexRequest{
		WorkspaceID: "ws-1",
		Name:        "test session",
		AgentType:   AgentTypeCodex,
		CcProject:   "ws-1__codex",
		CcSessionID: "cc-abc",
		SessionKey:  "chatui:ws-1:cc-abc",
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agent/sessions", jsonBody(body))
	h.HandleSessionsRoot(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create status %d, body %s", rr.Code, rr.Body.String())
	}
	var created ChatSessionRecord
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("create returned empty id")
	}
	if created.WorkspaceID != "ws-1" {
		t.Fatalf("create returned wrong workspace %q", created.WorkspaceID)
	}
	if created.AgentType != AgentTypeCodex {
		t.Fatalf("create returned wrong agent_type %q", created.AgentType)
	}

	// List
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/agent/sessions?workspace_id=ws-1", nil)
	h.HandleSessionsRoot(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list status %d", rr.Code)
	}
	var listed []ChatSessionRecord
	if err := json.NewDecoder(rr.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("listed %d, want 1", len(listed))
	}

	// Get
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/agent/sessions/"+created.ID, nil)
	h.HandleSessionsItem(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get status %d", rr.Code)
	}

	// Delete
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/agent/sessions/"+created.ID, nil)
	h.HandleSessionsItem(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete status %d", rr.Code)
	}

	// Get after delete → 404
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/agent/sessions/"+created.ID, nil)
	h.HandleSessionsItem(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("get-after-delete status %d, want 404", rr.Code)
	}
}

func TestHandlerCreateRejectsMissingFields(t *testing.T) {
	h, _ := newTestHandler(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agent/sessions",
		strings.NewReader(`{"workspace_id":"ws-1"}`))
	h.HandleSessionsRoot(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rr.Code)
	}
}

func TestHandlerCreateTmpEphemeral(t *testing.T) {
	h, _ := newTestHandler(t)
	body := IndexRequest{
		WorkspaceID: "oneshot",
		Name:        "brainstorm",
		AgentType:   AgentTypeGrokBuild,
		Ephemeral:   true,
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agent/sessions", jsonBody(body))
	h.HandleSessionsRoot(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create status %d, body %s", rr.Code, rr.Body.String())
	}
	var created ChatSessionRecord
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.HasPrefix(created.WorkspaceID, "tmp-") {
		t.Fatalf("workspace_id = %q, want tmp-<sessionId>", created.WorkspaceID)
	}
	if created.WorkspaceID != "tmp-"+created.ID {
		t.Fatalf("workspace_id %q should be tmp-+session id %q", created.WorkspaceID, created.ID)
	}
	if created.Cwd == "" {
		t.Fatal("expected disposable cwd for tmp session")
	}
	if !strings.Contains(created.Cwd, "1agents-chat") {
		t.Fatalf("cwd %q should be under 1agents-chat", created.Cwd)
	}
	if st, err := os.Stat(created.Cwd); err != nil || !st.IsDir() {
		t.Fatalf("cwd should exist as directory: %v", err)
	}
	// Real projects row (kind=tmp) must resolve path.
	path, err := h.resolveWorkspacePath(created.WorkspaceID)
	if err != nil {
		t.Fatalf("resolveWorkspacePath: %v", err)
	}
	if path != created.Cwd {
		t.Fatalf("resolved path %q != cwd %q", path, created.Cwd)
	}
	// Seeded lightweight project config.
	for _, rel := range []string{
		filepath.Join(".grok", "config.toml"),
		"AGENTS.md",
		"Claude.md",
	} {
		p := filepath.Join(created.Cwd, rel)
		if st, err := os.Stat(p); err != nil || st.IsDir() {
			t.Fatalf("seeded file missing %s: %v", rel, err)
		}
	}
	_ = os.RemoveAll(created.Cwd)
}

func TestHandlerMethodNotAllowed(t *testing.T) {
	h, _ := newTestHandler(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/agent/sessions", nil)
	h.HandleSessionsRoot(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status %d, want 405", rr.Code)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/agent/agent-types", nil)
	h.HandleAgentTypes(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("agent-types POST status %d, want 405", rr.Code)
	}
}

func jsonBody(v any) *bytes.Reader {
	b, _ := json.Marshal(v)
	return bytes.NewReader(b)
}

// ── Issue-model task endpoints ──────────────────────────────────────────────

func seedTask(t *testing.T, h *Handler, ws string) Task {
	t.Helper()
	cfg, err := h.tasksStore.Load(ws)
	if err != nil {
		t.Fatalf("load tasks: %v", err)
	}
	task := Task{
		ID:          "task-1",
		Title:       "优化登录",
		Description: "初始描述",
		Status:      TaskStatusPending,
		IssueState:  IssueOpen,
	}
	cfg.Tasks = append(cfg.Tasks, task)
	if err := h.tasksStore.Save(ws, cfg); err != nil {
		t.Fatalf("save tasks: %v", err)
	}
	return task
}

func TestHandlerTaskGraph(t *testing.T) {
	h, _ := newTestHandler(t)
	ws := t.TempDir()
	// req (#1) is the upstream requirement; impl (#2) references it.
	cfg, err := h.tasksStore.Load(ws)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	cfg.Tasks = append(cfg.Tasks,
		Task{ID: "req", Title: "登录需求", Status: TaskStatusPending, IssueState: IssueOpen},
		Task{ID: "impl", Title: "实现登录", Description: "实现 #1", Status: TaskStatusPending, IssueState: IssueOpen,
			Links: []TaskLink{{Target: "req", Rel: LinkRelates}}},
	)
	if err := h.tasksStore.Save(ws, cfg); err != nil {
		t.Fatalf("save: %v", err)
	}

	// req's graph: no outgoing, one incoming backlink from impl.
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/agent/project-items/req/graph", nil)
	h.HandleTasksItem(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("graph status %d: %s", rr.Code, rr.Body.String())
	}
	var g LinkGraph
	if err := json.NewDecoder(rr.Body).Decode(&g); err != nil {
		t.Fatalf("decode graph: %v", err)
	}
	if len(g.Outgoing) != 0 {
		t.Fatalf("req outgoing = %+v, want none", g.Outgoing)
	}
	if len(g.Incoming) != 1 || g.Incoming[0].Task.ID != "impl" {
		t.Fatalf("req incoming = %+v, want one backlink from impl", g.Incoming)
	}

	// Unknown task → 404.
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/agent/project-items/nope/graph", nil)
	h.HandleTasksItem(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("missing graph status %d, want 404", rr.Code)
	}
}

func TestHandlerTaskGetPatchReply(t *testing.T) {
	h, _ := newTestHandler(t)
	ws := t.TempDir()
	seedTask(t, h, ws)

	// GET single task
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/agent/project-items/task-1", nil)
	h.HandleTasksItem(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get task status %d: %s", rr.Code, rr.Body.String())
	}
	var got Task
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Title != "优化登录" || got.Description != "初始描述" {
		t.Fatalf("wrong task: %+v", got)
	}

	// GET missing → 404
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/agent/project-items/nope", nil)
	h.HandleTasksItem(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("get missing status %d, want 404", rr.Code)
	}

	// POST a user reply
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/agent/project-items/task-1/replies",
		strings.NewReader(`{"text":"先调研","mode":"new","author":"scott"}`))
	h.HandleTasksItem(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("reply status %d: %s", rr.Code, rr.Body.String())
	}
	var reply Reply
	if err := json.NewDecoder(rr.Body).Decode(&reply); err != nil {
		t.Fatalf("decode reply: %v", err)
	}
	if reply.ID == "" || reply.Author.Name != "scott" || reply.Mode != ModeNewSession {
		t.Fatalf("wrong reply: %+v", reply)
	}

	// Empty text → 400
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/agent/project-items/task-1/replies",
		strings.NewReader(`{"text":"  "}`))
	h.HandleTasksItem(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("empty text status %d, want 400", rr.Code)
	}

	// PATCH description + close the issue
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/api/agent/project-items/task-1",
		strings.NewReader(`{"description":"新描述","issueState":"closed"}`))
	h.HandleTasksItem(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("patch status %d: %s", rr.Code, rr.Body.String())
	}
	var patched Task
	if err := json.NewDecoder(rr.Body).Decode(&patched); err != nil {
		t.Fatalf("decode patched: %v", err)
	}
	if patched.Description != "新描述" || patched.IssueState != IssueClosed {
		t.Fatalf("patch not applied: %+v", patched)
	}
	if len(patched.Replies) != 2 || patched.Replies[0].Text != "先调研" ||
		patched.ClosedBy == nil || patched.ClosedBy.TaskRunID == "" {
		t.Fatalf("timeline missing after patch: %+v", patched.Replies)
	}

	// Closed issue: new-session reply rejected with 422, pure comment OK
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/agent/project-items/task-1/replies",
		strings.NewReader(`{"text":"再来一轮","mode":"new"}`))
	h.HandleTasksItem(rr, req)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("closed new-session status %d, want 422", rr.Code)
	}
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/agent/project-items/task-1/replies",
		strings.NewReader(`{"text":"纯评论可以","mode":"pure_comment"}`))
	h.HandleTasksItem(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("closed pure-comment status %d, want 200: %s", rr.Code, rr.Body.String())
	}

	// Invalid issueState → 400
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/api/agent/project-items/task-1",
		strings.NewReader(`{"issueState":"banana"}`))
	h.HandleTasksItem(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad issueState status %d, want 400", rr.Code)
	}
}

// TestManualStatusOverrideLeavesAuditTrail covers #132's "限制为人工 override 并留痕":
// a manual PATCH to a terminal status (the human-override lane) records an
// append-only audit note so the override is never silent.
func TestManualStatusOverrideLeavesAuditTrail(t *testing.T) {
	h, _ := newTestHandler(t)
	ws := t.TempDir()
	seedTask(t, h, ws) // starts pending

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/agent/project-items/task-1",
		strings.NewReader(`{"status":"completed"}`))
	h.HandleTasksItem(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("patch status %d: %s", rr.Code, rr.Body.String())
	}
	var patched Task
	if err := json.NewDecoder(rr.Body).Decode(&patched); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if patched.Status != TaskStatusCompleted || patched.CompletedAt == nil {
		t.Fatalf("manual override should complete the task: %+v", patched)
	}
	var audits int
	for _, rep := range patched.Replies {
		if strings.Contains(rep.Text, "手动 override") {
			audits++
		}
	}
	if audits != 1 {
		t.Fatalf("expected exactly one audit reply, got %d: %+v", audits, patched.Replies)
	}
}

// TestHandlerPatchRename covers the sidebar "重命名会话" path: PATCH
// /api/agent/sessions/{id} with {"name":"..."} must persist the title and
// set user_named so subsequent list/get won't overwrite it with AI titles.
func TestHandlerPatchRename(t *testing.T) {
	h, s := newTestHandler(t)

	rec := ChatSessionRecord{
		ID:          "rename-1",
		WorkspaceID: "ws-1",
		Name:        "新建会话",
		AgentType:   AgentTypeClaudecode,
	}
	if err := s.Add(rec); err != nil {
		t.Fatalf("Add: %v", err)
	}

	const newName = "我的项目会话"
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/agent/sessions/"+rec.ID,
		strings.NewReader(`{"name":"`+newName+`"}`))
	h.HandleSessionsItem(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("patch status %d: %s", rr.Code, rr.Body.String())
	}
	var got ChatSessionRecord
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Name != newName {
		t.Fatalf("name = %q, want %q", got.Name, newName)
	}
	if !got.UserNamed {
		t.Fatalf("user_named should be true after rename")
	}

	// Empty / whitespace-only name is rejected.
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/api/agent/sessions/"+rec.ID,
		strings.NewReader(`{"name":"   "}`))
	h.HandleSessionsItem(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("empty name status %d, want 400", rr.Code)
	}

	// Name is unchanged after the rejected patch.
	stored, ok, err := s.Get(rec.ID)
	if err != nil || !ok {
		t.Fatalf("Get after reject: ok=%v err=%v", ok, err)
	}
	if stored.Name != newName {
		t.Fatalf("name mutated by rejected patch: %q", stored.Name)
	}
}

// TestSessionUserRenamePreservedByList covers #94: a session whose name the
// user set to something that matches the default "会话"-suffix pattern (and
// therefore would have been overwritten by the list endpoint's AI title
// auto-resolution before the fix) must survive a subsequent GET and list.
// The handler reads wsPath from the workspace registry (empty in tests), and
// resolveAcpSessionTitle returns the existing name as a fallback, so the
// title is left alone — but user_named must gate the entire branch.
func TestSessionUserRenamePreservedByList(t *testing.T) {
	h, s := newTestHandler(t)

	// Seed a default-named session with an AcpSessionID so the AI-title
	// auto-resolution branch is on the code path.
	rec := ChatSessionRecord{
		ID:           "user-1",
		WorkspaceID:  "ws-1",
		Name:         "新建会话",
		AgentType:    AgentTypeClaudecode,
		AcpSessionID: "uuid-1",
	}
	if err := s.Add(rec); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// User renames the session to a value that still matches the default
	// suffix pattern. UpdateName sets user_named=1.
	const userName = "我的项目会话"
	if err := s.UpdateName(rec.ID, userName); err != nil {
		t.Fatalf("UpdateName: %v", err)
	}

	// GET single session: handler must not overwrite the user's title.
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/agent/sessions/"+rec.ID, nil)
	h.HandleSessionsItem(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get status %d: %s", rr.Code, rr.Body.String())
	}
	var got ChatSessionRecord
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Name != userName {
		t.Fatalf("get overwrote user-renamed name: %q (want %q)", got.Name, userName)
	}
	if !got.UserNamed {
		t.Fatalf("get should surface user_named=true, got false")
	}

	// LIST workspace sessions: same protection applies.
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/agent/sessions?workspace_id=ws-1", nil)
	h.HandleSessionsRoot(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list status %d", rr.Code)
	}
	var listed []ChatSessionRecord
	if err := json.NewDecoder(rr.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listed) != 1 || listed[0].Name != userName {
		t.Fatalf("list overwrote user-renamed name: %+v", listed)
	}
	if !listed[0].UserNamed {
		t.Fatalf("list should surface user_named=true, got false")
	}
}

// TestSessionDefaultNameStillAutoTitle covers the backward-compat side of #94:
// a session the user has NOT renamed keeps auto AI-title resolution (gated
// only by isDefaultSessionName). In the test environment wsPath is empty so
// resolveAcpSessionTitle falls back to the existing name; we assert the
// default name survives unchanged and user_named stays false.
func TestSessionDefaultNameStillAutoTitle(t *testing.T) {
	h, s := newTestHandler(t)

	rec := ChatSessionRecord{
		ID:           "auto-1",
		WorkspaceID:  "ws-1",
		Name:         "新建会话",
		AgentType:    AgentTypeClaudecode,
		AcpSessionID: "uuid-auto",
	}
	if err := s.Add(rec); err != nil {
		t.Fatalf("Add: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/agent/sessions?workspace_id=ws-1", nil)
	h.HandleSessionsRoot(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list status %d", rr.Code)
	}
	var listed []ChatSessionRecord
	if err := json.NewDecoder(rr.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("got %d records, want 1", len(listed))
	}
	if listed[0].UserNamed {
		t.Fatalf("default-named session should keep user_named=false after list")
	}
	if listed[0].Name != "新建会话" {
		t.Fatalf("default-named session was overwritten unexpectedly: %q", listed[0].Name)
	}
}

func TestProjectItemsExecutorMatrix(t *testing.T) {
	h, _ := newTestHandler(t)
	// Seed a workspace path the handler can resolve via meta EnsureProject.
	// Use empty resolve — create without workspace may 404; use personal path.
	// Build create body with illegal agent+user via explicit executor.
	body := map[string]any{
		"workspace_id": "default",
		"title":        "bad agent+user",
		"executor":     "agent",
		"assignee":     "user",
	}
	raw, _ := json.Marshal(body)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agent/project-items", bytes.NewReader(raw))
	h.HandleTasksRoot(rr, req)
	// Either 400 (matrix) or 404 (workspace) — matrix should win if workspace resolves.
	// When workspace is missing we get 404 first after matrix in current code order
	// matrix runs before workspace resolve — so expect 400.
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("agent+user: status %d body %s, want 400", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "executor=agent cannot use assignee=user") {
		t.Fatalf("agent+user message: %s", rr.Body.String())
	}

	// function without type
	body2 := map[string]any{
		"workspace_id": "default",
		"title":        "bad fn",
		"executor":     "function",
	}
	raw2, _ := json.Marshal(body2)
	rr2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/agent/project-items", bytes.NewReader(raw2))
	h.HandleTasksRoot(rr2, req2)
	if rr2.Code != http.StatusBadRequest {
		t.Fatalf("function no type: status %d body %s", rr2.Code, rr2.Body.String())
	}
}
