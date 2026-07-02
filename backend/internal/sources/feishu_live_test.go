package sources

import (
	"context"
	"os"
	"testing"
	"time"

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

// TestFeishuMessagePullLive verifies the message kind (timestamp flavor, per-chat
// fan-out) lands raw messages into bronze — the bronze side of the message
// migration. Gated behind LARK_LIVE=1.
func TestFeishuMessagePullLive(t *testing.T) {
	if os.Getenv("LARK_LIVE") != "1" {
		t.Skip("set LARK_LIVE=1 to run the live lark-cli integration test")
	}
	st := openSourcesStore(t)
	client := feishu.NewClient("", "default")

	chats, err := client.ListChats(newLiveCtx(t))
	if err != nil {
		t.Fatalf("list chats: %v", err)
	}
	if len(chats) == 0 {
		t.Skip("account is in no chats — nothing to pull messages from")
	}
	// Pull messages for the first chat over a wide lookback.
	spec := FeishuSpec{Kind: "feishu_message", PageSize: 50, LookbackDays: 30, ChatIDs: []string{chats[0].ChatID}}
	p := NewFeishuPuller(client, []FeishuSpec{spec})
	stats, err := st.Sync(p, "default")
	if err != nil {
		t.Fatalf("message sync: %v", err)
	}
	recs, _ := st.ListRecords(feishu.Source, "feishu_message", 0)
	t.Logf("chat=%s message sync: changed=%d bronze feishu_message records=%d", chats[0].ChatID, stats.Changed, len(recs))
	if len(recs) > 0 {
		if recs[0].UID == "" || recs[0].ContentType != "application/json" {
			t.Fatalf("malformed message record: %+v", recs[0])
		}
		t.Logf("sample message: uid=%s payload=%.100s", recs[0].UID, recs[0].Payload)
	}
}

// TestFeishuCalendarPullLive verifies a non-"items" ItemPath (data.calendar_list)
// lands into bronze. Gated behind LARK_LIVE=1.
func TestFeishuCalendarPullLive(t *testing.T) {
	if os.Getenv("LARK_LIVE") != "1" {
		t.Skip("set LARK_LIVE=1 to run the live lark-cli integration test")
	}
	st := openSourcesStore(t)
	client := feishu.NewClient("", "default")
	p := NewFeishuPuller(client, []FeishuSpec{{Kind: "feishu_calendar", PageSize: 50}})
	stats, err := st.Sync(p, "default")
	if err != nil {
		t.Fatalf("calendar sync: %v", err)
	}
	recs, _ := st.ListRecords(feishu.Source, "feishu_calendar", 0)
	t.Logf("calendar sync: changed=%d bronze feishu_calendar records=%d", stats.Changed, len(recs))
	if len(recs) > 0 && (recs[0].UID == "" || recs[0].ContentType != "application/json") {
		t.Fatalf("malformed calendar record: %+v", recs[0])
	}
}

func newLiveCtx(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	return ctx
}
