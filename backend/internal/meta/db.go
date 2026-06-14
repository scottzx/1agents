package meta

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Sentinel errors shared by all stores. internal/agent aliases these so the
// HTTP handlers' errors.Is checks keep working unchanged.
var (
	ErrDuplicate = fmt.Errorf("meta: duplicate record id")
	ErrNotFound  = fmt.Errorf("meta: record not found")
)

// DB wraps the global metadata database.
type DB struct {
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

// DefaultPath returns ~/.1agents/meta.db (honoring ONEAGENTS_HOME, same as
// the legacy JSON stores did).
func DefaultPath() string {
	return filepath.Join(get1AgentsHome(), ".1agents", "meta.db")
}

// Open opens (creating if needed) the metadata database at path and ensures
// the schema is current. WAL + busy_timeout make concurrent access from the
// server process and CLI invocations safe.
func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("meta: ensure db dir: %w", err)
	}
	// _txlock=immediate: transactions take the write lock at BEGIN, so
	// concurrent writers queue on busy_timeout instead of failing with
	// SQLITE_BUSY when a deferred tx tries to upgrade read→write.
	dsn := "file:" + url.PathEscape(path) +
		"?_txlock=immediate" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=synchronous(NORMAL)"
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("meta: open %s: %w", path, err)
	}
	// One connection serializes all in-process access; cross-process writes
	// are handled by WAL + busy_timeout.
	sqlDB.SetMaxOpenConns(1)
	db := &DB{sql: sqlDB}
	if err := db.migrateSchema(); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return db, nil
}

var (
	openMu    sync.Mutex
	openCache = map[string]*DB{}
)

// OpenDefault opens (or returns the cached handle for) the database at
// DefaultPath(). Cached per resolved path so tests that switch
// ONEAGENTS_HOME get isolated databases.
func OpenDefault() (*DB, error) {
	path := DefaultPath()
	openMu.Lock()
	defer openMu.Unlock()
	if db, ok := openCache[path]; ok {
		return db, nil
	}
	db, err := Open(path)
	if err != nil {
		return nil, err
	}
	openCache[path] = db
	return db, nil
}

// Close closes the underlying connection. Not used by the long-lived server;
// mainly for CLI one-shots and tests.
func (db *DB) Close() error { return db.sql.Close() }

const schemaVersion = 4

func (db *DB) migrateSchema() error {
	var version int
	if err := db.sql.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("meta: read user_version: %w", err)
	}
	if version < 1 {
		if _, err := db.sql.Exec(schemaV1); err != nil {
			return fmt.Errorf("meta: apply schema v1: %w", err)
		}
	}
	if version < 2 {
		if _, err := db.sql.Exec(schemaV2); err != nil {
			return fmt.Errorf("meta: apply schema v2: %w", err)
		}
	}
	if version < 3 {
		if _, err := db.sql.Exec(schemaV3); err != nil {
			return fmt.Errorf("meta: apply schema v3: %w", err)
		}
	}
	if version < 4 {
		if _, err := db.sql.Exec(schemaV4); err != nil {
			return fmt.Errorf("meta: apply schema v4: %w", err)
		}
	}
	if version < schemaVersion {
		if _, err := db.sql.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion)); err != nil {
			return fmt.Errorf("meta: set user_version: %w", err)
		}
	}
	return nil
}

const schemaV1 = `
CREATE TABLE IF NOT EXISTS projects (
    id             TEXT PRIMARY KEY,
    name           TEXT NOT NULL DEFAULT '',
    workspace_path TEXT NOT NULL DEFAULT '',
    status         TEXT NOT NULL DEFAULT 'active',
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_projects_path ON projects(workspace_path);

CREATE TABLE IF NOT EXISTS tasks (
    id            TEXT PRIMARY KEY,
    project_id    TEXT NOT NULL,
    title         TEXT NOT NULL DEFAULT '',
    description   TEXT NOT NULL DEFAULT '',
    issue_state   TEXT NOT NULL DEFAULT 'open',
    status        TEXT NOT NULL DEFAULT 'pending',
    schedule_type TEXT NOT NULL DEFAULT 'immediate',
    scheduled_at  TEXT,
    planned_start TEXT,
    planned_end   TEXT,
    started_at    TEXT,
    completed_at  TEXT,
    summary       TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_tasks_project ON tasks(project_id, status);

CREATE TABLE IF NOT EXISTS task_deps (
    task_id    TEXT NOT NULL,
    depends_on TEXT NOT NULL,
    seq        INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (task_id, depends_on)
);

CREATE TABLE IF NOT EXISTS replies (
    id             TEXT PRIMARY KEY,
    task_id        TEXT NOT NULL,
    seq            INTEGER NOT NULL DEFAULT 0,
    author_kind    TEXT NOT NULL DEFAULT '',
    author_name    TEXT NOT NULL DEFAULT '',
    agent_type     TEXT NOT NULL DEFAULT '',
    text           TEXT NOT NULL DEFAULT '',
    session_ref    TEXT NOT NULL DEFAULT '',
    acp_session_id TEXT NOT NULL DEFAULT '',
    in_reply_to    TEXT NOT NULL DEFAULT '',
    mode           TEXT NOT NULL DEFAULT 'pure_comment',
    created_at     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_replies_task ON replies(task_id, seq);

CREATE TABLE IF NOT EXISTS sessions (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL DEFAULT '',
    task_id         TEXT NOT NULL DEFAULT '',
    name            TEXT NOT NULL DEFAULT '',
    agent_type      TEXT NOT NULL DEFAULT '',
    cc_project      TEXT NOT NULL DEFAULT '',
    cc_session_id   TEXT NOT NULL DEFAULT '',
    acp_session_id  TEXT NOT NULL DEFAULT '',
    session_key     TEXT NOT NULL DEFAULT '',
    permission_mode TEXT NOT NULL DEFAULT '',
    exec_status     TEXT NOT NULL DEFAULT '',
    exec_summary    TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL,
    last_event_at   TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_sessions_project ON sessions(project_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_sessions_task ON sessions(task_id);
`

// schemaV2 adds the PM + automation fields (priority, assignee, labels,
// hierarchy, acceptance criteria, recurrence, retry budget). SQLite ALTER
// TABLE ADD COLUMN is metadata-only, so upgrading an existing v1 database
// keeps all rows intact.
const schemaV2 = `
ALTER TABLE tasks ADD COLUMN priority            TEXT    NOT NULL DEFAULT 'medium';
ALTER TABLE tasks ADD COLUMN assignee            TEXT    NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN labels              TEXT    NOT NULL DEFAULT '[]';
ALTER TABLE tasks ADD COLUMN created_by          TEXT    NOT NULL DEFAULT 'user';
ALTER TABLE tasks ADD COLUMN parent_id           TEXT    NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN milestone           TEXT    NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN acceptance_criteria TEXT    NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN recurrence          TEXT    NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN max_retries         INTEGER NOT NULL DEFAULT 1;
ALTER TABLE tasks ADD COLUMN retry_count         INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN timeout_minutes     INTEGER NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_tasks_parent ON tasks(parent_id);
`

// schemaV3 adds the sprint label (free-text PM grouping, e.g. "Sprint 23").
// Backward-compat: the DEFAULT '' means existing v2 rows survive untouched
// and report Sprint == "" until the user opts a task into a sprint.
const schemaV3 = `
ALTER TABLE tasks ADD COLUMN sprint TEXT NOT NULL DEFAULT '';
`

// schemaV4 adds the issue-type discriminator (GitHub-style: task/requirement/
// bug share one table). DEFAULT 'task' keeps every pre-v4 row a normal task;
// requirement cards (the "需求池") are just tasks with type != 'task'.
const schemaV4 = `
ALTER TABLE tasks ADD COLUMN type TEXT NOT NULL DEFAULT 'task';
`

// ── shared helpers ──────────────────────────────────────────────────────────

// newID returns a random 16-byte hex string (same format as agent.newID).
func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "meta-fallback-id"
	}
	return hex.EncodeToString(b[:])
}

// NewID exposes the id generator for callers (e.g. the CLI) that create
// records themselves.
func NewID() string { return newID() }

// timeToStr serializes a time for storage; zero time becomes ''.
func timeToStr(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// strToTime parses a stored timestamp; '' becomes the zero time.
func strToTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

// timePtrToVal converts *time.Time to a driver value (NULL when nil/zero).
func timePtrToVal(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// valToTimePtr converts a nullable column back to *time.Time.
func valToTimePtr(ns sql.NullString) *time.Time {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, ns.String)
	if err != nil {
		return nil
	}
	t = t.UTC()
	return &t
}
