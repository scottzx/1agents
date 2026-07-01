package sources

import (
	"os"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/feishu"
)

// TestFeishuPullerLive crawls real Feishu chats through the installed, logged-in
// lark-cli into a temp bronze store. It is gated behind LARK_LIVE=1 so normal
// test runs never shell out; run it with:
//
//	LARK_LIVE=1 go test ./internal/sources/ -run TestFeishuPullerLive -v
func TestFeishuPullerLive(t *testing.T) {
	if os.Getenv("LARK_LIVE") != "1" {
		t.Skip("set LARK_LIVE=1 to run the live lark-cli integration test")
	}
	st := openSourcesStore(t)
	client := feishu.NewClient("", "default")
	p := NewFeishuPuller(client, []FeishuSpec{{Kind: "feishu_chat", PageSize: 50}})

	stats, err := st.Sync(p, "default")
	if err != nil {
		t.Fatalf("live sync: %v", err)
	}
	t.Logf("live sync stats: collections=%d changed=%d skipped=%d", stats.Collections, stats.Changed, stats.Skipped)

	recs, err := st.ListRecords(feishu.Source, "feishu_chat", 0)
	if err != nil {
		t.Fatalf("list records: %v", err)
	}
	t.Logf("bronze feishu_chat records: %d", len(recs))
	if len(recs) == 0 {
		t.Skip("no chats returned (account may be in no groups) — path still exercised")
	}
	// Every landed record must carry a stable uid, an etag, and verbatim JSON.
	r := recs[0]
	if r.UID == "" || r.ETag == "" || r.ContentType != "application/json" || r.Payload == "" {
		t.Fatalf("malformed bronze record: %+v", r)
	}
	t.Logf("sample chat record: uid=%s payload=%.120s", r.UID, r.Payload)
}
