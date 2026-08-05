package kernel

import "database/sql"

// CrossDomainWrite is a kernel/platform package writing into a reserved
// domain's tables.
// VIOLATION cross_domain_sql_write: only presales may write presales_ tables.
func CrossDomainWrite(db *sql.DB) error {
	_, err := db.Exec(`INSERT INTO presales_opportunities (id, name) VALUES (?, ?)`, "o1", "acme")
	return err
}
