package data

import "database/sql"

// silver_icloud.go — Apple/iCloud silver: DDL + row type + upsert, self-registered
// so the source is added by this one file (issue #399).

func init() {
	registerSilverSource(silverSource{
		ddl:    icloudSilverSchema,
		tables: []silverTableDef{{"contacts", "icloud", "silver_icloud_contacts"}},
	})
}

// 联系人 · Apple/iCloud — lossless vCard: promoted columns only (bronze holds the
// verbatim vCard, so silver never re-duplicates the raw payload).
const icloudSilverSchema = `
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
    deleted      INTEGER NOT NULL DEFAULT 0,
    updated_at   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (source, account_id, external_id)
);
CREATE INDEX IF NOT EXISTS idx_silver_icloud_contacts_wm ON silver_icloud_contacts(updated_at);
`

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
