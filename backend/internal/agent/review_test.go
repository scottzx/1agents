package agent

import (
	"testing"
	"time"
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

func TestApplyReviewVerdictPassCompletes(t *testing.T) {
	store, path, id := reviewTask(t, 0)
	got, err := applyReviewVerdict(store, path, id, []CriterionResult{
		{Criterion: "must compile", Pass: true},
		{Criterion: "tests pass", Pass: true},
	}, "all good", "claudecode")
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
	}, "", "claudecode")
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
	}, "", "claudecode")
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

func TestApplyReviewVerdictRejectsWhenNotRunning(t *testing.T) {
	store, path, id := reviewTask(t, 0)
	setStatus(t, store, path, id, TaskStatusPending)
	if _, err := applyReviewVerdict(store, path, id, []CriterionResult{
		{Criterion: "x", Pass: true},
	}, "", "claudecode"); err == nil {
		t.Fatal("expected rejection when the task is not under review (running)")
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
