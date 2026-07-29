package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/localtoken"
	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/workspace"
)

func mutationAttributionRig(t *testing.T) (*Handler, *meta.DB, string, string, meta.AgentTurn) {
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
	turnStore := meta.NewAgentTurnStore(db)
	client := NewAcpxClient(38082, turnStore)
	scheduler := NewScheduler(tasksStore, func() ([]WorkspaceRef, error) { return nil, nil })
	h := NewHandler(store, tasksStore, client, scheduler, NewCatalogStore(), "http://127.0.0.1:0")
	turn, _, err := turnStore.Create(meta.AgentTurn{
		ProjectID:       wsID,
		SessionID:       "session-1",
		ClientRequestID: "request-1",
		AgentType:       "codex",
		PromptText:      "create three tasks",
	})
	if err != nil {
		t.Fatalf("Create Turn: %v", err)
	}
	turn, err = turnStore.Transition(turn.ID, meta.AgentTurnTransition{Status: meta.AgentTurnRunning})
	if err != nil {
		t.Fatalf("start Turn: %v", err)
	}
	return h, db, wsID, wsPath, turn
}

func attributedCreateRequest(wsID, title, sessionID, token string) *http.Request {
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/agent/project-items",
		strings.NewReader(`{"workspace_id":"`+wsID+`","title":"`+title+`","type":"task"}`),
	)
	if sessionID != "" {
		req.Header.Set("X-OneAgents-Session-ID", sessionID)
		req.Header.Set("X-OneAgents-Session-Token", token)
	}
	req.Header.Set("X-OneAgents-Origin", "cli")
	return req
}

func TestAttributedTurnCreatesThreeProjectItemEvents(t *testing.T) {
	h, db, wsID, _, turn := mutationAttributionRig(t)
	sessionToken := localtoken.SessionToken("session-1")

	eventIDs := map[string]bool{}
	for _, title := range []string{"one", "two", "three"} {
		rr := httptest.NewRecorder()
		h.HandleTasksRoot(rr, attributedCreateRequest(wsID, title, "session-1", sessionToken))
		if rr.Code != http.StatusOK {
			t.Fatalf("create %s status %d: %s", title, rr.Code, rr.Body.String())
		}
		var response map[string]any
		if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
			t.Fatalf("decode create response: %v", err)
		}
		if response["sessionId"] != "session-1" || response["turnId"] != turn.ID {
			t.Fatalf("wrong attribution response: %v", response)
		}
		eventID, _ := response["eventId"].(string)
		if eventID == "" || eventIDs[eventID] {
			t.Fatalf("eventId is empty or duplicated: %q", eventID)
		}
		eventIDs[eventID] = true
	}

	page, err := meta.NewProjectEventStore(db).List(meta.ProjectEventListOptions{
		ProjectID:  wsID,
		TurnID:     turn.ID,
		TargetType: "project_item",
		Limit:      20,
	})
	if err != nil {
		t.Fatalf("List ProjectEvents: %v", err)
	}
	if len(page.Items) != 3 {
		t.Fatalf("project-item Events=%d, want 3: %+v", len(page.Items), page.Items)
	}
	for _, event := range page.Items {
		if event.TurnID != turn.ID || event.SessionID != "session-1" ||
			event.ActorKind != "agent" || event.ActorName != "codex" ||
			event.Origin != "cli" || event.EventType != "project_item.create" {
			t.Fatalf("wrong Event attribution: %+v", event)
		}
	}
	rec, ok, err := h.store.Get("session-1")
	if err != nil || !ok || rec.TaskID != "" {
		t.Fatalf("Session.task_id changed: ok=%v task=%q err=%v", ok, rec.TaskID, err)
	}
}

func TestMutationAttributionSecurityAndOutsideCLI(t *testing.T) {
	h, db, wsID, _, running := mutationAttributionRig(t)

	rr := httptest.NewRecorder()
	h.HandleTasksRoot(rr, attributedCreateRequest(wsID, "outside", "", ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("outside CLI create status %d: %s", rr.Code, rr.Body.String())
	}
	var outside map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&outside)
	if outside["eventId"] == "" || outside["sessionId"] != nil || outside["turnId"] != nil {
		t.Fatalf("outside CLI response attribution: %v", outside)
	}
	event, ok, err := meta.NewProjectEventStore(db).Get(outside["eventId"].(string))
	if err != nil || !ok || event.TurnID != "" || event.SessionID != "" || event.Origin != "cli" {
		t.Fatalf("outside CLI Event: ok=%v event=%+v err=%v", ok, event, err)
	}

	rr = httptest.NewRecorder()
	h.HandleTasksRoot(rr, attributedCreateRequest(wsID, "forged-session", "session-1", "bad"))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("forged Session status=%d, want 401: %s", rr.Code, rr.Body.String())
	}

	req := attributedCreateRequest(wsID, "forged-turn", "session-1", localtoken.SessionToken("session-1"))
	req.Header.Set("X-OneAgents-Turn-ID", "another-turn")
	rr = httptest.NewRecorder()
	h.HandleTasksRoot(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("forged Turn status=%d, want 400: %s", rr.Code, rr.Body.String())
	}

	ws2Path := t.TempDir()
	wh := workspace.NewHandler()
	cfg, err := wh.LoadWorkspacesConfig()
	if err != nil {
		t.Fatalf("LoadWorkspacesConfig: %v", err)
	}
	cfg.Workspaces = append(cfg.Workspaces, workspace.Workspace{
		ID: "project-2", Name: "Project 2", Path: ws2Path, Status: "active",
	})
	if err := wh.SaveWorkspacesConfig(cfg); err != nil {
		t.Fatalf("Save project 2: %v", err)
	}
	if err := db.EnsureProject("project-2", "Project 2", ws2Path); err != nil {
		t.Fatalf("Ensure project 2: %v", err)
	}
	rr = httptest.NewRecorder()
	h.HandleTasksRoot(rr, attributedCreateRequest(
		"project-2", "cross-project", "session-1", localtoken.SessionToken("session-1"),
	))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("cross-project status=%d, want 403: %s", rr.Code, rr.Body.String())
	}

	if _, err := h.turnStore.Transition(running.ID, meta.AgentTurnTransition{
		Status: meta.AgentTurnCompleted,
	}); err != nil {
		t.Fatalf("complete running Turn: %v", err)
	}
	rr = httptest.NewRecorder()
	h.HandleTasksRoot(rr, attributedCreateRequest(
		wsID, "no-running-turn", "session-1", localtoken.SessionToken("session-1"),
	))
	if rr.Code != http.StatusConflict {
		t.Fatalf("no-running Turn status=%d, want 409: %s", rr.Code, rr.Body.String())
	}
}

func TestAttributedMilestoneAndDependencyEvents(t *testing.T) {
	h, db, wsID, _, turn := mutationAttributionRig(t)
	token := localtoken.SessionToken("session-1")

	create := func(body string) map[string]any {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/agent/project-items", strings.NewReader(body))
		req.Header.Set("X-OneAgents-Session-ID", "session-1")
		req.Header.Set("X-OneAgents-Session-Token", token)
		req.Header.Set("X-OneAgents-Origin", "mcp")
		rr := httptest.NewRecorder()
		h.HandleTasksRoot(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("create status %d: %s", rr.Code, rr.Body.String())
		}
		var response map[string]any
		_ = json.NewDecoder(rr.Body).Decode(&response)
		return response
	}
	base := create(`{"workspace_id":"` + wsID + `","title":"base","type":"task"}`)
	baseID, _ := base["id"].(string)
	dependent := create(`{"workspace_id":"` + wsID + `","title":"dependent","type":"task","dependsOn":["` + baseID + `"]}`)
	dependentID, _ := dependent["id"].(string)

	deps, err := meta.NewProjectEventStore(db).List(meta.ProjectEventListOptions{
		ProjectID: wsID, TurnID: turn.ID, TargetType: "dependency", Limit: 10,
	})
	if err != nil || len(deps.Items) != 1 {
		t.Fatalf("dependency Events=%+v err=%v", deps, err)
	}
	if deps.Items[0].TargetID != dependentID+":"+baseID ||
		deps.Items[0].Operation != "link" || deps.Items[0].Origin != "mcp" {
		t.Fatalf("dependency Event=%+v", deps.Items[0])
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/agent/milestones",
		strings.NewReader(`{"workspace_id":"`+wsID+`","bump":"minor"}`),
	)
	req.Header.Set("X-OneAgents-Session-ID", "session-1")
	req.Header.Set("X-OneAgents-Session-Token", token)
	req.Header.Set("X-OneAgents-Origin", "mcp")
	rr := httptest.NewRecorder()
	h.HandleMilestonesRoot(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create milestone status %d: %s", rr.Code, rr.Body.String())
	}
	var milestone map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&milestone)
	if milestone["turnId"] != turn.ID || milestone["sessionId"] != "session-1" || milestone["eventId"] == "" {
		t.Fatalf("milestone response=%v", milestone)
	}
	milestones, err := meta.NewProjectEventStore(db).List(meta.ProjectEventListOptions{
		ProjectID: wsID, TurnID: turn.ID, TargetType: "milestone", Limit: 10,
	})
	if err != nil || len(milestones.Items) != 1 ||
		milestones.Items[0].EventType != "milestone.create" {
		t.Fatalf("milestone Events=%+v err=%v", milestones, err)
	}
}

func TestInjectSessionAttributionIntoProjectItemsMCP(t *testing.T) {
	raw := json.RawMessage(`[
		{"name":"project_items","env":[{"name":"EXISTING","value":"yes"}]},
		{"name":"other","env":[]}
	]`)
	updated := injectSessionAttribution(raw, "session-1", "signed")
	var servers []struct {
		Name string              `json:"name"`
		Env  []map[string]string `json:"env"`
	}
	if err := json.Unmarshal(updated, &servers); err != nil {
		t.Fatalf("unmarshal injected MCP: %v", err)
	}
	got := map[string]string{}
	for _, entry := range servers[0].Env {
		got[entry["name"]] = entry["value"]
	}
	if got["EXISTING"] != "yes" || got["ONEAGENTS_SESSION_ID"] != "session-1" ||
		got["ONEAGENTS_SESSION_TOKEN"] != "signed" {
		t.Fatalf("project-items env = %v", got)
	}
	if len(servers[1].Env) != 0 {
		t.Fatalf("unrelated MCP env changed: %+v", servers[1].Env)
	}
}
