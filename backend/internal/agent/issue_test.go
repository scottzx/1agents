package agent

import (
	"strings"
	"testing"
	"time"
)

func TestBuildIssueBackground(t *testing.T) {
	created := time.Date(2026, 6, 9, 14, 20, 0, 0, time.UTC)
	task := &Task{
		ID:          "task_abc123",
		Title:       "优化登录流程",
		IssueState:  IssueOpen,
		Status:      TaskStatusRunning,
		Description: "当前登录流程太慢",
		Replies: []Reply{
			{ID: "r1", Author: Author{Kind: "user", Name: "scott"}, Text: "先调研一下当前的实现", Mode: ModeNewSession, CreatedAt: created},
			{ID: "r2", Author: Author{Kind: "agent", Name: "claudecode"}, AgentType: "claudecode", SessionRef: "sess-a", Text: "调研完成\n关键发现: cookie 没设 Secure", Mode: ModePureComment, CreatedAt: created.Add(time.Hour)},
		},
	}

	got := buildIssueBackground(task, "/tmp/ws")

	for _, want := range []string{
		"=== ISSUE BACKGROUND ===",
		"Task ID: task_abc123",
		"Title: 优化登录流程",
		"Issue State: open",
		"Workflow Status: running",
		"Workspace: /tmp/ws",
		"Description:\n当前登录流程太慢",
		"Replies (chronological, 2 entries):",
		"[1] scott @ 2026-06-09T14:20:00Z",
		"    先调研一下当前的实现",
		"[2] agent (claudecode, session #sess-a) @ 2026-06-09T15:20:00Z",
		"    关键发现: cookie 没设 Secure",
		"End of background.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("background missing %q\n---\n%s", want, got)
		}
	}
}

func TestBridgeTurnTextAccumulation(t *testing.T) {
	b := &ActiveBridge{}
	b.appendTurnText("中间叙述，")
	b.appendTurnText("将被工具调用截断")
	b.resetTurnText() // tool_call resets: only text after the LAST tool call survives
	b.appendTurnText("最终结论：")
	b.appendTurnText("OAuth 已实现")
	if got := b.takeTurnText(); got != "最终结论：OAuth 已实现" {
		t.Fatalf("takeTurnText = %q", got)
	}
	if got := b.takeTurnText(); got != "" {
		t.Fatalf("second take should be empty, got %q", got)
	}
}

func TestAppendAgentReplyWritesTimeline(t *testing.T) {
	h, store := newTestHandler(t)
	ws := t.TempDir()
	seedTask(t, h, ws)

	// Chat record so the write-back can resolve the acp session id.
	if err := store.Add(ChatSessionRecord{ID: "sess-x", WorkspaceID: "ws", AcpSessionID: "uuid-x"}); err != nil {
		t.Fatalf("add session: %v", err)
	}

	
	bridge := &ActiveBridge{
		SessionID:     "sess-x",
		WorkspacePath: ws,
		TaskID:        "task-1",
		AgentType:     "claudecode",
		ReplyID:       "user-reply-1",
	}
	bridge.appendTurnText("调研完成，结论如下")
	writeAgentReply(bridge, h.tasksStore, store)

	task, ok, err := h.tasksStore.GetTask("task-1")
	if err != nil || !ok {
		t.Fatalf("GetTask: ok=%v err=%v", ok, err)
	}
	if len(task.Replies) != 1 {
		t.Fatalf("replies = %d, want 1", len(task.Replies))
	}
	rp := task.Replies[0]
	if rp.Author.Kind != "agent" || rp.Text != "调研完成，结论如下" ||
		rp.SessionRef != "sess-x" || rp.AcpSessionID != "uuid-x" || rp.InReplyTo != "user-reply-1" {
		t.Fatalf("wrong agent reply: %+v", rp)
	}

	// Empty turn → no reply appended.
	writeAgentReply(bridge, h.tasksStore, store)
	task, _, _ = h.tasksStore.GetTask("task-1")
	if len(task.Replies) != 1 {
		t.Fatalf("empty turn appended a reply: %d", len(task.Replies))
	}
}
