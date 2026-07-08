package govern

import (
	"os"
	"os/exec"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/data"
	"github.com/scottzx/1Agents/backend/internal/sources"
)

var execLookPath = exec.LookPath

// TestXunjiLivePipeline proves the full manifest path against the real 训记 API:
// generic REST puller → bronze (Store.Sync) → generic silver governor → 数据归一
// viewer. Runs into a throwaway ONEAGENTS_HOME so it never touches real DBs.
// Guarded by XUNJI_TOKEN.
//
//	XUNJI_TOKEN=xjllm_... go test ./internal/govern/ -run TestXunjiLivePipeline -v
func TestXunjiLivePipeline(t *testing.T) {
	token := os.Getenv("XUNJI_TOKEN")
	if token == "" {
		t.Skip("set XUNJI_TOKEN to run the live 训记 pipeline test")
	}
	t.Setenv("ONEAGENTS_HOME", t.TempDir())

	bronze, err := sources.OpenDefault()
	if err != nil {
		t.Fatalf("open bronze: %v", err)
	}
	silver, err := data.OpenDefault()
	if err != nil {
		t.Fatalf("open silver: %v", err)
	}

	d := sources.RESTDescriptor{
		Kind:         "xunji_train",
		Method:       "POST",
		Endpoint:     "/api_trains_for_llm_v2",
		Body:         map[string]any{"schema_version": "train_open_api_v2", "include_full_data": false},
		AuthScheme:   "bearer",
		ItemPath:     "res.trains",
		UIDField:     "localid",
		CursorFlavor: "date-window",
		DateParam:    "datestr",
		LookbackDays: 4, // covers the 2026-07-06 training day
	}
	puller := sources.NewRESTPuller("xunji", "https://trains.xunjiapp.cn",
		[]sources.RESTDescriptor{d}, func() (string, bool) { return token, true })

	// 1) Ingest: REST → bronze.
	stats, err := bronze.Sync(puller, "default")
	if err != nil {
		t.Fatalf("bronze sync: %v", err)
	}
	t.Logf("bronze: %d collections, %d changed", stats.Collections, stats.Changed)
	if stats.Changed == 0 {
		t.Fatal("expected at least one bronze row from the live window")
	}

	// 2) Govern: bronze → generic silver.
	spec := ManifestSilverSpec{
		Source: "xunji", Kind: "xunji_train", Table: "silver_xunji", Domain: "fitness",
		Promote: map[string]string{"title": "title", "datestr": "datestr"},
	}
	data.RegisterViewerTable(spec.Domain, spec.Source, spec.Table)
	n, err := SilverManifest(bronze, silver, spec)
	if err != nil {
		t.Fatalf("SilverManifest: %v", err)
	}
	t.Logf("silver: %d rows into %s", n, spec.Table)

	// 3) Govern silver→gold: declarative SQL step projects payload JSON → columns.
	goldStep := SQLStep{
		Name: "gold_xunji_train", Upstreams: []string{"silver_xunji"}, Output: "gold_xunji_train",
		IncrTable: "silver_xunji", IncrCol: "updated_at",
		CreateSQL: `CREATE TABLE IF NOT EXISTS gold_xunji_train (
			source TEXT NOT NULL DEFAULT 'xunji', external_id TEXT NOT NULL,
			datestr TEXT, title TEXT, movements INTEGER, duration_s INTEGER,
			updated_at INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (external_id))`,
		Body: `INSERT INTO gold_xunji_train
			(source, external_id, datestr, title, movements, duration_s, updated_at)
			SELECT 'xunji', external_id, datestr, title,
			  COALESCE(json_array_length(json_extract(payload,'$.movements')),0),
			  CAST((COALESCE(json_extract(payload,'$.end'),0)-COALESCE(json_extract(payload,'$.start'),0))/1000 AS INTEGER),
			  updated_at
			FROM silver_xunji WHERE updated_at > :since
			ON CONFLICT(external_id) DO UPDATE SET
			  datestr=excluded.datestr, title=excluded.title, movements=excluded.movements,
			  duration_s=excluded.duration_s, updated_at=excluded.updated_at`,
	}
	if gn, err := RunSQLStep(silver, goldStep); err != nil {
		t.Fatalf("gold SQL step: %v", err)
	} else {
		t.Logf("gold: %d rows into gold_xunji_train", gn)
	}
	var mv, dur int
	var title string
	if err := silver.SQL().QueryRow(
		`SELECT title, movements, duration_s FROM gold_xunji_train WHERE external_id='1783328972762'`,
	).Scan(&title, &mv, &dur); err != nil {
		t.Fatalf("read gold row: %v", err)
	}
	t.Logf("gold row: title=%s movements=%d duration_s=%d", title, mv, dur)
	if mv != 5 { // the 2026-07-06 training has 5 movements
		t.Errorf("movements = %d, want 5", mv)
	}

	// 3b) Govern via Python script step (nested-array aggregation SQL can't do):
	// run the real example script against the live record.
	if _, err := execLookPath("python3"); err == nil {
		scriptStep := ScriptStep{
			Name: "gold_xunji_volume", Upstreams: []string{"silver_xunji"}, Output: "gold_xunji_volume",
			Script:  "../../examples/connectors/scripts/xunji_volume.py",
			InputSQL: "SELECT external_id, datestr, payload, updated_at FROM silver_xunji WHERE updated_at > :since",
			IncrCol: "updated_at", Conflict: []string{"external_id"},
			CreateSQL: `CREATE TABLE IF NOT EXISTS gold_xunji_volume (
				source TEXT, external_id TEXT PRIMARY KEY, datestr TEXT,
				total_volume_kg REAL, total_sets INTEGER, updated_at INTEGER)`,
		}
		if sn, err := RunScriptStep(silver, scriptStep); err != nil {
			t.Fatalf("script step: %v", err)
		} else {
			t.Logf("script gold: %d rows into gold_xunji_volume", sn)
		}
		var vol float64
		var sets int
		if err := silver.SQL().QueryRow(
			`SELECT total_volume_kg, total_sets FROM gold_xunji_volume WHERE external_id='1783328972762'`,
		).Scan(&vol, &sets); err != nil {
			t.Fatalf("read script gold: %v", err)
		}
		t.Logf("script gold row: total_volume_kg=%v total_sets=%d", vol, sets)
		if sets == 0 || vol == 0 {
			t.Errorf("expected non-zero volume/sets, got vol=%v sets=%d", vol, sets)
		}
	}

	// 4) View: schema-free viewer renders the promoted columns.
	rows, err := silver.ListSilver("fitness", "xunji", 100)
	if err != nil {
		t.Fatalf("ListSilver: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("viewer returned no rows")
	}
	for _, r := range rows {
		title, datestr := "", ""
		for _, f := range r.Fields {
			switch f.Key {
			case "title":
				title = f.Value
			case "datestr":
				datestr = f.Value
			}
		}
		t.Logf("  viewer row: uid=%s datestr=%s title=%s", r.UID, datestr, title)
	}
}
