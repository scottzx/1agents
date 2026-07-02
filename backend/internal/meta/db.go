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

// SQL returns the underlying *sql.DB handle. Used by platform packages
// (appregistry, domainstore) that run idempotent CREATE TABLE IF NOT EXISTS
// migrations without touching the global schemaVersion counter.
func (db *DB) SQL() *sql.DB { return db.sql }

const schemaVersion = 20

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
	if version < 5 {
		if _, err := db.sql.Exec(schemaV5); err != nil {
			return fmt.Errorf("meta: apply schema v5: %w", err)
		}
	}
	if version < 6 {
		if _, err := db.sql.Exec(schemaV6); err != nil {
			return fmt.Errorf("meta: apply schema v6: %w", err)
		}
	}
	if version < 7 {
		if _, err := db.sql.Exec(schemaV7); err != nil {
			return fmt.Errorf("meta: apply schema v7: %w", err)
		}
	}
	if version < 8 {
		if _, err := db.sql.Exec(schemaV8); err != nil {
			return fmt.Errorf("meta: apply schema v8: %w", err)
		}
	}
	if version < 13 {
		if _, err := db.sql.Exec(schemaV13); err != nil {
			return fmt.Errorf("meta: apply schema v13: %w", err)
		}
	}
	if version < 14 {
		if _, err := db.sql.Exec(schemaV14); err != nil {
			return fmt.Errorf("meta: apply schema v14: %w", err)
		}
	}
	// v15 (#chat-digest) adds the value-extraction template library + per-chat
	// bindings. New tables only (CREATE IF NOT EXISTS), so version-gated is fine.
	if version < 15 {
		if _, err := db.sql.Exec(schemaV15); err != nil {
			return fmt.Errorf("meta: apply schema v15: %w", err)
		}
	}
	// v16 (联系人聚合) adds the contacts + channel-identity tables. New tables
	// only (CREATE IF NOT EXISTS), so version-gated is fine.
	if version < 16 {
		if _, err := db.sql.Exec(schemaV16); err != nil {
			return fmt.Errorf("meta: apply schema v16: %w", err)
		}
	}
	// v17 (飞书渠道配置) adds the tracked-chats + global sync-config tables. New
	// tables only (CREATE IF NOT EXISTS), so version-gated is fine.
	if version < 17 {
		if _, err := db.sql.Exec(schemaV17); err != nil {
			return fmt.Errorf("meta: apply schema v17: %w", err)
		}
	}
	// v18 (二度联系人) adds the feishu_group_members roster table. New table only
	// (CREATE IF NOT EXISTS), so version-gated is fine; the contacts.degree column
	// is added by ensureContactsColumns below (unconditional, idempotent).
	if version < 18 {
		if _, err := db.sql.Exec(schemaV18); err != nil {
			return fmt.Errorf("meta: apply schema v18: %w", err)
		}
	}
	// v19 (公司基础信息表) adds the companies + company_tenants tables: the
	// tenant_key→org-name mapping that replaces the hardcoded 飞书官方 constant. New
	// tables only (CREATE IF NOT EXISTS), so version-gated is fine.
	if version < 19 {
		if _, err := db.sql.Exec(schemaV19); err != nil {
			return fmt.Errorf("meta: apply schema v19: %w", err)
		}
	}
	// v20 (#318) adds task kernel fields: executor tri-state, business_ref
	// binding seam, task_target dispatch spec, result, and cost_tokens. All are
	// idempotent ADD COLUMNs handled by ensureTasksColumns below, so the
	// version gate only bumps the counter (new DBs get the columns from v1).
	if version < 20 {
		if _, err := db.sql.Exec(schemaV20); err != nil {
			return fmt.Errorf("meta: apply schema v20: %w", err)
		}
	}
	// Schema v9–v12 only add tasks columns, but the v9 branch collision between
	// #47 (source, user_confirm) and #50 (verifier/review fields) left some DBs
	// with user_version bumped to the latest while the other branch's columns
	// were never added. A version-gated ALTER can't recover that: it re-adds an
	// existing column ("duplicate column name") or skips a missing one forever.
	// Run unconditionally (NOT under a version gate) so a DB already at the
	// latest user_version but missing columns still gets healed — idempotent.
	// v12 (#74) adds the GitHub Issue/PR mapping columns the same way.
	if err := db.ensureTasksColumns(); err != nil {
		return fmt.Errorf("meta: reconcile tasks columns: %w", err)
	}
	// v14 (#141) adds the project archive/close columns. Reconciled
	// unconditionally (same rationale as ensureTasksColumns): idempotent ADD
	// COLUMN that heals a DB whose user_version was bumped by a sibling branch
	// before these columns landed (v13 was taken by #60's Inbox table).
	if err := db.ensureProjectsColumns(); err != nil {
		return fmt.Errorf("meta: reconcile projects columns: %w", err)
	}
	// v18 (二度联系人) adds contacts.degree. Reconciled unconditionally (same
	// rationale as the other ensure* helpers): an idempotent ADD COLUMN that heals
	// a DB whose user_version was bumped by a sibling branch before this landed.
	if err := db.ensureContactsColumns(); err != nil {
		return fmt.Errorf("meta: reconcile contacts columns: %w", err)
	}
	// 渠道子模块同意 + 爬取规则 (channel_modules). New table only, created
	// unconditionally (CREATE IF NOT EXISTS) to sidestep the meta schema-version
	// collisions above. Idempotent.
	if err := db.ensureChannelModules(); err != nil {
		return fmt.Errorf("meta: ensure channel_modules: %w", err)
	}
	// 数据源爬取配置 (source_collection_config). New table only, created
	// unconditionally (CREATE IF NOT EXISTS) to sidestep the meta schema-version
	// collisions above. Idempotent.
	if err := db.ensureSourceCollectionConfig(); err != nil {
		return fmt.Errorf("meta: ensure source_collection_config: %w", err)
	}
	// 数据源账号注册表 (source_accounts). 厂家 + 账号 = 一个源; created
	// unconditionally (CREATE IF NOT EXISTS), same rationale as above. Idempotent.
	if err := db.ensureSourceAccounts(); err != nil {
		return fmt.Errorf("meta: ensure source_accounts: %w", err)
	}
	if version < schemaVersion {
		if _, err := db.sql.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion)); err != nil {
			return fmt.Errorf("meta: set user_version: %w", err)
		}
	}
	return nil
}

// ensureTasksColumns adds any of the schema v9–v14 tasks columns that are
// missing, skipping those already present. Idempotent and independent of
// user_version, so it recovers DBs left half-migrated by the v9 branch
// collision (#47 ⇄ #50). DDL must match the original ADD COLUMN definitions.
func (db *DB) ensureTasksColumns() error {
	have, err := db.tasksColumns()
	if err != nil {
		return err
	}
	type col struct{ name, ddl string }
	wanted := []col{
		{"verifier", "ALTER TABLE tasks ADD COLUMN verifier TEXT NOT NULL DEFAULT ''"},
		{"review_max_attempts", "ALTER TABLE tasks ADD COLUMN review_max_attempts INTEGER NOT NULL DEFAULT 0"},
		{"review_count", "ALTER TABLE tasks ADD COLUMN review_count INTEGER NOT NULL DEFAULT 0"},
		{"review", "ALTER TABLE tasks ADD COLUMN review TEXT NOT NULL DEFAULT ''"},
		{"source", "ALTER TABLE tasks ADD COLUMN source TEXT NOT NULL DEFAULT ''"},
		{"user_confirm", "ALTER TABLE tasks ADD COLUMN user_confirm INTEGER NOT NULL DEFAULT 0"},
		// ── GitHub Issue/PR mapping (v12, #74) ──
		{"github_repo", "ALTER TABLE tasks ADD COLUMN github_repo TEXT NOT NULL DEFAULT ''"},
		{"github_kind", "ALTER TABLE tasks ADD COLUMN github_kind TEXT NOT NULL DEFAULT ''"},
		{"github_number", "ALTER TABLE tasks ADD COLUMN github_number INTEGER NOT NULL DEFAULT 0"},
		{"github_node_id", "ALTER TABLE tasks ADD COLUMN github_node_id TEXT NOT NULL DEFAULT ''"},
		{"github_url", "ALTER TABLE tasks ADD COLUMN github_url TEXT NOT NULL DEFAULT ''"},
		{"github_state", "ALTER TABLE tasks ADD COLUMN github_state TEXT NOT NULL DEFAULT ''"},
		{"github_assignees", "ALTER TABLE tasks ADD COLUMN github_assignees TEXT NOT NULL DEFAULT '[]'"},
		{"last_synced_at", "ALTER TABLE tasks ADD COLUMN last_synced_at TEXT"},
		// ── adversarial multi-verifier (v14, #131) ──
		{"verifier_count", "ALTER TABLE tasks ADD COLUMN verifier_count INTEGER NOT NULL DEFAULT 0"},
		{"verify_pass_threshold", "ALTER TABLE tasks ADD COLUMN verify_pass_threshold INTEGER NOT NULL DEFAULT 0"},
		{"review_pool", "ALTER TABLE tasks ADD COLUMN review_pool TEXT NOT NULL DEFAULT ''"},
		// ── task kernel executor tri-state + binding seam (v20, #318) ──
		{"executor", "ALTER TABLE tasks ADD COLUMN executor TEXT NOT NULL DEFAULT 'agent'"},
		{"business_ref", "ALTER TABLE tasks ADD COLUMN business_ref TEXT NOT NULL DEFAULT ''"},
		{"task_target", "ALTER TABLE tasks ADD COLUMN task_target TEXT NOT NULL DEFAULT ''"},
		{"result", "ALTER TABLE tasks ADD COLUMN result TEXT NOT NULL DEFAULT ''"},
		{"cost_tokens", "ALTER TABLE tasks ADD COLUMN cost_tokens INTEGER NOT NULL DEFAULT 0"},
	}
	for _, c := range wanted {
		if have[c.name] {
			continue
		}
		if _, err := db.sql.Exec(c.ddl); err != nil {
			return fmt.Errorf("add tasks.%s: %w", c.name, err)
		}
	}
	return nil
}

// ensureProjectsColumns adds the schema v14 project archive/close columns
// (#141) that are missing, skipping any already present. Idempotent and
// independent of user_version, mirroring ensureTasksColumns.
func (db *DB) ensureProjectsColumns() error {
	have, err := db.tableColumns("projects")
	if err != nil {
		return err
	}
	type col struct{ name, ddl string }
	wanted := []col{
		// archive_reason: '' for active projects; 'completed' (阶段性完成归档) or
		// 'superseded' (竞品出现砍掉) when archived. Free of any verdict for an
		// active row.
		{"archive_reason", "ALTER TABLE projects ADD COLUMN archive_reason TEXT NOT NULL DEFAULT ''"},
		// archive_note: optional free-text rationale captured at archive/close time.
		{"archive_note", "ALTER TABLE projects ADD COLUMN archive_note TEXT NOT NULL DEFAULT ''"},
		// archived_at: timestamp the project left the active view; NULL while active.
		{"archived_at", "ALTER TABLE projects ADD COLUMN archived_at TEXT"},
		// v15 — workspace registry fields absorbed from workspaces_dir.json so the
		// projects table is the single source of truth for the sidebar/workspace API.
		{"terminal_dir", "ALTER TABLE projects ADD COLUMN terminal_dir TEXT NOT NULL DEFAULT ''"},
		{"chat_channel", "ALTER TABLE projects ADD COLUMN chat_channel TEXT NOT NULL DEFAULT ''"},
		{"default_agent", "ALTER TABLE projects ADD COLUMN default_agent TEXT NOT NULL DEFAULT ''"},
		{"builtin", "ALTER TABLE projects ADD COLUMN builtin INTEGER NOT NULL DEFAULT 0"},
		{"position", "ALTER TABLE projects ADD COLUMN position INTEGER NOT NULL DEFAULT 0"},
		// available_agents: JSON array of allowed agent type slugs (e.g. ["claudecode"]).
		// Empty array means unrestricted. Added by Wave 2a platform layer (#325).
		{"available_agents", "ALTER TABLE projects ADD COLUMN available_agents TEXT NOT NULL DEFAULT '[]'"},
		// kind: 'assistant' | 'project'. Legacy rows default to 'project';
		// EnsureDefaultWorkspace bumps the reserved default row to 'assistant'.
		{"kind", "ALTER TABLE projects ADD COLUMN kind TEXT NOT NULL DEFAULT 'project'"},
		// avatar: image ref for the assistant/project card. Either a URL served
		// by GET /avatars/... or a "emoji:X" string; empty when unset.
		{"avatar", "ALTER TABLE projects ADD COLUMN avatar TEXT NOT NULL DEFAULT ''"},
	}
	for _, c := range wanted {
		if have[c.name] {
			continue
		}
		if _, err := db.sql.Exec(c.ddl); err != nil {
			return fmt.Errorf("add projects.%s: %w", c.name, err)
		}
	}
	return nil
}

// ensureContactsColumns adds the schema v18 columns when missing: contacts.degree
// (1 = first-degree/manual, 2 = second-degree/roster-only) and
// contact_channels.tenant_key (the member's Feishu org, free in chat.members).
// Idempotent and independent of user_version, mirroring ensureTasksColumns.
func (db *DB) ensureContactsColumns() error {
	contactCols, err := db.tableColumns("contacts")
	if err != nil {
		return err
	}
	if !contactCols["degree"] {
		if _, err := db.sql.Exec(`ALTER TABLE contacts ADD COLUMN degree INTEGER NOT NULL DEFAULT 1`); err != nil {
			return fmt.Errorf("add contacts.degree: %w", err)
		}
	}
	chanCols, err := db.tableColumns("contact_channels")
	if err != nil {
		return err
	}
	if !chanCols["tenant_key"] {
		if _, err := db.sql.Exec(`ALTER TABLE contact_channels ADD COLUMN tenant_key TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add contact_channels.tenant_key: %w", err)
		}
	}
	// feishu_tracked_chats.member_total: the chat's true member count (API
	// member_total), distinct from the enumerable roster the API caps for very
	// large groups.
	trackedCols, err := db.tableColumns("feishu_tracked_chats")
	if err != nil {
		return err
	}
	if len(trackedCols) > 0 && !trackedCols["member_total"] {
		if _, err := db.sql.Exec(`ALTER TABLE feishu_tracked_chats ADD COLUMN member_total INTEGER NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("add feishu_tracked_chats.member_total: %w", err)
		}
	}
	// feishu_tracked_chats.members_fetched: set once the full chat.members roster
	// has been fetched + ingested, so later syncs skip the (expensive) roster call
	// and reuse the cached roster for sender-name enrichment.
	if len(trackedCols) > 0 && !trackedCols["members_fetched"] {
		if _, err := db.sql.Exec(`ALTER TABLE feishu_tracked_chats ADD COLUMN members_fetched INTEGER NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("add feishu_tracked_chats.members_fetched: %w", err)
		}
	}
	return nil
}

// tasksColumns returns the set of column names currently on the tasks table.
func (db *DB) tasksColumns() (map[string]bool, error) {
	return db.tableColumns("tasks")
}

// tableColumns returns the set of column names currently on the given table.
func (db *DB) tableColumns(table string) (map[string]bool, error) {
	rows, err := db.sql.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var (
			cid        int
			name, typ  string
			notNull    int
			dflt       sql.NullString
			primaryKey int
		)
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &primaryKey); err != nil {
			return nil, err
		}
		cols[name] = true
	}
	return cols, rows.Err()
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
// Backward-compat: the DEFAULT ” means existing v2 rows survive untouched
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

// schemaV5 adds the per-project short id (#N) and peer cross-reference links.
// number is backfilled per project in (created_at, id) order so existing rows
// get stable, gap-free #N; links holds a JSON array of {target, rel} mirroring
// the labels column. New tasks get their number assigned at upsert time.
const schemaV5 = `
ALTER TABLE tasks ADD COLUMN number INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN links  TEXT    NOT NULL DEFAULT '[]';
UPDATE tasks SET number = sub.rn FROM (
    SELECT id, ROW_NUMBER() OVER (
        PARTITION BY project_id ORDER BY created_at, id
    ) AS rn FROM tasks
) AS sub WHERE tasks.id = sub.id;
`

// schemaV6 adds the session role discriminator. DEFAULT ” keeps every
// existing session an ordinary chat; role = 'pm' marks the in-app AI Project
// Manager session (PM system prompt + project-locked task-tool MCP server).
const schemaV6 = `
ALTER TABLE sessions ADD COLUMN role TEXT NOT NULL DEFAULT '';
`

// schemaV7 adds soft-delete for sessions: archived_at holds the archive
// timestamp (empty = active). Closing a session from the sidebar archives it
// rather than dropping the row, so the conversation metadata survives and
// stays searchable in the 会话 archive view. DEFAULT ” keeps every existing
// session active.
const schemaV7 = `
ALTER TABLE sessions ADD COLUMN archived_at TEXT NOT NULL DEFAULT '';
`

// schemaV8 promotes the milestone label to a first-class entity. The new table
// stores per-milestone metadata (target date, ordering, description) keyed by
// (project_id, name); tasks keep linking via their existing milestone column,
// so no task row is touched. The backfill seeds one milestone row per distinct
// non-empty Task.Milestone (per project) so existing groupings survive intact,
// and assigns position in first-appearance order (mirrors the v5 number
// backfill). lower(hex(randomblob(16))) matches newID()'s 32-char hex format.
const schemaV8 = `
CREATE TABLE IF NOT EXISTS milestones (
    id             TEXT PRIMARY KEY,
    project_id     TEXT NOT NULL,
    name           TEXT NOT NULL DEFAULT '',
    description    TEXT NOT NULL DEFAULT '',
    target_date    TEXT,
    position       INTEGER NOT NULL DEFAULT 0,
    predecessor_id TEXT NOT NULL DEFAULT '',
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_milestones_proj_name ON milestones(project_id, name);
CREATE INDEX IF NOT EXISTS idx_milestones_project ON milestones(project_id, position);

INSERT OR IGNORE INTO milestones (id, project_id, name, description, target_date, position, created_at, updated_at)
SELECT lower(hex(randomblob(16))), project_id, milestone, '', NULL, 0, MIN(created_at), MIN(created_at)
FROM tasks WHERE milestone != '' GROUP BY project_id, milestone;

UPDATE milestones SET position = sub.rn FROM (
    SELECT id, ROW_NUMBER() OVER (
        PARTITION BY project_id ORDER BY created_at, name
    ) - 1 AS rn FROM milestones
) AS sub WHERE milestones.id = sub.id;
`

// Schema v9–v11 added tasks columns (#50 verifier/review fields; #47 source and
// user_confirm). These ALTERs now live in ensureTasksColumns, which adds them
// idempotently regardless of user_version — see the note in migrateSchema for
// why the version-gated form couldn't recover the v9 branch collision.

// schemaV13 adds the Inbox 统一信息收口层 table (#60): the most-upstream layer
// that aggregates external context (manual capture / IM / email / RSS / misc)
// into one intake list before PMO 分发 (#61) routes it downstream. Items are
// never deleted — archiving is a status flip so the trail "what did this turn
// into" survives. PMO-dispatch fields (dispatched_to / linked_requirement) are
// deliberately out of scope here; #61 owns them.
const schemaV13 = `
CREATE TABLE IF NOT EXISTS inbox_items (
    id         TEXT PRIMARY KEY,
    source     TEXT NOT NULL DEFAULT 'manual',
    title      TEXT NOT NULL DEFAULT '',
    content    TEXT NOT NULL DEFAULT '',
    url        TEXT NOT NULL DEFAULT '',
    summary    TEXT NOT NULL DEFAULT '',
    tags       TEXT NOT NULL DEFAULT '[]',
    status     TEXT NOT NULL DEFAULT 'unread',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_inbox_status ON inbox_items(status, created_at DESC);
`

// schemaV14 adds the adversarial multi-verifier fields (#131): verifier_count /
// verify_pass_threshold configure the verification panel, review_pool holds the
// running cycle's accumulated per-verifier verdicts. DEFAULTs keep every pre-v14
// task on the classic single-verifier flow (count 0 ⇒ 1 verifier).
const schemaV14 = `
ALTER TABLE tasks ADD COLUMN verifier_count        INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN verify_pass_threshold INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN review_pool           TEXT    NOT NULL DEFAULT '';
`

// schemaV15 adds the chat-digest value-extraction layer. digest_templates is a
// library of reusable Markdown standards ("what counts as valuable" + output
// schema); is_default marks the global fallback(s). digest_bindings attaches
// templates to a chat session, many-to-many, so e.g. an investment group can
// stack 投资 + 产品 templates. Resolution (in the digest package): a chat's
// bound templates, or the is_default ones when it has no binding.
const schemaV15 = `
CREATE TABLE IF NOT EXISTS digest_templates (
    id         TEXT PRIMARY KEY,
    name       TEXT    NOT NULL DEFAULT '',
    scope      TEXT    NOT NULL DEFAULT 'global',
    body_md    TEXT    NOT NULL DEFAULT '',
    builtin    INTEGER NOT NULL DEFAULT 0,
    is_default INTEGER NOT NULL DEFAULT 0,
    created_at TEXT    NOT NULL,
    updated_at TEXT    NOT NULL
);

CREATE TABLE IF NOT EXISTS digest_bindings (
    session_id  TEXT NOT NULL,
    template_id TEXT NOT NULL,
    created_at  TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (session_id, template_id)
);
CREATE INDEX IF NOT EXISTS idx_digest_bindings_session ON digest_bindings(session_id);
`

// schemaV16 adds the 联系人聚合 layer. contacts is the user-curated address book
// keyed by phone (the unique merge key across channels; Feishu can't return a
// phone for external group members, so the user creates the contact). Empty
// phones are allowed (partial-unique index excludes them). contact_channels
// maps a synced channel identity (platform + channel_id, e.g. a Feishu open_id)
// to a contact, idempotent on UNIQUE(platform, channel_id) so re-discovery is
// safe. platform is a discriminator for future WeChat/email; v1 is Feishu-only.
const schemaV16 = `
CREATE TABLE IF NOT EXISTS contacts (
    id         TEXT PRIMARY KEY,
    phone      TEXT NOT NULL DEFAULT '',
    name       TEXT NOT NULL DEFAULT '',
    company    TEXT NOT NULL DEFAULT '',
    title      TEXT NOT NULL DEFAULT '',
    note       TEXT NOT NULL DEFAULT '',
    tags       TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_contacts_phone ON contacts(phone) WHERE phone != '';

CREATE TABLE IF NOT EXISTS contact_channels (
    id         TEXT PRIMARY KEY,
    contact_id TEXT NOT NULL DEFAULT '',
    platform   TEXT NOT NULL DEFAULT 'feishu',
    channel_id TEXT NOT NULL,
    nickname   TEXT NOT NULL DEFAULT '',
    session_id TEXT NOT NULL DEFAULT '',
    last_seen  INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(platform, channel_id)
);
CREATE INDEX IF NOT EXISTS idx_contact_channels_contact ON contact_channels(contact_id);
`

// schemaV17 adds the 飞书渠道配置 layer (Phase 2). feishu_tracked_chats is the
// user's curated set of groups to keep synced: auto_sync gates each chat in the
// periodic loop, last_synced_at drives the per-chat cadence. feishu_sync_config
// is the single-row global toggle + interval (minutes) governing auto-sync.
// Tracking only records which groups to sync — the fetch loop / cross-run
// watermark / message_id dedup all stay in sync.db (unified_*), reused as-is.
const schemaV17 = `
CREATE TABLE IF NOT EXISTS feishu_tracked_chats (
    chat_id        TEXT PRIMARY KEY,
    chat_name      TEXT NOT NULL DEFAULT '',
    avatar         TEXT NOT NULL DEFAULT '',
    external       INTEGER NOT NULL DEFAULT 0,
    auto_sync      INTEGER NOT NULL DEFAULT 1,
    last_synced_at INTEGER NOT NULL DEFAULT 0,
    created_at     TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS feishu_sync_config (
    id               INTEGER PRIMARY KEY CHECK (id = 1),
    enabled          INTEGER NOT NULL DEFAULT 1,
    interval_minutes INTEGER NOT NULL DEFAULT 180
);
`

// schemaV18 adds the 二度联系人 layer (Phase 3). feishu_group_members is the full
// roster of every tracked group: one row per (session_id, channel_id=open_id),
// fetched ONCE on the first sync (gated by feishu_tracked_chats.members_fetched)
// — including silent members who never posted. Later syncs reuse this cache for
// sender-name enrichment and incrementally add active speakers. It drives
// degree-2 contact ingestion (a channel discovered only from the roster, never
// from a sender) and the "在哪些群" detail. The contacts.degree
// column is added separately by ensureContactsColumns (unconditional ALTER).
const schemaV18 = `
CREATE TABLE IF NOT EXISTS feishu_group_members (
    session_id TEXT NOT NULL,
    channel_id TEXT NOT NULL,
    nickname   TEXT NOT NULL DEFAULT '',
    tenant_key TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL,
    PRIMARY KEY (session_id, channel_id)
);
CREATE INDEX IF NOT EXISTS idx_fgm_channel ON feishu_group_members(channel_id);
`

// schemaV19 adds the 公司基础信息表 layer. companies owns the org metadata
// (full/short name, reserved unified_id business id, note); company_tenants maps
// each Feishu tenant_key to a company (1:1 on tenant_key, many tenants per
// company). Together they replace the hardcoded 飞书官方 tenant constant — the org
// name shown next to a contact's channel now resolves through this map, with
// 飞书官方 seeded (see CompanyStore.SeedFeishuOfficial).
const schemaV19 = `
CREATE TABLE IF NOT EXISTS companies (
    id         TEXT PRIMARY KEY,
    full_name  TEXT NOT NULL DEFAULT '',
    short_name TEXT NOT NULL DEFAULT '',
    unified_id TEXT NOT NULL DEFAULT '',
    note       TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS company_tenants (
    tenant_key TEXT PRIMARY KEY,
    company_id TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_company_tenants_company ON company_tenants(company_id);
`

// schemaV20 (#318) adds the task kernel executor tri-state columns. All five
// are idempotent ADD COLUMNs already handled by ensureTasksColumns; the const
// body is intentionally empty — the version counter is what matters here.
// (ensureTasksColumns runs unconditionally so new DBs and existing DBs both get
// the columns regardless of which version path applies.)
const schemaV20 = ``

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

// timeToStr serializes a time for storage; zero time becomes ”.
func timeToStr(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// strToTime parses a stored timestamp; ” becomes the zero time.
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
