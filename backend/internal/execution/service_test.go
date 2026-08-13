package execution

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

func TestServiceCreatesAndUpdatesFunctionJobAndTrigger(t *testing.T) {
	db, err := meta.Open(filepath.Join(t.TempDir(), "meta.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	workspace := t.TempDir()
	store := meta.NewTaskStore(db)
	if err := store.Mutate(workspace, func(cfg *meta.TasksConfig) bool {
		cfg.Tasks = append(cfg.Tasks, meta.Task{ID: "task-1", Title: "Run", Type: meta.ItemTypeTask, IssueState: meta.IssueOpen})
		return true
	}); err != nil {
		t.Fatal(err)
	}
	var id string
	if err := db.SQL().QueryRow(`SELECT id FROM projects WHERE workspace_path=?`, workspace).Scan(&id); err != nil {
		t.Fatal(err)
	}
	repo, err := NewRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(repo, nil)
	job, err := service.CreateJob(CreateJobInput{ProjectID: id, WorkItemID: "task-1", ExecutorKind: "function", FunctionType: "core.noop"})
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if job.Revision != 1 || job.Status != JobStatusActive {
		t.Fatalf("job = %#v", job)
	}
	cwd := "/tmp/work"
	updated, err := service.UpdateJob(job.ID, UpdateJobInput{Cwd: &cwd})
	if err != nil {
		t.Fatalf("UpdateJob: %v", err)
	}
	if updated.Revision != 2 || updated.Cwd != cwd {
		t.Fatalf("updated = %#v", updated)
	}
	trigger, err := service.UpsertTrigger(job.ID, TriggerSpec{Kind: TriggerAt, Spec: []byte(`{"at":"2026-08-12T00:00:00Z"}`)})
	if err != nil {
		t.Fatalf("UpsertTrigger: %v", err)
	}
	if trigger.Status != TriggerArmed || trigger.MisfirePolicy != "skip" {
		t.Fatalf("trigger = %#v", trigger)
	}
}

func TestSchedulerDispatchesDueAtTriggerOnce(t *testing.T) {
	db, err := meta.Open(filepath.Join(t.TempDir(), "meta.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	workspace := t.TempDir()
	store := meta.NewTaskStore(db)
	if err := store.Mutate(workspace, func(cfg *meta.TasksConfig) bool {
		cfg.Tasks = append(cfg.Tasks, meta.Task{ID: "task-1", Title: "Run", Type: meta.ItemTypeTask, IssueState: meta.IssueOpen})
		return true
	}); err != nil {
		t.Fatal(err)
	}
	var projectID string
	if err := db.SQL().QueryRow(`SELECT id FROM projects WHERE workspace_path=?`, workspace).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	repo, err := NewRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(repo, nil)
	job, err := service.CreateJob(CreateJobInput{ProjectID: projectID, WorkItemID: "task-1", ExecutorKind: "function", FunctionType: "core.noop"})
	if err != nil {
		t.Fatal(err)
	}
	called := 0
	service.SetDispatcher(func(context.Context, Job) error { called++; return nil })
	past := time.Now().UTC().Add(-time.Minute)
	if _, err := service.UpsertTrigger(job.ID, TriggerSpec{Kind: TriggerAt, Spec: []byte(`{"at":"2026-08-12T00:00:00Z"}`), NextRunAt: &past}); err != nil {
		t.Fatal(err)
	}
	NewScheduler(service).Tick(context.Background())
	if called != 1 {
		t.Fatalf("dispatches = %d, want 1", called)
	}
	trigger, err := repo.TriggerByJob(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if trigger.Status != TriggerExhausted {
		t.Fatalf("trigger status = %q", trigger.Status)
	}
}

func TestServiceListsProjectJobsWithTriggers(t *testing.T) {
	db, err := meta.Open(filepath.Join(t.TempDir(), "meta.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	workspace := t.TempDir()
	store := meta.NewTaskStore(db)
	if err := store.Mutate(workspace, func(cfg *meta.TasksConfig) bool {
		cfg.Tasks = append(cfg.Tasks, meta.Task{ID: "task-1", Title: "Run", Type: meta.ItemTypeTask, IssueState: meta.IssueOpen})
		return true
	}); err != nil {
		t.Fatal(err)
	}
	var projectID string
	if err := db.SQL().QueryRow(`SELECT id FROM projects WHERE workspace_path=?`, workspace).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	repo, err := NewRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(repo, nil)
	job, err := service.CreateJob(CreateJobInput{ProjectID: projectID, WorkItemID: "task-1", ExecutorKind: "function", FunctionType: "core.noop"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.UpsertTrigger(job.ID, TriggerSpec{Kind: TriggerRecurrence, Spec: []byte(`{"everyMinutes":15}`)}); err != nil {
		t.Fatal(err)
	}
	details, err := service.ListJobs(projectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(details) != 1 || details[0].ID != job.ID || details[0].Trigger == nil || details[0].Trigger.Kind != TriggerRecurrence {
		t.Fatalf("details = %#v", details)
	}
}

func TestServiceAcceptsAgentPreambleAndRejectsInvalid(t *testing.T) {
	service, projectID := newFunctionService(t)
	service.SetFunctionLookup(func(name string) bool { return name == "core.script" })

	if _, err := service.CreateJob(CreateJobInput{
		ProjectID: projectID, WorkItemID: "task-1", ExecutorKind: "function",
		FunctionType: "core.noop", PreambleFunctionType: "core.script",
	}); err == nil {
		t.Fatal("function job with preamble should fail")
	}

	if _, err := service.CreateJob(CreateJobInput{
		ProjectID: projectID, WorkItemID: "task-1", ExecutorKind: "agent",
		LegacyAgentType: "codex", PreambleFunctionType: "missing.fn",
	}); err == nil {
		t.Fatal("unregistered preamble should fail")
	}

	job, err := service.CreateJob(CreateJobInput{
		ProjectID: projectID, WorkItemID: "task-1", ExecutorKind: "agent",
		LegacyAgentType: "codex", PreambleFunctionType: "core.script",
		BusinessRef: "automation:task-1", Capabilities: []string{"script:digest.py"},
	})
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if job.PreambleFunctionType != "core.script" || job.BusinessRef != "automation:task-1" {
		t.Fatalf("job = %#v", job)
	}
	got, err := service.GetJob(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.PreambleFunctionType != "core.script" {
		t.Fatalf("GetJob preamble = %q", got.PreambleFunctionType)
	}

	empty := ""
	updated, err := service.UpdateJob(job.ID, UpdateJobInput{PreambleFunctionType: &empty})
	if err != nil {
		t.Fatalf("clear preamble: %v", err)
	}
	if updated.PreambleFunctionType != "" || updated.Revision != 2 {
		t.Fatalf("updated = %#v", updated)
	}
}

func TestJobSchemaReconcilesLegacyTableMissingPreambleColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.db")
	db, err := meta.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL().Exec(`
		DROP TABLE IF EXISTS kernel_execution_jobs;
		CREATE TABLE kernel_execution_jobs (
			id TEXT PRIMARY KEY, project_id TEXT NOT NULL, work_item_id TEXT NOT NULL,
			business_ref TEXT NOT NULL DEFAULT '', executor_kind TEXT NOT NULL,
			profile_id TEXT NOT NULL DEFAULT '', profile_source TEXT NOT NULL DEFAULT '',
			legacy_agent_type TEXT NOT NULL DEFAULT '', function_type TEXT NOT NULL DEFAULT '',
			cwd TEXT NOT NULL DEFAULT '', capabilities_json TEXT NOT NULL DEFAULT '[]',
			status TEXT NOT NULL, revision INTEGER NOT NULL, timeout_minutes INTEGER NOT NULL DEFAULT 0,
			max_attempts INTEGER NOT NULL DEFAULT 1, blocked_code TEXT NOT NULL DEFAULT '',
			blocked_reason TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
			UNIQUE(project_id, work_item_id)
		);
	`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = meta.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo, err := NewRepository(db)
	if err != nil {
		t.Fatalf("NewRepository should add missing column: %v", err)
	}
	var count int
	if err := db.SQL().QueryRow(`SELECT COUNT(1) FROM pragma_table_info('kernel_execution_jobs') WHERE name='preamble_function_type'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("preamble column missing after reconcile count=%d err=%v", count, err)
	}

	workspace := t.TempDir()
	if err := meta.NewTaskStore(db).Mutate(workspace, func(cfg *meta.TasksConfig) bool {
		cfg.Tasks = append(cfg.Tasks, meta.Task{ID: "task-1", Title: "Run", Type: meta.ItemTypeTask, IssueState: meta.IssueOpen})
		return true
	}); err != nil {
		t.Fatal(err)
	}
	var projectID string
	if err := db.SQL().QueryRow(`SELECT id FROM projects WHERE workspace_path=?`, workspace).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	service := NewService(repo, nil)
	job, err := service.CreateJob(CreateJobInput{ProjectID: projectID, WorkItemID: "task-1", ExecutorKind: "function", FunctionType: "core.noop"})
	if err != nil {
		t.Fatalf("CreateJob on reconciled table: %v", err)
	}
	if job.PreambleFunctionType != "" {
		t.Fatalf("preamble = %q", job.PreambleFunctionType)
	}
}

func newFunctionService(t *testing.T) (*Service, string) {
	t.Helper()
	db, err := meta.Open(filepath.Join(t.TempDir(), "meta.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	workspace := t.TempDir()
	if err := meta.NewTaskStore(db).Mutate(workspace, func(cfg *meta.TasksConfig) bool {
		cfg.Tasks = append(cfg.Tasks, meta.Task{ID: "task-1", Title: "Run", Type: meta.ItemTypeTask, IssueState: meta.IssueOpen})
		return true
	}); err != nil {
		t.Fatal(err)
	}
	var projectID string
	if err := db.SQL().QueryRow(`SELECT id FROM projects WHERE workspace_path=?`, workspace).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	repo, err := NewRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	return NewService(repo, nil), projectID
}

func TestHandlerListsJobsForExecutionOverview(t *testing.T) {
	db, err := meta.Open(filepath.Join(t.TempDir(), "meta.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	workspace := t.TempDir()
	store := meta.NewTaskStore(db)
	if err := store.Mutate(workspace, func(cfg *meta.TasksConfig) bool {
		cfg.Tasks = append(cfg.Tasks, meta.Task{ID: "task-1", Title: "Run", Type: meta.ItemTypeTask, IssueState: meta.IssueOpen})
		return true
	}); err != nil {
		t.Fatal(err)
	}
	var projectID string
	if err := db.SQL().QueryRow(`SELECT id FROM projects WHERE workspace_path=?`, workspace).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	repo, err := NewRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(repo, nil)
	job, err := service.CreateJob(CreateJobInput{ProjectID: projectID, WorkItemID: "task-1", ExecutorKind: "function", FunctionType: "core.noop"})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(service)
	req := httptest.NewRequest(http.MethodGet, "/api/execution-jobs", nil)
	res := httptest.NewRecorder()
	handler.Root(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if got := res.Body.String(); !strings.Contains(got, job.ID) || !strings.Contains(got, "items") {
		t.Fatalf("response=%s", got)
	}
}
