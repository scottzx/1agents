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
	tasksStore := NewTasksStore()
	acpxClient := NewAcpxClient(38082)
	workspacesFn := func() ([]string, error) {
		return []string{}, nil
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
