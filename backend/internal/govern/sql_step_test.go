package govern

import (
	"database/sql"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/data"
)

// TestSQLStep_MultiUpstreamJoinUpsertIncremental exercises the full contract: a
// step that JOINs two silver tables into a gold table, ON CONFLICT upsert, and a
// cursor that makes a re-run process only new rows.
func TestSQLStep_MultiUpstreamJoinUpsertIncremental(t *testing.T) {
	t.Setenv("ONEAGENTS_HOME", t.TempDir())
	st, err := data.OpenDefault()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db := st.SQL()

	// Two upstream silver tables (person + score), joined on id.
	mustExec(t, db, `CREATE TABLE silver_person (id TEXT PRIMARY KEY, name TEXT, updated_at INTEGER)`)
	mustExec(t, db, `CREATE TABLE silver_score (id TEXT PRIMARY KEY, points INTEGER, updated_at INTEGER)`)
	mustExec(t, db, `INSERT INTO silver_person VALUES ('a','Ann',100),('b','Bob',100)`)
	mustExec(t, db, `INSERT INTO silver_score  VALUES ('a',10,100),('b',20,100)`)

	step := SQLStep{
		Name:      "gold_ranked",
		Upstreams: []string{"silver_person", "silver_score"},
		Output:    "gold_ranked",
		IncrTable: "silver_person",
		IncrCol:   "updated_at",
		CreateSQL: `CREATE TABLE IF NOT EXISTS gold_ranked (
			external_id TEXT PRIMARY KEY, name TEXT, points INTEGER, updated_at INTEGER)`,
		Body: `INSERT INTO gold_ranked (external_id, name, points, updated_at)
			SELECT p.id, p.name, s.points, p.updated_at
			FROM silver_person p JOIN silver_score s ON s.id = p.id
			WHERE p.updated_at > :since
			ON CONFLICT(external_id) DO UPDATE SET
				name=excluded.name, points=excluded.points, updated_at=excluded.updated_at`,
	}

	// First run: both rows join through.
	if n, err := RunSQLStep(st, step); err != nil || n != 2 {
		t.Fatalf("run1 = %d/%v, want 2", n, err)
	}
	if got := scalarInt(t, db, `SELECT points FROM gold_ranked WHERE external_id='a'`); got != 10 {
		t.Fatalf("a.points = %d, want 10", got)
	}

	// Re-run with no new upstream rows: cursor gates it to zero.
	if n, err := RunSQLStep(st, step); err != nil || n != 0 {
		t.Fatalf("run2 (no change) = %d/%v, want 0", n, err)
	}

	// New + updated rows past the watermark: incremental picks them up, upsert
	// overwrites the existing one.
	mustExec(t, db, `INSERT INTO silver_person VALUES ('c','Cid',200)`)
	mustExec(t, db, `INSERT INTO silver_score  VALUES ('c',30,200)`)
	mustExec(t, db, `UPDATE silver_person SET name='Bobby', updated_at=200 WHERE id='b'`)
	mustExec(t, db, `UPDATE silver_score  SET points=99,      updated_at=200 WHERE id='b'`)
	if n, err := RunSQLStep(st, step); err != nil || n != 2 { // b (updated) + c (new)
		t.Fatalf("run3 = %d/%v, want 2", n, err)
	}
	if got := scalarInt(t, db, `SELECT points FROM gold_ranked WHERE external_id='b'`); got != 99 {
		t.Fatalf("b.points after upsert = %d, want 99", got)
	}
	if total := scalarInt(t, db, `SELECT COUNT(*) FROM gold_ranked`); total != 3 {
		t.Fatalf("total gold rows = %d, want 3", total)
	}
}

func TestTopoSortSteps(t *testing.T) {
	// b depends on a's output; c independent. Expect a before b.
	steps := []SQLStep{
		{Name: "b", Upstreams: []string{"t_a"}, Output: "t_b"},
		{Name: "a", Upstreams: []string{"silver_x"}, Output: "t_a"},
		{Name: "c", Upstreams: []string{"silver_y"}, Output: "t_c"},
	}
	ordered, err := topoSortSteps(steps)
	if err != nil {
		t.Fatal(err)
	}
	pos := map[string]int{}
	for i, s := range ordered {
		pos[s.Name] = i
	}
	if pos["a"] > pos["b"] {
		t.Fatalf("a must run before b: %v", pos)
	}

	// A cycle is rejected.
	cyc := []SQLStep{
		{Name: "x", Upstreams: []string{"t_y"}, Output: "t_x"},
		{Name: "y", Upstreams: []string{"t_x"}, Output: "t_y"},
	}
	if _, err := topoSortSteps(cyc); err == nil {
		t.Fatal("expected cycle error")
	}
}

func mustExec(t *testing.T, db *sql.DB, q string) {
	t.Helper()
	if _, err := db.Exec(q); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

func scalarInt(t *testing.T, db *sql.DB, q string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(q).Scan(&n); err != nil {
		t.Fatalf("scalar %q: %v", q, err)
	}
	return n
}
