package meta

// reset.go supports the "重置本地数据" feature (settings → 重置数据): wipe all
// App-owned rows in place while keeping the database file and its schema
// (user_version) untouched, so the server keeps running against the same
// connection. relay/pairing identity lives in files under ~/.happy and
// ~/.1agents/relay-creds.json — never in meta.db — so clearing these tables can
// never touch it.

// dataTables is the set of meta.db tables that hold App data and are safe to
// truncate on reset. Schema-versioning state (PRAGMA user_version) is metadata,
// not a table, so it survives untouched.
var dataTables = []string{
	"project_events",
	"task_runs",
	"turn_change_reports",
	"agent_turns",
	"feature_catalog_restore_requests",
	"feature_catalog_versions",
	"feature_item_links",
	"feature_nodes",
	"projects",
	"project_items",
	"task_deps",
	"replies",
	"sessions",
	"milestones",
	"inbox_items",
	"digest_templates",
	"digest_bindings",
}

// ClearAllData deletes every row from the App data tables in a single
// transaction (all-or-nothing). The file and schema are left intact. Returns
// the list of tables cleared so the caller can report a summary.
func (db *DB) ClearAllData() ([]string, error) {
	tx, err := db.sql.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	for _, t := range dataTables {
		if _, err := tx.Exec("DELETE FROM " + t); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	cleared := make([]string, len(dataTables))
	copy(cleared, dataTables)
	return cleared, nil
}
