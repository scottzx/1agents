package govern

import (
	"testing"

	"github.com/scottzx/1Agents/backend/internal/data"
	"github.com/scottzx/1Agents/backend/internal/sources"
)

func TestSilverManifest_LandsBronzeIntoGenericTable(t *testing.T) {
	t.Setenv("ONEAGENTS_HOME", t.TempDir())

	// Fresh bronze (sync.db) + silver (data.db) stores over temp home.
	bronze, err := sources.OpenDefault()
	if err != nil {
		t.Fatalf("open bronze: %v", err)
	}
	silver, err := data.OpenDefault()
	if err != nil {
		t.Fatalf("open silver: %v", err)
	}

	// Seed two 训记-shaped bronze rows via a canned puller through Store.Sync.
	p := &cannedPuller{source: "xunji", kind: "xunji_train", recs: []sources.RawRecord{
		{Kind: "xunji_train", Collection: "xunji_train", UID: "t-1", ContentType: "application/json",
			Payload: `{"localid":"t-1","title":"胸部训练","datestr":"2026-04-02"}`},
		{Kind: "xunji_train", Collection: "xunji_train", UID: "t-2", ContentType: "application/json",
			Payload: `{"localid":"t-2","title":"背部训练","datestr":"2026-04-03"}`},
	}}
	if _, err := bronze.Sync(p, "default"); err != nil {
		t.Fatalf("sync bronze: %v", err)
	}

	spec := ManifestSilverSpec{
		Source: "xunji", Kind: "xunji_train", Table: "silver_xunji", Domain: "fitness",
		Promote: map[string]string{"title": "title", "datestr": "datestr"},
	}
	data.RegisterViewerTable(spec.Domain, spec.Source, spec.Table)

	n, err := SilverManifest(bronze, silver, spec)
	if err != nil {
		t.Fatalf("SilverManifest: %v", err)
	}
	if n != 2 {
		t.Fatalf("wrote %d rows, want 2", n)
	}

	// Re-run must be a no-op (cursor advanced, idempotent).
	if n2, err := SilverManifest(bronze, silver, spec); err != nil || n2 != 0 {
		t.Fatalf("re-run = %d/%v, want 0/nil", n2, err)
	}

	// Viewer renders the promoted columns + payload, schema-free.
	rows, err := silver.ListSilver("fitness", "xunji", 100)
	if err != nil {
		t.Fatalf("ListSilver: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("viewer rows = %d, want 2", len(rows))
	}
	found := false
	for _, r := range rows {
		for _, f := range r.Fields {
			if f.Key == "title" && f.Value == "胸部训练" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("promoted title column not rendered: %+v", rows)
	}
}

// cannedPuller replays a fixed record set once, for driving Store.Sync in tests.
type cannedPuller struct {
	source string
	kind   string
	recs   []sources.RawRecord
}

func (p *cannedPuller) Source() string { return p.source }
func (p *cannedPuller) Discover(string) ([]sources.Collection, error) {
	return []sources.Collection{{Kind: p.kind, ID: p.kind}}, nil
}
func (p *cannedPuller) Pull(string, sources.Collection, sources.Cursor) ([]sources.RawRecord, sources.Cursor, bool, error) {
	return p.recs, sources.Cursor{Kind: "none"}, true, nil
}
