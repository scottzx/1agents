package agent

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/scottzx/1Agents/backend/internal/execution"
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

// srcReqID is the id of the seed requirement that other test tasks link to so
// they satisfy the #68 sourcing gate (every project-internal executable task
// must trace to a requirement/bug). Tests that only exercise other gates use
// withSource/srcReq to keep their executable tasks runnable.
const srcReqID = "src-req"

// srcReq returns a seed requirement to drop into a test's task slice; pair it
// with withSource-stamped tasks.
func srcReq(now time.Time) Task {
	return Task{ID: srcReqID, Title: "来源需求", Type: TaskTypeRequirement, Description: "x",
		IssueState: IssueOpen, Status: TaskStatusPending, CreatedAt: now, UpdatedAt: now}
}

// withSource stamps a relates-link to the seed requirement onto t so it passes
// the #68 sourcing gate, unless t already carries links.
func withSource(t Task) Task {
	if len(t.Links) == 0 {
		t.Links = []TaskLink{{Target: srcReqID, Rel: LinkRelates}}
	}
	return t
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
		srcReq(now),
		withSource(Task{ID: "parent", Title: "P", Description: "父任务自己的活", AcceptanceCriteria: "done", Status: TaskStatusPending, CreatedAt: now, UpdatedAt: now}),
		// child inherits the parent's sourcing — no own link needed.
		{ID: "child", Title: "C", Description: "子任务", AcceptanceCriteria: "done", ParentID: "parent", Status: TaskStatusPending, CreatedAt: now.Add(time.Second), UpdatedAt: now},
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

func TestSchedulerSkipsDiscussion(t *testing.T) {
	s, ref, store := newTestScheduler(t)
	now := time.Now().UTC()
	// A discussion is a concept record: pending + open, but it must never be
	// queued or run by the scheduler.
	saveTasks(t, store, ref.Path, []Task{
		{ID: "disc", Title: "方向讨论", Type: TaskTypeDiscussion, Description: "x",
			IssueState: IssueOpen, Status: TaskStatusPending, CreatedAt: now, UpdatedAt: now},
	})

	s.Tick()

	if got := statusOf(t, store, ref.Path, "disc"); got != TaskStatusPending {
		t.Fatalf("discussion = %s, want pending (never scheduled)", got)
	}
}

func TestSchedulerSkipsSuggestion(t *testing.T) {
	s, ref, store := newTestScheduler(t)
	now := time.Now().UTC()
	// An AI suggestion (issue #47) is a proposal: pending + open like a normal
	// task, but source = agent-suggested must hold it out of scheduling until a
	// human adopts it (which clears the source marker).
	saveTasks(t, store, ref.Path, []Task{
		{ID: "sug", Title: "顺手清理死代码", Source: TaskSourceAgent, Description: "见 foo.go:42",
			IssueState: IssueOpen, Status: TaskStatusPending, CreatedAt: now, UpdatedAt: now},
	})

	s.Tick()

	if got := statusOf(t, store, ref.Path, "sug"); got != TaskStatusPending {
		t.Fatalf("suggestion = %s, want pending (never scheduled until adopted)", got)
	}
}

func TestSchedulerHoldsTaskWithoutAcceptanceCriteria(t *testing.T) {
	s, ref, store := newTestScheduler(t)
	now := time.Now().UTC()
	// An executable task with real work but no acceptance criteria (#135) must be
	// held as not_ready and never queued/run — the agent has no "怎样算完成".
	saveTasks(t, store, ref.Path, []Task{
		srcReq(now),
		withSource(Task{ID: "vague", Title: "随便做点啥", Description: "做点事", IssueState: IssueOpen,
			Status: TaskStatusPending, CreatedAt: now, UpdatedAt: now}),
	})

	s.Tick()
	if got := statusOf(t, store, ref.Path, "vague"); got != TaskStatusNotReady {
		t.Fatalf("task without criteria = %s, want not_ready", got)
	}
	if _, occupied := s.Lock.GetRunning(ref.Path); occupied {
		t.Fatalf("not_ready task must not acquire the workspace lock")
	}

	// Filling in acceptance criteria releases the hold: not_ready → pending →
	// queued/running on the next tick.
	cfg, _ := store.Load(ref.Path)
	for i := range cfg.Tasks {
		if cfg.Tasks[i].ID == "vague" {
			cfg.Tasks[i].AcceptanceCriteria = "做完且通过自查"
		}
	}
	saveTasks(t, store, ref.Path, cfg.Tasks)
	s.Tick()
	if got := statusOf(t, store, ref.Path, "vague"); got != TaskStatusRunning {
		t.Fatalf("task with criteria = %s, want running after criteria filled", got)
	}
}

func TestSchedulerBlockedLabelHoldsTask(t *testing.T) {
	s, ref, store := newTestScheduler(t)
	now := time.Now().UTC()
	// The `blocked` reserved label (#134) is an explicit manual hold: a fully
	// runnable task must be gated into `blocked` and never acquire the lock.
	saveTasks(t, store, ref.Path, []Task{
		srcReq(now),
		withSource(Task{ID: "held", Title: "勿动", Description: "x", AcceptanceCriteria: "done",
			Labels: []string{"blocked"}, IssueState: IssueOpen,
			Status: TaskStatusPending, CreatedAt: now, UpdatedAt: now}),
	})

	s.Tick()
	if got := statusOf(t, store, ref.Path, "held"); got != TaskStatusBlocked {
		t.Fatalf("task with blocked label = %s, want blocked", got)
	}
	if _, occupied := s.Lock.GetRunning(ref.Path); occupied {
		t.Fatalf("blocked-label task must not acquire the workspace lock")
	}

	// Removing the label releases the hold: blocked → pending → running.
	cfg, _ := store.Load(ref.Path)
	for i := range cfg.Tasks {
		if cfg.Tasks[i].ID == "held" {
			cfg.Tasks[i].Labels = nil
		}
	}
	saveTasks(t, store, ref.Path, cfg.Tasks)
	s.Tick()
	if got := statusOf(t, store, ref.Path, "held"); got != TaskStatusRunning {
		t.Fatalf("task after label removed = %s, want running", got)
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
	parent, found, err := store.GetTask("parent")
	if err != nil || !found || parent.ClosedBy == nil || parent.ClosedBy.TaskRunID == "" {
		t.Fatalf("container completion missing audit: parent=%+v found=%v err=%v", parent, found, err)
	}
	runs, err := store.TaskRuns().ListByTask("parent")
	if err != nil || len(runs) != 1 || len(runs[0].Evidence) != 1 ||
		runs[0].Evidence[0].Kind != "children_terminal" {
		t.Fatalf("container TaskRun=%+v err=%v", runs, err)
	}
}

func TestSchedulerPriorityOrder(t *testing.T) {
	s, ref, store := newTestScheduler(t)
	now := time.Now().UTC()
	saveTasks(t, store, ref.Path, []Task{
		srcReq(now),
		withSource(Task{ID: "low", Title: "L", Description: "x", AcceptanceCriteria: "done", Priority: PriorityLow, Status: TaskStatusPending, CreatedAt: now, UpdatedAt: now}),
		withSource(Task{ID: "urgent", Title: "U", Description: "y", AcceptanceCriteria: "done", Priority: PriorityUrgent, Status: TaskStatusPending, CreatedAt: now.Add(time.Minute), UpdatedAt: now}),
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
		srcReq(now),
		withSource(Task{ID: "later", Title: "L", Description: "x", AcceptanceCriteria: "done", PlannedStart: &future, Status: TaskStatusPending, CreatedAt: now, UpdatedAt: now}),
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
		srcReq(now),
		withSource(Task{ID: "flaky", Title: "F", Description: "x", AcceptanceCriteria: "done", MaxRetries: 1, Status: TaskStatusFailed, CreatedAt: now, UpdatedAt: now}),
	})

	s.Tick() // requeues (retry 1/1) and immediately picks it up
	cfg, _ := store.Load(ref.Path)
	var flaky *Task
	for i := range cfg.Tasks {
		if cfg.Tasks[i].ID == "flaky" {
			flaky = &cfg.Tasks[i]
		}
	}
	if flaky.RetryCount != 1 {
		t.Fatalf("retryCount = %d, want 1", flaky.RetryCount)
	}
	if flaky.Status != TaskStatusRunning {
		t.Fatalf("status = %s, want running (requeued then started)", flaky.Status)
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
		srcReq(now),
		withSource(Task{ID: "dep", Title: "D", Description: "x", AcceptanceCriteria: "done", Status: TaskStatusPending, CreatedAt: now, UpdatedAt: now}),
		withSource(Task{ID: "waiter", Title: "W", Description: "y", AcceptanceCriteria: "done", DependsOn: []string{"dep"}, Status: TaskStatusPending, CreatedAt: now.Add(time.Second), UpdatedAt: now}),
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

// A requirement/bug auto-closes once every task decomposed under it (its
// ParentID children) is terminal — completed or cancelled. This is the natural
// close path that replaced the old explicit "closes"-link propagation.
func TestSchedulerRequirementAutoClosesWhenChildrenDone(t *testing.T) {
	s, ref, store := newTestScheduler(t)
	now := time.Now().UTC()
	saveTasks(t, store, ref.Path, []Task{
		{ID: "req", Title: "需求", Type: TaskTypeRequirement, Description: "x", IssueState: IssueOpen,
			Status: TaskStatusPending, CreatedAt: now, UpdatedAt: now},
		{ID: "c1", Title: "任务1", Description: "y", ParentID: "req", Status: TaskStatusCompleted,
			CreatedAt: now.Add(time.Second), UpdatedAt: now},
		{ID: "c2", Title: "任务2", Description: "z", ParentID: "req", Status: TaskStatusCancelled,
			CreatedAt: now.Add(2 * time.Second), UpdatedAt: now},
	})

	s.Tick()

	cfg, _ := store.Load(ref.Path)
	var req *Task
	for i := range cfg.Tasks {
		if cfg.Tasks[i].ID == "req" {
			req = &cfg.Tasks[i]
		}
	}
	if req.IssueState != IssueClosed {
		t.Fatalf("req issueState = %s, want closed", req.IssueState)
	}
	if req.CompletedAt == nil {
		t.Fatalf("req should have CompletedAt set")
	}
	if len(req.Replies) != 1 || req.Replies[0].Mode != ModePureComment {
		t.Fatalf("req should have one pure_comment timeline entry, got %+v", req.Replies)
	}

	// Idempotent: a second tick must not append another close comment.
	s.Tick()
	cfg, _ = store.Load(ref.Path)
	for i := range cfg.Tasks {
		if cfg.Tasks[i].ID == "req" && len(cfg.Tasks[i].Replies) != 1 {
			t.Fatalf("second tick re-closed: %d replies", len(cfg.Tasks[i].Replies))
		}
	}
}

// A requirement with an unfinished child stays open; one with no children is
// never auto-closed (nothing was decomposed to finish).
func TestSchedulerRequirementStaysOpenUntilChildrenDone(t *testing.T) {
	s, ref, store := newTestScheduler(t)
	now := time.Now().UTC()
	saveTasks(t, store, ref.Path, []Task{
		{ID: "req", Title: "需求", Type: TaskTypeRequirement, Description: "x", IssueState: IssueOpen,
			Status: TaskStatusPending, CreatedAt: now, UpdatedAt: now},
		{ID: "c1", Title: "任务1", Description: "y", ParentID: "req", Status: TaskStatusCompleted,
			CreatedAt: now.Add(time.Second), UpdatedAt: now},
		{ID: "c2", Title: "任务2", Description: "z", ParentID: "req", Status: TaskStatusRunning,
			CreatedAt: now.Add(2 * time.Second), UpdatedAt: now},
		{ID: "lonely", Title: "无子需求", Type: TaskTypeRequirement, Description: "x", IssueState: IssueOpen,
			Status: TaskStatusPending, CreatedAt: now, UpdatedAt: now},
	})

	s.Tick()

	cfg, _ := store.Load(ref.Path)
	for i := range cfg.Tasks {
		if (cfg.Tasks[i].ID == "req" || cfg.Tasks[i].ID == "lonely") && cfg.Tasks[i].IssueState == IssueClosed {
			t.Fatalf("%s should stay open", cfg.Tasks[i].ID)
		}
	}
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }

func TestSchedulerRecurrenceRespawn(t *testing.T) {
	s, ref, store := newTestScheduler(t)
	now := time.Now().UTC()
	done := now.Add(-time.Hour)
	saveTasks(t, store, ref.Path, []Task{
		srcReq(now),
		withSource(Task{ID: "daily", Title: "日报", Description: "写日报", AcceptanceCriteria: "done", Status: TaskStatusCompleted,
			CompletedAt: &done, Recurrence: &Recurrence{Freq: "daily", At: "09:00"},
			CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now}),
	})

	s.Tick()
	cfg, _ := store.Load(ref.Path)
	if len(cfg.Tasks) != 3 {
		t.Fatalf("tasks = %d, want 3 (seed requirement + original + respawn)", len(cfg.Tasks))
	}
	var original, clone *Task
	for i := range cfg.Tasks {
		switch cfg.Tasks[i].ID {
		case "daily":
			original = &cfg.Tasks[i]
		case srcReqID:
			// seed requirement, ignore
		default:
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
	if len(cfg.Tasks) != 3 {
		t.Fatalf("tasks = %d after second tick, want 3 (no duplicate respawn)", len(cfg.Tasks))
	}
}

// TestSchedulerHoldsTaskWithoutSourcing covers the #68 任务归口 gate: a
// project-internal executable task with full acceptance criteria but no
// requirement/bug sourcing link is held as not_ready, and adding a relates-link
// to a requirement releases it.
func TestSchedulerHoldsTaskWithoutSourcing(t *testing.T) {
	s, ref, store := newTestScheduler(t)
	now := time.Now().UTC()
	saveTasks(t, store, ref.Path, []Task{
		{ID: "req", Title: "需求", Type: TaskTypeRequirement, Description: "x",
			IssueState: IssueOpen, Status: TaskStatusPending, CreatedAt: now, UpdatedAt: now},
		// Fully-specified executable task, but it traces to nothing.
		{ID: "orphan", Title: "裸任务", Description: "做点事", AcceptanceCriteria: "done",
			IssueState: IssueOpen, Status: TaskStatusPending, CreatedAt: now.Add(time.Second), UpdatedAt: now},
	})

	s.Tick()
	if got := statusOf(t, store, ref.Path, "orphan"); got != TaskStatusNotReady {
		t.Fatalf("un-sourced task = %s, want not_ready", got)
	}
	if _, occupied := s.Lock.GetRunning(ref.Path); occupied {
		t.Fatalf("un-sourced task must not acquire the workspace lock")
	}

	// Add a relates-link to the requirement → the hold releases.
	cfg, _ := store.Load(ref.Path)
	for i := range cfg.Tasks {
		if cfg.Tasks[i].ID == "orphan" {
			cfg.Tasks[i].Links = []TaskLink{{Target: "req", Rel: LinkRelates}}
		}
	}
	saveTasks(t, store, ref.Path, cfg.Tasks)
	s.Tick()
	if got := statusOf(t, store, ref.Path, "orphan"); got != TaskStatusRunning {
		t.Fatalf("sourced task = %s, want running after link added", got)
	}
}

// TestSchedulerSourcingExemptions confirms the gate's exemptions: a requirement
// and a bug are sources (never executed anyway), and a task that links to a bug
// counts as sourced.
func TestSchedulerSourcingExemptions(t *testing.T) {
	s, ref, store := newTestScheduler(t)
	now := time.Now().UTC()
	saveTasks(t, store, ref.Path, []Task{
		{ID: "bug", Title: "缺陷", Type: TaskTypeBug, Description: "x", AcceptanceCriteria: "done",
			IssueState: IssueOpen, Status: TaskStatusPending, CreatedAt: now, UpdatedAt: now},
		{ID: "fix", Title: "修复", Description: "改 bug", AcceptanceCriteria: "done",
			Links: []TaskLink{{Target: "bug", Rel: LinkRelates}}, IssueState: IssueOpen,
			Status: TaskStatusPending, CreatedAt: now.Add(time.Second), UpdatedAt: now},
	})

	s.Tick()
	// The bug is a non-executable issue: it is neither gated as not_ready nor run.
	if got := statusOf(t, store, ref.Path, "bug"); got != TaskStatusPending {
		t.Fatalf("bug = %s, want pending (issue, never gated/run)", got)
	}
	// The fix traces to the bug → sourced → runnable.
	if got := statusOf(t, store, ref.Path, "fix"); got != TaskStatusRunning {
		t.Fatalf("bug-sourced fix = %s, want running", got)
	}
}

// TestNeedsSourcing unit-tests the gate predicate, including subtask
// inheritance and the type/source/container exemptions.
func TestNeedsSourcing(t *testing.T) {
	req := &Task{ID: "r", Type: TaskTypeRequirement}
	parent := &Task{ID: "p", Description: "活", Links: []TaskLink{{Target: "r", Rel: LinkRelates}}}
	taskMap := map[string]*Task{
		"r": req,
		"p": parent,
	}
	cases := []struct {
		name string
		task *Task
		want bool
	}{
		{"orphan executable", &Task{ID: "a", Description: "活"}, true},
		{"sourced via link", &Task{ID: "b", Description: "活", Links: []TaskLink{{Target: "r", Rel: LinkRelates}}}, false},
		{"subtask inherits parent sourcing", &Task{ID: "c", Description: "活", ParentID: "p"}, false},
		{"requirement is a source", &Task{ID: "d", Type: TaskTypeRequirement, Description: "活"}, false},
		{"bug is a source", &Task{ID: "e", Type: TaskTypeBug, Description: "活"}, false},
		{"agent suggestion exempt", &Task{ID: "f", Source: TaskSourceAgent, Description: "活"}, false},
		{"container parent exempt", &Task{ID: "g"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			taskMap[c.task.ID] = c.task
			if got := needsSourcing(c.task, taskMap); got != c.want {
				t.Fatalf("needsSourcing(%s) = %v, want %v", c.name, got, c.want)
			}
		})
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
	// Interval: fixed spacing from the previous run, ignoring clock time.
	iv := nextOccurrence(base, &Recurrence{Freq: "interval", EveryMinutes: 30})
	if want := base.Add(30 * time.Minute).UTC(); !iv.Equal(want) {
		t.Fatalf("interval: got %v, want %v", iv, want)
	}
	// Interval guards a zero/negative EveryMinutes to at least 1 minute (no busy loop).
	iz := nextOccurrence(base, &Recurrence{Freq: "interval", EveryMinutes: 0})
	if want := base.Add(time.Minute).UTC(); !iz.Equal(want) {
		t.Fatalf("interval zero-guard: got %v, want %v", iz, want)
	}
}

func TestRunExecutionJobPreambleFailureDoesNotStartAgent(t *testing.T) {
	s, ref, store := newTestScheduler(t)
	now := time.Now().UTC()
	saveTasks(t, store, ref.Path, []Task{
		srcReq(now),
		withSource(Task{
			ID: "task-1", Title: "Auto", Type: TaskTypeTask, Description: "do",
			AcceptanceCriteria: "ok", Status: TaskStatusPending, CreatedAt: now, UpdatedAt: now,
		}),
	})
	s.SetRunner(NewTaskRunner(1, "", store, nil, s))
	called := 0
	s.SetPreambleRunner(func(task Task, workspacePath string, job execution.Job) (string, error) {
		called++
		return "", fmt.Errorf("boom")
	})
	if err := s.RunExecutionJob(context.Background(), execution.Job{
		ID: "job-1", WorkItemID: "task-1", ProjectID: ref.ID, ExecutorKind: "agent",
		PreambleFunctionType: "core.script",
	}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && statusOf(t, store, ref.Path, "task-1") != TaskStatusFailed {
		time.Sleep(20 * time.Millisecond)
	}
	if called != 1 {
		t.Fatalf("preamble calls = %d", called)
	}
	if got := statusOf(t, store, ref.Path, "task-1"); got != TaskStatusFailed {
		t.Fatalf("status = %s", got)
	}
}
