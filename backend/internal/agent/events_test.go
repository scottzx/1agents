package agent

import (
	"strings"
	"testing"
	"time"
)

// --- Pure rule-table tests (no scheduler, no store) ----------------------

func TestRouteOnCreatedAssignsDefault(t *testing.T) {
	eng := DefaultEventEngine()
	ev := TaskEvent{Kind: EventCreated, Task: Task{ID: "t1", Type: TaskTypeTask}}
	actions := eng.Evaluate(ev)
	if len(actions) != 1 || actions[0].Kind != ActionAssign {
		t.Fatalf("want one assign action, got %+v", actions)
	}
	if actions[0].AgentType != DefaultAgentType {
		t.Fatalf("default route = %s, want %s", actions[0].AgentType, DefaultAgentType)
	}
}

func TestRouteOnCreatedRespectsExistingAssignee(t *testing.T) {
	eng := DefaultEventEngine()
	ev := TaskEvent{Kind: EventCreated, Task: Task{ID: "t1", Assignee: AgentTypeCodex}}
	if actions := eng.Evaluate(ev); len(actions) != 0 {
		t.Fatalf("already-assigned task should not be routed, got %+v", actions)
	}
}

func TestRouteByDomainLabelAndType(t *testing.T) {
	eng := DefaultEventEngine()
	cases := []struct {
		name   string
		task   Task
		expect AgentType
	}{
		{"frontend label", Task{Labels: []string{"frontend"}}, AgentTypeClaudecode},
		{"research label", Task{Labels: []string{"调研"}}, AgentTypeGemini},
		{"bug type", Task{Type: TaskTypeBug}, AgentTypeCodex},
		{"plain task", Task{Type: TaskTypeTask}, DefaultAgentType},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actions := eng.Evaluate(TaskEvent{Kind: EventCreated, Task: c.task})
			if len(actions) != 1 || actions[0].AgentType != c.expect {
				t.Fatalf("route = %+v, want assign %s", actions, c.expect)
			}
		})
	}
}

func TestNotifyPMOnBlocked(t *testing.T) {
	eng := DefaultEventEngine()
	actions := eng.Evaluate(TaskEvent{Kind: EventBlocked, Task: Task{ID: "t1"}})
	if len(actions) != 1 || actions[0].Kind != ActionNotify || actions[0].Role != SessionRolePM {
		t.Fatalf("want notify PM, got %+v", actions)
	}
}

func TestRequeueOnVerifyFailedCarriesContext(t *testing.T) {
	eng := DefaultEventEngine()
	task := Task{ID: "t1", Review: &ReviewVerdict{Summary: "缺少单测"}}
	actions := eng.Evaluate(TaskEvent{Kind: EventVerifyFailed, Task: task})
	if len(actions) != 1 || actions[0].Kind != ActionRequeue {
		t.Fatalf("want requeue, got %+v", actions)
	}
	if !strings.Contains(actions[0].Note, "缺少单测") {
		t.Fatalf("requeue note must carry failure context, got %q", actions[0].Note)
	}
}

func TestApplyAssignEmitsFollowUp(t *testing.T) {
	now := time.Now().UTC()
	task := &Task{ID: "t1"}
	mod, follow := applyEventActions(task, []EventAction{{Kind: ActionAssign, AgentType: AgentTypeCodex}}, now)
	if !mod || task.Assignee != AgentTypeCodex {
		t.Fatalf("assign not applied: mod=%v assignee=%s", mod, task.Assignee)
	}
	if len(follow) != 1 || follow[0] != EventAssigned {
		t.Fatalf("assign should emit EventAssigned follow-up, got %+v", follow)
	}
}

// --- Chain tests through the scheduler (created→route, verify-failed→requeue) ---

func TestChainCreatedAutoRoutes(t *testing.T) {
	s, ref, store := newTestScheduler(t)
	now := time.Now().UTC()
	// Fresh executable task, no assignee, with criteria so it isn't held
	// not_ready. A "调研" domain label routes to a non-default agent (gemini)
	// so the routing decision is observable (a plain task would route to the
	// default, indistinguishable from "no routing happened").
	saveTasks(t, store, ref.Path, []Task{
		srcReq(now),
		withSource(Task{ID: "t1", Title: "调研竞品", Description: "查一下", AcceptanceCriteria: "通过",
			Labels: []string{"调研"}, Type: TaskTypeTask,
			Status: TaskStatusPending, CreatedAt: now, UpdatedAt: now}),
	})

	s.Tick()

	got := loadTask(t, store, ref.Path, "t1")
	if got.Assignee != AgentTypeGemini {
		t.Fatalf("created→route: assignee = %q, want %s", got.Assignee, AgentTypeGemini)
	}
	// The task should still proceed to run (routing doesn't block scheduling).
	if got.Status != TaskStatusRunning {
		t.Fatalf("status = %s, want running after route", got.Status)
	}
}

func TestChainCreatedRouteIsIdempotent(t *testing.T) {
	s, ref, store := newTestScheduler(t)
	now := time.Now().UTC()
	// User pre-assigned codex; routing must not overwrite it.
	saveTasks(t, store, ref.Path, []Task{
		{ID: "t1", Title: "x", Description: "y", AcceptanceCriteria: "z", Assignee: AgentTypeGemini,
			Type: TaskTypeTask, Status: TaskStatusPending, CreatedAt: now, UpdatedAt: now},
	})
	s.Tick()
	if got := loadTask(t, store, ref.Path, "t1").Assignee; got != AgentTypeGemini {
		t.Fatalf("pre-assigned task got re-routed to %q", got)
	}
}

func TestChainVerifyFailedRequeuesWithContext(t *testing.T) {
	_, ref, store := newTestScheduler(t)
	now := time.Now().UTC()
	// Task mid-verification (running), configured for review with budget left.
	saveTasks(t, store, ref.Path, []Task{
		{ID: "t1", Title: "活", Description: "做", AcceptanceCriteria: "达标",
			Verifier: AgentTypeClaudecode, ReviewMaxAttempts: 2, ReviewCount: 0,
			Status: TaskStatusRunning, StartedAt: &now, CreatedAt: now, UpdatedAt: now},
	})

	crit := []CriterionResult{{Criterion: "达标", Pass: false, Comment: "差一步"}}
	if _, err := applyReviewVerdict(store, ref.Path, "t1", crit, false, "整体未达标", AgentTypeClaudecode); err != nil {
		t.Fatalf("applyReviewVerdict: %v", err)
	}

	got := loadTask(t, store, ref.Path, "t1")
	if got.Status != TaskStatusPending {
		t.Fatalf("verify-failed→requeue: status = %s, want pending", got.Status)
	}
	if got.StartedAt != nil {
		t.Fatalf("requeue must clear StartedAt, got %v", got.StartedAt)
	}
	// The engine's requeue note carrying the failure context must be on the timeline.
	found := false
	for _, r := range got.Replies {
		if strings.Contains(r.Text, "自动重派") {
			found = true
		}
	}
	if !found {
		t.Fatalf("requeue context note missing from timeline: %+v", got.Replies)
	}
}

func TestChainVerifyFailedExhaustedFails(t *testing.T) {
	_, ref, store := newTestScheduler(t)
	now := time.Now().UTC()
	// Last attempt: cycle reaches the budget → terminal failed, no requeue.
	saveTasks(t, store, ref.Path, []Task{
		{ID: "t1", Title: "活", Description: "做", AcceptanceCriteria: "达标",
			Verifier: AgentTypeClaudecode, ReviewMaxAttempts: 1, ReviewCount: 0,
			Status: TaskStatusRunning, StartedAt: &now, CreatedAt: now, UpdatedAt: now},
	})
	crit := []CriterionResult{{Criterion: "达标", Pass: false}}
	if _, err := applyReviewVerdict(store, ref.Path, "t1", crit, false, "未达标", AgentTypeClaudecode); err != nil {
		t.Fatalf("applyReviewVerdict: %v", err)
	}
	if got := loadTask(t, store, ref.Path, "t1").Status; got != TaskStatusFailed {
		t.Fatalf("exhausted budget: status = %s, want failed", got)
	}
}

func TestChainBlockedNotifiesPM(t *testing.T) {
	s, ref, store := newTestScheduler(t)
	now := time.Now().UTC()
	// t2 depends on incomplete t1 → t2 gets blocked → PM notified once.
	saveTasks(t, store, ref.Path, []Task{
		srcReq(now),
		withSource(Task{ID: "t1", Title: "前置", Description: "做", AcceptanceCriteria: "ok",
			Status: TaskStatusPending, CreatedAt: now, UpdatedAt: now}),
		withSource(Task{ID: "t2", Title: "后置", Description: "做", AcceptanceCriteria: "ok", DependsOn: []string{"t1"},
			Status: TaskStatusPending, CreatedAt: now.Add(time.Second), UpdatedAt: now}),
	})
	s.Tick()
	got := loadTask(t, store, ref.Path, "t2")
	if got.Status != TaskStatusBlocked {
		t.Fatalf("t2 status = %s, want blocked", got.Status)
	}
	notifies := 0
	for _, r := range got.Replies {
		if strings.Contains(r.Text, "@"+SessionRolePM) {
			notifies++
		}
	}
	if notifies != 1 {
		t.Fatalf("PM notify count = %d, want 1 (once on transition)", notifies)
	}

	// A second tick must NOT re-notify (already blocked, no new transition).
	s.Tick()
	got = loadTask(t, store, ref.Path, "t2")
	notifies = 0
	for _, r := range got.Replies {
		if strings.Contains(r.Text, "@"+SessionRolePM) {
			notifies++
		}
	}
	if notifies != 1 {
		t.Fatalf("PM re-notified on second tick: count = %d, want 1", notifies)
	}
}

// loadTask fetches a task by id from a workspace config.
func loadTask(t *testing.T, store *TasksStore, path, id string) Task {
	t.Helper()
	cfg, err := store.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, task := range cfg.Tasks {
		if task.ID == id {
			return task
		}
	}
	t.Fatalf("task %s not found", id)
	return Task{}
}
