package meta

import "testing"

func TestContactsCRUDAndPhoneUniqueness(t *testing.T) {
	db := newTestDB(t)
	s := NewContactStore(db)

	a, err := s.CreateContact("13800000001", "张三", "Acme", "CTO", "note", []string{"vip", "投资"})
	if err != nil {
		t.Fatalf("create a: %v", err)
	}
	if len(a.Tags) != 2 || a.Tags[0] != "vip" {
		t.Fatalf("tags not round-tripped: %+v", a.Tags)
	}

	// Empty phone allowed multiple times.
	if _, err := s.CreateContact("", "李四", "", "", "", nil); err != nil {
		t.Fatalf("create empty-phone 1: %v", err)
	}
	if _, err := s.CreateContact("", "王五", "", "", "", nil); err != nil {
		t.Fatalf("create empty-phone 2: %v", err)
	}

	// Duplicate non-empty phone rejected.
	if _, err := s.CreateContact("13800000001", "dup", "", "", "", nil); err != ErrDuplicatePhone {
		t.Fatalf("expected ErrDuplicatePhone, got %v", err)
	}

	list, err := s.ListContacts()
	if err != nil || len(list) != 3 {
		t.Fatalf("list: len=%d err=%v", len(list), err)
	}

	// Update fields.
	upd, err := s.UpdateContact(a.ID, "13800000009", "张三丰", "Acme2", "VP", "n2", []string{"x"})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if upd.Phone != "13800000009" || upd.Name != "张三丰" || len(upd.Tags) != 1 {
		t.Fatalf("update not applied: %+v", upd)
	}

	// Update to another contact's phone → conflict. First give 李四 a phone.
	b, _ := s.CreateContact("13800000002", "赵六", "", "", "", nil)
	if _, err := s.UpdateContact(a.ID, "13800000002", "x", "", "", "", nil); err != ErrDuplicatePhone {
		t.Fatalf("expected dup on update, got %v", err)
	}
	_ = b

	// Update unknown id → ErrNotFound.
	if _, err := s.UpdateContact("nope", "", "", "", "", "", nil); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestContactChannelsUpsertLinkUnlinkAndDeleteUnbinds(t *testing.T) {
	db := newTestDB(t)
	s := NewContactStore(db)

	// First upsert inserts.
	if err := s.UpsertChannel("feishu", "ou_aaa", "Alice", "oc_1", 100); err != nil {
		t.Fatalf("upsert 1: %v", err)
	}
	chans, err := s.ListChannels("", false)
	if err != nil || len(chans) != 1 {
		t.Fatalf("list channels: len=%d err=%v", len(chans), err)
	}
	chID := chans[0].ID
	if chans[0].ContactID != "" {
		t.Fatalf("new channel should be unlinked")
	}

	// Link to a contact.
	c, _ := s.CreateContact("13800000001", "Alice Real", "", "", "", nil)
	if err := s.LinkChannel(chID, c.ID); err != nil {
		t.Fatalf("link: %v", err)
	}

	// Idempotent re-upsert refreshes nickname/session/last_seen but MUST NOT
	// clobber the now-set contact_id.
	if err := s.UpsertChannel("feishu", "ou_aaa", "Alice Updated", "oc_2", 200); err != nil {
		t.Fatalf("upsert 2: %v", err)
	}
	all, _ := s.ListChannels("", false)
	if len(all) != 1 {
		t.Fatalf("upsert created a duplicate: %d", len(all))
	}
	if all[0].ContactID != c.ID {
		t.Fatalf("contact_id clobbered by re-upsert: %q", all[0].ContactID)
	}
	if all[0].Nickname != "Alice Updated" || all[0].SessionID != "oc_2" || all[0].LastSeen != 200 {
		t.Fatalf("re-upsert did not refresh fields: %+v", all[0])
	}

	// Unlinked filter now excludes the linked one.
	if un, _ := s.ListChannels("", true); len(un) != 0 {
		t.Fatalf("expected no unlinked, got %d", len(un))
	}

	// Contact carries its channel.
	cwc, _ := s.ContactsWithChannels()
	var found bool
	for _, ct := range cwc {
		if ct.ID == c.ID {
			if len(ct.Channels) != 1 || ct.Channels[0].ChannelID != "ou_aaa" {
				t.Fatalf("contact channels: %+v", ct.Channels)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("contact with channel not found")
	}

	// Unlink.
	if err := s.UnlinkChannel(chID); err != nil {
		t.Fatalf("unlink: %v", err)
	}
	if un, _ := s.ListChannels("", true); len(un) != 1 {
		t.Fatalf("expected 1 unlinked after unlink, got %d", len(un))
	}

	// Re-link, then delete contact → channel survives but is unbound.
	if err := s.LinkChannel(chID, c.ID); err != nil {
		t.Fatalf("relink: %v", err)
	}
	if err := s.DeleteContact(c.ID); err != nil {
		t.Fatalf("delete contact: %v", err)
	}
	after, _ := s.ListChannels("", false)
	if len(after) != 1 {
		t.Fatalf("channel should survive contact delete, got %d", len(after))
	}
	if after[0].ContactID != "" {
		t.Fatalf("channel should be unbound after contact delete: %q", after[0].ContactID)
	}

	// LinkChannel on unknown channel → ErrNotFound.
	if err := s.LinkChannel("nope", c.ID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound on bad link, got %v", err)
	}
}
