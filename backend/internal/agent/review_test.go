package agent

import (
	"testing"
	"time"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

// reviewTask seeds one running, verification-configured task and returns the
// scheduler test rig.
func reviewTask(t *testing.T, reviewMax int) (*TasksStore, string, string) {
	t.Helper()
	t.Setenv("ONEAGENTS_HOME", t.TempDir())
	store, err := NewTasksStore()
	if err != nil {
		t.Fatalf("NewTasksStore: %v", err)
	}
	path := t.TempDir()
	now := time.Now().UTC()
	started := now
	saveTasks(t, store, path, []Task{{
		ID:                 "t1",
		Title:              "T",
		Description:        "do the thing",
		AcceptanceCriteria: "must compile; tests pass",
		Verifier:           "claudecode",
		ReviewMaxAttempts:  reviewMax,
		Status:             TaskStatusRunning,
		StartedAt:          &started,
		CreatedAt:          now,
		UpdatedAt:          now,
	}})
	return store, path, "t1"
}

// panelTask seeds one running task configured for an N-verifier adversarial
// panel with an optional explicit pass threshold (0 = majority).
func panelTask(t *testing.T, verifiers, threshold, reviewMax int) (*TasksStore, string, string) {
	t.Helper()
	t.Setenv("ONEAGENTS_HOME", t.TempDir())
	store, err := NewTasksStore()
	if err != nil {
		t.Fatalf("NewTasksStore: %v", err)
	}
	path := t.TempDir()
	now := time.Now().UTC()
	started := now
	saveTasks(t, store, path, []Task{{
		ID:                  "t1",
		Title:               "T",
		Description:         "do the thing",
		AcceptanceCriteria:  "must compile; tests pass",
		Verifier:            "claudecode",
		VerifierCount:       verifiers,
		VerifyPassThreshold: threshold,
		ReviewMaxAttempts:   reviewMax,
		Status:              TaskStatusRunning,
		StartedAt:           &started,
		CreatedAt:           now,
		UpdatedAt:           now,
	}})
	return store, path, "t1"
}

func passCrit() []CriterionResult {
	return []CriterionResult{{Criterion: "must compile", Pass: true}, {Criterion: "tests pass", Pass: true}}
}

func failCrit() []CriterionResult {
	return []CriterionResult{{Criterion: "must compile", Pass: true}, {Criterion: "tests pass", Pass: false, Comment: "red"}}
}

func TestApplyReviewVerdictPassCompletes(t *testing.T) {
	store, path, id := reviewTask(t, 0)
	got, err := applyReviewVerdict(store, path, id, []CriterionResult{
		{Criterion: "must compile", Pass: true},
		{Criterion: "tests pass", Pass: true},
	}, false, "all good", "claudecode")
	if err != nil {
		t.Fatalf("applyReviewVerdict: %v", err)
	}
	if got.Status != TaskStatusCompleted {
		t.Fatalf("status = %s, want completed", got.Status)
	}
	if got.Review == nil || !got.Review.Pass {
		t.Fatalf("review verdict not recorded as pass: %+v", got.Review)
	}
	if got.CompletedAt == nil {
		t.Error("CompletedAt should be set on pass")
	}
	if got.ClosedBy == nil || got.ClosedBy.TaskRunID == "" || got.ClosedBy.Verdict != "passed" {
		t.Fatalf("completed task missing audit provenance: %+v", got.ClosedBy)
	}
	runs, err := store.TaskRuns().ListByTask(id)
	if err != nil {
		t.Fatalf("ListByTask: %v", err)
	}
	if len(runs) != 1 || runs[0].Kind != meta.TaskRunVerification || len(runs[0].Evidence) != 1 {
		t.Fatalf("verification TaskRun=%+v", runs)
	}
	// The verdict must land on the timeline.
	if len(got.Replies) == 0 {
		t.Error("expected a verdict reply on the timeline")
	}
}

func TestApplyReviewVerdictFailRequeuesWithinBudget(t *testing.T) {
	store, path, id := reviewTask(t, 2) // two cycles allowed
	got, err := applyReviewVerdict(store, path, id, []CriterionResult{
		{Criterion: "must compile", Pass: true},
		{Criterion: "tests pass", Pass: false, Comment: "3 failing"},
	}, false, "", "claudecode")
	if err != nil {
		t.Fatalf("applyReviewVerdict: %v", err)
	}
	// First rejection of a 2-budget task: back to pending for re-execution.
	if got.Status != TaskStatusPending {
		t.Fatalf("status = %s, want pending (re-execute)", got.Status)
	}
	if got.ReviewCount != 1 {
		t.Fatalf("ReviewCount = %d, want 1", got.ReviewCount)
	}
	if got.StartedAt != nil {
		t.Error("StartedAt should reset so the re-execution looks fresh")
	}
	if reviewExhausted(got) {
		t.Error("budget not exhausted after one rejection")
	}
}

func TestApplyReviewVerdictFailExhaustsBudget(t *testing.T) {
	store, path, id := reviewTask(t, 1) // single cycle
	got, err := applyReviewVerdict(store, path, id, []CriterionResult{
		{Criterion: "tests pass", Pass: false, Comment: "still red"},
	}, false, "", "claudecode")
	if err != nil {
		t.Fatalf("applyReviewVerdict: %v", err)
	}
	if got.Status != TaskStatusFailed {
		t.Fatalf("status = %s, want failed (报异常)", got.Status)
	}
	if !reviewExhausted(got) {
		t.Error("reviewExhausted should be true once budget is spent")
	}
}

// TestApplyReviewVerdictNeedsHumanEscalates: a verifier that flags needsHuman
// parks the task at awaiting_human without spending review budget — even a
// tight 1-cycle budget doesn't fail it, because escalation is not a rejection.
// The all-pass criteria here also assert the mutual exclusion: the chosen route
// (needs_human) wins over an accidental criteria pass.
func TestApplyReviewVerdictNeedsHumanEscalates(t *testing.T) {
	store, path, id := reviewTask(t, 1) // single cycle: a reject would exhaust it
	got, err := applyReviewVerdict(store, path, id, passCrit(), true,
		"需要产品先决定是否支持多租户,执行者无法自行取舍", "claudecode")
	if err != nil {
		t.Fatalf("applyReviewVerdict: %v", err)
	}
	if got.Status != TaskStatusAwaitingHuman {
		t.Fatalf("status = %s, want awaiting_human (升级人工)", got.Status)
	}
	if got.ReviewCount != 0 {
		t.Fatalf("ReviewCount = %d, want 0 — escalation must not consume budget", got.ReviewCount)
	}
	if reviewExhausted(got) {
		t.Error("escalation is not a rejection; budget must stay intact")
	}
	if got.Review == nil || !got.Review.NeedsHuman || got.Review.Pass {
		t.Fatalf("verdict should be needs-human, not pass: %+v", got.Review)
	}
	if got.CompletedAt != nil {
		t.Error("an escalated task is not completed")
	}
	if len(got.Replies) == 0 {
		t.Error("expected an escalation reply on the timeline")
	}
}

// TestPanelPassBeatsNeedsHuman: 2 pass + 1 needs-human at majority threshold 2 →
// a pass consensus still ships; escalation only diverts the reject path.
func TestPanelPassBeatsNeedsHuman(t *testing.T) {
	store, path, id := panelTask(t, 3, 0, 2)
	_, _ = applyReviewVerdict(store, path, id, passCrit(), false, "v1", "claudecode")
	_, _ = applyReviewVerdict(store, path, id, passCrit(), false, "v2", "claudecode")
	got, err := applyReviewVerdict(store, path, id, passCrit(), true, "需人工", "claudecode")
	if err != nil {
		t.Fatalf("verdict 3: %v", err)
	}
	if got.Status != TaskStatusCompleted {
		t.Fatalf("status = %s, want completed (2/3 pass ≥ majority beats 1 escalation)", got.Status)
	}
	if got.Review == nil || !got.Review.Pass || got.Review.NeedsHuman {
		t.Fatalf("aggregate should be pass, not escalation: %+v", got.Review)
	}
}

// TestPanelNeedsHumanBeatsReject: no pass consensus (1/3) but one panelist flags
// needs-human → escalate to awaiting_human instead of a pointless re-execution.
func TestPanelNeedsHumanBeatsReject(t *testing.T) {
	store, path, id := panelTask(t, 3, 0, 2)
	_, _ = applyReviewVerdict(store, path, id, passCrit(), false, "v1", "claudecode")
	_, _ = applyReviewVerdict(store, path, id, failCrit(), false, "v2", "claudecode")
	got, err := applyReviewVerdict(store, path, id, failCrit(), true, "需人工决策", "claudecode")
	if err != nil {
		t.Fatalf("verdict 3: %v", err)
	}
	if got.Status != TaskStatusAwaitingHuman {
		t.Fatalf("status = %s, want awaiting_human (1/3 pass, one escalation diverts reject)", got.Status)
	}
	if got.ReviewCount != 0 {
		t.Fatalf("ReviewCount = %d, want 0 — escalation must not consume budget", got.ReviewCount)
	}
	if got.Review == nil || !got.Review.NeedsHuman || got.Review.Pass {
		t.Fatalf("aggregate should be needs-human: %+v", got.Review)
	}
}

func TestApplyReviewVerdictRejectsWhenNotRunning(t *testing.T) {
	store, path, id := reviewTask(t, 0)
	setStatus(t, store, path, id, TaskStatusPending)
	if _, err := applyReviewVerdict(store, path, id, []CriterionResult{
		{Criterion: "x", Pass: true},
	}, false, "", "claudecode"); err == nil {
		t.Fatal("expected rejection when the task is not under review (running)")
	}
}

func TestEffectiveVerifierCountAndThreshold(t *testing.T) {
	cases := []struct {
		name          string
		count, thr    int
		wantN, wantTh int
	}{
		{"unset → single verifier majority", 0, 0, 1, 1},
		{"one verifier", 1, 0, 1, 1},
		{"three default majority", 3, 0, 3, 2},
		{"five default majority", 5, 0, 5, 3},
		{"three unanimous", 3, 3, 3, 3},
		{"threshold clamped to N", 3, 9, 3, 3},
		{"threshold floored to 1", 3, -1, 3, 2}, // -1 → majority
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			task := &Task{VerifierCount: c.count, VerifyPassThreshold: c.thr}
			if got := effectiveVerifierCount(task); got != c.wantN {
				t.Errorf("count = %d, want %d", got, c.wantN)
			}
			if got := effectivePassThreshold(task); got != c.wantTh {
				t.Errorf("threshold = %d, want %d", got, c.wantTh)
			}
		})
	}
}

// TestPanelStaysUnderReviewUntilComplete: with N=3, the first two verdicts must
// not transition the task — it stays running, accumulating the pool.
func TestPanelStaysUnderReviewUntilComplete(t *testing.T) {
	store, path, id := panelTask(t, 3, 0, 2)

	got, err := applyReviewVerdict(store, path, id, passCrit(), false, "v1", "claudecode")
	if err != nil {
		t.Fatalf("verdict 1: %v", err)
	}
	if got.Status != TaskStatusRunning {
		t.Fatalf("after 1/3 status = %s, want running", got.Status)
	}
	if len(got.ReviewPool) != 1 {
		t.Fatalf("pool = %d, want 1", len(got.ReviewPool))
	}

	got, err = applyReviewVerdict(store, path, id, passCrit(), false, "v2", "claudecode")
	if err != nil {
		t.Fatalf("verdict 2: %v", err)
	}
	if got.Status != TaskStatusRunning {
		t.Fatalf("after 2/3 status = %s, want running", got.Status)
	}
	if len(got.ReviewPool) != 2 {
		t.Fatalf("pool = %d, want 2", len(got.ReviewPool))
	}
	if got.Review != nil {
		t.Error("no aggregate verdict should exist before the panel completes")
	}
}

// TestPanelMajorityPassCompletes: 2 of 3 pass with default majority threshold
// (2) → the panel accepts and the task completes.
func TestPanelMajorityPassCompletes(t *testing.T) {
	store, path, id := panelTask(t, 3, 0, 2)
	_, _ = applyReviewVerdict(store, path, id, passCrit(), false, "v1", "claudecode")
	_, _ = applyReviewVerdict(store, path, id, failCrit(), false, "v2", "claudecode")
	got, err := applyReviewVerdict(store, path, id, passCrit(), false, "v3", "claudecode")
	if err != nil {
		t.Fatalf("verdict 3: %v", err)
	}
	if got.Status != TaskStatusCompleted {
		t.Fatalf("status = %s, want completed (2/3 ≥ majority 2)", got.Status)
	}
	if got.Review == nil || !got.Review.Pass {
		t.Fatalf("aggregate verdict not pass: %+v", got.Review)
	}
	if len(got.ReviewPool) != 0 {
		t.Errorf("pool should be cleared after aggregation, got %d", len(got.ReviewPool))
	}
}

// TestPanelBelowThresholdRejects: only 1 of 3 passes (< majority 2) → the panel
// rejects; with budget left the task requeues for re-execution.
func TestPanelBelowThresholdRejects(t *testing.T) {
	store, path, id := panelTask(t, 3, 0, 2)
	_, _ = applyReviewVerdict(store, path, id, passCrit(), false, "v1", "claudecode")
	_, _ = applyReviewVerdict(store, path, id, failCrit(), false, "v2", "claudecode")
	got, err := applyReviewVerdict(store, path, id, failCrit(), false, "v3", "claudecode")
	if err != nil {
		t.Fatalf("verdict 3: %v", err)
	}
	if got.Status != TaskStatusPending {
		t.Fatalf("status = %s, want pending (1/3 < majority 2, budget left)", got.Status)
	}
	if got.ReviewCount != 1 {
		t.Fatalf("ReviewCount = %d, want 1", got.ReviewCount)
	}
	if got.Review == nil || got.Review.Pass {
		t.Fatalf("aggregate verdict should be a rejection: %+v", got.Review)
	}
	if len(got.ReviewPool) != 0 {
		t.Errorf("pool should reset between cycles, got %d", len(got.ReviewPool))
	}
}

// TestPanelUnanimousThreshold: explicit threshold = N means one dissenter sinks
// the artifact even if the majority passes.
func TestPanelUnanimousThreshold(t *testing.T) {
	store, path, id := panelTask(t, 3, 3, 2) // require all 3
	_, _ = applyReviewVerdict(store, path, id, passCrit(), false, "v1", "claudecode")
	_, _ = applyReviewVerdict(store, path, id, passCrit(), false, "v2", "claudecode")
	got, err := applyReviewVerdict(store, path, id, failCrit(), false, "v3", "claudecode")
	if err != nil {
		t.Fatalf("verdict 3: %v", err)
	}
	if got.Status != TaskStatusPending {
		t.Fatalf("status = %s, want pending (2/3 < unanimous 3)", got.Status)
	}
}

// TestPanelPoolPersistsRoundTrip: an in-progress panel's pool must survive a
// store reload (it lives in the review_pool column).
func TestPanelPoolPersistsRoundTrip(t *testing.T) {
	store, path, id := panelTask(t, 3, 0, 2)
	if _, err := applyReviewVerdict(store, path, id, passCrit(), false, "v1", "claudecode"); err != nil {
		t.Fatalf("verdict 1: %v", err)
	}
	got, ok, err := store.GetTask(id)
	if err != nil || !ok {
		t.Fatalf("GetTask: ok=%v err=%v", ok, err)
	}
	if len(got.ReviewPool) != 1 {
		t.Fatalf("reloaded pool = %d, want 1", len(got.ReviewPool))
	}
	if got.ReviewPool[0].Verifier != "claudecode" || !got.ReviewPool[0].Pass {
		t.Errorf("reloaded pool verdict wrong: %+v", got.ReviewPool[0])
	}
}

func TestNeedsReview(t *testing.T) {
	cases := []struct {
		name     string
		verifier string
		criteria string
		want     bool
	}{
		{"both set", "claudecode", "must pass", true},
		{"no verifier", "", "must pass", false},
		{"no criteria", "claudecode", "", false},
		{"neither", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := needsReview(&Task{Verifier: c.verifier, AcceptanceCriteria: c.criteria})
			if got != c.want {
				t.Errorf("needsReview = %v, want %v", got, c.want)
			}
		})
	}
}

// TestSchedulerRoutesPendingReview: a pending_review task is treated as
// runnable and acquires the lock (would dispatch to Verify with a runner
// attached). With no runner, the lock releases but the task is marked running.
func TestSchedulerRoutesPendingReview(t *testing.T) {
	s, ref, store := newTestScheduler(t)
	now := time.Now().UTC()
	saveTasks(t, store, ref.Path, []Task{{
		ID: "pr", Title: "PR", Description: "x", AcceptanceCriteria: "c",
		Verifier: "claudecode", Status: TaskStatusPendingReview,
		IssueState: IssueOpen, CreatedAt: now, UpdatedAt: now,
	}})

	s.Tick()

	// No runner attached → the lock is released, but the task was picked up and
	// marked running (the dispatch path ran), proving pending_review is routed.
	if got := statusOf(t, store, ref.Path, "pr"); got != TaskStatusRunning {
		t.Fatalf("pending_review task = %s, want running (routed to verify)", got)
	}
}

// TestSchedulerKeepsReviewExhaustedFailed: a failed task whose review budget is
// spent must not be requeued by the execution-retry path.
func TestSchedulerKeepsReviewExhaustedFailed(t *testing.T) {
	s, ref, store := newTestScheduler(t)
	now := time.Now().UTC()
	saveTasks(t, store, ref.Path, []Task{{
		ID: "ex", Title: "EX", Description: "x", AcceptanceCriteria: "c",
		Verifier: "claudecode", ReviewMaxAttempts: 1, ReviewCount: 1,
		Review:     &ReviewVerdict{Pass: false, Attempt: 1},
		MaxRetries: 3, RetryCount: 0,
		Status:     TaskStatusFailed,
		IssueState: IssueOpen, CreatedAt: now, UpdatedAt: now,
	}})

	s.Tick()

	if got := statusOf(t, store, ref.Path, "ex"); got != TaskStatusFailed {
		t.Fatalf("review-exhausted task = %s, want failed (no requeue)", got)
	}
}
