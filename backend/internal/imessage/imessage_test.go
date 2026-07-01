package imessage

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestAppleToUnixMs(t *testing.T) {
	// Nanoseconds form (recent macOS): 2001-01-01 + 700000000s ≈ 2023.
	if got := appleToUnixMs(700_000_000_000_000_000); got != 1_678_307_200_000 {
		t.Errorf("ns form: got %d", got)
	}
	// Legacy seconds form.
	if got := appleToUnixMs(700_000_000); got != 1_678_307_200_000 {
		t.Errorf("seconds form: got %d", got)
	}
	if got := appleToUnixMs(0); got != 0 {
		t.Errorf("zero should map to 0, got %d", got)
	}
}

// attrBody crafts a minimal typedstream-shaped attributedBody blob carrying text.
func attrBody(text string, longLen bool) []byte {
	b := []byte("streamtyped....NSString")
	b = append(b, 0x01, 0x94, 0x84, 0x01, 0x2B) // class chain + 0x2B string-start tag
	if longLen {
		b = append(b, 0x81, byte(len(text)), byte(len(text)>>8))
	} else {
		b = append(b, byte(len(text)))
	}
	return append(b, text...)
}

func TestDecodeAttributedBody(t *testing.T) {
	if got := decodeAttributedBody(attrBody("hello world", false)); got != "hello world" {
		t.Errorf("short: got %q", got)
	}
	long := "x23456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789ABCDEF" // >128 bytes
	if got := decodeAttributedBody(attrBody(long, true)); got != long {
		t.Errorf("0x81 length form failed: got %q", got)
	}
	if got := decodeAttributedBody(nil); got != "" {
		t.Errorf("nil should be empty, got %q", got)
	}
	if got := decodeAttributedBody([]byte("no marker here")); got != "" {
		t.Errorf("no marker should be empty, got %q", got)
	}
}

// TestReadFixture builds a synthetic chat.db with the real schema subset and
// verifies join, text resolution (text column + attributedBody fallback), and
// time conversion — exercising the full read path without Full Disk Access.
func TestReadFixture(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chat.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	schema := `
		CREATE TABLE handle (ROWID INTEGER PRIMARY KEY, id TEXT);
		CREATE TABLE chat (ROWID INTEGER PRIMARY KEY, guid TEXT);
		CREATE TABLE message (ROWID INTEGER PRIMARY KEY, guid TEXT, text TEXT,
			attributedBody BLOB, date INTEGER, is_from_me INTEGER, handle_id INTEGER,
			service TEXT, cache_has_attachments INTEGER);
		CREATE TABLE chat_message_join (chat_id INTEGER, message_id INTEGER);`
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	db.Exec(`INSERT INTO handle (ROWID, id) VALUES (1, '+8613800138000')`)
	db.Exec(`INSERT INTO chat (ROWID, guid) VALUES (1, 'iMessage;-;+8613800138000')`)
	// m1: plain text, incoming. m2: text NULL, body in attributedBody, outgoing.
	db.Exec(`INSERT INTO message (ROWID, guid, text, date, is_from_me, handle_id, service, cache_has_attachments)
		VALUES (1, 'GUID-1', 'hi there', 700000000000000000, 0, 1, 'iMessage', 0)`)
	db.Exec(`INSERT INTO message (ROWID, guid, text, attributedBody, date, is_from_me, handle_id, service, cache_has_attachments)
		VALUES (2, 'GUID-2', NULL, ?, 700000000000000001, 1, 0, 'iMessage', 0)`, attrBody("from attributed body", false))
	db.Exec(`INSERT INTO chat_message_join (chat_id, message_id) VALUES (1, 1), (1, 2)`)
	db.Close()

	msgs, maxApple, err := Read(path, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("want 2 messages, got %d", len(msgs))
	}
	if msgs[0].GUID != "GUID-1" || msgs[0].Text != "hi there" || msgs[0].Handle != "+8613800138000" || msgs[0].IsFromMe {
		t.Errorf("m1 wrong: %+v", msgs[0])
	}
	if msgs[1].Text != "from attributed body" || !msgs[1].IsFromMe {
		t.Errorf("m2 attributedBody/from-me wrong: %+v", msgs[1])
	}
	if msgs[0].ChatGUID != "iMessage;-;+8613800138000" || msgs[0].CreateTime == 0 {
		t.Errorf("chat/time wrong: %+v", msgs[0])
	}
	if maxApple != 700000000000000001 {
		t.Errorf("maxApple watermark wrong: %d", maxApple)
	}

	// Incremental: re-read from the watermark returns nothing new.
	again, _, err := Read(path, maxApple, 0)
	if err != nil || len(again) != 0 {
		t.Errorf("incremental from watermark should be empty, got %d (%v)", len(again), err)
	}
}
