package feishu

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "sync.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStore_UpsertDedupAndList(t *testing.T) {
	s := openTestStore(t)
	batch := []Message{
		{Channel: Channel, ChannelAccID: "a", MessageID: "om_1", SessionID: "oc_x", SenderID: "ou_1", MsgType: "text", Content: `{"text":"hi"}`, CreateTime: 1000},
		{Channel: Channel, ChannelAccID: "a", MessageID: "om_2", SessionID: "oc_x", SenderID: "ou_2", MsgType: "text", Content: `{"text":"yo"}`, CreateTime: 2000},
	}
	n, err := s.UpsertMessages(batch)
	if err != nil || n != 2 {
		t.Fatalf("first upsert: n=%d err=%v", n, err)
	}
	// Overlap: om_2 repeats (boundary re-fetch), om_3 is new.
	n, err = s.UpsertMessages([]Message{
		batch[1],
		{Channel: Channel, ChannelAccID: "a", MessageID: "om_3", SessionID: "oc_x", SenderID: "ou_1", MsgType: "text", Content: `{"text":"again"}`, CreateTime: 3000},
	})
	if err != nil || n != 1 {
		t.Fatalf("dedup upsert: want 1 new, got n=%d err=%v", n, err)
	}
	got, err := s.ListMessages(Channel, "oc_x", 0, 0)
	if err != nil || len(got) != 3 {
		t.Fatalf("list: len=%d err=%v", len(got), err)
	}
	if got[0].MessageID != "om_1" || got[2].MessageID != "om_3" {
		t.Fatalf("not ascending by create_time: %v", []string{got[0].MessageID, got[1].MessageID, got[2].MessageID})
	}
	// since filter
	since, err := s.ListMessages(Channel, "oc_x", 2000, 0)
	if err != nil || len(since) != 2 {
		t.Fatalf("since filter: len=%d err=%v", len(since), err)
	}
	cnt, err := s.CountMessages(Channel, "oc_x")
	if err != nil || cnt != 3 {
		t.Fatalf("count: %d err=%v", cnt, err)
	}
}

// A limit must return the LATEST N messages (in ascending display order), not
// the oldest N — otherwise a chat timeline truncates away its newest messages.
func TestStore_ListMessages_LimitReturnsLatest(t *testing.T) {
	s := openTestStore(t)
	batch := make([]Message, 0, 5)
	for i := 1; i <= 5; i++ {
		batch = append(batch, Message{
			Channel: Channel, ChannelAccID: "a",
			MessageID: "om_" + string(rune('0'+i)),
			SessionID: "oc_x", SenderID: "ou_1", MsgType: "text",
			Content:    `{"text":"m"}`,
			CreateTime: int64(i * 1000),
		})
	}
	if _, err := s.UpsertMessages(batch); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	// limit 2 → latest two (4000, 5000), still ascending.
	got, err := s.ListMessages(Channel, "oc_x", 0, 2)
	if err != nil || len(got) != 2 {
		t.Fatalf("limited list: len=%d err=%v", len(got), err)
	}
	if got[0].CreateTime != 4000 || got[1].CreateTime != 5000 {
		t.Fatalf("want latest-2 ascending [4000 5000], got [%d %d]", got[0].CreateTime, got[1].CreateTime)
	}
	// MessagesBySenders applies the same latest-N semantics (no session filter).
	bys, err := s.MessagesBySenders(Channel, []string{"ou_1"}, "", 2)
	if err != nil || len(bys) != 2 || bys[0].CreateTime != 4000 || bys[1].CreateTime != 5000 {
		t.Fatalf("MessagesBySenders latest-2: %+v err=%v", bys, err)
	}
	// A session filter restricts to one group.
	inSession, err := s.MessagesBySenders(Channel, []string{"ou_1"}, "oc_x", 0)
	if err != nil || len(inSession) != 5 {
		t.Fatalf("MessagesBySenders session oc_x: want 5, got %d err=%v", len(inSession), err)
	}
	none, err := s.MessagesBySenders(Channel, []string{"ou_1"}, "oc_other", 0)
	if err != nil || len(none) != 0 {
		t.Fatalf("MessagesBySenders session oc_other: want 0, got %d err=%v", len(none), err)
	}
}

func TestStore_Watermark(t *testing.T) {
	s := openTestStore(t)
	if _, ok, err := s.GetWatermark(Channel, "a", "oc_x"); err != nil || ok {
		t.Fatalf("expected no watermark, ok=%v err=%v", ok, err)
	}
	if err := s.SetWatermark(Channel, "a", "oc_x", 1700000000123, "2026-06-25T00:00:00Z"); err != nil {
		t.Fatalf("set: %v", err)
	}
	v, ok, err := s.GetWatermark(Channel, "a", "oc_x")
	if err != nil || !ok || v != 1700000000123 {
		t.Fatalf("get: v=%d ok=%v err=%v", v, ok, err)
	}
	// upsert overwrites
	if err := s.SetWatermark(Channel, "a", "oc_x", 1700000999999, "2026-06-25T01:00:00Z"); err != nil {
		t.Fatalf("set2: %v", err)
	}
	if v, _, _ := s.GetWatermark(Channel, "a", "oc_x"); v != 1700000999999 {
		t.Fatalf("overwrite: %d", v)
	}
}

// fakeCLI returns canned message JSON. SyncChat no longer fetches chat.members
// (the name map is supplied by the caller), so any /members call here is a bug —
// it errors loudly to assert that ZERO roster calls happen during a sync.
func fakeCLI(msgsJSON string) CLIRunner {
	return func(ctx context.Context, args ...string) ([]byte, error) {
		for _, a := range args {
			if strings.Contains(a, "/members") {
				return nil, errTestMembersCalled
			}
		}
		return []byte(msgsJSON), nil
	}
}

var errTestMembersCalled = fmtError("chat.members must not be called during SyncChat")

type fmtError string

func (e fmtError) Error() string { return string(e) }

func TestSyncChat_FakeCLI(t *testing.T) {
	s := openTestStore(t)
	msgs := `{"code":0,"data":{"items":[
        {"message_id":"om_1","msg_type":"text","create_time":"1700000001000","sender":{"id":"ou_1","tenant_key":"tnt_a"},"body":{"content":"{\"text\":\"hi\"}"}},
        {"message_id":"om_2","msg_type":"post","create_time":"1700000002000","sender":{"id":"ou_2","tenant_key":"tnt_b"},"body":{"content":"{\"title\":\"my project\"}"}}
    ]}}`
	// Name map comes from the caller (the stored roster cache), NOT a chat.members
	// call — fakeCLI errors if /members is hit.
	names := map[string]string{"ou_1": "叶子", "ou_2": "志伟"}
	client := newClientWithRunner(fakeCLI(msgs), "self")
	syncer := NewSyncer(s, client)

	res, err := syncer.SyncChat(context.Background(), "oc_x", names)
	if err != nil {
		t.Fatalf("SyncChat: %v", err)
	}
	if res.Fetched != 2 || res.Inserted != 2 || res.Watermark != 1700000002000 {
		t.Fatalf("unexpected result: %+v", res)
	}
	got, _ := s.ListMessages(Channel, "oc_x", 0, 0)
	if got[0].SenderName != "叶子" || got[1].SenderName != "志伟" {
		t.Fatalf("names not enriched from map: %q %q", got[0].SenderName, got[1].SenderName)
	}
	if got[0].SenderTenantKey != "tnt_a" || got[1].SenderTenantKey != "tnt_b" {
		t.Fatalf("sender tenant_key not parsed/stored: %q %q", got[0].SenderTenantKey, got[1].SenderTenantKey)
	}
	if got[1].Title != "my project" {
		t.Fatalf("post title not extracted: %q", got[1].Title)
	}
	// Returned senders carry open_id + tenant_key for incremental ingestion.
	if len(res.Senders) != 2 {
		t.Fatalf("expected 2 distinct senders, got %d: %+v", len(res.Senders), res.Senders)
	}
	byID := map[string]string{}
	for _, sr := range res.Senders {
		byID[sr.OpenID] = sr.TenantKey
	}
	if byID["ou_1"] != "tnt_a" || byID["ou_2"] != "tnt_b" {
		t.Fatalf("sender refs missing tenant_key: %+v", res.Senders)
	}
	// Second run: same batch, nothing new, watermark stable.
	res2, err := syncer.SyncChat(context.Background(), "oc_x", names)
	if err != nil {
		t.Fatalf("SyncChat 2: %v", err)
	}
	if res2.Inserted != 0 || res2.Watermark != 1700000002000 {
		t.Fatalf("second run should insert 0, keep watermark: %+v", res2)
	}
}

// TestSyncChatLive hits the real lark-cli. Gated so CI/offline runs skip it:
//
//	FEISHU_LIVE=1 FEISHU_CHAT_ID=oc_xxx go test ./internal/feishu/ -run Live -v
func TestSyncChatLive(t *testing.T) {
	if os.Getenv("FEISHU_LIVE") == "" {
		t.Skip("set FEISHU_LIVE=1 (and FEISHU_CHAT_ID) to run the live sync test")
	}
	chatID := os.Getenv("FEISHU_CHAT_ID")
	if chatID == "" {
		t.Fatal("FEISHU_CHAT_ID required for live test")
	}
	s := openTestStore(t)
	client := NewClient("", "self")
	syncer := NewSyncer(s, client)
	// Live: pull the roster once to build the name map (mirrors the digest
	// handler's first-sync path), then sync with that map.
	members, _, merr := client.FetchMembersDetailed(context.Background(), chatID)
	names := map[string]string{}
	if merr == nil {
		for _, m := range members {
			if m.Name != "" {
				names[m.OpenID] = m.Name
			}
		}
	}
	res, err := syncer.SyncChat(context.Background(), chatID, names)
	if err != nil {
		t.Fatalf("live SyncChat: %v", err)
	}
	t.Logf("live sync: fetched=%d inserted=%d watermark=%d senders=%d", res.Fetched, res.Inserted, res.Watermark, len(res.Senders))
	if res.Inserted == 0 {
		t.Fatalf("expected to sync some messages from %s", chatID)
	}
	got, _ := s.ListMessages(Channel, chatID, 0, 5)
	named := 0
	for _, m := range got {
		if m.SenderName != "" {
			named++
		}
		t.Logf("  [%d] %s %s: %.40s", m.CreateTime, m.SenderName, m.MsgType, m.Content)
	}
	if named == 0 {
		t.Fatalf("no sender names resolved — roster name-map enrichment broken")
	}
}
