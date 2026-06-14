package agent

import (
	"testing"
	"time"
)

// newTestScheduler returns a scheduler (no runner: state transitions only)
// plus its workspace ref and store.
func newTestScheduler(t *testing.T) (*Scheduler, WorkspaceRef, *TasksStore) {
	t.Helper()
	t.Setenv("ONEAGENTS_HOME", t.TempDir())
	store, err := NewTasksStore()
	if err != nil {
		t.Fatalf("NewTasksStore: %v", err)
	}
	ref := WorkspaceRef{ID: "ws1", Name: "W", Path: t.TempDir()}
	s := NewScheduler(store, func() ([]WorkspaceRef, error) { return []WorkspaceRef{ref}, nil })
	return s, ref, store
}

func saveTasks(t *testing.T, store *TasksStore, path string, tasks []Task) {
	t.Helper()
	if err := store.Save(path, &TasksConfig{Tasks: tasks}); err != nil {
		t.Fatalf("Save: %v", err)
	}
}

func statusOf(t *testing.T, store *TasksStore, path, id string) TaskStatus {
	t.Helper()
	cfg, err := store.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, task := range cfg.Tasks {
		if task.ID == id {
			return task.Status
		}
	}
	t.Fatalf("task %s not found", id)
	return ""
}

func setStatus(t *testing.T, store *TasksStore, path, id string, status TaskStatus) {
	t.Helper()
	cfg, _ := store.Load(path)
	for i := range cfg.Tasks {
		if cfg.Tasks[i].ID == id {
			cfg.Tasks[i].Status = status
		}
	}
	saveTasks(t, store, path, cfg.Tasks)
}

func TestSchedulerSubtaskGatesParent(t *testing.T) {
	s, ref, store := newTestScheduler(t)
	now := time.Now().UTC()
	saveTasks(t, store, ref.Path, []Task{
		{ID: "parent", Title: "P", Description: "父任务自己的活", Status: TaskStatusPending, CreatedAt: now, UpdatedAt: now},
		{ID: "child", Title: "C", Description: "子任务", ParentID: "parent", Status: TaskStatusPending, CreatedAt: now.Add(time.Second), UpdatedAt: now},
	})

	s.Tick()
	// Child runs first; parent is gated by its unfinished subtask.
	if got := statusOf(t, store, ref.Path, "child"); got != TaskStatusRunning {
		t.Fatalf("child = %s, want running", got)
	}
	if got := statusOf(t, store, ref.Path, "parent"); got != TaskStatusPending {
		t.Fatalf("parent = %s, want pending (gated)", got)
	}

	// Child completes → parent becomes runnable.
	s.Lock.Release(ref.Path)
	setStatus(t, store, ref.Path, "child", TaskStatusCompleted)
	s.Tick()
	if got := statusOf(t, store, ref.Path, "parent"); got != TaskStatusRunning {
		t.Fatalf("parent = %s, want running after children done", got)
	}
}

func TestSchedulerContainerParentAutoCompletes(t *testing.T) {
	s, ref, store := newTestScheduler(t)
	now := time.Now().UTC()
	saveTasks(t, store, ref.Path, []Task{
		{ID: "parent", Title: "纯容器", Status: TaskStatusPending, CreatedAt: now, UpdatedAt: now}, // no description
		{ID: "c1", Title: "C1", Description: "x", ParentID: "parent", Status: TaskStatusCompleted, CreatedAt: now, UpdatedAt: now},
		{ID: "c2", Title: "C2", Description: "y", ParentID: "parent", Status: TaskStatusCompleted, CreatedAt: now, UpdatedAt: now},
	})

	s.Tick()
	if got := statusOf(t, store, ref.Path, "parent"); got != TaskStatusCompleted {
		t.Fatalf("container parent = %s, want completed", got)
	}
}

func TestSchedulerPriorityOrder(t *testing.T) {
	s, ref, store := newTestScheduler(t)
	now := time.Now().UTC()
	saveTasks(t, store, ref.Path, []Task{
		{ID: "low", Title: "L", Description: "x", Priority: PriorityLow, Status: TaskStatusPending, CreatedAt: now, UpdatedAt: now},
		{ID: "urgent", Title: "U", Description: "y", Priority: PriorityUrgent, Status: TaskStatusPending, CreatedAt: now.Add(time.Minute), UpdatedAt: now},
	})

	s.Tick()
	if got := statusOf(t, store, ref.Path, "urgent"); got != TaskStatusRunning {
		t.Fatalf("urgent = %s, want running (priority wins over FIFO)", got)
	}
	if got := statusOf(t, store, ref.Path, "low"); got == TaskStatusRunning {
		t.Fatalf("low should not run while urgent holds the lock")
	}
}

func TestSchedulerFutureTriggerWaits(t *testing.T) {
	s, ref, store := newTestScheduler(t)
	now := time.Now().UTC()
	future := now.Add(time.Hour)
	saveTasks(t, store, ref.Path, []Task{
		{ID: "later", Title: "L", Description: "x", PlannedStart: &future, Status: TaskStatusPending, CreatedAt: now, UpdatedAt: now},
	})
	s.Tick()
	if got := statusOf(t, store, ref.Path, "later"); got != TaskStatusPending {
		t.Fatalf("future task = %s, want pending", got)
	}
}

func TestSchedulerRetryRequeue(t *testing.T) {
	s, ref, store := newTestScheduler(t)
	now := time.Now().UTC()
	saveTasks(t, store, ref.Path, []Task{
		{ID: "flaky", Title: "F", Description: "x", MaxRetries: 1, Status: TaskStatusFailed, CreatedAt: now, UpdatedAt: now},
	})

	s.Tick() // requeues (retry 1/1) and immediately picks it up
	cfg, _ := store.Load(ref.Path)
	if cfg.Tasks[0].RetryCount != 1 {
		t.Fatalf("retryCount = %d, want 1", cfg.Tasks[0].RetryCount)
	}
	if cfg.Tasks[0].Status != TaskStatusRunning {
		t.Fatalf("status = %s, want running (requeued then started)", cfg.Tasks[0].Status)
	}

	// Fails again: budget exhausted → stays failed.
	s.Lock.Release(ref.Path)
	setStatus(t, store, ref.Path, "flaky", TaskStatusFailed)
	s.Tick()
	if got := statusOf(t, store, ref.Path, "flaky"); got != TaskStatusFailed {
		t.Fatalf("status = %s, want failed (no budget left)", got)
	}
}

func TestSchedulerDependencyBlocks(t *testing.T) {
	s, ref, store := newTestScheduler(t)
	now := time.Now().UTC()
	saveTasks(t, store, ref.Path, []Task{
		{ID: "dep", Title: "D", Description: "x", Status: TaskStatusPending, CreatedAt: now, UpdatedAt: now},
		{ID: "waiter", Title: "W", Description: "y", DependsOn: []string{"dep"}, Status: TaskStatusPending, CreatedAt: now.Add(time.Second), UpdatedAt: now},
	})

	s.Tick()
	// The dep is incomplete, so waiter is surfaced as blocked; dep itself runs.
	if got := statusOf(t, store, ref.Path, "waiter"); got != TaskStatusBlocked {
		t.Fatalf("waiter = %s, want blocked while dep incomplete", got)
	}

	// dep completes → next tick unblocks waiter (→pending) and, with the lock
	// free, starts it.
	s.Lock.Release(ref.Path)
	setStatus(t, store, ref.Path, "dep", TaskStatusCompleted)
	s.Tick()
	if got := statusOf(t, store, ref.Path, "waiter"); got != TaskStatusRunning {
		t.Fatalf("waiter = %s, want running after dep completed", got)
	}
}

func TestSchedulerRecurrenceRespawn(t *testing.T) {
	s, ref, store := newTestScheduler(t)
	now := time.Now().UTC()
	done := now.Add(-time.Hour)
	saveTasks(t, store, ref.Path, []Task{
		{ID: "daily", Title: "日报", Description: "写日报", Status: TaskStatusCompleted,
			CompletedAt: &done, Recurrence: &Recurrence{Freq: "daily", At: "09:00"},
			CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now},
	})

	s.Tick()
	cfg, _ := store.Load(ref.Path)
	if len(cfg.Tasks) != 2 {
		t.Fatalf("tasks = %d, want 2 (original + respawn)", len(cfg.Tasks))
	}
	var original, clone *Task
	for i := range cfg.Tasks {
		if cfg.Tasks[i].ID == "daily" {
			original = &cfg.Tasks[i]
		} else {
			clone = &cfg.Tasks[i]
		}
	}
	if original.Recurrence != nil {
		t.Fatalf("original should lose its recurrence after respawn")
	}
	if clone == nil || clone.Status != TaskStatusPending || clone.ScheduledAt == nil ||
		!clone.ScheduledAt.After(now) || clone.Recurrence == nil || clone.CreatedBy != "scheduler" {
		t.Fatalf("bad respawn: %+v", clone)
	}
	if len(clone.Replies) != 0 || clone.RetryCount != 0 {
		t.Fatalf("respawn should be clean: %+v", clone)
	}

	// Second tick must not respawn again.
	s.Tick()
	cfg, _ = store.Load(ref.Path)
	if len(cfg.Tasks) != 2 {
		t.Fatalf("tasks = %d after second tick, want 2 (no duplicate respawn)", len(cfg.Tasks))
	}
}

func TestNextOccurrence(t *testing.T) {
	// 2026-06-10 is a Wednesday.
	base := time.Date(2026, 6, 10, 12, 0, 0, 0, time.Local)

	d := nextOccurrence(base, &Recurrence{Freq: "daily", At: "09:00"}).Local()
	if d.Day() != 11 || d.Hour() != 9 {
		t.Fatalf("daily: got %v", d)
	}
	// Weekly Monday (1) → 2026-06-15.
	w := nextOccurrence(base, &Recurrence{Freq: "weekly", Weekday: 1, At: "09:00"}).Local()
	if w.Weekday() != time.Monday || w.Day() != 15 {
		t.Fatalf("weekly: got %v", w)
	}
	// Monthly on the 5th → next month (July 5) since June 5 already passed.
	m := nextOccurrence(base, &Recurrence{Freq: "monthly", Monthday: 5, At: "09:00"}).Local()
	if m.Month() != time.July || m.Day() != 5 {
		t.Fatalf("monthly: got %v", m)
	}
	// Monthly day 31 clamps in shorter months: from June 12, June has 30 days → June 30.
	c := nextOccurrence(time.Date(2026, 6, 12, 0, 0, 0, 0, time.Local), &Recurrence{Freq: "monthly", Monthday: 31, At: "09:00"}).Local()
	if c.Month() != time.June || c.Day() != 30 {
		t.Fatalf("monthly clamp: got %v", c)
	}
}
