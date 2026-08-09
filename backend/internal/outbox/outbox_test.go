package outbox_test

// Outbox contract tests (#324 acceptance): transactional append, retryable
// at-least-once delivery, consumer-side idempotent consumption via receipts,
// failure diagnosis (attempts/last_error/failed status), backoff scheduling
// and cursor pagination.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/scottzx/1Agents/backend/internal/outbox"
)

// ── rig ─────────────────────────────────────────────────────────────────────

func newOutboxDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "outbox.db")
	dsn := "file:" + path +
		"?_txlock=immediate" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(10)
	if err := outbox.EnsureSchema(db); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	// Minimal project_events stand-in: the outbox reads the fact from the
	// owner's audit table but never writes it.
	if _, err := db.Exec(`
		CREATE TABLE project_events (
			id             TEXT PRIMARY KEY,
			project_id     TEXT NOT NULL,
			correlation_id TEXT NOT NULL DEFAULT '',
			actor_kind     TEXT NOT NULL,
			actor_name     TEXT NOT NULL DEFAULT '',
			origin         TEXT NOT NULL,
			event_type     TEXT NOT NULL,
			target_type    TEXT NOT NULL,
			target_id      TEXT NOT NULL,
			operation      TEXT NOT NULL,
			before_json    TEXT NOT NULL DEFAULT '{}',
			after_json     TEXT NOT NULL DEFAULT '{}',
			status         TEXT NOT NULL,
			sequence       INTEGER NOT NULL DEFAULT 0,
			created_at     TEXT NOT NULL
		)`); err != nil {
		t.Fatalf("create project_events: %v", err)
	}
	// The consumer's own business-effect table for the idempotency tests.
	if _, err := db.Exec(`CREATE TABLE consumer_effects (event_id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("create consumer_effects: %v", err)
	}
	return db
}

// seedEvent appends one fact row and its outbox event in a single
// transaction, mirroring the command gateway's atomic append.
func seedEvent(t *testing.T, db *sql.DB, id, workspace, eventType string, occurredAt time.Time) outbox.Event {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`
		INSERT INTO project_events (
			id, project_id, correlation_id, actor_kind, actor_name, origin,
			event_type, target_type, target_id, operation, before_json,
			after_json, status, sequence, created_at
		) VALUES (?, ?, ?, 'user', 'tester', 'test', ?, 'kv', ?, 'create',
			'{}', ?, 'succeeded', 0, ?)`,
		id, workspace, "corr-"+id, eventType, id,
		fmt.Sprintf(`{"key":%q}`, id), occurredAt.UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("insert fact: %v", err)
	}
	e := outbox.Event{
		ID:            id,
		WorkspaceID:   workspace,
		EventType:     eventType,
		SchemaVersion: 1,
		CorrelationID: "corr-" + id,
		CausationID:   "exec-" + id,
		SubjectRef:    "kv:" + id,
		ActorKind:     "user",
		ActorName:     "tester",
		Origin:        "test",
		OccurredAt:    occurredAt,
	}
	if err := outbox.AppendTx(tx, e); err != nil {
		t.Fatalf("AppendTx: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return e
}

// applyOnceConsumer applies a per-event business effect guarded by ClaimTx;
// its first `fail` invocations return an error to simulate consumer failure.
type applyOnceConsumer struct {
	db    *sql.DB
	id    string
	mu    sync.Mutex
	fail  int
	calls int
}

func (c *applyOnceConsumer) ConsumerID() string { return c.id }

func (c *applyOnceConsumer) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func (c *applyOnceConsumer) Handle(ctx context.Context, e outbox.Entry) error {
	c.mu.Lock()
	c.calls++
	shouldFail := c.fail > 0
	if shouldFail {
		c.fail--
	}
	c.mu.Unlock()
	if shouldFail {
		return errors.New("consumer boom")
	}
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	first, err := outbox.ClaimTx(tx, e.ID, c.ConsumerID())
	if err != nil {
		return err
	}
	if first {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO consumer_effects (event_id) VALUES (?)`, e.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func effectCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(1) FROM consumer_effects`).Scan(&n); err != nil {
		t.Fatalf("count effects: %v", err)
	}
	return n
}

func entryStatus(t *testing.T, d *outbox.Dispatcher, id string) outbox.Entry {
	t.Helper()
	entry, ok, err := d.Get(id)
	if err != nil || !ok {
		t.Fatalf("Get(%s): ok=%v err=%v", id, ok, err)
	}
	return entry
}

// ── append validation ───────────────────────────────────────────────────────

func TestAppendValidatesEnvelope(t *testing.T) {
	db := newOutboxDB(t)
	base := outbox.Event{
		ID:            "evt-1",
		WorkspaceID:   "ws-1",
		EventType:     "kv.created",
		SchemaVersion: 1,
		SubjectRef:    "kv:evt-1",
		ActorKind:     "user",
		OccurredAt:    time.Now().UTC(),
	}
	cases := []struct {
		name   string
		mutate func(*outbox.Event)
	}{
		{"missing id", func(e *outbox.Event) { e.ID = "" }},
		{"missing workspace", func(e *outbox.Event) { e.WorkspaceID = "" }},
		{"missing event type", func(e *outbox.Event) { e.EventType = "" }},
		{"zero schema version", func(e *outbox.Event) { e.SchemaVersion = 0 }},
		{"missing subject ref", func(e *outbox.Event) { e.SubjectRef = "" }},
		{"missing actor kind", func(e *outbox.Event) { e.ActorKind = "" }},
		{"zero occurred at", func(e *outbox.Event) { e.OccurredAt = time.Time{} }},
	}
	for _, tc := range cases {
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		e := base
		tc.mutate(&e)
		err = outbox.AppendTx(tx, e)
		tx.Rollback()
		if !errors.Is(err, outbox.ErrInvalidEvent) {
			t.Fatalf("%s: err=%v, want ErrInvalidEvent", tc.name, err)
		}
	}
}

func TestAppendRejectsDuplicateEventID(t *testing.T) {
	db := newOutboxDB(t)
	seedEvent(t, db, "evt-dup", "ws-1", "kv.created", time.Now().UTC())

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	dup := outbox.Event{
		ID: "evt-dup", WorkspaceID: "ws-1", EventType: "kv.created",
		SchemaVersion: 1, SubjectRef: "kv:evt-dup", ActorKind: "user",
		OccurredAt: time.Now().UTC(),
	}
	if err := outbox.AppendTx(tx, dup); err == nil {
		t.Fatal("duplicate event id accepted")
	}
}

// ── retryable delivery + idempotent consumption ─────────────────────────────

func TestConsumerFailureRetryableWithoutDuplicateEffect(t *testing.T) {
	db := newOutboxDB(t)
	d, err := outbox.NewDispatcher(db)
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	d.Backoff = func(attempts int) time.Duration { return 0 } // retry immediately

	seedEvent(t, db, "evt-r", "ws-1", "kv.created", time.Now().UTC())
	consumer := &applyOnceConsumer{db: db, id: "apply-once", fail: 1}
	if err := d.Register(consumer); err != nil {
		t.Fatalf("Register: %v", err)
	}
	ctx := context.Background()

	// Pass 1: the consumer fails → the event stays pending for retry, with
	// the failure recorded for diagnosis. No business effect was applied.
	results, err := d.DispatchPending(ctx)
	if err != nil {
		t.Fatalf("pass 1: %v", err)
	}
	if len(results) != 1 || results[0].EventID != "evt-r" ||
		results[0].Outcome != outbox.OutcomeRetry || results[0].Error == "" {
		t.Fatalf("pass 1 results=%+v, want one retry with error", results)
	}
	entry := entryStatus(t, d, "evt-r")
	if entry.Status != outbox.StatusPending || entry.Attempts != 1 ||
		entry.LastError == "" {
		t.Fatalf("after failed attempt: %+v", entry)
	}
	if n := effectCount(t, db); n != 0 {
		t.Fatalf("effects=%d after failed attempt, want 0", n)
	}

	// Pass 2: retry succeeds → delivered, receipt recorded, effect applied once.
	results, err = d.DispatchPending(ctx)
	if err != nil {
		t.Fatalf("pass 2: %v", err)
	}
	if len(results) != 1 || results[0].Outcome != outbox.OutcomeDelivered {
		t.Fatalf("pass 2 results=%+v, want delivered", results)
	}
	entry = entryStatus(t, d, "evt-r")
	if entry.Status != outbox.StatusDelivered || entry.DeliveredAt.IsZero() {
		t.Fatalf("after success: %+v", entry)
	}
	if n := effectCount(t, db); n != 1 {
		t.Fatalf("effects=%d, want 1", n)
	}

	// Simulate a redelivery (crash after the receipt was written but before
	// the delivered mark): the same event must not re-apply its effect.
	if _, err := db.Exec(`UPDATE outbox_events SET status = 'pending' WHERE event_id = 'evt-r'`); err != nil {
		t.Fatal(err)
	}
	callsBefore := consumer.callCount()
	results, err = d.DispatchPending(ctx)
	if err != nil {
		t.Fatalf("pass 3: %v", err)
	}
	if len(results) != 1 || results[0].Outcome != outbox.OutcomeDelivered {
		t.Fatalf("pass 3 results=%+v, want delivered via receipts", results)
	}
	if consumer.callCount() != callsBefore {
		t.Fatalf("consumer invoked %d extra times on redelivery, want 0", consumer.callCount()-callsBefore)
	}
	if n := effectCount(t, db); n != 1 {
		t.Fatalf("effects=%d after redelivery, want still 1", n)
	}
}

func TestReceiptMakesConsumptionIdempotent(t *testing.T) {
	db := newOutboxDB(t)

	// Claim inside the consumer's own transaction: first claim applies the
	// effect, every later claim reports already-consumed.
	apply := func(eventID, consumerID string) bool {
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		first, err := outbox.ClaimTx(tx, eventID, consumerID)
		if err != nil {
			t.Fatalf("ClaimTx: %v", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		return first
	}
	if !apply("evt-c", "consumer-a") {
		t.Fatal("first claim reported already consumed")
	}
	if apply("evt-c", "consumer-a") {
		t.Fatal("second claim consumed the same event twice")
	}
	// A different consumer still gets its own receipt.
	if !apply("evt-c", "consumer-b") {
		t.Fatal("second consumer blocked by the first consumer's receipt")
	}
	ok, err := outbox.HasReceipt(db, "evt-c", "consumer-a")
	if err != nil || !ok {
		t.Fatalf("HasReceipt: ok=%v err=%v", ok, err)
	}
}

// ── failure diagnosis ───────────────────────────────────────────────────────

func TestExhaustedRetriesSurfaceDiagnosis(t *testing.T) {
	db := newOutboxDB(t)
	d, err := outbox.NewDispatcher(db)
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	d.MaxAttempts = 3
	d.Backoff = func(attempts int) time.Duration { return 0 }

	seedEvent(t, db, "evt-f", "ws-1", "kv.created", time.Now().UTC())
	if err := d.Register(&applyOnceConsumer{db: db, id: "always-fail", fail: 1 << 30}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		results, err := d.DispatchPending(ctx)
		if err != nil {
			t.Fatalf("pass %d: %v", i+1, err)
		}
		if len(results) != 1 {
			t.Fatalf("pass %d results=%+v", i+1, results)
		}
		want := outbox.OutcomeRetry
		if i == 2 {
			want = outbox.OutcomeFailed
		}
		if results[0].Outcome != want {
			t.Fatalf("pass %d outcome=%s, want %s", i+1, results[0].Outcome, want)
		}
	}

	entry := entryStatus(t, d, "evt-f")
	if entry.Status != outbox.StatusFailed || entry.Attempts != 3 || entry.LastError == "" {
		t.Fatalf("terminal entry: %+v", entry)
	}

	// Failed events stop consuming retries: another pass finds nothing due.
	results, err := d.DispatchPending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("failed event retried again: %+v", results)
	}

	// The failure is visible through the diagnostics listing.
	page, err := d.List(outbox.Filter{Status: outbox.StatusFailed})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != "evt-f" || page.Items[0].LastError == "" {
		t.Fatalf("failed listing: %+v", page.Items)
	}
}

func TestBackoffSchedulesNextAttempt(t *testing.T) {
	db := newOutboxDB(t)
	d, err := outbox.NewDispatcher(db)
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	d.Backoff = func(attempts int) time.Duration { return time.Hour }

	seedEvent(t, db, "evt-b", "ws-1", "kv.created", time.Now().UTC())
	if err := d.Register(&applyOnceConsumer{db: db, id: "fail-once", fail: 1}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	ctx := context.Background()

	if _, err := d.DispatchPending(ctx); err != nil {
		t.Fatal(err)
	}
	entry := entryStatus(t, d, "evt-b")
	if entry.Status != outbox.StatusPending || entry.NextAttemptAt.IsZero() {
		t.Fatalf("after failure: %+v", entry)
	}
	// Not due yet: the next pass finds nothing.
	results, err := d.DispatchPending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("event retried before its backoff elapsed: %+v", results)
	}
	// Once the scheduled time passes, delivery resumes.
	if _, err := db.Exec(`UPDATE outbox_events SET next_attempt_at = '' WHERE event_id = 'evt-b'`); err != nil {
		t.Fatal(err)
	}
	results, err = d.DispatchPending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Outcome != outbox.OutcomeDelivered {
		t.Fatalf("after backoff elapsed: %+v", results)
	}
}

func TestDeliveryWithoutConsumersDelivers(t *testing.T) {
	db := newOutboxDB(t)
	d, err := outbox.NewDispatcher(db)
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	seedEvent(t, db, "evt-n", "ws-1", "kv.created", time.Now().UTC())
	results, err := d.DispatchPending(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Outcome != outbox.OutcomeDelivered {
		t.Fatalf("results=%+v, want delivered (no consumer waits for it)", results)
	}
}

func TestDeliveryCarriesFactFromAuditTable(t *testing.T) {
	db := newOutboxDB(t)
	d, err := outbox.NewDispatcher(db)
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	want := seedEvent(t, db, "evt-fact", "ws-1", "kv.created", time.Now().UTC())

	var got outbox.Entry
	recv := receiverFunc{
		id: "fact-check",
		fn: func(ctx context.Context, e outbox.Entry) error {
			got = e
			return nil
		},
	}
	if err := d.Register(recv); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, err := d.DispatchPending(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID || got.EventType != "kv.created" || got.SchemaVersion != 1 ||
		got.CorrelationID != "corr-evt-fact" || got.CausationID != "exec-evt-fact" ||
		got.SubjectRef != "kv:evt-fact" || got.ActorKind != "user" || got.ActorName != "tester" {
		t.Fatalf("delivered envelope mismatch: %+v", got)
	}
	if got.Fact.TargetType != "kv" || got.Fact.TargetID != "evt-fact" ||
		got.Fact.Operation != "create" || got.Fact.Status != "succeeded" ||
		string(got.Fact.After) != `{"key":"evt-fact"}` {
		t.Fatalf("delivered fact mismatch: %+v", got.Fact)
	}
}

type receiverFunc struct {
	id string
	fn func(ctx context.Context, e outbox.Entry) error
}

func (r receiverFunc) ConsumerID() string { return r.id }
func (r receiverFunc) Handle(ctx context.Context, e outbox.Entry) error {
	return r.fn(ctx, e)
}

// ── pagination & filtering ──────────────────────────────────────────────────

func TestListPaginationAndFilters(t *testing.T) {
	db := newOutboxDB(t)
	d, err := outbox.NewDispatcher(db)
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	base := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)
	const total = 7
	seen := map[string]bool{}
	for i := 0; i < total; i++ {
		id := fmt.Sprintf("evt-%02d", i)
		ws := "ws-1"
		if i == total-1 {
			ws = "ws-2"
		}
		seedEvent(t, db, id, ws, "kv.created", base.Add(time.Duration(i)*time.Second))
	}

	// Walk every page with limit 3: newest first, no duplicates, no gaps.
	cursor := ""
	pages := 0
	for {
		page, err := d.List(outbox.Filter{Limit: 3, Cursor: cursor})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		pages++
		if len(page.Items) == 0 || len(page.Items) > 3 {
			t.Fatalf("page %d has %d items, want 1..3", pages, len(page.Items))
		}
		for i, item := range page.Items {
			if i > 0 {
				prev := page.Items[i-1]
				if prev.OccurredAt.Before(item.OccurredAt) {
					t.Fatalf("page %d not newest-first at %s", pages, item.ID)
				}
			}
			if seen[item.ID] {
				t.Fatalf("event %s returned twice", item.ID)
			}
			seen[item.ID] = true
		}
		if !page.HasMore {
			if page.NextCursor != "" {
				t.Fatalf("last page carries a cursor: %q", page.NextCursor)
			}
			break
		}
		cursor = page.NextCursor
	}
	if pages != 3 || len(seen) != total {
		t.Fatalf("pages=%d seen=%d, want 3 pages covering %d events", pages, len(seen), total)
	}

	// Workspace + status filters.
	page, err := d.List(outbox.Filter{WorkspaceID: "ws-2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != "evt-06" {
		t.Fatalf("workspace filter: %+v", page.Items)
	}
	page, err = d.List(outbox.Filter{Status: outbox.StatusDelivered})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 0 {
		t.Fatalf("delivered filter returned %d pending events", len(page.Items))
	}

	// Malformed cursors and limits are rejected.
	if _, err := d.List(outbox.Filter{Cursor: "%%%"}); !errors.Is(err, outbox.ErrInvalidCursor) {
		t.Fatalf("bad cursor err=%v, want ErrInvalidCursor", err)
	}
	if _, err := d.List(outbox.Filter{Limit: 500}); !errors.Is(err, outbox.ErrInvalidCursor) {
		t.Fatalf("bad limit err=%v, want ErrInvalidCursor", err)
	}
}

func TestGetReportsMissingEvent(t *testing.T) {
	db := newOutboxDB(t)
	d, err := outbox.NewDispatcher(db)
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	_, ok, err := d.Get("evt-ghost")
	if err != nil || ok {
		t.Fatalf("Get(ghost): ok=%v err=%v", ok, err)
	}
}
