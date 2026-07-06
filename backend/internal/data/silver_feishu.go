package data

import "database/sql"

// silver_feishu.go — 飞书 silver: 二级用户 + 消息 + 群 metadata. Three tables, but only
// users (contacts) and messages (messages) are browsable — 群 metadata feeds gold
// thread titles, so it is registered for building but not exposed to the viewer
// (issue #399).

func init() {
	registerSilverSource(silverSource{
		ddl: feishuSilverSchema,
		tables: []silverTableDef{
			{"contacts", "feishu", "silver_feishu_users"},
			{"messages", "feishu", "silver_feishu_messages"},
			{"events", "feishu", "silver_feishu_events"},
			// silver_feishu_chats intentionally omitted: gold thread metadata,
			// not a browsable domain.
		},
	})
}

const feishuSilverSchema = `
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

-- 日历 · 飞书 日程事件 — one row per calendar event. Column names align with
-- silver_microsoft_events (subject/location/starts_at/ends_at/all_day) so the
-- events viewer domain reads consistently across sources and gold can fuse them.
CREATE TABLE IF NOT EXISTS silver_feishu_events (
    source        TEXT    NOT NULL DEFAULT 'feishu',
    account_id    TEXT    NOT NULL DEFAULT 'default',
    external_id   TEXT    NOT NULL,             -- event_id
    calendar_id   TEXT    NOT NULL DEFAULT '',  -- the calendar it lives in (collection)
    subject       TEXT    NOT NULL DEFAULT '',  -- summary
    description   TEXT    NOT NULL DEFAULT '',
    location      TEXT    NOT NULL DEFAULT '',
    starts_at     INTEGER NOT NULL DEFAULT 0,   -- epoch ms
    ends_at       INTEGER NOT NULL DEFAULT 0,   -- epoch ms
    all_day       INTEGER NOT NULL DEFAULT 0,
    status        TEXT    NOT NULL DEFAULT '',  -- tentative | confirmed | cancelled
    organizer_open_id     TEXT NOT NULL DEFAULT '', -- event_organizer.user_id (the PERSON → gold links to contact)
    organizer_name        TEXT NOT NULL DEFAULT '', -- event_organizer.display_name
    organizer_calendar_id TEXT NOT NULL DEFAULT '', -- organizer_calendar_id (owning calendar)
    meeting_url   TEXT    NOT NULL DEFAULT '',  -- vchat.meeting_url
    recurrence    TEXT    NOT NULL DEFAULT '',  -- RRULE
    deleted       INTEGER NOT NULL DEFAULT 0,
    updated_at    INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (source, account_id, external_id)
);
CREATE INDEX IF NOT EXISTS idx_silver_feishu_events_wm ON silver_feishu_events(updated_at);
`

// Mention is one 飞书 @mention: the OpenID plus the `@_user_N` key used in the
// body and the display name (both free from the message payload).
type Mention struct {
	OpenID    string `json:"openId"`
	Key       string `json:"key,omitempty"`
	Name      string `json:"name,omitempty"`
	TenantKey string `json:"tenantKey,omitempty"`
}

// SilverFeishuUser is one 二级用户 (OpenID) aggregated from message senders + mentions.
type SilverFeishuUser struct {
	AccountID, ExternalID  string // ExternalID = open_id
	Name, TenantKey        string
	DiscoveredVia, ChatIDs []string
	AsSenderCount          int
	FirstSeen, LastSeen    int64
	UpdatedAt              int64
}

// SilverFeishuMessage keeps mentions + the reply chain.
type SilverFeishuMessage struct {
	AccountID, ExternalID         string
	ChatID, MsgType               string
	SenderOpenID, SenderTenantKey string
	BodyText                      string
	Mentions                      []Mention
	ParentID, RootID, ThreadID    string
	CreateTime                    int64
	Deleted                       bool
	UpdatedAt                     int64
}

// SilverFeishuEvent is one 飞书 calendar event (events domain, aligned to MS events).
type SilverFeishuEvent struct {
	AccountID, ExternalID                      string
	CalendarID, Subject, Description, Location string
	StartsAt, EndsAt                           int64
	AllDay                                     bool
	Status                                     string
	// OrganizerOpenID/Name = the organizing PERSON (event_organizer), the fusion
	// key that links an event to a contact; OrganizerCalendarID = the owning calendar.
	OrganizerOpenID, OrganizerName, OrganizerCalendarID string
	MeetingURL, Recurrence                              string
	Deleted                                             bool
	UpdatedAt                                           int64
}

// SilverFeishuChat is 飞书 group metadata (thread source for gold).
type SilverFeishuChat struct {
	AccountID, ExternalID                       string
	Name, ChatMode                              string
	External                                    bool
	OwnerOpenID, TenantKey, Avatar, Description string
	Deleted                                     bool
	UpdatedAt                                   int64
}

func (s *Store) UpsertFeishuUsers(rows []SilverFeishuUser) (int, error) {
	return withTx(s.sql, rows, func(tx *sql.Tx) (*sql.Stmt, error) {
		return tx.Prepare(`INSERT OR REPLACE INTO silver_feishu_users
            (source, account_id, external_id, name, tenant_key, discovered_via, chat_ids,
             as_sender_count, first_seen, last_seen, deleted, updated_at)
            VALUES ('feishu', ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?)`)
	}, func(stmt *sql.Stmt, r SilverFeishuUser) error {
		_, err := stmt.Exec(acct(r.AccountID), r.ExternalID, r.Name, r.TenantKey,
			jsonOrEmpty(r.DiscoveredVia), jsonOrEmpty(r.ChatIDs), r.AsSenderCount,
			r.FirstSeen, r.LastSeen, r.UpdatedAt)
		return err
	})
}

func (s *Store) UpsertFeishuMessages(rows []SilverFeishuMessage) (int, error) {
	return withTx(s.sql, rows, func(tx *sql.Tx) (*sql.Stmt, error) {
		return tx.Prepare(`INSERT OR REPLACE INTO silver_feishu_messages
            (source, account_id, external_id, chat_id, msg_type, sender_open_id, sender_tenant_key,
             body_text, mentions, parent_id, root_id, thread_id, create_time, deleted, updated_at)
            VALUES ('feishu', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	}, func(stmt *sql.Stmt, r SilverFeishuMessage) error {
		_, err := stmt.Exec(acct(r.AccountID), r.ExternalID, r.ChatID, r.MsgType, r.SenderOpenID,
			r.SenderTenantKey, r.BodyText, jsonOrEmpty(r.Mentions), r.ParentID, r.RootID, r.ThreadID,
			r.CreateTime, boolInt(r.Deleted), r.UpdatedAt)
		return err
	})
}

func (s *Store) UpsertFeishuEvents(rows []SilverFeishuEvent) (int, error) {
	return withTx(s.sql, rows, func(tx *sql.Tx) (*sql.Stmt, error) {
		return tx.Prepare(`INSERT OR REPLACE INTO silver_feishu_events
            (source, account_id, external_id, calendar_id, subject, description, location,
             starts_at, ends_at, all_day, status, organizer_open_id, organizer_name,
             organizer_calendar_id, meeting_url, recurrence, deleted, updated_at)
            VALUES ('feishu', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	}, func(stmt *sql.Stmt, r SilverFeishuEvent) error {
		_, err := stmt.Exec(acct(r.AccountID), r.ExternalID, r.CalendarID, r.Subject, r.Description,
			r.Location, r.StartsAt, r.EndsAt, boolInt(r.AllDay), r.Status, r.OrganizerOpenID,
			r.OrganizerName, r.OrganizerCalendarID, r.MeetingURL, r.Recurrence,
			boolInt(r.Deleted), r.UpdatedAt)
		return err
	})
}

func (s *Store) UpsertFeishuChats(rows []SilverFeishuChat) (int, error) {
	return withTx(s.sql, rows, func(tx *sql.Tx) (*sql.Stmt, error) {
		return tx.Prepare(`INSERT OR REPLACE INTO silver_feishu_chats
            (source, account_id, external_id, name, chat_mode, external, owner_open_id, tenant_key,
             avatar, description, deleted, updated_at)
            VALUES ('feishu', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	}, func(stmt *sql.Stmt, r SilverFeishuChat) error {
		_, err := stmt.Exec(acct(r.AccountID), r.ExternalID, r.Name, r.ChatMode, boolInt(r.External),
			r.OwnerOpenID, r.TenantKey, r.Avatar, r.Description, boolInt(r.Deleted), r.UpdatedAt)
		return err
	})
}
