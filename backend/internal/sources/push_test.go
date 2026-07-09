package sources

import (
	"encoding/json"
	"testing"
)

const pushManifestYAML = `
vendor: agent_insights_test
label: Agent 洞察(测试)
collections:
  - kind: sentiment
    domain: insights
    label: 情感分析
    transport: push
    uidField: id
    schema:
      - name: id
        type: string
        required: true
      - name: score
        type: number
      - name: label
        type: string
    silver:
      table: silver_agent_sentiment_test
      domain: insights
`

func TestPushManifest_ParsesValidatesRegisters(t *testing.T) {
	m, err := ParseManifest([]byte(pushManifestYAML))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := ValidateManifest(m); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !m.hasPush() {
		t.Fatalf("hasPush = false, want true")
	}

	RegisterManifest(m)

	if v := VendorFor(m.Vendor); v == nil || v.AuthKind != AuthPush {
		t.Fatalf("vendor authKind = %+v, want %q", v, AuthPush)
	}
	if !IsPushSource(m.Vendor) {
		t.Fatalf("IsPushSource(%q) = false", m.Vendor)
	}
	if !IsPushKind(m.Vendor, "sentiment") {
		t.Fatalf("IsPushKind sentiment = false")
	}
	d, ok := PushDescriptorFor(m.Vendor, "sentiment")
	if !ok {
		t.Fatalf("PushDescriptorFor: not found")
	}
	if len(d.Schema) != 3 || d.UIDField != "id" {
		t.Fatalf("descriptor schema=%d uid=%q, want 3/id", len(d.Schema), d.UIDField)
	}

	// A silver table with no explicit promote falls back to the schema columns.
	promote := m.Collections[0].SchemaPromote()
	for _, col := range []string{"id", "score", "label"} {
		if promote[col] != col {
			t.Fatalf("SchemaPromote missing %q: %+v", col, promote)
		}
	}
}

func TestBuildPushRecords_ValidatesAndDedups(t *testing.T) {
	d := RESTDescriptor{
		Kind:      "sentiment",
		Transport: "push",
		UIDField:  "id",
		Schema: []PushField{
			{Name: "id", Type: "string", Required: true},
			{Name: "score", Type: "number"},
		},
	}

	// One valid item → one record; UID from the declared field.
	recs, rejects := BuildPushRecords(d, "", []json.RawMessage{
		json.RawMessage(`{"id":"s-1","score":0.9,"label":"positive"}`),
	})
	if len(rejects) != 0 || len(recs) != 1 {
		t.Fatalf("valid item: recs=%d rejects=%v", len(recs), rejects)
	}
	if recs[0].UID != "s-1" || recs[0].ContentType != "application/json" {
		t.Fatalf("record = %+v", recs[0])
	}
	firstETag := recs[0].ETag
	if firstETag == "" {
		t.Fatalf("etag not set")
	}

	// Identical re-push → identical UID + ETag (so CommitPage is a no-op).
	recs2, _ := BuildPushRecords(d, "", []json.RawMessage{
		json.RawMessage(`{"id":"s-1","score":0.9,"label":"positive"}`),
	})
	if recs2[0].UID != "s-1" || recs2[0].ETag != firstETag {
		t.Fatalf("re-push not idempotent: %+v vs etag %s", recs2[0], firstETag)
	}

	// Missing required field → reject; a non-object → reject; wrong type → reject.
	_, rej := BuildPushRecords(d, "", []json.RawMessage{
		json.RawMessage(`{"score":0.1}`),               // missing required id
		json.RawMessage(`[1,2,3]`),                     // not an object
		json.RawMessage(`{"id":"s-2","score":"high"}`), // score not a number
	})
	if len(rej) != 3 {
		t.Fatalf("rejects = %d, want 3: %+v", len(rej), rej)
	}

	// No uidField / no id → content hash keeps it stable + deduping.
	dNoUID := RESTDescriptor{Kind: "k", Transport: "push"}
	a, _ := BuildPushRecords(dNoUID, "", []json.RawMessage{json.RawMessage(`{"a":1}`)})
	b, _ := BuildPushRecords(dNoUID, "", []json.RawMessage{json.RawMessage(`{"a":1}`)})
	if a[0].UID == "" || a[0].UID != b[0].UID {
		t.Fatalf("content-hash uid unstable: %q vs %q", a[0].UID, b[0].UID)
	}
}
