package meta

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// InboxStore is the SQLite-backed Workspace Inbox (#202 / #60). Every row
// belongs to a recipient Workspace. Deliver is the single write path for
// function / agent / human / channel producers. Archiving never deletes — it
// flips Status so the "what did this become" trail survives.
type InboxStore struct {
	db *DB
}

// NewInboxStore returns an InboxStore over db.
func NewInboxStore(db *DB) *InboxStore {
	return &InboxStore{db: db}
}

// inboxCols is the SELECT/INSERT column list for inbox_items (Workspace envelope
// fields included; order matches scanInboxItem).
const inboxCols = `id, workspace_id, source, title, content, url, summary, tags, status,
	from_workspace_id, from_ref, payload, created_at, updated_at`

func scanInboxItem(r rowScanner) (InboxItem, error) {
	var it InboxItem
	var tags, payload, createdAt, updatedAt string
	if err := r.Scan(
		&it.ID, &it.WorkspaceID, &it.Source, &it.Title, &it.Content, &it.URL,
		&it.Summary, &tags, &it.Status, &it.FromWorkspaceID, &it.FromRef, &payload,
		&createdAt, &updatedAt,
	); err != nil {
		return InboxItem{}, err
	}
	it.Tags = jsonToStrings(tags)
	if strings.TrimSpace(payload) != "" {
		it.Payload = json.RawMessage(payload)
	}
	it.CreatedAt = strToTime(createdAt)
	it.UpdatedAt = strToTime(updatedAt)
	return it, nil
}

// normalizeSource maps an empty/unknown source onto a known constant so the
// list never carries free-form channel strings. Unknown channels collapse to
// "misc".
func normalizeSource(s string) string {
	switch s {
	case InboxSourceManual, InboxSourceAgent, InboxSourceFunction,
		InboxSourceIM, InboxSourceEmail, InboxSourceRSS,
		InboxSourceDataSource, InboxSourceMisc:
		return s
	default:
		return InboxSourceMisc
	}
}

// Deliver is the unique write entry for a Workspace Inbox: insert one envelope
// into the recipient workspace. WorkspaceID is required. Empty Source becomes
// misc; empty Status becomes unread.
func (s *InboxStore) Deliver(item InboxItem) (InboxItem, error) {
	item.WorkspaceID = strings.TrimSpace(item.WorkspaceID)
	if item.WorkspaceID == "" {
		return InboxItem{}, fmt.Errorf("meta: inbox deliver requires workspaceId")
	}
	if strings.TrimSpace(item.Title) == "" && strings.TrimSpace(item.Content) == "" &&
		strings.TrimSpace(item.URL) == "" {
		return InboxItem{}, fmt.Errorf("meta: inbox deliver requires title, content or url")
	}
	if item.ID == "" {
		item.ID = newID()
	}
	item.Source = normalizeSource(item.Source)
	if item.Status == "" {
		item.Status = InboxStatusUnread
	}
	item.FromWorkspaceID = strings.TrimSpace(item.FromWorkspaceID)
	item.FromRef = strings.TrimSpace(item.FromRef)
	now := time.Now().UTC()
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	item.UpdatedAt = now
	payload := ""
	if len(item.Payload) > 0 && string(item.Payload) != "null" {
		payload = string(item.Payload)
	}
	if _, err := s.db.sql.Exec(`
		INSERT INTO inbox_items (`+inboxCols+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.WorkspaceID, item.Source, item.Title, item.Content, item.URL,
		item.Summary, stringsToJSON(item.Tags), item.Status,
		item.FromWorkspaceID, item.FromRef, payload,
		timeToStr(item.CreatedAt), timeToStr(item.UpdatedAt)); err != nil {
		return InboxItem{}, err
	}
	return item, nil
}

// Capture inserts a manual-style intake item. WorkspaceID defaults to the
// builtin default assistant when empty (legacy callers / migration path).
// Prefer Deliver for explicit cross-workspace and agent/function producers.
func (s *InboxStore) Capture(item InboxItem) (InboxItem, error) {
	if strings.TrimSpace(item.WorkspaceID) == "" {
		item.WorkspaceID = DefaultInboxWorkspaceID
	}
	if item.Source == "" {
		item.Source = InboxSourceManual
	}
	return s.Deliver(item)
}

// ListByWorkspace returns intake items for one Workspace, newest-first.
// Archived items are excluded unless includeArchived is set.
func (s *InboxStore) ListByWorkspace(workspaceID string, includeArchived bool) ([]InboxItem, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return nil, fmt.Errorf("meta: list inbox requires workspaceId")
	}
	query := `SELECT ` + inboxCols + ` FROM inbox_items WHERE workspace_id = ?`
	if !includeArchived {
		query += ` AND status != '` + InboxStatusArchived + `'`
	}
	query += ` ORDER BY created_at DESC`
	rows, err := s.db.sql.Query(query, workspaceID)
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

// List returns intake items newest-first across all workspaces (legacy/global
// view). Prefer ListByWorkspace. Archived items are excluded unless
// includeArchived is set.
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

// UnreadCount is the global badge number (all workspaces). Prefer
// UnreadCountByWorkspace for per-box badges.
func (s *InboxStore) UnreadCount() (int, error) {
	var n int
	err := s.db.sql.QueryRow(`SELECT COUNT(1) FROM inbox_items WHERE status = ?`,
		InboxStatusUnread).Scan(&n)
	return n, err
}

// UnreadCountByWorkspace is the per-Workspace badge number.
func (s *InboxStore) UnreadCountByWorkspace(workspaceID string) (int, error) {
	var n int
	err := s.db.sql.QueryRow(
		`SELECT COUNT(1) FROM inbox_items WHERE workspace_id = ? AND status = ?`,
		strings.TrimSpace(workspaceID), InboxStatusUnread).Scan(&n)
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
// messages to the store as InboxItems; Deliver is the single write path.
type InboxSource interface {
	// Name identifies the channel (e.g. "feishu"); recorded loosely, normalized
	// to one of the InboxSource* constants on Deliver.
	Name() string
	// Drain returns the items captured since the last call and clears them. An
	// empty slice means nothing new.
	Drain() ([]InboxItem, error)
}

// IngestFrom pulls every pending item out of src and delivers it. Returns the
// number captured. Items without a WorkspaceID land on the default assistant.
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
