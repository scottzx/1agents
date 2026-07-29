package meta

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"
)

func runningTurnForEvents(t *testing.T, db *DB) AgentTurn {
	t.Helper()
	seedTurnProject(t, db, "p1", "s1")
	store := NewAgentTurnStore(db)
	turn, _, err := store.Create(AgentTurn{
		ProjectID: "p1", SessionID: "s1", ClientRequestID: "event-turn",
	})
	if err != nil {
		t.Fatal(err)
	}
	turn, err = store.Transition(turn.ID, AgentTurnTransition{Status: AgentTurnRunning})
	if err != nil {
		t.Fatal(err)
	}
	return turn
}

func TestProjectEventRegistrySequenceAndTerminalGuard(t *testing.T) {
	db := newTestDB(t)
	turn := runningTurnForEvents(t, db)
	store := NewProjectEventStore(db)

	for i := 0; i < 2; i++ {
		event, err := store.Append(ProjectEvent{
			ProjectID:  "p1",
			TurnID:     turn.ID,
			SessionID:  "s1",
			ActorKind:  "agent",
			ActorName:  "codex",
			Origin:     "mcp",
			EventType:  "project_item.create",
			TargetType: "project_item",
			TargetID:   fmt.Sprintf("task-%d", i),
			Operation:  "create",
			After:      json.RawMessage(`{"status":"pending"}`),
			Status:     ProjectEventSucceeded,
		})
		if err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
		// queue/start are sequence 0/1; domain events continue at 2/3.
		if event.Sequence != int64(i+2) {
			t.Fatalf("sequence=%d, want %d", event.Sequence, i+2)
		}
	}
	if _, err := store.Append(ProjectEvent{
		ProjectID: "p1", TurnID: turn.ID, ActorKind: "agent", Origin: "mcp",
		EventType: "project_item.bind", TargetType: "project_item",
		TargetID: "task-x", Operation: "bind", Status: ProjectEventSucceeded,
	}); !errors.Is(err, ErrInvalidProjectEvent) {
		t.Fatalf("unregistered event err=%v", err)
	}

	if _, err := NewAgentTurnStore(db).Transition(
		turn.ID, AgentTurnTransition{Status: AgentTurnCompleted},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append(ProjectEvent{
		ProjectID: "p1", TurnID: turn.ID, ActorKind: "agent", Origin: "mcp",
		EventType: "project_item.update", TargetType: "project_item",
		TargetID: "task-0", Operation: "update", Status: ProjectEventSucceeded,
	}); !errors.Is(err, ErrTurnNotRunning) {
		t.Fatalf("terminal Turn append err=%v, want ErrTurnNotRunning", err)
	}
}

func TestProjectEventTransactionCommitsOrRollsBackAtomically(t *testing.T) {
	db := newTestDB(t)
	turn := runningTurnForEvents(t, db)
	store := NewProjectEventStore(db)
	now := timeToStr(time.Now().UTC())
	if _, err := db.sql.Exec(`
		INSERT INTO project_items (
			id, project_id, title, description, issue_state, status,
			schedule_type, created_at, updated_at
		) VALUES ('task-1', 'p1', 'before', '', 'open', 'pending',
		          'immediate', ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}

	stop := errors.New("force rollback")
	err := store.WithTransaction(func(tx *ProjectMutationTx) error {
		if _, err := tx.Exec(`UPDATE project_items SET title = 'rolled back' WHERE id = 'task-1'`); err != nil {
			return err
		}
		if _, err := tx.AppendEvent(ProjectEvent{
			ProjectID: "p1", TurnID: turn.ID, SessionID: "s1",
			ActorKind: "agent", Origin: "mcp",
			EventType: "project_item.update", TargetType: "project_item",
			TargetID: "task-1", Operation: "update",
			Before: json.RawMessage(`{"title":"before"}`),
			After:  json.RawMessage(`{"title":"rolled back"}`),
			Status: ProjectEventSucceeded,
		}); err != nil {
			return err
		}
		return stop
	})
	if !errors.Is(err, stop) {
		t.Fatalf("rollback err=%v", err)
	}
	var title string
	if err := db.sql.QueryRow(`SELECT title FROM project_items WHERE id = 'task-1'`).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != "before" {
		t.Fatalf("title=%q after rollback", title)
	}
	var rolledBackEvents int
	if err := db.sql.QueryRow(
		`SELECT COUNT(1) FROM project_events WHERE target_id = 'task-1'`,
	).Scan(&rolledBackEvents); err != nil {
		t.Fatal(err)
	}
	if rolledBackEvents != 0 {
		t.Fatalf("rolled-back events=%d, want 0", rolledBackEvents)
	}

	if err := store.WithTransaction(func(tx *ProjectMutationTx) error {
		if _, err := tx.Exec(`UPDATE project_items SET title = 'after' WHERE id = 'task-1'`); err != nil {
			return err
		}
		_, err := tx.AppendEvent(ProjectEvent{
			ProjectID: "p1", TurnID: turn.ID, SessionID: "s1",
			ActorKind: "agent", Origin: "mcp",
			EventType: "project_item.update", TargetType: "project_item",
			TargetID: "task-1", Operation: "update",
			Before: json.RawMessage(`{"title":"before"}`),
			After:  json.RawMessage(`{"title":"after"}`),
			Status: ProjectEventSucceeded,
		})
		return err
	}); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := db.sql.QueryRow(`SELECT title FROM project_items WHERE id = 'task-1'`).Scan(&title); err != nil || title != "after" {
		t.Fatalf("committed title=%q err=%v", title, err)
	}
}

func TestProjectEventCorrelationAndPaginationFilters(t *testing.T) {
	db := newTestDB(t)
	seedTurnProject(t, db, "p1", "s1")
	store := NewProjectEventStore(db)
	base := time.Now().UTC().Add(-time.Hour)
	for i := 0; i < 5; i++ {
		status := ProjectEventSucceeded
		if i == 4 {
			status = ProjectEventRejected
		}
		event, err := store.Append(ProjectEvent{
			ID:            fmt.Sprintf("event-%d", i),
			ProjectID:     "p1",
			CorrelationID: "batch-1",
			ActorKind:     "user",
			Origin:        "cli",
			EventType:     "project_item.update",
			TargetType:    "project_item",
			TargetID:      fmt.Sprintf("task-%d", i),
			Operation:     "update",
			Status:        status,
			CreatedAt:     base.Add(time.Duration(i) * time.Minute),
		})
		if err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
		if event.Sequence != int64(i) {
			t.Fatalf("event %d sequence=%d", i, event.Sequence)
		}
	}

	first, err := store.List(ProjectEventListOptions{
		ProjectID: "p1", Origin: "cli", TargetType: "project_item", Limit: 2,
	})
	if err != nil || len(first.Items) != 2 || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("first page=%+v err=%v", first, err)
	}
	second, err := store.List(ProjectEventListOptions{
		ProjectID: "p1", Origin: "cli", TargetType: "project_item",
		Limit: 2, Cursor: first.NextCursor,
	})
	if err != nil || len(second.Items) != 2 || !second.HasMore {
		t.Fatalf("second page=%+v err=%v", second, err)
	}
	third, err := store.List(ProjectEventListOptions{
		ProjectID: "p1", Origin: "cli", TargetType: "project_item",
		Limit: 2, Cursor: second.NextCursor,
	})
	if err != nil || len(third.Items) != 1 || third.HasMore {
		t.Fatalf("third page=%+v err=%v", third, err)
	}
	rejected, err := store.List(ProjectEventListOptions{
		ProjectID: "p1", Status: ProjectEventRejected,
	})
	if err != nil || len(rejected.Items) != 1 || rejected.Items[0].ID != "event-4" {
		t.Fatalf("rejected=%+v err=%v", rejected, err)
	}
}

func TestProjectEventRejectsCrossProjectTurnAndInvalidSnapshots(t *testing.T) {
	db := newTestDB(t)
	turn := runningTurnForEvents(t, db)
	if err := db.EnsureProject("p2", "p2", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	store := NewProjectEventStore(db)
	if _, err := store.Append(ProjectEvent{
		ProjectID: "p2", TurnID: turn.ID, ActorKind: "agent", Origin: "mcp",
		EventType: "project_item.update", TargetType: "project_item",
		TargetID: "task-1", Operation: "update", Status: ProjectEventSucceeded,
	}); !errors.Is(err, ErrProjectMismatch) {
		t.Fatalf("cross-project err=%v, want ErrProjectMismatch", err)
	}
	if _, err := store.Append(ProjectEvent{
		ProjectID: "p1", TurnID: turn.ID, ActorKind: "agent", Origin: "mcp",
		EventType: "project_item.update", TargetType: "project_item",
		TargetID: "task-1", Operation: "update", Status: ProjectEventSucceeded,
		Before: json.RawMessage(`not-json`),
	}); !errors.Is(err, ErrInvalidProjectEvent) {
		t.Fatalf("invalid snapshot err=%v, want ErrInvalidProjectEvent", err)
	}
}

func TestTaskMutationRollsBackWhenEventAppendFails(t *testing.T) {
	db := newTestDB(t)
	ws := t.TempDir()
	if err := db.EnsureProject("p1", "p1", ws); err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	store := NewTaskStore(db)
	now := time.Now().UTC()
	events := []ProjectEvent{{
		ProjectID:  "p1",
		ActorKind:  "user",
		ActorName:  "user",
		Origin:     "cli",
		EventType:  "project_item.not_registered",
		TargetType: "project_item",
		TargetID:   "task-rollback",
		Operation:  "not_registered",
		Status:     ProjectEventSucceeded,
	}}
	_, err := store.MutateWithEvents(ws, events, func(cfg *TasksConfig) bool {
		cfg.Tasks = append(cfg.Tasks, ProjectItem{
			ID:        "task-rollback",
			Title:     "must roll back",
			Status:    TaskStatusPending,
			CreatedAt: now,
			UpdatedAt: now,
		})
		return true
	})
	if !errors.Is(err, ErrInvalidProjectEvent) {
		t.Fatalf("MutateWithEvents err=%v, want ErrInvalidProjectEvent", err)
	}
	if _, ok, getErr := store.GetTask("task-rollback"); getErr != nil || ok {
		t.Fatalf("task committed despite Event failure: ok=%v err=%v", ok, getErr)
	}
}
