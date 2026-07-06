package data

import (
	"database/sql"
	"encoding/json"
)

// Silver rows are per-SOURCE and source-faithful: each type mirrors one source's
// cleaned shape with its own native columns, so nothing valuable is flattened
// away. A governor parses a bronze payload into one of these and upserts it on
// the bronze grain (source, account_id, external_id). UpdatedAt carries the
// bronze fetched_at through verbatim, keeping it monotonic across a re-govern so
// the silver→gold stage can read `updated_at > cursor`. Cross-source unification
// is gold's job, not silver's.

// ---- shared value fragments (JSON-encoded into silver columns) ----

// EmailRef is one mail address+name (MS/AgentMail from & recipients).
type EmailRef struct {
	Addr string `json:"addr"`
	Name string `json:"name,omitempty"`
}

// Mention is one 飞书 @mention: the OpenID plus the `@_user_N` key used in the
// body and the display name (both free from the message payload).
type Mention struct {
	OpenID    string `json:"openId"`
	Key       string `json:"key,omitempty"`
	Name      string `json:"name,omitempty"`
	TenantKey string `json:"tenantKey,omitempty"`
}

// Attendee is a calendar-event participant (still a raw address, no contact).
type Attendee struct {
	Addr     string `json:"addr"`
	Name     string `json:"name,omitempty"`
	Response string `json:"response,omitempty"`
}

// ---- per-source silver row types ----

// SilverIcloudContact is one cleaned Apple contact. Only promoted columns — the
// verbatim vCard stays in bronze, so silver never re-duplicates the raw payload.
type SilverIcloudContact struct {
	AccountID, ExternalID                string
	FullName, FamilyName, GivenName      string
	Phones, Emails                       []string
	Org, Title, Birthday, Nickname, Note string
	IMHandles, URLs, Addresses           []string
	Deleted                              bool
	UpdatedAt                            int64
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

// SilverFeishuChat is 飞书 group metadata (thread source for gold).
type SilverFeishuChat struct {
	AccountID, ExternalID                       string
	Name, ChatMode                              string
	External                                    bool
	OwnerOpenID, TenantKey, Avatar, Description string
	Deleted                                     bool
	UpdatedAt                                   int64
}

// SilverMicrosoftMail is one MS Graph mail.
type SilverMicrosoftMail struct {
	AccountID, ExternalID   string
	Subject, BodyPreview    string
	ReceivedAt              int64
	FromAddr, FromName      string
	ToRecipients            []EmailRef
	IsRead                  bool
	WebLink, ConversationID string
	Deleted                 bool
	UpdatedAt               int64
}

// SilverAgentMail is one 腾讯 Agent Mail message.
type SilverAgentMail struct {
	AccountID, ExternalID  string
	Subject, Snippet       string
	CreatedAtSrc           int64
	FromEmail, FromName    string
	ToRecipients           []EmailRef
	DirName                string
	HasAttachments, IsRead bool
	Deleted                bool
	UpdatedAt              int64
}

// SilverMicrosoftEvent is one MS Graph calendar event.
type SilverMicrosoftEvent struct {
	AccountID, ExternalID               string
	CalendarID, Subject, Body, Location string
	StartsAt, EndsAt                    int64
	AllDay                              bool
	ShowAs, WebLink                     string
	OrganizerAddr, OrganizerName        string
	Attendees                           []Attendee
	Recurrence                          string // raw JSON
	Deleted                             bool
	UpdatedAt                           int64
}

// SilverMicrosoftTodo is one MS Graph to-do (recurrence/checklist/reminder kept).
type SilverMicrosoftTodo struct {
	AccountID, ExternalID                        string
	ListID, Title, Body                          string
	Status, Importance                           string
	DueAt, CompletedAt, CreatedAtSrc, ReminderAt int64
	IsReminderOn, HasAttachments                 bool
	Categories                                   []string
	Recurrence                                   string // raw JSON
	ChecklistItems, LinkedResources              string // raw JSON arrays
	Deleted                                      bool
	UpdatedAt                                    int64
}

// ---- upserts (INSERT OR REPLACE on the bronze grain; idempotent) ----

func (s *Store) UpsertIcloudContacts(rows []SilverIcloudContact) (int, error) {
	return withTx(s.sql, rows, func(tx *sql.Tx) (*sql.Stmt, error) {
		return tx.Prepare(`INSERT OR REPLACE INTO silver_icloud_contacts
            (source, account_id, external_id, full_name, family_name, given_name, phones, emails,
             org, title, birthday, nickname, note, im_handles, urls, addresses, deleted, updated_at)
            VALUES ('icloud', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	}, func(stmt *sql.Stmt, r SilverIcloudContact) error {
		_, err := stmt.Exec(acct(r.AccountID), r.ExternalID, r.FullName, r.FamilyName, r.GivenName,
			jsonOrEmpty(r.Phones), jsonOrEmpty(r.Emails), r.Org, r.Title, r.Birthday, r.Nickname, r.Note,
			jsonOrEmpty(r.IMHandles), jsonOrEmpty(r.URLs), jsonOrEmpty(r.Addresses),
			boolInt(r.Deleted), r.UpdatedAt)
		return err
	})
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

func (s *Store) UpsertMicrosoftMail(rows []SilverMicrosoftMail) (int, error) {
	return withTx(s.sql, rows, func(tx *sql.Tx) (*sql.Stmt, error) {
		return tx.Prepare(`INSERT OR REPLACE INTO silver_microsoft_mail
            (source, account_id, external_id, subject, body_preview, received_at, from_addr, from_name,
             to_recipients, is_read, web_link, conversation_id, deleted, updated_at)
            VALUES ('microsoft', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	}, func(stmt *sql.Stmt, r SilverMicrosoftMail) error {
		_, err := stmt.Exec(acct(r.AccountID), r.ExternalID, r.Subject, r.BodyPreview, r.ReceivedAt,
			r.FromAddr, r.FromName, jsonOrEmpty(r.ToRecipients), boolInt(r.IsRead), r.WebLink,
			r.ConversationID, boolInt(r.Deleted), r.UpdatedAt)
		return err
	})
}

func (s *Store) UpsertAgentMail(rows []SilverAgentMail) (int, error) {
	return withTx(s.sql, rows, func(tx *sql.Tx) (*sql.Stmt, error) {
		return tx.Prepare(`INSERT OR REPLACE INTO silver_agentmail_mail
            (source, account_id, external_id, subject, snippet, created_at_src, from_email, from_name,
             to_recipients, dir_name, has_attachments, is_read, deleted, updated_at)
            VALUES ('agentmail', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	}, func(stmt *sql.Stmt, r SilverAgentMail) error {
		_, err := stmt.Exec(acct(r.AccountID), r.ExternalID, r.Subject, r.Snippet, r.CreatedAtSrc,
			r.FromEmail, r.FromName, jsonOrEmpty(r.ToRecipients), r.DirName, boolInt(r.HasAttachments),
			boolInt(r.IsRead), boolInt(r.Deleted), r.UpdatedAt)
		return err
	})
}

func (s *Store) UpsertMicrosoftEvents(rows []SilverMicrosoftEvent) (int, error) {
	return withTx(s.sql, rows, func(tx *sql.Tx) (*sql.Stmt, error) {
		return tx.Prepare(`INSERT OR REPLACE INTO silver_microsoft_events
            (source, account_id, external_id, calendar_id, subject, body, location, starts_at, ends_at,
             all_day, show_as, web_link, organizer_addr, organizer_name, attendees, recurrence, deleted, updated_at)
            VALUES ('microsoft', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	}, func(stmt *sql.Stmt, r SilverMicrosoftEvent) error {
		_, err := stmt.Exec(acct(r.AccountID), r.ExternalID, r.CalendarID, r.Subject, r.Body, r.Location,
			r.StartsAt, r.EndsAt, boolInt(r.AllDay), r.ShowAs, r.WebLink, r.OrganizerAddr, r.OrganizerName,
			jsonOrEmpty(r.Attendees), r.Recurrence, boolInt(r.Deleted), r.UpdatedAt)
		return err
	})
}

func (s *Store) UpsertMicrosoftTodos(rows []SilverMicrosoftTodo) (int, error) {
	return withTx(s.sql, rows, func(tx *sql.Tx) (*sql.Stmt, error) {
		return tx.Prepare(`INSERT OR REPLACE INTO silver_microsoft_todos
            (source, account_id, external_id, list_id, title, body, status, importance, due_at, completed_at,
             created_at_src, reminder_at, is_reminder_on, has_attachments, categories, recurrence,
             checklist_items, linked_resources, deleted, updated_at)
            VALUES ('microsoft', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	}, func(stmt *sql.Stmt, r SilverMicrosoftTodo) error {
		_, err := stmt.Exec(acct(r.AccountID), r.ExternalID, r.ListID, r.Title, r.Body, r.Status,
			r.Importance, r.DueAt, r.CompletedAt, r.CreatedAtSrc, r.ReminderAt, boolInt(r.IsReminderOn),
			boolInt(r.HasAttachments), jsonOrEmpty(r.Categories), nz(r.Recurrence), nzArr(r.ChecklistItems),
			nzArr(r.LinkedResources), boolInt(r.Deleted), r.UpdatedAt)
		return err
	})
}

// ---- small helpers ----

func jsonOrEmpty(v any) string {
	b, err := json.Marshal(v)
	if err != nil || string(b) == "null" {
		return "[]"
	}
	return string(b)
}

// nz defaults an empty raw-JSON string to "" (object/scalar columns).
func nz(s string) string { return s }

// nzArr defaults an empty raw-JSON array string to "[]".
func nzArr(s string) string {
	if s == "" {
		return "[]"
	}
	return s
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// acct defaults an empty account id to "default" (matching bronze).
func acct(id string) string {
	if id == "" {
		return "default"
	}
	return id
}

// withTx runs one prepared statement over rows in a single transaction, counting
// successful execs. Generic so every silver upsert shares the boilerplate.
func withTx[T any](db *sql.DB, rows []T, prep func(*sql.Tx) (*sql.Stmt, error), exec func(*sql.Stmt, T) error) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	stmt, err := prep(tx)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	n := 0
	for _, r := range rows {
		if err := exec(stmt, r); err != nil {
			return 0, err
		}
		n++
	}
	return n, tx.Commit()
}
