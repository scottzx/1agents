package data

import "database/sql"

// silver_microsoft.go — Microsoft Graph silver: 邮件 + 日历 + 待办. One source, three
// domains, self-registered (issue #399).

func init() {
	registerSilverSource(silverSource{
		ddl: microsoftSilverSchema,
		tables: []silverTableDef{
			{"messages", "microsoft", "silver_microsoft_mail"},
			{"events", "microsoft", "silver_microsoft_events"},
			{"todos", "microsoft", "silver_microsoft_todos"},
		},
	})
}

const microsoftSilverSchema = `
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
    recurrence_std  TEXT    NOT NULL DEFAULT '',   -- canonical meta.Recurrence JSON (normalized)
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
    recurrence_std   TEXT    NOT NULL DEFAULT '',   -- canonical meta.Recurrence JSON (normalized)
    checklist_items  TEXT    NOT NULL DEFAULT '[]', -- JSON
    linked_resources TEXT    NOT NULL DEFAULT '[]', -- JSON
    deleted          INTEGER NOT NULL DEFAULT 0,
    updated_at       INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (source, account_id, external_id)
);
CREATE INDEX IF NOT EXISTS idx_silver_microsoft_todos_wm ON silver_microsoft_todos(updated_at);
`

// Attendee is a calendar-event participant (still a raw address, no contact).
type Attendee struct {
	Addr     string `json:"addr"`
	Name     string `json:"name,omitempty"`
	Response string `json:"response,omitempty"`
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
	RecurrenceStd                       string // canonical meta.Recurrence JSON
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
	RecurrenceStd                                string // canonical meta.Recurrence JSON
	ChecklistItems, LinkedResources              string // raw JSON arrays
	Deleted                                      bool
	UpdatedAt                                    int64
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

func (s *Store) UpsertMicrosoftEvents(rows []SilverMicrosoftEvent) (int, error) {
	return withTx(s.sql, rows, func(tx *sql.Tx) (*sql.Stmt, error) {
		return tx.Prepare(`INSERT OR REPLACE INTO silver_microsoft_events
            (source, account_id, external_id, calendar_id, subject, body, location, starts_at, ends_at,
             all_day, show_as, web_link, organizer_addr, organizer_name, attendees, recurrence, recurrence_std, deleted, updated_at)
            VALUES ('microsoft', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	}, func(stmt *sql.Stmt, r SilverMicrosoftEvent) error {
		_, err := stmt.Exec(acct(r.AccountID), r.ExternalID, r.CalendarID, r.Subject, r.Body, r.Location,
			r.StartsAt, r.EndsAt, boolInt(r.AllDay), r.ShowAs, r.WebLink, r.OrganizerAddr, r.OrganizerName,
			jsonOrEmpty(r.Attendees), r.Recurrence, r.RecurrenceStd, boolInt(r.Deleted), r.UpdatedAt)
		return err
	})
}

func (s *Store) UpsertMicrosoftTodos(rows []SilverMicrosoftTodo) (int, error) {
	return withTx(s.sql, rows, func(tx *sql.Tx) (*sql.Stmt, error) {
		return tx.Prepare(`INSERT OR REPLACE INTO silver_microsoft_todos
            (source, account_id, external_id, list_id, title, body, status, importance, due_at, completed_at,
             created_at_src, reminder_at, is_reminder_on, has_attachments, categories, recurrence, recurrence_std,
             checklist_items, linked_resources, deleted, updated_at)
            VALUES ('microsoft', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	}, func(stmt *sql.Stmt, r SilverMicrosoftTodo) error {
		_, err := stmt.Exec(acct(r.AccountID), r.ExternalID, r.ListID, r.Title, r.Body, r.Status,
			r.Importance, r.DueAt, r.CompletedAt, r.CreatedAtSrc, r.ReminderAt, boolInt(r.IsReminderOn),
			boolInt(r.HasAttachments), jsonOrEmpty(r.Categories), nz(r.Recurrence), nz(r.RecurrenceStd), nzArr(r.ChecklistItems),
			nzArr(r.LinkedResources), boolInt(r.Deleted), r.UpdatedAt)
		return err
	})
}
