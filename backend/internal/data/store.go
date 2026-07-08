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
	// Apply the shared gold + cursor skeleton, then every source's self-registered
	// silver DDL. Adding a source is a new file that registers itself (issue #399);
	// the skeleton here never changes. All statements are CREATE IF NOT EXISTS, so
	// order is irrelevant and re-open is a no-op.
	if _, err := sqlDB.Exec(coreSchema); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("data: apply core schema: %w", err)
	}
	for _, src := range silverSources {
		if _, err := sqlDB.Exec(src.ddl); err != nil {
			sqlDB.Close()
			return nil, fmt.Errorf("data: apply silver schema: %w", err)
		}
	}
	// Backfill columns added after a table's original DDL. silver is derived and
	// fully re-runnable, but an existing DB still needs the new column present
	// before an upsert can write it. Idempotent (skips columns already there).
	if err := ensureSilverColumns(sqlDB); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("data: ensure silver columns: %w", err)
	}
	return &Store{sql: sqlDB}, nil
}

// ensureSilverColumns adds columns introduced after a silver table's original
// CREATE (which is IF NOT EXISTS and so never alters an existing table). Mirrors
// meta.ensureTasksColumns. DDL must match the column definition in the table's
// schema constant.
func ensureSilverColumns(db *sql.DB) error {
	type addCol struct{ table, col, ddl string }
	wanted := []addCol{
		{"silver_microsoft_todos", "recurrence_std", "ALTER TABLE silver_microsoft_todos ADD COLUMN recurrence_std TEXT NOT NULL DEFAULT ''"},
		{"silver_microsoft_events", "recurrence_std", "ALTER TABLE silver_microsoft_events ADD COLUMN recurrence_std TEXT NOT NULL DEFAULT ''"},
		{"silver_feishu_events", "recurrence_std", "ALTER TABLE silver_feishu_events ADD COLUMN recurrence_std TEXT NOT NULL DEFAULT ''"},
	}
	for _, w := range wanted {
		has, err := columnExists(db, w.table, w.col)
		if err != nil {
			return err
		}
		if has {
			continue
		}
		if _, err := db.Exec(w.ddl); err != nil {
			return fmt.Errorf("add %s.%s: %w", w.table, w.col, err)
		}
	}
	return nil
}

// columnExists reports whether table has a column named col (via PRAGMA
// table_info). A missing table reports false without error.
func columnExists(db *sql.DB, table, col string) (bool, error) {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == col {
			return true, nil
		}
	}
	return false, rows.Err()
}

// silverSource is one data source's contribution to the silver layer: the DDL
// that builds its physical tables, plus which of those tables the 数据归一 viewer
// browses (domain-tagged; a subset — e.g. 飞书 群 metadata is not browsable). Each
// source registers itself from its own file's init() (registerSilverSource), so
// adding a source is a single new go file — no edits to this shared skeleton or
// to the viewer (issue #399).
type silverSource struct {
	ddl    string           // CREATE TABLE ... for this source's silver tables
	tables []silverTableDef // viewer-exposed tables (domain, source, table)
}

// silverSources is populated by each per-source file's init(). Order follows
// lexical file-name order; viewer consumers sort by domain/time, so it is not
// load-bearing.
var silverSources []silverSource

func registerSilverSource(s silverSource) { silverSources = append(silverSources, s) }

// RegisterViewerTable registers a viewer-exposed silver table at runtime, without
// DDL — the physical table is built by its governance step (manifest REST sources
// build it via CREATE TABLE IF NOT EXISTS, since they register after Open). The
// schema-free reader (SELECT *) renders any table that carries the conformed
// external_id/source/updated_at columns.
func RegisterViewerTable(domain, source, table string) {
	registerSilverSource(silverSource{tables: []silverTableDef{{Domain: domain, Source: source, Table: table}}})
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

// GovernanceRun is one governance-step execution outcome (数据治理 执行日志).
type GovernanceRun struct {
	Step        string `json:"step"`
	Source      string `json:"source"`
	OutputTable string `json:"outputTable"`
	Lang        string `json:"lang"`   // sql | python | go
	Status      string `json:"status"` // success | failed
	Rows        int    `json:"rows"`
	DurationMs  int64  `json:"durationMs"`
	Error       string `json:"error,omitempty"`
	RanAt       string `json:"ranAt"` // RFC3339
}

// RecordGovernanceRun appends one step-run to the execution log.
func (s *Store) RecordGovernanceRun(r GovernanceRun) error {
	if r.RanAt == "" {
		r.RanAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := s.sql.Exec(`INSERT INTO governance_runs
        (step, source, output_table, lang, status, rows, duration_ms, error, ran_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Step, r.Source, r.OutputTable, r.Lang, r.Status, r.Rows, r.DurationMs, r.Error, r.RanAt)
	return err
}

// ListGovernanceRuns returns the newest runs (all steps when step==""), capped.
func (s *Store) ListGovernanceRuns(step string, limit int) ([]GovernanceRun, error) {
	if limit <= 0 {
		limit = 200
	}
	q := `SELECT step, source, output_table, lang, status, rows, duration_ms, error, ran_at
        FROM governance_runs`
	args := []any{}
	if step != "" {
		q += ` WHERE step = ?`
		args = append(args, step)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.sql.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GovernanceRun{}
	for rows.Next() {
		var r GovernanceRun
		if err := rows.Scan(&r.Step, &r.Source, &r.OutputTable, &r.Lang, &r.Status, &r.Rows, &r.DurationMs, &r.Error, &r.RanAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// LastGovernanceRun returns the most recent run for a step (ok=false when none).
func (s *Store) LastGovernanceRun(step string) (GovernanceRun, bool, error) {
	runs, err := s.ListGovernanceRuns(step, 1)
	if err != nil || len(runs) == 0 {
		return GovernanceRun{}, false, err
	}
	return runs[0], true, nil
}

// TruncateGovernanceOutput clears a declarative step's output table for a clean
// rebuild (删除/rebuild). The name is whitelisted (a step output is a validated
// identifier) and must be a real table — a no-op if it does not exist yet.
func (s *Store) TruncateGovernanceOutput(table string) error {
	if !tableNameRe.MatchString(table) {
		return fmt.Errorf("data: unsafe output table %q", table)
	}
	var exists int
	if err := s.sql.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table,
	).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return nil
	}
	_, err := s.sql.Exec("DELETE FROM " + table)
	return err
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

// coreSchema is the source-agnostic skeleton: the gold (融合) domains + the
// transform cursor table. Per-source silver DDL lives in each silver_<source>.go
// file and is applied on top via the silverSources registry (issue #399).
// Idempotent (CREATE IF NOT EXISTS); every table is additive, so no version-gated
// migration is needed (mirrors the bronze store convention).
const coreSchema = `
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

-- ========================= GOVERNANCE EXECUTION LOG =========================
-- One row per governance-step run (SQL / Python / built-in), for the 数据治理
-- 执行日志. Append-only; the UI reads the newest N per step. Separate from the
-- work-order tasks table (that tracks source SYNC; this tracks TRANSFORM steps).
CREATE TABLE IF NOT EXISTS governance_runs (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    step         TEXT    NOT NULL,
    source       TEXT    NOT NULL DEFAULT '',
    output_table TEXT    NOT NULL DEFAULT '',
    lang         TEXT    NOT NULL DEFAULT '',
    status       TEXT    NOT NULL DEFAULT '',
    rows         INTEGER NOT NULL DEFAULT 0,
    duration_ms  INTEGER NOT NULL DEFAULT 0,
    error        TEXT    NOT NULL DEFAULT '',
    ran_at       TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_governance_runs_step ON governance_runs(step, id DESC);
`
