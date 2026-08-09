package taskapi_test

import (
	"testing"

	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/taskapi"
)

// TestQueryByCase covers the #322 North-API seams: Task and TaskRun are
// queryable by WorkCase through the kernel store's link table.
func TestQueryByCase(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	store := meta.NewTaskStore(db)
	api := taskapi.New(store)
	ws := t.TempDir()

	// Two tasks, one linked to the case.
	linkedID, err := api.DispatchTask("test", taskapi.DispatchSpec{Title: "linked", WorkspacePath: ws})
	if err != nil {
		t.Fatalf("DispatchTask linked: %v", err)
	}
	if _, err := api.DispatchTask("test", taskapi.DispatchSpec{Title: "unlinked", WorkspacePath: ws}); err != nil {
		t.Fatalf("DispatchTask unlinked: %v", err)
	}

	cases := meta.NewWorkCaseStore(db)
	projectID, err := store.ProjectIDForPath(ws)
	if err != nil || projectID == "" {
		t.Fatalf("ProjectIDForPath: id=%q err=%v", projectID, err)
	}
	workCase, err := cases.Create(projectID, meta.WorkCase{Title: "case seam"}, meta.ProjectEvent{
		ProjectID: projectID, ActorKind: "user", ActorName: "user", Origin: "http",
		EventType: "work_case.create", TargetType: "work_case", TargetID: "case-seam",
		Operation: "create", Status: meta.ProjectEventSucceeded,
	})
	if err != nil {
		t.Fatalf("WorkCase create: %v", err)
	}
	if _, err := cases.Link(projectID, workCase.ID, meta.CaseLinkTask, linkedID, workCase.Version, meta.ProjectEvent{
		ProjectID: projectID, ActorKind: "user", ActorName: "user", Origin: "http",
		EventType: "work_case.link", TargetType: "work_case", TargetID: workCase.ID,
		Operation: "link", Status: meta.ProjectEventSucceeded,
	}); err != nil {
		t.Fatalf("Link task: %v", err)
	}

	// Task by Case: only the linked task surfaces.
	tasks, err := api.QueryTasksByCase(workCase.ID)
	if err != nil || len(tasks) != 1 || tasks[0].ID != linkedID {
		t.Fatalf("QueryTasksByCase: n=%d err=%v tasks=%+v", len(tasks), err, tasks)
	}

	// TaskRun by Case joins through the task association.
	run, err := meta.NewTaskRunStore(db).Create(ws, meta.TaskRun{TaskID: linkedID, Kind: meta.TaskRunExecution})
	if err != nil {
		t.Fatalf("TaskRun create: %v", err)
	}
	runs, err := api.QueryTaskRunsByCase(workCase.ID)
	if err != nil || len(runs) != 1 || runs[0].ID != run.ID {
		t.Fatalf("QueryTaskRunsByCase: n=%d err=%v runs=%+v", len(runs), err, runs)
	}

	// Unknown case → empty results, no error.
	if tasks, err := api.QueryTasksByCase("ghost"); err != nil || len(tasks) != 0 {
		t.Fatalf("QueryTasksByCase ghost: n=%d err=%v", len(tasks), err)
	}
	if runs, err := api.QueryTaskRunsByCase("ghost"); err != nil || len(runs) != 0 {
		t.Fatalf("QueryTaskRunsByCase ghost: n=%d err=%v", len(runs), err)
	}
}
