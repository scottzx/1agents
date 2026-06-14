package meta

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TaskStore is the SQLite-backed replacement for the legacy per-workspace
// .1agents/tasks.json store. Load/Save keep the whole-config semantics of
// the old store (handlers mutate the in-memory config and Save it back), so
// internal/agent swaps over without changes.
type TaskStore struct {
	db *DB
	// importMu serializes the one-time lazy import of a workspace's legacy
	// tasks.json so concurrent Loads don't double-insert.
	importMu sync.Mutex
}

// NewTaskStore returns a TaskStore over db.
func NewTaskStore(db *DB) *TaskStore {
	return &TaskStore{db: db}
}

// taskCols is the canonical task column list shared by Load and GetTask
// (scanTask must stay in sync).
const taskCols = `id, title, description, issue_state, status, schedule_type,
	scheduled_at, planned_start, planned_end, started_at, completed_at,
	summary, created_at, updated_at,
	priority, assignee, labels, created_by, parent_id, milestone,
	acceptance_criteria, recurrence, max_retries, retry_count, timeout_minutes,
	sprint, type`

func scanTask(r rowScanner) (Task, error) {
	var t Task
	var scheduledAt, plannedStart, plannedEnd, startedAt, completedAt sql.NullString
	var createdAt, updatedAt, labels, recurrence string
	if err := r.Scan(&t.ID, &t.Title, &t.Description, &t.IssueState, &t.Status,
		&t.ScheduleType, &scheduledAt, &plannedStart, &plannedEnd, &startedAt,
		&completedAt, &t.Summary, &createdAt, &updatedAt,
		&t.Priority, &t.Assignee, &labels, &t.CreatedBy, &t.ParentID, &t.Milestone,
		&t.AcceptanceCriteria, &recurrence, &t.MaxRetries, &t.RetryCount,
		&t.TimeoutMinutes, &t.Sprint, &t.Type); err != nil {
		return Task{}, err
	}
	t.ScheduledAt = valToTimePtr(scheduledAt)
	t.PlannedStart = valToTimePtr(plannedStart)
	t.PlannedEnd = valToTimePtr(plannedEnd)
	t.StartedAt = valToTimePtr(startedAt)
	t.CompletedAt = valToTimePtr(completedAt)
	t.CreatedAt = strToTime(createdAt)
	t.UpdatedAt = strToTime(updatedAt)
	t.Labels = jsonToStrings(labels)
	t.Recurrence = jsonToRecurrence(recurrence)
	t.DependsOn = []string{}
	t.Replies = []Reply{}
	t.Sessions = []SessionMetadata{}
	return t, nil
}

func stringsToJSON(v []string) string {
	if len(v) == 0 {
		return "[]"
	}
	data, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func jsonToStrings(s string) []string {
	if s == "" || s == "[]" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}

func recurrenceToJSON(r *Recurrence) string {
	if r == nil || r.Freq == "" {
		return ""
	}
	data, err := json.Marshal(r)
	if err != nil {
		return ""
	}
	return string(data)
}

func jsonToRecurrence(s string) *Recurrence {
	if s == "" {
		return nil
	}
	var r Recurrence
	if err := json.Unmarshal([]byte(s), &r); err != nil || r.Freq == "" {
		return nil
	}
	return &r
}

// Load returns all tasks for the workspace at workspacePath, oldest-first
// (matching the legacy JSON array order).
func (s *TaskStore) Load(workspacePath string) (*TasksConfig, error) {
	if err := s.maybeImportLegacy(workspacePath); err != nil {
		return nil, err
	}
	projectID, err := s.db.projectIDByPath(workspacePath)
	if err != nil {
		return nil, err
	}
	if projectID == "" {
		return &TasksConfig{Tasks: []Task{}}, nil
	}

	rows, err := s.db.sql.Query(`
		SELECT `+taskCols+`
		FROM tasks WHERE project_id = ? ORDER BY created_at, id`, projectID)
	if err != nil {
		return nil, err
	}
	tasks := []Task{}
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		t.WorkspacePath = workspacePath
		tasks = append(tasks, t)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	for i := range tasks {
		if err := s.loadTaskChildren(&tasks[i]); err != nil {
			return nil, err
		}
	}
	return &TasksConfig{Tasks: tasks}, nil
}

func (s *TaskStore) loadTaskChildren(t *Task) error {
	// Dependencies, in original array order.
	depRows, err := s.db.sql.Query(
		`SELECT depends_on FROM task_deps WHERE task_id = ? ORDER BY seq`, t.ID)
	if err != nil {
		return err
	}
	for depRows.Next() {
		var dep string
		if err := depRows.Scan(&dep); err != nil {
			depRows.Close()
			return err
		}
		t.DependsOn = append(t.DependsOn, dep)
	}
	if err := depRows.Close(); err != nil {
		return err
	}

	// Timeline replies, chronological. Also build the session → reply
	// reverse index for SessionMetadata.ReplyIDs.
	replyIDsBySession := map[string][]string{}
	replyRows, err := s.db.sql.Query(`
		SELECT id, author_kind, author_name, agent_type, text, session_ref,
		       acp_session_id, in_reply_to, mode, created_at
		FROM replies WHERE task_id = ? ORDER BY seq, created_at`, t.ID)
	if err != nil {
		return err
	}
	for replyRows.Next() {
		var rp Reply
		var createdAt string
		if err := replyRows.Scan(&rp.ID, &rp.Author.Kind, &rp.Author.Name, &rp.AgentType,
			&rp.Text, &rp.SessionRef, &rp.AcpSessionID, &rp.InReplyTo, &rp.Mode,
			&createdAt); err != nil {
			replyRows.Close()
			return err
		}
		rp.CreatedAt = strToTime(createdAt)
		t.Replies = append(t.Replies, rp)
		if rp.SessionRef != "" {
			replyIDsBySession[rp.SessionRef] = append(replyIDsBySession[rp.SessionRef], rp.ID)
		}
	}
	if err := replyRows.Close(); err != nil {
		return err
	}

	// Execution sessions, aggregated from the sessions table by task_id
	// (project-model: SessionMetadata is no longer stored separately).
	sessRows, err := s.db.sql.Query(`
		SELECT id, name, agent_type, exec_status, exec_summary, created_at
		FROM sessions WHERE task_id = ? ORDER BY created_at, id`, t.ID)
	if err != nil {
		return err
	}
	for sessRows.Next() {
		var sm SessionMetadata
		var createdAt string
		if err := sessRows.Scan(&sm.ID, &sm.Name, &sm.AgentType, &sm.Status,
			&sm.Summary, &createdAt); err != nil {
			sessRows.Close()
			return err
		}
		sm.Kind = SessionKindChat
		sm.CreatedAt = strToTime(createdAt)
		sm.ReplyIDs = replyIDsBySession[sm.ID]
		t.Sessions = append(t.Sessions, sm)
	}
	return sessRows.Close()
}

// Save persists cfg as the complete task set for the workspace: tasks
// missing from cfg are deleted (legacy whole-file replace semantics).
func (s *TaskStore) Save(workspacePath string, cfg *TasksConfig) error {
	tx, err := s.db.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	projectID, err := ensureProjectByPathTx(tx, workspacePath)
	if err != nil {
		return err
	}

	keep := map[string]bool{}
	for i := range cfg.Tasks {
		t := &cfg.Tasks[i]
		if t.ID == "" {
			t.ID = newID()
		}
		keep[t.ID] = true
		if err := upsertTaskTx(tx, projectID, t); err != nil {
			return err
		}
	}

	// Drop tasks (and their children) that were removed from the config.
	rows, err := tx.Query(`SELECT id FROM tasks WHERE project_id = ?`, projectID)
	if err != nil {
		return err
	}
	var stale []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		if !keep[id] {
			stale = append(stale, id)
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, id := range stale {
		if err := deleteTaskTx(tx, id); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func upsertTaskTx(tx *sql.Tx, projectID string, t *Task) error {
	if t.IssueState == "" {
		t.IssueState = IssueOpen
	}
	if t.Priority == "" {
		t.Priority = PriorityMedium
	}
	if t.CreatedBy == "" {
		t.CreatedBy = "user"
	}
	if t.Type == "" {
		t.Type = TaskTypeTask
	}
	_, err := tx.Exec(`
		INSERT INTO tasks (id, project_id, title, description, issue_state, status,
			schedule_type, scheduled_at, planned_start, planned_end, started_at,
			completed_at, summary, created_at, updated_at,
			priority, assignee, labels, created_by, parent_id, milestone,
			acceptance_criteria, recurrence, max_retries, retry_count, timeout_minutes,
			sprint, type)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			project_id = excluded.project_id,
			title = excluded.title,
			description = excluded.description,
			issue_state = excluded.issue_state,
			status = excluded.status,
			schedule_type = excluded.schedule_type,
			scheduled_at = excluded.scheduled_at,
			planned_start = excluded.planned_start,
			planned_end = excluded.planned_end,
			started_at = excluded.started_at,
			completed_at = excluded.completed_at,
			summary = excluded.summary,
			updated_at = excluded.updated_at,
			priority = excluded.priority,
			assignee = excluded.assignee,
			labels = excluded.labels,
			created_by = excluded.created_by,
			parent_id = excluded.parent_id,
			milestone = excluded.milestone,
			acceptance_criteria = excluded.acceptance_criteria,
			recurrence = excluded.recurrence,
			max_retries = excluded.max_retries,
			retry_count = excluded.retry_count,
			timeout_minutes = excluded.timeout_minutes,
			sprint = excluded.sprint,
			type = excluded.type`,
		t.ID, projectID, t.Title, t.Description, t.IssueState, t.Status,
		t.ScheduleType, timePtrToVal(t.ScheduledAt), timePtrToVal(t.PlannedStart),
		timePtrToVal(t.PlannedEnd), timePtrToVal(t.StartedAt), timePtrToVal(t.CompletedAt),
		t.Summary, timeToStr(t.CreatedAt), timeToStr(t.UpdatedAt),
		t.Priority, t.Assignee, stringsToJSON(t.Labels), t.CreatedBy, t.ParentID,
		t.Milestone, t.AcceptanceCriteria, recurrenceToJSON(t.Recurrence),
		t.MaxRetries, t.RetryCount, t.TimeoutMinutes, t.Sprint, t.Type)
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`DELETE FROM task_deps WHERE task_id = ?`, t.ID); err != nil {
		return err
	}
	for i, dep := range t.DependsOn {
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO task_deps (task_id, depends_on, seq) VALUES (?, ?, ?)`,
			t.ID, dep, i); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(`DELETE FROM replies WHERE task_id = ?`, t.ID); err != nil {
		return err
	}
	for i := range t.Replies {
		rp := &t.Replies[i]
		if rp.ID == "" {
			rp.ID = newID()
		}
		if rp.CreatedAt.IsZero() {
			rp.CreatedAt = time.Now().UTC()
		}
		if _, err := tx.Exec(`
			INSERT INTO replies (id, task_id, seq, author_kind, author_name, agent_type,
				text, session_ref, acp_session_id, in_reply_to, mode, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			rp.ID, t.ID, i, rp.Author.Kind, rp.Author.Name, rp.AgentType,
			rp.Text, rp.SessionRef, rp.AcpSessionID, rp.InReplyTo, rp.Mode,
			timeToStr(rp.CreatedAt)); err != nil {
			return err
		}
	}

	// Execution sessions: upsert into the shared sessions table. Only the
	// task-execution fields are owned here; an existing chat record keeps
	// its own name/agent_type/project link.
	for i := range t.Sessions {
		sm := &t.Sessions[i]
		if sm.CreatedAt.IsZero() {
			sm.CreatedAt = time.Now().UTC()
		}
		if _, err := tx.Exec(`
			INSERT INTO sessions (id, project_id, task_id, name, agent_type,
				exec_status, exec_summary, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				task_id = excluded.task_id,
				exec_status = excluded.exec_status,
				exec_summary = excluded.exec_summary,
				name = CASE WHEN sessions.name = '' THEN excluded.name ELSE sessions.name END,
				agent_type = CASE WHEN sessions.agent_type = '' THEN excluded.agent_type ELSE sessions.agent_type END,
				project_id = CASE WHEN sessions.project_id = '' THEN excluded.project_id ELSE sessions.project_id END`,
			sm.ID, projectID, t.ID, sm.Name, sm.AgentType,
			sm.Status, sm.Summary, timeToStr(sm.CreatedAt)); err != nil {
			return err
		}
	}
	return nil
}

func deleteTaskTx(tx *sql.Tx, taskID string) error {
	if _, err := tx.Exec(`DELETE FROM task_deps WHERE task_id = ?`, taskID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM replies WHERE task_id = ?`, taskID); err != nil {
		return err
	}
	// Sessions are shared with the chat index: unlink, don't delete.
	if _, err := tx.Exec(`UPDATE sessions SET task_id = '' WHERE task_id = ?`, taskID); err != nil {
		return err
	}
	_, err := tx.Exec(`DELETE FROM tasks WHERE id = ?`, taskID)
	return err
}

// ensureProjectByPathTx is ensureProjectByPath running inside an open
// transaction (the pool is capped at one connection, so queries inside a tx
// must go through it).
func ensureProjectByPathTx(tx *sql.Tx, workspacePath string) (string, error) {
	var id string
	err := tx.QueryRow(
		`SELECT id FROM projects WHERE workspace_path = ? LIMIT 1`, workspacePath).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return "", err
	}
	id = newID()
	now := timeToStr(time.Now().UTC())
	if _, err := tx.Exec(`
		INSERT INTO projects (id, name, workspace_path, status, created_at, updated_at)
		VALUES (?, ?, ?, 'active', ?, ?)`,
		id, filepath.Base(workspacePath), workspacePath, now, now); err != nil {
		return "", err
	}
	return id, nil
}

// ── issue-model fine-grained mutations ──────────────────────────────────────

// AppendReply appends one reply to a task's timeline and bumps updated_at.
// Returns the stored reply (with ID/CreatedAt filled).
func (s *TaskStore) AppendReply(taskID string, rp Reply) (Reply, error) {
	if rp.ID == "" {
		rp.ID = newID()
	}
	if rp.CreatedAt.IsZero() {
		rp.CreatedAt = time.Now().UTC()
	}
	if rp.Mode == "" {
		rp.Mode = ModePureComment
	}
	tx, err := s.db.sql.Begin()
	if err != nil {
		return Reply{}, err
	}
	defer tx.Rollback()

	var exists int
	if err := tx.QueryRow(`SELECT COUNT(1) FROM tasks WHERE id = ?`, taskID).Scan(&exists); err != nil {
		return Reply{}, err
	}
	if exists == 0 {
		return Reply{}, ErrNotFound
	}

	var maxSeq sql.NullInt64
	if err := tx.QueryRow(`SELECT MAX(seq) FROM replies WHERE task_id = ?`, taskID).Scan(&maxSeq); err != nil {
		return Reply{}, err
	}
	seq := int64(0)
	if maxSeq.Valid {
		seq = maxSeq.Int64 + 1
	}

	if _, err := tx.Exec(`
		INSERT INTO replies (id, task_id, seq, author_kind, author_name, agent_type,
			text, session_ref, acp_session_id, in_reply_to, mode, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rp.ID, taskID, seq, rp.Author.Kind, rp.Author.Name, rp.AgentType,
		rp.Text, rp.SessionRef, rp.AcpSessionID, rp.InReplyTo, rp.Mode,
		timeToStr(rp.CreatedAt)); err != nil {
		return Reply{}, err
	}
	if _, err := tx.Exec(`UPDATE tasks SET updated_at = ? WHERE id = ?`,
		timeToStr(time.Now().UTC()), taskID); err != nil {
		return Reply{}, err
	}
	return rp, tx.Commit()
}

// SetReplySession backfills the session a reply spawned (Reply.SessionRef),
// once the WebSocket bridge knows the session id.
func (s *TaskStore) SetReplySession(replyID, sessionID string) error {
	return s.execOne(`UPDATE replies SET session_ref = ? WHERE id = ?`, sessionID, replyID)
}

// UpdateDescription replaces a task's Markdown description.
func (s *TaskStore) UpdateDescription(taskID, description string) error {
	return s.execOne(`UPDATE tasks SET description = ?, updated_at = ? WHERE id = ?`,
		description, timeToStr(time.Now().UTC()), taskID)
}

// SetIssueState toggles the open/closed dimension.
func (s *TaskStore) SetIssueState(taskID string, state IssueState) error {
	return s.execOne(`UPDATE tasks SET issue_state = ?, updated_at = ? WHERE id = ?`,
		string(state), timeToStr(time.Now().UTC()), taskID)
}

// GetTask returns one task (with children) plus its workspace path.
func (s *TaskStore) GetTask(taskID string) (Task, bool, error) {
	row := s.db.sql.QueryRow(`
		SELECT `+taskCols+`
		FROM tasks WHERE id = ?`, taskID)
	t, err := scanTask(row)
	if err == sql.ErrNoRows {
		return Task{}, false, nil
	}
	if err != nil {
		return Task{}, false, err
	}
	err = s.db.sql.QueryRow(`
		SELECT COALESCE(p.workspace_path, '')
		FROM tasks t LEFT JOIN projects p ON p.id = t.project_id
		WHERE t.id = ?`, taskID).Scan(&t.WorkspacePath)
	if err != nil {
		return Task{}, false, err
	}
	if err := s.loadTaskChildren(&t); err != nil {
		return Task{}, false, err
	}
	return t, true, nil
}

func (s *TaskStore) execOne(query string, args ...any) error {
	res, err := s.db.sql.Exec(query, args...)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ── legacy JSON import ──────────────────────────────────────────────────────

// maybeImportLegacy imports <workspacePath>/.1agents/tasks.json on first
// touch and renames it to tasks.json.migrated. Idempotent and safe under
// concurrent Loads.
func (s *TaskStore) maybeImportLegacy(workspacePath string) error {
	legacy := filepath.Join(workspacePath, ".1agents", "tasks.json")
	if _, err := os.Stat(legacy); err != nil {
		return nil // nothing to import
	}

	s.importMu.Lock()
	defer s.importMu.Unlock()
	// Re-check under the lock: another goroutine may have just migrated.
	if _, err := os.Stat(legacy); err != nil {
		return nil
	}

	data, err := os.ReadFile(legacy)
	if err != nil {
		return err
	}
	var cfg TasksConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("meta: parse legacy %s: %w", legacy, err)
	}

	projectID, err := s.db.ensureProjectByPath(workspacePath)
	if err != nil {
		return err
	}
	var count int
	if err := s.db.sql.QueryRow(
		`SELECT COUNT(1) FROM tasks WHERE project_id = ?`, projectID).Scan(&count); err != nil {
		return err
	}
	// Only import into an empty project — never merge into live data.
	if count == 0 && len(cfg.Tasks) > 0 {
		if err := s.Save(workspacePath, &cfg); err != nil {
			return err
		}
	}
	return os.Rename(legacy, legacy+".migrated")
}
