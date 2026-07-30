package meta

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func testFeatureEvent() ProjectEvent {
	return ProjectEvent{
		ActorKind: "user",
		ActorName: "user",
		Origin:    "http",
		Status:    ProjectEventSucceeded,
	}
}

func createFeatureNode(t *testing.T, store *FeatureCatalogStore, node FeatureNode) FeatureNode {
	t.Helper()
	created, err := store.Create(node, testFeatureEvent())
	if err != nil {
		t.Fatalf("Create(%q): %v", node.Title, err)
	}
	return created
}

func featureCatalogRig(t *testing.T) (*DB, *FeatureCatalogStore, string, string) {
	t.Helper()
	db := newTestDB(t)
	path1 := t.TempDir()
	path2 := t.TempDir()
	if err := db.EnsureProject("p1", "Project 1", path1); err != nil {
		t.Fatal(err)
	}
	if err := db.EnsureProject("p2", "Project 2", path2); err != nil {
		t.Fatal(err)
	}
	return db, NewFeatureCatalogStore(db), path1, path2
}

func TestFeatureCatalogHierarchyValidationAndOrdering(t *testing.T) {
	_, store, _, _ := featureCatalogRig(t)

	root := createFeatureNode(t, store, FeatureNode{
		ProjectID: "p1", Kind: FeatureNodeModule, Title: "root",
	})
	levels := []FeatureNode{root}
	for depth := 2; depth <= MaxFeatureModuleDepth; depth++ {
		levels = append(levels, createFeatureNode(t, store, FeatureNode{
			ProjectID: "p1", ParentID: levels[len(levels)-1].ID,
			Kind: FeatureNodeModule, Title: fmt.Sprintf("level %d", depth),
		}))
	}
	level2, level3, level9 := levels[1], levels[2], levels[8]
	createFeatureNode(t, store, FeatureNode{
		ProjectID: "p1", ParentID: root.ID, Kind: FeatureNodePoint, Title: "light feature",
	})
	leaf := createFeatureNode(t, store, FeatureNode{
		ProjectID: "p1", ParentID: level9.ID, Kind: FeatureNodePoint, Title: "deep feature",
	})

	_, err := store.Create(FeatureNode{
		ProjectID: "p1", ParentID: level9.ID, Kind: FeatureNodeModule, Title: "level 10",
	}, testFeatureEvent())
	if !errors.Is(err, ErrFeatureMaxDepth) {
		t.Fatalf("tenth module level err = %v, want ErrFeatureMaxDepth", err)
	}
	_, err = store.Create(FeatureNode{
		ProjectID: "p1", Kind: FeatureNodePoint, Title: "root feature",
	}, testFeatureEvent())
	if !errors.Is(err, ErrFeatureInvalidParent) {
		t.Fatalf("root feature err = %v, want ErrFeatureInvalidParent", err)
	}
	_, err = store.Create(FeatureNode{
		ProjectID: "p1", ParentID: leaf.ID, Kind: FeatureNodeModule, Title: "under feature",
	}, testFeatureEvent())
	if !errors.Is(err, ErrFeatureInvalidParent) {
		t.Fatalf("feature parent err = %v, want ErrFeatureInvalidParent", err)
	}

	parent := level2.ID
	if _, err := store.Update("p1", root.ID, FeatureNodePatch{ParentID: &parent}, testFeatureEvent()); !errors.Is(err, ErrFeatureCycle) {
		t.Fatalf("cycle move err = %v, want ErrFeatureCycle", err)
	}
	otherRoot := createFeatureNode(t, store, FeatureNode{
		ProjectID: "p2", Kind: FeatureNodeModule, Title: "other project",
	})
	parent = otherRoot.ID
	if _, err := store.Update("p1", root.ID, FeatureNodePatch{ParentID: &parent}, testFeatureEvent()); !errors.Is(err, ErrProjectMismatch) {
		t.Fatalf("cross-project move err = %v, want ErrProjectMismatch", err)
	}
	first := createFeatureNode(t, store, FeatureNode{
		ProjectID: "p1", Kind: FeatureNodeModule, Title: "insert first", Position: 0,
	})
	catalog, err := store.List("p1")
	if err != nil {
		t.Fatal(err)
	}
	rootPositions := map[string]int{}
	for _, node := range catalog.Nodes {
		if node.ParentID == "" {
			rootPositions[node.ID] = node.Position
		}
	}
	if rootPositions[first.ID] != 0 || rootPositions[root.ID] != 1 {
		t.Fatalf("root positions = %v, want inserted node first", rootPositions)
	}
	sameProjectRoot := createFeatureNode(t, store, FeatureNode{
		ProjectID: "p1", Kind: FeatureNodeModule, Title: "same project root",
	})
	parent = sameProjectRoot.ID
	if _, err := store.Update("p1", root.ID, FeatureNodePatch{ParentID: &parent}, testFeatureEvent()); !errors.Is(err, ErrFeatureMaxDepth) {
		t.Fatalf("deep subtree move err = %v, want ErrFeatureMaxDepth", err)
	}

	position := 0
	newParent := root.ID
	if _, err := store.Update("p1", level3.ID, FeatureNodePatch{
		ParentID: &newParent, Position: &position,
	}, testFeatureEvent()); err != nil {
		t.Fatalf("move level 3: %v", err)
	}
	events, err := NewProjectEventStore(store.db).List(ProjectEventListOptions{
		ProjectID: "p1", TargetType: "feature_node", Limit: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	var sawMove bool
	for _, event := range events.Items {
		if event.EventType == "feature_node.move" {
			sawMove = true
		}
	}
	if !sawMove {
		t.Fatal("expected feature_node.move event")
	}
}

func TestFeatureCatalogDocumentsRoundTripAndNormalize(t *testing.T) {
	_, store, _, _ := featureCatalogRig(t)

	root := createFeatureNode(t, store, FeatureNode{
		ProjectID: "p1", Kind: FeatureNodeModule, Title: "root",
		Documents: []string{" docs/prd.md ", "/tmp/architecture.md", "docs/prd.md", ""},
	})
	if got, want := root.Documents, []string{"docs/prd.md", "/tmp/architecture.md"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("created documents = %v, want %v", got, want)
	}

	documents := []string{"README.md", " docs/design.md ", "README.md"}
	updated, err := store.Update(
		"p1",
		root.ID,
		FeatureNodePatch{Documents: &documents},
		testFeatureEvent(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := updated.Documents, []string{"README.md", "docs/design.md"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("updated documents = %v, want %v", got, want)
	}

	catalog, err := store.List("p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Nodes) != 1 || fmt.Sprint(catalog.Nodes[0].Documents) != fmt.Sprint(updated.Documents) {
		t.Fatalf("listed documents = %+v, want %v", catalog.Nodes, updated.Documents)
	}
}

func TestFeatureCatalogLinksDeletionAndAtomicEvents(t *testing.T) {
	db, store, path1, path2 := featureCatalogRig(t)
	root := createFeatureNode(t, store, FeatureNode{
		ProjectID: "p1", Kind: FeatureNodeModule, Title: "root",
	})
	feature := createFeatureNode(t, store, FeatureNode{
		ProjectID: "p1", ParentID: root.ID, Kind: FeatureNodePoint, Title: "feature",
	})

	now := time.Now().UTC()
	seed := func(path string, items ...ProjectItem) {
		t.Helper()
		if err := NewTaskStore(db).Mutate(path, func(cfg *TasksConfig) bool {
			for i := range items {
				items[i].CreatedAt = now
				items[i].UpdatedAt = now
				items[i].Status = TaskStatusPending
				cfg.Tasks = append(cfg.Tasks, items[i])
			}
			return true
		}); err != nil {
			t.Fatalf("seed project items: %v", err)
		}
	}
	seed(path1,
		ProjectItem{ID: "req", Title: "requirement", Type: ItemTypeRequirement},
		ProjectItem{ID: "bug", Title: "bug", Type: ItemTypeBug},
		ProjectItem{ID: "task", Title: "task", Type: ItemTypeTask},
		ProjectItem{ID: "task-delete", Title: "delete me", Type: ItemTypeTask},
	)
	seed(path2, ProjectItem{ID: "other-task", Title: "other", Type: ItemTypeTask})

	if _, created, err := store.LinkItem("p1", feature.ID, "req", FeatureItemSource, testFeatureEvent()); err != nil || !created {
		t.Fatalf("link source: created=%v err=%v", created, err)
	}
	if _, created, err := store.LinkItem("p1", feature.ID, "task", FeatureItemDelivery, testFeatureEvent()); err != nil || !created {
		t.Fatalf("link delivery: created=%v err=%v", created, err)
	}
	if _, created, err := store.LinkItem("p1", feature.ID, "task", FeatureItemDelivery, testFeatureEvent()); err != nil || created {
		t.Fatalf("duplicate link must be idempotent: created=%v err=%v", created, err)
	}
	if _, _, err := store.LinkItem("p1", feature.ID, "task", FeatureItemSource, testFeatureEvent()); !errors.Is(err, ErrFeatureInvalidItemType) {
		t.Fatalf("task as source err = %v", err)
	}
	if _, _, err := store.LinkItem("p1", feature.ID, "req", FeatureItemDelivery, testFeatureEvent()); !errors.Is(err, ErrFeatureInvalidItemType) {
		t.Fatalf("requirement as delivery err = %v", err)
	}
	if _, _, err := store.LinkItem("p1", feature.ID, "other-task", FeatureItemDelivery, testFeatureEvent()); !errors.Is(err, ErrProjectMismatch) {
		t.Fatalf("cross-project item err = %v", err)
	}
	if _, _, err := store.LinkItem("p1", root.ID, "task", FeatureItemDelivery, testFeatureEvent()); !errors.Is(err, ErrFeatureInvalidKind) {
		t.Fatalf("module link err = %v", err)
	}
	if err := store.Delete("p1", root.ID, testFeatureEvent()); !errors.Is(err, ErrFeatureHasChildren) {
		t.Fatalf("delete module with child err = %v", err)
	}

	feature2 := createFeatureNode(t, store, FeatureNode{
		ProjectID: "p1", ParentID: root.ID, Kind: FeatureNodePoint, Title: "cleanup task link",
	})
	if _, _, err := store.LinkItem("p1", feature2.ID, "task-delete", FeatureItemDelivery, testFeatureEvent()); err != nil {
		t.Fatal(err)
	}
	if err := NewTaskStore(db).Mutate(path1, func(cfg *TasksConfig) bool {
		filtered := cfg.Tasks[:0]
		for _, item := range cfg.Tasks {
			if item.ID != "task-delete" {
				filtered = append(filtered, item)
			}
		}
		cfg.Tasks = filtered
		return true
	}); err != nil {
		t.Fatalf("delete task: %v", err)
	}
	catalog, err := store.List("p1")
	if err != nil {
		t.Fatal(err)
	}
	for _, link := range catalog.Links {
		if link.ItemID == "task-delete" {
			t.Fatal("deleting a task must clean feature_item_links")
		}
	}

	milestone, err := NewTaskStore(db).CreateVersionMilestone(path1, MilestoneBumpMinor, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	target := milestone.ID
	if _, err := store.Update("p1", feature2.ID, FeatureNodePatch{TargetMilestoneID: &target}, testFeatureEvent()); err != nil {
		t.Fatal(err)
	}
	if err := NewTaskStore(db).DeleteMilestone(path1, milestone.ID); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.Get("p1", feature2.ID)
	if err != nil || !ok || got.TargetMilestoneID != "" {
		t.Fatalf("milestone deletion did not clear feature target: ok=%v node=%+v err=%v", ok, got, err)
	}

	if err := store.Delete("p1", feature.ID, testFeatureEvent()); err != nil {
		t.Fatalf("delete feature: %v", err)
	}
	if _, ok, err := NewTaskStore(db).GetTask("task"); err != nil || !ok {
		t.Fatalf("deleting feature must not delete delivery task: ok=%v err=%v", ok, err)
	}
	catalog, _ = store.List("p1")
	for _, link := range catalog.Links {
		if link.FeatureID == feature.ID {
			t.Fatal("deleting a feature must remove its links")
		}
	}

	beforeCount := len(catalog.Nodes)
	_, err = store.Create(FeatureNode{
		ProjectID: "p1", Kind: FeatureNodeModule, Title: "rollback",
	}, ProjectEvent{})
	if !errors.Is(err, ErrInvalidProjectEvent) {
		t.Fatalf("invalid event err = %v, want ErrInvalidProjectEvent", err)
	}
	after, _ := store.List("p1")
	if len(after.Nodes) != beforeCount {
		t.Fatal("node mutation committed even though event append failed")
	}
}

func TestFeatureCatalogBatchReferencesAndAtomicRollback(t *testing.T) {
	db, store, path1, _ := featureCatalogRig(t)
	now := time.Now().UTC()
	if err := NewTaskStore(db).Mutate(path1, func(cfg *TasksConfig) bool {
		cfg.Tasks = append(cfg.Tasks,
			ProjectItem{
				ID: "req", Title: "Requirement", Type: ItemTypeRequirement,
				Status: TaskStatusPending, CreatedAt: now, UpdatedAt: now,
			},
			ProjectItem{
				ID: "task", Title: "Delivery", Type: ItemTypeTask,
				Status: TaskStatusPending, CreatedAt: now, UpdatedAt: now,
			},
		)
		return true
	}); err != nil {
		t.Fatal(err)
	}

	rootTitle, childTitle, featureTitle := "Root", "Child", "Point"
	results, err := store.Batch("p1", []FeatureCatalogBatchOperation{
		{Operation: "create", ClientRef: "root", Kind: FeatureNodeModule, Title: &rootTitle},
		{Operation: "create", ClientRef: "child", ParentRef: "root", Kind: FeatureNodeModule, Title: &childTitle},
		{Operation: "create", ClientRef: "point", ParentRef: "child", Kind: FeatureNodePoint, Title: &featureTitle},
		{Operation: "link", FeatureRef: "point", ItemID: "req", Relation: FeatureItemSource},
		{Operation: "link", FeatureRef: "point", ItemID: "task", Relation: FeatureItemDelivery},
	}, testFeatureEvent())
	if err != nil {
		t.Fatalf("Batch success: %v", err)
	}
	if len(results) != 5 || results[0].Node == nil || results[1].Node == nil ||
		results[2].Node == nil || results[3].Link == nil || results[4].Link == nil {
		t.Fatalf("batch results = %+v", results)
	}
	if results[1].Node.ParentID != results[0].Node.ID ||
		results[2].Node.ParentID != results[1].Node.ID {
		t.Fatalf("clientRef/parentRef hierarchy resolved incorrectly: %+v", results)
	}

	if err := store.UnlinkItem(
		"p1", results[2].Node.ID, "task", FeatureItemDelivery, testFeatureEvent(),
	); err != nil {
		t.Fatalf("first unlink: %v", err)
	}
	if err := store.UnlinkItem(
		"p1", results[2].Node.ID, "task", FeatureItemDelivery, testFeatureEvent(),
	); err != nil {
		t.Fatalf("second unlink must be idempotent: %v", err)
	}

	before, err := store.List("p1")
	if err != nil {
		t.Fatal(err)
	}
	rollbackRoot, rollbackFeature := "Rollback Root", "Rollback Point"
	_, err = store.Batch("p1", []FeatureCatalogBatchOperation{
		{Operation: "create", ClientRef: "rollback-root", Kind: FeatureNodeModule, Title: &rollbackRoot},
		{Operation: "create", ClientRef: "rollback-point", ParentRef: "rollback-root", Kind: FeatureNodePoint, Title: &rollbackFeature},
		// A task cannot be a source. The two preceding node inserts and events
		// must roll back with this final validation failure.
		{Operation: "link", FeatureRef: "rollback-point", ItemID: "task", Relation: FeatureItemSource},
	}, testFeatureEvent())
	if !errors.Is(err, ErrFeatureInvalidItemType) {
		t.Fatalf("batch failure = %v, want ErrFeatureInvalidItemType", err)
	}
	after, err := store.List("p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Nodes) != len(before.Nodes) || len(after.Links) != len(before.Links) {
		t.Fatalf("failed batch was not atomic: before=%+v after=%+v", before, after)
	}
	for _, node := range after.Nodes {
		if node.Title == rollbackRoot || node.Title == rollbackFeature {
			t.Fatalf("failed batch leaked node %+v", node)
		}
	}
}

func TestFeatureTargetVersionCoverageAndExplicitTaskSync(t *testing.T) {
	db, store, path1, path2 := featureCatalogRig(t)
	taskStore := NewTaskStore(db)
	versionA, err := taskStore.CreateVersionMilestone(path1, MilestoneBumpMinor, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	versionB, err := taskStore.CreateVersionMilestone(path1, MilestoneBumpPatch, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := taskStore.CreateMilestone(path1, "legacy phase", "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	otherVersion, err := taskStore.CreateVersionMilestone(path2, MilestoneBumpMajor, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	root := createFeatureNode(t, store, FeatureNode{
		ProjectID: "p1", Kind: FeatureNodeModule, Title: "root",
	})
	first := createFeatureNode(t, store, FeatureNode{
		ProjectID: "p1", ParentID: root.ID, Kind: FeatureNodePoint,
		Title: "first", TargetMilestoneID: versionA.ID,
	})
	createFeatureNode(t, store, FeatureNode{
		ProjectID: "p1", ParentID: root.ID, Kind: FeatureNodePoint,
		Title: "second", TargetMilestoneID: versionA.ID,
	})
	third := createFeatureNode(t, store, FeatureNode{
		ProjectID: "p1", ParentID: root.ID, Kind: FeatureNodePoint,
		Title: "third", TargetMilestoneID: versionB.ID,
	})

	legacyID := legacy.ID
	if _, err := store.Update("p1", first.ID, FeatureNodePatch{
		TargetMilestoneID: &legacyID,
	}, testFeatureEvent()); !errors.Is(err, ErrFeatureInvalidMilestone) {
		t.Fatalf("legacy target err = %v, want ErrFeatureInvalidMilestone", err)
	}
	otherID := otherVersion.ID
	if _, err := store.Update("p1", first.ID, FeatureNodePatch{
		TargetMilestoneID: &otherID,
	}, testFeatureEvent()); !errors.Is(err, ErrProjectMismatch) {
		t.Fatalf("cross-project target err = %v, want ErrProjectMismatch", err)
	}

	now := time.Now().UTC()
	if err := taskStore.Mutate(path1, func(cfg *TasksConfig) bool {
		cfg.Tasks = append(cfg.Tasks,
			ProjectItem{
				ID: "match", Title: "matching", Type: ItemTypeTask,
				Status: TaskStatusPending, Milestone: versionB.Name,
				CreatedAt: now, UpdatedAt: now,
			},
			ProjectItem{
				ID: "different", Title: "different", Type: ItemTypeTask,
				Status: TaskStatusPending, Milestone: versionA.Name,
				CreatedAt: now, UpdatedAt: now,
			},
			ProjectItem{
				ID: "empty", Title: "empty", Type: ItemTypeTask,
				Status:    TaskStatusPending,
				CreatedAt: now, UpdatedAt: now,
			},
		)
		return true
	}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"match", "different", "empty"} {
		if _, _, err := store.LinkItem("p1", third.ID, id, FeatureItemDelivery, testFeatureEvent()); err != nil {
			t.Fatal(err)
		}
	}

	catalog, err := store.List("p1")
	if err != nil {
		t.Fatal(err)
	}
	var coverage []FeatureVersionCoverage
	for _, node := range catalog.Nodes {
		if node.ID == root.ID {
			coverage = node.VersionCoverage
			break
		}
	}
	if len(coverage) != 2 ||
		coverage[0].Version != versionA.Version || coverage[0].FeatureCount != 2 ||
		coverage[1].Version != versionB.Version || coverage[1].FeatureCount != 1 {
		t.Fatalf("module version coverage = %+v", coverage)
	}

	preview, err := store.PreviewMilestoneSync("p1", third.ID)
	if err != nil {
		t.Fatal(err)
	}
	if preview.TargetMilestone != versionB.Name || preview.TargetVersion != versionB.Version ||
		len(preview.Tasks) != 2 {
		t.Fatalf("sync preview = %+v", preview)
	}
	before, _, _ := taskStore.GetTask("different")
	if before.Milestone != versionA.Name {
		t.Fatalf("preview silently changed task milestone to %q", before.Milestone)
	}

	synced, err := store.SyncMilestone("p1", third.ID, testFeatureEvent())
	if err != nil {
		t.Fatal(err)
	}
	if len(synced.Tasks) != 2 {
		t.Fatalf("synced tasks = %+v", synced.Tasks)
	}
	for _, id := range []string{"match", "different", "empty"} {
		task, ok, err := taskStore.GetTask(id)
		if err != nil || !ok || task.Milestone != versionB.Name {
			t.Fatalf("task %s milestone = %q ok=%v err=%v", id, task.Milestone, ok, err)
		}
	}
}

func TestFeatureCatalogDerivesTraceabilityProgressAndCoverage(t *testing.T) {
	db, store, path1, _ := featureCatalogRig(t)
	root := createFeatureNode(t, store, FeatureNode{
		ProjectID: "p1", Kind: FeatureNodeModule, Title: "root",
	})
	unplanned := createFeatureNode(t, store, FeatureNode{
		ProjectID: "p1", ParentID: root.ID, Kind: FeatureNodePoint, Title: "unplanned",
	})
	zero := createFeatureNode(t, store, FeatureNode{
		ProjectID: "p1", ParentID: root.ID, Kind: FeatureNodePoint, Title: "zero percent",
	})
	inProgress := createFeatureNode(t, store, FeatureNode{
		ProjectID: "p1", ParentID: root.ID, Kind: FeatureNodePoint, Title: "in progress",
	})
	delivered := createFeatureNode(t, store, FeatureNode{
		ProjectID: "p1", ParentID: root.ID, Kind: FeatureNodePoint, Title: "delivered",
	})
	replan := createFeatureNode(t, store, FeatureNode{
		ProjectID: "p1", ParentID: root.ID, Kind: FeatureNodePoint, Title: "replan",
	})

	now := time.Now().UTC()
	items := []ProjectItem{
		{ID: "requirement", Title: "requirement", Type: ItemTypeRequirement, Status: TaskStatusPending},
		{ID: "pending", Title: "pending", Type: ItemTypeTask, Status: TaskStatusPending},
		{ID: "running", Title: "running", Type: ItemTypeTask, Status: TaskStatusRunning},
		{ID: "completed-a", Title: "completed a", Type: ItemTypeTask, Status: TaskStatusCompleted},
		{ID: "completed-b", Title: "completed b", Type: ItemTypeTask, Status: TaskStatusCompleted},
		{ID: "cancelled", Title: "cancelled", Type: ItemTypeTask, Status: TaskStatusCancelled},
	}
	if err := NewTaskStore(db).Mutate(path1, func(cfg *TasksConfig) bool {
		for i := range items {
			items[i].CreatedAt = now
			items[i].UpdatedAt = now
			cfg.Tasks = append(cfg.Tasks, items[i])
		}
		return true
	}); err != nil {
		t.Fatal(err)
	}

	link := func(featureID, itemID string, relation FeatureItemRelation) {
		t.Helper()
		if _, _, err := store.LinkItem("p1", featureID, itemID, relation, testFeatureEvent()); err != nil {
			t.Fatalf("LinkItem(%s, %s, %s): %v", featureID, itemID, relation, err)
		}
	}
	// Both directions are many-to-many: one requirement and one delivery task
	// may each trace to several feature points.
	link(zero.ID, "requirement", FeatureItemSource)
	link(inProgress.ID, "requirement", FeatureItemSource)
	link(zero.ID, "pending", FeatureItemDelivery)
	link(inProgress.ID, "running", FeatureItemDelivery)
	link(inProgress.ID, "completed-a", FeatureItemDelivery)
	link(inProgress.ID, "cancelled", FeatureItemDelivery)
	link(delivered.ID, "completed-b", FeatureItemDelivery)
	link(delivered.ID, "completed-a", FeatureItemDelivery)
	link(replan.ID, "cancelled", FeatureItemDelivery)

	catalog, err := store.List("p1")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]*FeatureProgress{}
	for _, node := range catalog.Nodes {
		byID[node.ID] = node.Progress
	}
	assertProgress := func(node FeatureNode, status FeatureProgressStatus, percent *int) {
		t.Helper()
		got := byID[node.ID]
		if got == nil || got.Status != status {
			t.Fatalf("%s progress = %+v, want status %s", node.Title, got, status)
		}
		if percent == nil {
			if got.ProgressPercent != nil {
				t.Fatalf("%s percent = %v, want nil", node.Title, *got.ProgressPercent)
			}
		} else if got.ProgressPercent == nil || *got.ProgressPercent != *percent {
			t.Fatalf("%s percent = %v, want %d", node.Title, got.ProgressPercent, *percent)
		}
	}
	zeroPercent, fiftyPercent, hundredPercent := 0, 50, 100
	assertProgress(unplanned, FeatureProgressUnplanned, nil)
	assertProgress(zero, FeatureProgressPending, &zeroPercent)
	assertProgress(inProgress, FeatureProgressInProgress, &fiftyPercent)
	assertProgress(delivered, FeatureProgressDelivered, &hundredPercent)
	assertProgress(replan, FeatureProgressReplan, nil)

	module := byID[root.ID]
	if module == nil {
		t.Fatal("root module has no derived progress")
	}
	if module.Status != FeatureProgressInProgress || module.ProgressPercent == nil || *module.ProgressPercent != 50 {
		t.Fatalf("module progress = %+v, want in_progress at 50%%", module)
	}
	if module.CoveredFeatures != 3 || module.TotalFeatures != 5 ||
		module.UnplannedFeatures != 1 || module.ReplanFeatures != 1 {
		t.Fatalf("module coverage = %+v, want covered=3 total=5 unplanned=1 replan=1", module)
	}
	if module.CompletedTasks != 2 || module.TotalTasks != 4 {
		t.Fatalf("module task denominator = %+v, cancelled and duplicate tasks must be excluded", module)
	}
}
