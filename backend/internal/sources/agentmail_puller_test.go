package sources

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"testing"
)

// agentMailSample mirrors the real `agently-cli message +list` stdout schema.
const agentMailSample = `{
  "ok": true,
  "data": {
    "data": [
      {
        "message_id": "msg_001",
        "subject": "周报",
        "snippet": "本周进展……",
        "is_read": false,
        "created_at": "2026-06-30T08:00:00Z",
        "from": {"name": "Bob", "email": "bob@example.com"}
      },
      {
        "message_id": "msg_002",
        "subject": "",
        "snippet": "no subject body",
        "is_read": true,
        "created_at": "2026-06-30T09:00:00Z",
        "from": {"name": "Alice", "email": "alice@example.com"}
      }
    ],
    "pagination": {"has_more": false, "next_cursor": ""}
  }
}`

// newFakeAgentMailPuller returns a puller whose exec seam serves out and records
// the --after value each Pull passed.
func newFakeAgentMailPuller(out string, gotAfter *string) *agentmailPuller {
	return &agentmailPuller{
		runList: func(_ context.Context, after string) ([]byte, error) {
			if gotAfter != nil {
				*gotAfter = after
			}
			return []byte(out), nil
		},
	}
}

func TestAgentMailDiscover(t *testing.T) {
	p := NewAgentMailPuller()
	colls, err := p.Discover("acct")
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(colls) != 1 || colls[0].Kind != agentMailKind || colls[0].ID != agentMailColl {
		t.Fatalf("discover = %+v, want one inbox collection", colls)
	}
	if colls[0].Gate != "" {
		t.Fatalf("gate = %q, want empty (always pull)", colls[0].Gate)
	}
}

func TestAgentMailPullMapsAndWatermarks(t *testing.T) {
	var after string
	p := newFakeAgentMailPuller(agentMailSample, &after)

	recs, next, done, err := p.Pull("acct", Collection{Kind: agentMailKind, ID: agentMailColl}, Cursor{})
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if !done {
		t.Fatal("want done=true (single page)")
	}
	if after != "" {
		t.Fatalf("first pull passed --after=%q, want empty", after)
	}
	if len(recs) != 2 {
		t.Fatalf("got %d records, want 2", len(recs))
	}
	r := recs[0]
	if r.UID != "msg_001" || r.Kind != agentMailKind || r.Collection != agentMailColl {
		t.Fatalf("record[0] mismapped: %+v", r)
	}
	if r.ContentType != "application/json" {
		t.Fatalf("content type = %q, want application/json", r.ContentType)
	}
	if r.ETag == "" {
		t.Fatal("etag (content hash) must be set for dedup")
	}
	// Payload is the verbatim message object, not the outer envelope.
	if !jsonHasField(t, r.Payload, "message_id", "msg_001") {
		t.Fatalf("payload not verbatim message JSON: %s", r.Payload)
	}
	// Watermark advances to the newest created_at.
	if next.Kind != "timestamp" || next.Value != "2026-06-30T09:00:00Z" {
		t.Fatalf("cursor = %+v, want timestamp 2026-06-30T09:00:00Z", next)
	}
}

func TestAgentMailPullPassesWatermarkAsAfter(t *testing.T) {
	var after string
	p := newFakeAgentMailPuller(`{"ok":true,"data":{"data":[]}}`, &after)

	_, _, _, err := p.Pull("acct", Collection{Kind: agentMailKind, ID: agentMailColl},
		Cursor{Kind: "timestamp", Value: "2026-06-30T09:00:00Z"})
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if after != "2026-06-30T09:00:00Z" {
		t.Fatalf("second pull --after=%q, want the stored watermark", after)
	}
}

func TestParseAgentMailListErrors(t *testing.T) {
	if _, err := parseAgentMailList([]byte(`{"ok":false,"data":{"data":[]}}`)); err == nil {
		t.Fatal("ok=false should error")
	}
	if _, err := parseAgentMailList([]byte(`not json`)); err == nil {
		t.Fatal("malformed json should error")
	}
	msgs, err := parseAgentMailList([]byte(`{"ok":true,"data":{"data":[]}}`))
	if err != nil || len(msgs) != 0 {
		t.Fatalf("empty inbox = (%v, %d), want (nil, 0)", err, len(msgs))
	}
}

// jsonHasField reports whether raw JSON has a top-level string field == want.
func jsonHasField(t *testing.T, raw, field, want string) bool {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("payload not json: %v", err)
	}
	s, _ := m[field].(string)
	return s == want
}

// TestAgentMailLive drives the real agently-cli into a temp bronze store. Skipped
// unless AGENTMAIL_SMOKE=1 and agently-cli is on PATH + authorized.
func TestAgentMailLive(t *testing.T) {
	if os.Getenv("AGENTMAIL_SMOKE") != "1" {
		t.Skip("set AGENTMAIL_SMOKE=1 to run the live agently-cli pull")
	}
	if _, err := exec.LookPath(agentMailBin); err != nil {
		t.Skipf("%s not on PATH: %v", agentMailBin, err)
	}
	st := openSourcesStore(t)
	stats, err := st.Sync(NewAgentMailPuller(), "live")
	if err != nil {
		t.Fatalf("live sync: %v", err)
	}
	t.Logf("live sync stats: %+v", stats)
	if stats.Collections != 1 {
		t.Fatalf("collections = %d, want 1", stats.Collections)
	}
	// Second sync should add nothing new (watermark + ETag dedup).
	stats2, err := st.Sync(NewAgentMailPuller(), "live")
	if err != nil {
		t.Fatalf("second live sync: %v", err)
	}
	if stats2.Changed != 0 {
		t.Fatalf("re-sync changed = %d, want 0 (dedup)", stats2.Changed)
	}
}
