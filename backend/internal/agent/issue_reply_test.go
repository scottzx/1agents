package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestWriteUserReplyAppendsToTimeline verifies the user side of the issue-model
// write-back: a prompt flowing through the bridge is recorded to the task
// timeline as a user reply grouped under the session branch (SessionRef).
func TestWriteUserReplyAppendsToTimeline(t *testing.T) {
	h, _ := newTestHandler(t)
	ws := t.TempDir()
	task := seedTask(t, h, ws)

	bridge := &ActiveBridge{SessionID: "sess-1", TaskID: task.ID}

	// No-ops: empty/whitespace prompt and a task-less bridge must not write.
	writeUserReply(bridge, h.tasksStore, "   ")
	writeUserReply(&ActiveBridge{SessionID: "x", TaskID: ""}, h.tasksStore, "ignored")

	// A real prompt is recorded as a user reply under the session branch.
	writeUserReply(bridge, h.tasksStore, "可以，看下是啥情况")

	got, ok, err := h.tasksStore.GetTask(task.ID)
	if err != nil || !ok {
		t.Fatalf("GetTask(%s): ok=%v err=%v", task.ID, ok, err)
	}
	if len(got.Replies) != 1 {
		t.Fatalf("want exactly 1 reply (no-ops skipped), got %d: %+v", len(got.Replies), got.Replies)
	}
	rp := got.Replies[0]
	if rp.Author.Kind != "user" {
		t.Errorf("Author.Kind = %q, want user", rp.Author.Kind)
	}
	if rp.Text != "可以，看下是啥情况" {
		t.Errorf("Text = %q, want the prompt text", rp.Text)
	}
	if rp.SessionRef != "sess-1" {
		t.Errorf("SessionRef = %q, want sess-1 (so it threads under the branch)", rp.SessionRef)
	}
	if rp.Mode != ModeFollowUp {
		t.Errorf("Mode = %q, want follow_up", rp.Mode)
	}
}

// TestReplyCreateAcceptsSessionRef verifies the reply-create endpoint persists
// an explicit sessionRef, so an inline branch follow-up is grouped at creation
// time rather than only on the chat WS connect.
func TestReplyCreateAcceptsSessionRef(t *testing.T) {
	h, _ := newTestHandler(t)
	ws := t.TempDir()
	seedTask(t, h, ws)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agent/tasks/task-1/replies",
		strings.NewReader(`{"text":"追问","mode":"follow_up","sessionRef":"sess-9"}`))
	h.HandleTasksItem(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("reply status %d: %s", rr.Code, rr.Body.String())
	}
	var reply Reply
	if err := json.NewDecoder(rr.Body).Decode(&reply); err != nil {
		t.Fatalf("decode reply: %v", err)
	}
	if reply.SessionRef != "sess-9" {
		t.Fatalf("SessionRef = %q, want sess-9", reply.SessionRef)
	}
}
