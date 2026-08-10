package meta

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type AgentTurnStore struct {
	db *DB
}

func NewAgentTurnStore(db *DB) *AgentTurnStore {
	return &AgentTurnStore{db: db}
}

const agentTurnCols = `id, project_id, session_id, client_request_id,
	initiating_reply_id, agent_type, status, prompt_text, request_fingerprint,
	profile_snapshot_json,
	final_answer, error_code, error_text, runtime_record_id, runtime_request_id,
	prompt_message_id, final_reply_id, stop_reason, terminal_source,
	last_event_seq, started_at, completed_at, created_at, updated_at`

func scanAgentTurn(row rowScanner) (AgentTurn, error) {
	var turn AgentTurn
	var startedAt, completedAt sql.NullString
	var createdAt, updatedAt, snapshotJSON string
	if err := row.Scan(
		&turn.ID, &turn.ProjectID, &turn.SessionID, &turn.ClientRequestID,
		&turn.InitiatingReplyID, &turn.AgentType, &turn.Status,
		&turn.PromptText, &turn.RequestFingerprint, &snapshotJSON, &turn.FinalAnswer,
		&turn.ErrorCode, &turn.ErrorText, &turn.RuntimeRecordID,
		&turn.RuntimeRequestID, &turn.PromptMessageID, &turn.FinalReplyID,
		&turn.StopReason, &turn.TerminalSource, &turn.LastEventSeq,
		&startedAt, &completedAt, &createdAt, &updatedAt,
	); err != nil {
		return AgentTurn{}, err
	}
	turn.StartedAt = valToTimePtr(startedAt)
	if snapshotJSON != "" {
		turn.ProfileSnapshot = json.RawMessage(snapshotJSON)
	}
	turn.CompletedAt = valToTimePtr(completedAt)
	turn.CreatedAt = strToTime(createdAt)
	turn.UpdatedAt = strToTime(updatedAt)
	return turn, nil
}

// Create queues a new Turn. A repeated non-empty client_request_id in the same
// Session returns the existing Turn with created=false and never duplicates
// execution.
func (s *AgentTurnStore) Create(turn AgentTurn) (stored AgentTurn, created bool, err error) {
	if turn.ProjectID == "" || turn.SessionID == "" {
		return AgentTurn{}, false, fmt.Errorf("%w: project_id and session_id are required", ErrInvalidTurnTransition)
	}
	if turn.Status == "" {
		turn.Status = AgentTurnQueued
	}
	if turn.Status != AgentTurnQueued {
		return AgentTurn{}, false, fmt.Errorf("%w: new Turn must be queued", ErrInvalidTurnTransition)
	}
	if turn.RequestFingerprint == "" {
		turn.RequestFingerprint = fmt.Sprintf("%x", sha256.Sum256([]byte(turn.PromptText)))
	}

	tx, err := s.db.sql.Begin()
	if err != nil {
		return AgentTurn{}, false, err
	}
	defer tx.Rollback()

	var sessionProject string
	err = tx.QueryRow(`SELECT project_id FROM sessions WHERE id = ?`, turn.SessionID).Scan(&sessionProject)
	if err == sql.ErrNoRows {
		return AgentTurn{}, false, ErrNotFound
	}
	if err != nil {
		return AgentTurn{}, false, err
	}
	if sessionProject != turn.ProjectID {
		return AgentTurn{}, false, ErrProjectMismatch
	}

	if turn.ClientRequestID != "" {
		existing, lookupErr := scanAgentTurn(tx.QueryRow(
			`SELECT `+agentTurnCols+` FROM agent_turns
			 WHERE session_id = ? AND client_request_id = ?`,
			turn.SessionID, turn.ClientRequestID,
		))
		switch lookupErr {
		case nil:
			fingerprintMismatch := existing.RequestFingerprint != "" &&
				existing.RequestFingerprint != turn.RequestFingerprint
			legacyPromptMismatch := existing.RequestFingerprint == "" &&
				existing.PromptText != turn.PromptText
			if fingerprintMismatch || legacyPromptMismatch {
				return AgentTurn{}, false, ErrIdempotencyConflict
			}
			return existing, false, nil
		case sql.ErrNoRows:
		default:
			return AgentTurn{}, false, lookupErr
		}
	}

	if turn.ID == "" {
		turn.ID = newID()
	}
	now := time.Now().UTC()
	if turn.CreatedAt.IsZero() {
		turn.CreatedAt = now
	}
	turn.UpdatedAt = turn.CreatedAt
	if _, err := tx.Exec(`
		INSERT INTO agent_turns (
			id, project_id, session_id, client_request_id, initiating_reply_id,
			agent_type, status, prompt_text, request_fingerprint, profile_snapshot_json, final_answer,
			error_code, error_text, runtime_record_id, runtime_request_id,
			prompt_message_id, final_reply_id, stop_reason, terminal_source,
			last_event_seq, started_at, completed_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		turn.ID, turn.ProjectID, turn.SessionID, turn.ClientRequestID,
		turn.InitiatingReplyID, turn.AgentType, turn.Status, turn.PromptText,
		turn.RequestFingerprint, string(turn.ProfileSnapshot), turn.FinalAnswer, turn.ErrorCode, turn.ErrorText,
		turn.RuntimeRecordID, turn.RuntimeRequestID, turn.PromptMessageID,
		turn.FinalReplyID, turn.StopReason, turn.TerminalSource,
		turn.LastEventSeq, nil, nil,
		timeToStr(turn.CreatedAt), timeToStr(turn.UpdatedAt),
	); err != nil {
		return AgentTurn{}, false, err
	}
	if _, err := appendTurnLifecycleEventTx(tx, turn, "queue"); err != nil {
		return AgentTurn{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return AgentTurn{}, false, err
	}
	return turn, true, nil
}

func (s *AgentTurnStore) Get(id string) (AgentTurn, bool, error) {
	turn, err := scanAgentTurn(s.db.sql.QueryRow(
		`SELECT `+agentTurnCols+` FROM agent_turns WHERE id = ?`, id,
	))
	if err == sql.ErrNoRows {
		return AgentTurn{}, false, nil
	}
	if err != nil {
		return AgentTurn{}, false, err
	}
	return turn, true, nil
}

func (s *AgentTurnStore) SetReplyLinks(id, initiatingReplyID, finalReplyID string) error {
	if initiatingReplyID == "" && finalReplyID == "" {
		return nil
	}
	res, err := s.db.sql.Exec(`
		UPDATE agent_turns
		SET initiating_reply_id = CASE WHEN ? != '' THEN ? ELSE initiating_reply_id END,
		    final_reply_id = CASE WHEN ? != '' THEN ? ELSE final_reply_id END,
		    updated_at = ?
		WHERE id = ?`,
		initiatingReplyID, initiatingReplyID, finalReplyID, finalReplyID,
		timeToStr(time.Now().UTC()), id,
	)
	if err != nil {
		return err
	}
	return affectedOrNotFound(res)
}

func (s *AgentTurnStore) RunningBySession(sessionID string) (AgentTurn, bool, error) {
	turn, err := scanAgentTurn(s.db.sql.QueryRow(
		`SELECT `+agentTurnCols+` FROM agent_turns
		 WHERE session_id = ? AND status = 'running'`, sessionID,
	))
	if err == sql.ErrNoRows {
		return AgentTurn{}, false, nil
	}
	if err != nil {
		return AgentTurn{}, false, err
	}
	return turn, true, nil
}

// NextQueued returns the FIFO head for a Session without changing its state.
func (s *AgentTurnStore) NextQueued(sessionID string) (AgentTurn, bool, error) {
	turn, err := scanAgentTurn(s.db.sql.QueryRow(
		`SELECT `+agentTurnCols+` FROM agent_turns
		 WHERE session_id = ? AND status = 'queued'
		 ORDER BY created_at, id LIMIT 1`, sessionID,
	))
	if err == sql.ErrNoRows {
		return AgentTurn{}, false, nil
	}
	if err != nil {
		return AgentTurn{}, false, err
	}
	return turn, true, nil
}

// QueuedBySession returns the SQLite queue projection. 1ACP owns dispatch and
// reconnect state; callers use this view for audits and projection repair.
func (s *AgentTurnStore) QueuedBySession(sessionID string) ([]AgentTurn, error) {
	rows, err := s.db.sql.Query(
		`SELECT `+agentTurnCols+` FROM agent_turns
		 WHERE session_id = ? AND status = 'queued'
		 ORDER BY created_at, id`, sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var turns []AgentTurn
	for rows.Next() {
		turn, err := scanAgentTurn(rows)
		if err != nil {
			return nil, err
		}
		turns = append(turns, turn)
	}
	return turns, rows.Err()
}

func (s *AgentTurnStore) Outstanding() ([]AgentTurn, error) {
	rows, err := s.db.sql.Query(
		`SELECT ` + agentTurnCols + ` FROM agent_turns
		 WHERE status IN ('running', 'queued')
		 ORDER BY created_at, id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var turns []AgentTurn
	for rows.Next() {
		turn, err := scanAgentTurn(rows)
		if err != nil {
			return nil, err
		}
		turns = append(turns, turn)
	}
	return turns, rows.Err()
}

// RecoverInterrupted closes Turns that cannot survive a backend process
// restart. Running work is failed because its runtime ownership was lost;
// queued work is cancelled rather than replayed without an explicit user
// action.
func (s *AgentTurnStore) RecoverInterrupted(errorCode, errorText string) (failed, cancelled int, err error) {
	rows, err := s.db.sql.Query(`
		SELECT id, status
		FROM agent_turns
		WHERE status IN ('running', 'queued')
		ORDER BY created_at, id`)
	if err != nil {
		return 0, 0, err
	}
	type interruptedTurn struct {
		id     string
		status AgentTurnStatus
	}
	var turns []interruptedTurn
	for rows.Next() {
		var turn interruptedTurn
		if err := rows.Scan(&turn.id, &turn.status); err != nil {
			rows.Close()
			return failed, cancelled, err
		}
		turns = append(turns, turn)
	}
	if err := rows.Close(); err != nil {
		return failed, cancelled, err
	}
	if errorCode == "" {
		errorCode = "backend_restarted"
	}
	if errorText == "" {
		errorText = "Backend restarted before the Turn reached a terminal state."
	}
	for _, turn := range turns {
		next := AgentTurnFailed
		if turn.status == AgentTurnQueued {
			next = AgentTurnCancelled
		}
		if _, transitionErr := s.Transition(turn.id, AgentTurnTransition{
			Status:    next,
			ErrorCode: errorCode,
			ErrorText: errorText,
		}); transitionErr != nil {
			return failed, cancelled, transitionErr
		}
		if next == AgentTurnFailed {
			failed++
		} else {
			cancelled++
		}
	}
	return failed, cancelled, nil
}

func (s *AgentTurnStore) Transition(id string, change AgentTurnTransition) (AgentTurn, error) {
	tx, err := s.db.sql.Begin()
	if err != nil {
		return AgentTurn{}, err
	}
	defer tx.Rollback()

	current, err := scanAgentTurn(tx.QueryRow(
		`SELECT `+agentTurnCols+` FROM agent_turns WHERE id = ?`, id,
	))
	if err == sql.ErrNoRows {
		return AgentTurn{}, ErrNotFound
	}
	if err != nil {
		return AgentTurn{}, err
	}
	if !validTurnTransition(current.Status, change.Status) {
		return AgentTurn{}, fmt.Errorf("%w: %s -> %s",
			ErrInvalidTurnTransition, current.Status, change.Status)
	}
	if change.At.IsZero() {
		change.At = time.Now().UTC()
	}

	next := current
	next.Status = change.Status
	next.UpdatedAt = change.At
	if change.RuntimeRecordID != "" {
		next.RuntimeRecordID = change.RuntimeRecordID
	}
	if change.RuntimeRequestID != "" {
		next.RuntimeRequestID = change.RuntimeRequestID
	}
	if change.PromptMessageID != "" {
		next.PromptMessageID = change.PromptMessageID
	}
	if change.FinalReplyID != "" {
		next.FinalReplyID = change.FinalReplyID
	}
	if change.StopReason != "" {
		next.StopReason = change.StopReason
	}
	if change.TerminalSource != "" {
		next.TerminalSource = change.TerminalSource
	}
	if change.LastEventSeq > next.LastEventSeq {
		next.LastEventSeq = change.LastEventSeq
	}
	switch change.Status {
	case AgentTurnRunning:
		next.StartedAt = &change.At
	case AgentTurnCompleted:
		next.FinalAnswer = change.FinalAnswer
		next.ErrorCode = ""
		next.ErrorText = ""
		next.CompletedAt = &change.At
	case AgentTurnFailed, AgentTurnCancelled:
		next.FinalAnswer = change.FinalAnswer
		next.ErrorCode = change.ErrorCode
		next.ErrorText = change.ErrorText
		next.CompletedAt = &change.At
	}

	res, err := tx.Exec(`
		UPDATE agent_turns
		SET status = ?, final_answer = ?, error_code = ?, error_text = ?,
		    runtime_record_id = ?, runtime_request_id = ?, prompt_message_id = ?,
		    final_reply_id = ?, stop_reason = ?, terminal_source = ?,
		    last_event_seq = ?, started_at = ?, completed_at = ?, updated_at = ?
		WHERE id = ? AND status = ?`,
		next.Status, next.FinalAnswer, next.ErrorCode, next.ErrorText,
		next.RuntimeRecordID, next.RuntimeRequestID, next.PromptMessageID,
		next.FinalReplyID, next.StopReason, next.TerminalSource, next.LastEventSeq,
		timePtrToVal(next.StartedAt), timePtrToVal(next.CompletedAt),
		timeToStr(next.UpdatedAt), id, current.Status,
	)
	if err != nil {
		return AgentTurn{}, err
	}
	if err := affectedOrNotFound(res); err != nil {
		return AgentTurn{}, err
	}
	if _, err := appendTurnLifecycleEventTx(tx, next, turnOperation(change.Status)); err != nil {
		return AgentTurn{}, err
	}
	if err := tx.Commit(); err != nil {
		return AgentTurn{}, err
	}
	return next, nil
}

func validTurnTransition(from, to AgentTurnStatus) bool {
	switch from {
	case AgentTurnQueued:
		return to == AgentTurnRunning || to == AgentTurnCancelled
	case AgentTurnRunning:
		return to == AgentTurnCompleted || to == AgentTurnFailed || to == AgentTurnCancelled
	default:
		return false
	}
}

func turnOperation(status AgentTurnStatus) string {
	switch status {
	case AgentTurnQueued:
		return "queue"
	case AgentTurnRunning:
		return "start"
	case AgentTurnCompleted:
		return "complete"
	case AgentTurnFailed:
		return "fail"
	case AgentTurnCancelled:
		return "cancel"
	default:
		return ""
	}
}

func appendTurnLifecycleEventTx(tx *sql.Tx, turn AgentTurn, operation string) (ProjectEvent, error) {
	after, err := json.Marshal(map[string]any{
		"id":        turn.ID,
		"sessionId": turn.SessionID,
		"status":    turn.Status,
		"errorCode": turn.ErrorCode,
	})
	if err != nil {
		return ProjectEvent{}, err
	}
	actorName := turn.AgentType
	return appendProjectEventTx(tx, ProjectEvent{
		ProjectID:  turn.ProjectID,
		TurnID:     turn.ID,
		SessionID:  turn.SessionID,
		ActorKind:  "agent",
		ActorName:  actorName,
		Origin:     "bridge",
		EventType:  "turn." + operation,
		TargetType: "turn",
		TargetID:   turn.ID,
		Operation:  operation,
		After:      after,
		Status:     ProjectEventSucceeded,
		CreatedAt:  turn.UpdatedAt,
	}, true)
}

func (s *AgentTurnStore) List(opts AgentTurnListOptions) (AgentTurnPage, error) {
	if opts.ProjectID == "" && opts.SessionID == "" {
		return AgentTurnPage{}, fmt.Errorf("%w: project_id or session_id is required", ErrInvalidTurnTransition)
	}
	limit, err := normalizePageLimit(opts.Limit)
	if err != nil {
		return AgentTurnPage{}, err
	}
	cursor, err := decodeStoreCursor(opts.Cursor)
	if err != nil {
		return AgentTurnPage{}, err
	}

	var query strings.Builder
	query.WriteString(`SELECT ` + agentTurnCols + ` FROM agent_turns WHERE 1=1`)
	args := []any{}
	if opts.ProjectID != "" {
		query.WriteString(` AND project_id = ?`)
		args = append(args, opts.ProjectID)
	}
	if opts.SessionID != "" {
		query.WriteString(` AND session_id = ?`)
		args = append(args, opts.SessionID)
	}
	if opts.Status != "" {
		query.WriteString(` AND status = ?`)
		args = append(args, opts.Status)
	}
	if cursor.At != "" {
		query.WriteString(` AND (created_at < ? OR (created_at = ? AND id < ?))`)
		args = append(args, cursor.At, cursor.At, cursor.ID)
	}
	query.WriteString(` ORDER BY created_at DESC, id DESC LIMIT ?`)
	args = append(args, limit+1)

	rows, err := s.db.sql.Query(query.String(), args...)
	if err != nil {
		return AgentTurnPage{}, err
	}
	defer rows.Close()
	items := make([]AgentTurn, 0, limit+1)
	for rows.Next() {
		turn, err := scanAgentTurn(rows)
		if err != nil {
			return AgentTurnPage{}, err
		}
		items = append(items, turn)
	}
	if err := rows.Err(); err != nil {
		return AgentTurnPage{}, err
	}
	page := AgentTurnPage{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.HasMore = true
		last := page.Items[len(page.Items)-1]
		page.NextCursor = encodeStoreCursor(last.CreatedAt, last.ID)
	}
	return page, nil
}
