package meta

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// caseEvent builds a minimal valid work_case ProjectEvent for store calls.
func caseEvent(projectID, caseID, op string) ProjectEvent {
	return ProjectEvent{
		ProjectID:  projectID,
		ActorKind:  "user",
		ActorName:  "user",
		Origin:     "http",
		EventType:  "work_case." + op,
		TargetType: "work_case",
		TargetID:   caseID,
		Operation:  op,
		Status:     ProjectEventSucceeded,
	}
}

// newCaseFixture creates a project row and returns a store bound to it.
func newCaseFixture(t *testing.T, db *DB, projectID string) *WorkCaseStore {
	t.Helper()
	if err := db.EnsureProject(projectID, "Project "+projectID, "/tmp/"+projectID); err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	return NewWorkCaseStore(db)
}

// mustCreateCase creates a case with a pre-assigned id so the accompanying
// event carries a valid target.
func mustCreateCase(t *testing.T, s *WorkCaseStore, projectID, title string) WorkCase {
	t.Helper()
	id := newID()
	c, err := s.Create(projectID, WorkCase{ID: id, Title: title}, caseEvent(projectID, id, "create"))
	if err != nil {
		t.Fatalf("Create(%s): %v", title, err)
	}
	return c
}

// ── migration: 可重复执行并兼容现有数据库 ─────────────────────────────────────

func TestWorkCaseSchemaMigrationRerunnableAndCompatible(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.db")

	// Fresh database: Open applies the full chain including the v30 tables.
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open fresh: %v", err)
	}
	if err := db.EnsureProject("proj-1", "Project One", "/tmp/proj-1"); err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	store := NewWorkCaseStore(db)
	mustCreateCase(t, store, "proj-1", "seed case")

	// Simulate a pre-#322 production database that a sibling branch already
	// bumped to user_version 29: drop the WorkCase tables and reset the
	// version counter behind meta's back.
	if _, err := db.sql.Exec(`
		DROP TABLE work_case_links;
		DROP TABLE work_cases;
		PRAGMA user_version = 29;
	`); err != nil {
		t.Fatalf("simulate legacy db: %v", err)
	}
	db.Close()

	// Re-Open must heal the half-migrated database: tables recreated,
	// pre-existing unrelated data untouched.
	db, err = Open(path)
	if err != nil {
		t.Fatalf("Open healed: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if ok, err := db.tableExists("work_cases"); err != nil || !ok {
		t.Fatalf("work_cases healed: ok=%v err=%v", ok, err)
	}
	if ok, err := db.tableExists("work_case_links"); err != nil || !ok {
		t.Fatalf("work_case_links healed: ok=%v err=%v", ok, err)
	}
	if _, ok, err := db.GetProject("proj-1"); err != nil || !ok {
		t.Fatalf("pre-existing project lost after re-migration: ok=%v err=%v", ok, err)
	}

	// The migration is re-runnable: opening the same file again never fails
	// and the store accepts writes. The seed case was dropped with its table,
	// so the list starts empty.
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatalf("Open again: %v", err)
	}
	store = NewWorkCaseStore(db)
	cases, err := store.List("proj-1", "")
	if err != nil {
		t.Fatalf("List after reopen: %v", err)
	}
	if len(cases) != 0 {
		t.Fatalf("expected empty list after simulated drop, got %d", len(cases))
	}
	if _, err := store.Create("proj-1", WorkCase{ID: newID(), Title: "post-heal"}, caseEvent("proj-1", "post-heal", "create")); err != nil {
		t.Fatalf("Create after heal: %v", err)
	}
}

// ── CRUD ────────────────────────────────────────────────────────────────────

func TestWorkCaseCreateGetListRoundTrip(t *testing.T) {
	db := newTestDB(t)
	store := newCaseFixture(t, db, "proj-1")

	due := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	in := WorkCase{
		ID:              newID(),
		Title:           "长期业务事项",
		Objective:       "跑通一条从线索到验收的闭环",
		CaseDefinition:  "delivery@1",
		Owner:           "scott",
		CreatedBy:       "user",
		CurrentPhase:    "qualification-research",
		PrimarySubject:  "presales:opportunity:42",
		SubjectRefs:     []string{"presales:evidence:7", "sources:feishu:feishu_chat"},
		Participants:    []CaseParticipant{{ID: "u1", Kind: "user", Name: "Scott", Role: "owner"}, {ID: "claudecode", Kind: "agent", Role: "executor"}},
		ExpectedCloseAt: &due,
	}
	created, err := store.Create("proj-1", in, caseEvent("proj-1", in.ID, "create"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Status != CaseStatusOpen || created.Version != 1 {
		t.Fatalf("bad create result: %+v", created)
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatalf("timestamps not stamped: %+v", created)
	}

	got, ok, err := store.Get(created.ID)
	if err != nil || !ok {
		t.Fatalf("Get: ok=%v err=%v", ok, err)
	}
	if got.Title != in.Title || got.Objective != in.Objective ||
		got.CaseDefinition != in.CaseDefinition || got.Owner != in.Owner ||
		got.CurrentPhase != in.CurrentPhase || got.PrimarySubject != in.PrimarySubject {
		t.Fatalf("scalar round-trip mismatch: %+v", got)
	}
	if len(got.SubjectRefs) != 2 || got.SubjectRefs[0] != in.SubjectRefs[0] || got.SubjectRefs[1] != in.SubjectRefs[1] {
		t.Fatalf("subjectRefs round-trip mismatch: %+v", got.SubjectRefs)
	}
	if len(got.Participants) != 2 || got.Participants[0].ID != "u1" || got.Participants[1].Role != "executor" {
		t.Fatalf("participants round-trip mismatch: %+v", got.Participants)
	}
	if got.ExpectedCloseAt == nil || !got.ExpectedCloseAt.Equal(due) {
		t.Fatalf("expectedCloseAt round-trip mismatch: %+v", got.ExpectedCloseAt)
	}

	// CaseRef round-trip (§4.3 identity; domainref from #321).
	ref, err := got.Ref()
	if err != nil {
		t.Fatalf("Ref: %v", err)
	}
	if ref.String() != "case:proj-1:"+got.ID {
		t.Fatalf("case ref = %q", ref.String())
	}

	// List: all, then filtered by status.
	second := mustCreateCase(t, store, "proj-1", "second")
	if _, err := store.Transition("proj-1", second.ID, CaseStatusCancelled, "dup", second.Version, caseEvent("proj-1", second.ID, "transition")); err != nil {
		t.Fatalf("cancel second: %v", err)
	}
	all, err := store.List("proj-1", "")
	if err != nil || len(all) != 2 {
		t.Fatalf("List all: n=%d err=%v", len(all), err)
	}
	if all[0].ID != second.ID {
		t.Fatalf("List should be newest-first, got %q first", all[0].ID)
	}
	open, err := store.List("proj-1", CaseStatusOpen)
	if err != nil || len(open) != 1 || open[0].ID != created.ID {
		t.Fatalf("List open filter: n=%d err=%v", len(open), err)
	}
	if _, err := store.List("proj-1", CaseStatus("bogus")); !errors.Is(err, ErrInvalidProjectEvent) {
		t.Fatalf("bogus status filter err=%v", err)
	}

	// Delete removes the case and its links.
	if _, err := store.Link("proj-1", created.ID, CaseLinkArtifact, ".artifacts/app/a.png", created.Version, caseEvent("proj-1", created.ID, "link")); err != nil {
		t.Fatalf("Link: %v", err)
	}
	if err := store.Delete("proj-1", created.ID, caseEvent("proj-1", created.ID, "delete")); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok, err := store.Get(created.ID); err != nil || ok {
		t.Fatalf("Get after delete: ok=%v err=%v", ok, err)
	}
	links, err := store.ListLinks(created.ID)
	if err != nil || len(links) != 0 {
		t.Fatalf("links after delete: n=%d err=%v", len(links), err)
	}
	if err := store.Delete("proj-1", created.ID, caseEvent("proj-1", created.ID, "delete")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("double delete err=%v", err)
	}
}

func TestWorkCaseCreateValidation(t *testing.T) {
	db := newTestDB(t)
	store := newCaseFixture(t, db, "proj-1")

	if _, err := store.Create("proj-1", WorkCase{ID: newID()}, caseEvent("proj-1", "x", "create")); !errors.Is(err, ErrInvalidProjectEvent) {
		t.Fatalf("missing title err=%v", err)
	}
	if _, err := store.Create("no-such-project", WorkCase{ID: newID(), Title: "t"}, caseEvent("no-such-project", "x", "create")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown project err=%v", err)
	}
	if _, err := store.Create("proj-1", WorkCase{ID: newID(), Title: "t", PrimarySubject: "not a ref"}, caseEvent("proj-1", "x", "create")); err == nil {
		t.Fatal("malformed primarySubject accepted")
	}
	if _, err := store.Create("proj-1", WorkCase{ID: newID(), Title: "t", SubjectRefs: []string{"presales:opportunity:1", "::"}}, caseEvent("proj-1", "x", "create")); err == nil {
		t.Fatal("malformed subjectRef accepted")
	}
	if _, err := store.Create("proj-1", WorkCase{ID: newID(), Title: "t", Participants: []CaseParticipant{{ID: "u1"}}}, caseEvent("proj-1", "x", "create")); !errors.Is(err, ErrInvalidProjectEvent) {
		t.Fatalf("participant without kind err=%v", err)
	}
	// The workspace id must be CaseRef-safe (domainref identifier rules).
	if err := db.EnsureProject("UPPER-CASE", "Upper", "/tmp/upper"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("UPPER-CASE", WorkCase{ID: newID(), Title: "t"}, caseEvent("UPPER-CASE", "x", "create")); err == nil {
		t.Fatal("CaseRef-unsafe workspace id accepted")
	}
}

// ── lifecycle: 合法转换通过，非法终态回退被拒绝 ────────────────────────────────

func TestWorkCaseLegalTransitions(t *testing.T) {
	db := newTestDB(t)
	store := newCaseFixture(t, db, "proj-1")

	// open → suspended → open → closed
	c := mustCreateCase(t, store, "proj-1", "lifecycle")
	c, err := store.Transition("proj-1", c.ID, CaseStatusSuspended, "waiting on customer", c.Version, caseEvent("proj-1", c.ID, "transition"))
	if err != nil || c.Status != CaseStatusSuspended {
		t.Fatalf("open→suspended: status=%s err=%v", c.Status, err)
	}
	if c.ClosedAt != nil || c.CloseReason != "" {
		t.Fatalf("suspend must not stamp close fields: %+v", c)
	}
	c, err = store.Transition("proj-1", c.ID, CaseStatusOpen, "resumed", c.Version, caseEvent("proj-1", c.ID, "transition"))
	if err != nil || c.Status != CaseStatusOpen {
		t.Fatalf("suspended→open: status=%s err=%v", c.Status, err)
	}
	c, err = store.Transition("proj-1", c.ID, CaseStatusClosed, "delivered", c.Version, caseEvent("proj-1", c.ID, "transition"))
	if err != nil || c.Status != CaseStatusClosed {
		t.Fatalf("open→closed: status=%s err=%v", c.Status, err)
	}
	if c.ClosedAt == nil || c.CloseReason != "delivered" {
		t.Fatalf("close must stamp ClosedAt/CloseReason: %+v", c)
	}

	// open → cancelled
	c2 := mustCreateCase(t, store, "proj-1", "cancel me")
	c2, err = store.Transition("proj-1", c2.ID, CaseStatusCancelled, "duplicate", c2.Version, caseEvent("proj-1", c2.ID, "transition"))
	if err != nil || c2.Status != CaseStatusCancelled || c2.CloseReason != "duplicate" {
		t.Fatalf("open→cancelled: %+v err=%v", c2, err)
	}

	// suspended → cancelled
	c3 := mustCreateCase(t, store, "proj-1", "suspend then cancel")
	c3, _ = store.Transition("proj-1", c3.ID, CaseStatusSuspended, "", c3.Version, caseEvent("proj-1", c3.ID, "transition"))
	c3, err = store.Transition("proj-1", c3.ID, CaseStatusCancelled, "dropped", c3.Version, caseEvent("proj-1", c3.ID, "transition"))
	if err != nil || c3.Status != CaseStatusCancelled {
		t.Fatalf("suspended→cancelled: %+v err=%v", c3, err)
	}
}

func TestWorkCaseTerminalRegressionRejected(t *testing.T) {
	db := newTestDB(t)
	store := newCaseFixture(t, db, "proj-1")

	closed := mustCreateCase(t, store, "proj-1", "closed case")
	closed, err := store.Transition("proj-1", closed.ID, CaseStatusClosed, "done", closed.Version, caseEvent("proj-1", closed.ID, "transition"))
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	for _, to := range []CaseStatus{CaseStatusOpen, CaseStatusSuspended, CaseStatusClosed, CaseStatusCancelled} {
		_, err := store.Transition("proj-1", closed.ID, to, "regression", closed.Version, caseEvent("proj-1", closed.ID, "transition"))
		if !errors.Is(err, ErrInvalidCaseTransition) {
			t.Fatalf("closed→%s err=%v, want ErrInvalidCaseTransition", to, err)
		}
	}

	cancelled := mustCreateCase(t, store, "proj-1", "cancelled case")
	cancelled, err = store.Transition("proj-1", cancelled.ID, CaseStatusCancelled, "dup", cancelled.Version, caseEvent("proj-1", cancelled.ID, "transition"))
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	for _, to := range []CaseStatus{CaseStatusOpen, CaseStatusSuspended, CaseStatusClosed, CaseStatusCancelled} {
		_, err := store.Transition("proj-1", cancelled.ID, to, "regression", cancelled.Version, caseEvent("proj-1", cancelled.ID, "transition"))
		if !errors.Is(err, ErrInvalidCaseTransition) {
			t.Fatalf("cancelled→%s err=%v, want ErrInvalidCaseTransition", to, err)
		}
	}

	// Same-state no-ops and unknown statuses are rejected too.
	live := mustCreateCase(t, store, "proj-1", "live case")
	if _, err := store.Transition("proj-1", live.ID, CaseStatusOpen, "", live.Version, caseEvent("proj-1", live.ID, "transition")); !errors.Is(err, ErrInvalidCaseTransition) {
		t.Fatalf("open→open err=%v", err)
	}
	if _, err := store.Transition("proj-1", live.ID, CaseStatus("won"), "", live.Version, caseEvent("proj-1", live.ID, "transition")); !errors.Is(err, ErrInvalidCaseTransition) {
		t.Fatalf("unknown status err=%v", err)
	}
	// Terminal guard holds at the value level as well.
	if !CaseStatusClosed.Terminal() || !CaseStatusCancelled.Terminal() || CaseStatusOpen.Terminal() {
		t.Fatal("Terminal() semantics wrong")
	}
}

// ── optimistic versioning: 递增正确 ─────────────────────────────────────────

func TestWorkCaseOptimisticVersioning(t *testing.T) {
	db := newTestDB(t)
	store := newCaseFixture(t, db, "proj-1")
	c := mustCreateCase(t, store, "proj-1", "versioned")
	if c.Version != 1 {
		t.Fatalf("create version=%d, want 1", c.Version)
	}

	// Stale versions are rejected on every mutating path (and bump nothing).
	title := "renamed"
	if _, err := store.Update("proj-1", c.ID, WorkCasePatch{Title: &title}, 99, caseEvent("proj-1", c.ID, "update")); !errors.Is(err, ErrCaseVersionConflict) {
		t.Fatalf("stale update err=%v", err)
	}
	if _, err := store.Transition("proj-1", c.ID, CaseStatusSuspended, "", 99, caseEvent("proj-1", c.ID, "transition")); !errors.Is(err, ErrCaseVersionConflict) {
		t.Fatalf("stale transition err=%v", err)
	}
	if _, err := store.Link("proj-1", c.ID, CaseLinkArtifact, "a", 99, caseEvent("proj-1", c.ID, "link")); !errors.Is(err, ErrCaseVersionConflict) {
		t.Fatalf("stale link err=%v", err)
	}
	if _, err := store.Unlink("proj-1", c.ID, CaseLinkArtifact, "a", 99, caseEvent("proj-1", c.ID, "unlink")); !errors.Is(err, ErrCaseVersionConflict) {
		t.Fatalf("stale unlink err=%v", err)
	}
	if got, _, _ := store.Get(c.ID); got.Version != 1 {
		t.Fatalf("rejected mutations changed the version: %d", got.Version)
	}

	// Each accepted mutation bumps the version by exactly one.
	want := 1
	expectVersion := func(label string, v int) {
		t.Helper()
		want++
		if v != want {
			t.Fatalf("%s version=%d, want %d", label, v, want)
		}
	}
	c, err := store.Update("proj-1", c.ID, WorkCasePatch{Title: &title}, 1, caseEvent("proj-1", c.ID, "update"))
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	expectVersion("update", c.Version)
	c, err = store.Link("proj-1", c.ID, CaseLinkArtifact, ".artifacts/x.png", c.Version, caseEvent("proj-1", c.ID, "link"))
	if err != nil {
		t.Fatalf("link: %v", err)
	}
	expectVersion("link", c.Version)
	c, err = store.Unlink("proj-1", c.ID, CaseLinkArtifact, ".artifacts/x.png", c.Version, caseEvent("proj-1", c.ID, "unlink"))
	if err != nil {
		t.Fatalf("unlink: %v", err)
	}
	expectVersion("unlink", c.Version)
	c, err = store.Transition("proj-1", c.ID, CaseStatusSuspended, "", c.Version, caseEvent("proj-1", c.ID, "transition"))
	if err != nil {
		t.Fatalf("transition: %v", err)
	}
	expectVersion("transition", c.Version)

	// The stored row agrees with the returned one.
	got, _, err := store.Get(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != want {
		t.Fatalf("stored version=%d, want %d", got.Version, want)
	}
}

// ── update patch semantics ───────────────────────────────────────────────────

func TestWorkCaseUpdatePatch(t *testing.T) {
	db := newTestDB(t)
	store := newCaseFixture(t, db, "proj-1")
	c := mustCreateCase(t, store, "proj-1", "patchable")

	objective := "新目标"
	refs := []string{"commerce:product:9"}
	got, err := store.Update("proj-1", c.ID, WorkCasePatch{
		Objective:   &objective,
		SubjectRefs: &refs,
	}, c.Version, caseEvent("proj-1", c.ID, "update"))
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Objective != objective ||
		len(got.SubjectRefs) != 1 || got.SubjectRefs[0] != refs[0] {
		t.Fatalf("patch mismatch: %+v", got)
	}
	if got.Title != "patchable" {
		t.Fatalf("untouched field changed: %q", got.Title)
	}
	// Status and phase are never touched by Update (#323: phase advances
	// only through the command-gated SetPhaseInTx).
	if got.Status != CaseStatusOpen || got.CurrentPhase != "" {
		t.Fatalf("update changed status/phase: %s %q", got.Status, got.CurrentPhase)
	}

	// SetPhaseInTx is the only phase path: opaque app vocabulary accepted,
	// version bumped, stale versions rejected.
	tx, err := db.sql.Begin()
	if err != nil {
		t.Fatal(err)
	}
	phaseCase, err := store.SetPhaseInTx(tx, "proj-1", c.ID, "listing-shoot", got.Version)
	if err != nil {
		tx.Rollback()
		t.Fatalf("SetPhaseInTx: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if phaseCase.CurrentPhase != "listing-shoot" || phaseCase.Version != got.Version+1 {
		t.Fatalf("phase advance mismatch: %+v", phaseCase)
	}
	tx, err = db.sql.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetPhaseInTx(tx, "proj-1", c.ID, "stale", got.Version); !errors.Is(err, ErrCaseVersionConflict) {
		tx.Rollback()
		t.Fatalf("stale phase advance err=%v, want version conflict", err)
	}
	tx.Rollback()
	// Invalid refs are rejected on the patch path too.
	bad := "::"
	if _, err := store.Update("proj-1", c.ID, WorkCasePatch{PrimarySubject: &bad}, phaseCase.Version, caseEvent("proj-1", c.ID, "update")); err == nil {
		t.Fatal("malformed primarySubject patch accepted")
	}
	empty := ""
	if _, err := store.Update("proj-1", c.ID, WorkCasePatch{Title: &empty}, phaseCase.Version, caseEvent("proj-1", c.ID, "update")); !errors.Is(err, ErrInvalidProjectEvent) {
		t.Fatalf("empty title patch err=%v", err)
	}
}

// ── associations: 多对象关联 + 按 Case 查询 ──────────────────────────────────

func TestWorkCaseLinksAndByCaseQueries(t *testing.T) {
	db := newTestDB(t)
	store := newCaseFixture(t, db, "proj-1")
	newCaseFixture(t, db, "proj-2")
	wsPath := "/tmp/proj-1"

	// Two tasks in this workspace, one in another.
	taskStore := NewTaskStore(db)
	now := time.Now().UTC()
	if err := taskStore.Save(wsPath, &TasksConfig{Tasks: []Task{
		{ID: "task-a", Title: "A", Status: TaskStatusPending, CreatedAt: now, UpdatedAt: now},
		{ID: "task-b", Title: "B", Status: TaskStatusPending, CreatedAt: now.Add(time.Second), UpdatedAt: now},
	}}); err != nil {
		t.Fatalf("Save tasks: %v", err)
	}
	if err := taskStore.Save("/tmp/proj-2", &TasksConfig{Tasks: []Task{
		{ID: "task-foreign", Title: "foreign", Status: TaskStatusPending, CreatedAt: now, UpdatedAt: now},
	}}); err != nil {
		t.Fatalf("Save foreign task: %v", err)
	}
	if err := NewSessionStore(db).Add(ChatSessionRecord{ID: "sess-1", WorkspaceID: "proj-1", Name: "case chat"}); err != nil {
		t.Fatalf("Add session: %v", err)
	}

	c := mustCreateCase(t, store, "proj-1", "linked")

	// Link: task ×2, session ×1, artifact ×2 — multiple objects per kind.
	for i, target := range []struct {
		kind   CaseLinkKind
		target string
	}{
		{CaseLinkTask, "task-a"},
		{CaseLinkTask, "task-b"},
		{CaseLinkSession, "sess-1"},
		{CaseLinkArtifact, ".artifacts/presales/brief.md"},
		{CaseLinkArtifact, ".artifacts/presales/deck.pdf"},
	} {
		var err error
		c, err = store.Link("proj-1", c.ID, target.kind, target.target, c.Version, caseEvent("proj-1", c.ID, "link"))
		if err != nil {
			t.Fatalf("Link %d (%s/%s): %v", i, target.kind, target.target, err)
		}
	}
	links, err := store.ListLinks(c.ID)
	if err != nil || len(links) != 5 {
		t.Fatalf("ListLinks n=%d err=%v", len(links), err)
	}

	// Duplicate link rejected (and does not bump the version).
	versionBefore := c.Version
	if _, err := store.Link("proj-1", c.ID, CaseLinkTask, "task-a", c.Version, caseEvent("proj-1", c.ID, "link")); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate link err=%v", err)
	}
	// Unknown / cross-workspace targets rejected.
	if _, err := store.Link("proj-1", c.ID, CaseLinkTask, "nope", c.Version, caseEvent("proj-1", c.ID, "link")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown task err=%v", err)
	}
	if _, err := store.Link("proj-1", c.ID, CaseLinkTask, "task-foreign", c.Version, caseEvent("proj-1", c.ID, "link")); !errors.Is(err, ErrProjectMismatch) {
		t.Fatalf("foreign task err=%v", err)
	}
	if _, err := store.Link("proj-1", c.ID, CaseLinkKind("weird"), "x", c.Version, caseEvent("proj-1", c.ID, "link")); !errors.Is(err, ErrInvalidProjectEvent) {
		t.Fatalf("unknown kind err=%v", err)
	}
	if got, _, _ := store.Get(c.ID); got.Version != versionBefore {
		t.Fatalf("rejected links changed the version: %d → %d", versionBefore, got.Version)
	}

	// Tasks by Case: only linked tasks, oldest first.
	tasks, err := store.ListTasksByCase(c.ID)
	if err != nil || len(tasks) != 2 {
		t.Fatalf("ListTasksByCase n=%d err=%v", len(tasks), err)
	}
	if tasks[0].ID != "task-a" || tasks[1].ID != "task-b" {
		t.Fatalf("tasks by case order: %q,%q", tasks[0].ID, tasks[1].ID)
	}

	// TaskRuns by Case join through the task association.
	runStore := NewTaskRunStore(db)
	run, err := runStore.Create(wsPath, TaskRun{TaskID: "task-a", Kind: TaskRunExecution})
	if err != nil {
		t.Fatalf("TaskRun create: %v", err)
	}
	if _, err := runStore.Finish(run.ID, TaskRunCompleted, nil, nil, &ClosedBy{Kind: "manual_decision", Verdict: "accepted"}, ""); err != nil {
		t.Fatalf("TaskRun finish: %v", err)
	}
	// A run of a task that gets unlinked must stop surfacing.
	if _, err := runStore.Create(wsPath, TaskRun{TaskID: "task-b", Kind: TaskRunExecution}); err != nil {
		t.Fatalf("TaskRun create b: %v", err)
	}
	c, err = store.Unlink("proj-1", c.ID, CaseLinkTask, "task-b", c.Version, caseEvent("proj-1", c.ID, "unlink"))
	if err != nil {
		t.Fatalf("Unlink task-b: %v", err)
	}
	runs, err := store.ListTaskRunsByCase(c.ID)
	if err != nil {
		t.Fatalf("ListTaskRunsByCase: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != run.ID || runs[0].Status != TaskRunCompleted {
		t.Fatalf("runs by case: %+v", runs)
	}

	// Stale-version unlink is rejected; fresh-version succeeds.
	if _, err := store.Unlink("proj-1", c.ID, CaseLinkSession, "sess-1", c.Version-1, caseEvent("proj-1", c.ID, "unlink")); !errors.Is(err, ErrCaseVersionConflict) {
		t.Fatalf("stale unlink err=%v", err)
	}
	c, err = store.Unlink("proj-1", c.ID, CaseLinkSession, "sess-1", c.Version, caseEvent("proj-1", c.ID, "unlink"))
	if err != nil {
		t.Fatalf("Unlink session: %v", err)
	}
	links, err = store.ListLinks(c.ID)
	if err != nil || len(links) != 3 {
		t.Fatalf("ListLinks after unlinks n=%d err=%v", len(links), err)
	}
	if _, err := store.Unlink("proj-1", c.ID, CaseLinkSession, "sess-1", c.Version, caseEvent("proj-1", c.ID, "unlink")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("double unlink err=%v", err)
	}
}

// ── events: 与状态变更原子提交，可按 Case 查询 ────────────────────────────────

func TestWorkCaseEventsCommittedAtomically(t *testing.T) {
	db := newTestDB(t)
	store := newCaseFixture(t, db, "proj-1")
	events := NewProjectEventStore(db)

	id := newID()
	c, err := store.Create("proj-1", WorkCase{ID: id, Title: "audited"}, caseEvent("proj-1", id, "create"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := store.Transition("proj-1", c.ID, CaseStatusClosed, "done", c.Version, caseEvent("proj-1", c.ID, "transition")); err != nil {
		t.Fatalf("Transition: %v", err)
	}
	// A rejected transition must not leave an event behind.
	if _, err := store.Transition("proj-1", c.ID, CaseStatusOpen, "regress", c.Version+1, caseEvent("proj-1", c.ID, "transition")); err == nil {
		t.Fatal("terminal regression accepted")
	}

	page, err := events.List(ProjectEventListOptions{
		ProjectID:  "proj-1",
		TargetType: "work_case",
		TargetID:   c.ID,
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("List events: %v", err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("work_case events=%d, want 2 (create+transition): %+v", len(page.Items), page.Items)
	}
	ops := map[string]bool{}
	for _, ev := range page.Items {
		ops[ev.Operation] = true
		if ev.ActorKind != "user" || ev.Status != ProjectEventSucceeded {
			t.Fatalf("bad event attribution: %+v", ev)
		}
	}
	if !ops["create"] || !ops["transition"] {
		t.Fatalf("missing operations: %v", ops)
	}
}

// ── kernel neutrality: 内核不得出现售前/电商专属字段 ─────────────────────────

func TestWorkCaseSchemaIsDomainNeutral(t *testing.T) {
	db := newTestDB(t)
	cols, err := db.tableColumns("work_cases")
	if err != nil || len(cols) == 0 {
		t.Fatalf("tableColumns: %v", err)
	}
	// Generic coordination columns that must exist.
	for _, required := range []string{
		"id", "project_id", "case_definition", "status", "title", "objective",
		"owner", "current_phase", "primary_subject", "subject_refs",
		"participants", "version", "close_reason", "closed_at",
	} {
		if !cols[required] {
			t.Fatalf("work_cases missing required neutral column %q", required)
		}
	}
	// No presales/commerce vocabulary may leak into the kernel schema.
	for name := range cols {
		for _, banned := range []string{
			"opportunity", "budget", "sku", "listing", "price", "lead",
			"stage", "product", "order", "customer",
		} {
			if strings.Contains(name, banned) {
				t.Fatalf("domain-specific column %q in kernel work_cases schema", name)
			}
		}
	}
	// The lifecycle CHECK admits exactly the four generic states.
	if _, err := db.sql.Exec(`
		INSERT INTO work_cases (id, project_id, status, title, version, created_at, updated_at)
		VALUES ('chk-ok', 'proj-x', 'suspended', 't', 1, '', '')`); err != nil {
		t.Fatalf("valid status rejected by CHECK: %v", err)
	}
	if _, err := db.sql.Exec(`
		INSERT INTO work_cases (id, project_id, status, title, version, created_at, updated_at)
		VALUES ('chk-bad', 'proj-x', 'won', 't', 1, '', '')`); err == nil {
		t.Fatal("CHECK accepted an out-of-lifecycle status")
	}
}

// ── project scoping ────────────────────────────────────────────────────────

func TestWorkCaseProjectScoping(t *testing.T) {
	db := newTestDB(t)
	store := newCaseFixture(t, db, "proj-1")
	newCaseFixture(t, db, "proj-2")

	c := mustCreateCase(t, store, "proj-1", "scoped")
	// Mutations through the wrong workspace are rejected.
	if _, err := store.Update("proj-2", c.ID, WorkCasePatch{}, c.Version, caseEvent("proj-2", c.ID, "update")); !errors.Is(err, ErrProjectMismatch) {
		t.Fatalf("cross-project update err=%v", err)
	}
	if _, err := store.Transition("proj-2", c.ID, CaseStatusClosed, "", c.Version, caseEvent("proj-2", c.ID, "transition")); !errors.Is(err, ErrProjectMismatch) {
		t.Fatalf("cross-project transition err=%v", err)
	}
	if err := store.Delete("proj-2", c.ID, caseEvent("proj-2", c.ID, "delete")); !errors.Is(err, ErrProjectMismatch) {
		t.Fatalf("cross-project delete err=%v", err)
	}
	// The case is invisible in the other workspace's list.
	cases, err := store.List("proj-2", "")
	if err != nil || len(cases) != 0 {
		t.Fatalf("cross-project list n=%d err=%v", len(cases), err)
	}
	// Still intact in its own workspace.
	if _, ok, err := store.Get(c.ID); err != nil || !ok {
		t.Fatalf("Get after cross-project attempts: ok=%v err=%v", ok, err)
	}
}
