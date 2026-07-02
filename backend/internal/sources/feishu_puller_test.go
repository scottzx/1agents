package sources

import (
	"context"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/feishu"
)

func TestExtractItemsAndFields(t *testing.T) {
	raw := []byte(`{"code":0,"data":{"items":[{"chat_id":"oc_1","name":"A"},{"chat_id":"oc_2"}]}}`)
	items, err := extractItems(raw, "data.items")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	if got := fieldString(items[0], "chat_id"); got != "oc_1" {
		t.Errorf("uid = %q, want oc_1", got)
	}
	// Missing path → no items, no error.
	none, err := extractItems([]byte(`{"data":{}}`), "data.items")
	if err != nil || len(none) != 0 {
		t.Errorf("missing path: got %v items err=%v", len(none), err)
	}
}

func TestItemEpochSecMs(t *testing.T) {
	item := []byte(`{"create_time":"1700000000123"}`)
	if got := itemEpochSec(item, "create_time", true); got != 1700000000 {
		t.Errorf("ms→s = %d, want 1700000000", got)
	}
	if got := itemEpochSec(item, "create_time", false); got != 1700000000123 {
		t.Errorf("sec = %d, want raw", got)
	}
}

// TestFeishuPullerPullChats drives a page_token collection (feishu_chat) end to
// end through an injected CLIRunner: the raw items land as RawRecords with a
// content-hash ETag and no cursor.
func TestFeishuPullerPullChats(t *testing.T) {
	run := func(ctx context.Context, args ...string) ([]byte, error) {
		return []byte(`{"code":0,"data":{"items":[{"chat_id":"oc_1","name":"群A"},{"chat_id":"oc_2","name":"群B"}]}}`), nil
	}
	client := feishu.NewClientWithRunner(run, "default")
	p := NewFeishuPuller(client, []FeishuSpec{{Kind: "feishu_chat", PageSize: 50}})

	colls, err := p.Discover("default")
	if err != nil {
		t.Fatal(err)
	}
	if len(colls) != 1 || colls[0].Kind != "feishu_chat" {
		t.Fatalf("discover = %+v", colls)
	}

	recs, next, done, err := p.Pull("default", colls[0], Cursor{})
	if err != nil {
		t.Fatal(err)
	}
	if !done {
		t.Errorf("expected done=true (RawAPI aggregates all pages)")
	}
	if len(recs) != 2 {
		t.Fatalf("recs = %d, want 2", len(recs))
	}
	if recs[0].UID != "oc_1" || recs[0].ContentType != "application/json" || recs[0].ETag == "" {
		t.Errorf("bad record: %+v", recs[0])
	}
	if next.Kind != "page_token" || next.Value != "" {
		t.Errorf("page_token cursor should be empty, got %+v", next)
	}
}

// TestFeishuPullerPullMessages exercises the timestamp flavor: the watermark
// advances to the newest create_time (ms→s).
func TestFeishuPullerPullMessages(t *testing.T) {
	run := func(ctx context.Context, args ...string) ([]byte, error) {
		return []byte(`{"code":0,"data":{"items":[
			{"message_id":"om_1","create_time":"1700000000000"},
			{"message_id":"om_2","create_time":"1700000600000"}
		]}}`), nil
	}
	client := feishu.NewClientWithRunner(run, "default")
	p := NewFeishuPuller(client, []FeishuSpec{{Kind: "feishu_message", PageSize: 50, ChatIDs: []string{"oc_1"}}})

	colls, _ := p.Discover("default")
	if len(colls) != 1 || colls[0].ID != "oc_1" {
		t.Fatalf("per-chat discover = %+v", colls)
	}
	recs, next, _, err := p.Pull("default", colls[0], Cursor{})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("recs = %d", len(recs))
	}
	if next.Kind != "timestamp" || next.Value != "1700000600" {
		t.Errorf("watermark = %+v, want 1700000600s", next)
	}
}

func TestFeishuPullerAPICodeError(t *testing.T) {
	run := func(ctx context.Context, args ...string) ([]byte, error) {
		return []byte(`{"code":99991663,"msg":"token expired"}`), nil
	}
	client := feishu.NewClientWithRunner(run, "default")
	p := NewFeishuPuller(client, []FeishuSpec{{Kind: "feishu_chat"}})
	colls, _ := p.Discover("default")
	if _, _, _, err := p.Pull("default", colls[0], Cursor{}); err == nil {
		t.Errorf("expected API code error")
	}
}
