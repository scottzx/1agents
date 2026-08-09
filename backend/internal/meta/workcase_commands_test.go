package meta

// WorkCase command tests (#323): the registered commands are the only write
// path for case state — idempotent, version-guarded, permission-checked and
// fully audited.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/commandbus"
	"github.com/scottzx/1Agents/backend/internal/domainref"
	"github.com/scottzx/1Agents/backend/internal/outbox"
)

// caseBusRig wires a gateway with the WorkCase commands over a fresh DB.
func caseBusRig(t *testing.T, projects ...string) (*commandbus.Gateway, *WorkCaseStore, *DB) {
	t.Helper()
	db := newTestDB(t)
	for i, p := range projects {
		if err := db.EnsureProject(p, fmt.Sprintf("Project %d", i), "/tmp/"+p); err != nil {
			t.Fatalf("EnsureProject %s: %v", p, err)
		}
	}
	bus, err := commandbus.New(db.SQL())
	if err != nil {
		t.Fatalf("commandbus.New: %v", err)
	}
	store := NewWorkCaseStore(db)
	if err := RegisterWorkCaseCommands(bus, store); err != nil {
		t.Fatalf("RegisterWorkCaseCommands: %v", err)
	}
	return bus, store, db
}

func caseCommand(contract, ws, key, actorKind, actorName string, expectedVersion int, payload any) commandbus.Command {
	raw, _ := json.Marshal(payload)
	return commandbus.Command{
		Contract:        contract,
		SchemaVersion:   1,
		WorkspaceID:     ws,
		Actor:           commandbus.Actor{Kind: commandbus.ActorKind(actorKind), Name: actorName, Origin: "http"},
		IdempotencyKey:  key,
		ExpectedVersion: expectedVersion,
		Payload:         raw,
	}
}

func dispatchCase(t *testing.T, bus *commandbus.Gateway, cmd commandbus.Command) commandbus.Result {
	t.Helper()
	res, err := bus.Dispatch(context.Background(), cmd)
	if err != nil {
		t.Fatalf("dispatch %s: %v", cmd.Contract, err)
	}
	return res
}

func createCaseViaCommand(t *testing.T, bus *commandbus.Gateway, ws, key, title string) WorkCase {
	t.Helper()
	res := dispatchCase(t, bus, caseCommand(CommandWorkCaseCreate, ws, key, "user", "user", 0,
		map[string]any{"title": title}))
	var c WorkCase
	if err := json.Unmarshal(res.Payload, &c); err != nil {
		t.Fatalf("unmarshal created case: %v", err)
	}
	return c
}

// ── happy path through the gateway ─────────────────────────────────────────

func TestWorkCaseCommandLifecycle(t *testing.T) {
	bus, store, _ := caseBusRig(t, "proj-1")
	ctx := context.Background()

	created := createCaseViaCommand(t, bus, "proj-1", "lc-create", "命令创建的 Case")
	if created.Version != 1 || created.Status != CaseStatusOpen {
		t.Fatalf("created: %+v", created)
	}

	// update (field edit, no phase/status possible in the payload shape).
	res := dispatchCase(t, bus, caseCommand(CommandWorkCaseUpdate, "proj-1", "lc-update", "user", "user", 1,
		map[string]any{"caseId": created.ID, "objective": "通过命令更新"}))
	if res.Version != 2 {
		t.Fatalf("update version=%d, want 2", res.Version)
	}

	// phase advances only through the dedicated command.
	res = dispatchCase(t, bus, caseCommand(CommandWorkCaseSetPhase, "proj-1", "lc-phase", "user", "user", 2,
		map[string]any{"caseId": created.ID, "currentPhase": "qualification"}))
	if res.Version != 3 {
		t.Fatalf("phase version=%d, want 3", res.Version)
	}
	got, ok, err := store.Get(created.ID)
	if err != nil || !ok || got.CurrentPhase != "qualification" {
		t.Fatalf("phase not stored: ok=%v err=%v case=%+v", ok, err, got)
	}

	// suspend → reopen → close (terminal, user actor).
	res = dispatchCase(t, bus, caseCommand(CommandWorkCaseTransition, "proj-1", "lc-suspend", "user", "user", 3,
		map[string]any{"caseId": created.ID, "status": "suspended"}))
	res = dispatchCase(t, bus, caseCommand(CommandWorkCaseTransition, "proj-1", "lc-reopen", "user", "user", int(res.Version),
		map[string]any{"caseId": created.ID, "status": "open"}))
	res = dispatchCase(t, bus, caseCommand(CommandWorkCaseTransition, "proj-1", "lc-close", "user", "user", int(res.Version),
		map[string]any{"caseId": created.ID, "status": "closed", "reason": "won"}))
	var closed WorkCase
	if err := json.Unmarshal(res.Payload, &closed); err != nil {
		t.Fatal(err)
	}
	if closed.Status != CaseStatusClosed || closed.CloseReason != "won" || closed.ClosedAt == nil {
		t.Fatalf("closed: %+v", closed)
	}

	// domain rejection: terminal states never move again.
	_, err = bus.Dispatch(ctx, caseCommand(CommandWorkCaseTransition, "proj-1", "lc-revive", "user", "user", int(res.Version),
		map[string]any{"caseId": created.ID, "status": "open"}))
	if !commandbus.IsCode(err, commandbus.CodeDomainRejected) || !errors.Is(err, ErrInvalidCaseTransition) {
		t.Fatalf("terminal regression err=%v, want domain_rejected/ErrInvalidCaseTransition", err)
	}

	// delete (user actor).
	res = dispatchCase(t, bus, caseCommand(CommandWorkCaseDelete, "proj-1", "lc-delete", "user", "user", 0,
		map[string]any{"caseId": created.ID}))
	if _, ok, _ := store.Get(created.ID); ok {
		t.Fatal("case still present after delete command")
	}
}

// ── idempotency ─────────────────────────────────────────────────────────────

func TestWorkCaseCommandIdempotentCreate(t *testing.T) {
	bus, store, _ := caseBusRig(t, "proj-1")

	cmd := caseCommand(CommandWorkCaseCreate, "proj-1", "same-key", "user", "user", 0,
		map[string]any{"title": "幂等创建"})
	first := dispatchCase(t, bus, cmd)

	for i := 0; i < 3; i++ {
		again, err := bus.Dispatch(context.Background(), cmd)
		if err != nil {
			t.Fatalf("replay %d: %v", i, err)
		}
		if again.Status != commandbus.ResultReplayed || again.EventID != first.EventID ||
			again.Version != first.Version || string(again.Payload) != string(first.Payload) {
			t.Fatalf("replay %d differs: %+v vs %+v", i, again, first)
		}
	}

	cases, err := store.List("proj-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 1 {
		t.Fatalf("cases=%d, want exactly 1 (effect produced once)", len(cases))
	}
}

// ── optimistic concurrency ──────────────────────────────────────────────────

func TestWorkCaseCommandStaleVersionRejected(t *testing.T) {
	bus, _, _ := caseBusRig(t, "proj-1")
	created := createCaseViaCommand(t, bus, "proj-1", "stale-create", "并发对象")

	dispatchCase(t, bus, caseCommand(CommandWorkCaseUpdate, "proj-1", "stale-bump", "user", "user", 1,
		map[string]any{"caseId": created.ID, "objective": "v2"}))

	_, err := bus.Dispatch(context.Background(), caseCommand(CommandWorkCaseUpdate, "proj-1", "stale-hit", "user", "user", 1,
		map[string]any{"caseId": created.ID, "objective": "conflict"}))
	if !commandbus.IsCode(err, commandbus.CodeVersionConflict) || !errors.Is(err, ErrCaseVersionConflict) {
		t.Fatalf("stale update err=%v, want version_conflict", err)
	}

	// Missing expectedVersion on a mutating command → invalid payload.
	_, err = bus.Dispatch(context.Background(), caseCommand(CommandWorkCaseSetPhase, "proj-1", "stale-nover", "user", "user", 0,
		map[string]any{"caseId": created.ID, "currentPhase": "x"}))
	if !commandbus.IsCode(err, commandbus.CodeInvalidPayload) {
		t.Fatalf("missing version err=%v, want invalid_payload", err)
	}
}

// ── error code classes ──────────────────────────────────────────────────────

func TestWorkCaseCommandErrorCodeClasses(t *testing.T) {
	bus, _, _ := caseBusRig(t, "proj-1")
	ctx := context.Background()
	created := createCaseViaCommand(t, bus, "proj-1", "codes-create", "错误码")

	// Unregistered command.
	_, err := bus.Dispatch(ctx, caseCommand("workcase.ghost", "proj-1", "codes-ghost", "user", "user", 0, nil))
	if !commandbus.IsCode(err, commandbus.CodeUnknownCommand) {
		t.Fatalf("ghost err=%v, want unknown_command", err)
	}

	// Invalid payload (missing title).
	_, err = bus.Dispatch(ctx, caseCommand(CommandWorkCaseCreate, "proj-1", "codes-notitle", "user", "user", 0,
		map[string]any{"objective": "no title"}))
	if !commandbus.IsCode(err, commandbus.CodeInvalidPayload) {
		t.Fatalf("missing title err=%v, want invalid_payload", err)
	}

	// Domain rejection: unknown case.
	_, err = bus.Dispatch(ctx, caseCommand(CommandWorkCaseUpdate, "proj-1", "codes-missing", "user", "user", 1,
		map[string]any{"caseId": "does-not-exist", "objective": "x"}))
	if !commandbus.IsCode(err, commandbus.CodeDomainRejected) || !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown case err=%v, want domain_rejected wrapping not found", err)
	}

	// Domain rejection: illegal lifecycle move.
	_, err = bus.Dispatch(ctx, caseCommand(CommandWorkCaseTransition, "proj-1", "codes-badmove", "user", "user", 1,
		map[string]any{"caseId": created.ID, "status": "open"}))
	if !commandbus.IsCode(err, commandbus.CodeDomainRejected) || !errors.Is(err, ErrInvalidCaseTransition) {
		t.Fatalf("same-state transition err=%v, want domain_rejected", err)
	}

	// Invalid payload: malformed lifecycle status.
	_, err = bus.Dispatch(ctx, caseCommand(CommandWorkCaseTransition, "proj-1", "codes-badstatus", "user", "user", 1,
		map[string]any{"caseId": created.ID, "status": "won"}))
	if !commandbus.IsCode(err, commandbus.CodeInvalidPayload) {
		t.Fatalf("bad status err=%v, want invalid_payload", err)
	}

	// Cross-workspace access is a domain rejection (project mismatch).
	foreign := caseCommand(CommandWorkCaseUpdate, "ghost-ws", "codes-foreign", "user", "user", 1,
		map[string]any{"caseId": created.ID, "objective": "x"})
	_, err = bus.Dispatch(ctx, foreign)
	if !commandbus.IsCode(err, commandbus.CodeDomainRejected) || !errors.Is(err, ErrProjectMismatch) {
		t.Fatalf("foreign workspace err=%v, want domain_rejected/project mismatch", err)
	}
}

// ── actor permission policy ─────────────────────────────────────────────────

func TestWorkCaseCommandActorPermissions(t *testing.T) {
	bus, store, _ := caseBusRig(t, "proj-1")
	ctx := context.Background()
	created := createCaseViaCommand(t, bus, "proj-1", "perm-create", "权限")

	// Agent may update and suspend…
	dispatchCase(t, bus, caseCommand(CommandWorkCaseUpdate, "proj-1", "perm-agent-update", "agent", "codex", 1,
		map[string]any{"caseId": created.ID, "objective": "agent 可写"}))
	res := dispatchCase(t, bus, caseCommand(CommandWorkCaseTransition, "proj-1", "perm-agent-suspend", "agent", "codex", 2,
		map[string]any{"caseId": created.ID, "status": "suspended"}))

	// …but never terminate a case (重要人工决定不可被 Agent 静默覆盖).
	_, err := bus.Dispatch(ctx, caseCommand(CommandWorkCaseTransition, "proj-1", "perm-agent-close", "agent", "codex", int(res.Version),
		map[string]any{"caseId": created.ID, "status": "closed"}))
	if !commandbus.IsCode(err, commandbus.CodePermissionDenied) {
		t.Fatalf("agent terminal transition err=%v, want permission_denied", err)
	}
	// Same for function executors.
	_, err = bus.Dispatch(ctx, caseCommand(CommandWorkCaseTransition, "proj-1", "perm-fn-cancel", "function", "worker", int(res.Version),
		map[string]any{"caseId": created.ID, "status": "cancelled"}))
	if !commandbus.IsCode(err, commandbus.CodePermissionDenied) {
		t.Fatalf("function terminal transition err=%v, want permission_denied", err)
	}
	// Agent may not delete either.
	_, err = bus.Dispatch(ctx, caseCommand(CommandWorkCaseDelete, "proj-1", "perm-agent-delete", "agent", "codex", 0,
		map[string]any{"caseId": created.ID}))
	if !commandbus.IsCode(err, commandbus.CodePermissionDenied) {
		t.Fatalf("agent delete err=%v, want permission_denied", err)
	}

	// Human actor may terminate.
	res = dispatchCase(t, bus, caseCommand(CommandWorkCaseTransition, "proj-1", "perm-human-reopen", "human", "scott", int(res.Version),
		map[string]any{"caseId": created.ID, "status": "open"}))
	dispatchCase(t, bus, caseCommand(CommandWorkCaseTransition, "proj-1", "perm-human-close", "human", "scott", int(res.Version),
		map[string]any{"caseId": created.ID, "status": "closed", "reason": "人工关闭"}))

	got, _, err := store.Get(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != CaseStatusClosed || got.CloseReason != "人工关闭" {
		t.Fatalf("final state mismatch: %+v", got)
	}
}

// ── audit trail ─────────────────────────────────────────────────────────────

func TestWorkCaseCommandAuditTrail(t *testing.T) {
	bus, _, _ := caseBusRig(t, "proj-1")
	created := createCaseViaCommand(t, bus, "proj-1", "audit-create", "审计")
	phase := caseCommand(CommandWorkCaseSetPhase, "proj-1", "audit-phase", "user", "user", 1,
		map[string]any{"caseId": created.ID, "currentPhase": "phase-a"})
	phase.TargetID = created.ID
	dispatchCase(t, bus, phase)
	// One rejected attempt is recorded too — with its target, so rejections
	// are auditable against the case they tried to change.
	conflict := caseCommand(CommandWorkCaseUpdate, "proj-1", "audit-conflict", "user", "user", 1,
		map[string]any{"caseId": created.ID, "objective": "stale"})
	conflict.TargetID = created.ID
	if _, err := bus.Dispatch(context.Background(), conflict); err == nil {
		t.Fatal("stale update should fail")
	}

	rows, err := bus.ListExecutions(commandbus.ExecutionFilter{WorkspaceID: "proj-1", TargetID: created.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("case audit rows=%d, want 3: %+v", len(rows), rows)
	}
	// Newest first: the rejected conflict.
	if rows[0].Status != "rejected" || rows[0].ErrorCode != string(commandbus.CodeVersionConflict) ||
		rows[0].Contract != CommandWorkCaseUpdate {
		t.Fatalf("rejected audit mismatch: %+v", rows[0])
	}
	for _, row := range rows[1:] {
		if row.ActorKind != "user" || row.ActorName != "user" || row.TargetID != created.ID ||
			row.DurationMS < 0 || row.CreatedAt.IsZero() || row.EventID == "" {
			t.Fatalf("audit row missing fields: %+v", row)
		}
	}
	if rows[1].Contract != CommandWorkCaseSetPhase || rows[1].NewVersion != 2 {
		t.Fatalf("phase audit mismatch: %+v", rows[1])
	}
	if rows[2].Contract != CommandWorkCaseCreate || rows[2].NewVersion != 1 {
		t.Fatalf("create audit mismatch: %+v", rows[2])
	}
}

// ── concurrency: conflicting transitions & duplicate creates ───────────────

func TestWorkCaseCommandConcurrentUpdatesSingleWinner(t *testing.T) {
	bus, store, _ := caseBusRig(t, "proj-1")
	created := createCaseViaCommand(t, bus, "proj-1", "cc-create", "并发更新")

	const workers = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	statuses := make([]string, workers)
	codes := make([]commandbus.Code, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			cmd := caseCommand(CommandWorkCaseUpdate, "proj-1", fmt.Sprintf("cc-race-%d", i), "user", "user", 1,
				map[string]any{"caseId": created.ID, "objective": fmt.Sprintf("v%d", i)})
			cmd.TargetID = created.ID
			res, err := bus.Dispatch(context.Background(), cmd)
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
	got, _, err := store.Get(created.ID)
	if err != nil || got.Version != 2 {
		t.Fatalf("case after race: %+v err=%v", got, err)
	}
}

func TestWorkCaseCommandConcurrentDuplicateCreates(t *testing.T) {
	bus, store, _ := caseBusRig(t, "proj-1")

	const workers = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make([]commandbus.Result, workers)
	errs := make([]error, workers)
	cmd := caseCommand(CommandWorkCaseCreate, "proj-1", "dup-create", "user", "user", 0,
		map[string]any{"title": "只创建一次"})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], errs[i] = bus.Dispatch(context.Background(), cmd)
		}(i)
	}
	close(start)
	wg.Wait()

	succeeded := 0
	var caseID, eventID string
	for i := range results {
		if errs[i] != nil {
			t.Fatalf("worker %d: %v", i, errs[i])
		}
		if results[i].Status == commandbus.ResultSucceeded {
			succeeded++
			caseID, eventID = results[i].TargetID, results[i].EventID
		}
	}
	if succeeded != 1 {
		t.Fatalf("succeeded=%d, want 1", succeeded)
	}
	for i := range results {
		if results[i].TargetID != caseID || results[i].EventID != eventID {
			t.Fatalf("worker %d observed a different case/event: %+v", i, results[i])
		}
	}
	cases, err := store.List("proj-1", "")
	if err != nil || len(cases) != 1 {
		t.Fatalf("cases=%d err=%v, want exactly 1", len(cases), err)
	}
}

// ── phase guard at the store level ─────────────────────────────────────────

func TestWorkCasePhaseOnlyThroughCommand(t *testing.T) {
	bus, store, db := caseBusRig(t, "proj-1")
	created := createCaseViaCommand(t, bus, "proj-1", "guard-create", "相位守卫")

	// The generic Update path carries no phase field at all (compile-time
	// guarantee), and SetPhaseInTx is only reachable inside a command
	// transaction — a direct call still demands its own transaction and
	// version, mirroring what the gateway provides.
	tx, err := db.sql.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetPhaseInTx(tx, "proj-1", created.ID, "direct", created.Version); err != nil {
		tx.Rollback()
		t.Fatalf("SetPhaseInTx: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	got, _, err := store.Get(created.ID)
	if err != nil || got.CurrentPhase != "direct" {
		t.Fatalf("phase not applied: %+v err=%v", got, err)
	}

	// Update (any patch) can never move the phase.
	title := "换个标题"
	updated, err := store.Update("proj-1", created.ID, WorkCasePatch{Title: &title}, got.Version,
		caseEvent("proj-1", created.ID, "update"))
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.CurrentPhase != "direct" {
		t.Fatalf("update moved the phase: %q", updated.CurrentPhase)
	}
}

// ── outbox events (#324) ────────────────────────────────────────────────────

func TestWorkCaseCommandOutboxEnvelope(t *testing.T) {
	bus, _, db := caseBusRig(t, "proj-1")
	disp, err := outbox.NewDispatcher(db.SQL())
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}

	cmd := caseCommand(CommandWorkCaseCreate, "proj-1", "ob-create", "user", "user", 0,
		map[string]any{"title": "Outbox Case"})
	cmd.CorrelationID = "corr-case"
	res := dispatchCase(t, bus, cmd)

	entry, ok, err := disp.Get(res.EventID)
	if err != nil || !ok {
		t.Fatalf("outbox entry for %s: ok=%v err=%v", res.EventID, ok, err)
	}
	wantSubject, err := domainref.NewCaseRef("proj-1", res.TargetID, 0)
	if err != nil {
		t.Fatalf("NewCaseRef: %v", err)
	}
	if entry.Status != outbox.StatusPending ||
		entry.EventType != "work_case.create" || entry.SchemaVersion != 1 ||
		entry.WorkspaceID != "proj-1" || entry.CorrelationID != "corr-case" ||
		entry.CausationID == "" || entry.SubjectRef != wantSubject.String() ||
		entry.ActorKind != "user" || entry.ActorName != "user" ||
		entry.OccurredAt.IsZero() {
		t.Fatalf("outbox envelope mismatch: %+v (want subject %s)", entry, wantSubject.String())
	}
	// The fact is read from the ProjectEvent audit row — no parallel copy.
	if entry.Fact.TargetType != "work_case" || entry.Fact.TargetID != res.TargetID ||
		entry.Fact.Operation != "create" || entry.Fact.Status != string(ProjectEventSucceeded) {
		t.Fatalf("fact mismatch: %+v", entry.Fact)
	}

	// The causation id is the command execution that committed the fact.
	execs, err := bus.ListExecutions(commandbus.ExecutionFilter{WorkspaceID: "proj-1", TargetID: res.TargetID})
	if err != nil {
		t.Fatalf("ListExecutions: %v", err)
	}
	matched := false
	for _, e := range execs {
		if e.Status == "succeeded" && e.EventID == res.EventID && e.ExecutionID == entry.CausationID {
			matched = true
		}
	}
	if !matched {
		t.Fatalf("causation %q does not match any succeeded execution: %+v", entry.CausationID, execs)
	}

	// An idempotent replay appends no second outbox row.
	replay, err := bus.Dispatch(context.Background(), cmd)
	if err != nil || replay.Status != commandbus.ResultReplayed {
		t.Fatalf("replay: %+v err=%v", replay, err)
	}
	var rows int
	if err := db.SQL().QueryRow(`SELECT COUNT(1) FROM outbox_events`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("outbox rows=%d after replay, want 1", rows)
	}
}

func TestWorkCaseDomainRejectionProducesNoOutbox(t *testing.T) {
	bus, _, db := caseBusRig(t, "proj-1")
	created := createCaseViaCommand(t, bus, "proj-1", "ob-seed", "Outbox 拒绝测试")

	// Stale expectedVersion → domain rejection: no outbox row is appended.
	stale := caseCommand(CommandWorkCaseUpdate, "proj-1", "ob-stale", "user", "user", 99,
		map[string]any{"caseId": created.ID, "title": "改不动"})
	_, err := bus.Dispatch(context.Background(), stale)
	if !commandbus.IsCode(err, commandbus.CodeVersionConflict) {
		t.Fatalf("stale update err=%v, want version_conflict", err)
	}
	var rows int
	if err := db.SQL().QueryRow(`SELECT COUNT(1) FROM outbox_events`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 { // only the create's event
		t.Fatalf("outbox rows=%d, want 1 (rejected command appends nothing)", rows)
	}
}

func TestWorkCaseOutboxCausationChain(t *testing.T) {
	bus, _, db := caseBusRig(t, "proj-1")
	disp, err := outbox.NewDispatcher(db.SQL())
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}

	create := caseCommand(CommandWorkCaseCreate, "proj-1", "chain-create", "user", "user", 0,
		map[string]any{"title": "因果链"})
	create.CorrelationID = "corr-flow"
	created := dispatchCase(t, bus, create)

	transition := caseCommand(CommandWorkCaseTransition, "proj-1", "chain-suspend", "user", "user", created.Version,
		map[string]any{"caseId": created.TargetID, "status": "suspended"})
	transition.CorrelationID = "corr-flow"
	transition.CausationID = created.EventID
	suspended := dispatchCase(t, bus, transition)

	// Walk backwards: transition event → its execution → the create event.
	ev2, ok, err := disp.Get(suspended.EventID)
	if err != nil || !ok {
		t.Fatalf("transition event: ok=%v err=%v", ok, err)
	}
	if ev2.EventType != "work_case.transition" || ev2.CorrelationID != "corr-flow" {
		t.Fatalf("transition envelope: %+v", ev2)
	}
	execs, err := bus.ListExecutions(commandbus.ExecutionFilter{WorkspaceID: "proj-1", Contract: CommandWorkCaseTransition})
	if err != nil {
		t.Fatal(err)
	}
	var exec2 *commandbus.Execution
	for i := range execs {
		if execs[i].EventID == suspended.EventID && execs[i].Status == "succeeded" {
			exec2 = &execs[i]
		}
	}
	if exec2 == nil || exec2.ExecutionID != ev2.CausationID {
		t.Fatalf("transition causation %q not matched by its execution %+v", ev2.CausationID, execs)
	}
	if exec2.CausationID != created.EventID {
		t.Fatalf("execution causation=%q, want create event %q", exec2.CausationID, created.EventID)
	}
	ev1, ok, err := disp.Get(created.EventID)
	if err != nil || !ok {
		t.Fatalf("create event: ok=%v err=%v", ok, err)
	}
	if ev1.CorrelationID != "corr-flow" || ev1.CausationID == "" {
		t.Fatalf("create envelope: %+v", ev1)
	}
}
