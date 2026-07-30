package meta

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"
)

func seedVersionedCatalog(t *testing.T) (*DB, *FeatureCatalogStore, string, FeatureNode, FeatureNode, Milestone) {
	t.Helper()
	db, store, path, _ := featureCatalogRig(t)
	taskStore := NewTaskStore(db)
	now := time.Now().UTC().Add(-time.Hour)
	if err := taskStore.Mutate(path, func(cfg *TasksConfig) bool {
		cfg.Tasks = append(cfg.Tasks,
			ProjectItem{
				ID: "version-req", Title: "Requirement", Type: ItemTypeRequirement,
				Status: TaskStatusPending, CreatedAt: now, UpdatedAt: now,
			},
			ProjectItem{
				ID: "version-task", Title: "Delivery", Type: ItemTypeTask,
				Status: TaskStatusPending, CreatedAt: now, UpdatedAt: now,
			},
		)
		return true
	}); err != nil {
		t.Fatal(err)
	}
	milestone, err := taskStore.CreateVersionMilestone(path, MilestoneBumpMinor, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	root := createFeatureNode(t, store, FeatureNode{
		ID: "version-root", ProjectID: "p1", Kind: FeatureNodeModule,
		Title: "Original root",
	})
	feature := createFeatureNode(t, store, FeatureNode{
		ID: "version-feature", ProjectID: "p1", ParentID: root.ID,
		Kind: FeatureNodePoint, Title: "Original feature", Documents: []string{"docs/original.md"},
		Description: "snapshot description", TargetMilestoneID: milestone.ID,
	})
	if _, _, err := store.LinkItem(
		"p1", feature.ID, "version-req", FeatureItemSource, testFeatureEvent(),
	); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.LinkItem(
		"p1", feature.ID, "version-task", FeatureItemDelivery, testFeatureEvent(),
	); err != nil {
		t.Fatal(err)
	}
	return db, store, path, root, feature, milestone
}

func TestFeatureCatalogVersionRoundTripAndIdempotentSafetyRestore(t *testing.T) {
	db, store, _, root, feature, _ := seedVersionedCatalog(t)
	version, err := store.CreateVersion("p1", "  Release A  ", testFeatureEvent())
	if err != nil {
		t.Fatal(err)
	}
	if version.Alias != "Release A" || version.Kind != FeatureCatalogVersionManual ||
		version.NodeCount != 2 || version.LinkCount != 2 {
		t.Fatalf("unexpected version metadata: %+v", version)
	}
	if _, err := store.RenameVersion("p2", version.ID, "leak", testFeatureEvent()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-project rename err=%v, want ErrNotFound", err)
	}

	changed := "Changed after snapshot"
	if _, err := store.Update(
		"p1", feature.ID, FeatureNodePatch{Title: &changed}, testFeatureEvent(),
	); err != nil {
		t.Fatal(err)
	}
	extra := createFeatureNode(t, store, FeatureNode{
		ID: "post-snapshot", ProjectID: "p1", ParentID: root.ID,
		Kind: FeatureNodePoint, Title: "Post snapshot",
	})
	if err := store.UnlinkItem(
		"p1", feature.ID, "version-task", FeatureItemDelivery, testFeatureEvent(),
	); err != nil {
		t.Fatal(err)
	}

	result, err := store.RestoreVersion("p1", version.ID, "restore-once", testFeatureEvent())
	if err != nil {
		t.Fatal(err)
	}
	if result.SafetyVersion.Kind != FeatureCatalogVersionPreRestore ||
		result.RestoredNodeCount != 2 || result.RestoredLinkCount != 2 {
		t.Fatalf("unexpected restore result: %+v", result)
	}
	catalog, err := store.List("p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Nodes) != 2 || len(catalog.Links) != 2 {
		t.Fatalf("restored catalog = %+v", catalog)
	}
	if _, ok, err := store.Get("p1", extra.ID); err != nil || ok {
		t.Fatalf("post-snapshot node survived restore: ok=%v err=%v", ok, err)
	}
	restored, ok, err := store.Get("p1", feature.ID)
	if err != nil || !ok || restored.Title != "Original feature" ||
		restored.Description != "snapshot description" || len(restored.Documents) != 1 ||
		restored.Documents[0] != "docs/original.md" {
		t.Fatalf("feature not restored exactly: ok=%v node=%+v err=%v", ok, restored, err)
	}

	var versionsBefore, eventsBefore int
	if err := db.sql.QueryRow(`
		SELECT COUNT(1) FROM feature_catalog_versions WHERE project_id = 'p1'`,
	).Scan(&versionsBefore); err != nil {
		t.Fatal(err)
	}
	if err := db.sql.QueryRow(`
		SELECT COUNT(1) FROM project_events
		WHERE project_id = 'p1' AND target_type = 'feature_catalog_version'`,
	).Scan(&eventsBefore); err != nil {
		t.Fatal(err)
	}
	retried, err := store.RestoreVersion("p1", version.ID, "restore-once", testFeatureEvent())
	if err != nil {
		t.Fatal(err)
	}
	if retried.SafetyVersion.ID != result.SafetyVersion.ID {
		t.Fatalf("idempotent result changed: first=%+v retry=%+v", result, retried)
	}
	var versionsAfter, eventsAfter int
	db.sql.QueryRow(`SELECT COUNT(1) FROM feature_catalog_versions WHERE project_id = 'p1'`).Scan(&versionsAfter)
	db.sql.QueryRow(`
		SELECT COUNT(1) FROM project_events
		WHERE project_id = 'p1' AND target_type = 'feature_catalog_version'`,
	).Scan(&eventsAfter)
	if versionsAfter != versionsBefore || eventsAfter != eventsBefore {
		t.Fatalf("retry created side effects: versions %d→%d events %d→%d",
			versionsBefore, versionsAfter, eventsBefore, eventsAfter)
	}

	if _, err := store.RestoreVersion(
		"p1", result.SafetyVersion.ID, "restore-safety", testFeatureEvent(),
	); err != nil {
		t.Fatal(err)
	}
	safetyCatalog, err := store.List("p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(safetyCatalog.Nodes) != 3 || len(safetyCatalog.Links) != 1 {
		t.Fatalf("safety restore did not return to pre-restore state: %+v", safetyCatalog)
	}
	safetyFeature, _, _ := store.Get("p1", feature.ID)
	if safetyFeature.Title != changed {
		t.Fatalf("safety restore title=%q, want %q", safetyFeature.Title, changed)
	}
}

func TestFeatureCatalogRestoreSkipsStaleReferencesAndClearsMilestone(t *testing.T) {
	db, store, path, _, feature, milestone := seedVersionedCatalog(t)
	version, err := store.CreateVersion("p1", "with refs", testFeatureEvent())
	if err != nil {
		t.Fatal(err)
	}
	if err := NewTaskStore(db).Mutate(path, func(cfg *TasksConfig) bool {
		filtered := cfg.Tasks[:0]
		for _, item := range cfg.Tasks {
			if item.ID != "version-req" && item.ID != "version-task" {
				filtered = append(filtered, item)
			}
		}
		cfg.Tasks = filtered
		return true
	}); err != nil {
		t.Fatal(err)
	}
	if err := NewTaskStore(db).DeleteMilestone(path, milestone.ID); err != nil {
		t.Fatal(err)
	}
	result, err := store.RestoreVersion("p1", version.ID, "stale-refs", testFeatureEvent())
	if err != nil {
		t.Fatal(err)
	}
	if result.SkippedLinkCount != 2 || result.ClearedTargetMilestoneCount != 1 ||
		result.RestoredLinkCount != 0 || len(result.Warnings) != 3 {
		t.Fatalf("unexpected restore warnings: %+v", result)
	}
	restored, ok, err := store.Get("p1", feature.ID)
	if err != nil || !ok || restored.TargetMilestoneID != "" {
		t.Fatalf("stale milestone was not cleared: ok=%v node=%+v err=%v", ok, restored, err)
	}
	if !restored.UpdatedAt.After(feature.UpdatedAt) {
		t.Fatalf("cleared milestone did not advance updatedAt: before=%v after=%v",
			feature.UpdatedAt, restored.UpdatedAt)
	}
}

func TestFeatureCatalogRestoreRejectsCorruptionWithoutSideEffects(t *testing.T) {
	db, store, _, _, _, _ := seedVersionedCatalog(t)
	version, err := store.CreateVersion("p1", "corrupt me", testFeatureEvent())
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.List("p1")
	if err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := db.sql.QueryRow(`
		SELECT snapshot_json FROM feature_catalog_versions WHERE id = ?`, version.ID,
	).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var snapshot map[string]any
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		t.Fatal(err)
	}
	nodes := snapshot["nodes"].([]any)
	nodes = append(nodes, nodes[0])
	snapshot["nodes"] = nodes
	corrupt, _ := json.Marshal(snapshot)
	if _, err := db.sql.Exec(`
		UPDATE feature_catalog_versions
		SET snapshot_json = ?, node_count = node_count + 1 WHERE id = ?`,
		string(corrupt), version.ID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RestoreVersion(
		"p1", version.ID, "corrupt-restore", testFeatureEvent(),
	); !errors.Is(err, ErrFeatureCatalogInvalidSnapshot) {
		t.Fatalf("corrupt restore err=%v, want invalid snapshot", err)
	}
	after, err := store.List("p1")
	if err != nil {
		t.Fatal(err)
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	if string(beforeJSON) != string(afterJSON) {
		t.Fatalf("corrupt restore changed catalog:\nbefore=%s\nafter=%s", beforeJSON, afterJSON)
	}
	var safetyCount, requestCount int
	db.sql.QueryRow(`
		SELECT COUNT(1) FROM feature_catalog_versions
		WHERE project_id = 'p1' AND kind = 'pre_restore'`,
	).Scan(&safetyCount)
	db.sql.QueryRow(`
		SELECT COUNT(1) FROM feature_catalog_restore_requests
		WHERE project_id = 'p1' AND request_id = 'corrupt-restore'`,
	).Scan(&requestCount)
	if safetyCount != 0 || requestCount != 0 {
		t.Fatalf("corrupt restore leaked writes: safety=%d request=%d", safetyCount, requestCount)
	}
}

func TestFeatureCatalogVersionPaginationIsStableWithEqualTimestamps(t *testing.T) {
	db, store, _, _, _, _ := seedVersionedCatalog(t)
	for i := 0; i < 52; i++ {
		if _, err := store.CreateVersion("p1", fmt.Sprintf("v-%02d", i), testFeatureEvent()); err != nil {
			t.Fatal(err)
		}
	}
	sameTime := timeToStr(time.Now().UTC().Add(-24 * time.Hour))
	if _, err := db.sql.Exec(`
		UPDATE feature_catalog_versions SET created_at = ?, updated_at = ?
		WHERE project_id = 'p1'`, sameTime, sameTime,
	); err != nil {
		t.Fatal(err)
	}
	first, err := store.ListVersions("p1", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 50 || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("first page = %+v", first)
	}
	second, err := store.ListVersions("p1", first.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 2 || second.HasMore {
		t.Fatalf("second page = %+v", second)
	}
	seen := map[string]bool{}
	for _, version := range append(first.Items, second.Items...) {
		if seen[version.ID] {
			t.Fatalf("duplicate version %s across pages", version.ID)
		}
		seen[version.ID] = true
	}
	if len(seen) != 52 {
		t.Fatalf("pagination returned %d versions, want 52", len(seen))
	}
}
