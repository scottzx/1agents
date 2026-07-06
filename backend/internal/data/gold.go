package data

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
)

// gold.go is the silver→gold fusion stage, wholly within data.db (no bronze, no
// network). It resolves every source-native identifier — 飞书 message-sender +
// @mention OpenID — to a canonical contact via contact_channels(platform,
// address), auto-creating a degree-2 contact + channel on first sight (the
// roster-discovery semantics of meta.IngestGroupMembers). Silver messages then
// fuse into gold threads/messages/message_participants. The stage is cursor-gated
// on silver.updated_at and idempotent (gold ids are derived from the source key,
// participants are replaced per message), so resetting StageGold safely re-fuses
// everything. The orchestration/shaping lives in internal/govern (Gold); this
// file owns the data.db reads + writes.

// newGoldID returns a random 16-byte hex id, used for contacts/channels —
// identities that may later merge across sources, so they cannot be derived from
// a single source key (mirrors meta.newID).
func newGoldID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "gold-fallback-id"
	}
	return hex.EncodeToString(b[:])
}

func nowRFC() string { return time.Now().UTC().Format(time.RFC3339) }

// Fingerprint hashes a gold entity's identifying parts (v2 cross-source dedup key,
// stored now so that pass needs no backfill). Parts are joined with a unit
// separator that can't appear in the inputs.
func Fingerprint(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(sum[:])
}

// ---- silver readers (feeding the fusion) ----

// FeishuChatMeta is the thread-shaping metadata for one 飞书 chat.
type FeishuChatMeta struct {
	Name string
	Mode string // group | p2p
}

// SilverFeishuMessagesSince returns silver 飞书 messages with updated_at > since
// (oldest first) plus the max updated_at seen, for advancing the StageGold cursor.
func (s *Store) SilverFeishuMessagesSince(since int64) ([]SilverFeishuMessage, int64, error) {
	rows, err := s.sql.Query(`SELECT account_id, external_id, chat_id, msg_type, sender_open_id,
        sender_tenant_key, body_text, mentions, parent_id, root_id, thread_id, create_time, deleted, updated_at
        FROM silver_feishu_messages WHERE updated_at > ? ORDER BY updated_at`, since)
	if err != nil {
		return nil, since, err
	}
	defer rows.Close()
	out := []SilverFeishuMessage{}
	max := since
	for rows.Next() {
		var m SilverFeishuMessage
		var mentions string
		var deleted int
		if err := rows.Scan(&m.AccountID, &m.ExternalID, &m.ChatID, &m.MsgType, &m.SenderOpenID,
			&m.SenderTenantKey, &m.BodyText, &mentions, &m.ParentID, &m.RootID, &m.ThreadID,
			&m.CreateTime, &deleted, &m.UpdatedAt); err != nil {
			return nil, since, err
		}
		m.Deleted = deleted == 1
		_ = json.Unmarshal([]byte(mentions), &m.Mentions)
		if m.UpdatedAt > max {
			max = m.UpdatedAt
		}
		out = append(out, m)
	}
	return out, max, rows.Err()
}

// SilverFeishuUserNames maps every 二级用户 OpenID to its best-known display name,
// so a sender (whose message carries no name) can still seed a named contact.
func (s *Store) SilverFeishuUserNames() (map[string]string, error) {
	rows, err := s.sql.Query(`SELECT external_id, name FROM silver_feishu_users WHERE name != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[id] = name
	}
	return out, rows.Err()
}

// SilverFeishuChatMetas maps chat_id → thread-shaping metadata (title + mode).
func (s *Store) SilverFeishuChatMetas() (map[string]FeishuChatMeta, error) {
	rows, err := s.sql.Query(`SELECT external_id, name, chat_mode FROM silver_feishu_chats`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]FeishuChatMeta{}
	for rows.Next() {
		var id, name, mode string
		if err := rows.Scan(&id, &name, &mode); err != nil {
			return nil, err
		}
		out[id] = FeishuChatMeta{Name: name, Mode: mode}
	}
	return out, rows.Err()
}

// SilverIcloudContactsSince returns silver iCloud contacts with updated_at > since
// (oldest first) plus the max updated_at seen, for advancing the StageGold cursor.
// Only the columns the address-book seed needs are read (the verbatim vCard lives
// in bronze).
func (s *Store) SilverIcloudContactsSince(since int64) ([]SilverIcloudContact, int64, error) {
	rows, err := s.sql.Query(`SELECT account_id, external_id, full_name, phones, emails,
        org, title, deleted, updated_at
        FROM silver_icloud_contacts WHERE updated_at > ? ORDER BY updated_at`, since)
	if err != nil {
		return nil, since, err
	}
	defer rows.Close()
	out := []SilverIcloudContact{}
	max := since
	for rows.Next() {
		var c SilverIcloudContact
		var phones, emails string
		var deleted int
		if err := rows.Scan(&c.AccountID, &c.ExternalID, &c.FullName, &phones, &emails,
			&c.Org, &c.Title, &deleted, &c.UpdatedAt); err != nil {
			return nil, since, err
		}
		c.Deleted = deleted == 1
		_ = json.Unmarshal([]byte(phones), &c.Phones)
		_ = json.Unmarshal([]byte(emails), &c.Emails)
		if c.UpdatedAt > max {
			max = c.UpdatedAt
		}
		out = append(out, c)
	}
	return out, max, rows.Err()
}

// SilverMicrosoftMailSince / SilverAgentMailSince / SilverMicrosoftEventsSince /
// SilverFeishuEventsSince mirror the feishu reader: rows past the StageGold cursor
// plus the max updated_at, feeding the per-source fusion governors.
func (s *Store) SilverMicrosoftMailSince(since int64) ([]SilverMicrosoftMail, int64, error) {
	rows, err := s.sql.Query(`SELECT account_id, external_id, subject, body_preview, received_at,
        from_addr, from_name, to_recipients, conversation_id, deleted, updated_at
        FROM silver_microsoft_mail WHERE updated_at > ? ORDER BY updated_at`, since)
	if err != nil {
		return nil, since, err
	}
	defer rows.Close()
	out := []SilverMicrosoftMail{}
	max := since
	for rows.Next() {
		var m SilverMicrosoftMail
		var to string
		var deleted int
		if err := rows.Scan(&m.AccountID, &m.ExternalID, &m.Subject, &m.BodyPreview, &m.ReceivedAt,
			&m.FromAddr, &m.FromName, &to, &m.ConversationID, &deleted, &m.UpdatedAt); err != nil {
			return nil, since, err
		}
		m.Deleted = deleted == 1
		_ = json.Unmarshal([]byte(to), &m.ToRecipients)
		if m.UpdatedAt > max {
			max = m.UpdatedAt
		}
		out = append(out, m)
	}
	return out, max, rows.Err()
}

func (s *Store) SilverAgentMailSince(since int64) ([]SilverAgentMail, int64, error) {
	rows, err := s.sql.Query(`SELECT account_id, external_id, subject, snippet, created_at_src,
        from_email, from_name, to_recipients, deleted, updated_at
        FROM silver_agentmail_mail WHERE updated_at > ? ORDER BY updated_at`, since)
	if err != nil {
		return nil, since, err
	}
	defer rows.Close()
	out := []SilverAgentMail{}
	max := since
	for rows.Next() {
		var m SilverAgentMail
		var to string
		var deleted int
		if err := rows.Scan(&m.AccountID, &m.ExternalID, &m.Subject, &m.Snippet, &m.CreatedAtSrc,
			&m.FromEmail, &m.FromName, &to, &deleted, &m.UpdatedAt); err != nil {
			return nil, since, err
		}
		m.Deleted = deleted == 1
		_ = json.Unmarshal([]byte(to), &m.ToRecipients)
		if m.UpdatedAt > max {
			max = m.UpdatedAt
		}
		out = append(out, m)
	}
	return out, max, rows.Err()
}

func (s *Store) SilverMicrosoftEventsSince(since int64) ([]SilverMicrosoftEvent, int64, error) {
	rows, err := s.sql.Query(`SELECT account_id, external_id, calendar_id, subject, location,
        starts_at, ends_at, all_day, show_as, organizer_addr, organizer_name, attendees,
        recurrence, deleted, updated_at
        FROM silver_microsoft_events WHERE updated_at > ? ORDER BY updated_at`, since)
	if err != nil {
		return nil, since, err
	}
	defer rows.Close()
	out := []SilverMicrosoftEvent{}
	max := since
	for rows.Next() {
		var e SilverMicrosoftEvent
		var attendees string
		var allDay, deleted int
		if err := rows.Scan(&e.AccountID, &e.ExternalID, &e.CalendarID, &e.Subject, &e.Location,
			&e.StartsAt, &e.EndsAt, &allDay, &e.ShowAs, &e.OrganizerAddr, &e.OrganizerName,
			&attendees, &e.Recurrence, &deleted, &e.UpdatedAt); err != nil {
			return nil, since, err
		}
		e.AllDay = allDay == 1
		e.Deleted = deleted == 1
		_ = json.Unmarshal([]byte(attendees), &e.Attendees)
		if e.UpdatedAt > max {
			max = e.UpdatedAt
		}
		out = append(out, e)
	}
	return out, max, rows.Err()
}

func (s *Store) SilverFeishuEventsSince(since int64) ([]SilverFeishuEvent, int64, error) {
	rows, err := s.sql.Query(`SELECT account_id, external_id, calendar_id, subject, location,
        starts_at, ends_at, all_day, status, organizer_open_id, organizer_name, recurrence,
        deleted, updated_at
        FROM silver_feishu_events WHERE updated_at > ? ORDER BY updated_at`, since)
	if err != nil {
		return nil, since, err
	}
	defer rows.Close()
	out := []SilverFeishuEvent{}
	max := since
	for rows.Next() {
		var e SilverFeishuEvent
		var allDay, deleted int
		if err := rows.Scan(&e.AccountID, &e.ExternalID, &e.CalendarID, &e.Subject, &e.Location,
			&e.StartsAt, &e.EndsAt, &allDay, &e.Status, &e.OrganizerOpenID, &e.OrganizerName,
			&e.Recurrence, &deleted, &e.UpdatedAt); err != nil {
			return nil, since, err
		}
		e.AllDay = allDay == 1
		e.Deleted = deleted == 1
		if e.UpdatedAt > max {
			max = e.UpdatedAt
		}
		out = append(out, e)
	}
	return out, max, rows.Err()
}

func (s *Store) SilverMicrosoftTodosSince(since int64) ([]SilverMicrosoftTodo, int64, error) {
	rows, err := s.sql.Query(`SELECT account_id, external_id, list_id, title, body, status,
        importance, due_at, completed_at, deleted, updated_at
        FROM silver_microsoft_todos WHERE updated_at > ? ORDER BY updated_at`, since)
	if err != nil {
		return nil, since, err
	}
	defer rows.Close()
	out := []SilverMicrosoftTodo{}
	max := since
	for rows.Next() {
		var td SilverMicrosoftTodo
		var deleted int
		if err := rows.Scan(&td.AccountID, &td.ExternalID, &td.ListID, &td.Title, &td.Body,
			&td.Status, &td.Importance, &td.DueAt, &td.CompletedAt, &deleted, &td.UpdatedAt); err != nil {
			return nil, since, err
		}
		td.Deleted = deleted == 1
		if td.UpdatedAt > max {
			max = td.UpdatedAt
		}
		out = append(out, td)
	}
	return out, max, rows.Err()
}

// ---- address normalization (deterministic channel keys) ----

// NormEmail lowercases + trims an email so the same address from different sources
// (MS Graph, AgentMail, iCloud vCard) collapses to one contact_channels key.
func NormEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// NormPhone keeps only digits and a single leading '+', dropping spaces/dashes/
// parens so formatting variants of one number collapse to one key. Country-code
// unification is out of scope for v1 (deterministic-only merge).
func NormPhone(s string) string {
	var b strings.Builder
	for i, r := range strings.TrimSpace(s) {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else if r == '+' && i == 0 {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ---- identity resolution (the fusion anchor) ----

// ResolveContact maps (platform, address) to a canonical contact_id, creating a
// degree-2 contact + channel on first sight and linking an orphan channel. name
// (best-known) seeds a new contact and backfills an existing one whose name is
// still empty; a non-empty tenant_key is never overwritten with empty. Returns
// created=true only when a new contact row was inserted. An empty address (system
// messages) resolves to ("", false) — no phantom contact.
func (s *Store) ResolveContact(platform, address, name, tenantKey string) (string, bool, error) {
	if address == "" {
		return "", false, nil
	}
	now := nowRFC()
	tx, err := s.sql.Begin()
	if err != nil {
		return "", false, err
	}
	defer tx.Rollback()

	var chID, contactID string
	err = tx.QueryRow(`SELECT id, contact_id FROM contact_channels
        WHERE platform = ? AND address = ?`, platform, address).Scan(&chID, &contactID)
	created := false
	switch {
	case err == sql.ErrNoRows:
		// No channel → new degree-2 contact + linked channel.
		contactID = newGoldID()
		if _, err := tx.Exec(`INSERT INTO contacts (id, name, degree, created_at, updated_at)
            VALUES (?, ?, 2, ?, ?)`, contactID, name, now, now); err != nil {
			return "", false, err
		}
		if _, err := tx.Exec(`INSERT INTO contact_channels
            (id, contact_id, platform, address, nickname, tenant_key, last_seen, created_at, updated_at)
            VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?)`,
			newGoldID(), contactID, platform, address, name, tenantKey, now, now); err != nil {
			return "", false, err
		}
		created = true
	case err != nil:
		return "", false, err
	case contactID == "":
		// Orphan channel → new degree-2 contact, link it, refresh nickname/tenant.
		contactID = newGoldID()
		if _, err := tx.Exec(`INSERT INTO contacts (id, name, degree, created_at, updated_at)
            VALUES (?, ?, 2, ?, ?)`, contactID, name, now, now); err != nil {
			return "", false, err
		}
		if _, err := tx.Exec(`UPDATE contact_channels SET contact_id = ?, nickname = ?, updated_at = ?,
            tenant_key = CASE WHEN ? != '' THEN ? ELSE tenant_key END WHERE id = ?`,
			contactID, name, now, tenantKey, tenantKey, chID); err != nil {
			return "", false, err
		}
		created = true
	default:
		// Already linked → only backfill a still-empty contact name (never relink,
		// never touch degree, preserving a future manual/1st-degree promotion).
		if name != "" {
			if _, err := tx.Exec(`UPDATE contacts SET name = ?, updated_at = ?
                WHERE id = ? AND name = ''`, name, now, contactID); err != nil {
				return "", false, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return "", false, err
	}
	return contactID, created, nil
}

// mergeContactsTx folds dupID into canonicalID: every gold reference to dupID —
// channels, message sender/participant, thread, event attendee — is repointed to
// canonicalID, then the now-empty dup contact row is deleted. UPDATE OR IGNORE +
// a cleanup DELETE handles the tables whose (…, contact_id) is a unique/primary
// key: if canonical is already the participant/attendee/channel, the dup's row
// can't move and is simply dropped (dedup). A no-op when the ids are equal.
func mergeContactsTx(tx *sql.Tx, canonicalID, dupID string) error {
	if dupID == "" || dupID == canonicalID {
		return nil
	}
	// No-conflict columns (no unique constraint) → plain UPDATE.
	for _, q := range []string{
		`UPDATE messages SET sender_contact_id = ? WHERE sender_contact_id = ?`,
		`UPDATE threads SET contact_id = ? WHERE contact_id = ?`,
	} {
		if _, err := tx.Exec(q, canonicalID, dupID); err != nil {
			return err
		}
	}
	// Unique/PK columns → move what can move, drop the rest (canonical already has it).
	for _, q := range []string{
		`UPDATE OR IGNORE contact_channels SET contact_id = ? WHERE contact_id = ?`,
		`UPDATE OR IGNORE message_participants SET contact_id = ? WHERE contact_id = ?`,
		`UPDATE OR IGNORE event_attendees SET contact_id = ? WHERE contact_id = ?`,
	} {
		if _, err := tx.Exec(q, canonicalID, dupID); err != nil {
			return err
		}
	}
	for _, q := range []string{
		`DELETE FROM contact_channels WHERE contact_id = ?`,
		`DELETE FROM message_participants WHERE contact_id = ?`,
		`DELETE FROM event_attendees WHERE contact_id = ?`,
		`DELETE FROM contacts WHERE id = ?`,
	} {
		if _, err := tx.Exec(q, dupID); err != nil {
			return err
		}
	}
	return nil
}

// ensureChannelTx guarantees a (platform, address) channel points at canonicalID.
// Missing → create it. Orphan (no contact) → link it. Owned by another contact →
// merge that contact into canonicalID (deterministic same-address = same person).
// Returns 1 when a distinct contact was merged in, else 0.
func ensureChannelTx(tx *sql.Tx, canonicalID, platform, address, nickname, now string) (int, error) {
	if address == "" {
		return 0, nil
	}
	var chID, owner string
	err := tx.QueryRow(`SELECT id, contact_id FROM contact_channels
        WHERE platform = ? AND address = ?`, platform, address).Scan(&chID, &owner)
	switch {
	case err == sql.ErrNoRows:
		_, err := tx.Exec(`INSERT INTO contact_channels
            (id, contact_id, platform, address, nickname, last_seen, created_at, updated_at)
            VALUES (?, ?, ?, ?, ?, 0, ?, ?)`, newGoldID(), canonicalID, platform, address, nickname, now, now)
		return 0, err
	case err != nil:
		return 0, err
	case owner == "" || owner == canonicalID:
		_, err := tx.Exec(`UPDATE contact_channels SET contact_id = ?, updated_at = ? WHERE id = ?`,
			canonicalID, now, chID)
		return 0, err
	default:
		// Channel belongs to a different contact → fold it into canonical.
		if err := mergeContactsTx(tx, canonicalID, owner); err != nil {
			return 0, err
		}
		return 1, nil
	}
}

// UpsertAddressBookContact seeds/refreshes one iCloud contact as a degree-1
// (address-book authoritative) contact and attaches an email/phone channel per
// address, folding in any degree-2 contact a message/roster had already created
// on the same email/phone. The iCloud vCard UID is the stable anchor channel
// (platform='icloud'), so re-runs converge. Returns created (a new contact row)
// and merged (how many discovered contacts were folded in).
func (s *Store) UpsertAddressBookContact(c SilverIcloudContact) (created bool, merged int, err error) {
	tx, err := s.sql.Begin()
	if err != nil {
		return false, 0, err
	}
	defer tx.Rollback()

	now := nowRFC()
	name := strings.TrimSpace(c.FullName)

	// 1. Anchor on the vCard UID so the same address-book entry always resolves to
	//    the same contact across runs.
	var canonicalID string
	err = tx.QueryRow(`SELECT contact_id FROM contact_channels
        WHERE platform = 'icloud' AND address = ?`, c.ExternalID).Scan(&canonicalID)
	switch {
	case err == sql.ErrNoRows:
		canonicalID = newGoldID()
		if _, err := tx.Exec(`INSERT INTO contacts (id, name, degree, created_at, updated_at)
            VALUES (?, ?, 1, ?, ?)`, canonicalID, name, now, now); err != nil {
			return false, 0, err
		}
		if _, err := tx.Exec(`INSERT INTO contact_channels
            (id, contact_id, platform, address, nickname, last_seen, created_at, updated_at)
            VALUES (?, ?, 'icloud', ?, ?, 0, ?, ?)`,
			newGoldID(), canonicalID, c.ExternalID, name, now, now); err != nil {
			return false, 0, err
		}
		created = true
	case err != nil:
		return false, 0, err
	}

	// 2. iCloud is authoritative → set degree-1 + rich fields (name only if present).
	if _, err := tx.Exec(`UPDATE contacts SET degree = 1, company = ?, title = ?, updated_at = ?,
        name = CASE WHEN ? != '' THEN ? ELSE name END WHERE id = ?`,
		c.Org, c.Title, now, name, name, canonicalID); err != nil {
		return false, 0, err
	}
	// phone convenience column is UNIQUE — best-effort only; the phone *channel*
	// below is the real identity key.
	for _, raw := range c.Phones {
		if p := NormPhone(raw); p != "" {
			_, _ = tx.Exec(`UPDATE OR IGNORE contacts SET phone = ? WHERE id = ? AND phone = ''`,
				p, canonicalID)
			break
		}
	}

	// 3. Attach every email/phone channel, merging any contact already on it.
	for _, raw := range c.Emails {
		m, err := ensureChannelTx(tx, canonicalID, "email", NormEmail(raw), name, now)
		if err != nil {
			return false, 0, err
		}
		merged += m
	}
	for _, raw := range c.Phones {
		m, err := ensureChannelTx(tx, canonicalID, "phone", NormPhone(raw), name, now)
		if err != nil {
			return false, 0, err
		}
		merged += m
	}

	if err := tx.Commit(); err != nil {
		return false, 0, err
	}
	return created, merged, nil
}

// ---- gold writers ----

// GoldThread is one fused conversation (飞书 chat → thread).
type GoldThread struct {
	ID, Source, AccountID, ExternalID string
	Kind, Title, ContactID            string
	LastMessageAt                     int64
}

// GoldParticipant links a contact to a message with a role (from | mention | ...).
type GoldParticipant struct {
	ContactID, Role string
}

// GoldMessage is one fused message plus its participants (sender + @mentions /
// to-recipients). MsgKind is im|email|...; Subject is empty for chat/IM.
type GoldMessage struct {
	ID, ThreadID, Source, AccountID, ExternalID string
	MsgKind, Subject                            string
	SenderContactID, BodyText                   string
	SentAt                                      int64
	Fingerprint                                 string
	Participants                                []GoldParticipant
}

// UpsertThreads upserts gold threads on their (source, account_id, external_id)
// grain; last_message_at only ever advances so late-arriving old messages don't
// rewind a thread's recency, and created_at is preserved on update.
func (s *Store) UpsertThreads(rows []GoldThread) error {
	_, err := withTx(s.sql, rows, func(tx *sql.Tx) (*sql.Stmt, error) {
		return tx.Prepare(`INSERT INTO threads
            (id, source, account_id, external_id, kind, title, contact_id, last_message_at, created_at)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
            ON CONFLICT(source, account_id, external_id) DO UPDATE SET
                kind            = excluded.kind,
                title           = excluded.title,
                contact_id      = excluded.contact_id,
                last_message_at = MAX(threads.last_message_at, excluded.last_message_at)`)
	}, func(stmt *sql.Stmt, t GoldThread) error {
		_, err := stmt.Exec(t.ID, t.Source, acct(t.AccountID), t.ExternalID, t.Kind, t.Title,
			t.ContactID, t.LastMessageAt, nowRFC())
		return err
	})
	return err
}

// UpsertMessages upserts gold messages on their (source, account_id, external_id)
// grain and replaces each message's participants, so a re-fuse converges rather
// than duplicating. created_at is preserved on update.
func (s *Store) UpsertMessages(rows []GoldMessage) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := s.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	msgStmt, err := tx.Prepare(`INSERT INTO messages
        (id, thread_id, msg_kind, source, account_id, external_id, sender_contact_id,
         subject, body_text, sent_at, fingerprint, created_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(source, account_id, external_id) DO UPDATE SET
            thread_id         = excluded.thread_id,
            msg_kind          = excluded.msg_kind,
            sender_contact_id = excluded.sender_contact_id,
            subject           = excluded.subject,
            body_text         = excluded.body_text,
            sent_at           = excluded.sent_at,
            fingerprint       = excluded.fingerprint`)
	if err != nil {
		return err
	}
	defer msgStmt.Close()
	delPart, err := tx.Prepare(`DELETE FROM message_participants WHERE message_id = ?`)
	if err != nil {
		return err
	}
	defer delPart.Close()
	insPart, err := tx.Prepare(`INSERT OR IGNORE INTO message_participants
        (message_id, contact_id, role) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer insPart.Close()

	now := nowRFC()
	for _, m := range rows {
		kind := m.MsgKind
		if kind == "" {
			kind = "im"
		}
		if _, err := msgStmt.Exec(m.ID, m.ThreadID, kind, m.Source, acct(m.AccountID), m.ExternalID,
			m.SenderContactID, m.Subject, m.BodyText, m.SentAt, m.Fingerprint, now); err != nil {
			return err
		}
		if _, err := delPart.Exec(m.ID); err != nil {
			return err
		}
		for _, p := range m.Participants {
			if p.ContactID == "" {
				continue
			}
			if _, err := insPart.Exec(m.ID, p.ContactID, p.Role); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// ---- gold calendar writers (domain ③ 日历) ----

// GoldEventAttendee links a contact to an event with an RSVP response.
type GoldEventAttendee struct {
	ContactID, Response string
}

// GoldEvent is one fused calendar event plus its attendees. Fingerprint
// (title+start+end) lets the same real meeting synced from two sources be grouped
// at read time without a destructive physical merge.
type GoldEvent struct {
	ID, Source, AccountID, ExternalID string
	CalendarID, Title, Location       string
	StartsAt, EndsAt                  int64
	AllDay                            bool
	RRule, OrganizerContactID, Status string
	Fingerprint                       string
	Attendees                         []GoldEventAttendee
}

// UpsertCalendarEvents upserts gold events on their (source, account_id,
// external_id) grain and replaces each event's attendees, so a re-fuse converges
// rather than duplicating. created_at is preserved on update.
func (s *Store) UpsertCalendarEvents(rows []GoldEvent) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := s.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	evStmt, err := tx.Prepare(`INSERT INTO calendar_events
        (id, source, account_id, external_id, calendar_id, title, location, starts_at, ends_at,
         all_day, rrule, organizer_contact_id, status, fingerprint, created_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(source, account_id, external_id) DO UPDATE SET
            calendar_id          = excluded.calendar_id,
            title                = excluded.title,
            location             = excluded.location,
            starts_at            = excluded.starts_at,
            ends_at              = excluded.ends_at,
            all_day              = excluded.all_day,
            rrule                = excluded.rrule,
            organizer_contact_id = excluded.organizer_contact_id,
            status               = excluded.status,
            fingerprint          = excluded.fingerprint`)
	if err != nil {
		return err
	}
	defer evStmt.Close()
	delAtt, err := tx.Prepare(`DELETE FROM event_attendees WHERE event_id = ?`)
	if err != nil {
		return err
	}
	defer delAtt.Close()
	insAtt, err := tx.Prepare(`INSERT OR IGNORE INTO event_attendees
        (event_id, contact_id, response) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer insAtt.Close()

	now := nowRFC()
	for _, e := range rows {
		if _, err := evStmt.Exec(e.ID, e.Source, acct(e.AccountID), e.ExternalID, e.CalendarID,
			e.Title, e.Location, e.StartsAt, e.EndsAt, boolInt(e.AllDay), e.RRule,
			e.OrganizerContactID, e.Status, e.Fingerprint, now); err != nil {
			return err
		}
		if _, err := delAtt.Exec(e.ID); err != nil {
			return err
		}
		for _, a := range e.Attendees {
			if a.ContactID == "" {
				continue
			}
			if _, err := insAtt.Exec(e.ID, a.ContactID, a.Response); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// ---- gold todo writer (domain ④ 待办) ----

// GoldTodo is one fused to-do. Single-source today (MS), but the gold grain
// (source, account_id, external_id) + fingerprint is ready for cross-source
// dedup, and linked_task_id (preserved on update) can promote it to an agent
// work-order later.
type GoldTodo struct {
	ID, Source, AccountID, ExternalID      string
	ListID, Title, Notes, Status, Priority string
	DueAt, CompletedAt                     int64
	Fingerprint                            string
}

// UpsertTodos upserts gold todos on their (source, account_id, external_id) grain.
// created_at and linked_task_id are preserved on update (a re-fuse never unlinks
// a promoted task).
func (s *Store) UpsertTodos(rows []GoldTodo) error {
	_, err := withTx(s.sql, rows, func(tx *sql.Tx) (*sql.Stmt, error) {
		return tx.Prepare(`INSERT INTO todos
            (id, source, account_id, external_id, list_id, title, notes, due_at, completed_at,
             status, priority, fingerprint, created_at)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            ON CONFLICT(source, account_id, external_id) DO UPDATE SET
                list_id      = excluded.list_id,
                title        = excluded.title,
                notes        = excluded.notes,
                due_at       = excluded.due_at,
                completed_at = excluded.completed_at,
                status       = excluded.status,
                priority     = excluded.priority,
                fingerprint  = excluded.fingerprint`)
	}, func(stmt *sql.Stmt, td GoldTodo) error {
		_, err := stmt.Exec(td.ID, td.Source, acct(td.AccountID), td.ExternalID, td.ListID, td.Title,
			td.Notes, td.DueAt, td.CompletedAt, td.Status, td.Priority, td.Fingerprint, nowRFC())
		return err
	})
	return err
}
