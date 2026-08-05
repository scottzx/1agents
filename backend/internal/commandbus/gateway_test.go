package commandbus_test

// Gateway contract tests (#323 acceptance): idempotent replays, stale
// expectedVersion rejection, distinct error codes for unregistered commands /
// missing permission / invalid payloads / domain rejections, the execution
// audit trail, and stable behavior under concurrent submissions and retries.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/scottzx/1Agents/backend/internal/commandbus"
)

// ── rig ─────────────────────────────────────────────────────────────────────

func newGateway(t *testing.T) (*commandbus.Gateway, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "commandbus.db")
	// Same DSN shape as meta.Open: immediate write-lock transactions that
	// queue on busy_timeout instead of failing with SQLITE_BUSY.
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
	if _, err := db.Exec(`CREATE TABLE test_kv (
		k TEXT PRIMARY KEY, value TEXT NOT NULL, version INTEGER NOT NULL
	)`); err != nil {
		t.Fatalf("create test_kv: %v", err)
	}
	bus, err := commandbus.New(db)
	if err != nil {
		t.Fatalf("New gateway: %v", err)
	}
	return bus, db
}

type kvCounters struct {
	mu      sync.Mutex
	creates int
	sets    int
}

func (c *kvCounters) add(field *int, n int) {
	c.mu.Lock()
	*field += n
	c.mu.Unlock()
}

// registerKVCommands installs a tiny test domain: kv.create / kv.set /
// kv.user_only / kv.policy / kv.fail_once.
func registerKVCommands(t *testing.T, bus *commandbus.Gateway, counters *kvCounters) {
	t.Helper()
	descriptors := []commandbus.Descriptor{
		{
			Contract:       "kv.create",
			SchemaVersions: []int{1},
			AllowedKinds:   []commandbus.ActorKind{commandbus.ActorUser, commandbus.ActorAgent},
			Handler: func(ctx context.Context, cmd commandbus.Command, tx *sql.Tx) (commandbus.Result, error) {
				var p struct {
					Key   string `json:"key"`
					Value string `json:"value"`
				}
				if err := cmd.PayloadObject(&p); err != nil {
					return commandbus.Result{}, err
				}
				if p.Key == "" {
					return commandbus.Result{}, commandbus.NewError(commandbus.CodeInvalidPayload, "payload: key is required")
				}
				if _, err := tx.Exec(`INSERT INTO test_kv (k, value, version) VALUES (?, ?, 1)`, p.Key, p.Value); err != nil {
					return commandbus.Result{}, commandbus.WrapError(commandbus.CodeDomainRejected, err, "key %q already exists", p.Key)
				}
				counters.add(&counters.creates, 1)
				payload, _ := json.Marshal(map[string]any{"key": p.Key, "value": p.Value})
				return commandbus.Result{
					Version: 1, EventID: "evt-" + p.Key + "-c1", TargetID: p.Key, Payload: payload,
					EventType: "kv.created", EventSchemaVersion: 1, SubjectRef: "kv:" + p.Key,
				}, nil
			},
		},
		{
			Contract:       "kv.set",
			SchemaVersions: []int{1},
			AllowedKinds:   []commandbus.ActorKind{commandbus.ActorUser, commandbus.ActorAgent},
			Handler: func(ctx context.Context, cmd commandbus.Command, tx *sql.Tx) (commandbus.Result, error) {
				var p struct {
					Key   string `json:"key"`
					Value string `json:"value"`
				}
				if err := cmd.PayloadObject(&p); err != nil {
					return commandbus.Result{}, err
				}
				if cmd.ExpectedVersion <= 0 {
					return commandbus.Result{}, commandbus.NewError(commandbus.CodeInvalidPayload, "expectedVersion is required")
				}
				res, err := tx.Exec(
					`UPDATE test_kv SET value = ?, version = version + 1 WHERE k = ? AND version = ?`,
					p.Value, p.Key, cmd.ExpectedVersion)
				if err != nil {
					return commandbus.Result{}, commandbus.WrapError(commandbus.CodeInternal, err, "update: %v", err)
				}
				if n, _ := res.RowsAffected(); n == 0 {
					var current int
					serr := tx.QueryRow(`SELECT version FROM test_kv WHERE k = ?`, p.Key).Scan(&current)
					if serr == sql.ErrNoRows {
						return commandbus.Result{}, commandbus.NewError(commandbus.CodeDomainRejected, "key %q not found", p.Key)
					}
					if serr != nil {
						return commandbus.Result{}, commandbus.WrapError(commandbus.CodeInternal, serr, "lookup: %v", serr)
					}
					return commandbus.Result{}, commandbus.NewError(commandbus.CodeVersionConflict,
						"expected version %d, current is %d", cmd.ExpectedVersion, current)
				}
				counters.add(&counters.sets, 1)
				payload, _ := json.Marshal(map[string]any{"key": p.Key, "value": p.Value})
				return commandbus.Result{
					Version:  cmd.ExpectedVersion + 1,
					EventID:  fmt.Sprintf("evt-%s-s%d", p.Key, cmd.ExpectedVersion+1),
					TargetID: p.Key, Payload: payload,
					EventType: "kv.updated", EventSchemaVersion: 1, SubjectRef: "kv:" + p.Key,
				}, nil
			},
		},
		{
			Contract:       "kv.user_only",
			SchemaVersions: []int{1},
			AllowedKinds:   []commandbus.ActorKind{commandbus.ActorUser},
			Handler: func(ctx context.Context, cmd commandbus.Command, tx *sql.Tx) (commandbus.Result, error) {
				return commandbus.Result{}, nil
			},
		},
		{
			Contract:       "kv.policy",
			SchemaVersions: []int{1},
			AllowedKinds:   []commandbus.ActorKind{commandbus.ActorUser, commandbus.ActorAgent},
			Authorize: func(cmd commandbus.Command) error {
				var p struct {
					Value string `json:"value"`
				}
				if err := cmd.PayloadObject(&p); err != nil || p.Value == "" {
					return nil // handler rejects malformed payloads
				}
				if p.Value == "forbidden" {
					return commandbus.NewError(commandbus.CodePermissionDenied, "value %q needs human approval", p.Value)
				}
				return nil
			},
			Handler: func(ctx context.Context, cmd commandbus.Command, tx *sql.Tx) (commandbus.Result, error) {
				return commandbus.Result{}, nil
			},
		},
	}
	for _, d := range descriptors {
		if err := bus.Register(d); err != nil {
			t.Fatalf("register %s: %v", d.Contract, err)
		}
	}
}

func userCommand(contract, key, payload string) commandbus.Command {
	return commandbus.Command{
		Contract:       contract,
		SchemaVersion:  1,
		WorkspaceID:    "ws-1",
		Actor:          commandbus.Actor{Kind: commandbus.ActorUser, Name: "tester", Origin: "http"},
		IdempotencyKey: key,
		Payload:        json.RawMessage(payload),
	}
}

// ── envelope validation ─────────────────────────────────────────────────────

func TestEnvelopeValidation(t *testing.T) {
	valid := userCommand("kv.create", "k", `{}`)
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid envelope rejected: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*commandbus.Command)
	}{
		{"bad contract", func(c *commandbus.Command) { c.Contract = "Not-Dotted" }},
		{"empty contract", func(c *commandbus.Command) { c.Contract = "" }},
		{"zero schema version", func(c *commandbus.Command) { c.SchemaVersion = 0 }},
		{"empty workspace", func(c *commandbus.Command) { c.WorkspaceID = "" }},
		{"bad actor kind", func(c *commandbus.Command) { c.Actor.Kind = "robot" }},
		{"empty actor name", func(c *commandbus.Command) { c.Actor.Name = "" }},
		{"empty idempotency key", func(c *commandbus.Command) { c.IdempotencyKey = "" }},
		{"negative version", func(c *commandbus.Command) { c.ExpectedVersion = -1 }},
		{"invalid payload json", func(c *commandbus.Command) { c.Payload = json.RawMessage("{oops") }},
	}
	for _, tc := range cases {
		cmd := valid
		tc.mutate(&cmd)
		err := cmd.Validate()
		if !commandbus.IsCode(err, commandbus.CodeInvalidPayload) {
			t.Fatalf("%s: err=%v, want %s", tc.name, err, commandbus.CodeInvalidPayload)
		}
	}
}

// ── distinct error codes ────────────────────────────────────────────────────

func TestDispatchErrorCodeClasses(t *testing.T) {
	bus, _ := newGateway(t)
	counters := &kvCounters{}
	registerKVCommands(t, bus, counters)
	ctx := context.Background()

	// Unregistered command.
	_, err := bus.Dispatch(ctx, userCommand("kv.ghost", "u1", `{}`))
	if !commandbus.IsCode(err, commandbus.CodeUnknownCommand) {
		t.Fatalf("unknown command err=%v, want %s", err, commandbus.CodeUnknownCommand)
	}

	// Actor kind not permitted.
	agentCmd := userCommand("kv.user_only", "p1", `{}`)
	agentCmd.Actor = commandbus.Actor{Kind: commandbus.ActorAgent, Name: "codex"}
	_, err = bus.Dispatch(ctx, agentCmd)
	if !commandbus.IsCode(err, commandbus.CodePermissionDenied) {
		t.Fatalf("kind denial err=%v, want %s", err, commandbus.CodePermissionDenied)
	}

	// Fine-grained policy denial.
	_, err = bus.Dispatch(ctx, userCommand("kv.policy", "p2", `{"value":"forbidden"}`))
	if !commandbus.IsCode(err, commandbus.CodePermissionDenied) {
		t.Fatalf("policy denial err=%v, want %s", err, commandbus.CodePermissionDenied)
	}

	// Invalid payload (handler-level).
	_, err = bus.Dispatch(ctx, userCommand("kv.create", "p3", `{"value":"no key"}`))
	if !commandbus.IsCode(err, commandbus.CodeInvalidPayload) {
		t.Fatalf("invalid payload err=%v, want %s", err, commandbus.CodeInvalidPayload)
	}

	// Unsupported schema version.
	v2 := userCommand("kv.create", "p4", `{"key":"x"}`)
	v2.SchemaVersion = 2
	_, err = bus.Dispatch(ctx, v2)
	if !commandbus.IsCode(err, commandbus.CodeInvalidPayload) {
		t.Fatalf("unsupported version err=%v, want %s", err, commandbus.CodeInvalidPayload)
	}

	// Domain rejection (duplicate key).
	if _, err := bus.Dispatch(ctx, userCommand("kv.create", "d1", `{"key":"dup","value":"1"}`)); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err = bus.Dispatch(ctx, userCommand("kv.create", "d2", `{"key":"dup","value":"2"}`))
	if !commandbus.IsCode(err, commandbus.CodeDomainRejected) {
		t.Fatalf("domain rejection err=%v, want %s", err, commandbus.CodeDomainRejected)
	}

	// Optimistic concurrency conflict.
	_, err = bus.Dispatch(ctx, withVersion(userCommand("kv.set", "c1", `{"key":"dup","value":"3"}`), 99))
	if !commandbus.IsCode(err, commandbus.CodeVersionConflict) {
		t.Fatalf("version conflict err=%v, want %s", err, commandbus.CodeVersionConflict)
	}

	// Every class is a distinct machine-readable code.
	seen := map[commandbus.Code]bool{}
	for _, code := range []commandbus.Code{
		commandbus.CodeUnknownCommand, commandbus.CodePermissionDenied,
		commandbus.CodeInvalidPayload, commandbus.CodeDomainRejected,
		commandbus.CodeVersionConflict,
	} {
		if seen[code] {
			t.Fatalf("duplicate code %s", code)
		}
		seen[code] = true
	}
}

func withVersion(cmd commandbus.Command, v int) commandbus.Command {
	cmd.ExpectedVersion = v
	return cmd
}

// ── idempotency ─────────────────────────────────────────────────────────────

func TestIdempotentReplayProducesEffectOnce(t *testing.T) {
	bus, db := newGateway(t)
	counters := &kvCounters{}
	registerKVCommands(t, bus, counters)
	ctx := context.Background()

	cmd := userCommand("kv.create", "once", `{"key":"a","value":"v1"}`)
	first, err := bus.Dispatch(ctx, cmd)
	if err != nil {
		t.Fatalf("first dispatch: %v", err)
	}
	if first.Status != commandbus.ResultSucceeded || first.EventID != "evt-a-c1" || first.Version != 1 {
		t.Fatalf("first result mismatch: %+v", first)
	}

	// Duplicate submissions — any number — replay the stored result.
	for i := 0; i < 3; i++ {
		again, err := bus.Dispatch(ctx, cmd)
		if err != nil {
			t.Fatalf("replay %d: %v", i, err)
		}
		if again.Status != commandbus.ResultReplayed {
			t.Fatalf("replay %d status=%s, want replayed", i, again.Status)
		}
		if again.EventID != first.EventID || again.Version != first.Version ||
			string(again.Payload) != string(first.Payload) || again.TargetID != first.TargetID {
			t.Fatalf("replay %d differs from original:\n got %+v\nwant %+v", i, again, first)
		}
	}

	if counters.creates != 1 {
		t.Fatalf("handler executed %d times, want exactly 1", counters.creates)
	}
	var rows int
	if err := db.QueryRow(`SELECT COUNT(1) FROM test_kv`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("test_kv rows=%d, want 1 (effect produced once)", rows)
	}
}

func TestIdempotencyKeyReuseAcrossContractsRejected(t *testing.T) {
	bus, _ := newGateway(t)
	registerKVCommands(t, bus, &kvCounters{})
	ctx := context.Background()

	if _, err := bus.Dispatch(ctx, userCommand("kv.create", "shared", `{"key":"a","value":"1"}`)); err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err := bus.Dispatch(ctx, withVersion(userCommand("kv.set", "shared", `{"key":"a","value":"2"}`), 1))
	if !commandbus.IsCode(err, commandbus.CodeInvalidPayload) {
		t.Fatalf("cross-contract reuse err=%v, want %s", err, commandbus.CodeInvalidPayload)
	}
}

func TestFailedCommandRetryableWithSameKey(t *testing.T) {
	bus, _ := newGateway(t)
	counters := &kvCounters{}
	registerKVCommands(t, bus, counters)
	ctx := context.Background()

	if _, err := bus.Dispatch(ctx, userCommand("kv.create", "r1", `{"key":"r","value":"1"}`)); err != nil {
		t.Fatalf("create: %v", err)
	}
	// Stale version → rejected; the failed attempt must NOT consume the key.
	bad := withVersion(userCommand("kv.set", "r2", `{"key":"r","value":"2"}`), 99)
	if _, err := bus.Dispatch(ctx, bad); !commandbus.IsCode(err, commandbus.CodeVersionConflict) {
		t.Fatalf("stale set err=%v", err)
	}
	// Retry under the same key with the corrected version succeeds once.
	retry := withVersion(userCommand("kv.set", "r2", `{"key":"r","value":"2"}`), 1)
	res, err := bus.Dispatch(ctx, retry)
	if err != nil || res.Status != commandbus.ResultSucceeded {
		t.Fatalf("retry after failure: res=%+v err=%v", res, err)
	}
	// And replays afterwards.
	again, err := bus.Dispatch(ctx, retry)
	if err != nil || again.Status != commandbus.ResultReplayed || again.EventID != res.EventID {
		t.Fatalf("post-retry replay: res=%+v err=%v", again, err)
	}
	if counters.sets != 1 {
		t.Fatalf("set executed %d times, want 1", counters.sets)
	}
}

// ── audit trail ─────────────────────────────────────────────────────────────

func TestExecutionAuditTrail(t *testing.T) {
	bus, _ := newGateway(t)
	registerKVCommands(t, bus, &kvCounters{})
	ctx := context.Background()

	cmd := userCommand("kv.create", "audit-1", `{"key":"aud","value":"1"}`)
	cmd.CorrelationID = "corr-1"
	if _, err := bus.Dispatch(ctx, cmd); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	// One rejected attempt is audited too.
	if _, err := bus.Dispatch(ctx, userCommand("kv.ghost", "audit-2", `{}`)); err == nil {
		t.Fatal("ghost command should be rejected")
	}

	rows, err := bus.ListExecutions(commandbus.ExecutionFilter{WorkspaceID: "ws-1"})
	if err != nil {
		t.Fatalf("ListExecutions: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("audit rows=%d, want 2", len(rows))
	}
	// Newest first: the rejection.
	rej := rows[0]
	if rej.Status != "rejected" || rej.ErrorCode != string(commandbus.CodeUnknownCommand) ||
		rej.Contract != "kv.ghost" || rej.DurationMS < 0 {
		t.Fatalf("rejection audit mismatch: %+v", rej)
	}
	ok := rows[1]
	if ok.Status != "succeeded" || ok.Contract != "kv.create" ||
		ok.ActorKind != "user" || ok.ActorName != "tester" ||
		ok.TargetID != "aud" || ok.NewVersion != 1 || ok.EventID != "evt-aud-c1" ||
		ok.IdempotencyKey != "audit-1" || ok.CorrelationID != "corr-1" ||
		ok.DurationMS < 0 || ok.CreatedAt.IsZero() || len(ok.Result) == 0 {
		t.Fatalf("success audit mismatch: %+v", ok)
	}

	// Target-scoped audit query.
	scoped, err := bus.ListExecutions(commandbus.ExecutionFilter{WorkspaceID: "ws-1", TargetID: "aud"})
	if err != nil || len(scoped) != 1 || scoped[0].TargetID != "aud" {
		t.Fatalf("scoped audit rows=%d err=%v", len(scoped), err)
	}
}

// ── concurrency ─────────────────────────────────────────────────────────────

func TestConcurrentDuplicateSubmissionsProduceOneEffect(t *testing.T) {
	bus, db := newGateway(t)
	counters := &kvCounters{}
	registerKVCommands(t, bus, counters)
	ctx := context.Background()

	const workers = 8
	results := make([]commandbus.Result, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], errs[i] = bus.Dispatch(ctx,
				userCommand("kv.create", "race-once", `{"key":"race","value":"1"}`))
		}(i)
	}
	close(start)
	wg.Wait()

	succeeded, replayed := 0, 0
	var winnerEvent string
	for i := range results {
		if errs[i] != nil {
			t.Fatalf("worker %d error: %v", i, errs[i])
		}
		switch results[i].Status {
		case commandbus.ResultSucceeded:
			succeeded++
			winnerEvent = results[i].EventID
		case commandbus.ResultReplayed:
			replayed++
		default:
			t.Fatalf("worker %d unexpected status %s", i, results[i].Status)
		}
		if results[i].EventID != winnerEvent && winnerEvent != "" {
			t.Fatalf("worker %d observed a different event id: %q vs %q", i, results[i].EventID, winnerEvent)
		}
	}
	if succeeded != 1 || replayed != workers-1 {
		t.Fatalf("succeeded=%d replayed=%d, want 1/%d", succeeded, replayed, workers-1)
	}
	if counters.creates != 1 {
		t.Fatalf("handler executed %d times, want 1", counters.creates)
	}
	var rows int
	if err := db.QueryRow(`SELECT COUNT(1) FROM test_kv`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("test_kv rows=%d, want 1", rows)
	}
}

func TestConcurrentConflictingWritesSingleWinner(t *testing.T) {
	bus, _ := newGateway(t)
	counters := &kvCounters{}
	registerKVCommands(t, bus, counters)
	ctx := context.Background()

	if _, err := bus.Dispatch(ctx, userCommand("kv.create", "seed", `{"key":"c","value":"0"}`)); err != nil {
		t.Fatalf("seed: %v", err)
	}

	const workers = 8
	statuses := make([]string, workers)
	codes := make([]commandbus.Code, workers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cmd := withVersion(
				userCommand("kv.set", fmt.Sprintf("race-set-%d", i), fmt.Sprintf(`{"key":"c","value":"v%d"}`, i)), 1)
			res, err := bus.Dispatch(ctx, cmd)
			if err == nil {
				statuses[i] = res.Status
				return
			}
			codes[i], _ = commandbus.CodeOf(err)
		}(i)
	}
	close(start)
	wg.Wait()

	winners := 0
	for i := range statuses {
		if statuses[i] == commandbus.ResultSucceeded {
			winners++
			continue
		}
		if codes[i] != commandbus.CodeVersionConflict {
			t.Fatalf("worker %d code=%s, want version_conflict", i, codes[i])
		}
	}
	if winners != 1 {
		t.Fatalf("winners=%d, want exactly 1", winners)
	}
	if counters.sets != 1 {
		t.Fatalf("set executed %d times, want 1", counters.sets)
	}
}

// ── registry ────────────────────────────────────────────────────────────────

func TestRegistryRejectsDuplicatesAndMalformed(t *testing.T) {
	bus, _ := newGateway(t)
	noop := commandbus.HandlerFunc(func(ctx context.Context, cmd commandbus.Command, tx *sql.Tx) (commandbus.Result, error) {
		return commandbus.Result{}, nil
	})
	if err := bus.Register(commandbus.Descriptor{Contract: "x.y", SchemaVersions: []int{1}, Handler: noop}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := bus.Register(commandbus.Descriptor{Contract: "x.y", SchemaVersions: []int{1}, Handler: noop}); err == nil {
		t.Fatal("duplicate registration accepted")
	}
	if err := bus.Register(commandbus.Descriptor{Contract: "NoDots", SchemaVersions: []int{1}, Handler: noop}); err == nil {
		t.Fatal("malformed contract accepted")
	}
	if err := bus.Register(commandbus.Descriptor{Contract: "x.z", SchemaVersions: nil, Handler: noop}); err == nil {
		t.Fatal("versionless contract accepted")
	}
	if err := bus.Register(commandbus.Descriptor{Contract: "x.w", SchemaVersions: []int{1}, Handler: nil}); err == nil {
		t.Fatal("nil handler accepted")
	}
	contracts := bus.Contracts()
	if len(contracts) != 1 || contracts[0] != "x.y" {
		t.Fatalf("contracts=%v", contracts)
	}
}

// ── outbox integration (#324) ───────────────────────────────────────────────

func outboxCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(1) FROM outbox_events`).Scan(&n); err != nil {
		t.Fatalf("count outbox_events: %v", err)
	}
	return n
}

func TestDispatchAppendsOutboxEventInCommandTransaction(t *testing.T) {
	bus, db := newGateway(t)
	registerKVCommands(t, bus, &kvCounters{})
	ctx := context.Background()

	cmd := userCommand("kv.create", "ob-1", `{"key":"ob","value":"1"}`)
	cmd.CorrelationID = "corr-ob"
	res, err := bus.Dispatch(ctx, cmd)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	// Exactly one outbox row, committed atomically with the business write,
	// carrying the full §5.3 envelope.
	var (
		eventID, ws, eventType                                        string
		schemaVersion                                                 int
		corrID, causationID, subjectRef, actorKind, actorName, origin string
		status                                                        string
	)
	err = db.QueryRow(`
		SELECT event_id, workspace_id, event_type, schema_version, correlation_id,
			causation_id, subject_ref, actor_kind, actor_name, origin, status
		FROM outbox_events`).Scan(
		&eventID, &ws, &eventType, &schemaVersion, &corrID, &causationID,
		&subjectRef, &actorKind, &actorName, &origin, &status)
	if err != nil {
		t.Fatalf("read outbox row: %v", err)
	}
	if eventID != res.EventID || ws != "ws-1" || eventType != "kv.created" ||
		schemaVersion != 1 || corrID != "corr-ob" || causationID == "" ||
		subjectRef != "kv:ob" || actorKind != "user" || actorName != "tester" ||
		origin != "http" || status != "pending" {
		t.Fatalf("outbox envelope mismatch: id=%s ws=%s type=%s v=%d corr=%s caus=%s subj=%s actor=%s/%s origin=%s status=%s",
			eventID, ws, eventType, schemaVersion, corrID, causationID, subjectRef, actorKind, actorName, origin, status)
	}

	// The causation id is the command execution that produced the event.
	execs, err := bus.ListExecutions(commandbus.ExecutionFilter{WorkspaceID: "ws-1"})
	if err != nil {
		t.Fatalf("ListExecutions: %v", err)
	}
	matched := false
	for _, e := range execs {
		if e.Status == "succeeded" && e.EventID == res.EventID && e.ExecutionID == causationID {
			matched = true
		}
	}
	if !matched {
		t.Fatalf("no succeeded execution matches event causation %q: %+v", causationID, execs)
	}

	// An idempotent replay appends no second outbox row.
	again, err := bus.Dispatch(ctx, cmd)
	if err != nil || again.Status != commandbus.ResultReplayed {
		t.Fatalf("replay: res=%+v err=%v", again, err)
	}
	if n := outboxCount(t, db); n != 1 {
		t.Fatalf("outbox rows=%d after replay, want 1", n)
	}
}

func TestBusinessFailureProducesNoOutboxEvent(t *testing.T) {
	bus, db := newGateway(t)
	registerKVCommands(t, bus, &kvCounters{})
	// A handler that writes domain state and then fails: both the write and
	// the outbox row must roll back.
	if err := bus.Register(commandbus.Descriptor{
		Contract:       "kv.write_then_fail",
		SchemaVersions: []int{1},
		AllowedKinds:   []commandbus.ActorKind{commandbus.ActorUser},
		Handler: func(ctx context.Context, cmd commandbus.Command, tx *sql.Tx) (commandbus.Result, error) {
			if _, err := tx.Exec(`INSERT INTO test_kv (k, value, version) VALUES ('boom', '1', 1)`); err != nil {
				return commandbus.Result{}, commandbus.WrapError(commandbus.CodeInternal, err, "insert: %v", err)
			}
			return commandbus.Result{}, commandbus.NewError(commandbus.CodeDomainRejected, "business rule violated after write")
		},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	_, err := bus.Dispatch(context.Background(), userCommand("kv.write_then_fail", "bf-1", `{}`))
	if !commandbus.IsCode(err, commandbus.CodeDomainRejected) {
		t.Fatalf("dispatch err=%v, want domain_rejected", err)
	}
	var kvRows int
	if err := db.QueryRow(`SELECT COUNT(1) FROM test_kv`).Scan(&kvRows); err != nil {
		t.Fatal(err)
	}
	if kvRows != 0 {
		t.Fatalf("test_kv rows=%d, want 0 (business write rolled back)", kvRows)
	}
	if n := outboxCount(t, db); n != 0 {
		t.Fatalf("outbox rows=%d, want 0 (failed write produces no event)", n)
	}
}

func TestOutboxFailureRollsBackBusinessTransaction(t *testing.T) {
	bus, db := newGateway(t)
	registerKVCommands(t, bus, &kvCounters{})
	ctx := context.Background()

	// Sabotage the outbox table so the append inside the command transaction
	// fails: the business write and the idempotency claim roll back with it.
	if _, err := db.Exec(`DROP TABLE outbox_events`); err != nil {
		t.Fatalf("drop outbox_events: %v", err)
	}
	_, err := bus.Dispatch(ctx, userCommand("kv.create", "of-1", `{"key":"victim","value":"1"}`))
	if !commandbus.IsCode(err, commandbus.CodeInternal) {
		t.Fatalf("dispatch err=%v, want internal", err)
	}
	var kvRows int
	if err := db.QueryRow(`SELECT COUNT(1) FROM test_kv`).Scan(&kvRows); err != nil {
		t.Fatal(err)
	}
	if kvRows != 0 {
		t.Fatalf("test_kv rows=%d, want 0 (outbox failure rolls back the business write)", kvRows)
	}
	var idemRows int
	if err := db.QueryRow(`SELECT COUNT(1) FROM command_idempotency`).Scan(&idemRows); err != nil {
		t.Fatal(err)
	}
	if idemRows != 0 {
		t.Fatalf("command_idempotency rows=%d, want 0 (nothing committed)", idemRows)
	}
}

func TestEnvelopeRequiredWhenHandlerProducesEvent(t *testing.T) {
	bus, db := newGateway(t)
	// A handler that reports an event id but omits the envelope violates the
	// outbox contract; the whole command fails and rolls back.
	if err := bus.Register(commandbus.Descriptor{
		Contract:       "kv.no_envelope",
		SchemaVersions: []int{1},
		AllowedKinds:   []commandbus.ActorKind{commandbus.ActorUser},
		Handler: func(ctx context.Context, cmd commandbus.Command, tx *sql.Tx) (commandbus.Result, error) {
			if _, err := tx.Exec(`INSERT INTO test_kv (k, value, version) VALUES ('env', '1', 1)`); err != nil {
				return commandbus.Result{}, err
			}
			return commandbus.Result{Version: 1, EventID: "evt-env", TargetID: "env"}, nil
		},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	_, err := bus.Dispatch(context.Background(), userCommand("kv.no_envelope", "ne-1", `{}`))
	if !commandbus.IsCode(err, commandbus.CodeInternal) {
		t.Fatalf("dispatch err=%v, want internal", err)
	}
	var kvRows int
	if err := db.QueryRow(`SELECT COUNT(1) FROM test_kv`).Scan(&kvRows); err != nil {
		t.Fatal(err)
	}
	if kvRows != 0 {
		t.Fatalf("test_kv rows=%d, want 0", kvRows)
	}
	if n := outboxCount(t, db); n != 0 {
		t.Fatalf("outbox rows=%d, want 0", n)
	}
}

func TestCausationChainReconstructsCommandFlow(t *testing.T) {
	bus, db := newGateway(t)
	registerKVCommands(t, bus, &kvCounters{})
	ctx := context.Background()

	// Step 1 of the flow.
	first := userCommand("kv.create", "chain-1", `{"key":"c1","value":"1"}`)
	first.CorrelationID = "corr-chain"
	res1, err := bus.Dispatch(ctx, first)
	if err != nil {
		t.Fatalf("first dispatch: %v", err)
	}

	// Step 2: a command caused by the first event, in the same correlation.
	second := withVersion(userCommand("kv.set", "chain-2", `{"key":"c1","value":"2"}`), 1)
	second.CorrelationID = "corr-chain"
	second.CausationID = res1.EventID
	res2, err := bus.Dispatch(ctx, second)
	if err != nil {
		t.Fatalf("second dispatch: %v", err)
	}

	outboxEnvelope := func(id string) (causation, correlation string) {
		t.Helper()
		err := db.QueryRow(
			`SELECT causation_id, correlation_id FROM outbox_events WHERE event_id = ?`, id).
			Scan(&causation, &correlation)
		if err != nil {
			t.Fatalf("read outbox %s: %v", id, err)
		}
		return causation, correlation
	}
	execs, err := bus.ListExecutions(commandbus.ExecutionFilter{WorkspaceID: "ws-1"})
	if err != nil {
		t.Fatalf("ListExecutions: %v", err)
	}
	execByEvent := map[string]commandbus.Execution{}
	corrCount := 0
	for _, e := range execs {
		if e.Status == "succeeded" {
			execByEvent[e.EventID] = e
			if e.CorrelationID == "corr-chain" {
				corrCount++
			}
		}
	}
	if corrCount != 2 {
		t.Fatalf("succeeded executions under corr-chain=%d, want 2", corrCount)
	}

	// Walk the chain backwards: event2 → causing execution → the event that
	// caused that execution → its causing execution → the first command.
	caus2, corr2 := outboxEnvelope(res2.EventID)
	if corr2 != "corr-chain" {
		t.Fatalf("event2 correlation=%q, want corr-chain", corr2)
	}
	exec2, ok := execByEvent[res2.EventID]
	if !ok || exec2.ExecutionID != caus2 || exec2.Contract != "kv.set" {
		t.Fatalf("event2 causation %q does not match its execution %+v", caus2, exec2)
	}
	if exec2.CausationID != res1.EventID {
		t.Fatalf("execution2 causation=%q, want event1 id %q", exec2.CausationID, res1.EventID)
	}
	caus1, corr1 := outboxEnvelope(res1.EventID)
	if corr1 != "corr-chain" {
		t.Fatalf("event1 correlation=%q, want corr-chain", corr1)
	}
	exec1, ok := execByEvent[res1.EventID]
	if !ok || exec1.ExecutionID != caus1 || exec1.Contract != "kv.create" {
		t.Fatalf("event1 causation %q does not match its execution %+v", caus1, exec1)
	}
}
