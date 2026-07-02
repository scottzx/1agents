package ingest

import (
	"os"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/taskapi"
)

func openTestDB(t *testing.T) (*meta.DB, func()) {
	t.Helper()
	f, err := os.CreateTemp("", "ingest-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	db, err := meta.Open(f.Name())
	if err != nil {
		os.Remove(f.Name())
		t.Fatal(err)
	}
	return db, func() { db.Close(); os.Remove(f.Name()) }
}

func TestDispatcherSyncNowShape(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()
	store := meta.NewTaskStore(db)
	api := taskapi.New(store)
	ws := t.TempDir()
	d := NewDispatcher(api, store, ws)

	id, err := d.SyncNow("feishu", "feishu_chat", "")
	if err != nil {
		t.Fatal(err)
	}
	task, ok, err := store.GetTask(id)
	if err != nil || !ok {
		t.Fatalf("task not found: %v", err)
	}
	if task.Executor != meta.TaskExecutorFunction {
		t.Errorf("executor = %q, want function", task.Executor)
	}
	if task.BusinessRef != "sources:feishu:feishu_chat" {
		t.Errorf("business_ref = %q", task.BusinessRef)
	}
	if taskapi.ExtractFunctionType(task.Labels) != "sources.feishu.sync" {
		t.Errorf("fn label = %v", task.Labels)
	}
	if task.Recurrence != nil {
		t.Errorf("sync-now must not be recurring")
	}
}

func TestDispatcherEnsureRecurringIdempotent(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()
	store := meta.NewTaskStore(db)
	api := taskapi.New(store)
	ws := t.TempDir()
	d := NewDispatcher(api, store, ws)

	for i := 0; i < 3; i++ {
		if err := d.EnsureRecurring("feishu", "feishu_chat", 30); err != nil {
			t.Fatalf("ensure %d: %v", i, err)
		}
	}
	tasks, err := store.ListTasksByBusinessRef("sources:feishu:feishu_chat")
	if err != nil {
		t.Fatal(err)
	}
	recurring := 0
	for _, tk := range tasks {
		if tk.Recurrence != nil {
			recurring++
			if tk.Recurrence.Freq != "interval" || tk.Recurrence.EveryMinutes != 30 {
				t.Errorf("recurrence = %+v, want interval/30", tk.Recurrence)
			}
		}
	}
	if recurring != 1 {
		t.Fatalf("recurring tasks = %d, want exactly 1 (idempotent)", recurring)
	}
}

func TestConfigStoreRoundTrip(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()
	cfg := meta.NewSourceCollectionStore(db)

	// Never-configured → defaults, ok=false.
	c, ok, err := cfg.Get("feishu", "feishu_chat")
	if err != nil {
		t.Fatal(err)
	}
	if ok || c.PageSize != 50 || c.IncrementalMinutes != 60 {
		t.Fatalf("defaults wrong: ok=%v %+v", ok, c)
	}

	// Upsert enabled → read back.
	if err := cfg.Upsert(meta.SourceCollectionConfig{
		Source: "feishu", Kind: "feishu_chat", Enabled: true,
		InitialLookbackDays: 30, IncrementalMinutes: 15, PageSize: 100,
	}); err != nil {
		t.Fatal(err)
	}
	c, ok, _ = cfg.Get("feishu", "feishu_chat")
	if !ok || !c.Enabled || c.IncrementalMinutes != 15 || c.PageSize != 100 {
		t.Fatalf("round-trip wrong: %+v", c)
	}
	enabled, _ := cfg.ListEnabled("feishu")
	if len(enabled) != 1 {
		t.Fatalf("enabled = %d, want 1", len(enabled))
	}
}
