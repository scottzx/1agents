package radio

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/taskapi"
)

// newTestApp builds an isolated App over a temp DB + temp workspace.
func newTestApp(t *testing.T) (*App, *meta.TaskStore, string, func()) {
	t.Helper()
	f, err := os.CreateTemp("", "radio-test-*.db")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	f.Close()
	db, err := meta.Open(f.Name())
	if err != nil {
		os.Remove(f.Name())
		t.Fatalf("open db: %v", err)
	}
	store := NewStore(db)
	if err := store.EnsureTables(); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}
	taskStore := meta.NewTaskStore(db)
	api := taskapi.New(taskStore)
	a := NewApp(api, store)
	ws := t.TempDir()
	return a, taskStore, ws, func() {
		db.Close()
		os.Remove(f.Name())
	}
}

// ── #344: EnsureTables idempotency ───────────────────────────────────────────

func TestEnsureTablesIdempotent(t *testing.T) {
	a, _, _, cleanup := newTestApp(t)
	defer cleanup()
	// Already ensured once in newTestApp; calling again must be a no-op.
	if err := a.store.EnsureTables(); err != nil {
		t.Fatalf("second EnsureTables: %v", err)
	}
	if err := a.store.EnsureTables(); err != nil {
		t.Fatalf("third EnsureTables: %v", err)
	}
}

// ── #344: episode CRUD + draft status ────────────────────────────────────────

func TestCreateAndGetEpisode(t *testing.T) {
	a, _, ws, cleanup := newTestApp(t)
	defer cleanup()

	ep, err := a.store.CreateEpisode(ws, "测试电台", "https://example.com/feed/1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if ep.ID == 0 {
		t.Fatal("expected non-zero id")
	}
	if ep.Status != StatusDraft {
		t.Errorf("status: got %q want draft", ep.Status)
	}
	got, ok, err := a.store.GetEpisode(ep.ID)
	if err != nil || !ok {
		t.Fatalf("get: err=%v ok=%v", err, ok)
	}
	if got.Title != "测试电台" {
		t.Errorf("title: %q", got.Title)
	}
	list, _ := a.store.ListEpisodes()
	if len(list) != 1 {
		t.Errorf("list len: %d", len(list))
	}
}

// ── #345: 3-stage DependsOn chain dispatch ───────────────────────────────────

func TestStartPipelineChainsThreeStages(t *testing.T) {
	a, _, ws, cleanup := newTestApp(t)
	defer cleanup()

	ep, _ := a.store.CreateEpisode(ws, "管线测试", "https://example.com/x")
	ids, err := a.StartPipeline(ep.ID, ws)
	if err != nil {
		t.Fatalf("StartPipeline: %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("expected 3 task ids, got %d", len(ids))
	}
	summaryID, transcriptID, ttsID := ids[0], ids[1], ids[2]

	// All bound to the same business_ref (reverse binding seam, used by the UI
	// for stage state).
	ref := businessRef(ep.ID)
	refTasks, err := a.api.ListTasksForBusiness(ref)
	if err != nil {
		t.Fatalf("ListTasksForBusiness: %v", err)
	}
	if len(refTasks) != 3 {
		t.Fatalf("expected 3 tasks for ref %q, got %d", ref, len(refTasks))
	}

	// DependsOn is asserted via the full Load path (QueryTasks) — that is the
	// path the executor-agnostic scheduler uses to advance the chain.
	// NOTE: ListTasksForBusiness does NOT hydrate DependsOn (platform gap, see
	// #347 report); use QueryTasks for dependency assertions.
	loaded, err := a.api.QueryTasks(ws, "", "")
	if err != nil {
		t.Fatalf("QueryTasks: %v", err)
	}
	byID := map[string]meta.Task{}
	for _, tk := range loaded {
		byID[tk.ID] = tk
	}

	// Stage executors.
	if byID[summaryID].Executor != meta.TaskExecutorAgent {
		t.Errorf("summary executor: %q", byID[summaryID].Executor)
	}
	if byID[transcriptID].Executor != meta.TaskExecutorAgent {
		t.Errorf("transcript executor: %q", byID[transcriptID].Executor)
	}
	if byID[ttsID].Executor != meta.TaskExecutorFunction {
		t.Errorf("tts executor: %q", byID[ttsID].Executor)
	}

	// DependsOn chain: summary ← transcript ← tts.
	if len(byID[transcriptID].DependsOn) == 0 || byID[transcriptID].DependsOn[0] != summaryID {
		t.Errorf("transcript.DependsOn: %v", byID[transcriptID].DependsOn)
	}
	if len(byID[ttsID].DependsOn) == 0 || byID[ttsID].DependsOn[0] != transcriptID {
		t.Errorf("tts.DependsOn: %v", byID[ttsID].DependsOn)
	}

	// TTS carries the fn: label and milestone.
	foundFn := false
	for _, l := range byID[ttsID].Labels {
		if l == "fn:"+TTSFunctionType {
			foundFn = true
		}
	}
	if !foundFn {
		t.Errorf("tts missing fn label: %v", byID[ttsID].Labels)
	}
	if byID[ttsID].Milestone != StageSynthesize {
		t.Errorf("tts milestone: %q", byID[ttsID].Milestone)
	}

	// Pipeline start advances the episode to summarizing.
	got, _, _ := a.store.GetEpisode(ep.ID)
	if got.Status != StatusSummarizing {
		t.Errorf("episode status after start: %q want summarizing", got.Status)
	}
}

// ── #345: tts_synthesize handler writes artifact, token≈0 ────────────────────

func TestTTSHandlerWritesArtifact(t *testing.T) {
	a, taskStore, ws, cleanup := newTestApp(t)
	defer cleanup()

	ep, _ := a.store.CreateEpisode(ws, "音频测试", "https://example.com/y")
	_ = a.store.SetTranscript(ep.ID, "大家好，这里是 AI 电台测试稿。")

	// Dispatch the function task through the North API so it carries the
	// fn: label + business_ref exactly as the pipeline would.
	id, err := a.api.DispatchTask(AppID, taskapi.DispatchSpec{
		Title:         "tts",
		Executor:      meta.TaskExecutorFunction,
		FunctionType:  TTSFunctionType,
		BusinessRef:   businessRef(ep.ID),
		Milestone:     StageSynthesize,
		WorkspacePath: ws,
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	task, _, _ := a.api.QueryTask(id)

	// Run the function synchronously (as the kernel's function-runner would),
	// firing the completion hook which writes audio_path back.
	taskapi.RunFunction(task, ws, taskStore, a.api)

	// Token cost must be ~0 (deterministic stub).
	done, _, _ := a.api.QueryTask(id)
	if done.Status != meta.TaskStatusCompleted {
		t.Fatalf("tts task status: %q", done.Status)
	}
	if done.CostTokens != 0 {
		t.Errorf("cost_tokens: got %d want 0", done.CostTokens)
	}

	// Artifact must exist on the FILE face.
	got, _, _ := a.store.GetEpisode(ep.ID)
	if got.AudioPath == "" {
		t.Fatal("audio_path not written back")
	}
	if got.Status != StatusReady {
		t.Errorf("episode status: %q want ready", got.Status)
	}
	abs := got.AudioPath
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(ws, got.AudioPath)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Errorf("artifact file missing: %v", err)
	}
	// Artifact lives under .artifacts/radio/audio (file face, not meta.db).
	if !contains(abs, filepath.Join(".artifacts", AppID, "audio")) {
		t.Errorf("artifact not under file face: %s", abs)
	}
}

// ── status writeback on agent-stage completion ───────────────────────────────

func TestCompletionHookAdvancesStatus(t *testing.T) {
	a, taskStore, ws, cleanup := newTestApp(t)
	defer cleanup()

	ep, _ := a.store.CreateEpisode(ws, "回写测试", "https://example.com/z")
	ids, _ := a.StartPipeline(ep.ID, ws)
	summaryID, transcriptID := ids[0], ids[1]

	// Complete stage 1 (summary) — simulate the agent terminal result.
	completeTask(t, taskStore, a.api, ws, summaryID, `{"text":"这是一段总结。"}`)
	got, _, _ := a.store.GetEpisode(ep.ID)
	if got.Summary != "这是一段总结。" {
		t.Errorf("summary writeback: %q", got.Summary)
	}
	if got.Status != StatusTranscribing {
		t.Errorf("status after summary: %q want transcribing", got.Status)
	}

	// Complete stage 2 (transcript).
	completeTask(t, taskStore, a.api, ws, transcriptID, "逐字稿全文。")
	got, _, _ = a.store.GetEpisode(ep.ID)
	if got.Transcript != "逐字稿全文。" {
		t.Errorf("transcript writeback: %q", got.Transcript)
	}
	if got.Status != StatusSynthesizing {
		t.Errorf("status after transcript: %q want synthesizing", got.Status)
	}
}

// ── hook ignores non-radio tasks (R1/contract: claim by business_ref) ────────

func TestCompletionHookIgnoresForeignTasks(t *testing.T) {
	a, taskStore, ws, cleanup := newTestApp(t)
	defer cleanup()

	ep, _ := a.store.CreateEpisode(ws, "隔离测试", "")
	// A task in another app's namespace must not touch radio state.
	id, _ := a.api.DispatchTask("crm", taskapi.DispatchSpec{
		Title:         "crm task",
		Executor:      meta.TaskExecutorAgent,
		BusinessRef:   "crm:lead:1",
		WorkspacePath: ws,
	})
	completeTask(t, taskStore, a.api, ws, id, "irrelevant")

	got, _, _ := a.store.GetEpisode(ep.ID)
	if got.Status != StatusDraft {
		t.Errorf("foreign task changed radio episode: %q", got.Status)
	}
}

// completeTask marks a task completed in the store and fires completion hooks,
// mirroring the kernel's finish path.
func completeTask(t *testing.T, store *meta.TaskStore, api *taskapi.API, ws, id, result string) {
	t.Helper()
	err := store.Mutate(ws, func(cfg *meta.TasksConfig) bool {
		for i := range cfg.Tasks {
			if cfg.Tasks[i].ID == id {
				cfg.Tasks[i].Status = meta.TaskStatusCompleted
				cfg.Tasks[i].Result = result
				return true
			}
		}
		return false
	})
	if err != nil {
		t.Fatalf("mutate: %v", err)
	}
	api.NotifyCompletion(taskapi.CompletionEvent{
		TaskID: id, Status: meta.TaskStatusCompleted, Result: result,
	})
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
