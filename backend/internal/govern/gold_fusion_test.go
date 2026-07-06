package govern

import (
	"testing"

	"github.com/scottzx/1Agents/backend/internal/sources"
)

// gold_fusion_test.go covers the v2 cross-source fusion: an iCloud address book as
// the degree-1 hub, deterministic email merge across mail sources, the merge
// primitive folding a discovered degree-2 contact, and calendar-event fusion.

func icloudVCardWith(uid, fn, tel, email string) string {
	return "BEGIN:VCARD\nVERSION:3.0\nUID:" + uid + "\nFN:" + fn +
		"\nTEL:" + tel + "\nEMAIL:" + email + "\nEND:VCARD"
}

const (
	// MS mail + AgentMail both FROM the same person — same email = same contact.
	msMailFromPerson = `{"id":"ms_p1","receivedDateTime":"2026-07-04T03:00:00Z","subject":"MS hi",` +
		`"bodyPreview":"hello","from":{"emailAddress":{"name":"Person","address":"person@corp.com"}},` +
		`"toRecipients":[{"emailAddress":{"name":"Me","address":"me@corp.com"}}]}`
	agentMailFromPerson = `{"created_at":"2026-07-04T04:00:00Z","message_id":"am_p1",` +
		`"from":{"email":"person@corp.com","name":"Person"},"dir":{"dir_name":"inbox"},` +
		`"snippet":"hey","subject":"AM hi","to":[{"email":"me@corp.com","name":"Me"}]}`
)

// TestGoldCrossSourceContactHub: an iCloud contact carrying an email+phone becomes
// the degree-1 hub, and the same person's MS mail + AgentMail both resolve to it —
// one person, not three.
func TestGoldCrossSourceContactHub(t *testing.T) {
	src := openStore(t)
	dst := openData(t)

	seedBronze(t, src, sources.SourceICloud, sources.KindContact, "/b/", "/b/cm",
		icloudVCardWith("cm", "Person", "+8613900000000", "person@corp.com"))
	seedBronze(t, src, sources.VendorMicrosoft, kindMSMail, "inbox", "ms_p1", msMailFromPerson)
	seedBronze(t, src, sources.VendorAgentMail, kindAgentMail, "inbox", "am_p1", agentMailFromPerson)

	if _, err := Silver(src, dst); err != nil {
		t.Fatalf("Silver: %v", err)
	}
	if _, err := Gold(dst); err != nil {
		t.Fatalf("Gold: %v", err)
	}
	db := dst.SQL()

	var contactID string
	var degree int
	if err := db.QueryRow(`SELECT c.id, c.degree FROM contacts c
        JOIN contact_channels ch ON ch.contact_id = c.id
        WHERE ch.platform='email' AND ch.address='person@corp.com'`).Scan(&contactID, &degree); err != nil {
		t.Fatalf("email channel → contact: %v", err)
	}
	if degree != 1 {
		t.Errorf("hub contact degree = %d, want 1 (address-book)", degree)
	}
	var platforms int
	db.QueryRow(`SELECT COUNT(DISTINCT platform) FROM contact_channels WHERE contact_id=?`, contactID).Scan(&platforms)
	if platforms != 3 {
		t.Errorf("distinct channel platforms = %d, want 3 (icloud/email/phone)", platforms)
	}
	var senders int
	db.QueryRow(`SELECT COUNT(*) FROM messages WHERE sender_contact_id=? AND msg_kind='email'`, contactID).Scan(&senders)
	if senders != 2 {
		t.Errorf("email messages by hub contact = %d, want 2 (MS + AgentMail)", senders)
	}
	var dupes int
	db.QueryRow(`SELECT COUNT(*) FROM contact_channels WHERE platform='email' AND address='person@corp.com'`).Scan(&dupes)
	if dupes != 1 {
		t.Errorf("email channels for person@corp.com = %d, want 1 (no duplicate person)", dupes)
	}
}

// TestGoldIcloudFoldsDiscoveredContact: a mail arriving first creates a bare
// degree-2 contact; when the address book later lands on the same email, the
// degree-2 is merged into the degree-1 hub (and message references repointed).
func TestGoldIcloudFoldsDiscoveredContact(t *testing.T) {
	src := openStore(t)
	dst := openData(t)

	seedBronze(t, src, sources.VendorMicrosoft, kindMSMail, "inbox", "ms_p1", msMailFromPerson)
	if _, err := Silver(src, dst); err != nil {
		t.Fatalf("Silver1: %v", err)
	}
	if _, err := Gold(dst); err != nil {
		t.Fatalf("Gold1: %v", err)
	}
	db := dst.SQL()

	var preDegree int
	var oldID string
	if err := db.QueryRow(`SELECT c.id, c.degree FROM contacts c
        JOIN contact_channels ch ON ch.contact_id = c.id
        WHERE ch.platform='email' AND ch.address='person@corp.com'`).Scan(&oldID, &preDegree); err != nil {
		t.Fatalf("pre-iCloud contact: %v", err)
	}
	if preDegree != 2 {
		t.Fatalf("pre-iCloud degree = %d, want 2 (discovered)", preDegree)
	}

	seedBronze(t, src, sources.SourceICloud, sources.KindContact, "/b/", "/b/cm",
		icloudVCardWith("cm", "Person", "+8613900000000", "person@corp.com"))
	if _, err := Silver(src, dst); err != nil {
		t.Fatalf("Silver2: %v", err)
	}
	if _, err := Gold(dst); err != nil {
		t.Fatalf("Gold2: %v", err)
	}

	var n int
	db.QueryRow(`SELECT COUNT(*) FROM contact_channels WHERE platform='email' AND address='person@corp.com'`).Scan(&n)
	if n != 1 {
		t.Errorf("email channels = %d, want 1 (merged)", n)
	}
	var newID string
	var postDegree int
	db.QueryRow(`SELECT c.id, c.degree FROM contacts c
        JOIN contact_channels ch ON ch.contact_id = c.id
        WHERE ch.platform='email' AND ch.address='person@corp.com'`).Scan(&newID, &postDegree)
	if postDegree != 1 {
		t.Errorf("post-iCloud degree = %d, want 1 (hub)", postDegree)
	}
	var msgSender string
	db.QueryRow(`SELECT sender_contact_id FROM messages WHERE external_id='ms_p1'`).Scan(&msgSender)
	if msgSender != newID {
		t.Errorf("message sender not repointed to hub: got %s want %s", msgSender, newID)
	}
	if oldID != newID {
		var stale int
		db.QueryRow(`SELECT COUNT(*) FROM contacts WHERE id=?`, oldID).Scan(&stale)
		if stale != 0 {
			t.Errorf("stale degree-2 contact %s survived merge", oldID)
		}
	}
}

// TestGoldMicrosoftEventFusion: an MS event fuses into calendar_events with its
// organizer resolved to a contact and its attendee recorded.
func TestGoldMicrosoftEventFusion(t *testing.T) {
	src := openStore(t)
	dst := openData(t)

	seedBronze(t, src, sources.VendorMicrosoft, kindMSEvent, "cal", "ms_event1", msEventPayload)
	if _, err := Silver(src, dst); err != nil {
		t.Fatalf("Silver: %v", err)
	}
	st, err := Gold(dst)
	if err != nil {
		t.Fatalf("Gold: %v", err)
	}
	if st.Events < 1 {
		t.Errorf("Events = %d, want >= 1", st.Events)
	}
	db := dst.SQL()

	var title, organizer string
	var attendees int
	if err := db.QueryRow(`SELECT e.title, COALESCE(oc.name, ''),
        (SELECT COUNT(*) FROM event_attendees a WHERE a.event_id = e.id)
        FROM calendar_events e LEFT JOIN contacts oc ON oc.id = e.organizer_contact_id
        WHERE e.source='microsoft' AND e.external_id='ms_event1'`).Scan(&title, &organizer, &attendees); err != nil {
		t.Fatalf("event row: %v", err)
	}
	if title == "" {
		t.Error("event title empty")
	}
	if organizer != "Org" {
		t.Errorf("organizer = %q, want Org", organizer)
	}
	if attendees != 1 {
		t.Errorf("attendees = %d, want 1", attendees)
	}
}
