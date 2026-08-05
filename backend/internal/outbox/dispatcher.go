package outbox

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Delivery outcome markers returned by DispatchPending for each event.
const (
	// OutcomeDelivered — every registered consumer now has a receipt.
	OutcomeDelivered = "delivered"
	// OutcomeRetry — at least one consumer failed; the event stays pending
	// and becomes due again after its backoff.
	OutcomeRetry = "retry"
	// OutcomeFailed — attempts exhausted; the event is terminal and carries
	// its diagnosis in LastError.
	OutcomeFailed = "failed"
	// OutcomeSkipped — the event row vanished between scan and delivery;
	// nothing was done.
	OutcomeSkipped = "skipped"
)

// ErrInvalidCursor is returned for malformed pagination cursors.
var ErrInvalidCursor = errors.New("outbox: invalid cursor")

// Consumer processes delivered events. Handle must be idempotent in its own
// effects: delivery is at-least-once (§5.1), and a crash between Handle
// success and receipt write redelivers the event. Guard state changes with
// ClaimTx in the consumer's own transaction when needed.
type Consumer interface {
	// ConsumerID is the stable identity recorded in receipts; it must be
	// unique across registered consumers.
	ConsumerID() string
	// Handle applies the event. Returning an error keeps the event pending
	// for retry (with backoff) and records the error for diagnosis.
	Handle(ctx context.Context, e Entry) error
}

// Fact is the immutable audit fact loaded from the owner's project_events
// row at delivery time. It is never copied into the outbox — the projector
// events table remains the single source of truth (§5 D3, §7.1).
type Fact struct {
	TargetType string          `json:"targetType"`
	TargetID   string          `json:"targetId"`
	Operation  string          `json:"operation"`
	Status     string          `json:"status"`
	Sequence   int64           `json:"sequence"`
	Before     json.RawMessage `json:"before,omitempty"`
	After      json.RawMessage `json:"after,omitempty"`
}

// Entry is one outbox event as seen by consumers and diagnostics: the
// envelope, the current delivery state, and the fact it notifies.
type Entry struct {
	Event
	Status        string    `json:"status"`
	Attempts      int       `json:"attempts"`
	LastError     string    `json:"lastError,omitempty"`
	NextAttemptAt time.Time `json:"nextAttemptAt,omitempty"`
	DeliveredAt   time.Time `json:"deliveredAt,omitempty"`
	Fact          Fact      `json:"fact"`
}

// DeliveryResult reports what one dispatch pass did to one event.
type DeliveryResult struct {
	EventID string `json:"eventId"`
	Outcome string `json:"outcome"` // delivered|retry|failed|skipped
	Error   string `json:"error,omitempty"`
}

// Dispatcher delivers committed outbox events to registered consumers with
// at-least-once semantics: an event is marked delivered only after every
// consumer has a receipt; consumer failures increment attempts, record the
// error for diagnosis and reschedule the event with backoff. Construct one
// per database handle via NewDispatcher.
type Dispatcher struct {
	db *sql.DB

	mu        sync.RWMutex
	consumers []Consumer

	// MaxAttempts bounds retries before an event turns failed (default 5;
	// <=0 means unbounded).
	MaxAttempts int
	// BatchSize bounds events processed per pass (default 20).
	BatchSize int
	// Backoff schedules the next attempt after a failure; it receives the
	// attempt count (>=1) and defaults to DefaultBackoff.
	Backoff func(attempts int) time.Duration
}

// NewDispatcher returns a Dispatcher over db and ensures the outbox tables
// exist.
func NewDispatcher(db *sql.DB) (*Dispatcher, error) {
	if db == nil {
		return nil, errors.New("outbox: nil database handle")
	}
	if err := EnsureSchema(db); err != nil {
		return nil, err
	}
	return &Dispatcher{db: db, MaxAttempts: 5, BatchSize: 20, Backoff: DefaultBackoff}, nil
}

// DefaultBackoff grows exponentially with the attempt count, capped at one
// minute: 1s, 2s, 4s, ...
func DefaultBackoff(attempts int) time.Duration {
	if attempts <= 1 {
		return time.Second
	}
	if attempts > 6 {
		attempts = 6
	}
	d := time.Second << (attempts - 1)
	if d > time.Minute {
		d = time.Minute
	}
	return d
}

// Register adds a consumer. Duplicate ConsumerIDs are rejected: receipts are
// keyed by consumer, so two consumers must never share one identity.
func (d *Dispatcher) Register(c Consumer) error {
	if c == nil || c.ConsumerID() == "" {
		return fmt.Errorf("%w: consumer id is required", ErrInvalidConsumer)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, existing := range d.consumers {
		if existing.ConsumerID() == c.ConsumerID() {
			return fmt.Errorf("%w: consumer %q already registered", ErrInvalidConsumer, c.ConsumerID())
		}
	}
	d.consumers = append(d.consumers, c)
	return nil
}

func (d *Dispatcher) snapshot() []Consumer {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]Consumer, len(d.consumers))
	copy(out, d.consumers)
	return out
}

func (d *Dispatcher) batchSize() int {
	if d.BatchSize <= 0 {
		return 20
	}
	return d.BatchSize
}

func (d *Dispatcher) maxAttempts() int {
	if d.MaxAttempts <= 0 {
		return 0 // unbounded
	}
	return d.MaxAttempts
}

func (d *Dispatcher) backoff(attempts int) time.Duration {
	if d.Backoff == nil {
		return DefaultBackoff(attempts)
	}
	return d.Backoff(attempts)
}

// DispatchPending runs one delivery pass over due pending events (newest
// last). It returns one DeliveryResult per processed event. With no
// consumers registered every event is immediately delivered (nothing waits
// for it). Errors reported by consumers never abort the pass.
func (d *Dispatcher) DispatchPending(ctx context.Context) ([]DeliveryResult, error) {
	now := time.Now().UTC()
	rows, err := d.db.Query(`
		SELECT event_id FROM outbox_events
		WHERE status = 'pending' AND (next_attempt_at = '' OR next_attempt_at <= ?)
		ORDER BY occurred_at, event_id LIMIT ?`,
		timeToStr(now), d.batchSize())
	if err != nil {
		return nil, fmt.Errorf("outbox: scan pending: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("outbox: scan pending: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("outbox: scan pending: %w", err)
	}
	rows.Close()

	results := make([]DeliveryResult, 0, len(ids))
	for _, id := range ids {
		results = append(results, d.deliver(ctx, id, now))
	}
	return results, nil
}

// Run dispatches pending events on every interval until ctx is done. Intended
// as a background goroutine in the running server.
func (d *Dispatcher) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = d.DispatchPending(ctx)
		}
	}
}

// deliver processes one event across all registered consumers.
func (d *Dispatcher) deliver(ctx context.Context, eventID string, now time.Time) DeliveryResult {
	res := DeliveryResult{EventID: eventID, Outcome: OutcomeRetry}

	entry, ok, err := d.loadDelivery(eventID)
	if err != nil {
		res.Error = err.Error()
		return res // transient read failure; the event stays pending as-is
	}
	if !ok {
		return DeliveryResult{EventID: eventID, Outcome: OutcomeSkipped}
	}

	fact, ok, err := d.loadFact(eventID)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	if !ok {
		// The fact row is gone (e.g. data was reset behind the outbox's
		// back). Diagnose loudly instead of delivering a hollow envelope.
		return d.failAttempt(eventID, entry, now,
			fmt.Sprintf("fact row %s missing from project_events", eventID))
	}
	entry.Fact = fact

	var failures []string
	for _, c := range d.snapshot() {
		consumed, err := HasReceipt(d.db, eventID, c.ConsumerID())
		if err != nil {
			failures = append(failures, c.ConsumerID()+": "+err.Error())
			continue
		}
		if consumed {
			continue
		}
		if err := c.Handle(ctx, entry); err != nil {
			failures = append(failures, c.ConsumerID()+": "+err.Error())
			continue
		}
		tx, err := d.db.Begin()
		if err == nil {
			_, err = ClaimTx(tx, eventID, c.ConsumerID())
			if err == nil {
				err = tx.Commit()
			} else {
				tx.Rollback()
			}
		}
		if err != nil {
			failures = append(failures, c.ConsumerID()+": receipt: "+err.Error())
		}
	}

	if len(failures) == 0 {
		if err := d.markDelivered(eventID, now); err != nil {
			res.Error = err.Error()
			return res
		}
		return DeliveryResult{EventID: eventID, Outcome: OutcomeDelivered}
	}
	return d.failAttempt(eventID, entry, now, strings.Join(failures, "; "))
}

// failAttempt records one failed delivery attempt: attempts++, last_error,
// and either a backoff reschedule (still pending) or the terminal failed
// status once MaxAttempts is exhausted.
func (d *Dispatcher) failAttempt(eventID string, entry Entry, now time.Time, msg string) DeliveryResult {
	attempts := entry.Attempts + 1
	max := d.maxAttempts()
	if max > 0 && attempts >= max {
		if _, err := d.db.Exec(`
			UPDATE outbox_events
			SET status = 'failed', attempts = ?, last_error = ?, next_attempt_at = ''
			WHERE event_id = ?`,
			attempts, msg, eventID); err != nil {
			return DeliveryResult{EventID: eventID, Outcome: OutcomeRetry, Error: err.Error()}
		}
		return DeliveryResult{EventID: eventID, Outcome: OutcomeFailed, Error: msg}
	}
	next := timeToStr(now.Add(d.backoff(attempts)))
	if _, err := d.db.Exec(`
		UPDATE outbox_events
		SET attempts = ?, last_error = ?, next_attempt_at = ?
		WHERE event_id = ?`,
		attempts, msg, next, eventID); err != nil {
		return DeliveryResult{EventID: eventID, Outcome: OutcomeRetry, Error: err.Error()}
	}
	return DeliveryResult{EventID: eventID, Outcome: OutcomeRetry, Error: msg}
}

func (d *Dispatcher) markDelivered(eventID string, now time.Time) error {
	_, err := d.db.Exec(`
		UPDATE outbox_events
		SET status = 'delivered', delivered_at = ?, last_error = '', next_attempt_at = ''
		WHERE event_id = ?`,
		timeToStr(now), eventID)
	if err != nil {
		return fmt.Errorf("outbox: mark delivered: %w", err)
	}
	return nil
}

const deliveryCols = `event_id, workspace_id, event_type, schema_version,
	correlation_id, causation_id, subject_ref, actor_kind, actor_name, origin,
	occurred_at, status, attempts, last_error, next_attempt_at, delivered_at`

// loadDelivery reads the envelope + delivery state of one event.
func (d *Dispatcher) loadDelivery(eventID string) (Entry, bool, error) {
	var e Entry
	var occurredAt, nextAttemptAt, deliveredAt sql.NullString
	err := d.db.QueryRow(`SELECT `+deliveryCols+` FROM outbox_events WHERE event_id = ?`, eventID).Scan(
		&e.ID, &e.WorkspaceID, &e.EventType, &e.SchemaVersion,
		&e.CorrelationID, &e.CausationID, &e.SubjectRef, &e.ActorKind,
		&e.ActorName, &e.Origin, &occurredAt, &e.Status, &e.Attempts,
		&e.LastError, &nextAttemptAt, &deliveredAt)
	if err == sql.ErrNoRows {
		return Entry{}, false, nil
	}
	if err != nil {
		return Entry{}, false, fmt.Errorf("outbox: load event %s: %w", eventID, err)
	}
	e.OccurredAt = strToTime(occurredAt.String)
	e.NextAttemptAt = strToTime(nextAttemptAt.String)
	e.DeliveredAt = strToTime(deliveredAt.String)
	return e, true, nil
}

// loadFact reads the immutable fact from project_events. ok=false when the
// fact row is missing; the outbox never writes that table (§7.1).
func (d *Dispatcher) loadFact(eventID string) (Fact, bool, error) {
	var f Fact
	var before, after string
	err := d.db.QueryRow(`
		SELECT target_type, target_id, operation, status, sequence, before_json, after_json
		FROM project_events WHERE id = ?`, eventID).Scan(
		&f.TargetType, &f.TargetID, &f.Operation, &f.Status, &f.Sequence, &before, &after)
	if err == sql.ErrNoRows {
		return Fact{}, false, nil
	}
	if err != nil {
		return Fact{}, false, fmt.Errorf("outbox: load fact %s: %w", eventID, err)
	}
	f.Before = json.RawMessage(before)
	f.After = json.RawMessage(after)
	return f, true, nil
}

// Get returns one outbox entry (envelope + delivery state + fact).
func (d *Dispatcher) Get(eventID string) (Entry, bool, error) {
	entry, ok, err := d.loadDelivery(eventID)
	if err != nil || !ok {
		return Entry{}, ok, err
	}
	fact, ok, err := d.loadFact(eventID)
	if err != nil {
		return Entry{}, false, err
	}
	if ok {
		entry.Fact = fact
	}
	return entry, true, nil
}

// Filter selects entries for List; all fields are optional except the
// implicit newest-first ordering.
type Filter struct {
	WorkspaceID string // restrict to one workspace
	Status      string // pending|delivered|failed|'' (all)
	EventType   string // restrict to one event type
	Limit       int    // default 50, max 200
	Cursor      string // opaque continuation token from a previous page
}

// Page is one List result.
type Page struct {
	Items      []Entry `json:"items"`
	NextCursor string  `json:"nextCursor,omitempty"`
	HasMore    bool    `json:"hasMore"`
}

type pageCursor struct {
	At string `json:"at"`
	ID string `json:"id"`
}

func encodeCursor(at time.Time, id string) string {
	raw, _ := json.Marshal(pageCursor{At: timeToStr(at), ID: id})
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeCursor(value string) (pageCursor, error) {
	if value == "" {
		return pageCursor{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return pageCursor{}, fmt.Errorf("%w: malformed base64", ErrInvalidCursor)
	}
	var c pageCursor
	if err := json.Unmarshal(raw, &c); err != nil || c.At == "" || c.ID == "" {
		return pageCursor{}, fmt.Errorf("%w: malformed payload", ErrInvalidCursor)
	}
	if strToTime(c.At).IsZero() {
		return pageCursor{}, fmt.Errorf("%w: invalid timestamp", ErrInvalidCursor)
	}
	return c, nil
}

// List returns outbox entries, newest first, with cursor pagination. Entries
// whose fact row is missing keep an empty Fact (diagnostics must not fail on
// data anomalies).
func (d *Dispatcher) List(f Filter) (Page, error) {
	limit := f.Limit
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 200 {
		return Page{}, fmt.Errorf("%w: limit must be between 1 and 200", ErrInvalidCursor)
	}
	cursor, err := decodeCursor(f.Cursor)
	if err != nil {
		return Page{}, err
	}

	query := `SELECT ` + deliveryCols + ` FROM outbox_events WHERE 1 = 1`
	args := []any{}
	if f.WorkspaceID != "" {
		query += ` AND workspace_id = ?`
		args = append(args, f.WorkspaceID)
	}
	if f.Status != "" {
		query += ` AND status = ?`
		args = append(args, f.Status)
	}
	if f.EventType != "" {
		query += ` AND event_type = ?`
		args = append(args, f.EventType)
	}
	if cursor.At != "" {
		query += ` AND (occurred_at < ? OR (occurred_at = ? AND event_id < ?))`
		args = append(args, cursor.At, cursor.At, cursor.ID)
	}
	query += ` ORDER BY occurred_at DESC, event_id DESC LIMIT ?`
	args = append(args, limit+1)

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return Page{}, fmt.Errorf("outbox: list: %w", err)
	}
	defer rows.Close()
	items := make([]Entry, 0, limit+1)
	for rows.Next() {
		var e Entry
		var occurredAt, nextAttemptAt, deliveredAt sql.NullString
		if err := rows.Scan(
			&e.ID, &e.WorkspaceID, &e.EventType, &e.SchemaVersion,
			&e.CorrelationID, &e.CausationID, &e.SubjectRef, &e.ActorKind,
			&e.ActorName, &e.Origin, &occurredAt, &e.Status, &e.Attempts,
			&e.LastError, &nextAttemptAt, &deliveredAt); err != nil {
			return Page{}, fmt.Errorf("outbox: list: %w", err)
		}
		e.OccurredAt = strToTime(occurredAt.String)
		e.NextAttemptAt = strToTime(nextAttemptAt.String)
		e.DeliveredAt = strToTime(deliveredAt.String)
		items = append(items, e)
	}
	if err := rows.Err(); err != nil {
		return Page{}, fmt.Errorf("outbox: list: %w", err)
	}

	page := Page{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.HasMore = true
		last := page.Items[len(page.Items)-1]
		page.NextCursor = encodeCursor(last.OccurredAt, last.ID)
	}
	// Best-effort fact enrichment for diagnostics.
	for i := range page.Items {
		if fact, ok, err := d.loadFact(page.Items[i].ID); err == nil && ok {
			page.Items[i].Fact = fact
		}
	}
	return page, nil
}
