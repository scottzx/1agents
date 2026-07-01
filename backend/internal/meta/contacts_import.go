package meta

import (
	"database/sql"
	"strings"
	"time"
)

// NormalizePhone reduces a phone string to a comparable merge key: a leading '+'
// (if present) followed by digits only. The same number written with spaces,
// dashes or parentheses collapses to one key, so a person reached on iMessage
// (handle = E.164 phone) lines up with their address-book / Feishu contact.
func NormalizePhone(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	for i, r := range s {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '+' && i == 0:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ImportedContact is one address-book entry to ingest. Phone is the merge key;
// entries without a phone are skipped (no stable key to dedup on).
type ImportedContact struct {
	Phone   string
	Name    string
	Company string
	Title   string
}

// IngestContacts upserts address-book entries (from iCloud CardDAV or any other
// source) as degree-1 contacts, keyed on the normalized phone. A new phone
// creates a contact; an existing phone (even a degree-2 one discovered from a
// roster) is refreshed and promoted to degree 1 — an explicit address book is an
// authoritative first-degree source. Non-empty fields never overwrite with empty.
// One transaction, idempotent. Returns the counts of created and updated contacts.
func (s *ContactStore) IngestContacts(people []ImportedContact) (created, updated int, err error) {
	tx, err := s.db.sql.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()

	now := timeToStr(time.Now().UTC())
	for _, p := range people {
		phone := NormalizePhone(p.Phone)
		if phone == "" {
			continue
		}
		var id string
		switch e := tx.QueryRow(`SELECT id FROM contacts WHERE phone = ?`, phone).Scan(&id); e {
		case sql.ErrNoRows:
			if _, err = tx.Exec(`INSERT INTO contacts (`+contactCols+`)
                VALUES (?, ?, ?, ?, ?, '', '', 1, ?, ?)`,
				newID(), phone, p.Name, p.Company, p.Title, now, now); err != nil {
				return
			}
			created++
		case nil:
			if _, err = tx.Exec(`UPDATE contacts SET
                name    = CASE WHEN ? != '' THEN ? ELSE name END,
                company = CASE WHEN ? != '' THEN ? ELSE company END,
                title   = CASE WHEN ? != '' THEN ? ELSE title END,
                degree = 1, updated_at = ?
                WHERE id = ?`,
				p.Name, p.Name, p.Company, p.Company, p.Title, p.Title, now, id); err != nil {
				return
			}
			updated++
		default:
			err = e
			return
		}
	}
	err = tx.Commit()
	return
}

// PhoneNameMap returns a normalized-phone → name map over all named contacts,
// used to resolve an iMessage handle (phone) to a display name at ingest time.
func (s *ContactStore) PhoneNameMap() (map[string]string, error) {
	rows, err := s.db.sql.Query(`SELECT phone, name FROM contacts WHERE phone != '' AND name != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]string{}
	for rows.Next() {
		var phone, name string
		if err := rows.Scan(&phone, &name); err != nil {
			return nil, err
		}
		m[NormalizePhone(phone)] = name
	}
	return m, rows.Err()
}

// UpsertIMessageChannel records an iMessage participant identity
// (platform='imessage', channel_id=handle) and auto-links it to a contact whose
// phone matches the handle — the phone merge key is what unifies an address-book
// contact with their iMessage thread. Idempotent on UNIQUE(platform, channel_id):
// a manual link is preserved, an empty contact_id is filled when a phone match is
// found, and a non-empty nickname is never clobbered with empty.
func (s *ContactStore) UpsertIMessageChannel(handle, nickname, sessionID string, lastSeen int64) error {
	contactID := ""
	// Resolve a phone-style handle to a contact; email handles have no phone key.
	if !strings.Contains(handle, "@") {
		if ph := NormalizePhone(handle); ph != "" {
			var id string
			switch e := s.db.sql.QueryRow(`SELECT id FROM contacts WHERE phone = ?`, ph).Scan(&id); e {
			case nil:
				contactID = id
			case sql.ErrNoRows:
			default:
				return e
			}
		}
	}
	now := timeToStr(time.Now().UTC())
	_, err := s.db.sql.Exec(`INSERT INTO contact_channels
        (id, contact_id, platform, channel_id, nickname, session_id, tenant_key, last_seen, created_at, updated_at)
        VALUES (?, ?, 'imessage', ?, ?, ?, '', ?, ?, ?)
        ON CONFLICT(platform, channel_id) DO UPDATE SET
            nickname = CASE WHEN excluded.nickname != '' THEN excluded.nickname ELSE contact_channels.nickname END,
            session_id = excluded.session_id,
            last_seen = excluded.last_seen,
            updated_at = excluded.updated_at,
            contact_id = CASE WHEN contact_channels.contact_id != '' THEN contact_channels.contact_id ELSE excluded.contact_id END`,
		newID(), contactID, handle, nickname, sessionID, lastSeen, now, now)
	return err
}
