// Package data owns the silver + gold layers of the multi-source medallion in a
// dedicated data.db, sibling to bronze (sync.db, package sources) and the app
// metadata (meta.db, package meta). Keeping the normalized personal-data domains
// (contacts / messages / calendar / todos) in their own file separates
// high-churn ingested data from core app state, so the data lake can be backed
// up / exported / wiped independently.
//
// Two transform stages feed this store, each re-runnable and network-free:
//
//	bronze (source_records)  --SilverXxx-->  silver_{contacts,messages,events,todos}
//	silver                   --GoldXxx---->  gold contacts/messages/calendar_events/todos
//
// Silver rows are per-source, conformed (typed columns, still source-native
// identifiers, no cross-entity links), keyed by (source, account_id,
// external_id) — the same grain as a bronze record. Gold rows are the fused,
// enriched canonical entities: people are merged across sources (phone/email),
// message senders and event attendees resolve to a contact_id, messages group
// into threads. Cross-source dedup of non-people entities is a v2 concern; each
// gold "thing" carries a fingerprint column so that pass needs no backfill.
package data

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Store wraps data.db (silver + gold + transform cursors).
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

// DefaultPath returns ~/.1agents/data.db (honoring ONEAGENTS_HOME), a sibling of
// meta.db and sync.db.
func DefaultPath() string {
	return filepath.Join(get1AgentsHome(), ".1agents", "data.db")
}

// Open opens (creating if needed) data.db and ensures the schema. Mirrors
// feishu.Open / meta.Open (WAL + busy_timeout + single connection).
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("data: ensure db dir: %w", err)
	}
	dsn := "file:" + url.PathEscape(path) +
		"?_txlock=immediate" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=synchronous(NORMAL)"
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("data: open %s: %w", path, err)
	}
	sqlDB.SetMaxOpenConns(1)
	if _, err := sqlDB.Exec(schema); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("data: apply schema: %w", err)
	}
	return &Store{sql: sqlDB}, nil
}

var (
	openMu    sync.Mutex
	openCache = map[string]*Store{}
)

// OpenDefault opens (or returns the cached handle for) DefaultPath(). Cached per
// resolved path so tests that switch ONEAGENTS_HOME stay isolated (mirrors
// feishu/meta).
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

// SQL exposes the underlying data.db handle for sibling stores in this package.
func (s *Store) SQL() *sql.DB { return s.sql }

// Transform stage discriminators for data_cursors.
const (
	StageSilver = "silver" // bronze → silver watermark (over bronze.fetched_at)
	StageGold   = "gold"   // silver → gold watermark (over silver.updated_at)
)

// GovernCursor reads the high-water mark (epoch ms) a transform stage has
// consumed for a (source, kind). 0 (and a full re-run) is always safe because
// every silver/gold upsert is idempotent.
func (s *Store) GovernCursor(stage, source, kind string) (int64, error) {
	var wm int64
	err := s.sql.QueryRow(`SELECT watermark FROM data_cursors
        WHERE stage = ? AND source = ? AND kind = ?`, stage, source, kind).Scan(&wm)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return wm, nil
}

// SaveGovernCursor persists a transform stage's high-water mark for (source, kind).
func (s *Store) SaveGovernCursor(stage, source, kind string, watermark int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.sql.Exec(`INSERT INTO data_cursors (stage, source, kind, watermark, updated_at)
        VALUES (?, ?, ?, ?, ?)
        ON CONFLICT(stage, source, kind) DO UPDATE SET
            watermark  = excluded.watermark,
            updated_at = excluded.updated_at`,
		stage, source, kind, watermark, now)
	return err
}

// schema is idempotent (CREATE IF NOT EXISTS); every table is additive, so no
// version-gated migration is needed (mirrors the bronze store convention).
const schema = `
-- ============================ SILVER (清洗) ============================
-- Per-SOURCE physical tables: each source keeps its own native columns so no
-- valuable field is flattened away (Apple birthday/nickname, 飞书 @mentions +
-- reply chain, MS todo recurrence/checklist). Cleaning is source-specific; the
-- viewer groups these by domain and the schema-free grid tolerates the differing
-- shapes. Cross-source unification is gold (step 3), not here. Every table keys
-- on the bronze grain (source, account_id, external_id) and carries updated_at
-- (bronze fetched_at) for the silver→gold cursor.

-- 联系人 · Apple/iCloud — lossless vCard: promoted columns + raw_props catch-all.
CREATE TABLE IF NOT EXISTS silver_icloud_contacts (
    source       TEXT    NOT NULL DEFAULT 'icloud',
    account_id   TEXT    NOT NULL DEFAULT 'default',
    external_id  TEXT    NOT NULL,               -- vCard href / UID
    full_name    TEXT    NOT NULL DEFAULT '',    -- FN
    family_name  TEXT    NOT NULL DEFAULT '',    -- N[0]
    given_name   TEXT    NOT NULL DEFAULT '',    -- N[1]
    phones       TEXT    NOT NULL DEFAULT '[]',  -- JSON (all TEL)
    emails       TEXT    NOT NULL DEFAULT '[]',  -- JSON (all EMAIL)
    org          TEXT    NOT NULL DEFAULT '',
    title        TEXT    NOT NULL DEFAULT '',
    birthday     TEXT    NOT NULL DEFAULT '',    -- BDAY (kept verbatim; may be --MM-DD)
    nickname     TEXT    NOT NULL DEFAULT '',
    note         TEXT    NOT NULL DEFAULT '',
    im_handles   TEXT    NOT NULL DEFAULT '[]',  -- JSON (IMPP)
    urls         TEXT    NOT NULL DEFAULT '[]',  -- JSON (URL)
    addresses    TEXT    NOT NULL DEFAULT '[]',  -- JSON (ADR)
    -- No raw_props catch-all: bronze (source_records) already holds the verbatim
    -- vCard, so any un-promoted property is recoverable by re-governing — silver
    -- keeps only the cleaned, promoted columns rather than re-duplicating bronze.
    deleted      INTEGER NOT NULL DEFAULT 0,
    updated_at   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (source, account_id, external_id)
);
CREATE INDEX IF NOT EXISTS idx_silver_icloud_contacts_wm ON silver_icloud_contacts(updated_at);

-- 联系人 · 飞书 二级用户 — every OpenID discovered from message senders + @mentions.
-- Aggregated across the message stream (name comes free from mentions[].name).
CREATE TABLE IF NOT EXISTS silver_feishu_users (
    source          TEXT    NOT NULL DEFAULT 'feishu',
    account_id      TEXT    NOT NULL DEFAULT 'default',
    external_id     TEXT    NOT NULL,            -- open_id
    name            TEXT    NOT NULL DEFAULT '', -- best-known display name (from a mention)
    tenant_key      TEXT    NOT NULL DEFAULT '',
    discovered_via  TEXT    NOT NULL DEFAULT '[]', -- JSON set: ["sender","mention"]
    chat_ids        TEXT    NOT NULL DEFAULT '[]', -- JSON set of chats seen in
    as_sender_count INTEGER NOT NULL DEFAULT 0,
    first_seen      INTEGER NOT NULL DEFAULT 0,  -- min create_time
    last_seen       INTEGER NOT NULL DEFAULT 0,  -- max create_time
    deleted         INTEGER NOT NULL DEFAULT 0,
    updated_at      INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (source, account_id, external_id)
);
CREATE INDEX IF NOT EXISTS idx_silver_feishu_users_wm ON silver_feishu_users(updated_at);

-- 消息 · 飞书 — keeps @mentions (OpenID+name+key), reply chain (parent/root/thread).
CREATE TABLE IF NOT EXISTS silver_feishu_messages (
    source            TEXT    NOT NULL DEFAULT 'feishu',
    account_id        TEXT    NOT NULL DEFAULT 'default',
    external_id       TEXT    NOT NULL,          -- message_id
    chat_id           TEXT    NOT NULL DEFAULT '',
    msg_type          TEXT    NOT NULL DEFAULT '',
    sender_open_id    TEXT    NOT NULL DEFAULT '',
    sender_tenant_key TEXT    NOT NULL DEFAULT '',
    body_text         TEXT    NOT NULL DEFAULT '',
    mentions          TEXT    NOT NULL DEFAULT '[]', -- JSON [{openId,key,name,tenantKey}]
    parent_id         TEXT    NOT NULL DEFAULT '', -- reply parent message
    root_id           TEXT    NOT NULL DEFAULT '', -- reply thread root
    thread_id         TEXT    NOT NULL DEFAULT '',
    create_time       INTEGER NOT NULL DEFAULT 0,
    deleted           INTEGER NOT NULL DEFAULT 0,
    updated_at        INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (source, account_id, external_id)
);
CREATE INDEX IF NOT EXISTS idx_silver_feishu_messages_wm ON silver_feishu_messages(updated_at);

-- 飞书 群 (thread metadata) — feeds gold thread titles; not a viewer domain itself.
CREATE TABLE IF NOT EXISTS silver_feishu_chats (
    source        TEXT    NOT NULL DEFAULT 'feishu',
    account_id    TEXT    NOT NULL DEFAULT 'default',
    external_id   TEXT    NOT NULL,             -- chat_id
    name          TEXT    NOT NULL DEFAULT '',
    chat_mode     TEXT    NOT NULL DEFAULT '',  -- group | p2p
    external      INTEGER NOT NULL DEFAULT 0,
    owner_open_id TEXT    NOT NULL DEFAULT '',
    tenant_key    TEXT    NOT NULL DEFAULT '',
    avatar        TEXT    NOT NULL DEFAULT '',
    description   TEXT    NOT NULL DEFAULT '',
    deleted       INTEGER NOT NULL DEFAULT 0,
    updated_at    INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (source, account_id, external_id)
);
CREATE INDEX IF NOT EXISTS idx_silver_feishu_chats_wm ON silver_feishu_chats(updated_at);

-- 消息 · MS 邮件
CREATE TABLE IF NOT EXISTS silver_microsoft_mail (
    source          TEXT    NOT NULL DEFAULT 'microsoft',
    account_id      TEXT    NOT NULL DEFAULT 'default',
    external_id     TEXT    NOT NULL,           -- Graph id
    subject         TEXT    NOT NULL DEFAULT '',
    body_preview    TEXT    NOT NULL DEFAULT '',
    received_at     INTEGER NOT NULL DEFAULT 0,
    from_addr       TEXT    NOT NULL DEFAULT '',
    from_name       TEXT    NOT NULL DEFAULT '',
    to_recipients   TEXT    NOT NULL DEFAULT '[]', -- JSON [{addr,name}]
    is_read         INTEGER NOT NULL DEFAULT 0,
    web_link        TEXT    NOT NULL DEFAULT '',
    conversation_id TEXT    NOT NULL DEFAULT '',
    deleted         INTEGER NOT NULL DEFAULT 0,
    updated_at      INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (source, account_id, external_id)
);
CREATE INDEX IF NOT EXISTS idx_silver_microsoft_mail_wm ON silver_microsoft_mail(updated_at);

-- 消息 · 腾讯 Agent Mail
CREATE TABLE IF NOT EXISTS silver_agentmail_mail (
    source          TEXT    NOT NULL DEFAULT 'agentmail',
    account_id      TEXT    NOT NULL DEFAULT 'default',
    external_id     TEXT    NOT NULL,           -- message_id
    subject         TEXT    NOT NULL DEFAULT '',
    snippet         TEXT    NOT NULL DEFAULT '',
    created_at_src  INTEGER NOT NULL DEFAULT 0,
    from_email      TEXT    NOT NULL DEFAULT '',
    from_name       TEXT    NOT NULL DEFAULT '',
    to_recipients   TEXT    NOT NULL DEFAULT '[]',
    dir_name        TEXT    NOT NULL DEFAULT '',
    has_attachments INTEGER NOT NULL DEFAULT 0,
    is_read         INTEGER NOT NULL DEFAULT 0,
    deleted         INTEGER NOT NULL DEFAULT 0,
    updated_at      INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (source, account_id, external_id)
);
CREATE INDEX IF NOT EXISTS idx_silver_agentmail_mail_wm ON silver_agentmail_mail(updated_at);

-- 日历 · MS
CREATE TABLE IF NOT EXISTS silver_microsoft_events (
    source          TEXT    NOT NULL DEFAULT 'microsoft',
    account_id      TEXT    NOT NULL DEFAULT 'default',
    external_id     TEXT    NOT NULL,
    calendar_id     TEXT    NOT NULL DEFAULT '',
    subject         TEXT    NOT NULL DEFAULT '',
    body            TEXT    NOT NULL DEFAULT '',
    location        TEXT    NOT NULL DEFAULT '',
    starts_at       INTEGER NOT NULL DEFAULT 0,
    ends_at         INTEGER NOT NULL DEFAULT 0,
    all_day         INTEGER NOT NULL DEFAULT 0,
    show_as         TEXT    NOT NULL DEFAULT '',
    web_link        TEXT    NOT NULL DEFAULT '',
    organizer_addr  TEXT    NOT NULL DEFAULT '',
    organizer_name  TEXT    NOT NULL DEFAULT '',
    attendees       TEXT    NOT NULL DEFAULT '[]', -- JSON [{addr,name,response}]
    recurrence      TEXT    NOT NULL DEFAULT '',   -- JSON (raw Graph recurrence)
    deleted         INTEGER NOT NULL DEFAULT 0,
    updated_at      INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (source, account_id, external_id)
);
CREATE INDEX IF NOT EXISTS idx_silver_microsoft_events_wm ON silver_microsoft_events(updated_at);

-- 待办 · MS — recurrence / checklist / reminder / categories all preserved.
CREATE TABLE IF NOT EXISTS silver_microsoft_todos (
    source           TEXT    NOT NULL DEFAULT 'microsoft',
    account_id       TEXT    NOT NULL DEFAULT 'default',
    external_id      TEXT    NOT NULL,
    list_id          TEXT    NOT NULL DEFAULT '',
    title            TEXT    NOT NULL DEFAULT '',
    body             TEXT    NOT NULL DEFAULT '',
    status           TEXT    NOT NULL DEFAULT '',
    importance       TEXT    NOT NULL DEFAULT '',
    due_at           INTEGER NOT NULL DEFAULT 0,
    completed_at     INTEGER NOT NULL DEFAULT 0,
    created_at_src   INTEGER NOT NULL DEFAULT 0,
    reminder_at      INTEGER NOT NULL DEFAULT 0,
    is_reminder_on   INTEGER NOT NULL DEFAULT 0,
    has_attachments  INTEGER NOT NULL DEFAULT 0,
    categories       TEXT    NOT NULL DEFAULT '[]', -- JSON
    recurrence       TEXT    NOT NULL DEFAULT '',   -- JSON (raw)
    checklist_items  TEXT    NOT NULL DEFAULT '[]', -- JSON
    linked_resources TEXT    NOT NULL DEFAULT '[]', -- JSON
    deleted          INTEGER NOT NULL DEFAULT 0,
    updated_at       INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (source, account_id, external_id)
);
CREATE INDEX IF NOT EXISTS idx_silver_microsoft_todos_wm ON silver_microsoft_todos(updated_at);

-- ============================ GOLD (融合) ============================

-- Domain ① 联系人 — canonical person + universal identity table (fusion anchor).
CREATE TABLE IF NOT EXISTS contacts (
    id         TEXT    PRIMARY KEY,
    phone      TEXT    NOT NULL DEFAULT '',
    name       TEXT    NOT NULL DEFAULT '',
    company    TEXT    NOT NULL DEFAULT '',
    title      TEXT    NOT NULL DEFAULT '',
    note       TEXT    NOT NULL DEFAULT '',
    tags       TEXT    NOT NULL DEFAULT '',
    degree     INTEGER NOT NULL DEFAULT 1,       -- 1=address-book/manual, 2=roster-discovered
    created_at TEXT    NOT NULL DEFAULT '',
    updated_at TEXT    NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_contacts_phone ON contacts(phone) WHERE phone != '';

CREATE TABLE IF NOT EXISTS contact_channels (
    id         TEXT    PRIMARY KEY,
    contact_id TEXT    NOT NULL DEFAULT '',
    platform   TEXT    NOT NULL DEFAULT '',      -- feishu|email|phone|imessage|microsoft|wechat
    address    TEXT    NOT NULL DEFAULT '',      -- source-native identifier
    nickname   TEXT    NOT NULL DEFAULT '',
    session_id TEXT    NOT NULL DEFAULT '',
    tenant_key TEXT    NOT NULL DEFAULT '',
    last_seen  INTEGER NOT NULL DEFAULT 0,
    created_at TEXT    NOT NULL DEFAULT '',
    updated_at TEXT    NOT NULL DEFAULT '',
    UNIQUE(platform, address)
);
CREATE INDEX IF NOT EXISTS idx_contact_channels_contact ON contact_channels(contact_id);

-- Domain ② 多媒体消息 — email + 飞书私信/群 + iMessage/SMS + RSS 文章, one model.
CREATE TABLE IF NOT EXISTS threads (
    id              TEXT    PRIMARY KEY,
    source          TEXT    NOT NULL DEFAULT '',
    account_id      TEXT    NOT NULL DEFAULT 'default',
    external_id     TEXT    NOT NULL DEFAULT '', -- chat_id | conversationId | feed url
    kind            TEXT    NOT NULL DEFAULT '', -- dm|group|mail_thread|feed|sms
    title           TEXT    NOT NULL DEFAULT '',
    contact_id      TEXT    NOT NULL DEFAULT '', -- only for dm
    last_message_at INTEGER NOT NULL DEFAULT 0,
    created_at      TEXT    NOT NULL DEFAULT '',
    UNIQUE(source, account_id, external_id)
);

CREATE TABLE IF NOT EXISTS messages (
    id                TEXT    PRIMARY KEY,
    thread_id         TEXT    NOT NULL DEFAULT '',
    msg_kind          TEXT    NOT NULL DEFAULT '', -- im|email|rss|sms|system
    source            TEXT    NOT NULL DEFAULT '',
    account_id        TEXT    NOT NULL DEFAULT 'default',
    external_id       TEXT    NOT NULL DEFAULT '',
    sender_contact_id TEXT    NOT NULL DEFAULT '',
    subject           TEXT    NOT NULL DEFAULT '',
    body_text         TEXT    NOT NULL DEFAULT '',
    body_html         TEXT    NOT NULL DEFAULT '',
    sent_at           INTEGER NOT NULL DEFAULT 0,
    has_attachments   INTEGER NOT NULL DEFAULT 0,
    fingerprint       TEXT    NOT NULL DEFAULT '', -- v2 cross-source dedup: hash(thread+sender+body+time)
    created_at        TEXT    NOT NULL DEFAULT '',
    UNIQUE(source, account_id, external_id)
);
CREATE INDEX IF NOT EXISTS idx_messages_thread ON messages(thread_id, sent_at);
CREATE INDEX IF NOT EXISTS idx_messages_fp ON messages(fingerprint) WHERE fingerprint != '';

CREATE TABLE IF NOT EXISTS message_participants (
    message_id TEXT NOT NULL,
    contact_id TEXT NOT NULL DEFAULT '',
    role       TEXT NOT NULL DEFAULT '',          -- from|to|cc|bcc
    PRIMARY KEY (message_id, contact_id, role)
);

CREATE TABLE IF NOT EXISTS message_attachments (
    id         TEXT    PRIMARY KEY,
    message_id TEXT    NOT NULL DEFAULT '',
    type       TEXT    NOT NULL DEFAULT '',        -- image|file|audio|video|location|link
    url        TEXT    NOT NULL DEFAULT '',
    blob_ref   TEXT    NOT NULL DEFAULT '',
    name       TEXT    NOT NULL DEFAULT '',
    mime       TEXT    NOT NULL DEFAULT '',
    size       INTEGER NOT NULL DEFAULT 0,
    meta_json  TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_message_attachments_msg ON message_attachments(message_id);

-- Domain ③ 日历
CREATE TABLE IF NOT EXISTS calendar_events (
    id                   TEXT    PRIMARY KEY,
    source               TEXT    NOT NULL DEFAULT '',
    account_id           TEXT    NOT NULL DEFAULT 'default',
    external_id          TEXT    NOT NULL DEFAULT '',
    calendar_id          TEXT    NOT NULL DEFAULT '',
    title                TEXT    NOT NULL DEFAULT '',
    location             TEXT    NOT NULL DEFAULT '',
    starts_at            INTEGER NOT NULL DEFAULT 0,
    ends_at              INTEGER NOT NULL DEFAULT 0,
    all_day              INTEGER NOT NULL DEFAULT 0,
    rrule                TEXT    NOT NULL DEFAULT '',
    organizer_contact_id TEXT    NOT NULL DEFAULT '',
    status               TEXT    NOT NULL DEFAULT '',
    fingerprint          TEXT    NOT NULL DEFAULT '', -- v2 dedup: hash(title+start+end)
    created_at           TEXT    NOT NULL DEFAULT '',
    UNIQUE(source, account_id, external_id)
);
CREATE INDEX IF NOT EXISTS idx_calendar_events_time ON calendar_events(starts_at);
CREATE INDEX IF NOT EXISTS idx_calendar_events_fp ON calendar_events(fingerprint) WHERE fingerprint != '';

CREATE TABLE IF NOT EXISTS event_attendees (
    event_id   TEXT NOT NULL,
    contact_id TEXT NOT NULL DEFAULT '',
    response   TEXT NOT NULL DEFAULT '',          -- accepted|declined|tentative|none
    PRIMARY KEY (event_id, contact_id)
);

-- Domain ④ 待办 — kept separate from meta.db agent tasks (linked_task_id promotes).
CREATE TABLE IF NOT EXISTS todos (
    id             TEXT    PRIMARY KEY,
    source         TEXT    NOT NULL DEFAULT '',
    account_id     TEXT    NOT NULL DEFAULT 'default',
    external_id    TEXT    NOT NULL DEFAULT '',
    list_id        TEXT    NOT NULL DEFAULT '',
    title          TEXT    NOT NULL DEFAULT '',
    notes          TEXT    NOT NULL DEFAULT '',
    due_at         INTEGER NOT NULL DEFAULT 0,
    completed_at   INTEGER NOT NULL DEFAULT 0,
    status         TEXT    NOT NULL DEFAULT '',
    priority       TEXT    NOT NULL DEFAULT '',
    linked_task_id TEXT    NOT NULL DEFAULT '',   -- optional: promote to an agent work-order
    fingerprint    TEXT    NOT NULL DEFAULT '',
    created_at     TEXT    NOT NULL DEFAULT '',
    UNIQUE(source, account_id, external_id)
);
CREATE INDEX IF NOT EXISTS idx_todos_due ON todos(due_at);

-- ============================ TRANSFORM CURSORS ============================
-- Per-(stage, source, kind) high-water mark. stage=silver over bronze.fetched_at,
-- stage=gold over silver.updated_at. Resetting to 0 safely re-runs a stage.
CREATE TABLE IF NOT EXISTS data_cursors (
    stage      TEXT    NOT NULL,
    source     TEXT    NOT NULL,
    kind       TEXT    NOT NULL,
    watermark  INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (stage, source, kind)
);
`
