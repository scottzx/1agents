package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/localtoken"
	"github.com/scottzx/1Agents/backend/internal/meta"
)

func TestActivityAndTurnQueryAPIs(t *testing.T) {
	h, db, wsID, _, turn := mutationAttributionRig(t)
	token := localtoken.SessionToken("session-1")
	var createdIDs []string
	for _, title := range []string{"one", "two"} {
		rr := httptest.NewRecorder()
		h.HandleTasksRoot(rr, attributedCreateRequest(wsID, title, "session-1", token))
		if rr.Code != http.StatusOK {
			t.Fatalf("create %s: %d %s", title, rr.Code, rr.Body.String())
		}
		var response map[string]any
		_ = json.NewDecoder(rr.Body).Decode(&response)
		createdIDs = append(createdIDs, response["id"].(string))
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/agent/activity?workspace_id="+wsID+"&session_id=session-1&limit=10", nil)
	h.HandleProjectActivity(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("activity status=%d: %s", rr.Code, rr.Body.String())
	}
	var activity meta.ProjectActivityPage
	if err := json.NewDecoder(rr.Body).Decode(&activity); err != nil {
		t.Fatalf("decode activity: %v", err)
	}
	if len(activity.Items) != 1 || activity.Items[0].Summary != "创建 2 个 Tasks" ||
		activity.Items[0].TurnID != turn.ID {
		t.Fatalf("activity=%+v", activity)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet,
		"/api/agent/activity?workspace_id="+wsID+"&target_type=project_item&target_id="+createdIDs[0], nil)
	h.HandleProjectActivity(rr, req)
	var itemActivity meta.ProjectActivityPage
	_ = json.NewDecoder(rr.Body).Decode(&itemActivity)
	if rr.Code != http.StatusOK || len(itemActivity.Items) != 1 ||
		itemActivity.Items[0].Count != 1 || itemActivity.Items[0].Targets[0].ID != createdIDs[0] {
		t.Fatalf("item activity status=%d page=%+v", rr.Code, itemActivity)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet,
		"/api/agent/turns?workspace_id="+wsID+"&session_id=session-1&status=running", nil)
	h.HandleTurns(rr, req)
	var turns meta.AgentTurnPage
	_ = json.NewDecoder(rr.Body).Decode(&turns)
	if rr.Code != http.StatusOK || len(turns.Items) != 1 || turns.Items[0].ID != turn.ID {
		t.Fatalf("turns status=%d page=%+v", rr.Code, turns)
	}

	for _, path := range []string{
		"/api/agent/activity?workspace_id=" + wsID + "&cursor=invalid",
		"/api/agent/turns?workspace_id=" + wsID + "&limit=0",
	} {
		rr = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, path, nil)
		if strings.Contains(path, "/activity") {
			h.HandleProjectActivity(rr, req)
		} else {
			h.HandleTurns(rr, req)
		}
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("%s status=%d, want 400", path, rr.Code)
		}
	}

	if err := db.EnsureProject("project-2", "Project 2", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := meta.NewSessionStore(db).Add(meta.ChatSessionRecord{
		ID: "session-2", WorkspaceID: "project-2", AgentType: "codex",
	}); err != nil {
		t.Fatal(err)
	}
	foreign, _, err := h.turnStore.Create(meta.AgentTurn{
		ProjectID: "project-2", SessionID: "session-2", ClientRequestID: "foreign",
	})
	if err != nil {
		t.Fatal(err)
	}
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet,
		"/api/agent/activity?workspace_id="+wsID+"&turn_id="+foreign.ID, nil)
	h.HandleProjectActivity(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("foreign Turn status=%d, want 403: %s", rr.Code, rr.Body.String())
	}
}
