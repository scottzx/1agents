package commandbus

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// schemaDDL owns the command infrastructure tables (§7.1: kernel tables,
// written only by this package). Idempotent CREATE IF NOT EXISTS so gateway
// construction heals any database, independent of meta's user_version.
var schemaDDL = []string{
	// command_idempotency stores the committed result of one idempotencyKey
	// so retried submissions replay the original effect instead of
	// re-executing. A row exists only after its transaction committed, so a
	// visible row is always 'completed'.
	`CREATE TABLE IF NOT EXISTS command_idempotency (
		workspace_id    TEXT NOT NULL,
		idempotency_key TEXT NOT NULL,
		contract        TEXT NOT NULL,
		execution_id    TEXT NOT NULL,
		status          TEXT NOT NULL CHECK (status IN ('running','completed')),
		result_json     TEXT NOT NULL DEFAULT '{}',
		new_version     INTEGER NOT NULL DEFAULT 0,
		event_id        TEXT NOT NULL DEFAULT '',
		target_id       TEXT NOT NULL DEFAULT '',
		created_at      TEXT NOT NULL,
		completed_at    TEXT,
		PRIMARY KEY (workspace_id, idempotency_key)
	)`,
	// command_executions is the execution audit trail: one row per dispatch
	// attempt — succeeded, replayed, rejected or failed — with actor,
	// command, target, result and duration (§5.1 审计).
	`CREATE TABLE IF NOT EXISTS command_executions (
		id               TEXT PRIMARY KEY,
		workspace_id     TEXT NOT NULL DEFAULT '',
		contract         TEXT NOT NULL,
		schema_version   INTEGER NOT NULL DEFAULT 0,
		actor_kind       TEXT NOT NULL DEFAULT '',
		actor_name       TEXT NOT NULL DEFAULT '',
		session_id       TEXT NOT NULL DEFAULT '',
		turn_id          TEXT NOT NULL DEFAULT '',
		task_run_id      TEXT NOT NULL DEFAULT '',
		origin           TEXT NOT NULL DEFAULT '',
		correlation_id   TEXT NOT NULL DEFAULT '',
		causation_id     TEXT NOT NULL DEFAULT '',
		idempotency_key  TEXT NOT NULL DEFAULT '',
		execution_id     TEXT NOT NULL DEFAULT '',
		target_id        TEXT NOT NULL DEFAULT '',
		expected_version INTEGER NOT NULL DEFAULT 0,
		status           TEXT NOT NULL CHECK (
			status IN ('succeeded','replayed','rejected','failed')
		),
		result_json      TEXT NOT NULL DEFAULT '{}',
		new_version      INTEGER NOT NULL DEFAULT 0,
		event_id         TEXT NOT NULL DEFAULT '',
		error_code       TEXT NOT NULL DEFAULT '',
		error_text       TEXT NOT NULL DEFAULT '',
		duration_ms      INTEGER NOT NULL DEFAULT 0,
		created_at       TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_command_executions_workspace
		ON command_executions(workspace_id, created_at DESC, id DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_command_executions_target
		ON command_executions(workspace_id, target_id, created_at DESC, id DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_command_executions_correlation
		ON command_executions(correlation_id, created_at DESC, id DESC)
		WHERE correlation_id != ''`,
}

// EnsureSchema creates the command infrastructure tables when missing.
// Idempotent.
func EnsureSchema(db *sql.DB) error {
	for _, ddl := range schemaDDL {
		if _, err := db.Exec(ddl); err != nil {
			return fmt.Errorf("commandbus: ensure schema: %w", err)
		}
	}
	return nil
}

// Gateway is the single entry point every state mutation must pass through
// (§5.2, D3). It combines the handler registry, idempotency, optimistic
// concurrency plumbing and the execution audit trail. Construct one per
// database handle via New and register the domain owners' handlers.
type Gateway struct {
	registry *Registry
	db       *sql.DB
}

// New returns a Gateway over db and ensures the infrastructure tables exist.
func New(db *sql.DB) (*Gateway, error) {
	if db == nil {
		return nil, errors.New("commandbus: nil database handle")
	}
	if err := EnsureSchema(db); err != nil {
		return nil, err
	}
	return &Gateway{registry: NewRegistry(), db: db}, nil
}

// Register adds a command descriptor to the gateway's registry.
func (g *Gateway) Register(d Descriptor) error { return g.registry.Register(d) }

// Contracts lists the registered command names (diagnostics/tests).
func (g *Gateway) Contracts() []string { return g.registry.Contracts() }

// Dispatch executes cmd through the unified pipeline (§5.2):
//
//  1. envelope validation          → CodeInvalidPayload
//  2. registry lookup              → CodeUnknownCommand
//  3. actor permission policy      → CodePermissionDenied
//  4. idempotency claim            → replay stored result on duplicate key
//  5. handler execution inside one transaction that also commits the
//     idempotency record and the success audit row atomically
//
// Every attempt — including rejections — is recorded in command_executions
// with its duration.
func (g *Gateway) Dispatch(ctx context.Context, cmd Command) (Result, error) {
	start := time.Now()

	if err := cmd.Validate(); err != nil {
		g.auditRejected(cmd, "", classify(err), time.Since(start))
		return Result{}, err
	}
	d, ok := g.registry.Lookup(cmd.Contract)
	if !ok {
		err := NewError(CodeUnknownCommand, "no handler registered for command %q", cmd.Contract)
		g.auditRejected(cmd, "", err, time.Since(start))
		return Result{}, err
	}
	if !d.supportsVersion(cmd.SchemaVersion) {
		err := NewError(CodeInvalidPayload,
			"command %q does not support schemaVersion %d (supported: %v)",
			cmd.Contract, cmd.SchemaVersion, d.SchemaVersions)
		g.auditRejected(cmd, "", err, time.Since(start))
		return Result{}, err
	}
	if !d.allows(cmd.Actor.Kind) {
		err := NewError(CodePermissionDenied,
			"actor kind %q is not permitted to execute %q", cmd.Actor.Kind, cmd.Contract)
		g.auditRejected(cmd, cmd.TargetID, err, time.Since(start))
		return Result{}, err
	}
	if d.Authorize != nil {
		if err := d.Authorize(cmd); err != nil {
			ce := classify(err)
			if ce.Code != CodePermissionDenied {
				ce = WrapError(CodePermissionDenied, err, "%s", ce.Message)
			}
			g.auditRejected(cmd, cmd.TargetID, ce, time.Since(start))
			return Result{}, ce
		}
	}

	execID := newID()
	tx, err := g.db.Begin()
	if err != nil {
		ce := WrapError(CodeInternal, err, "begin command transaction: %v", err)
		g.auditExecution(cmd, execID, "failed", cmd.TargetID, ce, Result{}, time.Since(start))
		return Result{}, ce
	}
	defer tx.Rollback()

	// Claim the idempotency key. The database serializes writers, so a
	// concurrent duplicate blocks on this write until we commit and then
	// reads our completed record instead of re-executing.
	res, err := tx.Exec(`
		INSERT OR IGNORE INTO command_idempotency
			(workspace_id, idempotency_key, contract, execution_id, status, created_at)
		VALUES (?, ?, ?, ?, 'running', ?)`,
		cmd.WorkspaceID, cmd.IdempotencyKey, cmd.Contract, execID, timeToStr(time.Now().UTC()))
	if err != nil {
		ce := WrapError(CodeInternal, err, "claim idempotency key: %v", err)
		tx.Rollback() // release the write lock before the audit write
		g.auditExecution(cmd, execID, "failed", cmd.TargetID, ce, Result{}, time.Since(start))
		return Result{}, ce
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return g.replay(tx, cmd, execID, start)
	}

	result, herr := d.Handler(ctx, cmd, tx)
	if herr != nil {
		ce := classify(herr)
		// Roll back before the best-effort audit write so the audit
		// connection can take the write lock.
		tx.Rollback()
		status := "rejected"
		if ce.Code == CodeInternal {
			status = "failed"
		}
		g.auditExecution(cmd, execID, status, targetOf(cmd, result), ce, Result{}, time.Since(start))
		return Result{}, ce
	}

	resultJSON := "{}"
	if len(result.Payload) > 0 {
		resultJSON = string(result.Payload)
	}
	target := targetOf(cmd, result)
	now := timeToStr(time.Now().UTC())
	if _, err := tx.Exec(`
		UPDATE command_idempotency
		SET status = 'completed', result_json = ?, new_version = ?, event_id = ?,
			target_id = ?, completed_at = ?
		WHERE workspace_id = ? AND idempotency_key = ?`,
		resultJSON, result.Version, result.EventID, target, now,
		cmd.WorkspaceID, cmd.IdempotencyKey); err != nil {
		ce := WrapError(CodeInternal, err, "complete idempotency record: %v", err)
		tx.Rollback()
		g.auditExecution(cmd, execID, "failed", target, ce, Result{}, time.Since(start))
		return Result{}, ce
	}
	if result.Status == "" {
		result.Status = ResultSucceeded
	}
	if result.TargetID == "" {
		result.TargetID = target
	}
	if err := g.auditExecutionTx(tx, cmd, execID, "succeeded", target, nil, result, time.Since(start)); err != nil {
		ce := WrapError(CodeInternal, err, "append execution audit: %v", err)
		tx.Rollback()
		g.auditExecution(cmd, execID, "failed", target, ce, Result{}, time.Since(start))
		return Result{}, ce
	}
	if err := tx.Commit(); err != nil {
		ce := WrapError(CodeInternal, err, "commit command transaction: %v", err)
		g.auditExecution(cmd, execID, "failed", target, ce, Result{}, time.Since(start))
		return Result{}, ce
	}
	return result, nil
}

// replay returns the stored result of a previously committed execution with
// the same idempotencyKey. The effect is produced exactly once; every
// duplicate submission observes the same result (§5.1 幂等键).
func (g *Gateway) replay(tx *sql.Tx, cmd Command, execID string, start time.Time) (Result, error) {
	var status, storedContract, resultJSON, eventID, targetID string
	var newVersion int
	err := tx.QueryRow(`
		SELECT status, contract, result_json, new_version, event_id, target_id
		FROM command_idempotency WHERE workspace_id = ? AND idempotency_key = ?`,
		cmd.WorkspaceID, cmd.IdempotencyKey).Scan(
		&status, &storedContract, &resultJSON, &newVersion, &eventID, &targetID)
	if err != nil {
		ce := WrapError(CodeInternal, err, "read idempotency record: %v", err)
		tx.Rollback()
		g.auditExecution(cmd, execID, "failed", cmd.TargetID, ce, Result{}, time.Since(start))
		return Result{}, ce
	}
	tx.Rollback() // read-only; nothing to commit

	if storedContract != cmd.Contract {
		ce := NewError(CodeInvalidPayload,
			"idempotencyKey %q was already used for command %q", cmd.IdempotencyKey, storedContract)
		g.auditExecution(cmd, execID, "rejected", cmd.TargetID, ce, Result{}, time.Since(start))
		return Result{}, ce
	}
	if status != "completed" {
		// Unreachable with transactional claims (a 'running' row is never
		// committed); guarded so a corrupt row fails loudly instead of
		// double-executing.
		ce := NewError(CodeInternal, "idempotency record for key %q is not completed", cmd.IdempotencyKey)
		g.auditExecution(cmd, execID, "failed", cmd.TargetID, ce, Result{}, time.Since(start))
		return Result{}, ce
	}

	result := Result{
		Status:   ResultReplayed,
		Version:  newVersion,
		EventID:  eventID,
		TargetID: targetID,
		Payload:  json.RawMessage(resultJSON),
	}
	g.auditExecution(cmd, execID, "replayed", targetID, nil, result, time.Since(start))
	return result, nil
}

func targetOf(cmd Command, result Result) string {
	if result.TargetID != "" {
		return result.TargetID
	}
	return cmd.TargetID
}

// auditRejected records a pre-dispatch rejection (envelope, registry or
// permission) best-effort: the audit write must never mask the rejection.
func (g *Gateway) auditRejected(cmd Command, targetID string, ce *Error, duration time.Duration) {
	g.auditExecution(cmd, newID(), "rejected", targetID, ce, Result{}, duration)
}

// auditExecution records one execution row in its own short transaction.
// Best-effort — an audit failure is dropped, never surfaced over the real
// command outcome.
func (g *Gateway) auditExecution(cmd Command, execID, status, targetID string, ce *Error, result Result, duration time.Duration) {
	tx, err := g.db.Begin()
	if err != nil {
		return
	}
	defer tx.Rollback()
	if err := g.auditExecutionTx(tx, cmd, execID, status, targetID, ce, result, duration); err != nil {
		return
	}
	_ = tx.Commit()
}

func (g *Gateway) auditExecutionTx(tx *sql.Tx, cmd Command, execID, status, targetID string, ce *Error, result Result, duration time.Duration) error {
	errorCode, errorText := "", ""
	if ce != nil {
		errorCode, errorText = string(ce.Code), ce.Message
	}
	resultJSON := "{}"
	if len(result.Payload) > 0 {
		resultJSON = string(result.Payload)
	}
	_, err := tx.Exec(`
		INSERT INTO command_executions (
			id, workspace_id, contract, schema_version, actor_kind, actor_name,
			session_id, turn_id, task_run_id, origin, correlation_id, causation_id,
			idempotency_key, execution_id, target_id, expected_version, status,
			result_json, new_version, event_id, error_code, error_text,
			duration_ms, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		newID(), cmd.WorkspaceID, cmd.Contract, cmd.SchemaVersion,
		string(cmd.Actor.Kind), cmd.Actor.Name, cmd.Actor.SessionID, cmd.Actor.TurnID,
		cmd.Actor.TaskRunID, cmd.Actor.Origin, cmd.CorrelationID, cmd.CausationID,
		cmd.IdempotencyKey, execID, targetID, cmd.ExpectedVersion, status,
		resultJSON, result.Version, result.EventID, errorCode, errorText,
		duration.Milliseconds(), timeToStr(time.Now().UTC()))
	return err
}

// Execution is one audit row from command_executions.
type Execution struct {
	ID              string          `json:"id"`
	WorkspaceID     string          `json:"workspaceId"`
	Contract        string          `json:"contract"`
	SchemaVersion   int             `json:"schemaVersion"`
	ActorKind       string          `json:"actorKind"`
	ActorName       string          `json:"actorName"`
	SessionID       string          `json:"sessionId,omitempty"`
	TurnID          string          `json:"turnId,omitempty"`
	TaskRunID       string          `json:"taskRunId,omitempty"`
	Origin          string          `json:"origin,omitempty"`
	CorrelationID   string          `json:"correlationId,omitempty"`
	CausationID     string          `json:"causationId,omitempty"`
	IdempotencyKey  string          `json:"idempotencyKey"`
	ExecutionID     string          `json:"executionId"`
	TargetID        string          `json:"targetId,omitempty"`
	ExpectedVersion int             `json:"expectedVersion,omitempty"`
	Status          string          `json:"status"`
	Result          json.RawMessage `json:"result,omitempty"`
	NewVersion      int             `json:"newVersion,omitempty"`
	EventID         string          `json:"eventId,omitempty"`
	ErrorCode       string          `json:"errorCode,omitempty"`
	ErrorText       string          `json:"errorText,omitempty"`
	DurationMS      int64           `json:"durationMs"`
	CreatedAt       time.Time       `json:"createdAt"`
}

// ExecutionFilter selects audit rows; WorkspaceID is required.
type ExecutionFilter struct {
	WorkspaceID string
	TargetID    string // optional: restrict to one object (e.g. a case)
	Contract    string // optional: restrict to one command contract
	Limit       int    // default 50, max 200
}

// ListExecutions returns the execution audit trail, newest first.
func (g *Gateway) ListExecutions(f ExecutionFilter) ([]Execution, error) {
	if f.WorkspaceID == "" {
		return nil, NewError(CodeInvalidPayload, "workspaceId is required")
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	query := `SELECT id, workspace_id, contract, schema_version, actor_kind, actor_name,
			session_id, turn_id, task_run_id, origin, correlation_id, causation_id,
			idempotency_key, execution_id, target_id, expected_version, status,
			result_json, new_version, event_id, error_code, error_text,
			duration_ms, created_at
		FROM command_executions WHERE workspace_id = ?`
	args := []any{f.WorkspaceID}
	if f.TargetID != "" {
		query += ` AND target_id = ?`
		args = append(args, f.TargetID)
	}
	if f.Contract != "" {
		query += ` AND contract = ?`
		args = append(args, f.Contract)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := g.db.Query(query, args...)
	if err != nil {
		return nil, WrapError(CodeInternal, err, "list executions: %v", err)
	}
	defer rows.Close()
	out := []Execution{}
	for rows.Next() {
		var e Execution
		var resultJSON, createdAt string
		if err := rows.Scan(&e.ID, &e.WorkspaceID, &e.Contract, &e.SchemaVersion,
			&e.ActorKind, &e.ActorName, &e.SessionID, &e.TurnID, &e.TaskRunID,
			&e.Origin, &e.CorrelationID, &e.CausationID, &e.IdempotencyKey,
			&e.ExecutionID, &e.TargetID, &e.ExpectedVersion, &e.Status,
			&resultJSON, &e.NewVersion, &e.EventID, &e.ErrorCode, &e.ErrorText,
			&e.DurationMS, &createdAt); err != nil {
			return nil, WrapError(CodeInternal, err, "scan execution: %v", err)
		}
		e.Result = json.RawMessage(resultJSON)
		e.CreatedAt = strToTime(createdAt)
		out = append(out, e)
	}
	return out, rows.Err()
}

// ── helpers ─────────────────────────────────────────────────────────────────

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "commandbus-fallback-id"
	}
	return hex.EncodeToString(b[:])
}

func timeToStr(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func strToTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

// IsBusy reports whether err is a SQLite lock/busy failure. Exposed for
// callers that implement their own retry loops around Dispatch.
func IsBusy(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "database is locked")
}
