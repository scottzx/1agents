package agent

import (
	"context"
	"log"
	"sort"
	"sync"
	"time"
)

type WorkspaceLock struct {
	mu      sync.Mutex
	running map[string]string // workspacePath -> task ID
}

func NewWorkspaceLock() *WorkspaceLock {
	return &WorkspaceLock{
		running: make(map[string]string),
	}
}

func (wl *WorkspaceLock) TryAcquire(workspace, taskId string) bool {
	wl.mu.Lock()
	defer wl.mu.Unlock()
	if _, occupied := wl.running[workspace]; occupied {
		return false // Workspace concurrency lock occupied
	}
	wl.running[workspace] = taskId
	return true
}

func (wl *WorkspaceLock) Release(workspace string) {
	wl.mu.Lock()
	defer wl.mu.Unlock()
	delete(wl.running, workspace)
}

func (wl *WorkspaceLock) GetRunning(workspace string) (string, bool) {
	wl.mu.Lock()
	defer wl.mu.Unlock()
	id, occupied := wl.running[workspace]
	return id, occupied
}

// Scheduler is the automation heart of the project model: time is the
// trigger. Every tick it walks each workspace and, for tasks whose trigger
// time has arrived, whose dependencies (including subtasks) are met, picks
// the highest-priority one and hands it to the headless TaskRunner — no
// frontend involvement. It also auto-completes container parents, requeues
// failed tasks with retry budget left, and respawns recurring tasks.
type Scheduler struct {
	Lock         *WorkspaceLock
	tasksStore   *TasksStore
	workspacesFn func() ([]WorkspaceRef, error)
	runner       *TaskRunner
	ticker       *time.Ticker
}

func NewScheduler(tasksStore *TasksStore, workspacesFn func() ([]WorkspaceRef, error)) *Scheduler {
	return &Scheduler{
		Lock:         NewWorkspaceLock(),
		tasksStore:   tasksStore,
		workspacesFn: workspacesFn,
	}
}

// SetRunner attaches the headless executor (set after construction — the
// runner needs the scheduler's lock, the scheduler needs the runner).
// Without a runner the scheduler only performs state transitions, which is
// what the unit tests exercise.
func (s *Scheduler) SetRunner(r *TaskRunner) { s.runner = r }

func (s *Scheduler) Start(ctx context.Context) {
	s.ticker = time.NewTicker(5 * time.Second)
	go func() {
		for {
			select {
			case <-s.ticker.C:
				s.Tick()
			case <-ctx.Done():
				if s.ticker != nil {
					s.ticker.Stop()
				}
				log.Println("[scheduler] Tasks scheduler stopped.")
				return
			}
		}
	}()
	log.Println("[scheduler] Tasks scheduler started.")
}

func (s *Scheduler) Tick() {
	refs, err := s.workspacesFn()
	if err != nil {
		log.Printf("[scheduler] Failed to list workspaces: %v", err)
		return
	}

	for _, ref := range refs {
		s.tickWorkspace(ref)
	}
}

// triggerTime returns when a task becomes eligible to run: explicit
// schedule first, then plannedStart, else immediately (nil).
func triggerTime(t *Task) *time.Time {
	if t.ScheduleType == ScheduleTypeScheduled && t.ScheduledAt != nil {
		return t.ScheduledAt
	}
	if t.PlannedStart != nil {
		return t.PlannedStart
	}
	return nil
}

func (s *Scheduler) tickWorkspace(ref WorkspaceRef) {
	cfg, err := s.tasksStore.Load(ref.Path)
	if err != nil {
		return
	}

	modified := false
	now := time.Now().UTC()

	taskMap := make(map[string]*Task)
	childrenOf := make(map[string][]*Task)
	for i := range cfg.Tasks {
		t := &cfg.Tasks[i]
		taskMap[t.ID] = t
		if t.ParentID != "" {
			childrenOf[t.ParentID] = append(childrenOf[t.ParentID], t)
		}
	}

	allChildrenCompleted := func(t *Task) bool {
		for _, c := range childrenOf[t.ID] {
			if c.Status != TaskStatusCompleted {
				return false
			}
		}
		return true
	}

	// 1. Container parents (no description of their own): once every
	//    subtask is completed, the parent is complete — nothing to run.
	for i := range cfg.Tasks {
		t := &cfg.Tasks[i]
		if t.Status == TaskStatusCompleted || t.Status == TaskStatusRunning {
			continue
		}
		if t.Description == "" && len(childrenOf[t.ID]) > 0 && allChildrenCompleted(t) {
			t.Status = TaskStatusCompleted
			t.CompletedAt = &now
			t.UpdatedAt = now
			modified = true
			log.Printf("[scheduler] Container task %s completed (all subtasks done)", t.ID)
		}
	}

	// 2. Failed tasks with retry budget left go back to pending. The
	//    failure reason is already on the timeline, so the next run's
	//    injected background carries it.
	for i := range cfg.Tasks {
		t := &cfg.Tasks[i]
		if t.Status == TaskStatusFailed && t.RetryCount < t.MaxRetries {
			t.RetryCount++
			t.Status = TaskStatusPending
			t.UpdatedAt = now
			modified = true
			log.Printf("[scheduler] Task %s requeued for retry %d/%d", t.ID, t.RetryCount, t.MaxRetries)
		}
	}

	// 3. Recurring tasks: when an instance completes, spawn the next one
	//    and strip the rule from the finished instance (history stays).
	for i := range cfg.Tasks {
		t := &cfg.Tasks[i]
		if t.Status != TaskStatusCompleted || t.Recurrence == nil {
			continue
		}
		next := nextOccurrence(now, t.Recurrence)
		clone := *t
		clone.ID = newID()
		clone.Status = TaskStatusPending
		clone.ScheduleType = ScheduleTypeScheduled
		clone.ScheduledAt = &next
		clone.PlannedStart = nil
		clone.StartedAt = nil
		clone.CompletedAt = nil
		clone.Summary = ""
		clone.RetryCount = 0
		clone.CreatedBy = "scheduler"
		clone.CreatedAt = now
		clone.UpdatedAt = now
		clone.Replies = []Reply{}
		clone.Sessions = []SessionMetadata{}
		t.Recurrence = nil
		t.UpdatedAt = now
		cfg.Tasks = append(cfg.Tasks, clone)
		modified = true
		log.Printf("[scheduler] Recurring task %s respawned as %s (next run %s)", t.ID, clone.ID, next)
	}
	// cfg.Tasks may have been reallocated by append: rebuild the index
	// before the ready-scan below.
	taskMap = make(map[string]*Task)
	childrenOf = make(map[string][]*Task)
	for i := range cfg.Tasks {
		t := &cfg.Tasks[i]
		taskMap[t.ID] = t
		if t.ParentID != "" {
			childrenOf[t.ParentID] = append(childrenOf[t.ParentID], t)
		}
	}

	// 4. Collect ready tasks: trigger time arrived, dependencies met,
	//    subtasks (implicit dependencies) all completed, issue open.
	var ready []*Task
	for i := range cfg.Tasks {
		t := &cfg.Tasks[i]
		if t.Status != TaskStatusPending && t.Status != TaskStatusQueued {
			continue
		}
		if t.IssueState == IssueClosed {
			continue
		}
		if trig := triggerTime(t); trig != nil && trig.After(now) {
			continue
		}
		depsMet := true
		for _, depId := range t.DependsOn {
			dep, exists := taskMap[depId]
			if !exists || dep.Status != TaskStatusCompleted {
				depsMet = false
				break
			}
		}
		if !depsMet {
			continue
		}
		// 父任务天生将子任务作为依赖项: a parent with unfinished
		// subtasks is not runnable.
		if !allChildrenCompleted(t) {
			continue
		}
		if t.Status == TaskStatusPending {
			t.Status = TaskStatusQueued
			t.UpdatedAt = now
			modified = true
		}
		ready = append(ready, t)
	}

	// 5. Highest priority first; FIFO by creation within a rank.
	sort.SliceStable(ready, func(i, j int) bool {
		ri, rj := PriorityRank(ready[i].Priority), PriorityRank(ready[j].Priority)
		if ri != rj {
			return ri < rj
		}
		return ready[i].CreatedAt.Before(ready[j].CreatedAt)
	})

	if len(ready) > 0 && s.Lock.TryAcquire(ref.Path, ready[0].ID) {
		task := ready[0]
		task.Status = TaskStatusRunning
		task.StartedAt = &now
		task.UpdatedAt = now
		modified = true
		log.Printf("[scheduler] Lock acquired. Task %s (%s, priority %s) starting in %s",
			task.ID, task.Title, task.Priority, ref.Path)

		if s.runner != nil {
			// Copy the task before Save below mutates the slice.
			run := *task
			go s.runner.Execute(ref.Path, ref.ID, run)
		} else {
			// No executor attached (unit tests): release so the lock
			// doesn't leak.
			s.Lock.Release(ref.Path)
		}
	}

	if modified {
		if err := s.tasksStore.Save(ref.Path, cfg); err != nil {
			log.Printf("[scheduler] Failed to save tasks config in %s: %v", ref.Path, err)
		}
	}
}

// nextOccurrence computes the next trigger after `after` for a simple-enum
// recurrence rule. At ("HH:MM", local time) defaults to midnight.
func nextOccurrence(after time.Time, r *Recurrence) time.Time {
	hour, minute := 0, 0
	if len(r.At) == 5 {
		if t, err := time.Parse("15:04", r.At); err == nil {
			hour, minute = t.Hour(), t.Minute()
		}
	}
	local := after.Local()
	candidate := time.Date(local.Year(), local.Month(), local.Day(), hour, minute, 0, 0, local.Location())

	switch r.Freq {
	case "weekly":
		for candidate.Weekday() != time.Weekday(r.Weekday) || !candidate.After(local) {
			candidate = candidate.AddDate(0, 0, 1)
		}
	case "monthly":
		day := r.Monthday
		if day < 1 {
			day = 1
		}
		candidate = monthlyAt(local, day, hour, minute)
		if !candidate.After(local) {
			candidate = monthlyAt(local.AddDate(0, 1, 0), day, hour, minute)
		}
	default: // daily
		if !candidate.After(local) {
			candidate = candidate.AddDate(0, 0, 1)
		}
	}
	return candidate.UTC()
}

// monthlyAt returns the given day-of-month (clamped to the month's length)
// at hour:minute in ref's month.
func monthlyAt(ref time.Time, day, hour, minute int) time.Time {
	firstOfMonth := time.Date(ref.Year(), ref.Month(), 1, 0, 0, 0, 0, ref.Location())
	lastDay := firstOfMonth.AddDate(0, 1, -1).Day()
	if day > lastDay {
		day = lastDay
	}
	return time.Date(ref.Year(), ref.Month(), day, hour, minute, 0, 0, ref.Location())
}
