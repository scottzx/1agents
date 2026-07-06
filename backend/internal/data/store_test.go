package data

import (
	"path/filepath"
	"testing"
)

// openTemp opens a fresh data.db under a temp dir (no ONEAGENTS_HOME coupling).
func openTemp(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// TestSchemaCreatesAllTables asserts every silver/gold/cursor table exists after
// Open, so a fresh DB is immediately usable by the transforms.
func TestSchemaCreatesAllTables(t *testing.T) {
	st := openTemp(t)
	want := []string{
		"silver_icloud_contacts", "silver_feishu_users", "silver_feishu_messages",
		"silver_feishu_chats", "silver_microsoft_mail", "silver_agentmail_mail",
		"silver_microsoft_events", "silver_microsoft_todos",
		"contacts", "contact_channels",
		"threads", "messages", "message_participants", "message_attachments",
		"calendar_events", "event_attendees", "todos",
		"data_cursors",
	}
	for _, tbl := range want {
		var name string
		err := st.SQL().QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl).Scan(&name)
		if err != nil {
			t.Errorf("table %q missing: %v", tbl, err)
		}
	}
}

// TestOpenIdempotent asserts re-opening the same file (CREATE IF NOT EXISTS) is a
// no-op rather than an error, matching the bronze store convention.
func TestOpenIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.db")
	s1, err := Open(path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	s1.Close()
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	s2.Close()
}

// TestGovernCursorRoundTrip covers the transform watermark: unset reads 0, and a
// saved value reads back per (stage, source, kind) without cross-talk.
func TestGovernCursorRoundTrip(t *testing.T) {
	st := openTemp(t)

	if wm, err := st.GovernCursor(StageSilver, "icloud", "contact"); err != nil || wm != 0 {
		t.Fatalf("unset cursor = (%d, %v), want (0, nil)", wm, err)
	}
	if err := st.SaveGovernCursor(StageSilver, "icloud", "contact", 1234); err != nil {
		t.Fatalf("SaveGovernCursor: %v", err)
	}
	if wm, err := st.GovernCursor(StageSilver, "icloud", "contact"); err != nil || wm != 1234 {
		t.Fatalf("silver cursor = (%d, %v), want (1234, nil)", wm, err)
	}
	// Same (source, kind) under a different stage is an independent slot.
	if wm, err := st.GovernCursor(StageGold, "icloud", "contact"); err != nil || wm != 0 {
		t.Fatalf("gold cursor = (%d, %v), want (0, nil)", wm, err)
	}
	// Overwrite advances in place.
	if err := st.SaveGovernCursor(StageSilver, "icloud", "contact", 9999); err != nil {
		t.Fatalf("SaveGovernCursor overwrite: %v", err)
	}
	if wm, _ := st.GovernCursor(StageSilver, "icloud", "contact"); wm != 9999 {
		t.Fatalf("cursor after overwrite = %d, want 9999", wm)
	}
}
