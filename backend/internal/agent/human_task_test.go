package agent

import (
	"testing"
	"time"
)

// TestHumanTaskUnifiedLane verifies the merged human lane (4条道→3): a task
// assigned to the user is NOT held as not_ready (no acceptance criteria / no
// sourcing needed), is never dispatched to an agent, and parks at awaiting_human
// — the same state whether declared via assignee=user or legacy executor=human.
func TestHumanTaskUnifiedLane(t *testing.T) {
	s, ref, store := newTestScheduler(t)
	now := time.Now().UTC()
	saveTasks(t, store, ref.Path, []Task{
		// assignee=user, no acceptance criteria, no sourcing link — a personal todo.
		{ID: "reminder", Title: "交报告", Description: "写完交上去", Assignee: AssigneeUser,
			IssueState: IssueOpen, Status: TaskStatusPending, CreatedAt: now, UpdatedAt: now},
		// legacy executor=human declaration — must reach the same state.
		{ID: "gate", Title: "审批", Description: "批一下", Executor: TaskExecutorHuman,
			IssueState: IssueOpen, Status: TaskStatusPending, CreatedAt: now, UpdatedAt: now},
	})
	s.Tick()
	if got := statusOf(t, store, ref.Path, "reminder"); got != TaskStatusAwaitingHuman {
		t.Fatalf("assignee=user status = %s, want awaiting_human (not skipped, not not_ready)", got)
	}
	if got := statusOf(t, store, ref.Path, "gate"); got != TaskStatusAwaitingHuman {
		t.Fatalf("executor=human status = %s, want awaiting_human", got)
	}
}

// TestHumanTaskGatesDependentAndAgentNotStarved verifies a human task blocks its
// dependents until completed, while an independent agent task still runs the same
// tick (parking is lock-free, so human todos don't monopolise the runner slot).
func TestHumanTaskGatesDependentAndAgentNotStarved(t *testing.T) {
	s, ref, store := newTestScheduler(t)
	now := time.Now().UTC()
	saveTasks(t, store, ref.Path, []Task{
		srcReq(now),
		{ID: "human", Title: "人决策", Description: "选个方案", Assignee: AssigneeUser,
			IssueState: IssueOpen, Status: TaskStatusPending, CreatedAt: now, UpdatedAt: now},
		withSource(Task{ID: "downstream", Title: "落地", Description: "按决策做", AcceptanceCriteria: "done",
			DependsOn: []string{"human"}, IssueState: IssueOpen, Status: TaskStatusPending,
			CreatedAt: now, UpdatedAt: now}),
		withSource(Task{ID: "agentwork", Title: "独立活", Description: "干活", AcceptanceCriteria: "done",
			IssueState: IssueOpen, Status: TaskStatusPending, CreatedAt: now, UpdatedAt: now}),
	})
	s.Tick()
	if got := statusOf(t, store, ref.Path, "human"); got != TaskStatusAwaitingHuman {
		t.Fatalf("human status = %s, want awaiting_human", got)
	}
	if got := statusOf(t, store, ref.Path, "downstream"); got != TaskStatusBlocked {
		t.Fatalf("downstream status = %s, want blocked (gated by the human task)", got)
	}
	if got := statusOf(t, store, ref.Path, "agentwork"); got != TaskStatusRunning {
		t.Fatalf("independent agent status = %s, want running (not starved by the human task)", got)
	}

	// Completing the human task releases the dependent on the next tick.
	setStatus(t, store, ref.Path, "human", TaskStatusCompleted)
	s.Tick()
	if got := statusOf(t, store, ref.Path, "downstream"); got != TaskStatusQueued && got != TaskStatusRunning {
		t.Fatalf("downstream after human done = %s, want queued/running", got)
	}
}
