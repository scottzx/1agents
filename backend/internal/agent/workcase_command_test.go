package agent

// HTTP-level Command Gateway tests for the WorkCase surface (#323):
// idempotent submissions, the /commands execution audit endpoint, and the
// agent-attribution permission policy (Agent 路径只能走统一 Command，且不可
// 静默覆盖人工决策).

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/commandbus"
	"github.com/scottzx/1Agents/backend/internal/localtoken"
	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/workspace"
)

// workCaseAgentRig extends workCaseRig with one registered session and a
// running turn so requests can be attributed to an agent actor.
func workCaseAgentRig(t *testing.T) (*Handler, *meta.DB, string, meta.AgentTurn) {
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
	if err := store.Add(meta.ChatSessionRecord{
		ID:          "session-1",
		WorkspaceID: wsID,
		AgentType:   "codex",
	}); err != nil {
		t.Fatalf("Add session: %v", err)
	}
	tasksStore := meta.NewTaskStore(db)
	scheduler := NewScheduler(tasksStore, func() ([]WorkspaceRef, error) { return nil, nil })
	h := NewHandler(store, tasksStore, nil, scheduler, NewCatalogStore(), "http://127.0.0.1:0")
	turnStore := meta.NewAgentTurnStore(db)
	turn, _, err := turnStore.Create(meta.AgentTurn{
		ProjectID:       wsID,
		SessionID:       "session-1",
		ClientRequestID: "request-1",
		AgentType:       "codex",
		PromptText:      "work on the case",
	})
	if err != nil {
		t.Fatalf("Create Turn: %v", err)
	}
	turn, err = turnStore.Transition(turn.ID, meta.AgentTurnTransition{Status: meta.AgentTurnRunning})
	if err != nil {
		t.Fatalf("start Turn: %v", err)
	}
	return h, db, wsID, turn
}

// agentCaseRequest attributes the request to the rig's running agent turn.
func agentCaseRequest(method, path string, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("X-OneAgents-Session-ID", "session-1")
	req.Header.Set("X-OneAgents-Session-Token", localtoken.SessionToken("session-1"))
	req.Header.Set("X-OneAgents-Origin", "mcp")
	return req
}

// ── idempotency over HTTP ───────────────────────────────────────────────────

func TestWorkCaseHTTPIdempotentCreate(t *testing.T) {
	h, db, wsID, _ := workCaseRig(t)

	body := `{"workspace_id":"` + wsID + `","title":"幂等 Case"}`
	req := httptest.NewRequest(http.MethodPost, "/api/agent/work-cases", strings.NewReader(body))
	req.Header.Set("Idempotency-Key", "http-key-1")
	rr := httptest.NewRecorder()
	h.HandleWorkCasesRoot(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create status %d: %s", rr.Code, rr.Body.String())
	}
	var first map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&first); err != nil {
		t.Fatal(err)
	}

	// Duplicate submissions under the same key return the identical result.
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/agent/work-cases", strings.NewReader(body))
		req.Header.Set("Idempotency-Key", "http-key-1")
		rr := httptest.NewRecorder()
		h.HandleWorkCasesRoot(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("replay %d status %d: %s", i, rr.Code, rr.Body.String())
		}
		var again map[string]any
		if err := json.NewDecoder(rr.Body).Decode(&again); err != nil {
			t.Fatal(err)
		}
		if again["id"] != first["id"] || again["eventId"] != first["eventId"] || again["version"] != first["version"] {
			t.Fatalf("replay %d differs:\n got %v\nwant %v", i, again, first)
		}
		if again["replayed"] != true {
			t.Fatalf("replay %d missing replayed flag: %v", i, again)
		}
	}

	// The effect happened exactly once; a fresh key creates a new case.
	cases, err := meta.NewWorkCaseStore(db).List(wsID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 1 {
		t.Fatalf("cases=%d, want 1", len(cases))
	}
	rr = doWorkCase(t, h, http.MethodPost, "/api/agent/work-cases", map[string]any{
		"workspace_id": wsID, "title": "第二个 Case", "idempotencyKey": "http-key-2",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("second create status %d: %s", rr.Code, rr.Body.String())
	}
	cases, err = meta.NewWorkCaseStore(db).List(wsID, "")
	if err != nil || len(cases) != 2 {
		t.Fatalf("cases=%d err=%v, want 2", len(cases), err)
	}
}

// ── execution audit endpoint ────────────────────────────────────────────────

func TestWorkCaseHTTPCommandAuditEndpoint(t *testing.T) {
	h, _, wsID, _ := workCaseRig(t)
	created := createWorkCaseHTTP(t, h, wsID, "audit case")
	id := created["id"].(string)

	// One success, one phase advance, one rejected conflict.
	if rr := doWorkCase(t, h, http.MethodPost, "/api/agent/work-cases/"+id+"/phase", map[string]any{
		"expectedVersion": 1, "currentPhase": "phase-a",
	}); rr.Code != http.StatusOK {
		t.Fatalf("set phase status %d: %s", rr.Code, rr.Body.String())
	}
	if rr := doWorkCase(t, h, http.MethodPatch, "/api/agent/work-cases/"+id, map[string]any{
		"expectedVersion": 1, "objective": "stale",
	}); rr.Code != http.StatusConflict {
		t.Fatalf("stale patch status %d, want 409", rr.Code)
	}

	rr := doWorkCase(t, h, http.MethodGet, "/api/agent/work-cases/"+id+"/commands", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("commands status %d: %s", rr.Code, rr.Body.String())
	}
	var page struct {
		Items []struct {
			Contract   string `json:"contract"`
			Status     string `json:"status"`
			ActorKind  string `json:"actorKind"`
			ActorName  string `json:"actorName"`
			TargetID   string `json:"targetId"`
			ErrorCode  string `json:"errorCode"`
			NewVersion int    `json:"newVersion"`
			DurationMS int64  `json:"durationMs"`
			CreatedAt  string `json:"createdAt"`
			IdempotKey string `json:"idempotencyKey"`
		} `json:"items"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 3 {
		t.Fatalf("audit items=%d, want 3: %+v", len(page.Items), page.Items)
	}
	// Newest first: rejected conflict, phase advance, create.
	want := []struct {
		contract string
		status   string
		errCode  string
		version  int
	}{
		{"workcase.update", "rejected", "version_conflict", 0},
		{"workcase.set_phase", "succeeded", "", 2},
		{"workcase.create", "succeeded", "", 1},
	}
	for i, w := range want {
		got := page.Items[i]
		if got.Contract != w.contract || got.Status != w.status || got.ErrorCode != w.errCode ||
			got.NewVersion != w.version {
			t.Fatalf("audit[%d]=%+v, want %+v", i, got, w)
		}
		if got.ActorKind != "user" || got.TargetID != id || got.CreatedAt == "" || got.DurationMS < 0 {
			t.Fatalf("audit[%d] missing actor/target/duration: %+v", i, got)
		}
	}
}

// ── agent attribution: commands only, human decisions protected ────────────

func TestWorkCaseHTTPAgentPermissionPolicies(t *testing.T) {
	h, db, wsID, turn := workCaseAgentRig(t)
	created := createWorkCaseHTTP(t, h, wsID, "agent 参与")
	id := created["id"].(string)

	rr := httptest.NewRecorder()
	h.HandleWorkCasesItem(rr, agentCaseRequest(http.MethodPost,
		"/api/agent/work-cases/"+id+"/transition",
		`{"status":"closed","expectedVersion":1,"reason":"agent 想关单"}`))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("agent terminal transition status %d, want 403: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "permission_denied") {
		t.Fatalf("agent transition body missing permission_denied code: %s", rr.Body.String())
	}

	// Agent may not delete.
	rr = httptest.NewRecorder()
	h.HandleWorkCasesItem(rr, agentCaseRequest(http.MethodDelete,
		"/api/agent/work-cases/"+id+"?workspace_id="+wsID, ""))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("agent delete status %d, want 403: %s", rr.Code, rr.Body.String())
	}

	// Agent may suspend — through the same gateway, attributed to the turn.
	rr = httptest.NewRecorder()
	h.HandleWorkCasesItem(rr, agentCaseRequest(http.MethodPost,
		"/api/agent/work-cases/"+id+"/transition",
		`{"status":"suspended","expectedVersion":1,"reason":"等待人工"}`))
	if rr.Code != http.StatusOK {
		t.Fatalf("agent suspend status %d, want 200: %s", rr.Code, rr.Body.String())
	}
	var suspended map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&suspended); err != nil {
		t.Fatal(err)
	}
	if suspended["status"] != "suspended" || suspended["sessionId"] != "session-1" || suspended["turnId"] != turn.ID {
		t.Fatalf("agent suspend result mismatch: %v", suspended)
	}

	// The case state and its audit trail agree.
	got, ok, err := meta.NewWorkCaseStore(db).Get(id)
	if err != nil || !ok || got.Status != meta.CaseStatusSuspended {
		t.Fatalf("case after agent suspend: ok=%v err=%v case=%+v", ok, err, got)
	}
	executions, err := h.commandBus.ListExecutions(commandbus.ExecutionFilter{WorkspaceID: wsID, TargetID: id})
	if err != nil {
		t.Fatal(err)
	}
	// Newest first: suspend, denied delete, denied close, user create.
	if len(executions) != 4 {
		t.Fatalf("executions=%d, want 4 (create, denied close, denied delete, suspend)", len(executions))
	}
	denied := executions[2] // the terminal-transition denial
	if denied.Status != "rejected" || denied.ErrorCode != "permission_denied" ||
		denied.ActorKind != "agent" || denied.ActorName != "codex" ||
		denied.SessionID != "session-1" || denied.TurnID != turn.ID ||
		denied.Origin != "mcp" {
		t.Fatalf("denied execution attribution mismatch: %+v", denied)
	}
	if del := executions[1]; del.Status != "rejected" || del.ErrorCode != "permission_denied" ||
		del.Contract != "workcase.delete" {
		t.Fatalf("delete denial mismatch: %+v", del)
	}
	suspend := executions[0]
	if suspend.Status != "succeeded" || suspend.ActorKind != "agent" || suspend.TurnID != turn.ID {
		t.Fatalf("suspend execution mismatch: %+v", suspend)
	}
	if create := executions[3]; create.Status != "succeeded" || create.ActorKind != "user" {
		t.Fatalf("create execution mismatch: %+v", create)
	}

	// Phase advance by the agent is also command-gated and attributed.
	rr = httptest.NewRecorder()
	h.HandleWorkCasesItem(rr, agentCaseRequest(http.MethodPost,
		"/api/agent/work-cases/"+id+"/phase",
		`{"currentPhase":"waiting-human","expectedVersion":2}`))
	if rr.Code != http.StatusOK {
		t.Fatalf("agent set phase status %d: %s", rr.Code, rr.Body.String())
	}
	got, _, _ = meta.NewWorkCaseStore(db).Get(id)
	if got.CurrentPhase != "waiting-human" || got.Version != 3 {
		t.Fatalf("case after agent phase: %+v", got)
	}
}
