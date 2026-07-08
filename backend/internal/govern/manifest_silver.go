package govern

// manifest_silver.go is the ONE generic bronze→silver governor shared by every
// manifest-declared REST source (训记 and friends). Where each built-in source has
// a hand-written silver_<source>.go, a manifest source needs none: this governor
// reads the source's bronze rows and lands each into a generic silver table —
// stable meta columns (source/account_id/external_id/payload/deleted/updated_at)
// plus any payload fields the manifest chose to "promote" to their own columns for
// nicer viewer rendering. The full payload is always kept, so nothing is lost and
// the schema-free viewer renders it verbatim. Cursor-incremental + idempotent,
// same contract as the built-in governors.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/scottzx/1Agents/backend/internal/data"
	"github.com/scottzx/1Agents/backend/internal/sources"
)

// ManifestSilverSpec declares a manifest source's generic bronze→silver transform.
type ManifestSilverSpec struct {
	Source  string            // bronze source discriminator (vendor)
	Kind    string            // bronze kind
	Table   string            // silver table name (validated identifier)
	Domain  string            // viewer domain
	Promote map[string]string // silver column → dotted json path in payload
}

var identRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// promotedCols returns the promoted column names in stable (sorted) order.
func (s ManifestSilverSpec) promotedCols() []string {
	cols := make([]string, 0, len(s.Promote))
	for c := range s.Promote {
		cols = append(cols, c)
	}
	sort.Strings(cols)
	return cols
}

func (s ManifestSilverSpec) validate() error {
	if !identRe.MatchString(s.Table) {
		return fmt.Errorf("govern: bad silver table name %q", s.Table)
	}
	for _, c := range s.promotedCols() {
		if !identRe.MatchString(c) || isReservedSilverCol(c) {
			return fmt.Errorf("govern: bad promoted column %q", c)
		}
	}
	return nil
}

func isReservedSilverCol(c string) bool {
	switch c {
	case "source", "account_id", "external_id", "payload", "deleted", "updated_at":
		return true
	}
	return false
}

// SilverManifest lands the source's newly-synced bronze rows into its generic
// silver table, returning how many rows were written.
func SilverManifest(src *sources.Store, dst *data.Store, spec ManifestSilverSpec) (int, error) {
	if err := spec.validate(); err != nil {
		return 0, err
	}
	db := dst.SQL()
	if err := ensureManifestSilverTable(db, spec); err != nil {
		return 0, err
	}
	since, err := dst.GovernCursor(data.StageSilver, spec.Source, spec.Kind)
	if err != nil {
		return 0, err
	}
	recs, maxFetched, err := src.RecordsSince(spec.Source, spec.Kind, since)
	if err != nil {
		return 0, err
	}
	if len(recs) == 0 {
		return 0, nil
	}

	promoted := spec.promotedCols()
	cols := append([]string{"source", "account_id", "external_id"}, promoted...)
	cols = append(cols, "payload", "deleted", "updated_at")
	stmt := upsertSQL(spec.Table, cols)

	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range recs {
		args := make([]any, 0, len(cols))
		args = append(args, spec.Source, r.AccountID, r.UID)
		for _, c := range promoted {
			args = append(args, payloadField(r.Payload, spec.Promote[c]))
		}
		args = append(args, r.Payload, boolInt(r.Deleted), r.FetchedAt)
		if _, err := tx.Exec(stmt, args...); err != nil {
			_ = tx.Rollback()
			return n, err
		}
		n++
	}
	if err := tx.Commit(); err != nil {
		return n, err
	}
	if err := dst.SaveGovernCursor(data.StageSilver, spec.Source, spec.Kind, maxFetched); err != nil {
		return n, err
	}
	return n, nil
}

// ensureManifestSilverTable builds the generic silver table (idempotent). Runtime
// DDL because manifest sources register after data.db was opened.
func ensureManifestSilverTable(db *sql.DB, spec ManifestSilverSpec) error {
	var b strings.Builder
	b.WriteString("CREATE TABLE IF NOT EXISTS " + spec.Table + " (\n")
	b.WriteString("  source TEXT NOT NULL,\n  account_id TEXT NOT NULL DEFAULT 'default',\n  external_id TEXT NOT NULL,\n")
	for _, c := range spec.promotedCols() {
		b.WriteString("  " + c + " TEXT NOT NULL DEFAULT '',\n")
	}
	b.WriteString("  payload TEXT NOT NULL DEFAULT '',\n  deleted INTEGER NOT NULL DEFAULT 0,\n  updated_at INTEGER NOT NULL DEFAULT 0,\n")
	b.WriteString("  PRIMARY KEY (source, account_id, external_id)\n)")
	_, err := db.Exec(b.String())
	return err
}

// upsertSQL builds an INSERT ... ON CONFLICT DO UPDATE over the (source,
// account_id, external_id) grain, updating every non-key column from excluded.
func upsertSQL(table string, cols []string) string {
	ph := make([]string, len(cols))
	for i := range cols {
		ph[i] = "?"
	}
	var sets []string
	for _, c := range cols {
		if isReservedKey(c) {
			continue
		}
		sets = append(sets, c+"=excluded."+c)
	}
	return fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s) ON CONFLICT(source, account_id, external_id) DO UPDATE SET %s",
		table, strings.Join(cols, ", "), strings.Join(ph, ", "), strings.Join(sets, ", "),
	)
}

func isReservedKey(c string) bool {
	return c == "source" || c == "account_id" || c == "external_id"
}

// payloadField navigates a dotted path through the JSON payload and returns the
// value as a string ("" when absent).
func payloadField(payload, path string) string {
	if path == "" {
		return ""
	}
	cur := json.RawMessage(payload)
	for _, part := range strings.Split(path, ".") {
		var m map[string]json.RawMessage
		if json.Unmarshal(cur, &m) != nil {
			return ""
		}
		nx, ok := m[part]
		if !ok {
			return ""
		}
		cur = nx
	}
	var s string
	if json.Unmarshal(cur, &s) == nil {
		return s
	}
	return strings.Trim(string(cur), `"`)
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
