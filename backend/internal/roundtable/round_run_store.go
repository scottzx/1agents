package roundtable

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

const roundProgressTotal = 5

// ClaimRoundRun atomically creates the only run for a room/round and moves the
// room out of its waiting state before any seat prompt can start. A concurrent
// or retried caller receives the existing run with created=false.
func (s *Store) ClaimRoundRun(roomID string, round int, idempotencyKey string) (*RoundRun, bool, error) {
	if round != 2 && round != 3 {
		return nil, false, fmt.Errorf("roundtable: round must be 2 or 3")
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		idempotencyKey = fmt.Sprintf("%s:r%d", roomID, round)
	}

	tx, err := s.db.SQL().Begin()
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()

	if existing, err := getRoundRunTx(tx, roomID, round); err == nil {
		return existing, false, nil
	} else if err != meta.ErrNotFound {
		return nil, false, err
	}

	var state string
	var confirmedBriefVersion int
	if err := tx.QueryRow(
		`SELECT state, confirmed_brief_version
		 FROM agents_roundtable_rooms WHERE id = ?`,
		roomID,
	).Scan(&state, &confirmedBriefVersion); err != nil {
		if err == sql.ErrNoRows {
			return nil, false, meta.ErrNotFound
		}
		return nil, false, err
	}

	wantState := StateWaitingR2
	claimedState := StateSummarizingR2
	if round == 3 {
		wantState = StateWaitingR3
		claimedState = StateSummarizingR3
	}
	if RoomState(state) != wantState {
		return nil, false, fmt.Errorf(
			"roundtable: r%d only allowed in %s (state=%s)",
			round,
			wantState,
			state,
		)
	}
	if round == 2 && confirmedBriefVersion <= 0 {
		return nil, false, fmt.Errorf("roundtable: confirmed brief required before r2")
	}

	now := time.Now().UTC()
	run := &RoundRun{
		ID:             meta.NewID(),
		RoomID:         roomID,
		Round:          round,
		Status:         RunQueued,
		IdempotencyKey: idempotencyKey,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if _, err := tx.Exec(
		`INSERT INTO agents_roundtable_runs
			(id, room_id, round, status, idempotency_key, created_at, updated_at,
			 started_at, finished_at, error, error_scope)
		 VALUES (?, ?, ?, ?, ?, ?, ?, NULL, NULL, '', '')`,
		run.ID, roomID, round, string(run.Status), idempotencyKey,
		timeToStr(now), timeToStr(now),
	); err != nil {
		// A different process may have won the unique(room_id, round) race.
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			_ = tx.Rollback()
			existing, getErr := s.GetRoundRunByRoomRound(roomID, round)
			return existing, false, getErr
		}
		return nil, false, err
	}

	var roomUpdate sql.Result
	if round == 2 {
		roomUpdate, err = tx.Exec(
			`UPDATE agents_roundtable_rooms
			 SET state = ?, r2_brief_version = confirmed_brief_version, updated_at = ?
			 WHERE id = ? AND state = ? AND confirmed_brief_version > 0`,
			string(claimedState), timeToStr(now), roomID, string(wantState),
		)
	} else {
		roomUpdate, err = tx.Exec(
			`UPDATE agents_roundtable_rooms
			 SET state = ?, updated_at = ?
			 WHERE id = ? AND state = ?`,
			string(claimedState), timeToStr(now), roomID, string(wantState),
		)
	}
	if err != nil {
		return nil, false, err
	}
	affected, _ := roomUpdate.RowsAffected()
	if affected != 1 {
		return nil, false, fmt.Errorf("roundtable: room state claim lost for r%d", round)
	}

	for _, role := range PanelistRoles {
		if _, err := tx.Exec(
			`INSERT INTO agents_roundtable_run_seats
				(run_id, role, status, started_at, finished_at, error)
			 VALUES (?, ?, 'queued', NULL, NULL, '')`,
			run.ID, string(role),
		); err != nil {
			return nil, false, err
		}
	}
	if err := appendRoundEventTx(tx, run, "run", string(RunQueued), "", ""); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return run, true, nil
}

func scanRoundRun(row rowScanner) (*RoundRun, error) {
	var run RoundRun
	var status, createdAt, updatedAt string
	var startedAt, finishedAt sql.NullString
	if err := row.Scan(
		&run.ID, &run.RoomID, &run.Round, &status, &run.IdempotencyKey,
		&createdAt, &updatedAt, &startedAt, &finishedAt, &run.Error, &run.ErrorScope,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, meta.ErrNotFound
		}
		return nil, err
	}
	run.Status = RunStatus(status)
	run.CreatedAt = strToTime(createdAt)
	run.UpdatedAt = strToTime(updatedAt)
	if startedAt.Valid && startedAt.String != "" {
		value := strToTime(startedAt.String)
		run.StartedAt = &value
	}
	if finishedAt.Valid && finishedAt.String != "" {
		value := strToTime(finishedAt.String)
		run.FinishedAt = &value
	}
	return &run, nil
}

func getRoundRunTx(tx *sql.Tx, roomID string, round int) (*RoundRun, error) {
	return scanRoundRun(tx.QueryRow(
		`SELECT id, room_id, round, status, idempotency_key,
		        created_at, updated_at, started_at, finished_at, error, error_scope
		 FROM agents_roundtable_runs
		 WHERE room_id = ? AND round = ?`,
		roomID, round,
	))
}

func (s *Store) GetRoundRun(runID string) (*RoundRun, error) {
	return scanRoundRun(s.db.SQL().QueryRow(
		`SELECT id, room_id, round, status, idempotency_key,
		        created_at, updated_at, started_at, finished_at, error, error_scope
		 FROM agents_roundtable_runs WHERE id = ?`,
		runID,
	))
}

func (s *Store) GetRoundRunByRoomRound(roomID string, round int) (*RoundRun, error) {
	return scanRoundRun(s.db.SQL().QueryRow(
		`SELECT id, room_id, round, status, idempotency_key,
		        created_at, updated_at, started_at, finished_at, error, error_scope
		 FROM agents_roundtable_runs WHERE room_id = ? AND round = ?`,
		roomID, round,
	))
}

func (s *Store) latestRoundRun(roomID string) (*RoundRun, error) {
	return scanRoundRun(s.db.SQL().QueryRow(
		`SELECT id, room_id, round, status, idempotency_key,
		        created_at, updated_at, started_at, finished_at, error, error_scope
		 FROM agents_roundtable_runs
		 WHERE room_id = ?
		 ORDER BY round DESC LIMIT 1`,
		roomID,
	))
}

// UpdateRunStatus persists a lifecycle transition and its recoverable event.
func (s *Store) UpdateRunStatus(runID string, status RunStatus, errorText string) error {
	tx, err := s.db.SQL().Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	run, err := scanRoundRun(tx.QueryRow(
		`SELECT id, room_id, round, status, idempotency_key,
		        created_at, updated_at, started_at, finished_at, error, error_scope
		 FROM agents_roundtable_runs WHERE id = ?`,
		runID,
	))
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	startedAt := ""
	if run.StartedAt != nil {
		startedAt = timeToStr(*run.StartedAt)
	}
	if status == RunRunning && startedAt == "" {
		startedAt = timeToStr(now)
	}
	if _, err := tx.Exec(
		`UPDATE agents_roundtable_runs
		 SET status = ?, updated_at = ?, started_at = NULLIF(?, ''), error = ?, error_scope = ''
		 WHERE id = ?`,
		string(status), timeToStr(now), startedAt, strings.TrimSpace(errorText), runID,
	); err != nil {
		return err
	}
	run.Status = status
	run.UpdatedAt = now
	run.Error = strings.TrimSpace(errorText)
	eventKind := "run"
	if status == RunSummarizing {
		eventKind = "summary"
	}
	if err := appendRoundEventTx(tx, run, eventKind, string(status), "", run.Error); err != nil {
		return err
	}
	return tx.Commit()
}

// StartRunSeat is an atomic at-most-once gate for one role in one run.
func (s *Store) StartRunSeat(runID string, role Role) (bool, error) {
	tx, err := s.db.SQL().Begin()
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	run, err := scanRoundRun(tx.QueryRow(
		`SELECT id, room_id, round, status, idempotency_key,
		        created_at, updated_at, started_at, finished_at, error, error_scope
		 FROM agents_roundtable_runs WHERE id = ?`,
		runID,
	))
	if err != nil {
		return false, err
	}
	now := time.Now().UTC()
	res, err := tx.Exec(
		`UPDATE agents_roundtable_run_seats
		 SET status = 'running', started_at = ?
		 WHERE run_id = ? AND role = ? AND status = 'queued'`,
		timeToStr(now), runID, string(role),
	)
	if err != nil {
		return false, err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return false, tx.Commit()
	}
	if err := appendRoundEventTx(tx, run, "seat", "running", role, ""); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func (s *Store) FinishRunSeat(runID string, role Role, failed bool, errorText string) error {
	status := "completed"
	if failed {
		status = "failed"
	}
	tx, err := s.db.SQL().Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	run, err := scanRoundRun(tx.QueryRow(
		`SELECT id, room_id, round, status, idempotency_key,
		        created_at, updated_at, started_at, finished_at, error, error_scope
		 FROM agents_roundtable_runs WHERE id = ?`,
		runID,
	))
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	res, err := tx.Exec(
		`UPDATE agents_roundtable_run_seats
		 SET status = ?, finished_at = ?, error = ?
		 WHERE run_id = ? AND role = ? AND status = 'running'`,
		status, timeToStr(now), strings.TrimSpace(errorText), runID, string(role),
	)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected != 1 {
		return fmt.Errorf("roundtable: run seat %s is not running", role)
	}
	if err := appendRoundEventTx(tx, run, "seat", status, role, errorText); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) RunProgress(runID string) (RoundProgress, error) {
	progress := RoundProgress{
		Total:        roundProgressTotal,
		ActiveRoles:  []string{},
		FailedRoles:  []string{},
		SkippedRoles: []string{},
	}
	rows, err := s.db.SQL().Query(
		`SELECT role, status FROM agents_roundtable_run_seats
		 WHERE run_id = ? ORDER BY role`,
		runID,
	)
	if err != nil {
		return progress, err
	}
	defer rows.Close()
	for rows.Next() {
		var role, status string
		if err := rows.Scan(&role, &status); err != nil {
			return progress, err
		}
		switch status {
		case "completed":
			progress.Completed++
		case "running":
			progress.ActiveRoles = append(progress.ActiveRoles, role)
		case "failed":
			progress.FailedRoles = append(progress.FailedRoles, role)
		case "skipped":
			progress.SkippedRoles = append(progress.SkippedRoles, role)
		}
	}
	return progress, rows.Err()
}

// PauseRoundRunForSeats exposes failed seats as recoverable work without
// discarding completed seat records or starting the referee summary.
func (s *Store) PauseRoundRunForSeats(runID string) error {
	tx, err := s.db.SQL().Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	run, err := scanRoundRun(tx.QueryRow(
		`SELECT id, room_id, round, status, idempotency_key,
		        created_at, updated_at, started_at, finished_at, error, error_scope
		 FROM agents_roundtable_runs WHERE id = ?`,
		runID,
	))
	if err != nil {
		return err
	}
	var failed int
	if err := tx.QueryRow(
		`SELECT count(*) FROM agents_roundtable_run_seats
		 WHERE run_id = ? AND status = 'failed'`,
		runID,
	).Scan(&failed); err != nil {
		return err
	}
	if failed == 0 {
		return fmt.Errorf("roundtable: no failed seats to pause")
	}
	now := time.Now().UTC()
	message := fmt.Sprintf("%d panelist seat(s) failed", failed)
	if _, err := tx.Exec(
		`UPDATE agents_roundtable_runs
		 SET status = ?, updated_at = ?, finished_at = ?, error = ?, error_scope = ?
		 WHERE id = ?`,
		string(RunPartialFailed), timeToStr(now), timeToStr(now), message, string(RunErrorSeat), runID,
	); err != nil {
		return err
	}
	run.Status = RunPartialFailed
	run.UpdatedAt = now
	run.FinishedAt = &now
	run.Error = message
	run.ErrorScope = RunErrorSeat
	if err := appendRoundEventTx(tx, run, "run", string(RunPartialFailed), "", message); err != nil {
		return err
	}
	return tx.Commit()
}

// ClaimFailedSeatRetry atomically changes exactly one failed seat to running.
// Concurrent duplicate clicks cannot prompt the same role twice.
func (s *Store) ClaimFailedSeatRetry(runID string, role Role) (bool, error) {
	tx, err := s.db.SQL().Begin()
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	run, err := scanRoundRun(tx.QueryRow(
		`SELECT id, room_id, round, status, idempotency_key,
		        created_at, updated_at, started_at, finished_at, error, error_scope
		 FROM agents_roundtable_runs WHERE id = ?`,
		runID,
	))
	if err != nil {
		return false, err
	}
	var seatStatus string
	if err := tx.QueryRow(
		`SELECT status FROM agents_roundtable_run_seats WHERE run_id = ? AND role = ?`,
		runID, string(role),
	).Scan(&seatStatus); err != nil {
		if err == sql.ErrNoRows {
			return false, meta.ErrNotFound
		}
		return false, err
	}
	if seatStatus != "failed" {
		return false, tx.Commit()
	}
	if run.Status != RunPartialFailed || run.ErrorScope != RunErrorSeat {
		return false, fmt.Errorf("roundtable: seat retry unavailable in run status=%s scope=%s", run.Status, run.ErrorScope)
	}
	now := time.Now().UTC()
	res, err := tx.Exec(
		`UPDATE agents_roundtable_run_seats
		 SET status = 'running', started_at = ?, finished_at = NULL, error = ''
		 WHERE run_id = ? AND role = ? AND status = 'failed'`,
		timeToStr(now), runID, string(role),
	)
	if err != nil {
		return false, err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return false, tx.Commit()
	}
	if _, err := tx.Exec(
		`UPDATE agents_roundtable_runs
		 SET status = ?, updated_at = ?, finished_at = NULL, error = '', error_scope = ''
		 WHERE id = ?`,
		string(RunRunning), timeToStr(now), runID,
	); err != nil {
		return false, err
	}
	run.Status = RunRunning
	run.UpdatedAt = now
	run.FinishedAt = nil
	run.Error = ""
	run.ErrorScope = RunErrorNone
	if err := appendRoundEventTx(tx, run, "run", string(RunRunning), "", ""); err != nil {
		return false, err
	}
	if err := appendRoundEventTx(tx, run, "seat", "running", role, ""); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// SkipFailedSeats records the missing roles durably and opens only the summary
// phase. Completed seats and their turns remain untouched.
func (s *Store) SkipFailedSeats(runID string) ([]Role, error) {
	tx, err := s.db.SQL().Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	run, err := scanRoundRun(tx.QueryRow(
		`SELECT id, room_id, round, status, idempotency_key,
		        created_at, updated_at, started_at, finished_at, error, error_scope
		 FROM agents_roundtable_runs WHERE id = ?`,
		runID,
	))
	if err != nil {
		return nil, err
	}
	if run.Status != RunPartialFailed || run.ErrorScope != RunErrorSeat {
		return nil, fmt.Errorf("roundtable: skip unavailable in run status=%s scope=%s", run.Status, run.ErrorScope)
	}
	rows, err := tx.Query(
		`SELECT role FROM agents_roundtable_run_seats
		 WHERE run_id = ? AND status = 'failed' ORDER BY role`,
		runID,
	)
	if err != nil {
		return nil, err
	}
	var roles []Role
	for rows.Next() {
		var role Role
		if err := rows.Scan(&role); err != nil {
			_ = rows.Close()
			return nil, err
		}
		roles = append(roles, role)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(roles) == 0 {
		return nil, fmt.Errorf("roundtable: no failed seats to skip")
	}
	now := time.Now().UTC()
	if _, err := tx.Exec(
		`UPDATE agents_roundtable_run_seats
		 SET status = 'skipped', finished_at = ?, error = ''
		 WHERE run_id = ? AND status = 'failed'`,
		timeToStr(now), runID,
	); err != nil {
		return nil, err
	}
	for _, role := range roles {
		if _, err := tx.Exec(
			`UPDATE agents_roundtable_seats SET status = ? WHERE room_id = ? AND role = ?`,
			string(SeatSkipped), run.RoomID, string(role),
		); err != nil {
			return nil, err
		}
		if err := appendRoundEventTx(tx, run, "seat", "skipped", role, ""); err != nil {
			return nil, err
		}
	}
	if _, err := tx.Exec(
		`UPDATE agents_roundtable_runs
		 SET status = ?, updated_at = ?, finished_at = NULL, error = '', error_scope = ''
		 WHERE id = ?`,
		string(RunSummarizing), timeToStr(now), runID,
	); err != nil {
		return nil, err
	}
	run.Status = RunSummarizing
	run.UpdatedAt = now
	run.FinishedAt = nil
	run.Error = ""
	run.ErrorScope = RunErrorNone
	if err := appendRoundEventTx(tx, run, "summary", string(RunSummarizing), "", ""); err != nil {
		return nil, err
	}
	return roles, tx.Commit()
}

// ClaimSummaryRetry recovers a failed referee summary without reopening any
// panelist execution gate.
func (s *Store) ClaimSummaryRetry(runID string) (bool, error) {
	tx, err := s.db.SQL().Begin()
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	run, err := scanRoundRun(tx.QueryRow(
		`SELECT id, room_id, round, status, idempotency_key,
		        created_at, updated_at, started_at, finished_at, error, error_scope
		 FROM agents_roundtable_runs WHERE id = ?`,
		runID,
	))
	if err != nil {
		return false, err
	}
	if run.Status == RunSummarizing || run.Status == RunCompleted ||
		(run.Status == RunPartialFailed && run.ErrorScope == RunErrorNone) {
		return false, tx.Commit()
	}
	if run.Status != RunFailed || run.ErrorScope != RunErrorSummary {
		return false, fmt.Errorf("roundtable: summary retry unavailable in run status=%s scope=%s", run.Status, run.ErrorScope)
	}
	roomState := StateSummarizingR2
	if run.Round == 3 {
		roomState = StateSummarizingR3
	}
	now := time.Now().UTC()
	if _, err := tx.Exec(
		`UPDATE agents_roundtable_runs
		 SET status = ?, updated_at = ?, finished_at = NULL, error = '', error_scope = ''
		 WHERE id = ? AND status = ? AND error_scope = ?`,
		string(RunSummarizing), timeToStr(now), runID, string(RunFailed), string(RunErrorSummary),
	); err != nil {
		return false, err
	}
	if _, err := tx.Exec(
		`UPDATE agents_roundtable_rooms SET state = ?, updated_at = ? WHERE id = ?`,
		string(roomState), timeToStr(now), run.RoomID,
	); err != nil {
		return false, err
	}
	run.Status = RunSummarizing
	run.UpdatedAt = now
	run.FinishedAt = nil
	run.Error = ""
	run.ErrorScope = RunErrorNone
	if err := appendRoundEventTx(tx, run, "summary", string(RunSummarizing), "", ""); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// FinalizeRoundRun atomically records terminal run/summary events and the room
// summary/state, so reconnecting clients never observe a completed run paired
// with a stale room phase.
func (s *Store) FinalizeRoundRun(
	runID string,
	status RunStatus,
	roomState RoomState,
	summary string,
	errorText string,
	errorScope RunErrorScope,
) error {
	if status != RunCompleted && status != RunPartialFailed && status != RunFailed && status != RunCanceled {
		return fmt.Errorf("roundtable: invalid terminal run status %s", status)
	}
	tx, err := s.db.SQL().Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	run, err := scanRoundRun(tx.QueryRow(
		`SELECT id, room_id, round, status, idempotency_key,
		        created_at, updated_at, started_at, finished_at, error, error_scope
		 FROM agents_roundtable_runs WHERE id = ?`,
		runID,
	))
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	previousStatus := run.Status
	if status == RunFailed || status == RunCanceled {
		rows, queryErr := tx.Query(
			`SELECT role FROM agents_roundtable_run_seats
			 WHERE run_id = ? AND status IN ('queued', 'running')
			 ORDER BY role`,
			runID,
		)
		if queryErr != nil {
			return queryErr
		}
		var unfinished []Role
		for rows.Next() {
			var role Role
			if scanErr := rows.Scan(&role); scanErr != nil {
				_ = rows.Close()
				return scanErr
			}
			unfinished = append(unfinished, role)
		}
		if closeErr := rows.Close(); closeErr != nil {
			return closeErr
		}
		if rowsErr := rows.Err(); rowsErr != nil {
			return rowsErr
		}
		if _, err := tx.Exec(
			`UPDATE agents_roundtable_run_seats
			 SET status = 'failed', finished_at = ?, error = ?
			 WHERE run_id = ? AND status IN ('queued', 'running')`,
			timeToStr(now), strings.TrimSpace(errorText), runID,
		); err != nil {
			return err
		}
		for _, role := range unfinished {
			if err := appendRoundEventTx(tx, run, "seat", "failed", role, errorText); err != nil {
				return err
			}
		}
	}
	if _, err := tx.Exec(
		`UPDATE agents_roundtable_runs
		 SET status = ?, updated_at = ?, finished_at = ?, error = ?, error_scope = ?
		 WHERE id = ?`,
		string(status), timeToStr(now), timeToStr(now), strings.TrimSpace(errorText), string(errorScope), runID,
	); err != nil {
		return err
	}
	if run.Round == 2 {
		_, err = tx.Exec(
			`UPDATE agents_roundtable_rooms
			 SET summary_r2 = ?, state = ?, updated_at = ? WHERE id = ?`,
			summary, string(roomState), timeToStr(now), run.RoomID,
		)
	} else {
		_, err = tx.Exec(
			`UPDATE agents_roundtable_rooms
			 SET summary_r3 = ?, state = ?, updated_at = ? WHERE id = ?`,
			summary, string(roomState), timeToStr(now), run.RoomID,
		)
	}
	if err != nil {
		return err
	}
	run.Status = status
	run.UpdatedAt = now
	run.FinishedAt = &now
	run.Error = strings.TrimSpace(errorText)
	run.ErrorScope = errorScope
	if run.Status == RunCompleted || run.Status == RunPartialFailed {
		if err := appendRoundEventTx(tx, run, "summary", "completed", "", ""); err != nil {
			return err
		}
	} else if status == RunFailed && previousStatus == RunSummarizing {
		if err := appendRoundEventTx(tx, run, "summary", "failed", "", run.Error); err != nil {
			return err
		}
	}
	if err := appendRoundEventTx(tx, run, "run", string(status), "", run.Error); err != nil {
		return err
	}
	return tx.Commit()
}

func appendRoundEventTx(
	tx *sql.Tx,
	run *RoundRun,
	kind string,
	status string,
	role Role,
	errorText string,
) error {
	_, err := tx.Exec(
		`INSERT INTO agents_roundtable_events
			(room_id, run_id, round, kind, status, role, error, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		run.RoomID, run.ID, run.Round, kind, status, string(role),
		strings.TrimSpace(errorText), timeToStr(time.Now().UTC()),
	)
	return err
}

func (s *Store) ListRoundEvents(roomID string, after int64, limit int) ([]RoundEvent, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	rows, err := s.db.SQL().Query(
		`SELECT seq, room_id, run_id, round, kind, status, role, error, created_at
		 FROM agents_roundtable_events
		 WHERE room_id = ? AND seq > ?
		 ORDER BY seq ASC LIMIT ?`,
		roomID, after, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []RoundEvent{}
	for rows.Next() {
		var event RoundEvent
		var role, createdAt string
		if err := rows.Scan(
			&event.Seq, &event.RoomID, &event.RunID, &event.Round,
			&event.Kind, &event.Status, &role, &event.Error, &createdAt,
		); err != nil {
			return nil, err
		}
		event.Role = Role(role)
		event.CreatedAt = strToTime(createdAt)
		events = append(events, event)
	}
	return events, rows.Err()
}

func isTerminalRunStatus(status RunStatus) bool {
	switch status {
	case RunCompleted, RunPartialFailed, RunFailed, RunCanceled:
		return true
	default:
		return false
	}
}
