package kernel

import "database/sql"

// Legal kernel writes: kernel-owned legacy tables and kernel_-prefixed new
// tables. The gate must NOT flag these.
func KernelWrites(db *sql.DB) error {
	if _, err := db.Exec(`INSERT INTO projects (id, name) VALUES (?, ?)`, "p", "n"); err != nil {
		return err
	}
	if _, err := db.Exec(`INSERT INTO work_cases (id, project_id) VALUES (?, ?)`, "c", "p"); err != nil {
		return err
	}
	_, err := db.Exec(`INSERT INTO kernel_access_denials (at) VALUES (?)`, "now")
	return err
}

// Kernel reads of domain tables are permitted (the kernel is the trusted
// base); only writes are gated.
func KernelReads(db *sql.DB) {
	_, _ = db.Query(`SELECT id FROM presales_opportunities`)
}
