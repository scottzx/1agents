package meta

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// SourceAccount is one connected data-source instance: a (vendor, account)
// pair with a fixed region. The account is the unit the UI manages ("源为中心":
// google + 账号A and google + 账号B are two separate sources) and its Id is the
// account_id written into the bronze layer (sources.source_records.account_id).
// Secrets never live here — vendor-specific credential stores (iCloud Keychain,
// OAuth token blobs) key off Id/Label instead.
type SourceAccount struct {
	ID        string `json:"id"`     // = bronze account_id, e.g. "google-3f9a…"
	Vendor    string `json:"vendor"` // icloud|microsoft|google|feishu
	Region    string `json:"region"` // intl|cn
	Label     string `json:"label"`  // display name (email / Apple ID)
	Status    string `json:"status"` // active|disabled
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// Vendor names are the stable discriminators shared with the bronze layer and
// the frontend. Kept here (not in internal/sources) so meta has no import cycle.
const (
	VendorICloud    = "icloud"
	VendorMicrosoft = "microsoft"
	VendorGoogle    = "google"
	VendorFeishu    = "feishu"
	VendorAgentMail = "agentmail"
)

// SourceAccountStore persists the data-source account registry.
type SourceAccountStore struct{ db *DB }

// NewSourceAccountStore returns a store over db.
func NewSourceAccountStore(db *DB) *SourceAccountStore { return &SourceAccountStore{db: db} }

// ensureSourceAccounts creates the table if absent. Run unconditionally at Open
// (CREATE IF NOT EXISTS), same rationale as ensureSourceCollectionConfig: a new
// additive table that sidesteps the meta schema-version collisions. Idempotent.
func (db *DB) ensureSourceAccounts() error {
	_, err := db.sql.Exec(`CREATE TABLE IF NOT EXISTS source_accounts (
        id         TEXT NOT NULL PRIMARY KEY,
        vendor     TEXT NOT NULL,
        region     TEXT NOT NULL DEFAULT 'intl',
        label      TEXT NOT NULL DEFAULT '',
        status     TEXT NOT NULL DEFAULT 'active',
        created_at TEXT NOT NULL DEFAULT '',
        updated_at TEXT NOT NULL DEFAULT ''
    )`)
	return err
}

// List returns every account, newest first, grouped-friendly (ordered by vendor
// then creation) for the UI.
func (s *SourceAccountStore) List() ([]SourceAccount, error) {
	rows, err := s.db.sql.Query(`SELECT id, vendor, region, label, status, created_at, updated_at
        FROM source_accounts ORDER BY vendor, created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAccounts(rows)
}

// ListByVendor returns a vendor's accounts (the set a sync run fans out over).
func (s *SourceAccountStore) ListByVendor(vendor string) ([]SourceAccount, error) {
	rows, err := s.db.sql.Query(`SELECT id, vendor, region, label, status, created_at, updated_at
        FROM source_accounts WHERE vendor = ? ORDER BY created_at`, vendor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAccounts(rows)
}

// Get returns one account by id. ok=false when absent.
func (s *SourceAccountStore) Get(id string) (SourceAccount, bool, error) {
	var a SourceAccount
	err := s.db.sql.QueryRow(`SELECT id, vendor, region, label, status, created_at, updated_at
        FROM source_accounts WHERE id = ?`, id).
		Scan(&a.ID, &a.Vendor, &a.Region, &a.Label, &a.Status, &a.CreatedAt, &a.UpdatedAt)
	if err == sql.ErrNoRows {
		return a, false, nil
	}
	if err != nil {
		return a, false, err
	}
	return a, true, nil
}

// CountByVendor returns how many accounts a vendor has (used to enforce the
// single-account vendors like Feishu).
func (s *SourceAccountStore) CountByVendor(vendor string) (int, error) {
	var n int
	err := s.db.sql.QueryRow(`SELECT COUNT(*) FROM source_accounts WHERE vendor = ?`, vendor).Scan(&n)
	return n, err
}

// Create inserts a new account. A blank Id is filled with a fresh vendor-prefixed
// id; Region/Label are trimmed. singleAccount, when true, rejects a second
// account for the vendor (Feishu). Returns the stored account.
func (s *SourceAccountStore) Create(a SourceAccount, singleAccount bool) (SourceAccount, error) {
	a.Vendor = strings.TrimSpace(a.Vendor)
	a.Region = strings.TrimSpace(a.Region)
	a.Label = strings.TrimSpace(a.Label)
	if a.Vendor == "" {
		return a, fmt.Errorf("meta: source account vendor required")
	}
	if a.Region == "" {
		a.Region = "intl"
	}
	if singleAccount {
		n, err := s.CountByVendor(a.Vendor)
		if err != nil {
			return a, err
		}
		if n > 0 {
			return a, fmt.Errorf("meta: %s supports only one account", a.Vendor)
		}
	}
	if a.ID == "" {
		a.ID = a.Vendor + "-" + newID()[:12]
	}
	if a.Status == "" {
		a.Status = "active"
	}
	now := timeToStr(time.Now().UTC())
	a.CreatedAt, a.UpdatedAt = now, now
	_, err := s.db.sql.Exec(`INSERT INTO source_accounts
        (id, vendor, region, label, status, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.Vendor, a.Region, a.Label, a.Status, a.CreatedAt, a.UpdatedAt)
	if err != nil {
		return a, err
	}
	return a, nil
}

// SetLabel updates an account's display label (used after an OAuth connect to
// reflect the signed-in mailbox address). Blank labels are ignored.
func (s *SourceAccountStore) SetLabel(id, label string) error {
	label = strings.TrimSpace(label)
	if label == "" {
		return nil
	}
	_, err := s.db.sql.Exec(`UPDATE source_accounts SET label = ?, updated_at = ? WHERE id = ?`,
		label, timeToStr(time.Now().UTC()), id)
	return err
}

// Delete removes an account by id (its bronze rows/cursors are left in place;
// re-adding the same id would resume them — deliberate, cheap tombstone-free
// removal for the framework pass).
func (s *SourceAccountStore) Delete(id string) error {
	_, err := s.db.sql.Exec(`DELETE FROM source_accounts WHERE id = ?`, id)
	return err
}

func scanAccounts(rows *sql.Rows) ([]SourceAccount, error) {
	out := []SourceAccount{}
	for rows.Next() {
		var a SourceAccount
		if err := rows.Scan(&a.ID, &a.Vendor, &a.Region, &a.Label, &a.Status, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
