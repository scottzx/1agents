package meta

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

func TestFeatureCatalogGanttAndExportContract(t *testing.T) {
	db, store, path, _ := featureCatalogRig(t)
	taskStore := NewTaskStore(db)

	targetDate := time.Date(2026, time.August, 31, 0, 0, 0, 0, time.UTC)
	version, err := taskStore.CreateVersionMilestone(
		path,
		MilestoneBumpMinor,
		"Feature catalog release",
		&targetDate,
	)
	if err != nil {
		t.Fatal(err)
	}

	root := createFeatureNode(t, store, FeatureNode{
		ProjectID: "p1", Kind: FeatureNodeModule, Title: "Root",
	})
	level2 := createFeatureNode(t, store, FeatureNode{
		ProjectID: "p1", ParentID: root.ID, Kind: FeatureNodeModule, Title: "Level 2",
	})
	level3 := createFeatureNode(t, store, FeatureNode{
		ProjectID: "p1", ParentID: level2.ID, Kind: FeatureNodeModule, Title: "Level 3",
	})
	feature := createFeatureNode(t, store, FeatureNode{
		ProjectID: "p1", ParentID: level3.ID, Kind: FeatureNodePoint,
		Title: "Deep feature", TargetMilestoneID: version.ID,
	})

	startA := time.Date(2026, time.August, 4, 0, 0, 0, 0, time.UTC)
	endA := time.Date(2026, time.August, 8, 0, 0, 0, 0, time.UTC)
	startB := time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
	endB := time.Date(2026, time.August, 20, 0, 0, 0, 0, time.UTC)
	now := time.Now().UTC()
	if err := taskStore.Mutate(path, func(cfg *TasksConfig) bool {
		cfg.Tasks = append(cfg.Tasks,
			ProjectItem{
				ID: "requirement", Title: "Source requirement", Type: ItemTypeRequirement,
				IssueState: IssueOpen, Status: TaskStatusPending,
				CreatedAt: now, UpdatedAt: now,
			},
			ProjectItem{
				ID: "task-a", Title: "Completed delivery", Type: ItemTypeTask,
				IssueState: IssueOpen, Status: TaskStatusCompleted,
				PlannedStart: &startA, PlannedEnd: &endA, Milestone: version.Name,
				CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second),
			},
			ProjectItem{
				ID: "task-b", Title: "Dependent delivery", Type: ItemTypeTask,
				IssueState: IssueOpen, Status: TaskStatusPending,
				PlannedStart: &startB, PlannedEnd: &endB, DependsOn: []string{"task-a"},
				Milestone: version.Name,
				CreatedAt: now.Add(2 * time.Second), UpdatedAt: now.Add(2 * time.Second),
			},
			ProjectItem{
				ID: "task-unscheduled", Title: "Unscheduled delivery", Type: ItemTypeTask,
				IssueState: IssueOpen, Status: TaskStatusPending, Milestone: version.Name,
				CreatedAt: now.Add(3 * time.Second), UpdatedAt: now.Add(3 * time.Second),
			},
		)
		return true
	}); err != nil {
		t.Fatal(err)
	}

	if _, _, err := store.LinkItem(
		"p1", feature.ID, "requirement", FeatureItemSource, testFeatureEvent(),
	); err != nil {
		t.Fatal(err)
	}
	for _, taskID := range []string{"task-a", "task-b", "task-unscheduled"} {
		if _, _, err := store.LinkItem(
			"p1", feature.ID, taskID, FeatureItemDelivery, testFeatureEvent(),
		); err != nil {
			t.Fatal(err)
		}
	}

	gantt, err := store.GanttView("p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(gantt.Modules) != 1 ||
		len(gantt.Modules[0].Children) != 1 ||
		len(gantt.Modules[0].Children[0].Children) != 1 {
		t.Fatalf("three-level module tree was truncated: %+v", gantt.Modules)
	}
	deepest := gantt.Modules[0].Children[0].Children[0]
	if deepest.Depth != 2 || strings.Join(deepest.Path, "/") != "Root/Level 2" {
		t.Fatalf("deep module path/depth = %v/%d", deepest.Path, deepest.Depth)
	}
	if len(deepest.Tasks) != 3 {
		t.Fatalf("deep module tasks = %d, want 3", len(deepest.Tasks))
	}
	if !ganttTimeEqual(gantt.Modules[0].AggStart, startA) ||
		!ganttTimeEqual(gantt.Modules[0].AggEnd, endB) {
		t.Fatalf(
			"root aggregate = %v..%v, want %v..%v",
			gantt.Modules[0].AggStart,
			gantt.Modules[0].AggEnd,
			startA,
			endB,
		)
	}
	if math.Abs(gantt.Modules[0].Progress-(100.0/3.0)) > 0.001 {
		t.Fatalf("root progress = %v, want 33.33 percent", gantt.Modules[0].Progress)
	}
	completed := ganttTaskByID(t, deepest.Tasks, "task-a")
	if completed.Progress != 100 {
		t.Fatalf("completed task progress = %v, want 100", completed.Progress)
	}
	dependent := ganttTaskByID(t, deepest.Tasks, "task-b")
	if len(dependent.DependsOn) != 1 || dependent.DependsOn[0] != "task-a" {
		t.Fatalf("dependency contract = %v", dependent.DependsOn)
	}
	if len(gantt.Unscheduled) != 1 || gantt.Unscheduled[0].ID != "task-unscheduled" {
		t.Fatalf("unscheduled tasks = %+v", gantt.Unscheduled)
	}
	if len(gantt.Milestones) != 1 || gantt.Milestones[0].Version != version.Version {
		t.Fatalf("milestones = %+v", gantt.Milestones)
	}
	if gantt.Milestones[0].Name != version.Name {
		t.Fatalf("milestone name = %q, want %q", gantt.Milestones[0].Name, version.Name)
	}

	earlier := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	later := time.Date(2026, time.August, 25, 0, 0, 0, 0, time.UTC)
	if err := taskStore.Mutate(path, func(cfg *TasksConfig) bool {
		for i := range cfg.Tasks {
			if cfg.Tasks[i].ID == "task-b" {
				cfg.Tasks[i].PlannedStart = &earlier
				cfg.Tasks[i].PlannedEnd = &later
				cfg.Tasks[i].UpdatedAt = time.Now().UTC()
				return true
			}
		}
		return false
	}); err != nil {
		t.Fatal(err)
	}
	updated, err := store.GanttView("p1")
	if err != nil {
		t.Fatal(err)
	}
	if !ganttTimeEqual(updated.Modules[0].AggStart, earlier) ||
		!ganttTimeEqual(updated.Modules[0].AggEnd, later) {
		t.Fatalf(
			"task date update did not refresh aggregate: %v..%v",
			updated.Modules[0].AggStart,
			updated.Modules[0].AggEnd,
		)
	}

	markdown, err := store.ExportMarkdown("p1")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"## Root",
		"### Level 2",
		"#### Level 3",
		"**Deep feature**",
		"Sources: #1 Source requirement",
		"Delivery Tasks:",
		"Target Version: " + version.Version,
		"Status:",
	} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("markdown export missing %q:\n%s", want, markdown)
		}
	}

	raw, err := store.ExportJSON("p1")
	if err != nil {
		t.Fatal(err)
	}
	var exported FeatureCatalogExportData
	if err := json.Unmarshal(raw, &exported); err != nil {
		t.Fatalf("JSON export cannot be consumed: %v", err)
	}
	if exported.SchemaVersion != 1 {
		t.Fatalf("schemaVersion = %d, want 1", exported.SchemaVersion)
	}
	if len(exported.Catalog.Nodes) != 4 || len(exported.Catalog.Links) != 4 {
		t.Fatalf("catalog traceability missing: %+v", exported.Catalog)
	}
	if len(exported.Items) != 4 || len(exported.Gantt.Modules) != 1 {
		t.Fatalf("self-contained JSON export incomplete: %+v", exported)
	}

	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"schemaVersion", "catalog", "items", "gantt"} {
		if _, ok := wire[key]; !ok {
			t.Fatalf("stable JSON contract missing top-level %q", key)
		}
	}
	ganttWire := wire["gantt"].(map[string]any)
	moduleWire := ganttWire["modules"].([]any)[0].(map[string]any)
	if _, persisted := moduleWire["plannedStart"]; persisted {
		t.Fatal("module must not expose a second plannedStart field")
	}
	if _, persisted := moduleWire["plannedEnd"]; persisted {
		t.Fatal("module must not expose a second plannedEnd field")
	}
}

func ganttTaskByID(t *testing.T, tasks []GanttTaskEntry, id string) GanttTaskEntry {
	t.Helper()
	for _, task := range tasks {
		if task.ID == id {
			return task
		}
	}
	t.Fatalf("gantt task %q not found in %+v", id, tasks)
	return GanttTaskEntry{}
}

func ganttTimeEqual(got *time.Time, want time.Time) bool {
	return got != nil && got.Equal(want)
}
