package meta

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestTurnChangeReportSchemaHealsWithoutContentBackfill(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := db.sql.Exec(`
		DROP TABLE turn_change_reports;
		PRAGMA user_version = 32;
	`); err != nil {
		db.Close()
		t.Fatalf("strip table: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	db, err = Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db.Close()
	ok, err := db.tableExists("turn_change_reports")
	if err != nil || !ok {
		t.Fatalf("table healed: ok=%v err=%v", ok, err)
	}
	var version int
	if err := db.sql.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != schemaVersion {
		t.Fatalf("user_version=%d want %d", version, schemaVersion)
	}
	var n int
	if err := db.sql.QueryRow(`SELECT COUNT(*) FROM turn_change_reports`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("Open backfilled %d reports; want 0", n)
	}
}

func TestTurnChangeNeedsRecomputeAndSkip(t *testing.T) {
	if NeedsRecompute(nil, TurnChangeRecipeVersion, "") {
		// missing row always recomputes
	} else {
		t.Fatal("nil report must recompute")
	}
	current := &TurnChangeReport{TurnID: "t1", RecipeVersion: TurnChangeRecipeVersion}
	if NeedsRecompute(current, TurnChangeRecipeVersion, "") {
		t.Fatal("current recipe with no finished turn must skip")
	}
	if !NeedsRecompute(current, TurnChangeRecipeVersion, "t1") {
		t.Fatal("just-finished turn must recompute")
	}
	if NeedsRecompute(current, TurnChangeRecipeVersion, "t-other") {
		t.Fatal("unrelated finished turn must not invalidate this row")
	}
	stale := &TurnChangeReport{TurnID: "t1", RecipeVersion: TurnChangeRecipeVersion - 1}
	if !NeedsRecompute(stale, TurnChangeRecipeVersion, "") {
		t.Fatal("stale recipe must recompute")
	}
}

func TestTurnChangeStoreUpsertAndResolve(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	seedTurnProject(t, db, "p1", "s1")
	turns := NewAgentTurnStore(db)
	created, _, err := turns.Create(AgentTurn{
		ProjectID:       "p1",
		SessionID:       "s1",
		ClientRequestID: "req-1",
		PromptText:      "edit files",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := turns.Transition(created.ID, AgentTurnTransition{
		Status:           AgentTurnRunning,
		RuntimeRequestID: "runtime-1",
	}); err != nil {
		t.Fatal(err)
	}

	store := NewTurnChangeStore(db)
	for _, key := range []string{created.ID, "req-1", "runtime-1"} {
		id, ok, err := store.ResolveTurnID("s1", key)
		if err != nil || !ok || id != created.ID {
			t.Fatalf("ResolveTurnID(%q)=%s ok=%v err=%v want %s", key, id, ok, err, created.ID)
		}
	}

	report := TurnChangeReport{
		TurnID:        created.ID,
		RecipeVersion: TurnChangeRecipeVersion,
		AddedCount:    1,
		Files:         []TurnChangeFile{{Path: "a.ts", Op: TurnChangeAdded, Tool: "Write"}},
		Source:        TurnChangeBackfill,
		ComputedAt:    time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC),
	}
	if err := store.Upsert(report); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.Get(created.ID)
	if err != nil || !ok {
		t.Fatalf("Get: ok=%v err=%v", ok, err)
	}
	if got.AddedCount != 1 || len(got.Files) != 1 || got.Files[0].Path != "a.ts" {
		t.Fatalf("got %+v", got)
	}
	listed, err := store.ListByTurnIDs([]string{created.ID, "missing"})
	if err != nil || len(listed) != 1 || listed[created.ID].AddedCount != 1 {
		t.Fatalf("ListByTurnIDs=%v err=%v", listed, err)
	}

	report.AddedCount = 2
	report.Source = TurnChangeLive
	if err := store.Upsert(report); err != nil {
		t.Fatal(err)
	}
	got, _, _ = store.Get(created.ID)
	if got.AddedCount != 2 || got.Source != TurnChangeLive {
		t.Fatalf("upsert overwrite: %+v", got)
	}
}

func TestAggregateTurnChangesRecipeV1(t *testing.T) {
	items := []HistoryChangeItem{
		{Kind: "user", TurnID: "turn-a"},
		{Kind: "tool_use", TurnID: "turn-a", ToolName: "Read", ToolCallID: "c0", Input: json.RawMessage(`{"path":"skip.ts"}`)},
		{Kind: "tool_use", TurnID: "turn-a", ToolName: "Write", ToolCallID: "c1", Input: json.RawMessage(`{"path":"a.ts","contents":"x"}`)},
		{Kind: "tool_use", TurnID: "turn-a", ToolName: "Edit", ToolCallID: "c2", Input: json.RawMessage(`{"file_path":"a.ts","old_string":"x","new_string":"y"}`)},
		{Kind: "tool_use", TurnID: "turn-a", ToolName: "mv", ToolCallID: "c3", Input: json.RawMessage(`{"from":"old.ts","to":"new.ts"}`)},
		{Kind: "tool_use", TurnID: "turn-a", ToolName: "Bash", ToolCallID: "c4", Input: json.RawMessage(`{"command":"ls"}`)},
		{Kind: "tool_use", ToolName: "Write", Input: json.RawMessage(`{"path":"orphan.ts"}`)},
		{Kind: "assistant_text", TurnID: "turn-b"},
	}
	got := AggregateTurnChanges(items)
	if _, ok := got["turn-b"]; !ok {
		t.Fatal("turn with no file tools must still appear")
	}
	if len(got["turn-b"]) != 0 {
		t.Fatalf("turn-b files=%v", got["turn-b"])
	}
	if _, ok := got[""]; ok {
		t.Fatal("items without turnId must not write a report key")
	}
	files := got["turn-a"]
	byPath := map[string]TurnChangeFile{}
	for _, f := range files {
		byPath[f.Path] = f
	}
	if byPath["a.ts"].Op != TurnChangeModified {
		t.Fatalf("last op for a.ts=%s want modified", byPath["a.ts"].Op)
	}
	if byPath["old.ts"].Op != TurnChangeDeleted || byPath["new.ts"].Op != TurnChangeAdded {
		t.Fatalf("move: %+v", byPath)
	}
	if _, ok := byPath["skip.ts"]; ok {
		t.Fatal("Read must be ignored")
	}
	if _, ok := byPath["orphan.ts"]; ok {
		t.Fatal("tool_use without turnId must be dropped")
	}
	added, deleted, modified := CountTurnChangeOps(files)
	if added != 1 || deleted != 1 || modified != 1 {
		t.Fatalf("counts added=%d deleted=%d modified=%d files=%v", added, deleted, modified, files)
	}
}

func TestAggregateTurnChangesShellRmIsDeleted(t *testing.T) {
	items := []HistoryChangeItem{
		{Kind: "tool_use", TurnID: "turn-rm", ToolName: "Bash", ToolCallID: "c1", Input: json.RawMessage(`{"command":"rm -f .tmp/1acp-turn-smoke.txt"}`)},
	}
	got := AggregateTurnChanges(items)
	files := got["turn-rm"]
	if len(files) != 1 {
		t.Fatalf("files=%v", files)
	}
	if files[0].Path != ".tmp/1acp-turn-smoke.txt" || files[0].Op != TurnChangeDeleted {
		t.Fatalf("file=%+v", files[0])
	}
	added, deleted, modified := CountTurnChangeOps(files)
	if added != 0 || deleted != 1 || modified != 0 {
		t.Fatalf("counts added=%d deleted=%d modified=%d", added, deleted, modified)
	}
}
