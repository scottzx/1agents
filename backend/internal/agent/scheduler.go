package agent

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
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
	// FunctionRunner is called for executor=function tasks (instead of runner.Execute).
	// Set via SetFunctionRunner after construction. When nil, function tasks are
	// left in running state (no-op dispatch, useful in unit tests).
	FunctionRunner func(task Task, workspacePath string)
	ticker         *time.Ticker
	// engine is the event-driven orchestration layer (#133). The scheduler
	// owns state transitions and, at each transition point, emits a TaskEvent
	// the engine maps to declarative actions (route/notify/requeue). Never nil
	// after construction.
	engine *EventEngine
}

func NewScheduler(tasksStore *TasksStore, workspacesFn func() ([]WorkspaceRef, error)) *Scheduler {
	return &Scheduler{
		Lock:         NewWorkspaceLock(),
		tasksStore:   tasksStore,
		workspacesFn: workspacesFn,
		engine:       DefaultEventEngine(),
	}
}

// SetRunner attaches the headless executor (set after construction — the
// runner needs the scheduler's lock, the scheduler needs the runner).
// Without a runner the scheduler only performs state transitions, which is
// what the unit tests exercise.
func (s *Scheduler) SetRunner(r *TaskRunner) { s.runner = r }

// SetFunctionRunner attaches the function-executor dispatch callback.
// See FunctionRunner field.
func (s *Scheduler) SetFunctionRunner(fn func(task Task, workspacePath string)) {
	s.FunctionRunner = fn
}

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

// readyTask is a runnable task plus whether this run is a verification pass
// (pending_review → verifier) rather than a fresh execution.
type readyTask struct {
	task     *Task
	isReview bool
}

// isHumanTask reports whether a task is done by a person rather than dispatched
// to an agent. "谁来做" collapses onto assignee: assignee=user marks a human (the
// personal-todo / reminder / decision-gate case), and the legacy executor=human
// declaration maps onto the same one behavior. executor is thus a derived kind,
// not a second independent axis — one lane for human, not two.
func isHumanTask(t *Task) bool {
	return t.Assignee == AssigneeUser || t.Executor == TaskExecutorHuman
}

// needsAcceptanceCriteria reports whether an executable task is missing the
// acceptance criteria required to enter the runnable queue (#135). Only real
// work qualifies: requirement/bug/discussion items are non-executable issues
// (the ready-scan skips them) and AI suggestions are held out until adopted,
// so none of those are gated here. Container parents (no Description of their
// own) auto-complete from their subtasks and never execute, so they are exempt.
func needsAcceptanceCriteria(t *Task) bool {
	if t.Type != "" && t.Type != TaskTypeTask {
		return false
	}
	if t.Source == TaskSourceAgent {
		return false
	}
	// App/kernel-dispatched tasks (North Task API, #320) carry a business_ref and
	// run under the executor-agnostic ready gate (#319): function/human steps have
	// no agent self-check, and agent steps are scoped by the app's spec — none are
	// PM-authored project tasks, so the acceptance-criteria authoring gate is moot.
	if strings.TrimSpace(t.BusinessRef) != "" || isHumanTask(t) || (t.Executor != "" && t.Executor != TaskExecutorAgent) {
		return false
	}
	if strings.TrimSpace(t.Description) == "" {
		return false // container parent or empty stub — nothing to verify
	}
	return strings.TrimSpace(t.AcceptanceCriteria) == ""
}

// needsSourcing reports whether a project-internal executable task lacks the
// traceability link required to enter the runnable queue (#68 任务归口原则).
// Every project task must answer "我为什么存在" by linking to a requirement or
// bug; a task that doesn't is held as not_ready until someone adds the link.
//
// taskMap resolves a link target id to its task so the target's Type can be
// checked. The scheduler only sweeps the workspace registry, and the personal
// bucket (__personal__) is deliberately absent from it, so every task reaching
// this gate already has a real project_id — that is exactly the "有 project_id
// 强制；无 id 不强制" boundary, enforced structurally rather than re-checked.
//
// Exemptions mirror needsAcceptanceCriteria: requirement/bug/discussion items
// are the sources themselves (and never execute), AI suggestions are held until
// adopted, and container parents (no Description) auto-complete from subtasks. A
// subtask inherits its parent's sourcing — once the parent is sourced (or is a
// container deriving from a requirement/bug), its children need no own link.
func needsSourcing(t *Task, taskMap map[string]*Task) bool {
	if t.Type != "" && t.Type != TaskTypeTask {
		return false
	}
	if t.Source == TaskSourceAgent {
		return false
	}
	// An app task's business_ref IS its "我为什么存在" — it traces to a domain
	// object (lead/episode/material) instead of a requirement/bug. App/kernel-
	// dispatched tasks (#320) are therefore sourced by construction; the #68
	// traceability gate only governs PM-authored project tasks.
	if strings.TrimSpace(t.BusinessRef) != "" || isHumanTask(t) || (t.Executor != "" && t.Executor != TaskExecutorAgent) {
		return false
	}
	if strings.TrimSpace(t.Description) == "" {
		return false // container parent or empty stub
	}
	return !hasSourcingLink(t, taskMap, map[string]bool{})
}

// hasSourcingLink reports whether t (or, by inheritance, an ancestor) carries a
// link whose target resolves to a requirement or bug in the same project.
// "closes" and "relates" both count — either expresses derivation from the
// issue. seen guards against cycles in the parent chain.
func hasSourcingLink(t *Task, taskMap map[string]*Task, seen map[string]bool) bool {
	if t == nil || seen[t.ID] {
		return false
	}
	seen[t.ID] = true
	for _, link := range t.Links {
		tgt, ok := taskMap[link.Target]
		if !ok {
			continue
		}
		if tgt.Type == TaskTypeRequirement || tgt.Type == TaskTypeBug {
			return true
		}
	}
	if t.ParentID != "" {
		return hasSourcingLink(taskMap[t.ParentID], taskMap, seen)
	}
	return false
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
	now := time.Now().UTC()
	// Mutate serializes the whole Load→evaluate→Save cycle against the
	// headless runner's finish() and the chat-ws handlers, so a 5s tick can
	// never overwrite a just-completed status with a stale snapshot.
	_ = s.tasksStore.Mutate(ref.Path, func(cfg *TasksConfig) bool {
		modified := false

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
		//    injected background carries it. A task that failed because its
		//    verification budget is exhausted is terminal (报异常) — the
		//    execution-retry must not silently re-run it.
		for i := range cfg.Tasks {
			t := &cfg.Tasks[i]
			if t.Status == TaskStatusFailed && t.RetryCount < t.MaxRetries && !reviewExhausted(t) {
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
			// Termination: stop respawning when the next run would fall past
			// `until`, or when the occurrence budget (`count`) is spent. This
			// completed instance always counts as one occurrence.
			rec := *t.Recurrence // copy so we can decrement Count for the clone
			if until, ok := parseRecurrenceUntil(rec.Until); ok && next.After(until) {
				t.Recurrence = nil
				t.UpdatedAt = now
				modified = true
				continue
			}
			if rec.Count > 0 {
				if rec.Count <= 1 {
					t.Recurrence = nil
					t.UpdatedAt = now
					modified = true
					continue
				}
				rec.Count--
			}
			clone := *t
			clone.Recurrence = &rec
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

		// 3.4 created → auto-route (#133): a fresh executable task that no one
		//     has assigned an agent to fires an EventCreated; the rule engine
		//     routes it to an agent by type/domain. Idempotent — once Assignee
		//     is set the rule short-circuits, so this fires at most once per
		//     task. Runs before dependency gating so a task is routed even while
		//     it waits on a dependency. Non-executable issues (requirement/bug/
		//     discussion), AI suggestions, and user-owned reminders are skipped:
		//     they never run, so routing them is meaningless.
		for i := range cfg.Tasks {
			t := &cfg.Tasks[i]
			if t.Status != TaskStatusPending && t.Status != TaskStatusQueued {
				continue
			}
			if t.Assignee != "" {
				continue
			}
			if (t.Type != "" && t.Type != TaskTypeTask) || t.Source == TaskSourceAgent {
				continue
			}
			if s.emit(t, EventCreated, now) {
				modified = true
			}
		}

		// 3.5 Dependency gating (blocked state): a task whose explicit
		//     dependencies aren't all completed is surfaced as `blocked` so the
		//     board shows the upstream wait; once they complete it returns to
		//     pending and the ready-scan below can pick it up. Parent/subtask
		//     gating is handled separately (allChildrenCompleted), so it doesn't
		//     mark parents blocked here.
		depsAllCompleted := func(t *Task) bool {
			for _, depID := range t.DependsOn {
				dep, ok := taskMap[depID]
				if !ok || dep.Status != TaskStatusCompleted {
					return false
				}
			}
			return true
		}
		for i := range cfg.Tasks {
			t := &cfg.Tasks[i]
			// Label/field policy signals (#134): the `blocked` reserved label is
			// an explicit manual hold — independent of the dependency graph — so
			// it gates a task into `blocked` just like an unmet dependency would.
			forceBlocked := DeriveSignals(*t).ForceBlocked
			// not_ready covers both authoring gaps: missing acceptance criteria
			// (#135) and missing requirement/bug sourcing (#68). Either holds the
			// task out of the queue; filling the gap releases it back to pending.
			notReady := needsAcceptanceCriteria(t) || needsSourcing(t, taskMap)
			switch t.Status {
			case TaskStatusPending, TaskStatusQueued:
				// Readiness gate (#135/#68): an executable task missing acceptance
				// criteria or its requirement/bug sourcing is held as not_ready
				// instead of being queued. Checked before dependency gating so an
				// under-specified task surfaces its authoring gap first.
				if notReady {
					t.Status = TaskStatusNotReady
					t.UpdatedAt = now
					modified = true
				} else if forceBlocked || (len(t.DependsOn) > 0 && !depsAllCompleted(t)) {
					t.Status = TaskStatusBlocked
					t.UpdatedAt = now
					modified = true
					// blocked → notify PM (#133). Emitted only on the transition
					// into blocked so the PM is pinged once, not every tick.
					s.emit(t, EventBlocked, now)
					// blocked → push an IM approve/reject card (#129).
					emitNotify(TaskNotification{
						Kind:          NotifyBlocked,
						WorkspacePath: ref.Path,
						WorkspaceID:   ref.ID,
						TaskID:        t.ID,
						Number:        t.Number,
						Title:         t.Title,
						Summary:       t.Summary,
					})
				}
			case TaskStatusBlocked:
				if notReady {
					t.Status = TaskStatusNotReady
					t.UpdatedAt = now
					modified = true
				} else if !forceBlocked && depsAllCompleted(t) {
					t.Status = TaskStatusPending
					t.UpdatedAt = now
					modified = true
				}
			case TaskStatusNotReady:
				// Authoring gap filled (criteria + sourcing) → return to pending;
				// the dependency/ready scans below take over from there.
				if !notReady {
					t.Status = TaskStatusPending
					t.UpdatedAt = now
					modified = true
				}
			}
		}

		// 3.6 Closes-link auto-close: when a task completes, any task it links to
		//      with rel=="closes" is itself closed (GitHub-style "fixes #N").
		//      "relates" links are pure cross-references and never auto-act.
		//      Idempotent: an already-closed target is skipped.
		for i := range cfg.Tasks {
			src := &cfg.Tasks[i]
			if src.Status != TaskStatusCompleted {
				continue
			}
			for _, link := range src.Links {
				if link.Rel != LinkCloses {
					continue
				}
				tgt, ok := taskMap[link.Target]
				if !ok || tgt.IssueState == IssueClosed {
					continue
				}
				tgt.Status = TaskStatusCompleted
				tgt.IssueState = IssueClosed
				tgt.CompletedAt = &now
				tgt.UpdatedAt = now
				tgt.Replies = append(tgt.Replies, Reply{
					Author:    Author{Kind: "scheduler", Name: "scheduler"},
					Text:      fmt.Sprintf("由 #%d 修复并关闭", src.Number),
					Mode:      ModePureComment,
					CreatedAt: now,
				})
				modified = true
				log.Printf("[scheduler] Task %s closed by #%d (closes link)", tgt.ID, src.Number)
			}
		}

		// 4. Collect ready tasks: trigger time arrived, dependencies met,
		//    subtasks (implicit dependencies) all completed, issue open. A task
		//    in pending_review (executor finished, awaiting verification) is also
		//    runnable, but routes to the verifier instead of the executor; it has
		//    already passed trigger/dependency gating, so it skips that re-check.
		var ready []readyTask
		for i := range cfg.Tasks {
			t := &cfg.Tasks[i]
			isReview := t.Status == TaskStatusPendingReview
			if !isReview && t.Status != TaskStatusPending && t.Status != TaskStatusQueued {
				continue
			}
			if t.Type == TaskTypeDiscussion {
				continue // 讨论是概念记录，永不调度执行
			}
			if t.Type == TaskTypeRequirement || t.Type == TaskTypeBug {
				continue // 需求/缺陷是 open/close 的议题，不可直接执行；由 PM 拆成任务后再排期
			}
			if t.Source == TaskSourceAgent {
				continue // AI 建议未被采纳前不进入调度
			}
			// Human tasks (assignee=user / executor=human) are NOT skipped here:
			// they flow through the trigger/dependency gates so a dated reminder
			// only becomes actionable at its time and a decision gate blocks its
			// dependents. Dispatch (step 5) parks them at awaiting_human instead of
			// running an agent — one lane for "a person does it" (#192 收敛).
			if t.IssueState == IssueClosed {
				continue
			}
			if !isReview && DeriveSignals(*t).ForceBlocked {
				continue // `blocked` label is an explicit manual hold (#134)
			}
			if !isReview {
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
			}
			ready = append(ready, readyTask{task: t, isReview: isReview})
		}

		// 5. Highest priority first; FIFO by creation within a rank.
		sort.SliceStable(ready, func(i, j int) bool {
			ri, rj := PriorityRank(ready[i].task.Priority), PriorityRank(ready[j].task.Priority)
			if ri != rj {
				return ri < rj
			}
			return ready[i].task.CreatedAt.Before(ready[j].task.CreatedAt)
		})

		// Human tasks are decision points, not agent work: park every ready human
		// task at awaiting_human (no lock, no dispatch — it waits for the person,
		// and its dependents stay blocked until it is completed via
		// complete_human_task / the board), then dispatch the highest-priority
		// remaining agent/function task. Parking is lock-free, so a queue of human
		// todos never monopolises the runner slot.
		var runnable []readyTask
		for _, rt := range ready {
			if !rt.isReview && isHumanTask(rt.task) {
				if rt.task.Status != TaskStatusAwaitingHuman {
					rt.task.Status = TaskStatusAwaitingHuman
					if rt.task.StartedAt == nil {
						rt.task.StartedAt = &now
					}
					rt.task.UpdatedAt = now
					modified = true
					log.Printf("[scheduler] Human task %s (%s) → awaiting_human", rt.task.ID, rt.task.Title)
				}
				continue
			}
			runnable = append(runnable, rt)
		}
		if len(runnable) > 0 {
			rt := runnable[0]
			if s.Lock.TryAcquire(ref.Path, rt.task.ID) {
				task := rt.task
				task.Status = TaskStatusRunning
				if !rt.isReview {
					task.StartedAt = &now // preserve the original start across review cycles
				}
				task.UpdatedAt = now
				modified = true
				verb := "starting"
				if rt.isReview {
					verb = "verifying"
				}
				log.Printf("[scheduler] Lock acquired. Task %s (%s, priority %s) %s in %s",
					task.ID, task.Title, task.Priority, verb, ref.Path)

				// Copy the task before Save below mutates the slice.
				run := *task
				switch {
				case rt.isReview:
					if s.runner != nil {
						go s.runner.Verify(ref.Path, ref.ID, run)
					} else {
						s.Lock.Release(ref.Path)
					}
				case run.Executor == TaskExecutorFunction:
					// Dispatch to the function runner (executor=function, token≈0).
					if s.FunctionRunner != nil {
						go func() {
							defer func() {
								s.Lock.Release(ref.Path)
								s.Tick()
							}()
							s.FunctionRunner(run, ref.Path)
						}()
					} else {
						s.Lock.Release(ref.Path)
					}
				default:
					// executor=agent (default) or empty — existing behaviour.
					if s.runner != nil {
						go s.runner.Execute(ref.Path, ref.ID, run)
					} else {
						s.Lock.Release(ref.Path)
					}
				}
			}
		}

		return modified
	})
}

// emit runs the orchestration engine for one lifecycle event against the live
// task pointer and applies the resulting actions in place. It must be called
// inside a Mutate transaction (t points into cfg.Tasks). Returns whether the
// task was modified. Follow-on events the actions produce (e.g. an assign
// emits EventAssigned) are fanned back through the engine once, non-recursively,
// so a rule can react to a routing decision without risking a loop.
func (s *Scheduler) emit(t *Task, kind TaskEventKind, now time.Time) bool {
	if s.engine == nil {
		return false
	}
	ev := TaskEvent{
		Kind:          kind,
		Task:          *t,
		Signals:       DeriveSignals(*t),
		WorkspacePath: "",
		At:            now,
	}
	actions := s.engine.Evaluate(ev)
	modified, followUps := applyEventActions(t, actions, now)
	for _, fk := range followUps {
		follow := TaskEvent{Kind: fk, Task: *t, Signals: DeriveSignals(*t), At: now}
		more, _ := applyEventActions(t, s.engine.Evaluate(follow), now)
		if more {
			modified = true
		}
	}
	return modified
}

// parseRecurrenceUntil parses a recurrence Until bound, accepting either an
// RFC3339 timestamp or a bare date ("2006-01-02"). Returns ok=false when unset
// or unparseable (treated as "no end").
func parseRecurrenceUntil(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		// A date-only bound means "through the end of that day".
		return t.Add(24*time.Hour - time.Second), true
	}
	return time.Time{}, false
}

// nextOccurrence computes the next trigger after `after` for a simple-enum
// recurrence rule. At ("HH:MM", local time) defaults to midnight.
func nextOccurrence(after time.Time, r *Recurrence) time.Time {
	// interval: fixed spacing from the previous run, no clock alignment. This is
	// the machine cadence (e.g. data-source incremental sync every N minutes).
	if r.Freq == "interval" {
		every := r.EveryMinutes
		if every < 1 {
			every = 1 // guard: a 0/negative interval would busy-loop the scheduler
		}
		return after.Add(time.Duration(every) * time.Minute).UTC()
	}

	hour, minute := 0, 0
	if len(r.At) == 5 {
		if t, err := time.Parse("15:04", r.At); err == nil {
			hour, minute = t.Hour(), t.Minute()
		}
	}
	local := after.Local()
	candidate := time.Date(local.Year(), local.Month(), local.Day(), hour, minute, 0, 0, local.Location())

	step := r.Interval
	if step < 1 {
		step = 1
	}

	switch r.Freq {
	case "weekly":
		days := weekdaySet(r)
		// Advance day-by-day to the next allowed weekday strictly after `local`.
		for !candidate.After(local) || !days[int(candidate.Weekday())] {
			candidate = candidate.AddDate(0, 0, 1)
		}
		// Interval>1 counts in whole weeks from `local`'s week: skip candidates
		// that fall in a non-multiple week.
		if step > 1 {
			base := startOfWeek(local)
			for weeksBetween(base, candidate)%step != 0 {
				candidate = candidate.AddDate(0, 0, 1)
				for !days[int(candidate.Weekday())] {
					candidate = candidate.AddDate(0, 0, 1)
				}
			}
		}
	case "monthly":
		// Relative month ("first Monday") when WeekIndex + a weekday is set;
		// otherwise the legacy absolute Monthday path.
		if r.WeekIndex != 0 && len(weekdayList(r)) > 0 {
			wd := weekdayList(r)[0]
			candidate = nthWeekdayOfMonth(local, r.WeekIndex, wd, hour, minute)
			if !candidate.After(local) {
				candidate = nthWeekdayOfMonth(local.AddDate(0, step, 0), r.WeekIndex, wd, hour, minute)
			}
		} else {
			day := r.Monthday
			if day < 1 {
				day = 1
			}
			candidate = monthlyAt(local, day, hour, minute)
			if !candidate.After(local) {
				candidate = monthlyAt(local.AddDate(0, step, 0), day, hour, minute)
			}
		}
	case "yearly":
		month := time.Month(r.Month)
		if month < time.January || month > time.December {
			month = local.Month()
		}
		yearAnchor := time.Date(local.Year(), month, 1, hour, minute, 0, 0, local.Location())
		candidate = yearlyOccurrence(yearAnchor, r, hour, minute)
		if !candidate.After(local) {
			candidate = yearlyOccurrence(yearAnchor.AddDate(step, 0, 0), r, hour, minute)
		}
	default: // daily
		if !candidate.After(local) {
			candidate = candidate.AddDate(0, 0, step)
		}
	}
	return candidate.UTC()
}

// yearlyOccurrence resolves the day within `anchor`'s month for a yearly rule:
// relative (WeekIndex + weekday) when set, else the absolute Monthday.
func yearlyOccurrence(anchor time.Time, r *Recurrence, hour, minute int) time.Time {
	if r.WeekIndex != 0 && len(weekdayList(r)) > 0 {
		return nthWeekdayOfMonth(anchor, r.WeekIndex, weekdayList(r)[0], hour, minute)
	}
	day := r.Monthday
	if day < 1 {
		day = 1
	}
	return monthlyAt(anchor, day, hour, minute)
}

// weekdaySet returns the allowed weekdays for a weekly rule as a set. Falls back
// to the legacy single Weekday when DaysOfWeek is empty.
func weekdaySet(r *Recurrence) map[int]bool {
	set := map[int]bool{}
	for _, d := range weekdayList(r) {
		set[d] = true
	}
	return set
}

func weekdayList(r *Recurrence) []int {
	if len(r.DaysOfWeek) > 0 {
		return r.DaysOfWeek
	}
	return []int{r.Weekday}
}

func startOfWeek(t time.Time) time.Time {
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	return d.AddDate(0, 0, -int(d.Weekday()))
}

func weeksBetween(a, b time.Time) int {
	return int(startOfWeek(b).Sub(startOfWeek(a)).Hours()) / (24 * 7)
}

// nthWeekdayOfMonth returns the index-th (1..4, or -1 for last) weekday `wd` in
// ref's month at hour:minute. A too-large positive index clamps to the last.
func nthWeekdayOfMonth(ref time.Time, index, wd, hour, minute int) time.Time {
	first := time.Date(ref.Year(), ref.Month(), 1, hour, minute, 0, 0, ref.Location())
	// All matching weekdays in the month.
	var matches []time.Time
	for d := first; d.Month() == first.Month(); d = d.AddDate(0, 0, 1) {
		if int(d.Weekday()) == wd {
			matches = append(matches, d)
		}
	}
	if len(matches) == 0 {
		return first
	}
	if index < 0 {
		return matches[len(matches)-1]
	}
	i := index - 1
	if i >= len(matches) {
		i = len(matches) - 1
	}
	return matches[i]
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
