package media

import (
	"testing"

	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/taskapi"
)

// withTempDB points meta.OpenDefault at a fresh temp DB (via ONEAGENTS_HOME) for
// the test and wires a live API + store into the media runtime.
func withTempDB(t *testing.T) {
	t.Helper()
	t.Setenv("ONEAGENTS_HOME", t.TempDir())

	db, err := meta.OpenDefault()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := EnsureTables(); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}
	store := meta.NewTaskStore(db)
	api := taskapi.New(store)
	api.RegisterApp(taskapi.AppPermissions{Namespace: AppID, AllowedRefs: []string{AppID + ":"}})
	setRuntime(api, store)
}

// TestEnsureTablesIdempotent verifies the domain DDLs are safe to run repeatedly.
func TestEnsureTablesIdempotent(t *testing.T) {
	withTempDB(t)
	for i := 0; i < 3; i++ {
		if err := EnsureTables(); err != nil {
			t.Fatalf("ensure tables pass %d: %v", i, err)
		}
	}
	// A round-trip insert proves the schema is usable after repeated ensures.
	cp, err := CreateProject("", t.TempDir(), "幂等性测试")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	got, ok, err := GetProject(cp.ID)
	if err != nil || !ok {
		t.Fatalf("get project: err=%v ok=%v", err, ok)
	}
	if got.Title != "幂等性测试" || got.Status != "topic" {
		t.Errorf("unexpected project: %+v", got)
	}
}

// TestSilenceDetectHandler verifies the registered media.silence_detect handler
// returns concrete segments at token≈0.
func TestSilenceDetectHandler(t *testing.T) {
	registerFunctions() // safe: last-write-wins
	h := taskapi.Lookup(FnSilenceDetect)
	if h == nil {
		t.Fatal("media.silence_detect not registered")
	}
	ctx := taskapi.FunctionContext{Task: meta.Task{Description: "material=m1 duration=20"}}
	res, err := h(ctx)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if ctx.CostTokens != 0 {
		t.Errorf("expected token≈0, got %d", ctx.CostTokens)
	}
	sd, ok := res.(silenceDetectResult)
	if !ok {
		t.Fatalf("unexpected result type %T", res)
	}
	if len(sd.Segments) == 0 {
		t.Fatal("expected non-empty segments")
	}
	if sd.Source != "computed" {
		t.Errorf("expected source=computed, got %q", sd.Source)
	}
	// Deterministic: duration 20 → windows [0,8],[9.5,17.5],[19,20].
	if sd.Segments[0].Start != 0 || sd.Segments[0].End != 8 {
		t.Errorf("unexpected first segment: %+v", sd.Segments[0])
	}
}

// TestComputeSilenceSegmentsDeterministic checks the pure split function.
func TestComputeSilenceSegmentsDeterministic(t *testing.T) {
	got := computeSilenceSegments(20)
	want := []SilenceSegment{{0, 8}, {9.5, 17.5}, {19, 20}}
	if len(got) != len(want) {
		t.Fatalf("len: got %d want %d (%+v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("seg %d: got %+v want %+v", i, got[i], want[i])
		}
	}
	// Zero duration → single canned window.
	if z := computeSilenceSegments(0); len(z) != 1 {
		t.Errorf("zero duration: expected 1 window, got %+v", z)
	}
}

// TestBusinessRefRoundTrip dispatches the pipeline via IssueTasksFromBusiness and
// reads it back via ListTasksForBusiness (forward + reverse binding seams).
func TestBusinessRefRoundTrip(t *testing.T) {
	withTempDB(t)

	ws := t.TempDir()
	cp, err := CreateProject("", ws, "回流测试项目")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	mat, err := AddMaterial(Material{ProjectID: cp.ID, Kind: "video", Duration: 20})
	if err != nil {
		t.Fatalf("add material: %v", err)
	}

	ids, err := LaunchProcessingPipeline(cp.ID, mat.ID)
	if err != nil {
		t.Fatalf("launch pipeline: %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("expected 3 pipeline tasks, got %d", len(ids))
	}

	ref := BusinessRef("material", mat.ID)
	a, _, _ := runtime()
	tasks, err := a.ListTasksForBusiness(ref)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks for ref, got %d", len(tasks))
	}

	// Verify the mixed-executor tri-state (reverse binding seam view).
	byMilestone := map[string]meta.Task{}
	for _, tk := range tasks {
		byMilestone[tk.Milestone] = tk
	}
	if byMilestone["silence_detect"].Executor != meta.TaskExecutorFunction {
		t.Errorf("silence stage executor: got %q", byMilestone["silence_detect"].Executor)
	}
	if byMilestone["edit"].Executor != meta.TaskExecutorAgent {
		t.Errorf("edit stage executor: got %q", byMilestone["edit"].Executor)
	}
	if byMilestone["approve"].Executor != meta.TaskExecutorHuman {
		t.Errorf("approve stage executor: got %q", byMilestone["approve"].Executor)
	}

	// DependsOn lives in task_deps and is hydrated by GetTask (loadTaskChildren),
	// not by the business_ref list scan — assert dependency wiring via QueryTask.
	editTask, _, _ := a.QueryTask(ids[1])
	approveTask, _, _ := a.QueryTask(ids[2])
	if len(editTask.DependsOn) == 0 || editTask.DependsOn[0] != ids[0] {
		t.Errorf("edit stage should depend on silence stage, got %v", editTask.DependsOn)
	}
	if len(approveTask.DependsOn) == 0 || approveTask.DependsOn[0] != ids[1] {
		t.Errorf("approve stage should depend on edit stage, got %v", approveTask.DependsOn)
	}
}

// TestResolveHumanTask completes the human decision gate and asserts the domain
// material stage advances via the writeback hook.
func TestResolveHumanTask(t *testing.T) {
	withTempDB(t)
	a, _, _ := runtime()
	a.RegisterCompletionHook(onTaskCompletion)

	ws := t.TempDir()
	cp, err := CreateProject("", ws, "终审测试")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	mat, err := AddMaterial(Material{ProjectID: cp.ID, Kind: "video", Duration: 12})
	if err != nil {
		t.Fatalf("add material: %v", err)
	}
	ids, err := LaunchProcessingPipeline(cp.ID, mat.ID)
	if err != nil {
		t.Fatalf("launch pipeline: %v", err)
	}
	approveID := ids[2]

	if err := ResolveHumanTask(approveID, "approved", map[string]any{"kept": 2}); err != nil {
		t.Fatalf("resolve human task: %v", err)
	}
	task, ok, err := a.QueryTask(approveID)
	if err != nil || !ok {
		t.Fatalf("query task: err=%v ok=%v", err, ok)
	}
	if task.Status != meta.TaskStatusCompleted {
		t.Errorf("expected completed, got %q", task.Status)
	}
	got, _, _ := GetMaterial(mat.ID)
	if got.Stage != "approved" {
		t.Errorf("expected material stage approved, got %q", got.Stage)
	}
}
