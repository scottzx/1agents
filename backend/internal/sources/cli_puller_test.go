package sources

import (
	"context"
	"testing"
)

// agentmail-shaped descriptor: envelope {ok, data:{data:[…]}}, uid message_id,
// timestamp watermark from created_at, passed back as --after.
func agentmailCLIDesc() RESTDescriptor {
	return RESTDescriptor{
		Kind:          "agentmail_mail",
		Transport:     "cli",
		Command:       "agently-cli",
		Args:          []string{"message", "+list", "--dir", "inbox", "--limit", "50"},
		SuccessPath:   "ok",
		ItemPath:      "data.data",
		UIDField:      "message_id",
		CursorFlavor:  "timestamp",
		CursorArg:     "--after",
		TimeItemField: "created_at",
	}
}

func TestCLIPuller_TimestampWatermark(t *testing.T) {
	d := agentmailCLIDesc()
	var gotCursorArg string
	p := &cliPuller{source: "agentmail", kinds: []RESTDescriptor{d}}
	p.run = func(_ context.Context, _ RESTDescriptor, cursor string) ([]byte, error) {
		gotCursorArg = cursor
		return []byte(`{"ok":true,"data":{"data":[
			{"message_id":"m1","created_at":"2026-07-01T10:00:00Z","subject":"a"},
			{"message_id":"m2","created_at":"2026-07-03T09:00:00Z","subject":"b"}
		]}}`), nil
	}

	recs, next, done, err := p.Pull("default", Collection{Kind: d.Kind, ID: d.Kind}, Cursor{})
	if err != nil || !done {
		t.Fatalf("Pull = %v done=%v", err, done)
	}
	if len(recs) != 2 || recs[0].UID != "m1" || recs[1].UID != "m2" {
		t.Fatalf("records = %+v", recs)
	}
	// Watermark advances to the newest created_at.
	if next.Kind != "timestamp" || next.Value != "2026-07-03T09:00:00Z" {
		t.Fatalf("cursor = %+v", next)
	}
	// First run passes no --after; the returned cursor drives the next run.
	if gotCursorArg != "" {
		t.Fatalf("first run should not pass a cursor, got %q", gotCursorArg)
	}

	// Second run: the watermark is passed through to the command.
	_, _, _, _ = p.Pull("default", Collection{Kind: d.Kind, ID: d.Kind}, next)
	if gotCursorArg != "2026-07-03T09:00:00Z" {
		t.Fatalf("second run cursor = %q", gotCursorArg)
	}
}

func TestCLIPuller_SuccessPathFalseErrors(t *testing.T) {
	d := agentmailCLIDesc()
	p := &cliPuller{source: "agentmail", kinds: []RESTDescriptor{d}}
	p.run = func(_ context.Context, _ RESTDescriptor, _ string) ([]byte, error) {
		return []byte(`{"ok":false,"error":"not logged in"}`), nil
	}
	if _, _, _, err := p.Pull("default", Collection{Kind: d.Kind, ID: d.Kind}, Cursor{}); err == nil {
		t.Fatal("ok=false should error (don't advance cursor)")
	}
}
