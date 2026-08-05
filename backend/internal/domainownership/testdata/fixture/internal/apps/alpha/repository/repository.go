package repository

import "database/sql"

// MustOpen is alpha's private repository: the only code allowed to touch
// alpha_* tables.
func MustOpen() {}

func insertLead(db *sql.DB) error {
	_, err := db.Exec(`INSERT INTO alpha_leads (id, name) VALUES (?, ?)`, "l1", "acme")
	return err
}

func readLeads(db *sql.DB) error {
	_, err := db.Query(`SELECT id FROM alpha_leads`)
	_ = err
	return nil
}
