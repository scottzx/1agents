package data

import (
	"database/sql"
	"encoding/json"
)

// silver.go holds the SHARED silver-layer scaffolding; each source's row types,
// upserts, and DDL live in their own silver_<source>.go file (issue #399).
//
// Silver rows are per-SOURCE and source-faithful: each type mirrors one source's
// cleaned shape with its own native columns, so nothing valuable is flattened
// away. A governor parses a bronze payload into one of these and upserts it on
// the bronze grain (source, account_id, external_id). UpdatedAt carries the
// bronze fetched_at through verbatim, keeping it monotonic across a re-govern so
// the silver→gold stage can read `updated_at > cursor`. Cross-source unification
// is gold's job, not silver's.

// ---- shared value fragments (JSON-encoded into silver columns) ----

// EmailRef is one mail address+name (MS/AgentMail from & recipients).
type EmailRef struct {
	Addr string `json:"addr"`
	Name string `json:"name,omitempty"`
}

// ---- small helpers (shared by every source's upsert) ----

func jsonOrEmpty(v any) string {
	b, err := json.Marshal(v)
	if err != nil || string(b) == "null" {
		return "[]"
	}
	return string(b)
}

// nz defaults an empty raw-JSON string to "" (object/scalar columns).
func nz(s string) string { return s }

// nzArr defaults an empty raw-JSON array string to "[]".
func nzArr(s string) string {
	if s == "" {
		return "[]"
	}
	return s
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// acct defaults an empty account id to "default" (matching bronze).
func acct(id string) string {
	if id == "" {
		return "default"
	}
	return id
}

// withTx runs one prepared statement over rows in a single transaction, counting
// successful execs. Generic so every silver upsert shares the boilerplate.
func withTx[T any](db *sql.DB, rows []T, prep func(*sql.Tx) (*sql.Stmt, error), exec func(*sql.Stmt, T) error) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	stmt, err := prep(tx)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	n := 0
	for _, r := range rows {
		if err := exec(stmt, r); err != nil {
			return 0, err
		}
		n++
	}
	return n, tx.Commit()
}
