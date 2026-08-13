package taskapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scottzx/1Agents/backend/internal/execution"
	"github.com/scottzx/1Agents/backend/internal/meta"
)

func TestResolveScriptPathRejectsEscape(t *testing.T) {
	cwd := t.TempDir()
	if _, err := resolveScriptPath(cwd, "/tmp/x.py"); err == nil {
		t.Fatal("absolute path should fail")
	}
	if _, err := resolveScriptPath(cwd, "../secret.py"); err == nil {
		t.Fatal("parent escape should fail")
	}
	got, err := resolveScriptPath(cwd, "nested/ok.py")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(cwd, "nested", "ok.py") {
		t.Fatalf("got %q", got)
	}
}

func TestCoreScriptReturnsJSON(t *testing.T) {
	cwd := t.TempDir()
	script := filepath.Join(cwd, "automation.py")
	if err := os.WriteFile(script, []byte("print('{\"ok\": true, \"n\": 3}')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := runCoreScript(FunctionContext{Cwd: cwd, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	asMap, ok := result.(map[string]any)
	if !ok || asMap["ok"] != true {
		t.Fatalf("result = %#v", result)
	}
}

func TestCoreScriptRejectsNonJSON(t *testing.T) {
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "automation.py"), []byte("print('not-json')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runCoreScript(FunctionContext{Cwd: cwd, Timeout: 10 * time.Second}); err == nil {
		t.Fatal("non-json should fail")
	}
}

func TestCoreScriptNonZeroExit(t *testing.T) {
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "automation.py"), []byte("import sys\nsys.exit(2)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runCoreScript(FunctionContext{Cwd: cwd, Timeout: 10 * time.Second}); err == nil {
		t.Fatal("nonzero exit should fail")
	}
}

func TestRunFunctionPreambleDoesNotCompleteItem(t *testing.T) {
	db, err := meta.Open(filepath.Join(t.TempDir(), "meta.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := meta.NewTaskStore(db)
	ws := t.TempDir()
	if err := store.Mutate(ws, func(cfg *meta.TasksConfig) bool {
		cfg.Tasks = append(cfg.Tasks, meta.Task{
			ID: "task-1", Title: "Digest", Type: meta.ItemTypeTask,
			IssueState: meta.IssueOpen, Status: meta.TaskStatusRunning,
			Labels: []string{"fn:core.noop"},
		})
		return true
	}); err != nil {
		t.Fatal(err)
	}

	result, err := RunFunctionPreamble(meta.Task{ID: "task-1", Title: "Digest", Labels: []string{"fn:core.noop"}}, ws, store, execution.Job{
		ID: "job-1", PreambleFunctionType: "core.noop", Revision: 1,
	}, meta.TaskRun{JobID: "job-1", JobRevision: 1, OccurrenceKey: "preamble:test"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, `"status":"ok"`) {
		t.Fatalf("result = %s", result)
	}
	task, ok, err := store.GetTask("task-1")
	if err != nil || !ok {
		t.Fatalf("GetTask: ok=%v err=%v", ok, err)
	}
	if task.Status != meta.TaskStatusRunning {
		t.Fatalf("preamble must not complete item, status=%s", task.Status)
	}
	runs, err := store.TaskRuns().ListByJob("job-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status != meta.TaskRunCompleted {
		t.Fatalf("runs = %#v", runs)
	}
}

func TestRunFunctionPreambleUnknownTypeFails(t *testing.T) {
	_, err := RunFunctionPreamble(meta.Task{ID: "task-1"}, t.TempDir(), nil, execution.Job{
		PreambleFunctionType: "missing.fn",
	}, meta.TaskRun{})
	if err == nil {
		t.Fatal("expected unregistered preamble error")
	}
}
