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
	ID        string           `json:"id"`
	Phone     string           `json:"phone"`
	Name      string           `json:"name"`
	Company   string           `json:"company"`
	Title     string           `json:"title"`
	Note      string           `json:"note"`
	Tags      []string         `json:"tags"`
	CreatedAt time.Time        `json:"createdAt"`
	UpdatedAt time.Time        `json:"updatedAt"`
	Channels  []ContactChannel `json:"channels,omitempty"`
}

// ContactChannel binds a synced channel identity (platform + channelId, e.g. a
// Feishu open_id) to a contact. channelId is auto-discovered from messages;
// contactId is set when the user links it. sessionId records the last group the
// identity was seen in (best-effort context).
type ContactChannel struct {
	ID        string    `json:"id"`
	ContactID string    `json:"contactId"`
	Platform  string    `json:"platform"`
	ChannelID string    `json:"channelId"`
	Nickname  string    `json:"nickname"`
	SessionID string    `json:"sessionId"`
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

const contactCols = `id, phone, name, company, title, note, tags, created_at, updated_at`

func scanContact(r rowScanner) (Contact, error) {
	var c Contact
	var tags, createdAt, updatedAt string
	if err := r.Scan(&c.ID, &c.Phone, &c.Name, &c.Company, &c.Title, &c.Note, &tags, &createdAt, &updatedAt); err != nil {
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

const contactChannelCols = `id, contact_id, platform, channel_id, nickname, session_id, last_seen, created_at, updated_at`

func scanContactChannel(r rowScanner) (ContactChannel, error) {
	var cc ContactChannel
	var createdAt, updatedAt string
	if err := r.Scan(&cc.ID, &cc.ContactID, &cc.Platform, &cc.ChannelID, &cc.Nickname,
		&cc.SessionID, &cc.LastSeen, &createdAt, &updatedAt); err != nil {
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
	rows, err := s.db.sql.Query(`SELECT ` + contactCols + ` FROM contacts ORDER BY name, created_at`)
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
func (s *ContactStore) ContactsWithChannels() ([]Contact, error) {
	contacts, err := s.ListContacts()
	if err != nil {
		return nil, err
	}
	for i := range contacts {
		chans, err := s.ListChannelsForContact(contacts[i].ID)
		if err != nil {
			return nil, err
		}
		contacts[i].Channels = chans
	}
	return contacts, nil
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
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err := s.db.sql.Exec(`INSERT INTO contacts (`+contactCols+`)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.Phone, c.Name, c.Company, c.Title, c.Note, joinTags(c.Tags),
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
		return s.queryChannels(`SELECT `+contactChannelCols+` FROM contact_channels
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
        (id, contact_id, platform, channel_id, nickname, session_id, last_seen, created_at, updated_at)
        VALUES (?, '', ?, ?, ?, ?, ?, ?, ?)
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
