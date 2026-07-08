package govern

import (
	"testing"

	"github.com/scottzx/1Agents/backend/internal/data"
)

// TestRunSQLSteps_RecordsLog verifies the runner hands each step's outcome to the
// recorder, and that data.Store's governance-run log round-trips it.
func TestRunSQLSteps_RecordsLog(t *testing.T) {
	t.Setenv("ONEAGENTS_HOME", t.TempDir())
	st, err := data.OpenDefault()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	mustExec(t, st.SQL(), `CREATE TABLE src (id TEXT PRIMARY KEY, updated_at INTEGER)`)
	mustExec(t, st.SQL(), `INSERT INTO src VALUES ('a',1),('b',2)`)

	step := SQLStep{
		Name: "gold_demo", Upstreams: []string{"src"}, Output: "gold_demo",
		CreateSQL: `CREATE TABLE IF NOT EXISTS gold_demo (id TEXT PRIMARY KEY, updated_at INTEGER)`,
		Body: `INSERT INTO gold_demo (id, updated_at) SELECT id, updated_at FROM src WHERE true
			ON CONFLICT(id) DO UPDATE SET updated_at=excluded.updated_at`,
	}

	// Recorder writes each RunRecord into the execution log.
	rec := func(r RunRecord) {
		_ = st.RecordGovernanceRun(data.GovernanceRun{
			Step: r.Step, OutputTable: r.Output, Lang: r.Lang,
			Status: r.Status, Rows: r.Rows, DurationMs: r.DurationMs, Error: r.Err,
		})
	}
	if err := RunSQLSteps(st, []SQLStep{step}, rec); err != nil {
		t.Fatalf("run: %v", err)
	}

	runs, err := st.ListGovernanceRuns("gold_demo", 10)
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs = %v (%d), err=%v", runs, len(runs), err)
	}
	got := runs[0]
	if got.Status != "success" || got.Rows != 2 || got.Lang != "sql" || got.OutputTable != "gold_demo" {
		t.Fatalf("logged run = %+v", got)
	}
	if last, ok, _ := st.LastGovernanceRun("gold_demo"); !ok || last.Rows != 2 {
		t.Fatalf("last run = %+v ok=%v", last, ok)
	}

	// A second run appends another log entry.
	if err := RunSQLSteps(st, []SQLStep{step}, rec); err != nil {
		t.Fatal(err)
	}
	if runs, _ := st.ListGovernanceRuns("gold_demo", 10); len(runs) != 2 {
		t.Fatalf("after re-run want 2 log rows, got %d", len(runs))
	}
}
