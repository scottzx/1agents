package data

import "testing"

// TestListTable_And_Truncate covers the generic governance-output reader + the
// rebuild truncate: any table is read SELECT * newest-first, and TruncateGovernanceOutput
// clears it (no-op on an absent table, rejects an unsafe name).
func TestListTable_And_Truncate(t *testing.T) {
	t.Setenv("ONEAGENTS_HOME", t.TempDir())
	st, err := OpenDefault()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	mustExec := func(q string) {
		if _, err := st.SQL().Exec(q); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	mustExec(`CREATE TABLE unified_contacts (id TEXT PRIMARY KEY, full_name TEXT, updated_at INTEGER)`)
	mustExec(`INSERT INTO unified_contacts VALUES ('a','Ann',10),('b','Bob',20)`)

	rows, err := st.ListTable("unified_contacts", 10)
	if err != nil {
		t.Fatalf("ListTable: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows=%d want 2", len(rows))
	}
	// Newest-first by updated_at, id → UID, name → preview.
	if rows[0].UID != "b" || rows[0].Preview != "Bob" {
		t.Fatalf("row0=%+v", rows[0])
	}

	// Unknown table is an explicit error (not a panic); unsafe name rejected.
	if _, err := st.ListTable("no_such_table", 10); err == nil {
		t.Fatal("expected error for unknown table")
	}
	if _, err := st.ListTable("bad name;drop", 10); err == nil {
		t.Fatal("expected error for unsafe table name")
	}

	// Truncate clears rows; a missing table is a no-op; unsafe name rejected.
	if err := st.TruncateGovernanceOutput("unified_contacts"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if rows, _ := st.ListTable("unified_contacts", 10); len(rows) != 0 {
		t.Fatalf("after truncate rows=%d want 0", len(rows))
	}
	if err := st.TruncateGovernanceOutput("never_created"); err != nil {
		t.Fatalf("truncate missing table should be no-op: %v", err)
	}
	if err := st.TruncateGovernanceOutput("bad;name"); err == nil {
		t.Fatal("expected error for unsafe truncate name")
	}
}
