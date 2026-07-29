package agent

import (
	"testing"
	"time"
)

func seedDecisionTask(t *testing.T, status TaskStatus) (*TasksStore, string, string) {
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
		ID:        "t1",
		Title:     "决策任务",
		Status:    status,
		StartedAt: &started,
		CreatedAt: now,
		UpdatedAt: now,
	}})
	return store, path, "t1"
}

func TestApplyIMDecision_ApproveBlockedRequeues(t *testing.T) {
	store, path, id := seedDecisionTask(t, TaskStatusBlocked)
	_, status, ok, err := ApplyIMDecision(store, path, id, IMApprove)
	if err != nil || !ok {
		t.Fatalf("ApplyIMDecision: ok=%v err=%v", ok, err)
	}
	if status != string(TaskStatusPending) {
		t.Fatalf("status = %q, want pending", status)
	}
	if got := statusOf(t, store, path, id); got != TaskStatusPending {
		t.Fatalf("persisted status = %q, want pending", got)
	}
}

func TestApplyIMDecision_ApprovePendingReviewCompletes(t *testing.T) {
	store, path, id := seedDecisionTask(t, TaskStatusPendingReview)
	_, status, ok, err := ApplyIMDecision(store, path, id, IMApprove)
	if err != nil || !ok {
		t.Fatalf("ApplyIMDecision: ok=%v err=%v", ok, err)
	}
	if status != string(TaskStatusCompleted) {
		t.Fatalf("status = %q, want completed", status)
	}
	task, found, getErr := store.GetTask(id)
	if getErr != nil || !found || task.ClosedBy == nil || task.ClosedBy.TaskRunID == "" {
		t.Fatalf("completed IM decision missing audit: task=%+v found=%v err=%v", task, found, getErr)
	}
	runs, runsErr := store.TaskRuns().ListByTask(id)
	if runsErr != nil || len(runs) != 1 || len(runs[0].Evidence) != 1 ||
		runs[0].Evidence[0].Kind != "im_human_decision" {
		t.Fatalf("IM TaskRun=%+v err=%v", runs, runsErr)
	}
}

func TestApplyIMDecision_RejectCancels(t *testing.T) {
	store, path, id := seedDecisionTask(t, TaskStatusFailed)
	_, status, ok, err := ApplyIMDecision(store, path, id, IMReject)
	if err != nil || !ok {
		t.Fatalf("ApplyIMDecision: ok=%v err=%v", ok, err)
	}
	if status != string(TaskStatusCancelled) {
		t.Fatalf("status = %q, want cancelled", status)
	}
}

func TestApplyIMDecision_StaleStateIsNoOp(t *testing.T) {
	// A task that already advanced (running) is not user-decidable: the tap
	// is stale and must not transition.
	store, path, id := seedDecisionTask(t, TaskStatusRunning)
	_, _, ok, err := ApplyIMDecision(store, path, id, IMApprove)
	if err != nil {
		t.Fatalf("ApplyIMDecision err=%v", err)
	}
	if ok {
		t.Fatal("expected stale no-op (ok=false) for a running task")
	}
	if got := statusOf(t, store, path, id); got != TaskStatusRunning {
		t.Fatalf("status mutated to %q, want running unchanged", got)
	}
}

func TestApplyIMDecision_UnknownTask(t *testing.T) {
	store, path, _ := seedDecisionTask(t, TaskStatusBlocked)
	_, _, ok, err := ApplyIMDecision(store, path, "nope", IMApprove)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if ok {
		t.Fatal("expected ok=false for unknown task")
	}
}

// emitNotify must be a safe no-op when no notifier is registered.
func TestEmitNotify_NoNotifierIsSafe(t *testing.T) {
	SetTaskNotifier(nil)
	emitNotify(TaskNotification{Kind: NotifyBlocked, TaskID: "x"})
}

// A registered notifier receives the notification.
func TestEmitNotify_DeliversToNotifier(t *testing.T) {
	got := make(chan TaskNotification, 1)
	SetTaskNotifier(notifierFunc(func(n TaskNotification) { got <- n }))
	t.Cleanup(func() { SetTaskNotifier(nil) })

	emitNotify(TaskNotification{Kind: NotifyFailed, TaskID: "t9", Title: "X"})
	select {
	case n := <-got:
		if n.TaskID != "t9" || n.Kind != NotifyFailed {
			t.Fatalf("got %+v", n)
		}
	case <-time.After(time.Second):
		t.Fatal("notifier did not receive notification")
	}
}

type notifierFunc func(TaskNotification)

func (f notifierFunc) Notify(n TaskNotification) { f(n) }
