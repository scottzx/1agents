package taskapi_test

import (
	"os"
	"testing"
	"time"

	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/taskapi"
)

// openTestDB opens a fresh meta.DB in a temp file for the test.
func openTestDB(t *testing.T) (*meta.DB, func()) {
	t.Helper()
	f, err := os.CreateTemp("", "taskapi-test-*.db")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	f.Close()
	db, err := meta.Open(f.Name())
	if err != nil {
		os.Remove(f.Name())
		t.Fatalf("open db: %v", err)
	}
	return db, func() {
		db.Close()
		os.Remove(f.Name())
	}
}

// ── #318: executor field round-trip ─────────────────────────────────────────

func TestExecutorRoundTrip(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	store := meta.NewTaskStore(db)
	api := taskapi.New(store)

	ws := t.TempDir()

	cases := []struct {
		name     string
		executor meta.TaskExecutor
		fnType   string
	}{
		{"agent", meta.TaskExecutorAgent, ""},
		{"function", meta.TaskExecutorFunction, "core.noop"},
		{"human", meta.TaskExecutorHuman, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id, err := api.DispatchTask("test", taskapi.DispatchSpec{
				Title:         "executor roundtrip " + tc.name,
				Executor:      tc.executor,
				FunctionType:  tc.fnType,
				WorkspacePath: ws,
			})
			if err != nil {
				t.Fatalf("DispatchTask: %v", err)
			}
			task, ok, err := api.QueryTask(id)
			if err != nil || !ok {
				t.Fatalf("QueryTask: err=%v ok=%v", err, ok)
			}
			if task.Executor != tc.executor {
				t.Errorf("executor: got %q, want %q", task.Executor, tc.executor)
			}
			if tc.fnType != "" {
				found := false
				for _, l := range task.Labels {
					if l == "fn:"+tc.fnType {
						found = true
					}
				}
				if !found {
					t.Errorf("expected label fn:%s in %v", tc.fnType, task.Labels)
				}
			}
		})
	}
}

// ── #323: function registry + runner executes core.noop ─────────────────────

func TestFunctionRegistryAndRunner(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	store := meta.NewTaskStore(db)
	api := taskapi.New(store)
	ws := t.TempDir()

	id, err := api.DispatchTask("test", taskapi.DispatchSpec{
		Title:         "noop function task",
		Executor:      meta.TaskExecutorFunction,
		FunctionType:  "core.noop",
		WorkspacePath: ws,
	})
	if err != nil {
		t.Fatalf("DispatchTask: %v", err)
	}

	task, ok, err := api.QueryTask(id)
	if err != nil || !ok {
		t.Fatalf("QueryTask pre-run: %v %v", err, ok)
	}

	// Run the function task synchronously.
	taskapi.RunFunction(task, ws, store, api)

	// Verify terminal state.
	task, ok, err = api.QueryTask(id)
	if err != nil || !ok {
		t.Fatalf("QueryTask post-run: %v %v", err, ok)
	}
	if task.Status != meta.TaskStatusCompleted {
		t.Errorf("status: got %q, want completed", task.Status)
	}
	if task.CostTokens != 0 {
		t.Errorf("cost_tokens: got %d, want 0", task.CostTokens)
	}
	if task.Result == "" {
		t.Error("result should not be empty after function run")
	}
}

func TestFunctionRunnerUnknownType(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	store := meta.NewTaskStore(db)
	api := taskapi.New(store)
	ws := t.TempDir()

	id, err := api.DispatchTask("test", taskapi.DispatchSpec{
		Title:         "bad function type",
		Executor:      meta.TaskExecutorFunction,
		FunctionType:  "no.such.handler",
		WorkspacePath: ws,
	})
	if err != nil {
		t.Fatalf("DispatchTask: %v", err)
	}

	task, _, _ := api.QueryTask(id)
	taskapi.RunFunction(task, ws, store, api)

	task, _, _ = api.QueryTask(id)
	if task.Status != meta.TaskStatusFailed {
		t.Errorf("status: got %q, want failed", task.Status)
	}
}

// ── #324: human task completion unlocks downstream ───────────────────────────

func TestHumanTaskCompletionUnlocksDownstream(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	store := meta.NewTaskStore(db)
	api := taskapi.New(store)
	ws := t.TempDir()

	// Create a human task.
	humanID, err := api.DispatchTask("test", taskapi.DispatchSpec{
		Title:         "human decision gate",
		Executor:      meta.TaskExecutorHuman,
		WorkspacePath: ws,
	})
	if err != nil {
		t.Fatalf("DispatchTask human: %v", err)
	}

	// Create a downstream agent task that depends on the human task.
	agentID, err := api.DispatchTask("test", taskapi.DispatchSpec{
		Title:         "downstream agent task",
		Executor:      meta.TaskExecutorAgent,
		DependsOn:     []string{humanID},
		WorkspacePath: ws,
	})
	if err != nil {
		t.Fatalf("DispatchTask agent: %v", err)
	}

	// Downstream must be blocked while human is pending.
	agentTask, _, _ := api.QueryTask(agentID)
	if agentTask.DependsOn[0] != humanID {
		t.Errorf("dependsOn not set: %v", agentTask.DependsOn)
	}

	// Complete the human task (simulate user action via store).
	now := time.Now().UTC()
	_ = store.Mutate(ws, func(cfg *meta.TasksConfig) bool {
		for i := range cfg.Tasks {
			if cfg.Tasks[i].ID == humanID {
				cfg.Tasks[i].Status = meta.TaskStatusCompleted
				cfg.Tasks[i].CompletedAt = &now
				return true
			}
		}
		return false
	})

	// The human task must now be completed.
	human, _, _ := api.QueryTask(humanID)
	if human.Status != meta.TaskStatusCompleted {
		t.Errorf("human status: got %q, want completed", human.Status)
	}

	// depsAllCompleted checks for TaskStatusCompleted, so the downstream task
	// is now eligible. Verify via the store query (scheduler logic is tested
	// in the agent package's scheduler_test.go).
	cfg, _ := store.Load(ws)
	depsComplete := true
	for _, dep := range agentTask.DependsOn {
		for _, t2 := range cfg.Tasks {
			if t2.ID == dep && t2.Status != meta.TaskStatusCompleted {
				depsComplete = false
			}
		}
	}
	if !depsComplete {
		t.Error("downstream task's deps should be complete after human task completion")
	}
}

// ── #319: executor-agnostic ready gate with mixed executors ─────────────────

func TestMixedExecutorDependencyDAG(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	store := meta.NewTaskStore(db)
	api := taskapi.New(store)
	ws := t.TempDir()

	// Create: human → function → agent (chain)
	humanID, _ := api.DispatchTask("test", taskapi.DispatchSpec{
		Title:         "step 1: human",
		Executor:      meta.TaskExecutorHuman,
		WorkspacePath: ws,
	})
	fnID, _ := api.DispatchTask("test", taskapi.DispatchSpec{
		Title:         "step 2: function",
		Executor:      meta.TaskExecutorFunction,
		FunctionType:  "core.noop",
		DependsOn:     []string{humanID},
		WorkspacePath: ws,
	})
	agentID, _ := api.DispatchTask("test", taskapi.DispatchSpec{
		Title:         "step 3: agent",
		Executor:      meta.TaskExecutorAgent,
		DependsOn:     []string{fnID},
		WorkspacePath: ws,
	})

	cfg, _ := store.Load(ws)
	byID := make(map[string]meta.Task)
	for _, t2 := range cfg.Tasks {
		byID[t2.ID] = t2
	}

	// All three tasks must exist with correct executor.
	if byID[humanID].Executor != meta.TaskExecutorHuman {
		t.Errorf("human executor: %q", byID[humanID].Executor)
	}
	if byID[fnID].Executor != meta.TaskExecutorFunction {
		t.Errorf("fn executor: %q", byID[fnID].Executor)
	}
	if byID[agentID].Executor != meta.TaskExecutorAgent {
		t.Errorf("agent executor: %q", byID[agentID].Executor)
	}
	// Dependency chain must be wired.
	if len(byID[fnID].DependsOn) == 0 || byID[fnID].DependsOn[0] != humanID {
		t.Errorf("fn.DependsOn: %v", byID[fnID].DependsOn)
	}
	if len(byID[agentID].DependsOn) == 0 || byID[agentID].DependsOn[0] != fnID {
		t.Errorf("agent.DependsOn: %v", byID[agentID].DependsOn)
	}
}

// ── #321: binding seam helpers ───────────────────────────────────────────────

func TestBindingSeam(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	store := meta.NewTaskStore(db)
	api := taskapi.New(store)
	ws := t.TempDir()

	const ref = "crm:lead:42"

	ids, err := api.IssueTasksFromBusiness("crm", ref, "enrich", []taskapi.DispatchSpec{
		{Title: "enrich lead 42", Executor: meta.TaskExecutorAgent, WorkspacePath: ws},
		{Title: "score lead 42", Executor: meta.TaskExecutorFunction, FunctionType: "core.noop", WorkspacePath: ws},
	})
	if err != nil {
		t.Fatalf("IssueTasksFromBusiness: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 ids, got %d", len(ids))
	}

	tasks, err := api.ListTasksForBusiness(ref)
	if err != nil {
		t.Fatalf("ListTasksForBusiness: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks for ref %q, got %d", ref, len(tasks))
	}
	for _, tsk := range tasks {
		if tsk.BusinessRef != ref {
			t.Errorf("task %s business_ref: %q, want %q", tsk.ID, tsk.BusinessRef, ref)
		}
		if tsk.Milestone != "enrich" {
			t.Errorf("task %s milestone: %q, want enrich", tsk.ID, tsk.Milestone)
		}
	}
}

// ── completion hook ───────────────────────────────────────────────────────────

func TestCompletionHook(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	store := meta.NewTaskStore(db)
	api := taskapi.New(store)
	ws := t.TempDir()

	var fired []taskapi.CompletionEvent
	api.RegisterCompletionHook(func(ev taskapi.CompletionEvent) {
		fired = append(fired, ev)
	})

	id, _ := api.DispatchTask("test", taskapi.DispatchSpec{
		Title:         "hook test",
		Executor:      meta.TaskExecutorFunction,
		FunctionType:  "core.noop",
		WorkspacePath: ws,
	})

	task, _, _ := api.QueryTask(id)
	taskapi.RunFunction(task, ws, store, api)

	if len(fired) != 1 {
		t.Fatalf("expected 1 hook call, got %d", len(fired))
	}
	if fired[0].Status != meta.TaskStatusCompleted {
		t.Errorf("hook status: %q", fired[0].Status)
	}
	if fired[0].TaskID != id {
		t.Errorf("hook taskID: %q, want %q", fired[0].TaskID, id)
	}
}
