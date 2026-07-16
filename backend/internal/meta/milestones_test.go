package meta

import (
	"testing"
	"time"
)

// saveTask is a tiny helper to persist a single task into a workspace.
func saveTaskWithMilestone(t *testing.T, s *TaskStore, ws, id, title, milestone string, status TaskStatus) {
	t.Helper()
	err := s.Mutate(ws, func(cfg *TasksConfig) bool {
		cfg.Tasks = append(cfg.Tasks, Task{
			ID: id, Title: title, Milestone: milestone, Status: status,
			CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		})
		return true
	})
	if err != nil {
		t.Fatalf("save task %s: %v", id, err)
	}
}

func TestMilestoneCRUDAndCounts(t *testing.T) {
	s := NewTaskStore(newTestDB(t))
	ws := t.TempDir()

	m1, err := s.CreateMilestone(ws, "M1", "first stage", nil, "")
	if err != nil {
		t.Fatalf("create M1: %v", err)
	}
	if m1.Position != 0 {
		t.Fatalf("M1 position = %d, want 0", m1.Position)
	}
	due := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	m2, err := s.CreateMilestone(ws, "M2", "", &due, "")
	if err != nil {
		t.Fatalf("create M2: %v", err)
	}
	if m2.Position != 1 {
		t.Fatalf("M2 position = %d, want 1", m2.Position)
	}

	// Duplicate name is rejected.
	if _, err := s.CreateMilestone(ws, "M1", "", nil, ""); err != ErrMilestoneExists {
		t.Fatalf("duplicate create err = %v, want ErrMilestoneExists", err)
	}

	// Assign tasks and verify counts.
	saveTaskWithMilestone(t, s, ws, "t1", "A", "M1", TaskStatusCompleted)
	saveTaskWithMilestone(t, s, ws, "t2", "B", "M1", TaskStatusPending)
	saveTaskWithMilestone(t, s, ws, "t3", "C", "M2", TaskStatusCompleted)

	list, err := s.ListMilestones(ws)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list len = %d, want 2", len(list))
	}
	if list[0].Name != "M1" || list[0].Total != 2 || list[0].Completed != 1 {
		t.Fatalf("M1 counts wrong: %+v", list[0])
	}
	if list[1].Name != "M2" || list[1].Total != 1 || list[1].Completed != 1 {
		t.Fatalf("M2 counts wrong: %+v", list[1])
	}
	if list[1].TargetDate == nil || !list[1].TargetDate.Equal(due) {
		t.Fatalf("M2 target date wrong: %+v", list[1].TargetDate)
	}
}

func TestMilestoneRenameCascadesToTasks(t *testing.T) {
	s := NewTaskStore(newTestDB(t))
	ws := t.TempDir()

	m1, err := s.CreateMilestone(ws, "Old", "", nil, "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	saveTaskWithMilestone(t, s, ws, "t1", "A", "Old", TaskStatusPending)

	newName := "New"
	if _, err := s.UpdateMilestone(ws, m1.ID, MilestonePatch{Name: &newName}); err != nil {
		t.Fatalf("rename: %v", err)
	}

	// The task's denormalized label follows the rename.
	task, ok, err := s.GetTask("t1")
	if err != nil || !ok {
		t.Fatalf("get task: %v ok=%v", err, ok)
	}
	if task.Milestone != "New" {
		t.Fatalf("task milestone = %q, want New", task.Milestone)
	}
	// And the count still resolves under the new name.
	list, _ := s.ListMilestones(ws)
	if len(list) != 1 || list[0].Name != "New" || list[0].Total != 1 {
		t.Fatalf("post-rename list wrong: %+v", list)
	}
}

func TestMilestoneDeleteUnassignsTasks(t *testing.T) {
	s := NewTaskStore(newTestDB(t))
	ws := t.TempDir()
	m1, _ := s.CreateMilestone(ws, "M1", "", nil, "")
	saveTaskWithMilestone(t, s, ws, "t1", "A", "M1", TaskStatusPending)

	if err := s.DeleteMilestone(ws, m1.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if list, _ := s.ListMilestones(ws); len(list) != 0 {
		t.Fatalf("list after delete = %d, want 0", len(list))
	}
	task, _, _ := s.GetTask("t1")
	if task.Milestone != "" {
		t.Fatalf("task milestone = %q, want empty after delete", task.Milestone)
	}
}

func TestMilestoneReorder(t *testing.T) {
	s := NewTaskStore(newTestDB(t))
	ws := t.TempDir()
	a, _ := s.CreateMilestone(ws, "A", "", nil, "")
	b, _ := s.CreateMilestone(ws, "B", "", nil, "")
	c, _ := s.CreateMilestone(ws, "C", "", nil, "")

	if err := s.ReorderMilestones(ws, []string{c.ID, a.ID, b.ID}); err != nil {
		t.Fatalf("reorder: %v", err)
	}
	list, _ := s.ListMilestones(ws)
	got := []string{list[0].Name, list[1].Name, list[2].Name}
	if got[0] != "C" || got[1] != "A" || got[2] != "B" {
		t.Fatalf("order = %v, want [C A B]", got)
	}
}

func TestMilestonePredecessorAndDeleteReparents(t *testing.T) {
	s := NewTaskStore(newTestDB(t))
	ws := t.TempDir()
	// root → mid → leaf chain via predecessor.
	root, _ := s.CreateMilestone(ws, "root", "", nil, "")
	mid, _ := s.CreateMilestone(ws, "mid", "", nil, root.ID)
	leaf, _ := s.CreateMilestone(ws, "leaf", "", nil, mid.ID)

	list, _ := s.ListMilestones(ws)
	byName := map[string]Milestone{}
	for _, m := range list {
		byName[m.Name] = m
	}
	if byName["mid"].PredecessorID != root.ID || byName["leaf"].PredecessorID != mid.ID {
		t.Fatalf("predecessors not persisted: %+v", list)
	}

	// Deleting the middle node reparents its child (leaf) onto root.
	if err := s.DeleteMilestone(ws, mid.ID); err != nil {
		t.Fatalf("delete mid: %v", err)
	}
	list, _ = s.ListMilestones(ws)
	for _, m := range list {
		if m.Name == "leaf" && m.PredecessorID != root.ID {
			t.Fatalf("leaf not reparented to root after deleting mid: predecessor=%q", m.PredecessorID)
		}
	}

	// A milestone can't be its own predecessor.
	self := leaf.ID
	updated, err := s.UpdateMilestone(ws, leaf.ID, MilestonePatch{PredecessorID: &self})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.PredecessorID == leaf.ID {
		t.Fatalf("self-predecessor should be rejected, got %q", updated.PredecessorID)
	}
}

func TestMilestoneOrphanSelfHeal(t *testing.T) {
	s := NewTaskStore(newTestDB(t))
	ws := t.TempDir()
	// Task saved with a milestone label but no metadata row (mimics the legacy
	// import / seed path that bypasses the HTTP EnsureMilestone hook).
	saveTaskWithMilestone(t, s, ws, "t1", "A", "Orphan", TaskStatusPending)

	list, err := s.ListMilestones(ws)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].Name != "Orphan" || list[0].Total != 1 {
		t.Fatalf("orphan not healed: %+v", list)
	}
}

func TestMilestoneCountsExcludeIssueItems(t *testing.T) {
	s := NewTaskStore(newTestDB(t))
	ws := t.TempDir()
	if _, err := s.CreateMilestone(ws, "M1", "", nil, ""); err != nil {
		t.Fatalf("create: %v", err)
	}
	err := s.Mutate(ws, func(cfg *TasksConfig) bool {
		now := time.Now().UTC()
		cfg.Tasks = append(cfg.Tasks,
			Task{ID: "req", Title: "parent req", Type: TaskTypeRequirement, Milestone: "M1",
				Status: TaskStatusPending, IssueState: IssueClosed, CreatedAt: now, UpdatedAt: now},
			Task{ID: "bug", Title: "a bug", Type: TaskTypeBug, Milestone: "M1",
				Status: TaskStatusPending, IssueState: IssueOpen, CreatedAt: now, UpdatedAt: now},
			Task{ID: "t1", Title: "done task", Type: TaskTypeTask, Milestone: "M1",
				Status: TaskStatusCompleted, IssueState: IssueOpen, CreatedAt: now, UpdatedAt: now},
			Task{ID: "t2", Title: "open task", Type: TaskTypeTask, Milestone: "M1",
				Status: TaskStatusPending, IssueState: IssueOpen, CreatedAt: now, UpdatedAt: now},
		)
		return true
	})
	if err != nil {
		t.Fatalf("mutate: %v", err)
	}
	list, err := s.ListMilestones(ws)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list len = %d, want 1", len(list))
	}
	if list[0].Total != 2 || list[0].Completed != 1 {
		t.Fatalf("M1 counts = %d/%d, want 1/2", list[0].Completed, list[0].Total)
	}
}

func TestMilestoneCountsExcludeCancelled(t *testing.T) {
	s := NewTaskStore(newTestDB(t))
	ws := t.TempDir()
	if _, err := s.CreateMilestone(ws, "M1", "", nil, ""); err != nil {
		t.Fatalf("create: %v", err)
	}
	err := s.Mutate(ws, func(cfg *TasksConfig) bool {
		now := time.Now().UTC()
		cfg.Tasks = append(cfg.Tasks,
			Task{ID: "done", Title: "done", Type: TaskTypeTask, Milestone: "M1",
				Status: TaskStatusCompleted, CreatedAt: now, UpdatedAt: now},
			Task{ID: "cx", Title: "cancelled", Type: TaskTypeTask, Milestone: "M1",
				Status: TaskStatusCancelled, CreatedAt: now, UpdatedAt: now},
		)
		return true
	})
	if err != nil {
		t.Fatalf("mutate: %v", err)
	}
	list, err := s.ListMilestones(ws)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if list[0].Total != 1 || list[0].Completed != 1 {
		t.Fatalf("M1 counts = %d/%d, want 1/1", list[0].Completed, list[0].Total)
	}
}
