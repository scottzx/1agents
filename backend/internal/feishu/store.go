// Package feishu syncs Feishu/Lark chat messages into a local SQLite store
// (sync.db) by shelling out to the already-authenticated `lark-cli`. Raw,
// high-churn message data lives here, separate from the curated meta.db work
// state (tasks/discussions), so message ingestion never contends with the
// interactive task UI. The unified_messages schema is channel-aware
// (channel='feishu') so WeChat/email can be added later as sibling syncers.
package feishu

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	// With a limit we want the LATEST N messages, not the oldest N — so order
	// DESC + LIMIT, then reverse to ascending below for chat-timeline display.
	// Without a limit, ASC returns the whole history in order.
	order := "ASC"
	if limit > 0 {
		order = "DESC"
	}
	q := `SELECT channel, channel_acc_id, message_id, session_id, sender_id, sender_name, msg_type, title, content, create_time
        FROM unified_messages
        WHERE channel = ? AND session_id = ? AND create_time >= ?
        ORDER BY create_time ` + order + `, message_id ` + order
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
	if order == "DESC" {
		reverseMessages(out)
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

// SenderInfo is one distinct chat participant discovered from synced messages:
// their open_id, latest display name, the session they were last seen in, and
// that latest timestamp. Used by the contacts module to auto-discover channel
// identities (Feishu can't return phones for external group members).
type SenderInfo struct {
	SenderID   string
	SenderName string
	SessionID  string
	LastSeen   int64
}

// DistinctSenders returns one row per distinct sender_id on a channel, carrying
// the name/session from that sender's most recent message and the max
// create_time. Senders with an empty open_id (system messages) are excluded.
func (s *Store) DistinctSenders(channel string) ([]SenderInfo, error) {
	// For each sender pick the latest message's name + session via a correlated
	// max(create_time); GROUP BY then collapses to one row per sender.
	rows, err := s.sql.Query(`SELECT m.sender_id, m.sender_name, m.session_id, m.create_time
        FROM unified_messages m
        JOIN (
            SELECT sender_id, MAX(create_time) AS mt
            FROM unified_messages
            WHERE channel = ? AND sender_id != ''
            GROUP BY sender_id
        ) latest ON latest.sender_id = m.sender_id AND latest.mt = m.create_time
        WHERE m.channel = ? AND m.sender_id != ''
        GROUP BY m.sender_id
        ORDER BY m.create_time DESC`, channel, channel)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SenderInfo{}
	for rows.Next() {
		var si SenderInfo
		if err := rows.Scan(&si.SenderID, &si.SenderName, &si.SessionID, &si.LastSeen); err != nil {
			return nil, err
		}
		out = append(out, si)
	}
	return out, rows.Err()
}

// MessagesBySenders returns a channel's messages authored by any of senderIDs,
// in ascending time order. Used to assemble a contact's cross-group messages.
// limit <= 0 means no limit; an empty senderIDs returns no rows.
func (s *Store) MessagesBySenders(channel string, senderIDs []string, limit int) ([]Message, error) {
	if len(senderIDs) == 0 {
		return []Message{}, nil
	}
	placeholders := strings.Repeat("?,", len(senderIDs))
	placeholders = placeholders[:len(placeholders)-1]
	// Latest N when limited (DESC + LIMIT, reversed below); full history when not.
	order := "ASC"
	if limit > 0 {
		order = "DESC"
	}
	q := `SELECT channel, channel_acc_id, message_id, session_id, sender_id, sender_name, msg_type, title, content, create_time
        FROM unified_messages
        WHERE channel = ? AND sender_id IN (` + placeholders + `)
        ORDER BY create_time ` + order + `, message_id ` + order
	args := []any{channel}
	for _, id := range senderIDs {
		args = append(args, id)
	}
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
	if order == "DESC" {
		reverseMessages(out)
	}
	return out, rows.Err()
}

// reverseMessages flips a slice in place — used to turn a DESC (latest-first)
// query result back into ascending chat-timeline order.
func reverseMessages(m []Message) {
	for i, j := 0, len(m)-1; i < j; i, j = i+1, j-1 {
		m[i], m[j] = m[j], m[i]
	}
}

// SessionSummary is one chat session's overview: its id, a display name, the
// latest message's preview/time, and the total message count. Used by the
// contacts 消息 tab session list.
type SessionSummary struct {
	SessionID   string
	SessionName string
	LastPreview string
	LastTime    int64
	Count       int
}

// SessionSummaries returns a summary per session on a channel, built on top of
// ListSessions: per-session count + latest message time/preview.
func (s *Store) SessionSummaries(channel string) ([]SessionSummary, error) {
	sessions, err := s.ListSessions(channel)
	if err != nil {
		return nil, err
	}
	out := make([]SessionSummary, 0, len(sessions))
	for _, sid := range sessions {
		count, err := s.CountMessages(channel, sid)
		if err != nil {
			return nil, err
		}
		// sync.db has no session display-name source (chat title is not
		// persisted here), so SessionName falls back to the session id. The
		// contacts handler overlays tracked-chat names (meta.db v17) on top.
		sum := SessionSummary{SessionID: sid, SessionName: sid, Count: count}
		var name, content, msgType string
		err = s.sql.QueryRow(`SELECT sender_name, content, msg_type, create_time
            FROM unified_messages
            WHERE channel = ? AND session_id = ?
            ORDER BY create_time DESC, message_id DESC LIMIT 1`,
			channel, sid).Scan(&name, &content, &msgType, &sum.LastTime)
		if err != nil && err != sql.ErrNoRows {
			return nil, err
		}
		if err == nil {
			sum.LastPreview = previewText(name, msgType, content)
		}
		out = append(out, sum)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastTime > out[j].LastTime })
	return out, nil
}

// previewText builds a short, single-line preview for a session's latest
// message. Best-effort: only text bodies are unwrapped; other types degrade to
// a "[type]" placeholder. Mirrors the digest renderer's intent without coupling.
func previewText(senderName, msgType, content string) string {
	body := "[" + msgType + "]"
	if msgType == "text" {
		var t struct {
			Text string `json:"text"`
		}
		if json.Unmarshal([]byte(content), &t) == nil && t.Text != "" {
			body = t.Text
		}
	}
	body = strings.ReplaceAll(body, "\n", " ")
	if senderName != "" {
		return senderName + ": " + body
	}
	return body
}
