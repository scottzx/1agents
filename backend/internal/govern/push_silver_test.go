package govern

import (
	"encoding/json"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/data"
	"github.com/scottzx/1Agents/backend/internal/sources"
)

// TestPushRecords_LandInBronzeThenSilver exercises the full inbound-push path with
// no HTTP layer: build records from raw JSON (as HandlePush does), commit them to
// bronze verbatim (the retention hook), then land them into a schema-derived silver
// table — proving push reuses the same medallion pipeline as pull.
func TestPushRecords_LandInBronzeThenSilver(t *testing.T) {
	t.Setenv("ONEAGENTS_HOME", t.TempDir())

	bronze, err := sources.OpenDefault()
	if err != nil {
		t.Fatalf("open bronze: %v", err)
	}
	silver, err := data.OpenDefault()
	if err != nil {
		t.Fatalf("open silver: %v", err)
	}

	desc := sources.RESTDescriptor{
		Kind:      "sentiment",
		Transport: "push",
		UIDField:  "id",
		Schema: []sources.PushField{
			{Name: "id", Type: "string", Required: true},
			{Name: "score", Type: "number"},
		},
	}
	recs, rejects := sources.BuildPushRecords(desc, "", []json.RawMessage{
		json.RawMessage(`{"id":"s-1","score":0.9,"label":"positive"}`),
		json.RawMessage(`{"id":"s-2","score":0.1,"label":"negative"}`),
	})
	if len(rejects) != 0 {
		t.Fatalf("unexpected rejects: %+v", rejects)
	}
	changed, err := bronze.CommitPage("agent_insights_gov", "default", recs, sources.Cursor{})
	if err != nil {
		t.Fatalf("commit bronze: %v", err)
	}
	if changed != 2 {
		t.Fatalf("bronze committed %d, want 2", changed)
	}

	spec := ManifestSilverSpec{
		Source: "agent_insights_gov", Kind: "sentiment", Table: "silver_agent_sentiment_gov", Domain: "insights",
		Promote: map[string]string{"score": "score", "label": "label"},
	}
	data.RegisterViewerTable(spec.Domain, spec.Source, spec.Table)

	n, err := SilverManifest(bronze, silver, spec)
	if err != nil {
		t.Fatalf("SilverManifest: %v", err)
	}
	if n != 2 {
		t.Fatalf("silver wrote %d, want 2", n)
	}

	// Re-committing the identical push is a no-op (etag unchanged → fetched_at kept),
	// so a second silver run finds nothing new.
	if again, err := bronze.CommitPage("agent_insights_gov", "default", recs, sources.Cursor{}); err != nil || again != 0 {
		t.Fatalf("idempotent re-push = %d/%v, want 0/nil", again, err)
	}
	if n2, err := SilverManifest(bronze, silver, spec); err != nil || n2 != 0 {
		t.Fatalf("silver re-run = %d/%v, want 0/nil", n2, err)
	}

	rows, err := silver.ListSilver("insights", "agent_insights_gov", 100)
	if err != nil {
		t.Fatalf("ListSilver: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("viewer rows = %d, want 2", len(rows))
	}
}
