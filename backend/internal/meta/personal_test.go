package meta

import (
	"errors"
	"strings"
	"testing"
)

func newTestPersonalStore(t *testing.T) (*PersonalStore, *TaskStore) {
	t.Helper()
	db := newTestDB(t)
	ts := NewTaskStore(db)
	return NewPersonalStore(ts), ts
}

func TestPersonalCaptureAndList(t *testing.T) {
	s, _ := newTestPersonalStore(t)

	if _, err := s.Capture("  ", "", ""); err == nil {
		t.Fatal("blank title should be rejected")
	}

	a, err := s.Capture("买牛奶", "顺手记一下", "")
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if a.ID == "" || a.Number != 1 {
		t.Fatalf("expected id + #1, got id=%q number=%d", a.ID, a.Number)
	}
	if a.Type != TaskTypeTask || a.Status != TaskStatusPending || a.IssueState != IssueOpen {
		t.Fatalf("unexpected defaults: %+v", a)
	}

	b, err := s.Capture("读论文", "", "inbox-42")
	if err != nil {
		t.Fatalf("capture 2: %v", err)
	}
	if b.Number != 2 {
		t.Fatalf("second personal task should be #2, got #%d", b.Number)
	}
	// fromInbox is recorded as a backlink label.
	foundBacklink := false
	for _, l := range b.Labels {
		if l == capturedFromLabel+"inbox-42" {
			foundBacklink = true
		}
	}
	if !foundBacklink {
		t.Fatalf("expected captured-from backlink label, got %v", b.Labels)
	}

	list, err := s.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("personal list = %d, want 2", len(list))
	}
}

// TestPersonalTasksHaveNoProject asserts the bucket is invisible to the regular
// project-keyed task path — a personal task does not appear under any real
// workspace, satisfying #67's "no project_id, 不强制归口" rule.
func TestPersonalTasksHaveNoProject(t *testing.T) {
	s, ts := newTestPersonalStore(t)
	if _, err := s.Capture("私人事项", "", ""); err != nil {
		t.Fatalf("capture: %v", err)
	}
	// A real workspace sees none of them.
	cfg, err := ts.Load(t.TempDir())
	if err != nil {
		t.Fatalf("load real workspace: %v", err)
	}
	if len(cfg.Tasks) != 0 {
		t.Fatalf("real workspace should have no tasks, got %d", len(cfg.Tasks))
	}
}

func TestIncubatePromotesToProject(t *testing.T) {
	s, ts := newTestPersonalStore(t)
	personal, err := s.Capture("做个调度器", "够分量，要长期推", "inbox-7")
	if err != nil {
		t.Fatalf("capture: %v", err)
	}

	wsPath := t.TempDir()
	res, err := s.Incubate(personal.ID, "调度器项目", wsPath, []string{"MVP", "Beta", "MVP"})
	if err != nil {
		t.Fatalf("incubate: %v", err)
	}

	if res.Project.Name != "调度器项目" || res.Project.WorkspacePath != wsPath {
		t.Fatalf("project not created right: %+v", res.Project)
	}
	if res.Project.Status != ProjectStatusActive {
		t.Fatalf("incubated project should be active, got %q", res.Project.Status)
	}

	// The task now lives in the new project and is #1 there.
	if res.Task.Number != 1 {
		t.Fatalf("promoted task should be #1 in new project, got #%d", res.Task.Number)
	}
	// incubated-from backlink stamped; original captured-from preserved.
	var hasIncubated, hasCaptured bool
	for _, l := range res.Task.Labels {
		if l == incubatedFromLabel+personal.ID {
			hasIncubated = true
		}
		if strings.HasPrefix(l, capturedFromLabel) {
			hasCaptured = true
		}
	}
	if !hasIncubated {
		t.Fatalf("expected incubated-from label, got %v", res.Task.Labels)
	}
	if !hasCaptured {
		t.Fatalf("captured-from label should survive promotion, got %v", res.Task.Labels)
	}

	// It is gone from the personal bucket.
	list, _ := s.List()
	if len(list) != 0 {
		t.Fatalf("promoted task should leave the personal bucket, got %d", len(list))
	}

	// It is now visible under the real workspace.
	cfg, err := ts.Load(wsPath)
	if err != nil {
		t.Fatalf("load project: %v", err)
	}
	if len(cfg.Tasks) != 1 || cfg.Tasks[0].ID != personal.ID {
		t.Fatalf("task not re-homed into project: %+v", cfg.Tasks)
	}

	// Roadmap milestones were seeded, deduped (MVP appears once).
	ms, err := ts.ListMilestones(wsPath)
	if err != nil {
		t.Fatalf("list milestones: %v", err)
	}
	if len(ms) != 2 {
		t.Fatalf("expected 2 milestones (deduped), got %d: %+v", len(ms), ms)
	}
}

func TestIncubateValidation(t *testing.T) {
	s, ts := newTestPersonalStore(t)

	// Unknown task → not found.
	if _, err := s.Incubate("nope", "P", t.TempDir(), nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown task: got %v, want ErrNotFound", err)
	}

	personal, _ := s.Capture("候选", "", "")

	// Missing name / path → error.
	if _, err := s.Incubate(personal.ID, "", t.TempDir(), nil); err == nil {
		t.Fatal("missing project name should error")
	}
	if _, err := s.Incubate(personal.ID, "P", "", nil); err == nil {
		t.Fatal("missing workspace path should error")
	}

	// Path already taken by an existing project → error (no silent merge).
	taken := t.TempDir()
	if err := ts.db.EnsureProject("existing", "Existing", taken); err != nil {
		t.Fatalf("seed existing project: %v", err)
	}
	if _, err := s.Incubate(personal.ID, "P", taken, nil); err == nil {
		t.Fatal("incubating into an existing project path should error")
	}

	// A task that is not personal (lives in a real project) cannot be incubated.
	wsPath := t.TempDir()
	res, err := s.Incubate(personal.ID, "Promoted", wsPath, nil)
	if err != nil {
		t.Fatalf("first incubate: %v", err)
	}
	if _, err := s.Incubate(res.Task.ID, "Again", t.TempDir(), nil); err == nil {
		t.Fatal("re-incubating a non-personal task should error")
	}
}
