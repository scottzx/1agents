package agent

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	return NewHandler(s, tasksStore, acpxClient, scheduler), s
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

func TestHandlerTaskGetPatchReply(t *testing.T) {
	h, _ := newTestHandler(t)
	ws := t.TempDir()
	seedTask(t, h, ws)

	// GET single task
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/agent/tasks/task-1", nil)
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
	req = httptest.NewRequest(http.MethodGet, "/api/agent/tasks/nope", nil)
	h.HandleTasksItem(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("get missing status %d, want 404", rr.Code)
	}

	// POST a user reply
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/agent/tasks/task-1/replies",
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
	req = httptest.NewRequest(http.MethodPost, "/api/agent/tasks/task-1/replies",
		strings.NewReader(`{"text":"  "}`))
	h.HandleTasksItem(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("empty text status %d, want 400", rr.Code)
	}

	// PATCH description + close the issue
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/api/agent/tasks/task-1",
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
	if len(patched.Replies) != 1 || patched.Replies[0].Text != "先调研" {
		t.Fatalf("timeline missing after patch: %+v", patched.Replies)
	}

	// Closed issue: new-session reply rejected with 422, pure comment OK
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/agent/tasks/task-1/replies",
		strings.NewReader(`{"text":"再来一轮","mode":"new"}`))
	h.HandleTasksItem(rr, req)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("closed new-session status %d, want 422", rr.Code)
	}
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/agent/tasks/task-1/replies",
		strings.NewReader(`{"text":"纯评论可以","mode":"pure_comment"}`))
	h.HandleTasksItem(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("closed pure-comment status %d, want 200: %s", rr.Code, rr.Body.String())
	}

	// Invalid issueState → 400
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/api/agent/tasks/task-1",
		strings.NewReader(`{"issueState":"banana"}`))
	h.HandleTasksItem(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad issueState status %d, want 400", rr.Code)
	}
}
