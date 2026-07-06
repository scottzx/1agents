package govern

import (
	"testing"
	"time"

	"github.com/scottzx/1Agents/backend/internal/data"
	"github.com/scottzx/1Agents/backend/internal/sources"
)

// runToGold seeds bronze, cleans it to silver, then fuses to gold — the full
// pipeline a real sync drives. Returns the gold store for assertions.
func runToGold(t *testing.T) (*data.Store, GoldStats) {
	t.Helper()
	src := openStore(t)
	dst := openData(t)

	// A chat, a message @mentioning 卢宇轩 sent by ou_sender1, and a second message
	// in the same chat from the mentioned user — so both OpenIDs appear as senders
	// and mentions.
	seedBronze(t, src, sources.VendorFeishu, kindFeishuChat, "feishu_chat", "oc_chat1", feishuChatPayload)
	seedBronze(t, src, sources.VendorFeishu, kindFeishuMessage, "oc_chat1", "om_msg1", feishuMsgPayload)
	seedBronze(t, src, sources.VendorFeishu, kindFeishuMessage, "oc_chat1", "om_msg2",
		`{"body":{"content":"{\"text\":\"在这\"}"},"chat_id":"oc_chat1","create_time":"1782380560000",`+
			`"message_id":"om_msg2","msg_type":"text","sender":{"id":"ou_mentioned1","tenant_key":"tk2"}}`)

	if _, err := Silver(src, dst); err != nil {
		t.Fatalf("Silver: %v", err)
	}
	st, err := Gold(dst)
	if err != nil {
		t.Fatalf("Gold: %v", err)
	}
	return dst, st
}

// TestGoldFeishuFusion asserts the core issue #400 behavior: every 飞书 OpenID
// (sender + @mention) resolves to a contact via contact_channels, messages fuse
// into a titled thread, and the @-ed person lands in message_participants.
func TestGoldFeishuFusion(t *testing.T) {
	dst, st := runToGold(t)
	db := dst.SQL()

	// Two distinct OpenIDs (ou_sender1, ou_mentioned1) → two degree-2 contacts,
	// each with a feishu channel keyed on its open_id.
	if st.Contacts != 2 {
		t.Errorf("new contacts = %d, want 2", st.Contacts)
	}
	for _, openID := range []string{"ou_sender1", "ou_mentioned1"} {
		var contactID string
		if err := db.QueryRow(`SELECT contact_id FROM contact_channels
            WHERE platform='feishu' AND address=?`, openID).Scan(&contactID); err != nil {
			t.Fatalf("channel for %s missing: %v", openID, err)
		}
		if contactID == "" {
			t.Errorf("channel %s not linked to a contact", openID)
		}
		var degree int
		db.QueryRow(`SELECT degree FROM contacts WHERE id=?`, contactID).Scan(&degree)
		if degree != 2 {
			t.Errorf("contact for %s degree = %d, want 2", openID, degree)
		}
	}

	// The @mention carried a name; the mentioned contact is named 卢宇轩.
	var name string
	if err := db.QueryRow(`SELECT c.name FROM contacts c
        JOIN contact_channels ch ON ch.contact_id = c.id
        WHERE ch.platform='feishu' AND ch.address='ou_mentioned1'`).Scan(&name); err != nil {
		t.Fatalf("mentioned contact name: %v", err)
	}
	if name != "卢宇轩" {
		t.Errorf("mentioned contact name = %q, want 卢宇轩", name)
	}

	// Thread: one titled group thread for oc_chat1.
	var title, kind string
	if err := db.QueryRow(`SELECT title, kind FROM threads
        WHERE source='feishu' AND external_id='oc_chat1'`).Scan(&title, &kind); err != nil {
		t.Fatalf("thread missing: %v", err)
	}
	if title != "AI Builders" || kind != "group" {
		t.Errorf("thread = title %q kind %q, want 'AI Builders'/group", title, kind)
	}

	// Message om_msg1: sender linked, and the @-ed person is a 'mention' participant
	// resolving to the ou_mentioned1 contact.
	var senderContact string
	if err := db.QueryRow(`SELECT sender_contact_id FROM messages
        WHERE external_id='om_msg1'`).Scan(&senderContact); err != nil {
		t.Fatalf("gold message missing: %v", err)
	}
	var wantSender string
	db.QueryRow(`SELECT contact_id FROM contact_channels WHERE address='ou_sender1'`).Scan(&wantSender)
	if senderContact != wantSender {
		t.Errorf("sender_contact_id = %q, want %q", senderContact, wantSender)
	}

	var mentionContact string
	if err := db.QueryRow(`SELECT mp.contact_id FROM message_participants mp
        WHERE mp.message_id='feishu:om_msg1' AND mp.role='mention'`).Scan(&mentionContact); err != nil {
		t.Fatalf("mention participant missing: %v", err)
	}
	var wantMention string
	db.QueryRow(`SELECT contact_id FROM contact_channels WHERE address='ou_mentioned1'`).Scan(&wantMention)
	if mentionContact != wantMention {
		t.Errorf("@mention linked to %q, want ou_mentioned1's contact %q", mentionContact, wantMention)
	}

	// Fingerprint is populated for v2 dedup.
	var fp string
	db.QueryRow(`SELECT fingerprint FROM messages WHERE external_id='om_msg1'`).Scan(&fp)
	if fp == "" {
		t.Error("message fingerprint empty")
	}
}

// TestGoldIdempotent proves a re-fuse converges: no cursor advance re-processing,
// and a full re-run (cursor reset) neither duplicates messages/participants nor
// re-creates contacts.
func TestGoldIdempotent(t *testing.T) {
	dst, _ := runToGold(t)
	db := dst.SQL()

	countAll := func() (msgs, parts, contacts int) {
		db.QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&msgs)
		db.QueryRow(`SELECT COUNT(*) FROM message_participants`).Scan(&parts)
		db.QueryRow(`SELECT COUNT(*) FROM contacts`).Scan(&contacts)
		return
	}
	m0, p0, c0 := countAll()

	// Re-run with the cursor already advanced → a no-op (nothing new in silver).
	if st, err := Gold(dst); err != nil || st.Messages != 0 {
		t.Fatalf("no-op re-run = (%+v, %v), want 0 messages", st, err)
	}

	// Force a full re-fuse by resetting the cursor; counts must not grow.
	if err := dst.SaveGovernCursor(data.StageGold, sources.VendorFeishu, kindFeishuMessage, 0); err != nil {
		t.Fatalf("reset cursor: %v", err)
	}
	if _, err := Gold(dst); err != nil {
		t.Fatalf("full re-fuse: %v", err)
	}
	m1, p1, c1 := countAll()
	if m1 != m0 || p1 != p0 || c1 != c0 {
		t.Errorf("re-fuse not idempotent: messages %d→%d, participants %d→%d, contacts %d→%d",
			m0, m1, p0, p1, c0, c1)
	}
}

// TestGoldIncrementalCursor proves only silver rows past the StageGold cursor are
// fused on the next run.
func TestGoldIncrementalCursor(t *testing.T) {
	src := openStore(t)
	dst := openData(t)

	seedBronze(t, src, sources.VendorFeishu, kindFeishuChat, "feishu_chat", "oc_chat1", feishuChatPayload)
	seedBronze(t, src, sources.VendorFeishu, kindFeishuMessage, "oc_chat1", "om_msg1", feishuMsgPayload)
	if _, err := Silver(src, dst); err != nil {
		t.Fatalf("Silver: %v", err)
	}
	if st, err := Gold(dst); err != nil || st.Messages != 1 {
		t.Fatalf("first gold = (%+v, %v), want 1 message", st, err)
	}

	// A second message arrives later (silver.updated_at is bronze wall-clock ms).
	time.Sleep(2 * time.Millisecond)
	seedBronze(t, src, sources.VendorFeishu, kindFeishuMessage, "oc_chat1", "om_msg2",
		`{"body":{"content":"{\"text\":\"在这\"}"},"chat_id":"oc_chat1","create_time":"1782380560000",`+
			`"message_id":"om_msg2","msg_type":"text","sender":{"id":"ou_mentioned1","tenant_key":"tk2"}}`)
	if _, err := Silver(src, dst); err != nil {
		t.Fatalf("Silver 2: %v", err)
	}
	if st, err := Gold(dst); err != nil || st.Messages != 1 {
		t.Fatalf("incremental gold = (%+v, %v), want exactly 1 new message", st, err)
	}

	var total int
	dst.SQL().QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&total)
	if total != 2 {
		t.Fatalf("gold messages total = %d, want 2", total)
	}
}
