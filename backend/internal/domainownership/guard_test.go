package domainownership

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// newOwnershipDB opens an isolated sqlite DB with both domain tables and the
// denial-audit table, plus a registry where presales and commerce each own
// their namespace.
func newOwnershipDB(t *testing.T) (*sql.DB, *Registry) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ownership.db")
	// Same DSN shape as meta.Open: WAL (readers don't block writers),
	// immediate write-lock transactions that queue on busy_timeout instead
	// of failing with SQLITE_BUSY.
	db, err := sql.Open("sqlite",
		"file:"+path+"?_txlock=immediate&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	for _, ddl := range []string{
		`CREATE TABLE presales_leads (id TEXT PRIMARY KEY, name TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE commerce_products (id TEXT PRIMARY KEY, title TEXT NOT NULL DEFAULT '')`,
	} {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatalf("create table: %v", err)
		}
	}
	if err := EnsureAuditSchema(db); err != nil {
		t.Fatalf("ensure audit schema: %v", err)
	}
	reg := NewRegistry()
	for _, ns := range []string{NamespacePresales, NamespaceCommerce, NamespaceKernel} {
		if err := reg.RegisterNamespaceOwner(ns, ns); err != nil {
			t.Fatalf("register namespace %s: %v", ns, err)
		}
	}
	if err := reg.RegisterTable(NamespacePresales, "presales_leads"); err != nil {
		t.Fatal(err)
	}
	if err := reg.RegisterTable(NamespaceCommerce, "commerce_products"); err != nil {
		t.Fatal(err)
	}
	if err := reg.RegisterTable(NamespaceKernel, "kernel_access_denials"); err != nil {
		t.Fatal(err)
	}
	return db, reg
}

func TestGuardAllowsOwnDomain(t *testing.T) {
	db, reg := newOwnershipDB(t)
	g := GuardDB(NamespacePresales, reg, db)

	if _, err := g.Exec(`INSERT INTO presales_leads (id, name) VALUES (?, ?)`, "l1", "acme"); err != nil {
		t.Fatalf("own-domain write: %v", err)
	}
	rows, err := g.Query(`SELECT name FROM presales_leads WHERE id = ?`, "l1")
	if err != nil {
		t.Fatalf("own-domain read: %v", err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatal("expected one lead row")
	}
	var name string
	if err := rows.Scan(&name); err != nil || name != "acme" {
		t.Fatalf("scan: %v name=%q", err, name)
	}
	// DDL in the own namespace is allowed too.
	if _, err := g.Exec(`CREATE TABLE IF NOT EXISTS presales_notes (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("own-domain DDL: %v", err)
	}
}

func TestGuardBlocksCrossDomainWriteAndAudits(t *testing.T) {
	db, reg := newOwnershipDB(t)
	SetDenialSink(DBSink(db))
	defer SetDenialSink(nil)

	g := GuardDB(NamespaceCommerce, reg, db)
	_, err := g.Exec(`INSERT INTO presales_leads (id, name) VALUES (?, ?)`, "x", "stolen")
	if !IsCode(err, CodeCrossDomainWrite) {
		t.Fatalf("want cross_domain_write, got %v", err)
	}
	// The write must NOT have happened.
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM presales_leads`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("cross-domain write leaked into the table: count=%d err=%v", n, err)
	}
	// UPDATE and DELETE are blocked as well.
	if _, err := g.Exec(`UPDATE presales_leads SET name = ?`, "x"); !IsCode(err, CodeCrossDomainWrite) {
		t.Fatalf("UPDATE: want cross_domain_write, got %v", err)
	}
	if _, err := g.Exec(`DELETE FROM presales_leads`); !IsCode(err, CodeCrossDomainWrite) {
		t.Fatalf("DELETE: want cross_domain_write, got %v", err)
	}
	// Direct reads of another domain are blocked too (§4.2).
	if _, err := g.Query(`SELECT id FROM presales_leads`); !IsCode(err, CodeCrossDomainRead) {
		t.Fatalf("SELECT: want cross_domain_read, got %v", err)
	}

	denials, err := Denials(db, 50)
	if err != nil {
		t.Fatalf("read denials: %v", err)
	}
	if len(denials) < 4 {
		t.Fatalf("expected >=4 audited denials, got %d", len(denials))
	}
	found := map[Action]bool{}
	for _, d := range denials {
		found[d.Action] = true
		if d.CallerNamespace != NamespaceCommerce {
			t.Errorf("denial caller = %q, want commerce", d.CallerNamespace)
		}
	}
	if !found[ActionTableWrite] || !found[ActionTableRead] {
		t.Errorf("missing audited actions: %+v", denials)
	}
}

func TestGuardTxEnforcesOwnershipInsideCommandTransactions(t *testing.T) {
	db, reg := newOwnershipDB(t)

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	g := GuardTx(NamespacePresales, reg, tx)
	if _, err := g.Exec(`INSERT INTO presales_leads (id) VALUES (?)`, "l2"); err != nil {
		t.Fatalf("own write in tx: %v", err)
	}
	if _, err := g.Exec(`INSERT INTO commerce_products (id) VALUES (?)`, "p1"); !IsCode(err, CodeCrossDomainWrite) {
		t.Fatalf("want cross_domain_write in tx, got %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
}

func TestCheckClassifiesStatements(t *testing.T) {
	_, reg := newOwnershipDB(t)
	g := GuardDB(NamespaceCommerce, reg, nil) // Check never executes

	cases := []struct {
		query   string
		wantErr Code // "" means allowed
	}{
		{`INSERT OR IGNORE INTO presales_leads (id) VALUES ('a')`, CodeCrossDomainWrite},
		{`REPLACE INTO presales_leads (id) VALUES ('a')`, CodeCrossDomainWrite},
		{`CREATE INDEX idx_presales ON presales_leads (name)`, CodeCrossDomainWrite},
		{`DROP TABLE presales_leads`, CodeCrossDomainWrite},
		{`ALTER TABLE presales_leads ADD COLUMN x TEXT`, CodeCrossDomainWrite},
		{`WITH recent AS (SELECT * FROM presales_leads) SELECT * FROM recent`, CodeCrossDomainRead},
		{`SELECT c.id FROM commerce_products c JOIN presales_leads l ON l.id = c.id`, CodeCrossDomainRead},
		{`INSERT INTO commerce_products (id) VALUES ('ok')`, ""},
		{`SELECT id FROM commerce_products`, ""},
		{`PRAGMA table_info(commerce_products)`, ""},
	}
	for _, c := range cases {
		err := g.Check(c.query)
		if c.wantErr == "" {
			if err != nil {
				t.Errorf("Check(%q) = %v, want allowed", c.query, err)
			}
			continue
		}
		if !IsCode(err, c.wantErr) {
			t.Errorf("Check(%q) = %v, want code %s", c.query, err, c.wantErr)
		}
	}
}
