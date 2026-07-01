// Package imessage reads the local macOS Messages database (chat.db) into the
// unified, channel-agnostic message model (channel='imessage'), a sibling of the
// feishu syncer. chat.db is plaintext SQLite gated by the user's Full Disk Access
// grant — no decryption or process-memory reads happen here. The reader only ever
// opens the DB read-only; ingestion into sync.db/meta.db lives in the orchestrator.
package imessage

import (
	"bytes"
	"database/sql"
	"encoding/binary"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Channel is the channel discriminator written into every unified_messages row
// this source produces.
const Channel = "imessage"

// appleEpochOffsetMs is the gap between the Unix epoch (1970-01-01) and the Mac
// "absolute time" epoch (2001-01-01), in milliseconds. chat.db's message.date is
// measured from the Mac epoch.
const appleEpochOffsetMs int64 = 978307200000

// Message is one chat message read from chat.db, in the unified model's terms.
type Message struct {
	GUID       string // message.guid — unique message id
	ChatGUID   string // chat.guid — conversation/session id
	Handle     string // counterpart's phone (E.164) or email; "" for system rows
	IsFromMe   bool   // true when the local user sent it
	Text       string // resolved body: text column, else decoded attributedBody
	Service    string // 'iMessage' | 'SMS'
	HasAttach  bool   // message carried an attachment
	CreateTime int64  // creation time, epoch milliseconds
	AppleDate  int64  // raw message.date (Mac absolute time) — used as the watermark
}

// DefaultPath returns ~/Library/Messages/chat.db.
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Library", "Messages", "chat.db")
}

// Read returns chat.db messages with date > sinceApple (raw Mac absolute time),
// oldest first, up to limit rows (<= 0 means no limit). It also returns the max
// raw date seen, which the caller stores as the next watermark so subsequent
// pulls walk forward incrementally. Opens the DB read-only so it never contends
// with Messages.app.
func Read(dbPath string, sinceApple int64, limit int) (msgs []Message, maxApple int64, err error) {
	if _, statErr := os.Stat(dbPath); statErr != nil {
		return nil, 0, fmt.Errorf("imessage: chat.db not found at %s: %w", dbPath, statErr)
	}
	dsn := "file:" + url.PathEscape(dbPath) + "?mode=ro&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, 0, fmt.Errorf("imessage: open chat.db: %w", err)
	}
	defer db.Close()

	q := `SELECT m.guid, c.guid, IFNULL(h.id, ''), m.is_from_me, m.text, m.attributedBody,
            m.date, IFNULL(m.service, ''), IFNULL(m.cache_has_attachments, 0)
        FROM message m
        JOIN chat_message_join cmj ON cmj.message_id = m.ROWID
        JOIN chat c ON c.ROWID = cmj.chat_id
        LEFT JOIN handle h ON h.ROWID = m.handle_id
        WHERE m.date > ?
        ORDER BY m.date ASC`
	args := []any{sinceApple}
	if limit > 0 {
		q += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := db.Query(q, args...)
	if err != nil {
		// A bare "unable to open database file" / "authorization denied" here is
		// almost always the missing Full Disk Access grant — surface it plainly.
		return nil, 0, fmt.Errorf("imessage: query chat.db (Full Disk Access granted?): %w", err)
	}
	defer rows.Close()

	out := []Message{}
	for rows.Next() {
		var (
			m       Message
			fromMe  int
			text    sql.NullString
			attr    []byte
			service string
			attach  int
		)
		if err := rows.Scan(&m.GUID, &m.ChatGUID, &m.Handle, &fromMe, &text, &attr,
			&m.AppleDate, &service, &attach); err != nil {
			return nil, 0, err
		}
		m.IsFromMe = fromMe != 0
		m.Service = service
		m.HasAttach = attach != 0
		m.CreateTime = appleToUnixMs(m.AppleDate)
		if text.Valid && text.String != "" {
			m.Text = text.String
		} else {
			m.Text = decodeAttributedBody(attr)
		}
		if m.AppleDate > maxApple {
			maxApple = m.AppleDate
		}
		out = append(out, m)
	}
	return out, maxApple, rows.Err()
}

// UnixMsToAppleDate converts epoch milliseconds to a chat.db message.date
// (nanoseconds since the Mac epoch) — used to translate a "last N days" crawl
// rule into a watermark floor for Read.
func UnixMsToAppleDate(ms int64) int64 {
	return (ms - appleEpochOffsetMs) * 1_000_000
}

// appleToUnixMs converts a chat.db message.date to epoch milliseconds. Modern
// macOS stores nanoseconds since the Mac epoch; pre-High-Sierra DBs stored
// seconds. The two ranges are orders of magnitude apart, so the magnitude tells
// them apart (recent ns ≈ 7e17, recent seconds ≈ 7e8).
func appleToUnixMs(d int64) int64 {
	if d == 0 {
		return 0
	}
	if d > 1_000_000_000_000 { // nanoseconds form
		return d/1_000_000 + appleEpochOffsetMs
	}
	return d*1000 + appleEpochOffsetMs // legacy seconds form
}

// decodeAttributedBody best-effort extracts the plain message text from a
// message.attributedBody blob (an NSAttributedString serialized as a NeXT/Apple
// "typedstream"), used when the text column is NULL — the common case on recent
// macOS. The text follows the "NSString" class marker and a 0x2B ('+') tag, then
// a typedstream-encoded length (single byte, or 0x81/0x82 prefixing a 2-/4-byte
// little-endian length) and the UTF-8 bytes. Returns "" when the blob doesn't
// match this shape (e.g. attachment-only or richer payloads).
func decodeAttributedBody(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	i := bytes.Index(b, []byte("NSString"))
	if i < 0 {
		return ""
	}
	// The string-start tag 0x2B sits a few bytes after the class name; scan a
	// short window for it rather than assuming a fixed offset.
	start := i + len("NSString")
	tag := -1
	for k := start; k < len(b) && k < start+10; k++ {
		if b[k] == 0x2B {
			tag = k
			break
		}
	}
	if tag < 0 {
		return ""
	}
	p := tag + 1
	if p >= len(b) {
		return ""
	}
	length := int(b[p])
	p++
	switch length {
	case 0x81:
		if p+2 > len(b) {
			return ""
		}
		length = int(binary.LittleEndian.Uint16(b[p : p+2]))
		p += 2
	case 0x82:
		if p+4 > len(b) {
			return ""
		}
		length = int(binary.LittleEndian.Uint32(b[p : p+4]))
		p += 4
	}
	if length <= 0 || p+length > len(b) {
		return ""
	}
	return string(b[p : p+length])
}
