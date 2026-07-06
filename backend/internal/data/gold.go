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

// GoldMessage is one fused message plus its participants (sender + @mentions).
type GoldMessage struct {
	ID, ThreadID, Source, AccountID, ExternalID string
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
         body_text, sent_at, fingerprint, created_at)
        VALUES (?, ?, 'im', ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(source, account_id, external_id) DO UPDATE SET
            thread_id         = excluded.thread_id,
            sender_contact_id = excluded.sender_contact_id,
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
		if _, err := msgStmt.Exec(m.ID, m.ThreadID, m.Source, acct(m.AccountID), m.ExternalID,
			m.SenderContactID, m.BodyText, m.SentAt, m.Fingerprint, now); err != nil {
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
