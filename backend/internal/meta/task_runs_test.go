package meta

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTaskRunSchemaReconcilesLegacyTableBeforeCreatingJobIndex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open fresh database: %v", err)
	}
	if _, err := db.sql.Exec(`
		DROP TABLE task_runs;
		CREATE TABLE task_runs (
			id TEXT PRIMARY KEY, project_id TEXT NOT NULL, task_id TEXT NOT NULL,
			origin_turn_id TEXT, session_id TEXT NOT NULL DEFAULT '',
			kind TEXT NOT NULL, status TEXT NOT NULL, attempt INTEGER NOT NULL,
			evidence_json TEXT NOT NULL DEFAULT '[]', verdict_json TEXT NOT NULL DEFAULT '',
			closed_by_json TEXT NOT NULL DEFAULT '', error_text TEXT NOT NULL DEFAULT '',
			profile_snapshot_json TEXT NOT NULL DEFAULT '', started_at TEXT NOT NULL,
			completed_at TEXT, created_at TEXT NOT NULL, updated_at TEXT NOT NULL
		);
		PRAGMA user_version = 31;
	`); err != nil {
		db.Close()
		t.Fatalf("prepare legacy task_runs table: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	db, err = Open(path)
	if err != nil {
		t.Fatalf("reopen legacy database: %v", err)
	}
	defer db.Close()
	columns, err := db.tableColumns("task_runs")
	if err != nil {
		t.Fatalf("task_runs columns: %v", err)
	}
	for _, column := range []string{"job_id", "trigger_id", "occurrence_key", "client_request_id"} {
		if !columns[column] {
			t.Errorf("task_runs.%s was not reconciled", column)
		}
	}
	var indexCount int
	if err := db.sql.QueryRow(`SELECT COUNT(1) FROM sqlite_master WHERE type='index' AND name='idx_task_runs_job_occurrence_attempt'`).Scan(&indexCount); err != nil || indexCount != 1 {
		t.Fatalf("job occurrence index count=%d err=%v", indexCount, err)
	}
}

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
		ProfileSnapshot: json.RawMessage(`{"profile_id":"deepseek-build","profile_revision":2}`),
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
	if string(turn.ProfileSnapshot) != `{"profile_id":"deepseek-build","profile_revision":2}` {
		t.Fatalf("turn profile snapshot was not persisted: %s", turn.ProfileSnapshot)
	}
	store := NewTaskRunStore(db)
	run, err := store.Create(path, TaskRun{
		TaskID: "task-1", SessionID: "run-session", Kind: TaskRunExecution,
		ProfileSnapshot: json.RawMessage(`{"profile_id":"deepseek-build","profile_revision":2}`),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if run.OriginTurnID != turn.ID || run.Attempt != 1 || run.Status != TaskRunRunning {
		t.Fatalf("created run=%+v", run)
	}
	loaded, ok, err := store.Get(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("created task run was not found")
	}
	if string(loaded.ProfileSnapshot) != `{"profile_id":"deepseek-build","profile_revision":2}` ||
		strings.Contains(string(loaded.ProfileSnapshot), "api_key") {
		t.Fatalf("unsafe profile snapshot: %s", loaded.ProfileSnapshot)
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
