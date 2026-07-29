package meta

import (
	"fmt"
	"testing"
	"time"
)

func appendActivityEvent(t *testing.T, store *ProjectEventStore, event ProjectEvent) ProjectEvent {
	t.Helper()
	stored, err := store.Append(event)
	if err != nil {
		t.Fatalf("Append ProjectEvent: %v", err)
	}
	return stored
}

func TestProjectActivityAggregatesByTurnAndFiltersItem(t *testing.T) {
	db := newTestDB(t)
	turn := runningTurnForEvents(t, db)
	events := NewProjectEventStore(db)
	base := time.Now().UTC().Add(-time.Hour)
	taskIDs := []string{"task-1", "task-2", "task-3"}
	for i, taskID := range taskIDs {
		appendActivityEvent(t, events, ProjectEvent{
			ID:         fmt.Sprintf("create-%d", i),
			ProjectID:  "p1",
			TurnID:     turn.ID,
			SessionID:  turn.SessionID,
			ActorKind:  "agent",
			ActorName:  "codex",
			Origin:     "mcp",
			EventType:  "project_item.create",
			TargetType: "project_item",
			TargetID:   taskID,
			Operation:  "create",
			Status:     ProjectEventSucceeded,
			CreatedAt:  base.Add(time.Duration(i) * time.Second),
		})
	}

	activity := NewProjectActivityStore(db)
	page, err := activity.List(ProjectActivityListOptions{ProjectID: "p1", Limit: 10})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("activity page=%+v err=%v", page, err)
	}
	entry := page.Items[0]
	if entry.GroupKind != "turn" || entry.TurnID != turn.ID ||
		entry.Count != 3 || entry.Summary != "创建 3 个 Tasks" {
		t.Fatalf("aggregated entry=%+v", entry)
	}
	if len(entry.EventIDs) != 3 || len(entry.Targets) != 3 {
		t.Fatalf("entry evidence=%+v", entry)
	}
	filtered, err := activity.List(ProjectActivityListOptions{
		ProjectID: "p1", SessionID: turn.SessionID, TurnID: turn.ID,
		Origin: "mcp", Status: ProjectEventSucceeded,
	})
	if err != nil || len(filtered.Items) != 1 || filtered.Items[0].Count != 3 {
		t.Fatalf("combined filters=%+v err=%v", filtered, err)
	}

	itemPage, err := activity.List(ProjectActivityListOptions{
		ProjectID: "p1", TargetType: "project_item", TargetID: "task-2",
	})
	if err != nil || len(itemPage.Items) != 1 || itemPage.Items[0].Count != 1 ||
		itemPage.Items[0].Targets[0].ID != "task-2" {
		t.Fatalf("item activity=%+v err=%v", itemPage, err)
	}
}

func TestProjectActivityCorrelationGroupingAndLiveCursor(t *testing.T) {
	db := newTestDB(t)
	seedTurnProject(t, db, "p1", "s1")
	events := NewProjectEventStore(db)
	base := time.Now().UTC().Add(-time.Hour)
	for group := 1; group <= 3; group++ {
		for item := 1; item <= 2; item++ {
			appendActivityEvent(t, events, ProjectEvent{
				ID:            fmt.Sprintf("g%d-e%d", group, item),
				ProjectID:     "p1",
				CorrelationID: fmt.Sprintf("batch-%d", group),
				ActorKind:     "user",
				ActorName:     "user",
				Origin:        "http",
				EventType:     "project_item.update",
				TargetType:    "project_item",
				TargetID:      fmt.Sprintf("task-%d-%d", group, item),
				Operation:     "update",
				Status:        ProjectEventSucceeded,
				CreatedAt:     base.Add(time.Duration(group) * time.Minute),
			})
		}
	}
	activity := NewProjectActivityStore(db)
	first, err := activity.List(ProjectActivityListOptions{ProjectID: "p1", Limit: 2})
	if err != nil || len(first.Items) != 2 || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("first page=%+v err=%v", first, err)
	}
	if first.Items[0].Count != 2 || first.Items[0].GroupKind != "correlation" {
		t.Fatalf("correlation aggregation=%+v", first.Items[0])
	}

	// A newer insert between pages must not duplicate an entry already returned
	// or displace the remaining older group from the continuation.
	appendActivityEvent(t, events, ProjectEvent{
		ID: "new-live", ProjectID: "p1", ActorKind: "user", Origin: "http",
		EventType: "project_item.create", TargetType: "project_item",
		TargetID: "task-live", Operation: "create", Status: ProjectEventSucceeded,
		CreatedAt: time.Now().UTC(),
	})
	second, err := activity.List(ProjectActivityListOptions{
		ProjectID: "p1", Limit: 2, Cursor: first.NextCursor,
	})
	if err != nil || len(second.Items) != 1 || second.Items[0].ID != "correlation:batch-1" {
		t.Fatalf("second page=%+v err=%v", second, err)
	}
	for _, old := range first.Items {
		if second.Items[0].ID == old.ID {
			t.Fatalf("cursor duplicated entry %s", old.ID)
		}
	}
}

func TestProjectActivityExcludesPureChatLifecycleByDefault(t *testing.T) {
	db := newTestDB(t)
	seedTurnProject(t, db, "p1", "s1")
	turnStore := NewAgentTurnStore(db)
	turn, _, err := turnStore.Create(AgentTurn{
		ProjectID: "p1", SessionID: "s1", ClientRequestID: "chat-only",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := turnStore.Transition(turn.ID, AgentTurnTransition{Status: AgentTurnRunning}); err != nil {
		t.Fatal(err)
	}
	if _, err := turnStore.Transition(turn.ID, AgentTurnTransition{Status: AgentTurnCompleted}); err != nil {
		t.Fatal(err)
	}

	activity := NewProjectActivityStore(db)
	page, err := activity.List(ProjectActivityListOptions{ProjectID: "p1"})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("pure chat leaked into default activity: page=%+v err=%v", page, err)
	}
	lifecycle, err := activity.List(ProjectActivityListOptions{
		ProjectID: "p1", TargetType: "turn", IncludeLifecycle: true,
	})
	if err != nil || len(lifecycle.Items) != 1 || lifecycle.Items[0].TurnID != turn.ID {
		t.Fatalf("explicit lifecycle query=%+v err=%v", lifecycle, err)
	}
}
