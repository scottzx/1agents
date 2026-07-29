package meta

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func assertTableColumns(t *testing.T, db *DB, table string, want []string) {
	t.Helper()
	got, err := db.tableColumns(table)
	if err != nil {
		t.Fatalf("tableColumns(%s): %v", table, err)
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("%s.%s is missing", table, name)
		}
	}
}

func assertSchemaObject(t *testing.T, db *DB, kind, name string) {
	t.Helper()
	var count int
	if err := db.sql.QueryRow(
		`SELECT COUNT(1) FROM sqlite_master WHERE type = ? AND name = ?`,
		kind, name,
	).Scan(&count); err != nil {
		t.Fatalf("find %s %s: %v", kind, name, err)
	}
	if count != 1 {
		t.Errorf("%s %s count = %d, want 1", kind, name, count)
	}
}

// stripFeatureCatalogSchema rebuilds milestones in its v27 shape and removes
// the feature catalog tables. It lets the migration tests exercise both the
// normal v27 upgrade and the high-user_version reconcile path from the same
// realistic pre-#290 physical schema.
func stripFeatureCatalogSchema(t *testing.T, path string, userVersion int) {
	t.Helper()
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()

	if _, err := raw.Exec(`
		DROP TABLE IF EXISTS feature_item_links;
		DROP TABLE IF EXISTS feature_nodes;
		DROP INDEX IF EXISTS idx_milestones_project_version;

		CREATE TABLE milestones_v27 (
			id             TEXT PRIMARY KEY,
			project_id     TEXT NOT NULL,
			name           TEXT NOT NULL DEFAULT '',
			description    TEXT NOT NULL DEFAULT '',
			target_date    TEXT,
			position       INTEGER NOT NULL DEFAULT 0,
			predecessor_id TEXT NOT NULL DEFAULT '',
			created_at     TEXT NOT NULL,
			updated_at     TEXT NOT NULL
		);
		INSERT INTO milestones_v27 (
			id, project_id, name, description, target_date, position,
			predecessor_id, created_at, updated_at
		)
		SELECT
			id, project_id, name, description, target_date, position,
			predecessor_id, created_at, updated_at
		FROM milestones;
		DROP TABLE milestones;
		ALTER TABLE milestones_v27 RENAME TO milestones;
		CREATE UNIQUE INDEX idx_milestones_proj_name
			ON milestones(project_id, name);
		CREATE INDEX idx_milestones_project
			ON milestones(project_id, position);
	`); err != nil {
		t.Fatalf("strip feature catalog schema: %v", err)
	}
	if _, err := raw.Exec(fmt.Sprintf("PRAGMA user_version = %d", userVersion)); err != nil {
		t.Fatalf("set user_version=%d: %v", userVersion, err)
	}
}

func TestFeatureCatalogSchemaFreshDatabase(t *testing.T) {
	db := newTestDB(t)

	assertTableColumns(t, db, "feature_nodes", []string{
		"id", "project_id", "parent_id", "kind", "title", "description",
		"target_milestone_id", "position", "created_at", "updated_at",
	})
	assertTableColumns(t, db, "feature_item_links", []string{
		"feature_id", "item_id", "relation", "created_at",
	})
	assertTableColumns(t, db, "milestones", []string{"version"})
	assertSchemaObject(t, db, "index", "idx_feature_nodes_project_parent")
	assertSchemaObject(t, db, "index", "idx_feature_item_links_item")
	assertSchemaObject(t, db, "index", "idx_milestones_project_version")

	var version int
	if err := db.sql.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != schemaVersion {
		t.Fatalf("user_version = %d, want %d", version, schemaVersion)
	}

	now := timeToStr(time.Now().UTC())
	for _, projectID := range []string{"project-a", "project-b"} {
		if _, err := db.sql.Exec(`
			INSERT INTO projects (id, name, workspace_path, status, created_at, updated_at)
			VALUES (?, ?, ?, 'active', ?, ?)`,
			projectID, projectID, filepath.Join(t.TempDir(), projectID), now, now,
		); err != nil {
			t.Fatalf("seed %s: %v", projectID, err)
		}
	}
	insertMilestone := func(id, projectID, name, semver string) error {
		_, err := db.sql.Exec(`
			INSERT INTO milestones (
				id, project_id, name, version, description, target_date,
				position, predecessor_id, created_at, updated_at
			) VALUES (?, ?, ?, ?, '', NULL, 0, '', ?, ?)`,
			id, projectID, name, semver, now, now,
		)
		return err
	}
	if err := insertMilestone("m1", "project-a", "0.1.0", "0.1.0"); err != nil {
		t.Fatalf("insert first version: %v", err)
	}
	if err := insertMilestone("m2", "project-a", "another-name", "0.1.0"); err == nil {
		t.Fatal("duplicate non-empty version in one project should fail")
	}
	if err := insertMilestone("m3", "project-b", "0.1.0", "0.1.0"); err != nil {
		t.Fatalf("same version in another project should be allowed: %v", err)
	}
}

func TestFeatureCatalogSchemaV27UpgradePreservesLegacyMilestone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.db")
	workspace := t.TempDir()

	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewTaskStore(db)
	if _, err := store.CreateMilestone(workspace, "历史阶段", "legacy", nil, ""); err != nil {
		t.Fatalf("create legacy milestone: %v", err)
	}
	now := time.Now().UTC()
	if err := store.Mutate(workspace, func(cfg *TasksConfig) bool {
		cfg.Tasks = append(cfg.Tasks, ProjectItem{
			ID:        "legacy-task",
			Title:     "保留任务关联",
			Type:      ItemTypeTask,
			Status:    TaskStatusPending,
			Milestone: "历史阶段",
			CreatedAt: now,
			UpdatedAt: now,
		})
		return true
	}); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	stripFeatureCatalogSchema(t, path, 27)

	upgraded, err := Open(path)
	if err != nil {
		t.Fatalf("Open v27 database: %v", err)
	}
	defer upgraded.Close()

	var version int
	if err := upgraded.sql.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != schemaVersion {
		t.Fatalf("user_version = %d, want %d", version, schemaVersion)
	}
	assertTableColumns(t, upgraded, "feature_nodes", []string{"id", "project_id"})
	assertTableColumns(t, upgraded, "feature_item_links", []string{"feature_id", "item_id"})
	assertTableColumns(t, upgraded, "milestones", []string{"version"})

	milestones, err := NewTaskStore(upgraded).ListMilestones(workspace)
	if err != nil {
		t.Fatalf("ListMilestones after upgrade: %v", err)
	}
	if len(milestones) != 1 {
		t.Fatalf("milestones len = %d, want 1", len(milestones))
	}
	if milestones[0].Name != "历史阶段" || milestones[0].Version != "" || !milestones[0].IsLegacy {
		t.Fatalf("legacy milestone changed during upgrade: %+v", milestones[0])
	}
	task, ok, err := NewTaskStore(upgraded).GetTask("legacy-task")
	if err != nil || !ok {
		t.Fatalf("GetTask after upgrade: ok=%v err=%v", ok, err)
	}
	if task.Milestone != "历史阶段" {
		t.Fatalf("task milestone = %q, want 历史阶段", task.Milestone)
	}
}

func TestFeatureCatalogSchemaReconcilesAboveCurrentUserVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	const futureVersion = schemaVersion + 7
	stripFeatureCatalogSchema(t, path, futureVersion)

	reconciled, err := Open(path)
	if err != nil {
		t.Fatalf("Open high-version database: %v", err)
	}
	assertTableColumns(t, reconciled, "feature_nodes", []string{"id", "project_id", "parent_id"})
	assertTableColumns(t, reconciled, "feature_item_links", []string{"feature_id", "item_id", "relation"})
	assertTableColumns(t, reconciled, "milestones", []string{"version"})
	assertSchemaObject(t, reconciled, "index", "idx_milestones_project_version")

	var version int
	if err := reconciled.sql.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != futureVersion {
		t.Fatalf("reconcile changed future user_version from %d to %d", futureVersion, version)
	}
	if err := reconciled.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("second Open after reconcile must be idempotent: %v", err)
	}
	defer reopened.Close()
	assertSchemaObject(t, reopened, "index", "idx_feature_nodes_project_parent")
	assertSchemaObject(t, reopened, "index", "idx_feature_item_links_item")
}
