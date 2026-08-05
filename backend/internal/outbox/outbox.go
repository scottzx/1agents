// Package outbox implements the C0 Outbox Event contract frozen by
// docs/architecture/enterprise-foundation-v1.0.0.md (§5 D3, §7.1, §7.2):
// a fact committed by a domain Command is notified through a versioned,
// immutable Outbox Event appended inside the SAME local transaction as the
// domain write, then delivered at-least-once by a dispatcher. There is no
// global ordering guarantee; the Event ID is the consumer-side idempotency
// key, enforced via receipts.
//
// No parallel source of truth: the fact itself (before/after payload,
// target, operation) stays in the owner's immutable audit row
// (project_events); the outbox stores the delivery envelope plus delivery
// metadata only, and the dispatcher only ever mutates delivery metadata
// (§7.1 ownership table).
//
// This package is L1 kernel infrastructure: it imports only the standard
// library. Domain owners append through the commandbus.Gateway transaction;
// the dispatcher delivers to registered in-process consumers.
package outbox

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Delivery status of an outbox event (§5.2: at-least-once delivery).
const (
	// StatusPending — committed, awaiting (re)delivery.
	StatusPending = "pending"
	// StatusDelivered — every registered consumer has a receipt.
	StatusDelivered = "delivered"
	// StatusFailed — delivery exhausted MaxAttempts; last_error carries the
	// diagnosis. Failed events are not retried automatically.
	StatusFailed = "failed"
)

// ErrInvalidEvent is returned when an event envelope violates §5.3 invariants.
var ErrInvalidEvent = errors.New("outbox: invalid event")

// ErrInvalidConsumer is returned for malformed or duplicate consumer
// registrations.
var ErrInvalidConsumer = errors.New("outbox: invalid consumer")

// schemaDDL owns the outbox tables (§7.1: kernel tables, written only by this
// package). Idempotent CREATE IF NOT EXISTS so construction heals any
// database, independent of meta's user_version.
var schemaDDL = []string{
	// outbox_events is the delivery layer: one row per committed fact
	// notification. event_id is the owning project_events row — the Event ID
	// doubles as the consumer idempotency key (§5.3). The envelope columns
	// (eventType/schemaVersion/correlationId/causationId/subjectRef/actor)
	// are delivery routing metadata written once at append time; the fact
	// payload itself is never copied here.
	`CREATE TABLE IF NOT EXISTS outbox_events (
		event_id        TEXT PRIMARY KEY,
		workspace_id    TEXT NOT NULL,
		event_type      TEXT NOT NULL,
		schema_version  INTEGER NOT NULL DEFAULT 1,
		correlation_id  TEXT NOT NULL DEFAULT '',
		causation_id    TEXT NOT NULL DEFAULT '',
		subject_ref     TEXT NOT NULL DEFAULT '',
		actor_kind      TEXT NOT NULL DEFAULT '',
		actor_name      TEXT NOT NULL DEFAULT '',
		origin          TEXT NOT NULL DEFAULT '',
		occurred_at     TEXT NOT NULL,
		status          TEXT NOT NULL DEFAULT 'pending' CHECK (
			status IN ('pending','delivered','failed')
		),
		attempts        INTEGER NOT NULL DEFAULT 0,
		last_error      TEXT NOT NULL DEFAULT '',
		next_attempt_at TEXT NOT NULL DEFAULT '',
		delivered_at    TEXT
	)`,
	// Due-pending scan for the dispatcher.
	`CREATE INDEX IF NOT EXISTS idx_outbox_events_due
		ON outbox_events(status, next_attempt_at, occurred_at, event_id)`,
	// Per-workspace diagnostics, newest first.
	`CREATE INDEX IF NOT EXISTS idx_outbox_events_workspace
		ON outbox_events(workspace_id, occurred_at DESC, event_id DESC)`,
	// outbox_receipts is the consumer dedup ledger: (event_id, consumer_id)
	// is consumed exactly once. A visible receipt means the consumer already
	// applied the event, so redeliveries skip it (§5.1: 消费者必须幂等).
	`CREATE TABLE IF NOT EXISTS outbox_receipts (
		event_id    TEXT NOT NULL,
		consumer_id TEXT NOT NULL,
		consumed_at TEXT NOT NULL,
		PRIMARY KEY (event_id, consumer_id)
	)`,
}

// EnsureSchema creates the outbox tables when missing. Idempotent.
func EnsureSchema(db *sql.DB) error {
	for _, ddl := range schemaDDL {
		if _, err := db.Exec(ddl); err != nil {
			return fmt.Errorf("outbox: ensure schema: %w", err)
		}
	}
	return nil
}

// Event is the versioned fact-notification envelope (§5.3). It is immutable
// once appended: only the delivery columns (status/attempts/last_error/
// next_attempt_at/delivered_at) ever change afterwards.
type Event struct {
	// ID is the event id — the owning project_events row id. It is the
	// consumer-side idempotency key.
	ID string `json:"id"`
	// WorkspaceID scopes the event to one workspace (== projects.id).
	WorkspaceID string `json:"workspaceId"`
	// EventType names the fact in past-tense contract form, e.g.
	// "work_case.transition".
	EventType string `json:"eventType"`
	// SchemaVersion pins the fact payload schema the consumer must speak.
	SchemaVersion int `json:"schemaVersion"`
	// CorrelationID groups every command/event of one business flow.
	CorrelationID string `json:"correlationId,omitempty"`
	// CausationID is the command execution id that produced this event;
	// command_executions in turn records what caused that execution, so
	// alternating event→execution links reconstruct the full command chain.
	CausationID string `json:"causationId,omitempty"`
	// SubjectRef is the canonical DomainRef/CaseRef of the object the fact
	// is about.
	SubjectRef string `json:"subjectRef"`
	// ActorKind / ActorName / Origin attribute the fact to the command's
	// authenticated actor (§5.3 actor 与权限上下文).
	ActorKind string `json:"actorKind"`
	ActorName string `json:"actorName,omitempty"`
	Origin    string `json:"origin,omitempty"`
	// OccurredAt is when the fact committed.
	OccurredAt time.Time `json:"occurredAt"`
}

func (e Event) validate() error {
	if e.ID == "" {
		return fmt.Errorf("%w: event id is required", ErrInvalidEvent)
	}
	if e.WorkspaceID == "" {
		return fmt.Errorf("%w: workspaceId is required", ErrInvalidEvent)
	}
	if e.EventType == "" {
		return fmt.Errorf("%w: eventType is required", ErrInvalidEvent)
	}
	if e.SchemaVersion < 1 {
		return fmt.Errorf("%w: schemaVersion must be >= 1, got %d", ErrInvalidEvent, e.SchemaVersion)
	}
	if e.SubjectRef == "" {
		return fmt.Errorf("%w: subjectRef is required", ErrInvalidEvent)
	}
	if e.ActorKind == "" {
		return fmt.Errorf("%w: actorKind is required", ErrInvalidEvent)
	}
	if e.OccurredAt.IsZero() {
		return fmt.Errorf("%w: occurredAt is required", ErrInvalidEvent)
	}
	return nil
}

// AppendTx appends the outbox row for a committed fact inside the caller's
// transaction — the same transaction that wrote the domain state, so the
// event exists if and only if the business write committed (§5.2). A
// validation or insert failure returns an error, which the caller converts
// into a full transaction rollback.
func AppendTx(tx *sql.Tx, e Event) error {
	if err := e.validate(); err != nil {
		return err
	}
	_, err := tx.Exec(`
		INSERT INTO outbox_events (
			event_id, workspace_id, event_type, schema_version, correlation_id,
			causation_id, subject_ref, actor_kind, actor_name, origin,
			occurred_at, status
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending')`,
		e.ID, e.WorkspaceID, e.EventType, e.SchemaVersion, e.CorrelationID,
		e.CausationID, e.SubjectRef, e.ActorKind, e.ActorName, e.Origin,
		timeToStr(e.OccurredAt))
	if err != nil {
		return fmt.Errorf("outbox: append event %s: %w", e.ID, err)
	}
	return nil
}

// ClaimTx records that consumerID consumed eventID, inside the caller's own
// transaction, and reports whether this call recorded the first receipt.
// Consumers guard their business effect with it so a redelivered event never
// re-applies: run the effect only when first==true, in the same transaction.
// The dispatcher uses the same ledger after a successful Handle.
func ClaimTx(tx *sql.Tx, eventID, consumerID string) (first bool, err error) {
	if eventID == "" || consumerID == "" {
		return false, fmt.Errorf("%w: eventID and consumerID are required", ErrInvalidEvent)
	}
	res, err := tx.Exec(`
		INSERT OR IGNORE INTO outbox_receipts (event_id, consumer_id, consumed_at)
		VALUES (?, ?, ?)`,
		eventID, consumerID, timeToStr(time.Now().UTC()))
	if err != nil {
		return false, fmt.Errorf("outbox: claim receipt: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("outbox: claim receipt: %w", err)
	}
	return n > 0, nil
}

// HasReceipt reports whether consumerID already consumed eventID.
func HasReceipt(db *sql.DB, eventID, consumerID string) (bool, error) {
	var n int
	err := db.QueryRow(`
		SELECT COUNT(1) FROM outbox_receipts WHERE event_id = ? AND consumer_id = ?`,
		eventID, consumerID).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("outbox: read receipt: %w", err)
	}
	return n > 0, nil
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
