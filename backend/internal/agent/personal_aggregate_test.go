package agent

// Tests for the Personal Shell cross-shell work aggregation (task #329).
// They exercise the pure buildPersonalAggregate core against a real (temp)
// meta DB plus an injected domainref registry, covering the acceptance gates:
// kernel-only reads, same task id/status across shells, pagination /
// filtering / sorting / permission filtering, restricted placeholders, and
// empty / partial-provider / large-data behaviour.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/scottzx/1Agents/backend/internal/domainref"
	"github.com/scottzx/1Agents/backend/internal/meta"
)

// ── test helpers ─────────────────────────────────────────────────────────────

func openAggregateTestDB(t *testing.T) (*meta.DB, func()) {
	t.Helper()
	f, err := os.CreateTemp("", "personal-aggregate-test-*.db")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	f.Close()
	db, err := meta.Open(f.Name())
	if err != nil {
		os.Remove(f.Name())
		t.Fatalf("open db: %v", err)
	}
	return db, func() {
		db.Close()
		os.Remove(f.Name())
	}
}

// mustAddTask appends one executable task to workspace ws and returns it.
func mustAddTask(t *testing.T, store *meta.TaskStore, ws string, build func(*meta.ProjectItem)) meta.ProjectItem {
	t.Helper()
	var created meta.ProjectItem
	err := store.Mutate(ws, func(cfg *meta.TasksConfig) bool {
		now := time.Now().UTC()
		item := meta.ProjectItem{
			ID:         meta.NewID(),
			Title:      "task",
			Type:       meta.ItemTypeTask,
			IssueState: meta.IssueOpen,
			Status:     meta.TaskStatusPending,
			Executor:   meta.TaskExecutorAgent,
			CreatedAt:  now,
			UpdatedAt:  now,
			Replies:    []meta.Reply{},
			Sessions:   []meta.SessionMetadata{},
		}
		if build != nil {
			build(&item)
		}
		created = item
		cfg.Tasks = append(cfg.Tasks, item)
		return true
	})
	if err != nil {
		t.Fatalf("add task: %v", err)
	}
	return created
}

// fakeDomainProvider is a domainref.QueryProvider with a per-object ACL, used
// to exercise available, permission-denied and not-found subject summaries.
type fakeDomainProvider struct {
	ns      string
	objects map[string]string            // id → title; missing = not found
	acl     map[string][]string          // id → allowed actors; missing = public
}

func (f *fakeDomainProvider) Namespace() string { return f.ns }
func (f *fakeDomainProvider) Versions() []int   { return []int{0, 1} }

func (f *fakeDomainProvider) Query(_ context.Context, req domainref.QueryRequest) (domainref.ObjectSummary, error) {
	title, ok := f.objects[req.Ref.ID]
	if !ok {
		return domainref.ObjectSummary{}, domainref.NewError(domainref.CodeNotFound,
			"%s %q does not exist", req.Ref.Type, req.Ref.ID)
	}
	if allowed, hasACL := f.acl[req.Ref.ID]; hasACL {
		ok := false
		for _, a := range allowed {
			if a == req.Actor {
				ok = true
				break
			}
		}
		if !ok {
			return domainref.ObjectSummary{}, domainref.NewError(domainref.CodePermissionDenied,
				"actor %q may not read %s %q", req.Actor, req.Ref.Type, req.Ref.ID)
		}
	}
	return domainref.ObjectSummary{
		Ref:    req.Ref,
		Title:  title,
		Status: "active",
		Link:   "/" + f.ns + "/" + req.Ref.Type + "/" + req.Ref.ID,
	}, nil
}

// newAggregateDeps wires the kernel stores + a registry for the pure core.
func newAggregateDeps(db *meta.DB, reg *domainref.Registry) aggregateDeps {
	store := meta.NewTaskStore(db)
	return aggregateDeps{
		tasksStore: store,
		caseStore:  meta.NewWorkCaseStore(db),
		runStore:   meta.NewTaskRunStore(db),
		registry:   reg,
	}
}

// linkCaseToTask creates a WorkCase (with optional primary subject) in project
// and links it to taskID. Returns the case.
func linkCaseToTask(t *testing.T, db *meta.DB, store *meta.TaskStore, ws, taskID, title, primarySubject string) meta.WorkCase {
	t.Helper()
	cases := meta.NewWorkCaseStore(db)
	projectID, err := store.ProjectIDForPath(ws)
	if err != nil || projectID == "" {
		t.Fatalf("ProjectIDForPath: id=%q err=%v", projectID, err)
	}
	ev := meta.ProjectEvent{
		ProjectID: projectID, ActorKind: "user", ActorName: "user", Origin: "http",
		EventType: "work_case.create", TargetType: "work_case", TargetID: "x",
		Operation: "create", Status: meta.ProjectEventSucceeded,
	}
	wc, err := cases.Create(projectID, meta.WorkCase{Title: title, PrimarySubject: primarySubject}, ev)
	if err != nil {
		t.Fatalf("case create: %v", err)
	}
	if _, err := cases.Link(projectID, wc.ID, meta.CaseLinkTask, taskID, wc.Version, meta.ProjectEvent{
		ProjectID: projectID, ActorKind: "user", ActorName: "user", Origin: "http",
		EventType: "work_case.link", TargetType: "work_case", TargetID: wc.ID,
		Operation: "link", Status: meta.ProjectEventSucceeded,
	}); err != nil {
		t.Fatalf("case link: %v", err)
	}
	return wc
}

// runAggregate runs the pure core with sensible defaults and a fixed "now".
func runAggregate(t *testing.T, d aggregateDeps, opts aggregateOptions) PersonalAggregateResponse {
	t.Helper()
	if opts.Now.IsZero() {
		opts.Now = time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	}
	if opts.Actor == "" {
		opts.Actor = "user"
	}
	resp, err := buildPersonalAggregate(d, opts)
	if err != nil {
		t.Fatalf("buildPersonalAggregate: %v", err)
	}
	return resp
}

// ── empty state ──────────────────────────────────────────────────────────────

func TestAggregateEmptyState(t *testing.T) {
	db, cleanup := openAggregateTestDB(t)
	defer cleanup()
	d := newAggregateDeps(db, domainref.NewRegistry())

	resp := runAggregate(t, d, aggregateOptions{})
	if resp.Total != 0 || len(resp.Items) != 0 || resp.HasMore {
		t.Fatalf("expected empty aggregate, got total=%d items=%d hasMore=%v",
			resp.Total, len(resp.Items), resp.HasMore)
	}
	if resp.Counts[aggBucketAll] != 0 {
		t.Fatalf("expected counts.all=0, got %d", resp.Counts[aggBucketAll])
	}
	if resp.Shell != ShellPersonalID {
		t.Fatalf("expected shell=%q, got %q", ShellPersonalID, resp.Shell)
	}
}

// ── bucket classification ────────────────────────────────────────────────────

func TestAggregateBucketClassification(t *testing.T) {
	db, cleanup := openAggregateTestDB(t)
	defer cleanup()
	store := meta.NewTaskStore(db)
	ws := t.TempDir()
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)

	running := mustAddTask(t, store, ws, func(i *meta.ProjectItem) {
		i.Title = "running task"
		i.Status = meta.TaskStatusRunning
	})
	mustAddTask(t, store, ws, func(i *meta.ProjectItem) {
		i.Title = "awaiting human"
		i.Status = meta.TaskStatusAwaitingHuman
	})
	mustAddTask(t, store, ws, func(i *meta.ProjectItem) {
		i.Title = "failed task"
		i.Status = meta.TaskStatusFailed
	})
	mustAddTask(t, store, ws, func(i *meta.ProjectItem) {
		i.Title = "blocked task"
		i.Status = meta.TaskStatusBlocked
	})
	dueSoonAt := now.Add(24 * time.Hour)
	mustAddTask(t, store, ws, func(i *meta.ProjectItem) {
		i.Title = "due soon"
		i.Status = meta.TaskStatusPending
		i.PlannedEnd = &dueSoonAt
	})
	mustAddTask(t, store, ws, func(i *meta.ProjectItem) {
		i.Title = "plain open"
		i.Status = meta.TaskStatusPending
	})
	_ = running

	d := newAggregateDeps(db, domainref.NewRegistry())
	resp := runAggregate(t, d, aggregateOptions{Now: now})

	if resp.Total != 6 {
		t.Fatalf("expected 6 items, got %d", resp.Total)
	}
	wantCounts := map[string]int{
		aggBucketAll: 6, aggBucketRunning: 1, aggBucketAwaiting: 1,
		aggBucketFailed: 1, aggBucketBlocked: 1, aggBucketDueSoon: 1, aggBucketOpen: 1,
	}
	for k, v := range wantCounts {
		if resp.Counts[k] != v {
			t.Errorf("counts[%s]=%d, want %d", k, resp.Counts[k], v)
		}
	}

	// Find the running item and check its bucket tag.
	found := map[string]bool{}
	for _, it := range resp.Items {
		for _, b := range it.Buckets {
			found[it.Title+":"+b] = true
		}
	}
	for _, want := range []string{
		"running task:running", "awaiting human:awaiting", "failed task:failed",
		"blocked task:blocked", "due soon:due_soon",
	} {
		if !found[want] {
			t.Errorf("missing bucket assignment %q", want)
		}
	}
}

// ── case binding + subject resolution + restricted placeholder ───────────────

func TestAggregateCaseAndSubjectResolution(t *testing.T) {
	db, cleanup := openAggregateTestDB(t)
	defer cleanup()
	store := meta.NewTaskStore(db)
	ws := t.TempDir()

	// Task linked to a case with a presales subject (provider registered).
	task := mustAddTask(t, store, ws, func(i *meta.ProjectItem) {
		i.Title = "case-bound task"
		i.Status = meta.TaskStatusRunning
	})
	linkCaseToTask(t, db, store, ws, task.ID, "售前商机 case", "presales:opportunity:42")

	// Task linked to a case with a commerce subject (NO provider registered).
	task2 := mustAddTask(t, store, ws, func(i *meta.ProjectItem) {
		i.Title = "commerce case task"
		i.Status = meta.TaskStatusPending
	})
	linkCaseToCaseTask := linkCaseToTask(t, db, store, ws, task2.ID, "电商上新 case", "commerce:product:7")
	_ = linkCaseToCaseTask

	// Task with no case at all.
	mustAddTask(t, store, ws, func(i *meta.ProjectItem) {
		i.Title = "uncased task"
		i.Status = meta.TaskStatusPending
	})

	reg := domainref.NewRegistry()
	if err := reg.Register(&fakeDomainProvider{
		ns:      "presales",
		objects: map[string]string{"42": "商机 42"},
	}); err != nil {
		t.Fatalf("register presales provider: %v", err)
	}

	d := newAggregateDeps(db, reg)
	resp := runAggregate(t, d, aggregateOptions{Actor: "user"})

	if resp.Total != 3 {
		t.Fatalf("expected 3 items, got %d", resp.Total)
	}
	var caseBound, commerceBound, uncased *AggregateWorkItem
	for i := range resp.Items {
		switch resp.Items[i].Title {
		case "case-bound task":
			caseBound = &resp.Items[i]
		case "commerce case task":
			commerceBound = &resp.Items[i]
		case "uncased task":
			uncased = &resp.Items[i]
		}
	}
	if caseBound == nil || commerceBound == nil || uncased == nil {
		t.Fatalf("missing items: caseBound=%v commerceBound=%v uncased=%v", caseBound, commerceBound, uncased)
	}

	// presales subject resolves (provider present).
	if caseBound.CaseTitle != "售前商机 case" {
		t.Errorf("case title = %q, want 售前商机 case", caseBound.CaseTitle)
	}
	if caseBound.Subject == nil {
		t.Fatalf("expected presales subject to be present")
	}
	if !caseBound.Subject.Available || caseBound.Subject.Title != "商机 42" {
		t.Errorf("presales subject = %+v, want available with title 商机 42", caseBound.Subject)
	}
	if caseBound.DeepLink.SubjectShell != "presales" {
		t.Errorf("subject shell = %q, want presales", caseBound.DeepLink.SubjectShell)
	}

	// commerce subject is a restricted placeholder (no provider registered).
	if commerceBound.Subject == nil {
		t.Fatalf("expected commerce subject placeholder to be present")
	}
	if commerceBound.Subject.Available {
		t.Errorf("commerce subject should be restricted, got %+v", commerceBound.Subject)
	}
	if commerceBound.Subject.RestrictedReason != string(domainref.CodeUnknownProvider) {
		t.Errorf("commerce restrictedReason = %q, want %q",
			commerceBound.Subject.RestrictedReason, domainref.CodeUnknownProvider)
	}

	// uncased task has no case ref and no subject.
	if uncased.CaseRef != "" || uncased.Subject != nil {
		t.Errorf("uncased task should have no case/subject, got caseRef=%q subject=%+v",
			uncased.CaseRef, uncased.Subject)
	}

	// Canonical identity: the item id/status equal the kernel task's.
	if caseBound.ID != task.ID || caseBound.Status != meta.TaskStatusRunning {
		t.Errorf("identity mismatch: id=%q status=%q, want %q/running",
			caseBound.ID, caseBound.Status, task.ID)
	}
	// Deep link carries the canonical task coordinates.
	if caseBound.DeepLink.TaskID != task.ID || caseBound.DeepLink.TaskWorkspaceID == "" {
		t.Errorf("deep link task coords missing: %+v", caseBound.DeepLink)
	}
	if caseBound.DeepLink.CaseRef == "" {
		t.Errorf("deep link caseRef missing")
	}
}

// ── permission-denied subject → restricted placeholder ───────────────────────

func TestAggregateSubjectPermissionDenied(t *testing.T) {
	db, cleanup := openAggregateTestDB(t)
	defer cleanup()
	store := meta.NewTaskStore(db)
	ws := t.TempDir()

	task := mustAddTask(t, store, ws, func(i *meta.ProjectItem) {
		i.Title = "secret case task"
		i.Status = meta.TaskStatusPending
	})
	linkCaseToTask(t, db, store, ws, task.ID, "restricted case", "presales:opportunity:secret")

	reg := domainref.NewRegistry()
	if err := reg.Register(&fakeDomainProvider{
		ns:      "presales",
		objects: map[string]string{"secret": "受限商机"},
		acl:     map[string][]string{"secret": {"owner"}}, // only "owner" may read
	}); err != nil {
		t.Fatalf("register provider: %v", err)
	}

	d := newAggregateDeps(db, reg)

	// As "user" (not in the ACL) the subject must be a restricted placeholder.
	resp := runAggregate(t, d, aggregateOptions{Actor: "user"})
	if resp.Total != 1 {
		t.Fatalf("expected 1 item, got %d", resp.Total)
	}
	subj := resp.Items[0].Subject
	if subj == nil || subj.Available {
		t.Fatalf("expected restricted subject, got %+v", subj)
	}
	if subj.RestrictedReason != string(domainref.CodePermissionDenied) {
		t.Errorf("restrictedReason = %q, want %q", subj.RestrictedReason, domainref.CodePermissionDenied)
	}

	// As "owner" the same subject resolves — permission filtering in action.
	respOwner := runAggregate(t, d, aggregateOptions{Actor: "owner"})
	subjOwner := respOwner.Items[0].Subject
	if subjOwner == nil || !subjOwner.Available || subjOwner.Title != "受限商机" {
		t.Errorf("owner should resolve subject, got %+v", subjOwner)
	}
}

// ── pagination over large data ───────────────────────────────────────────────

func TestAggregatePaginationLargeData(t *testing.T) {
	db, cleanup := openAggregateTestDB(t)
	defer cleanup()
	store := meta.NewTaskStore(db)
	ws := t.TempDir()

	const n = 120
	for i := 0; i < n; i++ {
		idx := i
		mustAddTask(t, store, ws, func(item *meta.ProjectItem) {
			item.Title = "task " + string(rune('A'+idx%26))
			item.Status = meta.TaskStatusPending
		})
	}

	d := newAggregateDeps(db, domainref.NewRegistry())

	page1 := runAggregate(t, d, aggregateOptions{Limit: 50, Offset: 0})
	if page1.Total != n || len(page1.Items) != 50 || !page1.HasMore {
		t.Fatalf("page1: total=%d items=%d hasMore=%v, want %d/50/true",
			page1.Total, len(page1.Items), page1.HasMore, n)
	}
	page3 := runAggregate(t, d, aggregateOptions{Limit: 50, Offset: 100})
	if len(page3.Items) != 20 || page3.HasMore {
		t.Fatalf("page3: items=%d hasMore=%v, want 20/false", len(page3.Items), page3.HasMore)
	}
	// Offset beyond the end → empty page, no crash.
	beyond := runAggregate(t, d, aggregateOptions{Limit: 50, Offset: 500})
	if len(beyond.Items) != 0 || beyond.HasMore {
		t.Fatalf("beyond: items=%d hasMore=%v, want 0/false", len(beyond.Items), beyond.HasMore)
	}
	// Limit clamping above the max: the effective limit is capped; with fewer
	// tasks than the cap, all are returned and hasMore is false.
	clamped := runAggregate(t, d, aggregateOptions{Limit: 10000, Offset: 0})
	if clamped.Limit != maxAggregateLimit {
		t.Fatalf("clamped limit field=%d, want %d", clamped.Limit, maxAggregateLimit)
	}
	if len(clamped.Items) != n || clamped.HasMore {
		t.Fatalf("clamped: items=%d hasMore=%v, want %d/false", len(clamped.Items), clamped.HasMore, n)
	}

	// Pages are disjoint (stable ordering → no duplicate ids across pages).
	seen := map[string]bool{}
	for _, off := range []int{0, 50, 100} {
		p := runAggregate(t, d, aggregateOptions{Limit: 50, Offset: off})
		for _, it := range p.Items {
			if seen[it.ID] {
				t.Fatalf("duplicate task %q across pages", it.ID)
			}
			seen[it.ID] = true
		}
	}
	if len(seen) != n {
		t.Fatalf("expected %d unique tasks across pages, got %d", n, len(seen))
	}
}

// ── filtering + sorting ──────────────────────────────────────────────────────

func TestAggregateFilterAndSort(t *testing.T) {
	db, cleanup := openAggregateTestDB(t)
	defer cleanup()
	store := meta.NewTaskStore(db)
	wsA := t.TempDir()
	wsB := t.TempDir()

	mustAddTask(t, store, wsA, func(i *meta.ProjectItem) {
		i.Title = "alpha failed"
		i.Status = meta.TaskStatusFailed
		i.Priority = meta.PriorityHigh
	})
	mustAddTask(t, store, wsA, func(i *meta.ProjectItem) {
		i.Title = "beta running"
		i.Status = meta.TaskStatusRunning
		i.Priority = meta.PriorityLow
	})
	mustAddTask(t, store, wsB, func(i *meta.ProjectItem) {
		i.Title = "gamma failed"
		i.Status = meta.TaskStatusFailed
		i.Priority = meta.PriorityUrgent
	})

	d := newAggregateDeps(db, domainref.NewRegistry())

	// Bucket filter: only failed items.
	failed := runAggregate(t, d, aggregateOptions{Bucket: aggBucketFailed})
	if failed.Total != 2 {
		t.Fatalf("bucket=failed total=%d, want 2", failed.Total)
	}

	// Workspace filter: only wsA.
	projA, err := store.ProjectIDForPath(wsA)
	if err != nil {
		t.Fatalf("ProjectIDForPath(wsA): %v", err)
	}
	wsAOnly := runAggregate(t, d, aggregateOptions{Workspace: projA})
	if wsAOnly.Total != 2 {
		t.Fatalf("workspace filter total=%d, want 2", wsAOnly.Total)
	}

	// Status filter.
	statusFiltered := runAggregate(t, d, aggregateOptions{Status: string(meta.TaskStatusFailed)})
	if statusFiltered.Total != 2 {
		t.Fatalf("status filter total=%d, want 2", statusFiltered.Total)
	}

	// Sort by title ascending.
	byTitle := runAggregate(t, d, aggregateOptions{Sort: "title", Dir: "asc"})
	if byTitle.Items[0].Title != "alpha failed" || byTitle.Items[2].Title != "gamma failed" {
		t.Fatalf("title asc order wrong: %v", titles(byTitle.Items))
	}

	// Sort by priority ascending (urgent first).
	byPriority := runAggregate(t, d, aggregateOptions{Sort: "priority", Dir: "asc"})
	if byPriority.Items[0].Priority != meta.PriorityUrgent {
		t.Fatalf("priority asc first=%q, want urgent", byPriority.Items[0].Priority)
	}

	// Default salience ordering puts failed before running.
	salient := runAggregate(t, d, aggregateOptions{})
	if salient.Items[0].Status != meta.TaskStatusFailed {
		t.Fatalf("salience first status=%q, want failed", salient.Items[0].Status)
	}
}

func titles(items []AggregateWorkItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Title)
	}
	return out
}

// ── terminal / non-executable tasks are excluded ─────────────────────────────

func TestAggregateExcludesTerminalAndNonExecutable(t *testing.T) {
	db, cleanup := openAggregateTestDB(t)
	defer cleanup()
	store := meta.NewTaskStore(db)
	ws := t.TempDir()

	mustAddTask(t, store, ws, func(i *meta.ProjectItem) {
		i.Title = "completed task"
		i.Status = meta.TaskStatusCompleted
	})
	mustAddTask(t, store, ws, func(i *meta.ProjectItem) {
		i.Title = "cancelled task"
		i.Status = meta.TaskStatusCancelled
	})
	mustAddTask(t, store, ws, func(i *meta.ProjectItem) {
		i.Title = "a requirement"
		i.Type = meta.ItemTypeRequirement
		i.Status = meta.TaskStatusPending
	})
	mustAddTask(t, store, ws, func(i *meta.ProjectItem) {
		i.Title = "suggestion"
		i.Source = meta.TaskSourceAgent
		i.Status = meta.TaskStatusPending
	})
	mustAddTask(t, store, ws, func(i *meta.ProjectItem) {
		i.Title = "kept open task"
		i.Status = meta.TaskStatusPending
	})

	d := newAggregateDeps(db, domainref.NewRegistry())
	resp := runAggregate(t, d, aggregateOptions{})
	if resp.Total != 1 {
		t.Fatalf("expected only 1 item to survive, got %d (%v)", resp.Total, titles(resp.Items))
	}
	if resp.Items[0].Title != "kept open task" {
		t.Fatalf("unexpected surviving item %q", resp.Items[0].Title)
	}
}

// ── HTTP handler wiring ──────────────────────────────────────────────────────

// TestHandlePersonalAggregateHTTP exercises the REST surface end to end: query
// parsing, the kernel-store read path and the JSON envelope. It builds a minimal
// Handler over a temp DB (same package ⇒ unexported fields are reachable) so
// the test never touches the real ~/.1agents meta DB or the global registry.
func TestHandlePersonalAggregateHTTP(t *testing.T) {
	db, cleanup := openAggregateTestDB(t)
	defer cleanup()
	store := meta.NewTaskStore(db)
	ws := t.TempDir()

	mustAddTask(t, store, ws, func(i *meta.ProjectItem) {
		i.Title = "http running"
		i.Status = meta.TaskStatusRunning
	})
	mustAddTask(t, store, ws, func(i *meta.ProjectItem) {
		i.Title = "http failed"
		i.Status = meta.TaskStatusFailed
	})

	h := &Handler{
		tasksStore:    store,
		workCaseStore: meta.NewWorkCaseStore(db),
		taskRunStore:  meta.NewTaskRunStore(db),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/agent/personal/aggregate?limit=10", nil)
	rec := httptest.NewRecorder()
	h.HandlePersonalAggregate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp PersonalAggregateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v; body=%s", err, rec.Body.String())
	}
	if resp.Shell != ShellPersonalID {
		t.Errorf("shell = %q, want %q", resp.Shell, ShellPersonalID)
	}
	if resp.Total != 2 || len(resp.Items) != 2 {
		t.Fatalf("total=%d items=%d, want 2/2", resp.Total, len(resp.Items))
	}

	// bucket filter narrows to the single failed item.
	reqFailed := httptest.NewRequest(http.MethodGet, "/api/agent/personal/aggregate?bucket=failed", nil)
	recFailed := httptest.NewRecorder()
	h.HandlePersonalAggregate(recFailed, reqFailed)
	var respFailed PersonalAggregateResponse
	if err := json.Unmarshal(recFailed.Body.Bytes(), &respFailed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if respFailed.Total != 1 || respFailed.Items[0].Title != "http failed" {
		t.Fatalf("bucket=failed: total=%d items=%v, want 1 failed item",
			respFailed.Total, titles(respFailed.Items))
	}

	// Non-GET is rejected.
	reqPost := httptest.NewRequest(http.MethodPost, "/api/agent/personal/aggregate", nil)
	recPost := httptest.NewRecorder()
	h.HandlePersonalAggregate(recPost, reqPost)
	if recPost.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST status = %d, want 405", recPost.Code)
	}
}
