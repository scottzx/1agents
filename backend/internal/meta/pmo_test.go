package meta

import (
	"errors"
	"testing"
)

func newTestPMOStore(t *testing.T) (*PMOStore, *TaskStore, *InboxStore) {
	t.Helper()
	db := newTestDB(t)
	ts := NewTaskStore(db)
	inbox := NewInboxStore(db)
	return NewPMOStore(ts, inbox), ts, inbox
}

func TestDispatchWritesRequirementIntoProject(t *testing.T) {
	pmo, ts, inbox := newTestPMOStore(t)

	wsPath := t.TempDir()
	if err := ts.db.EnsureProject("proj-1", "客户端项目", wsPath); err != nil {
		t.Fatalf("ensure project: %v", err)
	}
	src, err := inbox.Capture(InboxItem{Title: "用户反馈：导出很慢", Source: InboxSourceManual})
	if err != nil {
		t.Fatalf("inbox capture: %v", err)
	}

	res, err := pmo.Dispatch("proj-1", "优化导出性能", "把导出从 30s 降到 5s 以内", "high", src.ID)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if res.Project.ID != "proj-1" {
		t.Fatalf("dispatch target project = %q, want proj-1", res.Project.ID)
	}
	req := res.Requirement
	if req.Type != TaskTypeRequirement {
		t.Fatalf("dispatched card type = %q, want requirement", req.Type)
	}
	if req.Number != 1 {
		t.Fatalf("requirement should be #1 in the new project, got #%d", req.Number)
	}
	if req.Priority != PriorityHigh {
		t.Fatalf("priority = %q, want high", req.Priority)
	}

	// dispatched-from backlink label records the originating inbox item.
	foundBacklink := false
	for _, l := range req.Labels {
		if l == dispatchedFromLabel+src.ID {
			foundBacklink = true
		}
	}
	if !foundBacklink {
		t.Fatalf("expected dispatched-from backlink label, got %v", req.Labels)
	}

	// The requirement actually lives in the target project's pool.
	cfg, err := ts.Load(wsPath)
	if err != nil {
		t.Fatalf("load project tasks: %v", err)
	}
	if len(cfg.Tasks) != 1 || cfg.Tasks[0].ID != req.ID {
		t.Fatalf("requirement not found in project pool: %+v", cfg.Tasks)
	}

	// The originating inbox item left the unread queue (loop closed).
	got, ok, err := inbox.Get(src.ID)
	if err != nil || !ok {
		t.Fatalf("inbox get: ok=%v err=%v", ok, err)
	}
	if got.Status != InboxStatusRead {
		t.Fatalf("dispatched inbox item status = %q, want read", got.Status)
	}
}

func TestDispatchWithoutInboxSource(t *testing.T) {
	pmo, _, _ := newTestPMOStore(t)
	wsPath := t.TempDir()
	if err := pmo.tasks.db.EnsureProject("proj-2", "P2", wsPath); err != nil {
		t.Fatalf("ensure project: %v", err)
	}
	res, err := pmo.Dispatch("proj-2", "直接分发的需求", "", "", "")
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if len(res.Requirement.Labels) != 0 {
		t.Fatalf("expected no backlink label without an inbox source, got %v", res.Requirement.Labels)
	}
}

func TestDispatchValidation(t *testing.T) {
	pmo, ts, _ := newTestPMOStore(t)

	// Blank title rejected.
	if _, err := pmo.Dispatch("proj", "  ", "", "", ""); err == nil {
		t.Fatal("blank title should be rejected")
	}
	// Blank projectId rejected.
	if _, err := pmo.Dispatch("", "T", "", "", ""); err == nil {
		t.Fatal("blank projectId should be rejected")
	}
	// Unknown project → ErrNotFound.
	if _, err := pmo.Dispatch("ghost", "T", "", "", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown project should be ErrNotFound, got %v", err)
	}
	// The reserved personal bucket is not a dispatch target.
	if err := ts.db.EnsureProject(PersonalProjectID, "个人任务", personalProjectPath); err != nil {
		t.Fatalf("ensure personal bucket: %v", err)
	}
	if _, err := pmo.Dispatch(PersonalProjectID, "T", "", "", ""); err == nil {
		t.Fatal("dispatching into the personal bucket should be rejected")
	}
}

func TestTargetsExcludesPersonalBucket(t *testing.T) {
	pmo, ts, _ := newTestPMOStore(t)
	if err := ts.db.EnsureProject("p-active", "活跃项目", t.TempDir()); err != nil {
		t.Fatalf("ensure project: %v", err)
	}
	if err := ts.db.EnsureProject(PersonalProjectID, "个人任务", personalProjectPath); err != nil {
		t.Fatalf("ensure personal bucket: %v", err)
	}
	targets, err := pmo.Targets()
	if err != nil {
		t.Fatalf("targets: %v", err)
	}
	if len(targets) != 1 || targets[0].ProjectID != "p-active" {
		t.Fatalf("targets should list only the active non-personal project, got %+v", targets)
	}
}
