package govern

import (
	"os"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/data"
	"github.com/scottzx/1Agents/backend/internal/sources"
)

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

	// 3) View: schema-free viewer renders the promoted columns.
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
