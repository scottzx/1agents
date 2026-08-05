package beta

import (
	"database/sql"

	// VIOLATION app_imports_app: beta imports alpha's implementation.
	"github.com/scottzx/1Agents/backend/internal/apps/alpha"
	// VIOLATION foreign_repository_import: beta reaches into alpha's
	// repository instead of using alpha's Query/Command contracts.
	"github.com/scottzx/1Agents/backend/internal/apps/alpha/repository"
	// VIOLATION app_imports_non_sdk: apps depend only on SDK interfaces;
	// internal/meta is a kernel implementation package.
	"github.com/scottzx/1Agents/backend/internal/meta"
)

func init() {
	_ = alpha.OK
	repository.MustOpen()
	_ = meta.DefaultPath()
}

// Violations against the SQL ownership rules.
func writeOtherDomain(db *sql.DB) error {
	// VIOLATION cross_domain_sql_write: beta writes an alpha_ table.
	if _, err := db.Exec(`INSERT INTO alpha_leads (id) VALUES (?)`, "x"); err != nil {
		return err
	}
	// VIOLATION cross_domain_sql_write: beta writes a kernel-owned table.
	if _, err := db.Exec(`UPDATE projects SET name = ? WHERE id = ?`, "n", "p"); err != nil {
		return err
	}
	// VIOLATION cross_domain_sql_write: beta writes a reserved-domain table.
	_, err := db.Exec(`DELETE FROM presales_opportunities WHERE id = ?`, "o1")
	return err
}

func readOtherDomain(db *sql.DB) {
	// VIOLATION cross_domain_sql_read: beta reads alpha's tables directly.
	_, _ = db.Query(`SELECT name FROM alpha_leads`)
}
