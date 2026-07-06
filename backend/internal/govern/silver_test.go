package govern

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scottzx/1Agents/backend/internal/data"
	"github.com/scottzx/1Agents/backend/internal/sources"
)

func openData(t *testing.T) *data.Store {
	t.Helper()
	st, err := data.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open data.db: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func seedBronze(t *testing.T, st *sources.Store, source, kind, coll, uid, payload string) {
	t.Helper()
	if _, err := st.CommitPage(source, "default", []sources.RawRecord{
		{Kind: kind, Collection: coll, UID: uid, ETag: uid, ContentType: "application/json", Payload: payload},
	}, sources.Cursor{Kind: "timestamp", Value: "1"}); err != nil {
		t.Fatalf("seed bronze %s/%s: %v", source, kind, err)
	}
}

// Real-shaped (trimmed) payloads from an actual sync.db.
const (
	// A 飞书 message that @mentions a user and is a reply (parent/root present).
	feishuMsgPayload = `{"body":{"content":"{\"text\":\"@_user_1 <p>看录屏在哪</p>\"}"},` +
		`"chat_id":"oc_chat1","create_time":"1782380558893","deleted":false,` +
		`"message_id":"om_msg1","msg_type":"text","parent_id":"om_parent","root_id":"om_root",` +
		`"sender":{"id":"ou_sender1","tenant_key":"tk1"},` +
		`"mentions":[{"id":"ou_mentioned1","key":"@_user_1","name":"卢宇轩","tenant_key":"tk2"}]}`

	msMailPayload = `{"id":"ms_mail1","receivedDateTime":"2026-07-03T03:00:57Z","subject":"Security code",` +
		`"bodyPreview":"code 123","isRead":true,"webLink":"https://outlook/x",` +
		`"from":{"emailAddress":{"name":"MS Team","address":"noreply@microsoft.com"}},` +
		`"toRecipients":[{"emailAddress":{"name":"Me","address":"me@outlook.com"}}]}`

	agentMailPayload = `{"created_at":"2026-07-03T15:46:57Z","message_id":"msg_am1",` +
		`"from":{"email":"admin@agent.qq.com","name":"Agent Mail 团队"},"has_attachments":false,` +
		`"is_read":false,"dir":{"dir_name":"inbox"},"snippet":"接入成功","subject":"接入成功",` +
		`"to":[{"email":"x@agent.qq.com","name":"x"}]}`

	msEventPayload = `{"id":"ms_event1","subject":"ROG 掌机","isAllDay":false,"showAs":"busy",` +
		`"start":{"dateTime":"2026-07-10T09:00:00.0000000","timeZone":"UTC"},` +
		`"end":{"dateTime":"2026-07-10T10:00:00.0000000","timeZone":"UTC"},` +
		`"location":{"displayName":"Room A"},"body":{"content":"<p>agenda</p>"},` +
		`"organizer":{"emailAddress":{"name":"Org","address":"org@x.com"}},` +
		`"attendees":[{"emailAddress":{"name":"A","address":"a@x.com"},"status":{"response":"accepted"}}]}`

	msTodoPayload = `{"id":"ms_todo1","importance":"normal","status":"completed","title":"ai记事本",` +
		`"body":{"content":"note body","contentType":"text"},"hasAttachments":false,` +
		`"completedDateTime":{"dateTime":"2026-06-18T00:00:00.0000000","timeZone":"UTC"},` +
		`"checklistItems":[{"displayName":"step1"}],"recurrence":{"pattern":{"type":"daily"}}}`

	// vCard with BDAY + NICKNAME + NOTE — the fields the lean cleaner used to drop.
	icloudVCard = "BEGIN:VCARD\nVERSION:3.0\nUID:c1\nFN:Zhang San\nN:Zhang;San;;;\n" +
		"TEL:+8613800000000\nEMAIL:zhang@example.com\nORG:Acme;Eng\nTITLE:CEO\n" +
		"BDAY:1990-05-15\nNICKNAME:Z\nNOTE:vip client\nEND:VCARD"

	feishuChatPayload = `{"chat_id":"oc_chat1","name":"AI Builders","chat_mode":"group","external":true,` +
		`"owner_id":"ou_owner1","tenant_key":"tk1","description":"demo day"}`
)

// TestSilverAllDomains seeds one bronze record per source and asserts per-source
// silver cleaning is lossless for the fields that were previously dropped.
func TestSilverAllDomains(t *testing.T) {
	src := openStore(t)
	dst := openData(t)

	seedBronze(t, src, sources.VendorFeishu, kindFeishuMessage, "oc_chat1", "om_msg1", feishuMsgPayload)
	seedBronze(t, src, sources.VendorFeishu, kindFeishuChat, "feishu_chat", "oc_chat1", feishuChatPayload)
	seedBronze(t, src, sources.VendorMicrosoft, kindMSMail, "inbox", "ms_mail1", msMailPayload)
	seedBronze(t, src, sources.VendorAgentMail, kindAgentMail, "inbox", "msg_am1", agentMailPayload)
	seedBronze(t, src, sources.VendorMicrosoft, kindMSEvent, "cal1", "ms_event1", msEventPayload)
	seedBronze(t, src, sources.VendorMicrosoft, kindMSTodo, "list1", "ms_todo1", msTodoPayload)
	seedBronze(t, src, sources.SourceICloud, sources.KindContact, "/b/", "/b/c1", icloudVCard)

	if _, err := Silver(src, dst); err != nil {
		t.Fatalf("Silver: %v", err)
	}
	db := dst.SQL()

	// iCloud contact: birthday/nickname/note promoted to columns (previously lost).
	var bday, nick, note string
	if err := db.QueryRow(`SELECT birthday, nickname, note FROM silver_icloud_contacts
        WHERE external_id='/b/c1'`).Scan(&bday, &nick, &note); err != nil {
		t.Fatalf("scan icloud contact: %v", err)
	}
	if bday != "1990-05-15" || nick != "Z" || note != "vip client" {
		t.Errorf("lossy contact: bday=%q nick=%q note=%q", bday, nick, note)
	}

	// 飞书 message: @mention OpenID + name captured, reply chain preserved.
	var mentions, parent, root string
	if err := db.QueryRow(`SELECT mentions, parent_id, root_id FROM silver_feishu_messages
        WHERE external_id='om_msg1'`).Scan(&mentions, &parent, &root); err != nil {
		t.Fatalf("scan feishu message: %v", err)
	}
	if !strings.Contains(mentions, "ou_mentioned1") || !strings.Contains(mentions, "卢宇轩") {
		t.Errorf("mention OpenID/name not captured: %s", mentions)
	}
	if parent != "om_parent" || root != "om_root" {
		t.Errorf("reply chain lost: parent=%q root=%q", parent, root)
	}

	// 飞书 二级用户: both the sender and the @mentioned user exist; mention carries a name.
	var senderVia, mentionedName, mentionedVia string
	if err := db.QueryRow(`SELECT discovered_via FROM silver_feishu_users WHERE external_id='ou_sender1'`).
		Scan(&senderVia); err != nil {
		t.Fatalf("sender not extracted as 二级用户: %v", err)
	}
	if !strings.Contains(senderVia, "sender") {
		t.Errorf("sender discovered_via = %s", senderVia)
	}
	if err := db.QueryRow(`SELECT name, discovered_via FROM silver_feishu_users WHERE external_id='ou_mentioned1'`).
		Scan(&mentionedName, &mentionedVia); err != nil {
		t.Fatalf("mentioned user not extracted: %v", err)
	}
	if mentionedName != "卢宇轩" || !strings.Contains(mentionedVia, "mention") {
		t.Errorf("mentioned user = name %q via %s", mentionedName, mentionedVia)
	}

	// MS todo: recurrence + checklist preserved.
	var recurrence, checklist string
	if err := db.QueryRow(`SELECT recurrence, checklist_items FROM silver_microsoft_todos
        WHERE external_id='ms_todo1'`).Scan(&recurrence, &checklist); err != nil {
		t.Fatalf("scan ms todo: %v", err)
	}
	if !strings.Contains(recurrence, "daily") || !strings.Contains(checklist, "step1") {
		t.Errorf("todo lost recurrence/checklist: rec=%s checklist=%s", recurrence, checklist)
	}

	// MS mail (isRead/webLink) + event (body) + agentmail land in their own tables.
	var isRead int
	db.QueryRow(`SELECT is_read FROM silver_microsoft_mail WHERE external_id='ms_mail1'`).Scan(&isRead)
	if isRead != 1 {
		t.Errorf("ms mail is_read not preserved")
	}
	var evBody string
	db.QueryRow(`SELECT body FROM silver_microsoft_events WHERE external_id='ms_event1'`).Scan(&evBody)
	if evBody != "agenda" {
		t.Errorf("event body = %q", evBody)
	}
}

// TestSilverIncrementalCursor proves the StageSilver cursor makes an unchanged
// re-run a no-op, while a fresh bronze record on the next run is picked up.
func TestSilverIncrementalCursor(t *testing.T) {
	src := openStore(t)
	dst := openData(t)

	seedBronze(t, src, sources.VendorMicrosoft, kindMSMail, "inbox", "ms_mail1", msMailPayload)
	if n, err := SilverMicrosoftMail(src, dst); err != nil || n != 1 {
		t.Fatalf("first run = (%d, %v), want (1, nil)", n, err)
	}
	if n, err := SilverMicrosoftMail(src, dst); err != nil || n != 0 {
		t.Fatalf("unchanged re-run = (%d, %v), want (0, nil)", n, err)
	}
	// Bronze fetched_at is millisecond wall-clock and the cursor window is strict
	// (>); a real second sync is seconds later, so nudge past the current ms.
	time.Sleep(2 * time.Millisecond)
	seedBronze(t, src, sources.VendorMicrosoft, kindMSMail, "inbox", "ms_mail2",
		`{"id":"ms_mail2","receivedDateTime":"2026-07-04T00:00:00Z","subject":"Second",`+
			`"from":{"emailAddress":{"address":"a@b.com"}},"toRecipients":[]}`)
	if n, err := SilverMicrosoftMail(src, dst); err != nil || n != 1 {
		t.Fatalf("incremental run = (%d, %v), want (1, nil)", n, err)
	}

	var total int
	dst.SQL().QueryRow(`SELECT COUNT(*) FROM silver_microsoft_mail`).Scan(&total)
	if total != 2 {
		t.Fatalf("silver_microsoft_mail total = %d, want 2", total)
	}
}
