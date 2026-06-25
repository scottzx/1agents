package digest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/feishu"
	"github.com/scottzx/1Agents/backend/internal/meta"
)

func TestPrepareBatchAndCreateTask(t *testing.T) {
	dir := t.TempDir()
	mdb, err := meta.Open(filepath.Join(dir, "meta.db"))
	if err != nil {
		t.Fatalf("meta open: %v", err)
	}
	defer mdb.Close()
	ds := meta.NewDigestStore(mdb)
	if err := Seed(ds); err != nil {
		t.Fatalf("seed: %v", err)
	}
	ts := meta.NewTaskStore(mdb)

	fs, err := feishu.Open(filepath.Join(dir, "sync.db"))
	if err != nil {
		t.Fatalf("feishu open: %v", err)
	}
	defer fs.Close()
	if _, err := fs.UpsertMessages([]feishu.Message{
		{Channel: feishu.Channel, MessageID: "om_1", SessionID: "oc_x", SenderName: "叶子", MsgType: "text", Content: `{"text":"今晚分享会"}`, CreateTime: 1782388560000},
		{Channel: feishu.Channel, MessageID: "om_2", SessionID: "oc_x", SenderName: "孙傲然", MsgType: "text", Content: `{"text":"AInvestor 投研 Agent，找合伙人"}`, CreateTime: 1782388620000},
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	prompt, n, err := PrepareBatch(fs, ds, feishu.Channel, "oc_x", "测试群", 0)
	if err != nil || n != 2 {
		t.Fatalf("PrepareBatch: n=%d err=%v", n, err)
	}
	for _, want := range []string{"通用社群", "今晚分享会", "AInvestor 投研 Agent", "群「测试群」"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q", want)
		}
	}

	ws := t.TempDir()
	task, err := CreateAnalysisTask(ts, ws, "测试群", prompt)
	if err != nil {
		t.Fatalf("CreateAnalysisTask: %v", err)
	}
	// Scheduler-eligibility-relevant attributes.
	if task.Status != meta.TaskStatusPending {
		t.Fatalf("status: %q", task.Status)
	}
	if task.Type != meta.TaskTypeTask {
		t.Fatalf("type: %q", task.Type)
	}
	if task.ScheduleType != meta.ScheduleTypeImmediate {
		t.Fatalf("schedule: %q", task.ScheduleType)
	}
	if task.Assignee != "" {
		t.Fatalf("assignee should be empty (not a user todo): %q", task.Assignee)
	}
	if task.Description != prompt {
		t.Fatalf("description != prompt")
	}
	if task.Number == 0 {
		t.Fatalf("short number not assigned")
	}
	// Round-trips from the store.
	got, ok, err := ts.GetTask(task.ID)
	if err != nil || !ok || got.Description != prompt {
		t.Fatalf("reload: ok=%v err=%v", ok, err)
	}
}

// TestLivePipeline runs the whole chain on the real group: sync → seed → bind a
// preset → prepare the batch, and prints the actual agent prompt head. Gated:
//
//	FEISHU_LIVE=1 FEISHU_CHAT_ID=oc_xxx go test ./internal/digest/ -run LivePipeline -v
func TestLivePipeline(t *testing.T) {
	if os.Getenv("FEISHU_LIVE") == "" {
		t.Skip("set FEISHU_LIVE=1 (and FEISHU_CHAT_ID) to run the live pipeline test")
	}
	chatID := os.Getenv("FEISHU_CHAT_ID")
	if chatID == "" {
		t.Fatal("FEISHU_CHAT_ID required")
	}
	dir := t.TempDir()
	fs, err := feishu.Open(filepath.Join(dir, "sync.db"))
	if err != nil {
		t.Fatalf("feishu open: %v", err)
	}
	defer fs.Close()
	res, err := feishu.NewSyncer(fs, feishu.NewClient("", "self")).SyncChat(context.Background(), chatID)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	t.Logf("synced %d messages", res.Inserted)

	mdb, err := meta.Open(filepath.Join(dir, "meta.db"))
	if err != nil {
		t.Fatalf("meta open: %v", err)
	}
	defer mdb.Close()
	ds := meta.NewDigestStore(mdb)
	if err := Seed(ds); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Bind the 产品创业群 preset to this chat.
	if err := ds.BindTemplate(chatID, "tpl-builtin-product"); err != nil {
		t.Fatalf("bind: %v", err)
	}

	prompt, n, err := PrepareBatch(fs, ds, feishu.Channel, chatID, "AGIBuilder", 0)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if n == 0 {
		t.Fatal("no messages in batch")
	}
	if !strings.Contains(prompt, "产品创业群") {
		t.Fatalf("bound template not in prompt")
	}
	head := prompt
	if len(head) > 1600 {
		head = head[:1600]
	}
	t.Logf("batch=%d msgs, prompt=%d bytes\n----- PROMPT HEAD -----\n%s\n-----", n, len(prompt), head)
}
