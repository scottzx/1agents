package data

import "database/sql"

// silver_agentmail.go — 腾讯 Agent Mail silver: one 邮件 table, self-registered
// (issue #399).

func init() {
	registerSilverSource(silverSource{
		ddl:    agentmailSilverSchema,
		tables: []silverTableDef{{"messages", "agentmail", "silver_agentmail_mail"}},
	})
}

const agentmailSilverSchema = `
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
`

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
