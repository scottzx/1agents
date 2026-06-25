package agent

import (
	"testing"
	"time"

	"github.com/scottzx/1Agents/backend/internal/workspace"
)

// dashboardRig wires a task store + session store sharing one ONEAGENTS_HOME so
// buildDashboard can be exercised without on-disk workspace config.
func dashboardRig(t *testing.T) (*TasksStore, *Store) {
	t.Helper()
	t.Setenv("ONEAGENTS_HOME", t.TempDir())
	tasks, err := NewTasksStore()
	if err != nil {
		t.Fatalf("NewTasksStore: %v", err)
	}
	sessions, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return tasks, sessions
}

func TestBuildDashboardHealthAndSalience(t *testing.T) {
	tasks, sessions := dashboardRig(t)
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)

	running := workspace.Workspace{ID: "ws-run", Name: "Running Proj", Path: t.TempDir()}
	blocked := workspace.Workspace{ID: "ws-blk", Name: "Blocked Proj", Path: t.TempDir()}
	stalled := workspace.Workspace{ID: "ws-stl", Name: "Stalled Proj", Path: t.TempDir()}
	done := workspace.Workspace{ID: "ws-done", Name: "Done Proj", Path: t.TempDir()}

	// Running: one running task + a fresh session heartbeat.
	saveTasks(t, tasks, running.Path, []Task{
		{ID: "r1", Title: "a", Status: TaskStatusRunning, CreatedAt: now, UpdatedAt: now},
		{ID: "r2", Title: "b", Status: TaskStatusCompleted, CreatedAt: now, UpdatedAt: now},
	})
	addSession(t, sessions, running.ID, now.Add(-1*time.Minute)) // on-station

	// Blocked: a blocked task dominates.
	saveTasks(t, tasks, blocked.Path, []Task{
		{ID: "b1", Title: "a", Status: TaskStatusBlocked, CreatedAt: now, UpdatedAt: now},
	})

	// Stalled: running task but stale session activity.
	saveTasks(t, tasks, stalled.Path, []Task{
		{ID: "s1", Title: "a", Status: TaskStatusRunning, CreatedAt: now, UpdatedAt: now},
	})
	addSession(t, sessions, stalled.ID, now.Add(-2*time.Hour)) // stale

	// Done: all tasks completed.
	saveTasks(t, tasks, done.Path, []Task{
		{ID: "d1", Title: "a", Status: TaskStatusCompleted, CreatedAt: now, UpdatedAt: now},
		{ID: "d2", Title: "b", Status: TaskStatusCompleted, CreatedAt: now, UpdatedAt: now},
	})

	// Pass in a deliberately non-salient order to prove sorting.
	resp := buildDashboard([]workspace.Workspace{done, running, stalled, blocked}, tasks, sessions, now)

	if got := len(resp.Projects); got != 4 {
		t.Fatalf("projects = %d, want 4", got)
	}

	// Salience order: blocked, stalled, running, done.
	wantOrder := []ProjectHealth{HealthBlocked, HealthStalled, HealthRunning, HealthDone}
	for i, want := range wantOrder {
		if got := resp.Projects[i].Health; got != want {
			t.Errorf("projects[%d].Health = %s, want %s", i, got, want)
		}
	}

	// Summary rollup.
	if resp.Summary.TotalProjects != 4 {
		t.Errorf("TotalProjects = %d, want 4", resp.Summary.TotalProjects)
	}
	if resp.Summary.BlockedProjects != 2 { // blocked + stalled
		t.Errorf("BlockedProjects = %d, want 2", resp.Summary.BlockedProjects)
	}
	if resp.Summary.RunningProjects != 1 {
		t.Errorf("RunningProjects = %d, want 1", resp.Summary.RunningProjects)
	}
	if resp.Summary.DoneProjects != 1 {
		t.Errorf("DoneProjects = %d, want 1", resp.Summary.DoneProjects)
	}
	if resp.Summary.ActiveAgents != 1 { // only the running project's fresh session
		t.Errorf("ActiveAgents = %d, want 1", resp.Summary.ActiveAgents)
	}
	if resp.Summary.DeliveredTasks != 3 { // 1 (running) + 2 (done)
		t.Errorf("DeliveredTasks = %d, want 3", resp.Summary.DeliveredTasks)
	}

	// Progress percent on the running project: 1/2 completed.
	for _, p := range resp.Projects {
		if p.ID == running.ID && p.ProgressPercent != 50 {
			t.Errorf("running ProgressPercent = %d, want 50", p.ProgressPercent)
		}
	}
}

func TestBuildDashboardExcludesNonExecutableAndSuggestions(t *testing.T) {
	tasks, sessions := dashboardRig(t)
	now := time.Now().UTC()
	ws := workspace.Workspace{ID: "ws-1", Name: "P", Path: t.TempDir()}

	saveTasks(t, tasks, ws.Path, []Task{
		{ID: "t1", Title: "real", Status: TaskStatusCompleted, CreatedAt: now, UpdatedAt: now},
		{ID: "t2", Title: "req", Type: TaskTypeRequirement, Status: TaskStatusPending, CreatedAt: now, UpdatedAt: now},
		{ID: "t3", Title: "bug", Type: TaskTypeBug, Status: TaskStatusBlocked, CreatedAt: now, UpdatedAt: now},
		{ID: "t4", Title: "sugg", Source: TaskSourceAgent, Type: TaskTypeBug, Status: TaskStatusBlocked, CreatedAt: now, UpdatedAt: now},
	})

	resp := buildDashboard([]workspace.Workspace{ws}, tasks, sessions, now)
	if len(resp.Projects) != 1 {
		t.Fatalf("projects = %d, want 1", len(resp.Projects))
	}
	p := resp.Projects[0]
	if p.TotalTasks != 1 {
		t.Errorf("TotalTasks = %d, want 1 (only the executable task counts)", p.TotalTasks)
	}
	// Only the one executable task, completed ⇒ done, not blocked (the blocked
	// bug is non-executable and must be ignored).
	if p.Health != HealthDone {
		t.Errorf("Health = %s, want done", p.Health)
	}
}

func addSession(t *testing.T, s *Store, wsID string, lastEvent time.Time) {
	t.Helper()
	rec := ChatSessionRecord{
		ID:          newID(),
		WorkspaceID: wsID,
		AgentType:   AgentTypeClaudecode,
		LastEventAt: lastEvent,
	}
	if err := s.Add(rec); err != nil {
		t.Fatalf("session Add: %v", err)
	}
}
