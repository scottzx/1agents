package meta

import (
	"fmt"
	"path/filepath"
	"sync"
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
			Task{ID: "req", Title: "parent req", Type: ItemTypeRequirement, Milestone: "M1",
				Status: TaskStatusPending, IssueState: IssueClosed, CreatedAt: now, UpdatedAt: now},
			Task{ID: "bug", Title: "a bug", Type: ItemTypeBug, Milestone: "M1",
				Status: TaskStatusPending, IssueState: IssueOpen, CreatedAt: now, UpdatedAt: now},
			Task{ID: "t1", Title: "done task", Type: ItemTypeTask, Milestone: "M1",
				Status: TaskStatusCompleted, IssueState: IssueOpen, CreatedAt: now, UpdatedAt: now},
			Task{ID: "t2", Title: "open task", Type: ItemTypeTask, Milestone: "M1",
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
			Task{ID: "done", Title: "done", Type: ItemTypeTask, Milestone: "M1",
				Status: TaskStatusCompleted, CreatedAt: now, UpdatedAt: now},
			Task{ID: "cx", Title: "cancelled", Type: ItemTypeTask, Milestone: "M1",
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

func TestCreateVersionMilestoneInitialBumps(t *testing.T) {
	tests := []struct {
		bump MilestoneBump
		want string
	}{
		{MilestoneBumpMinor, "0.1.0"},
		{MilestoneBumpPatch, "0.0.1"},
		{MilestoneBumpMajor, "1.0.0"},
	}
	for _, tt := range tests {
		t.Run(string(tt.bump), func(t *testing.T) {
			s := NewTaskStore(newTestDB(t))
			got, err := s.CreateVersionMilestone(t.TempDir(), tt.bump, "", nil)
			if err != nil {
				t.Fatalf("CreateVersionMilestone(%s): %v", tt.bump, err)
			}
			if got.Version != tt.want || got.Name != tt.want {
				t.Fatalf("version/name = %q/%q, want %q/%q", got.Version, got.Name, tt.want, tt.want)
			}
			if got.IsLegacy {
				t.Fatalf("new version milestone marked legacy: %+v", got)
			}
			if got.PredecessorID != "" {
				t.Fatalf("first version predecessor = %q, want empty", got.PredecessorID)
			}
		})
	}
}

func TestCreateVersionMilestoneUsesNumericSemVerMaximumAndChains(t *testing.T) {
	s := NewTaskStore(newTestDB(t))
	ws := t.TempDir()

	var previous Milestone
	for minor := 1; minor <= 10; minor++ {
		got, err := s.CreateVersionMilestone(ws, MilestoneBumpMinor, "", nil)
		if err != nil {
			t.Fatalf("create minor %d: %v", minor, err)
		}
		want := fmt.Sprintf("0.%d.0", minor)
		if got.Version != want {
			t.Fatalf("minor %d version = %q, want %q", minor, got.Version, want)
		}
		if minor == 1 {
			if got.PredecessorID != "" {
				t.Fatalf("first predecessor = %q, want empty", got.PredecessorID)
			}
		} else if got.PredecessorID != previous.ID {
			t.Fatalf("%s predecessor = %q, want %q (%s)", got.Version, got.PredecessorID, previous.ID, previous.Version)
		}
		previous = got
	}

	patch, err := s.CreateVersionMilestone(ws, MilestoneBumpPatch, "", nil)
	if err != nil {
		t.Fatalf("create patch after 0.10.0: %v", err)
	}
	if patch.Version != "0.10.1" || patch.PredecessorID != previous.ID {
		t.Fatalf("patch after 0.10.0 = %+v", patch)
	}
	major, err := s.CreateVersionMilestone(ws, MilestoneBumpMajor, "", nil)
	if err != nil {
		t.Fatalf("create major: %v", err)
	}
	if major.Version != "1.0.0" || major.PredecessorID != patch.ID {
		t.Fatalf("major after 0.10.1 = %+v", major)
	}
}

func TestCreateVersionMilestoneIgnoresLegacyAndInvalidVersions(t *testing.T) {
	db := newTestDB(t)
	s := NewTaskStore(db)
	ws := t.TempDir()
	legacy, err := s.CreateMilestone(ws, "release-candidate", "", nil, "")
	if err != nil {
		t.Fatalf("create legacy: %v", err)
	}
	projectID, err := s.ProjectIDForPath(ws)
	if err != nil {
		t.Fatalf("ProjectIDForPath: %v", err)
	}
	now := timeToStr(time.Now().UTC())
	if _, err := db.sql.Exec(`
		INSERT INTO milestones (
			id, project_id, name, version, description, target_date,
			position, predecessor_id, created_at, updated_at
		) VALUES ('invalid-version', ?, 'not-semver', 'not-semver', '', NULL, 1, '', ?, ?)`,
		projectID, now, now); err != nil {
		t.Fatalf("seed invalid version: %v", err)
	}

	got, err := s.CreateVersionMilestone(ws, MilestoneBumpMinor, "", nil)
	if err != nil {
		t.Fatalf("CreateVersionMilestone: %v", err)
	}
	if got.Version != "0.1.0" || got.PredecessorID != "" {
		t.Fatalf("legacy/invalid rows affected allocation: %+v (legacy=%+v)", got, legacy)
	}
}

func TestCreateVersionMilestoneConcurrentAllocationIsUnique(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.db")
	db1, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db1.Close() })
	db2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db2.Close() })

	stores := []*TaskStore{NewTaskStore(db1), NewTaskStore(db2)}
	ws := t.TempDir()
	const count = 12
	results := make(chan Milestone, count)
	errs := make(chan error, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			got, err := stores[i%len(stores)].CreateVersionMilestone(ws, MilestoneBumpPatch, "", nil)
			if err != nil {
				errs <- err
				return
			}
			results <- got
		}(i)
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		t.Errorf("concurrent create: %v", err)
	}

	versions := map[string]bool{}
	for got := range results {
		if versions[got.Version] {
			t.Errorf("duplicate version allocated: %s", got.Version)
		}
		versions[got.Version] = true
	}
	if len(versions) != count {
		t.Fatalf("unique versions = %d, want %d (%v)", len(versions), count, versions)
	}
	for i := 1; i <= count; i++ {
		want := fmt.Sprintf("0.0.%d", i)
		if !versions[want] {
			t.Errorf("missing allocated version %s: %v", want, versions)
		}
	}
}

func TestVersionMilestoneOrdinaryUpdateProtectsVersionAndChain(t *testing.T) {
	s := NewTaskStore(newTestDB(t))
	ws := t.TempDir()
	first, err := s.CreateVersionMilestone(ws, MilestoneBumpMinor, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.CreateVersionMilestone(ws, MilestoneBumpMinor, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	changedVersion := "9.9.9"
	if _, err := s.UpdateMilestone(ws, second.ID, MilestonePatch{Version: &changedVersion}); err != ErrMilestoneVersionImmutable {
		t.Fatalf("version patch err = %v, want ErrMilestoneVersionImmutable", err)
	}
	changedName := "renamed"
	if _, err := s.UpdateMilestone(ws, second.ID, MilestonePatch{Name: &changedName}); err != ErrMilestoneVersionImmutable {
		t.Fatalf("name patch err = %v, want ErrMilestoneVersionImmutable", err)
	}
	noPredecessor := ""
	if _, err := s.UpdateMilestone(ws, second.ID, MilestonePatch{PredecessorID: &noPredecessor}); err != ErrMilestoneChainImmutable {
		t.Fatalf("predecessor patch err = %v, want ErrMilestoneChainImmutable", err)
	}

	description := "editable"
	updated, err := s.UpdateMilestone(ws, second.ID, MilestonePatch{Description: &description})
	if err != nil {
		t.Fatalf("description patch: %v", err)
	}
	if updated.Description != description || updated.Version != second.Version ||
		updated.Name != second.Name || updated.PredecessorID != first.ID {
		t.Fatalf("allowed patch changed protected fields: %+v", updated)
	}
}

func TestLegacyMilestoneRemainsReadableEditableAndTaskAssociated(t *testing.T) {
	s := NewTaskStore(newTestDB(t))
	ws := t.TempDir()
	legacy, err := s.CreateMilestone(ws, "历史阶段", "legacy", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	saveTaskWithMilestone(t, s, ws, "legacy-task", "保留关联", legacy.Name, TaskStatusPending)

	renamed := "历史阶段（修订）"
	root := ""
	updated, err := s.UpdateMilestone(ws, legacy.ID, MilestonePatch{Name: &renamed, PredecessorID: &root})
	if err != nil {
		t.Fatalf("update legacy: %v", err)
	}
	if updated.Version != "" || !updated.IsLegacy || updated.Name != renamed {
		t.Fatalf("legacy metadata changed semantics: %+v", updated)
	}
	task, ok, err := s.GetTask("legacy-task")
	if err != nil || !ok {
		t.Fatalf("GetTask: ok=%v err=%v", ok, err)
	}
	if task.Milestone != renamed {
		t.Fatalf("legacy task association = %q, want %q", task.Milestone, renamed)
	}
	list, err := s.ListMilestones(ws)
	if err != nil || len(list) != 1 || list[0].Total != 1 || !list[0].IsLegacy {
		t.Fatalf("legacy list/count = %+v err=%v", list, err)
	}
}
