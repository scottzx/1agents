package meta

import (
	"encoding/json"
	"testing"
	"time"
)

func seedTaskRunFixture(t *testing.T) (*DB, string, AgentTurn) {
	t.Helper()
	db := newTestDB(t)
	path := t.TempDir()
	if err := db.EnsureProject("project-1", "project-1", path); err != nil {
		t.Fatal(err)
	}
	if err := NewSessionStore(db).Add(ChatSessionRecord{
		ID: "session-1", WorkspaceID: "project-1", Name: "session", AgentType: "codex",
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err := db.sql.Exec(`
		INSERT INTO project_items (
			id, project_id, title, description, issue_state, status,
			schedule_type, created_at, updated_at
		) VALUES ('task-1', 'project-1', 'task', 'work', 'open', 'running',
			'immediate', ?, ?)`, timeToStr(now), timeToStr(now)); err != nil {
		t.Fatal(err)
	}
	turns := NewAgentTurnStore(db)
	turn, _, err := turns.Create(AgentTurn{
		ProjectID: "project-1", SessionID: "session-1", PromptText: "create task",
	})
	if err != nil {
		t.Fatal(err)
	}
	turn, err = turns.Transition(turn.ID, AgentTurnTransition{Status: AgentTurnRunning})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(map[string]string{"id": "task-1"})
	if _, err := NewProjectEventStore(db).Append(ProjectEvent{
		ProjectID: "project-1", TurnID: turn.ID, SessionID: "session-1",
		ActorKind: "agent", Origin: "mcp", EventType: "project_item.create",
		TargetType: "project_item", TargetID: "task-1", Operation: "create",
		After: after, Status: ProjectEventSucceeded,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := turns.Transition(turn.ID, AgentTurnTransition{Status: AgentTurnCompleted}); err != nil {
		t.Fatal(err)
	}
	return db, path, turn
}

func TestTaskRunTracesOriginTurnEvidenceVerdictAndClosedBy(t *testing.T) {
	db, path, turn := seedTaskRunFixture(t)
	store := NewTaskRunStore(db)
	run, err := store.Create(path, TaskRun{
		TaskID: "task-1", SessionID: "run-session", Kind: TaskRunExecution,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if run.OriginTurnID != turn.ID || run.Attempt != 1 || run.Status != TaskRunRunning {
		t.Fatalf("created run=%+v", run)
	}

	closedBy := &ClosedBy{Kind: "runtime_evidence", Verdict: "passed"}
	run, err = store.Finish(run.ID, TaskRunCompleted, []CompletionEvidence{{
		Kind: "runtime_terminal", Summary: "bridge emitted done", SessionID: run.SessionID,
	}}, nil, closedBy, "")
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if run.ClosedBy == nil || run.ClosedBy.TaskRunID != run.ID ||
		run.ClosedBy.TurnID != turn.ID || len(run.ClosedBy.EvidenceIDs) != 1 {
		t.Fatalf("closedBy=%+v", run.ClosedBy)
	}
	if len(run.Evidence) != 1 || run.Evidence[0].ID == "" {
		t.Fatalf("evidence=%+v", run.Evidence)
	}

	events, err := NewProjectEventStore(db).List(ProjectEventListOptions{
		ProjectID: "project-1", TaskRunID: run.ID, Limit: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events.Items) != 4 {
		t.Fatalf("TaskRun events=%d, want create/start/complete/project_item.complete", len(events.Items))
	}
}

func TestVerificationTaskRunPersistsVerdictWithoutRawLogs(t *testing.T) {
	db, path, _ := seedTaskRunFixture(t)
	store := NewTaskRunStore(db)
	run, err := store.Create(path, TaskRun{
		TaskID: "task-1", SessionID: "verify-session", Kind: TaskRunVerification,
	})
	if err != nil {
		t.Fatal(err)
	}
	verdict := &ReviewVerdict{
		Pass: true, Attempt: 1, Verifier: "codex", CreatedAt: time.Now().UTC(),
		Criteria: []CriterionResult{{Criterion: "tests pass", Pass: true}},
	}
	run, err = store.Finish(run.ID, TaskRunCompleted, []CompletionEvidence{{
		Kind: "verifier_verdict", Summary: "1/1 criteria passed",
	}}, verdict, &ClosedBy{Kind: "verification", Verdict: "passed"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if run.Verdict == nil || !run.Verdict.Pass || run.ClosedBy == nil {
		t.Fatalf("verification audit=%+v", run)
	}
}
