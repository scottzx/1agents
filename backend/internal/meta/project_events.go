package meta

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidCursor         = errors.New("meta: invalid cursor")
	ErrInvalidTurnTransition = errors.New("meta: invalid turn transition")
	ErrIdempotencyConflict   = errors.New("meta: idempotency conflict")
	ErrProjectMismatch       = errors.New("meta: project mismatch")
	ErrTurnNotRunning        = errors.New("meta: turn is not running")
	ErrInvalidProjectEvent   = errors.New("meta: invalid project event")
)

var projectEventRegistry = map[string]map[string]bool{
	"project_item": {"create": true, "update": true, "close": true, "reopen": true, "complete": true, "cancel": true, "delete": true},
	"milestone":    {"create": true, "update": true, "delete": true},
	"dependency":   {"link": true, "unlink": true},
	"session":      {"create": true, "update": true, "archive": true, "reopen": true},
	"turn":         {"queue": true, "start": true, "complete": true, "fail": true, "cancel": true},
	"task_run":     {"create": true, "start": true, "complete": true, "fail": true, "cancel": true},
	"verification": {"create": true, "complete": true, "fail": true},
}

type ProjectEventStore struct {
	db *DB
}

func NewProjectEventStore(db *DB) *ProjectEventStore {
	return &ProjectEventStore{db: db}
}

// ProjectMutationTx is the atomic write boundary shared by a project mutation
// and its immutable ProjectEvent. The raw SQL helpers are intentionally small:
// domain stores remain responsible for their own validation and snapshots.
type ProjectMutationTx struct {
	tx *sql.Tx
}

func (tx *ProjectMutationTx) Exec(query string, args ...any) (sql.Result, error) {
	return tx.tx.Exec(query, args...)
}

func (tx *ProjectMutationTx) QueryRow(query string, args ...any) *sql.Row {
	return tx.tx.QueryRow(query, args...)
}

func (tx *ProjectMutationTx) AppendEvent(event ProjectEvent) (ProjectEvent, error) {
	return appendProjectEventTx(tx.tx, event, false)
}

// WithTransaction commits fn and every Event it appends together, or rolls the
// whole mutation back when fn returns an error.
func (s *ProjectEventStore) WithTransaction(fn func(*ProjectMutationTx) error) error {
	tx, err := s.db.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(&ProjectMutationTx{tx: tx}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *ProjectEventStore) Append(event ProjectEvent) (ProjectEvent, error) {
	tx, err := s.db.sql.Begin()
	if err != nil {
		return ProjectEvent{}, err
	}
	defer tx.Rollback()
	stored, err := appendProjectEventTx(tx, event, false)
	if err != nil {
		return ProjectEvent{}, err
	}
	return stored, tx.Commit()
}

func appendProjectEventTx(tx *sql.Tx, event ProjectEvent, allowNonRunningTurn bool) (ProjectEvent, error) {
	if err := validateProjectEvent(event); err != nil {
		return ProjectEvent{}, err
	}

	var projectExists int
	if err := tx.QueryRow(`SELECT COUNT(1) FROM projects WHERE id = ?`, event.ProjectID).Scan(&projectExists); err != nil {
		return ProjectEvent{}, err
	}
	if projectExists == 0 {
		return ProjectEvent{}, ErrNotFound
	}

	if event.TurnID != "" {
		var turnProject, turnSession string
		var turnStatus AgentTurnStatus
		err := tx.QueryRow(
			`SELECT project_id, session_id, status FROM agent_turns WHERE id = ?`,
			event.TurnID,
		).Scan(&turnProject, &turnSession, &turnStatus)
		if err == sql.ErrNoRows {
			return ProjectEvent{}, ErrNotFound
		}
		if err != nil {
			return ProjectEvent{}, err
		}
		if turnProject != event.ProjectID || (event.SessionID != "" && turnSession != event.SessionID) {
			return ProjectEvent{}, ErrProjectMismatch
		}
		if !allowNonRunningTurn && turnStatus != AgentTurnRunning {
			return ProjectEvent{}, ErrTurnNotRunning
		}
		if event.SessionID == "" {
			event.SessionID = turnSession
		}
	}

	if event.ID == "" {
		event.ID = newID()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	if len(event.Before) == 0 {
		event.Before = json.RawMessage(`{}`)
	}
	if len(event.After) == 0 {
		event.After = json.RawMessage(`{}`)
	}
	if !json.Valid(event.Before) || !json.Valid(event.After) {
		return ProjectEvent{}, fmt.Errorf("%w: before/after must be valid JSON", ErrInvalidProjectEvent)
	}

	var maxSeq sql.NullInt64
	switch {
	case event.TurnID != "":
		if err := tx.QueryRow(
			`SELECT MAX(sequence) FROM project_events WHERE turn_id = ?`,
			event.TurnID,
		).Scan(&maxSeq); err != nil {
			return ProjectEvent{}, err
		}
	case event.CorrelationID != "":
		if err := tx.QueryRow(
			`SELECT MAX(sequence) FROM project_events WHERE turn_id IS NULL AND correlation_id = ?`,
			event.CorrelationID,
		).Scan(&maxSeq); err != nil {
			return ProjectEvent{}, err
		}
	}
	event.Sequence = 0
	if maxSeq.Valid {
		event.Sequence = maxSeq.Int64 + 1
	}

	_, err := tx.Exec(`
		INSERT INTO project_events (
			id, project_id, correlation_id, turn_id, session_id, task_run_id,
			actor_kind, actor_name, origin, event_type, target_type, target_id,
			operation, before_json, after_json, status, error_code, error_text,
			sequence, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.ID, event.ProjectID, event.CorrelationID, nullableString(event.TurnID),
		event.SessionID, event.TaskRunID, event.ActorKind, event.ActorName,
		event.Origin, event.EventType, event.TargetType, event.TargetID,
		event.Operation, string(event.Before), string(event.After), event.Status,
		event.ErrorCode, event.ErrorText, event.Sequence, timeToStr(event.CreatedAt),
	)
	if err != nil {
		return ProjectEvent{}, err
	}
	return event, nil
}

func validateProjectEvent(event ProjectEvent) error {
	if event.ProjectID == "" || event.ActorKind == "" || event.Origin == "" ||
		event.TargetType == "" || event.TargetID == "" || event.Operation == "" {
		return fmt.Errorf("%w: missing required provenance or target", ErrInvalidProjectEvent)
	}
	ops, ok := projectEventRegistry[event.TargetType]
	if !ok || !ops[event.Operation] {
		return fmt.Errorf("%w: unregistered target/operation %s.%s",
			ErrInvalidProjectEvent, event.TargetType, event.Operation)
	}
	wantType := event.TargetType + "." + event.Operation
	if event.EventType != wantType {
		return fmt.Errorf("%w: event_type %q must be %q", ErrInvalidProjectEvent, event.EventType, wantType)
	}
	switch event.Status {
	case ProjectEventSucceeded, ProjectEventRejected, ProjectEventFailed:
	default:
		return fmt.Errorf("%w: invalid status %q", ErrInvalidProjectEvent, event.Status)
	}
	return nil
}

const projectEventCols = `id, project_id, correlation_id, turn_id, session_id,
	task_run_id, actor_kind, actor_name, origin, event_type, target_type,
	target_id, operation, before_json, after_json, status, error_code,
	error_text, sequence, created_at`

func scanProjectEvent(row rowScanner) (ProjectEvent, error) {
	var event ProjectEvent
	var turnID sql.NullString
	var before, after, createdAt string
	if err := row.Scan(
		&event.ID, &event.ProjectID, &event.CorrelationID, &turnID,
		&event.SessionID, &event.TaskRunID, &event.ActorKind, &event.ActorName,
		&event.Origin, &event.EventType, &event.TargetType, &event.TargetID,
		&event.Operation, &before, &after, &event.Status, &event.ErrorCode,
		&event.ErrorText, &event.Sequence, &createdAt,
	); err != nil {
		return ProjectEvent{}, err
	}
	event.TurnID = turnID.String
	event.Before = json.RawMessage(before)
	event.After = json.RawMessage(after)
	event.CreatedAt = strToTime(createdAt)
	return event, nil
}

func (s *ProjectEventStore) Get(id string) (ProjectEvent, bool, error) {
	event, err := scanProjectEvent(s.db.sql.QueryRow(
		`SELECT `+projectEventCols+` FROM project_events WHERE id = ?`, id,
	))
	if err == sql.ErrNoRows {
		return ProjectEvent{}, false, nil
	}
	if err != nil {
		return ProjectEvent{}, false, err
	}
	return event, true, nil
}

func (s *ProjectEventStore) List(opts ProjectEventListOptions) (ProjectEventPage, error) {
	if opts.ProjectID == "" {
		return ProjectEventPage{}, fmt.Errorf("%w: project_id is required", ErrInvalidProjectEvent)
	}
	limit, err := normalizePageLimit(opts.Limit)
	if err != nil {
		return ProjectEventPage{}, err
	}
	cursor, err := decodeStoreCursor(opts.Cursor)
	if err != nil {
		return ProjectEventPage{}, err
	}

	var query strings.Builder
	query.WriteString(`SELECT ` + projectEventCols + ` FROM project_events WHERE project_id = ?`)
	args := []any{opts.ProjectID}
	if opts.TurnID != "" {
		query.WriteString(` AND turn_id = ?`)
		args = append(args, opts.TurnID)
	}
	if opts.SessionID != "" {
		query.WriteString(` AND session_id = ?`)
		args = append(args, opts.SessionID)
	}
	if opts.TaskRunID != "" {
		query.WriteString(` AND task_run_id = ?`)
		args = append(args, opts.TaskRunID)
	}
	if opts.TargetType != "" {
		query.WriteString(` AND target_type = ?`)
		args = append(args, opts.TargetType)
	}
	if opts.TargetID != "" {
		query.WriteString(` AND target_id = ?`)
		args = append(args, opts.TargetID)
	}
	if opts.Status != "" {
		query.WriteString(` AND status = ?`)
		args = append(args, opts.Status)
	}
	if opts.Origin != "" {
		query.WriteString(` AND origin = ?`)
		args = append(args, opts.Origin)
	}
	if cursor.At != "" {
		query.WriteString(` AND (created_at < ? OR (created_at = ? AND id < ?))`)
		args = append(args, cursor.At, cursor.At, cursor.ID)
	}
	query.WriteString(` ORDER BY created_at DESC, id DESC LIMIT ?`)
	args = append(args, limit+1)

	rows, err := s.db.sql.Query(query.String(), args...)
	if err != nil {
		return ProjectEventPage{}, err
	}
	defer rows.Close()
	items := make([]ProjectEvent, 0, limit+1)
	for rows.Next() {
		event, err := scanProjectEvent(rows)
		if err != nil {
			return ProjectEventPage{}, err
		}
		items = append(items, event)
	}
	if err := rows.Err(); err != nil {
		return ProjectEventPage{}, err
	}
	page := ProjectEventPage{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.HasMore = true
		last := page.Items[len(page.Items)-1]
		page.NextCursor = encodeStoreCursor(last.CreatedAt, last.ID)
	}
	return page, nil
}
