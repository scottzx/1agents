// Package feishu syncs Feishu/Lark chat messages into a local SQLite store
// (sync.db) by shelling out to the already-authenticated `lark-cli`. Raw,
// high-churn message data lives here, separate from the curated meta.db work
// state (tasks/discussions), so message ingestion never contends with the
// interactive task UI. The unified_messages schema is channel-aware
// (channel='feishu') so WeChat/email can be added later as sibling syncers.
package feishu

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

// Channel is the channel discriminator written into every row this package
// produces. Future channels (wechat/email) get their own constant.
const Channel = "feishu"

// Message is one chat message in the unified, channel-agnostic model.
type Message struct {
	Channel      string // 'feishu'
	ChannelAccID string // local account id, distinguishes multiple accounts on one channel
	MessageID    string // remote message id (om_xxx); unique within a channel
	SessionID    string // chat_id (oc_xxx) / conversation id
	SenderID     string // sender open_id
	SenderName   string // resolved via chat.members (best-effort)
	MsgType      string // text | post | image | system | ...
	Title        string // message/post title (when present)
	Content      string // raw body.content JSON
	CreateTime   int64  // creation time, epoch milliseconds
}

// Store wraps sync.db.
type Store struct {
	sql *sql.DB
}

func get1AgentsHome() string {
	if val := os.Getenv("ONEAGENTS_HOME"); val != "" {
		return val
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return home
}

// DefaultPath returns ~/.1agents/sync.db (honoring ONEAGENTS_HOME), a sibling
// of meta.db.
func DefaultPath() string {
	return filepath.Join(get1AgentsHome(), ".1agents", "sync.db")
}

// Open opens (creating if needed) the sync database and ensures the schema.
// Mirrors meta.Open's WAL + busy_timeout + single-connection setup.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("feishu: ensure db dir: %w", err)
	}
	dsn := "file:" + url.PathEscape(path) +
		"?_txlock=immediate" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=synchronous(NORMAL)"
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("feishu: open %s: %w", path, err)
	}
	sqlDB.SetMaxOpenConns(1)
	s := &Store{sql: sqlDB}
	if _, err := sqlDB.Exec(schema); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("feishu: apply schema: %w", err)
	}
	return s, nil
}

var (
	openMu    sync.Mutex
	openCache = map[string]*Store{}
)

// OpenDefault opens (or returns the cached handle for) DefaultPath(). Cached
// per resolved path so tests that switch ONEAGENTS_HOME stay isolated.
func OpenDefault() (*Store, error) {
	path := DefaultPath()
	openMu.Lock()
	defer openMu.Unlock()
	if s, ok := openCache[path]; ok {
		return s, nil
	}
	s, err := Open(path)
	if err != nil {
		return nil, err
	}
	openCache[path] = s
	return s, nil
}

// Close closes the underlying connection (CLI one-shots and tests).
func (s *Store) Close() error { return s.sql.Close() }

// schema is idempotent (CREATE IF NOT EXISTS); the watermark/message tables are
// append-only and additive, so no version-gated migration is needed yet.
const schema = `
CREATE TABLE IF NOT EXISTS unified_messages (
    channel        TEXT    NOT NULL,
    channel_acc_id TEXT    NOT NULL DEFAULT 'default',
    message_id     TEXT    NOT NULL,
    session_id     TEXT    NOT NULL,
    sender_id      TEXT    NOT NULL DEFAULT '',
    sender_name    TEXT    NOT NULL DEFAULT '',
    msg_type       TEXT    NOT NULL DEFAULT '',
    title          TEXT    NOT NULL DEFAULT '',
    content        TEXT    NOT NULL DEFAULT '',
    create_time    INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (channel, message_id)
);
CREATE INDEX IF NOT EXISTS idx_unified_messages_session
    ON unified_messages(channel, session_id, create_time);

CREATE TABLE IF NOT EXISTS unified_sync_watermarks (
    channel         TEXT    NOT NULL,
    channel_acc_id  TEXT    NOT NULL DEFAULT 'default',
    session_id      TEXT    NOT NULL,
    watermark_value TEXT    NOT NULL DEFAULT '',
    updated_at      TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (channel, channel_acc_id, session_id)
);
`

// UpsertMessages inserts messages, skipping any whose (channel, message_id)
// already exists (INSERT OR IGNORE). Returns the number of newly inserted rows.
// The time-window watermark is inclusive on its boundary, so the last-synced
// message reappears on the next pull — dedup here makes that harmless.
func (s *Store) UpsertMessages(msgs []Message) (int, error) {
	if len(msgs) == 0 {
		return 0, nil
	}
	tx, err := s.sql.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO unified_messages
        (channel, channel_acc_id, message_id, session_id, sender_id, sender_name, msg_type, title, content, create_time)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	inserted := 0
	for _, m := range msgs {
		res, err := stmt.Exec(m.Channel, m.ChannelAccID, m.MessageID, m.SessionID,
			m.SenderID, m.SenderName, m.MsgType, m.Title, m.Content, m.CreateTime)
		if err != nil {
			return 0, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			inserted++
		}
	}
	return inserted, tx.Commit()
}

// GetWatermark returns the stored high-water mark (epoch ms) for a
// (channel, account, session). ok is false when no watermark exists yet.
func (s *Store) GetWatermark(channel, accID, sessionID string) (int64, bool, error) {
	var raw string
	err := s.sql.QueryRow(`SELECT watermark_value FROM unified_sync_watermarks
        WHERE channel = ? AND channel_acc_id = ? AND session_id = ?`,
		channel, accID, sessionID).Scan(&raw)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if raw == "" {
		return 0, false, nil
	}
	var v int64
	if _, err := fmt.Sscan(raw, &v); err != nil {
		return 0, false, fmt.Errorf("feishu: parse watermark %q: %w", raw, err)
	}
	return v, true, nil
}

// SetWatermark upserts the high-water mark (epoch ms) for a (channel, account,
// session).
func (s *Store) SetWatermark(channel, accID, sessionID string, value int64, updatedAt string) error {
	_, err := s.sql.Exec(`INSERT INTO unified_sync_watermarks
        (channel, channel_acc_id, session_id, watermark_value, updated_at)
        VALUES (?, ?, ?, ?, ?)
        ON CONFLICT(channel, channel_acc_id, session_id)
        DO UPDATE SET watermark_value = excluded.watermark_value, updated_at = excluded.updated_at`,
		channel, accID, sessionID, fmt.Sprintf("%d", value), updatedAt)
	return err
}

// ListMessages returns a session's messages with create_time >= since (epoch
// ms), in ascending time order. limit <= 0 means no limit. Used to assemble an
// analysis batch.
func (s *Store) ListMessages(channel, sessionID string, since int64, limit int) ([]Message, error) {
	q := `SELECT channel, channel_acc_id, message_id, session_id, sender_id, sender_name, msg_type, title, content, create_time
        FROM unified_messages
        WHERE channel = ? AND session_id = ? AND create_time >= ?
        ORDER BY create_time ASC, message_id ASC`
	args := []any{channel, sessionID, since}
	if limit > 0 {
		q += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := s.sql.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Message{}
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.Channel, &m.ChannelAccID, &m.MessageID, &m.SessionID,
			&m.SenderID, &m.SenderName, &m.MsgType, &m.Title, &m.Content, &m.CreateTime); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListSessions returns the distinct session ids (chat ids) that have at least
// one stored message on a channel. The periodic syncer uses this to refresh
// every chat it has already seen.
func (s *Store) ListSessions(channel string) ([]string, error) {
	rows, err := s.sql.Query(`SELECT DISTINCT session_id FROM unified_messages
        WHERE channel = ? ORDER BY session_id`, channel)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// CountMessages returns how many messages a session has stored (used for
// verification/diagnostics).
func (s *Store) CountMessages(channel, sessionID string) (int, error) {
	var n int
	err := s.sql.QueryRow(`SELECT COUNT(1) FROM unified_messages
        WHERE channel = ? AND session_id = ?`, channel, sessionID).Scan(&n)
	return n, err
}
