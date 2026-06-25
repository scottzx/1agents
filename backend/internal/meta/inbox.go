package meta

import (
	"database/sql"
	"strings"
	"time"
)

// InboxStore is the SQLite-backed intake list for the Inbox 收口层 (#60). It is
// deliberately source-agnostic: every channel (manual capture today; cc-connect
// IM / email / RSS later) lands a row through Capture. Archiving never deletes —
// it flips Status so the "what did this become" trail survives.
type InboxStore struct {
	db *DB
}

// NewInboxStore returns an InboxStore over db.
func NewInboxStore(db *DB) *InboxStore {
	return &InboxStore{db: db}
}

const inboxCols = `id, source, title, content, url, summary, tags, status, created_at, updated_at`

func scanInboxItem(r rowScanner) (InboxItem, error) {
	var it InboxItem
	var tags, createdAt, updatedAt string
	if err := r.Scan(&it.ID, &it.Source, &it.Title, &it.Content, &it.URL,
		&it.Summary, &tags, &it.Status, &createdAt, &updatedAt); err != nil {
		return InboxItem{}, err
	}
	it.Tags = jsonToStrings(tags)
	it.CreatedAt = strToTime(createdAt)
	it.UpdatedAt = strToTime(updatedAt)
	return it, nil
}

// normalizeSource maps an empty/unknown source onto a known constant so the
// list never carries free-form channel strings. Unknown channels collapse to
// "misc" (the杂项 bucket).
func normalizeSource(s string) string {
	switch s {
	case InboxSourceManual, InboxSourceIM, InboxSourceEmail, InboxSourceRSS, InboxSourceMisc:
		return s
	default:
		return InboxSourceMisc
	}
}

// Capture inserts a new intake item. ID is assigned when empty; Source is
// normalized; Status defaults to unread. This is the single write path every
// Source feeds, so the manual-capture HTTP handler and an injected cc-connect
// bridge share it.
func (s *InboxStore) Capture(item InboxItem) (InboxItem, error) {
	if item.ID == "" {
		item.ID = newID()
	}
	item.Source = normalizeSource(item.Source)
	if item.Status == "" {
		item.Status = InboxStatusUnread
	}
	now := time.Now().UTC()
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	item.UpdatedAt = now
	if _, err := s.db.sql.Exec(`
		INSERT INTO inbox_items (`+inboxCols+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.Source, item.Title, item.Content, item.URL, item.Summary,
		stringsToJSON(item.Tags), item.Status,
		timeToStr(item.CreatedAt), timeToStr(item.UpdatedAt)); err != nil {
		return InboxItem{}, err
	}
	return item, nil
}

// List returns intake items newest-first. Archived items are excluded unless
// includeArchived is set (the UI's 显示已归档 toggle passes true).
func (s *InboxStore) List(includeArchived bool) ([]InboxItem, error) {
	query := `SELECT ` + inboxCols + ` FROM inbox_items`
	if !includeArchived {
		query += ` WHERE status != '` + InboxStatusArchived + `'`
	}
	query += ` ORDER BY created_at DESC`
	rows, err := s.db.sql.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []InboxItem{}
	for rows.Next() {
		it, err := scanInboxItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// UnreadCount is the badge number: items still in the unread status.
func (s *InboxStore) UnreadCount() (int, error) {
	var n int
	err := s.db.sql.QueryRow(`SELECT COUNT(1) FROM inbox_items WHERE status = ?`,
		InboxStatusUnread).Scan(&n)
	return n, err
}

// Get returns a single item, or (zero, false) when not found.
func (s *InboxStore) Get(id string) (InboxItem, bool, error) {
	row := s.db.sql.QueryRow(`SELECT `+inboxCols+` FROM inbox_items WHERE id = ?`, id)
	it, err := scanInboxItem(row)
	if err == sql.ErrNoRows {
		return InboxItem{}, false, nil
	}
	if err != nil {
		return InboxItem{}, false, err
	}
	return it, true, nil
}

// SetStatus flips an item's status (unread / read / archived). Returns
// ErrNotFound when the id is unknown. Archiving is a status flip, not a delete.
func (s *InboxStore) SetStatus(id, status string) (InboxItem, error) {
	switch status {
	case InboxStatusUnread, InboxStatusRead, InboxStatusArchived:
	default:
		return InboxItem{}, ErrNotFound
	}
	res, err := s.db.sql.Exec(`UPDATE inbox_items SET status = ?, updated_at = ? WHERE id = ?`,
		status, timeToStr(time.Now().UTC()), id)
	if err != nil {
		return InboxItem{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return InboxItem{}, ErrNotFound
	}
	it, _, err := s.Get(id)
	return it, err
}

// InboxSource is the injectable seam for real intake channels (cc-connect IM,
// email, RSS — #60 Phase B). An adapter implements Drain to hand its pending
// messages to the store as InboxItems; Capture is the single write path. Kept
// here so callers can register a source without the store importing any channel
// package (the cc-connect submodule stays a read-only参照).
type InboxSource interface {
	// Name identifies the channel (e.g. "feishu"); recorded loosely, normalized
	// to one of the InboxSource* constants on Capture.
	Name() string
	// Drain returns the items captured since the last call and clears them. An
	// empty slice means nothing new.
	Drain() ([]InboxItem, error)
}

// IngestFrom pulls every pending item out of src and captures it. The manual
// HTTP path does not use this; it exists so a future cc-connect bridge can be
// wired in without touching the store internals. Returns the number captured.
func (s *InboxStore) IngestFrom(src InboxSource) (int, error) {
	items, err := src.Drain()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, it := range items {
		if strings.TrimSpace(it.Title) == "" && strings.TrimSpace(it.Content) == "" {
			continue
		}
		if _, err := s.Capture(it); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
