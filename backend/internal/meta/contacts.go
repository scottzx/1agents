package meta

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// ErrDuplicatePhone is returned when creating/updating a contact would collide
// with another contact's non-empty phone (the cross-channel merge key). Empty
// phones are allowed on multiple contacts.
var ErrDuplicatePhone = fmt.Errorf("meta: duplicate contact phone")

// Contact is one entry in the user-curated address book. phone is the unique
// merge key across channels (a person reached on Feishu + WeChat is one
// contact); it may be empty when unknown. tags is stored comma-separated.
type Contact struct {
	ID      string   `json:"id"`
	Phone   string   `json:"phone"`
	Name    string   `json:"name"`
	Company string   `json:"company"`
	Title   string   `json:"title"`
	Note    string   `json:"note"`
	Tags    []string `json:"tags"`
	// Degree records how the contact entered the book: 1 = first-degree (manual /
	// 好友, the proper 1st degree lands later), 2 = second-degree (discovered only
	// from a tracked group's roster — a person in a shared group you don't yet
	// know directly). Defaults to 1 for manually created contacts.
	Degree int `json:"degree"`
	// GroupCount is how many distinct tracked groups this contact appears in,
	// COUNT(DISTINCT session_id) over feishu_group_members across the contact's
	// Feishu channels (open_ids). Additive enrichment for the 所在群 grid column;
	// 0 when the contact has no Feishu channel or no roster rows yet.
	GroupCount int              `json:"groupCount"`
	CreatedAt  time.Time        `json:"createdAt"`
	UpdatedAt  time.Time        `json:"updatedAt"`
	Channels   []ContactChannel `json:"channels,omitempty"`
}

// ContactChannel binds a synced channel identity (platform + channelId, e.g. a
// Feishu open_id) to a contact. channelId is auto-discovered from messages;
// contactId is set when the user links it. sessionId records the last group the
// identity was seen in (best-effort context).
type ContactChannel struct {
	ID        string `json:"id"`
	ContactID string `json:"contactId"`
	Platform  string `json:"platform"`
	ChannelID string `json:"channelId"`
	Nickname  string `json:"nickname"`
	SessionID string `json:"sessionId"`
	// TenantKey is the member's Feishu org, captured free from chat.members during
	// roster ingestion (degree-2). Empty for sender-discovered channels.
	TenantKey string    `json:"tenantKey"`
	LastSeen  int64     `json:"lastSeen"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ContactStore manages the address book + channel identities (meta.db v16).
type ContactStore struct {
	db *DB
}

// NewContactStore returns a ContactStore over db.
func NewContactStore(db *DB) *ContactStore { return &ContactStore{db: db} }

const contactCols = `id, phone, name, company, title, note, tags, degree, created_at, updated_at`

func scanContact(r rowScanner) (Contact, error) {
	var c Contact
	var tags, createdAt, updatedAt string
	if err := r.Scan(&c.ID, &c.Phone, &c.Name, &c.Company, &c.Title, &c.Note, &tags, &c.Degree, &createdAt, &updatedAt); err != nil {
		return Contact{}, err
	}
	c.Tags = splitTags(tags)
	c.CreatedAt = strToTime(createdAt)
	c.UpdatedAt = strToTime(updatedAt)
	return c, nil
}

func splitTags(s string) []string {
	out := []string{}
	for _, t := range strings.Split(s, ",") {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func joinTags(tags []string) string {
	cleaned := make([]string, 0, len(tags))
	for _, t := range tags {
		if t = strings.TrimSpace(t); t != "" {
			cleaned = append(cleaned, t)
		}
	}
	return strings.Join(cleaned, ",")
}

const contactChannelCols = `id, contact_id, platform, channel_id, nickname, session_id, tenant_key, last_seen, created_at, updated_at`

func scanContactChannel(r rowScanner) (ContactChannel, error) {
	var cc ContactChannel
	var createdAt, updatedAt string
	if err := r.Scan(&cc.ID, &cc.ContactID, &cc.Platform, &cc.ChannelID, &cc.Nickname,
		&cc.SessionID, &cc.TenantKey, &cc.LastSeen, &createdAt, &updatedAt); err != nil {
		return ContactChannel{}, err
	}
	cc.CreatedAt = strToTime(createdAt)
	cc.UpdatedAt = strToTime(updatedAt)
	return cc, nil
}

// isPhoneConflict reports whether err is the partial-unique phone index firing.
func isPhoneConflict(err error) bool {
	return err != nil && strings.Contains(err.Error(), "contacts.phone")
}

// ListContacts returns all contacts (no channels attached) ordered by name.
func (s *ContactStore) ListContacts() ([]Contact, error) {
	return s.ListContactsByDegree(0)
}

// ListContactsByDegree returns contacts ordered by name. When degree is 1 or 2
// it filters to that degree; 0 (or any other value) returns all degrees.
func (s *ContactStore) ListContactsByDegree(degree int) ([]Contact, error) {
	q := `SELECT ` + contactCols + ` FROM contacts`
	args := []any{}
	if degree == 1 || degree == 2 {
		q += ` WHERE degree = ?`
		args = append(args, degree)
	}
	q += ` ORDER BY name, created_at`
	rows, err := s.db.sql.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Contact{}
	for rows.Next() {
		c, err := scanContact(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ContactsWithChannels returns all contacts with their bound channels attached.
// When degree is 1 or 2 it filters to that degree; 0 returns all.
func (s *ContactStore) ContactsWithChannels() ([]Contact, error) {
	return s.ContactsWithChannelsByDegree(0)
}

// ContactsWithChannelsByDegree returns contacts (optionally degree-filtered)
// with their bound channels attached.
func (s *ContactStore) ContactsWithChannelsByDegree(degree int) ([]Contact, error) {
	contacts, err := s.ListContactsByDegree(degree)
	if err != nil {
		return nil, err
	}
	for i := range contacts {
		chans, err := s.ListChannelsForContact(contacts[i].ID)
		if err != nil {
			return nil, err
		}
		contacts[i].Channels = chans
		gc, err := s.groupCountForChannels(chans)
		if err != nil {
			return nil, err
		}
		contacts[i].GroupCount = gc
	}
	return contacts, nil
}

// groupCountForChannels returns COUNT(DISTINCT session_id) over feishu_group_members
// for the contact's Feishu channel open_ids — the number of tracked groups the
// contact appears in. A person in N groups has one open_id channel but N roster
// rows, so the count must come from the roster, not the channel rows.
func (s *ContactStore) groupCountForChannels(chans []ContactChannel) (int, error) {
	openIDs := make([]string, 0, len(chans))
	for _, ch := range chans {
		if ch.Platform == "feishu" && ch.ChannelID != "" {
			openIDs = append(openIDs, ch.ChannelID)
		}
	}
	if len(openIDs) == 0 {
		return 0, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(openIDs)), ",")
	args := make([]any, len(openIDs))
	for i, id := range openIDs {
		args[i] = id
	}
	var n int
	err := s.db.sql.QueryRow(`SELECT COUNT(DISTINCT session_id) FROM feishu_group_members
        WHERE channel_id IN (`+placeholders+`)`, args...).Scan(&n)
	return n, err
}

// CreateContact inserts a new contact. Returns ErrDuplicatePhone when phone is
// non-empty and already used by another contact.
func (s *ContactStore) CreateContact(phone, name, company, title, note string, tags []string) (Contact, error) {
	now := time.Now().UTC()
	c := Contact{
		ID:        newID(),
		Phone:     strings.TrimSpace(phone),
		Name:      name,
		Company:   company,
		Title:     title,
		Note:      note,
		Tags:      tags,
		Degree:    1, // manual contacts are first-degree-ish (proper 1st degree later)
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err := s.db.sql.Exec(`INSERT INTO contacts (`+contactCols+`)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.Phone, c.Name, c.Company, c.Title, c.Note, joinTags(c.Tags), c.Degree,
		timeToStr(c.CreatedAt), timeToStr(c.UpdatedAt))
	if isPhoneConflict(err) {
		return Contact{}, ErrDuplicatePhone
	}
	if err != nil {
		return Contact{}, err
	}
	return s.getContact(c.ID)
}

func (s *ContactStore) getContact(id string) (Contact, error) {
	row := s.db.sql.QueryRow(`SELECT `+contactCols+` FROM contacts WHERE id = ?`, id)
	return scanContact(row)
}

// UpdateContact edits a contact's fields. Returns ErrNotFound when id is unknown
// and ErrDuplicatePhone on a non-empty phone collision.
func (s *ContactStore) UpdateContact(id, phone, name, company, title, note string, tags []string) (Contact, error) {
	res, err := s.db.sql.Exec(`UPDATE contacts SET
        phone = ?, name = ?, company = ?, title = ?, note = ?, tags = ?, updated_at = ?
        WHERE id = ?`,
		strings.TrimSpace(phone), name, company, title, note, joinTags(tags),
		timeToStr(time.Now().UTC()), id)
	if isPhoneConflict(err) {
		return Contact{}, ErrDuplicatePhone
	}
	if err != nil {
		return Contact{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Contact{}, ErrNotFound
	}
	return s.getContact(id)
}

// DeleteContact removes a contact and unbinds (does NOT delete) its channels, so
// the discovered identities survive and can be re-linked elsewhere.
func (s *ContactStore) DeleteContact(id string) error {
	tx, err := s.db.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE contact_channels SET contact_id = '', updated_at = ? WHERE contact_id = ?`,
		timeToStr(time.Now().UTC()), id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM contacts WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// ListChannelsForContact returns a contact's bound channels (newest seen first).
func (s *ContactStore) ListChannelsForContact(contactID string) ([]ContactChannel, error) {
	return s.queryChannels(`SELECT `+contactChannelCols+` FROM contact_channels
        WHERE contact_id = ? ORDER BY last_seen DESC`, contactID)
}

// ListChannels lists channels. When onlyUnlinked, returns those with no bound
// contact (the pickable pool); otherwise returns the contact's channels when
// contactID is set, or all channels when it is empty.
func (s *ContactStore) ListChannels(contactID string, onlyUnlinked bool) ([]ContactChannel, error) {
	if onlyUnlinked {
		return s.queryChannels(`SELECT ` + contactChannelCols + ` FROM contact_channels
            WHERE contact_id = '' ORDER BY last_seen DESC`)
	}
	if contactID != "" {
		return s.ListChannelsForContact(contactID)
	}
	return s.queryChannels(`SELECT ` + contactChannelCols + ` FROM contact_channels ORDER BY last_seen DESC`)
}

func (s *ContactStore) queryChannels(q string, args ...any) ([]ContactChannel, error) {
	rows, err := s.db.sql.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ContactChannel{}
	for rows.Next() {
		cc, err := scanContactChannel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, cc)
	}
	return out, rows.Err()
}

// UpsertChannel records a discovered channel identity, idempotent on
// UNIQUE(platform, channel_id). On conflict it refreshes nickname/session/
// last_seen but MUST NOT overwrite an existing non-empty contact_id (a manual
// link survives re-discovery).
func (s *ContactStore) UpsertChannel(platform, channelID, nickname, sessionID string, lastSeen int64) error {
	now := timeToStr(time.Now().UTC())
	_, err := s.db.sql.Exec(`INSERT INTO contact_channels
        (id, contact_id, platform, channel_id, nickname, session_id, tenant_key, last_seen, created_at, updated_at)
        VALUES (?, '', ?, ?, ?, ?, '', ?, ?, ?)
        ON CONFLICT(platform, channel_id) DO UPDATE SET
            nickname = excluded.nickname,
            session_id = excluded.session_id,
            last_seen = excluded.last_seen,
            updated_at = excluded.updated_at`,
		newID(), platform, channelID, nickname, sessionID, lastSeen, now, now)
	return err
}

// LinkChannel binds a channel identity to a contact. Returns ErrNotFound when
// the channel id is unknown.
func (s *ContactStore) LinkChannel(channelID, contactID string) error {
	res, err := s.db.sql.Exec(`UPDATE contact_channels SET contact_id = ?, updated_at = ? WHERE id = ?`,
		contactID, timeToStr(time.Now().UTC()), channelID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// UnlinkChannel detaches a channel identity from its contact (keeps the row).
func (s *ContactStore) UnlinkChannel(channelID string) error {
	res, err := s.db.sql.Exec(`UPDATE contact_channels SET contact_id = '', updated_at = ? WHERE id = ?`,
		timeToStr(time.Now().UTC()), channelID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetChannel returns a single channel by id; ok is false when absent.
func (s *ContactStore) GetChannel(id string) (ContactChannel, bool, error) {
	row := s.db.sql.QueryRow(`SELECT `+contactChannelCols+` FROM contact_channels WHERE id = ?`, id)
	cc, err := scanContactChannel(row)
	if err == sql.ErrNoRows {
		return ContactChannel{}, false, nil
	}
	if err != nil {
		return ContactChannel{}, false, err
	}
	return cc, true, nil
}

// GroupMember is one chat roster entry for ingestion: open_id + display name +
// the member's Feishu org (tenant_key). All three come free in one chat.members
// call; ingestion never does a per-member lookup.
type GroupMember struct {
	OpenID    string
	Name      string
	TenantKey string
}

// IngestGroupMembers records a tracked group's full member roster as degree-2
// contacts, refreshed on each sync. For each member it:
//  1. upserts the feishu_group_members row (session_id + channel_id=open_id +
//     nickname + tenant_key);
//  2. looks up the feishu channel by open_id:
//     - no channel        → create a degree-2 contact + a channel (with tenant_key)
//     linked to it;
//     - unlinked channel  → create a degree-2 contact and link this channel,
//     refreshing nickname + tenant_key;
//     - already-linked     → only refresh nickname (+ tenant_key when non-empty);
//     never relink and never touch the contact's degree (preserves manual /
//     1st-degree promotion).
//
// A non-empty tenant_key is never overwritten with empty. Runs in one
// transaction and is idempotent: re-ingesting the same roster creates nothing
// new. Returns the number of new degree-2 contacts created.
func (s *ContactStore) IngestGroupMembers(sessionID string, members []GroupMember) (int, error) {
	tx, err := s.db.sql.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	now := timeToStr(time.Now().UTC())
	lastSeen := time.Now().UnixMilli()
	created := 0

	for _, m := range members {
		if m.OpenID == "" {
			continue
		}
		// 1. Upsert the roster row.
		if _, err := tx.Exec(`INSERT INTO feishu_group_members
            (session_id, channel_id, nickname, tenant_key, updated_at)
            VALUES (?, ?, ?, ?, ?)
            ON CONFLICT(session_id, channel_id) DO UPDATE SET
                nickname = excluded.nickname,
                tenant_key = CASE WHEN excluded.tenant_key != '' THEN excluded.tenant_key ELSE feishu_group_members.tenant_key END,
                updated_at = excluded.updated_at`,
			sessionID, m.OpenID, m.Name, m.TenantKey, now); err != nil {
			return 0, err
		}

		// 2. Look up the existing feishu channel for this open_id.
		var chID, contactID string
		err := tx.QueryRow(`SELECT id, contact_id FROM contact_channels
            WHERE platform = 'feishu' AND channel_id = ?`, m.OpenID).Scan(&chID, &contactID)
		switch {
		case err == sql.ErrNoRows:
			// No channel yet → create a degree-2 contact + a linked channel.
			cID := newID()
			if _, err := tx.Exec(`INSERT INTO contacts (`+contactCols+`)
                VALUES (?, '', ?, '', '', '', '', 2, ?, ?)`,
				cID, m.Name, now, now); err != nil {
				return 0, err
			}
			if _, err := tx.Exec(`INSERT INTO contact_channels
                (id, contact_id, platform, channel_id, nickname, session_id, tenant_key, last_seen, created_at, updated_at)
                VALUES (?, ?, 'feishu', ?, ?, ?, ?, ?, ?, ?)`,
				newID(), cID, m.OpenID, m.Name, sessionID, m.TenantKey, lastSeen, now, now); err != nil {
				return 0, err
			}
			created++
		case err != nil:
			return 0, err
		case contactID == "":
			// Channel exists but is unlinked → create a degree-2 contact, link it,
			// refresh nickname + tenant_key (don't clobber a non-empty tenant_key).
			cID := newID()
			if _, err := tx.Exec(`INSERT INTO contacts (`+contactCols+`)
                VALUES (?, '', ?, '', '', '', '', 2, ?, ?)`,
				cID, m.Name, now, now); err != nil {
				return 0, err
			}
			if _, err := tx.Exec(`UPDATE contact_channels SET
                contact_id = ?, nickname = ?, updated_at = ?,
                tenant_key = CASE WHEN ? != '' THEN ? ELSE tenant_key END
                WHERE id = ?`,
				cID, m.Name, now, m.TenantKey, m.TenantKey, chID); err != nil {
				return 0, err
			}
			created++
		default:
			// Already linked to a contact → only refresh nickname (+ tenant_key when
			// non-empty); never relink and never change the contact's degree.
			if _, err := tx.Exec(`UPDATE contact_channels SET
                nickname = ?, updated_at = ?,
                tenant_key = CASE WHEN ? != '' THEN ? ELSE tenant_key END
                WHERE id = ?`,
				m.Name, now, m.TenantKey, m.TenantKey, chID); err != nil {
				return 0, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return created, nil
}

// SenderRef is one distinct message sender discovered from a sync batch: the
// open_id and the org (tenant_key) carried by the message sender object. Unlike
// a roster member it has no nickname (messages don't carry names).
type SenderRef struct {
	OpenID    string
	TenantKey string
}

// IngestMessageSenders incrementally records active message speakers as degree-2
// contacts/channels. It captures speakers beyond the roster's 100-member cap and
// keeps tenant_key/last_seen fresh, WITHOUT a chat.members roster call. For each
// distinct sender it:
//   - no channel yet      → create a degree-2 contact + a channel (empty nickname,
//     since messages carry no name) linked to it;
//   - unlinked channel     → create a degree-2 contact, link it, refresh tenant_key
//   - last_seen (never overwrite a non-empty nickname with empty);
//   - already-linked       → only refresh tenant_key + last_seen; never relink,
//     never touch degree, never clobber a non-empty nickname.
//
// It does NOT touch the feishu_group_members roster (that is the roster cache,
// distinct from sender discovery). A non-empty tenant_key is never overwritten
// with empty. Runs in one transaction; idempotent. Returns the number of new
// degree-2 contacts created.
func (s *ContactStore) IngestMessageSenders(sessionID string, senders []SenderRef) (int, error) {
	tx, err := s.db.sql.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	now := timeToStr(time.Now().UTC())
	lastSeen := time.Now().UnixMilli()
	created := 0

	for _, sr := range senders {
		if sr.OpenID == "" {
			continue
		}
		var chID, contactID string
		err := tx.QueryRow(`SELECT id, contact_id FROM contact_channels
            WHERE platform = 'feishu' AND channel_id = ?`, sr.OpenID).Scan(&chID, &contactID)
		switch {
		case err == sql.ErrNoRows:
			// No channel yet → create a degree-2 contact + a linked channel. Nickname
			// is empty (messages carry no name); the roster path fills it in later.
			cID := newID()
			if _, err := tx.Exec(`INSERT INTO contacts (`+contactCols+`)
                VALUES (?, '', '', '', '', '', '', 2, ?, ?)`,
				cID, now, now); err != nil {
				return 0, err
			}
			if _, err := tx.Exec(`INSERT INTO contact_channels
                (id, contact_id, platform, channel_id, nickname, session_id, tenant_key, last_seen, created_at, updated_at)
                VALUES (?, ?, 'feishu', ?, '', ?, ?, ?, ?, ?)`,
				newID(), cID, sr.OpenID, sessionID, sr.TenantKey, lastSeen, now, now); err != nil {
				return 0, err
			}
			created++
		case err != nil:
			return 0, err
		case contactID == "":
			// Channel exists but unlinked → create a degree-2 contact, link it, refresh
			// session/last_seen + tenant_key. Never clobber a non-empty nickname.
			cID := newID()
			if _, err := tx.Exec(`INSERT INTO contacts (`+contactCols+`)
                VALUES (?, '', '', '', '', '', '', 2, ?, ?)`,
				cID, now, now); err != nil {
				return 0, err
			}
			if _, err := tx.Exec(`UPDATE contact_channels SET
                contact_id = ?, session_id = ?, last_seen = ?, updated_at = ?,
                tenant_key = CASE WHEN ? != '' THEN ? ELSE tenant_key END
                WHERE id = ?`,
				cID, sessionID, lastSeen, now, sr.TenantKey, sr.TenantKey, chID); err != nil {
				return 0, err
			}
			created++
		default:
			// Already linked → only refresh session/last_seen + tenant_key; never relink,
			// never change degree, never clobber a non-empty nickname.
			if _, err := tx.Exec(`UPDATE contact_channels SET
                session_id = ?, last_seen = ?, updated_at = ?,
                tenant_key = CASE WHEN ? != '' THEN ? ELSE tenant_key END
                WHERE id = ?`,
				sessionID, lastSeen, now, sr.TenantKey, sr.TenantKey, chID); err != nil {
				return 0, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return created, nil
}

// RosterNameMap returns the open_id → nickname map for a tracked group's cached
// roster (feishu_group_members), skipping empty nicknames. This is the name
// cache that replaces a per-sync chat.members call: SyncChat enriches message
// sender names from it.
func (s *ContactStore) RosterNameMap(sessionID string) (map[string]string, error) {
	rows, err := s.db.sql.Query(`SELECT channel_id, nickname FROM feishu_group_members
        WHERE session_id = ? AND nickname != ''`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	names := map[string]string{}
	for rows.Next() {
		var openID, nick string
		if err := rows.Scan(&openID, &nick); err != nil {
			return nil, err
		}
		names[openID] = nick
	}
	return names, rows.Err()
}

// RosterMember is one feishu_group_members row exposed to the API: the member's
// open_id, cached nickname, and org (tenant_key).
type RosterMember struct {
	OpenID    string `json:"openId"`
	Nickname  string `json:"nickname"`
	TenantKey string `json:"tenantKey"`
}

// GroupMembersForSession returns the recorded roster for a tracked group,
// ordered by nickname.
func (s *ContactStore) GroupMembersForSession(sessionID string) ([]RosterMember, error) {
	rows, err := s.db.sql.Query(`SELECT channel_id, nickname, tenant_key
        FROM feishu_group_members WHERE session_id = ? ORDER BY nickname, channel_id`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RosterMember{}
	for rows.Next() {
		var m RosterMember
		if err := rows.Scan(&m.OpenID, &m.Nickname, &m.TenantKey); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// MemberCountForSession returns the number of roster members recorded for a
// tracked group (feishu_group_members rows with session_id = sessionID).
func (s *ContactStore) MemberCountForSession(sessionID string) (int, error) {
	var n int
	err := s.db.sql.QueryRow(`SELECT COUNT(*) FROM feishu_group_members WHERE session_id = ?`, sessionID).Scan(&n)
	return n, err
}

// GroupsForChannel returns the session_ids (tracked groups) a channel identity
// (open_id) belongs to, per the recorded rosters.
func (s *ContactStore) GroupsForChannel(channelID string) ([]string, error) {
	rows, err := s.db.sql.Query(`SELECT session_id FROM feishu_group_members WHERE channel_id = ? ORDER BY session_id`, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var sid string
		if err := rows.Scan(&sid); err != nil {
			return nil, err
		}
		out = append(out, sid)
	}
	return out, rows.Err()
}
