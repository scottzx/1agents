package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/localtoken"
	"github.com/scottzx/1Agents/backend/internal/meta"
)

func createAuditedTask(t *testing.T, h *Handler, wsID string) Task {
	t.Helper()
	rr := httptest.NewRecorder()
	h.HandleTasksRoot(rr, attributedCreateRequest(
		wsID, "audited task", "session-1", localtoken.SessionToken("session-1"),
	))
	if rr.Code != http.StatusOK {
		t.Fatalf("create task: %d %s", rr.Code, rr.Body.String())
	}
	var task Task
	if err := json.NewDecoder(rr.Body).Decode(&task); err != nil {
		t.Fatal(err)
	}
	return task
}

func TestInteractiveDoneCrossesCompletionGateWithTaskRunEvidence(t *testing.T) {
	h, db, wsID, wsPath, turn := mutationAttributionRig(t)
	task := createAuditedTask(t, h, wsID)
	if _, err := h.turnStore.Transition(turn.ID, meta.AgentTurnTransition{Status: meta.AgentTurnCompleted}); err != nil {
		t.Fatal(err)
	}

	h.acpxClient.handleTaskSessionDone(wsPath, task.ID, "session-1", turn.ID, "all checks passed", false, h.tasksStore)

	got, ok, err := h.tasksStore.GetTask(task.ID)
	if err != nil || !ok {
		t.Fatalf("GetTask ok=%v err=%v", ok, err)
	}
	if got.Status != TaskStatusCompleted || got.ClosedBy == nil {
		t.Fatalf("completion audit missing: status=%s closedBy=%+v", got.Status, got.ClosedBy)
	}
	runs, err := meta.NewTaskRunStore(db).ListByTask(task.ID)
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs=%+v err=%v", runs, err)
	}
	run := runs[0]
	if run.OriginTurnID != turn.ID || run.Status != meta.TaskRunCompleted ||
		len(run.Evidence) != 1 || got.ClosedBy.TaskRunID != run.ID {
		t.Fatalf("run=%+v closedBy=%+v", run, got.ClosedBy)
	}
	if len(got.Replies) == 0 || got.Replies[len(got.Replies)-1].Author.Name != "completion-gate" {
		t.Fatalf("completion audit timeline reply missing: %+v", got.Replies)
	}

	rr := httptest.NewRecorder()
	h.HandleTasksItem(rr, httptest.NewRequest(
		http.MethodGet, "/api/agent/project-items/"+task.ID+"/runs", nil,
	))
	if rr.Code != http.StatusOK {
		t.Fatalf("runs endpoint: %d %s", rr.Code, rr.Body.String())
	}
}

func TestFinalAnswerCannotCompleteWhenTaskRunAuditIsUnavailable(t *testing.T) {
	h, db, wsID, wsPath, turn := mutationAttributionRig(t)
	task := createAuditedTask(t, h, wsID)
	if _, err := h.turnStore.Transition(turn.ID, meta.AgentTurnTransition{Status: meta.AgentTurnCompleted}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL().Exec(`DROP TABLE task_runs`); err != nil {
		t.Fatal(err)
	}

	h.acpxClient.handleTaskSessionDone(wsPath, task.ID, "session-1", turn.ID, "I am done", false, h.tasksStore)

	got, ok, err := h.tasksStore.GetTask(task.ID)
	if err != nil || !ok {
		t.Fatalf("GetTask ok=%v err=%v", ok, err)
	}
	if got.Status == TaskStatusCompleted || got.ClosedBy != nil {
		t.Fatalf("final answer bypassed completion gate: status=%s closedBy=%+v", got.Status, got.ClosedBy)
	}
	if got.Status != TaskStatusFailed {
		t.Fatalf("status=%s, want failed audit gate", got.Status)
	}
}
