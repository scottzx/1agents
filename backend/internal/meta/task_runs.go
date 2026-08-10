package meta

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type TaskRunKind string
type TaskRunStatus string

const (
	TaskRunExecution    TaskRunKind = "execution"
	TaskRunVerification TaskRunKind = "verification"

	TaskRunRunning   TaskRunStatus = "running"
	TaskRunCompleted TaskRunStatus = "completed"
	TaskRunFailed    TaskRunStatus = "failed"
	TaskRunCancelled TaskRunStatus = "cancelled"
)

// TaskRun is one execution, verification, or rework attempt. Raw runtime logs
// deliberately remain in the Session transcript; this record is the queryable
// audit spine used by metrics and completion provenance.
type TaskRun struct {
	ID              string               `json:"id"`
	ProjectID       string               `json:"projectId"`
	TaskID          string               `json:"taskId"`
	OriginTurnID    string               `json:"originTurnId,omitempty"`
	OriginSessionID string               `json:"originSessionId,omitempty"`
	SessionID       string               `json:"sessionId,omitempty"`
	Kind            TaskRunKind          `json:"kind"`
	Status          TaskRunStatus        `json:"status"`
	Attempt         int                  `json:"attempt"`
	Evidence        []CompletionEvidence `json:"evidence"`
	Verdict         *ReviewVerdict       `json:"verdict,omitempty"`
	ClosedBy        *ClosedBy            `json:"closedBy,omitempty"`
	ErrorText       string               `json:"errorText,omitempty"`
	// ProfileSnapshot is a resolved, credential-free execution snapshot.
	ProfileSnapshot json.RawMessage `json:"profileSnapshot,omitempty"`
	StartedAt       time.Time       `json:"startedAt"`
	CompletedAt     *time.Time      `json:"completedAt,omitempty"`
	CreatedAt       time.Time       `json:"createdAt"`
	UpdatedAt       time.Time       `json:"updatedAt"`
}

type TaskRunStore struct {
	db *DB
}

func NewTaskRunStore(db *DB) *TaskRunStore {
	return &TaskRunStore{db: db}
}

func (s *TaskRunStore) Create(workspacePath string, run TaskRun) (TaskRun, error) {
	projectID, err := s.db.projectIDByPath(workspacePath)
	if err != nil {
		return TaskRun{}, err
	}
	if projectID == "" {
		return TaskRun{}, ErrNotFound
	}
	if run.TaskID == "" || (run.Kind != TaskRunExecution && run.Kind != TaskRunVerification) {
		return TaskRun{}, fmt.Errorf("%w: invalid TaskRun", ErrInvalidProjectEvent)
	}
	var taskProject string
	if err := s.db.sql.QueryRow(`SELECT project_id FROM project_items WHERE id = ?`, run.TaskID).Scan(&taskProject); err != nil {
		if err == sql.ErrNoRows {
			return TaskRun{}, ErrNotFound
		}
		return TaskRun{}, err
	}
	if taskProject != projectID {
		return TaskRun{}, ErrProjectMismatch
	}

	if run.ID == "" {
		run.ID = newID()
	}
	run.ProjectID = projectID
	run.Status = TaskRunRunning
	if run.Attempt <= 0 {
		if err := s.db.sql.QueryRow(
			`SELECT COUNT(1) + 1 FROM task_runs WHERE task_id = ? AND kind = ?`,
			run.TaskID, run.Kind,
		).Scan(&run.Attempt); err != nil {
			return TaskRun{}, err
		}
	}
	if run.OriginTurnID == "" {
		run.OriginTurnID, err = s.originTurnForTask(projectID, run.TaskID)
		if err != nil {
			return TaskRun{}, err
		}
	}
	if run.OriginTurnID != "" {
		var turnProject string
		if err := s.db.sql.QueryRow(
			`SELECT project_id, session_id FROM agent_turns WHERE id = ?`, run.OriginTurnID,
		).Scan(&turnProject, &run.OriginSessionID); err != nil {
			if err == sql.ErrNoRows {
				return TaskRun{}, ErrNotFound
			}
			return TaskRun{}, err
		}
		if turnProject != projectID {
			return TaskRun{}, ErrProjectMismatch
		}
	}
	now := time.Now().UTC()
	if run.StartedAt.IsZero() {
		run.StartedAt = now
	}
	run.CreatedAt = now
	run.UpdatedAt = now
	if run.Evidence == nil {
		run.Evidence = []CompletionEvidence{}
	}

	tx, err := s.db.sql.Begin()
	if err != nil {
		return TaskRun{}, err
	}
	defer tx.Rollback()
	evidenceJSON, _ := json.Marshal(run.Evidence)
	_, err = tx.Exec(`
		INSERT INTO task_runs (
			id, project_id, task_id, origin_turn_id, session_id, kind, status,
			attempt, evidence_json, verdict_json, closed_by_json, error_text,
			profile_snapshot_json, started_at, completed_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '', '', '', ?, ?, NULL, ?, ?)`,
		run.ID, run.ProjectID, run.TaskID, nullableString(run.OriginTurnID),
		run.SessionID, run.Kind, run.Status, run.Attempt, string(evidenceJSON),
		string(run.ProfileSnapshot),
		timeToStr(run.StartedAt), timeToStr(run.CreatedAt), timeToStr(run.UpdatedAt),
	)
	if err != nil {
		return TaskRun{}, err
	}
	if _, err := appendProjectEventTx(tx, taskRunEvent(run, "create", ProjectEventSucceeded), true); err != nil {
		return TaskRun{}, err
	}
	if _, err := appendProjectEventTx(tx, taskRunEvent(run, "start", ProjectEventSucceeded), true); err != nil {
		return TaskRun{}, err
	}
	return run, tx.Commit()
}

func (s *TaskRunStore) Finish(id string, status TaskRunStatus, evidence []CompletionEvidence, verdict *ReviewVerdict, closedBy *ClosedBy, errorText string) (TaskRun, error) {
	switch status {
	case TaskRunCompleted, TaskRunFailed, TaskRunCancelled:
	default:
		return TaskRun{}, fmt.Errorf("%w: invalid TaskRun terminal status", ErrInvalidProjectEvent)
	}
	run, ok, err := s.Get(id)
	if err != nil || !ok {
		if err == nil {
			err = ErrNotFound
		}
		return TaskRun{}, err
	}
	if run.Status != TaskRunRunning {
		return TaskRun{}, fmt.Errorf("%w: TaskRun is not running", ErrInvalidTurnTransition)
	}
	now := time.Now().UTC()
	for i := range evidence {
		if evidence[i].ID == "" {
			evidence[i].ID = newID()
		}
		if evidence[i].CreatedAt.IsZero() {
			evidence[i].CreatedAt = now
		}
	}
	if closedBy != nil {
		closedBy.TaskRunID = run.ID
		closedBy.TurnID = run.OriginTurnID
		closedBy.SessionID = run.SessionID
		closedBy.ClosedAt = now
		closedBy.EvidenceIDs = make([]string, len(evidence))
		for i := range evidence {
			closedBy.EvidenceIDs[i] = evidence[i].ID
		}
	}
	run.Status = status
	run.Evidence = evidence
	run.Verdict = verdict
	run.ClosedBy = closedBy
	run.ErrorText = errorText
	run.CompletedAt = &now
	run.UpdatedAt = now

	evidenceJSON, _ := json.Marshal(evidence)
	verdictJSON, _ := json.Marshal(verdict)
	closedByJSON, _ := json.Marshal(closedBy)
	verdictText := ""
	if verdict != nil {
		verdictText = string(verdictJSON)
	}
	closedByText := ""
	if closedBy != nil {
		closedByText = string(closedByJSON)
	}
	tx, err := s.db.sql.Begin()
	if err != nil {
		return TaskRun{}, err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`
		UPDATE task_runs SET status = ?, evidence_json = ?, verdict_json = ?,
			closed_by_json = ?, error_text = ?, completed_at = ?, updated_at = ?
		WHERE id = ? AND status = 'running'`,
		status, string(evidenceJSON), verdictText,
		closedByText, errorText, timeToStr(now), timeToStr(now), id,
	)
	if err != nil {
		return TaskRun{}, err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return TaskRun{}, ErrInvalidTurnTransition
	}
	if closedBy != nil {
		encoded := closedByToJSON(closedBy)
		if _, err := tx.Exec(`
			UPDATE project_items
			SET status = 'completed', completed_at = ?, closed_by = ?, updated_at = ?
			WHERE id = ?`,
			timeToStr(now), encoded, timeToStr(now), run.TaskID,
		); err != nil {
			return TaskRun{}, err
		}
	}
	operation := "complete"
	eventStatus := ProjectEventSucceeded
	if status == TaskRunFailed {
		operation = "fail"
		eventStatus = ProjectEventFailed
	} else if status == TaskRunCancelled {
		operation = "cancel"
		eventStatus = ProjectEventRejected
	}
	if _, err := appendProjectEventTx(tx, taskRunEvent(run, operation, eventStatus), true); err != nil {
		return TaskRun{}, err
	}
	if run.Kind == TaskRunVerification {
		verifyOperation := "complete"
		verifyStatus := ProjectEventSucceeded
		if verdict == nil || !verdict.Pass {
			verifyOperation = "fail"
			verifyStatus = ProjectEventFailed
		}
		if _, err := appendProjectEventTx(tx, verificationEvent(run, verifyOperation, verifyStatus), true); err != nil {
			return TaskRun{}, err
		}
	}
	if closedBy != nil {
		if _, err := appendProjectEventTx(tx, completionEvent(run), true); err != nil {
			return TaskRun{}, err
		}
	}
	return run, tx.Commit()
}

func (s *TaskRunStore) Get(id string) (TaskRun, bool, error) {
	run, err := scanTaskRun(s.db.sql.QueryRow(`SELECT `+taskRunCols+` FROM task_runs WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return TaskRun{}, false, nil
	}
	if err != nil {
		return TaskRun{}, false, err
	}
	if err := s.hydrateOriginSession(&run); err != nil {
		return TaskRun{}, false, err
	}
	return run, true, nil
}

func (s *TaskRunStore) RunningBySession(sessionID string) (TaskRun, bool, error) {
	run, err := scanTaskRun(s.db.sql.QueryRow(
		`SELECT `+taskRunCols+` FROM task_runs WHERE session_id = ? AND status = 'running' ORDER BY created_at DESC LIMIT 1`,
		sessionID,
	))
	if err == sql.ErrNoRows {
		return TaskRun{}, false, nil
	}
	if err != nil {
		return TaskRun{}, false, err
	}
	if err := s.hydrateOriginSession(&run); err != nil {
		return TaskRun{}, false, err
	}
	return run, true, nil
}

func (s *TaskRunStore) RunningByTask(taskID string, kind TaskRunKind) (TaskRun, bool, error) {
	run, err := scanTaskRun(s.db.sql.QueryRow(
		`SELECT `+taskRunCols+` FROM task_runs
		WHERE task_id = ? AND kind = ? AND status = 'running'
		ORDER BY created_at DESC LIMIT 1`,
		taskID, kind,
	))
	if err == sql.ErrNoRows {
		return TaskRun{}, false, nil
	}
	if err != nil {
		return TaskRun{}, false, err
	}
	if err := s.hydrateOriginSession(&run); err != nil {
		return TaskRun{}, false, err
	}
	return run, true, nil
}

func (s *TaskRunStore) ListByTask(taskID string) ([]TaskRun, error) {
	rows, err := s.db.sql.Query(
		`SELECT `+taskRunCols+` FROM task_runs WHERE task_id = ? ORDER BY created_at DESC, id DESC`,
		taskID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TaskRun{}
	for rows.Next() {
		run, err := scanTaskRun(rows)
		if err != nil {
			return nil, err
		}
		if err := s.hydrateOriginSession(&run); err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

func (s *TaskRunStore) hydrateOriginSession(run *TaskRun) error {
	if run.OriginTurnID == "" || run.OriginSessionID != "" {
		return nil
	}
	err := s.db.sql.QueryRow(
		`SELECT session_id FROM agent_turns WHERE id = ?`, run.OriginTurnID,
	).Scan(&run.OriginSessionID)
	if err == sql.ErrNoRows {
		return nil
	}
	return err
}

func (s *TaskRunStore) LatestTurnBySession(sessionID string) (string, error) {
	var turnID string
	err := s.db.sql.QueryRow(
		`SELECT id FROM agent_turns WHERE session_id = ? ORDER BY created_at DESC, id DESC LIMIT 1`,
		sessionID,
	).Scan(&turnID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return turnID, err
}

func (s *TaskRunStore) originTurnForTask(projectID, taskID string) (string, error) {
	var turnID string
	err := s.db.sql.QueryRow(`
		SELECT turn_id FROM project_events
		WHERE project_id = ? AND target_type = 'project_item' AND target_id = ?
			AND operation = 'create' AND turn_id IS NOT NULL
		ORDER BY created_at ASC, id ASC LIMIT 1`, projectID, taskID,
	).Scan(&turnID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return turnID, err
}

const taskRunCols = `id, project_id, task_id, origin_turn_id, session_id, kind,
	status, attempt, evidence_json, verdict_json, closed_by_json, error_text,
	profile_snapshot_json, started_at, completed_at, created_at, updated_at`

func scanTaskRun(row rowScanner) (TaskRun, error) {
	var run TaskRun
	var originTurn sql.NullString
	var evidenceJSON, verdictJSON, closedByJSON, snapshotJSON, startedAt, createdAt, updatedAt string
	var completed sql.NullString
	if err := row.Scan(
		&run.ID, &run.ProjectID, &run.TaskID, &originTurn, &run.SessionID,
		&run.Kind, &run.Status, &run.Attempt, &evidenceJSON, &verdictJSON,
		&closedByJSON, &run.ErrorText, &snapshotJSON, &startedAt, &completed, &createdAt, &updatedAt,
	); err != nil {
		return TaskRun{}, err
	}
	run.OriginTurnID = originTurn.String
	if snapshotJSON != "" {
		run.ProfileSnapshot = json.RawMessage(snapshotJSON)
	}
	run.StartedAt = strToTime(startedAt)
	run.CreatedAt = strToTime(createdAt)
	run.UpdatedAt = strToTime(updatedAt)
	if completed.Valid {
		at := strToTime(completed.String)
		run.CompletedAt = &at
	}
	_ = json.Unmarshal([]byte(evidenceJSON), &run.Evidence)
	if verdictJSON != "" {
		var verdict ReviewVerdict
		if json.Unmarshal([]byte(verdictJSON), &verdict) == nil {
			run.Verdict = &verdict
		}
	}
	if closedByJSON != "" {
		var closedBy ClosedBy
		if json.Unmarshal([]byte(closedByJSON), &closedBy) == nil {
			run.ClosedBy = &closedBy
		}
	}
	if run.Evidence == nil {
		run.Evidence = []CompletionEvidence{}
	}
	return run, nil
}

func taskRunEvent(run TaskRun, operation string, status ProjectEventStatus) ProjectEvent {
	after, _ := json.Marshal(map[string]any{
		"id": run.ID, "taskId": run.TaskID, "kind": run.Kind,
		"status": run.Status, "attempt": run.Attempt,
	})
	return ProjectEvent{
		ID: newID(), ProjectID: run.ProjectID, CorrelationID: run.ID,
		SessionID: run.SessionID, TaskRunID: run.ID,
		ActorKind: "system", ActorName: "task-runner", Origin: "scheduler",
		EventType: "task_run." + operation, TargetType: "task_run",
		TargetID: run.ID, Operation: operation, After: after, Status: status,
	}
}

func verificationEvent(run TaskRun, operation string, status ProjectEventStatus) ProjectEvent {
	after, _ := json.Marshal(map[string]any{
		"taskRunId": run.ID, "taskId": run.TaskID, "verdict": run.Verdict,
	})
	return ProjectEvent{
		ID: newID(), ProjectID: run.ProjectID, CorrelationID: run.ID,
		SessionID: run.SessionID, TaskRunID: run.ID,
		ActorKind: "system", ActorName: "verifier", Origin: "scheduler",
		EventType: "verification." + operation, TargetType: "verification",
		TargetID: run.ID, Operation: operation, After: after, Status: status,
	}
}

func completionEvent(run TaskRun) ProjectEvent {
	after, _ := json.Marshal(map[string]any{
		"taskRunId": run.ID, "closedBy": run.ClosedBy,
	})
	return ProjectEvent{
		ID: newID(), ProjectID: run.ProjectID, CorrelationID: run.ID,
		SessionID: run.SessionID, TaskRunID: run.ID,
		ActorKind: "system", ActorName: "completion-gate", Origin: "scheduler",
		EventType: "project_item.complete", TargetType: "project_item",
		TargetID: run.TaskID, Operation: "complete", After: after,
		Status: ProjectEventSucceeded,
	}
}

func (s *TaskStore) TaskRuns() *TaskRunStore {
	return NewTaskRunStore(s.db)
}
