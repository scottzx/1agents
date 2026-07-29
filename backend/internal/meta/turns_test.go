package meta

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func seedTurnProject(t *testing.T, db *DB, projectID, sessionID string) {
	t.Helper()
	if err := db.EnsureProject(projectID, projectID, t.TempDir()); err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := NewSessionStore(db).Add(ChatSessionRecord{
		ID:          sessionID,
		WorkspaceID: projectID,
		Name:        "Turn test",
		AgentType:   "codex",
	}); err != nil {
		t.Fatalf("Add session: %v", err)
	}
}

func TestTurnModelSchemaAndLegacyReconcile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	seedTurnProject(t, db, "p1", "s1")
	if _, err := db.sql.Exec(`
		INSERT INTO project_items (
			id, project_id, title, description, issue_state, status,
			schedule_type, created_at, updated_at
		) VALUES ('task-1', 'p1', 'legacy', '', 'open', 'pending',
		          'immediate', '2026-07-01T00:00:00Z', '2026-07-01T00:00:00Z');
		INSERT INTO replies (
			id, task_id, seq, author_kind, author_name, agent_type, text,
			session_ref, turn_id, acp_session_id, in_reply_to, mode, created_at
		) VALUES ('reply-1', 'task-1', 0, 'user', 'user', '', 'legacy prompt',
		          's1', NULL, '', '', 'follow_up', '2026-07-01T00:00:00Z');
		DROP INDEX idx_replies_turn;
		ALTER TABLE replies RENAME TO replies_with_turn;
		CREATE TABLE replies (
			id             TEXT PRIMARY KEY,
			task_id        TEXT NOT NULL,
			seq            INTEGER NOT NULL DEFAULT 0,
			author_kind    TEXT NOT NULL DEFAULT '',
			author_name    TEXT NOT NULL DEFAULT '',
			agent_type     TEXT NOT NULL DEFAULT '',
			text           TEXT NOT NULL DEFAULT '',
			session_ref    TEXT NOT NULL DEFAULT '',
			acp_session_id TEXT NOT NULL DEFAULT '',
			in_reply_to    TEXT NOT NULL DEFAULT '',
			mode           TEXT NOT NULL DEFAULT 'pure_comment',
			created_at     TEXT NOT NULL
		);
		INSERT INTO replies (
			id, task_id, seq, author_kind, author_name, agent_type, text,
			session_ref, acp_session_id, in_reply_to, mode, created_at
		) SELECT id, task_id, seq, author_kind, author_name, agent_type, text,
		         session_ref, acp_session_id, in_reply_to, mode, created_at
		    FROM replies_with_turn;
		DROP TABLE replies_with_turn;
		DROP TABLE project_events;
		DROP TABLE agent_turns;
		PRAGMA user_version = 25;
	`); err != nil {
		db.Close()
		t.Fatalf("prepare legacy db: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close legacy db: %v", err)
	}

	db, err = Open(path)
	if err != nil {
		t.Fatalf("reopen/reconcile: %v", err)
	}
	defer db.Close()

	for _, table := range []string{"agent_turns", "project_events"} {
		exists, err := db.tableExists(table)
		if err != nil || !exists {
			t.Fatalf("%s exists=%v err=%v", table, exists, err)
		}
	}
	turnCols, err := db.tableColumns("agent_turns")
	if err != nil {
		t.Fatalf("agent_turns columns: %v", err)
	}
	for _, column := range []string{
		"request_fingerprint", "runtime_record_id", "runtime_request_id",
		"prompt_message_id", "final_reply_id", "stop_reason",
		"terminal_source", "last_event_seq",
	} {
		if !turnCols[column] {
			t.Errorf("agent_turns.%s missing", column)
		}
	}
	for _, index := range []string{
		"idx_agent_turns_session",
		"idx_agent_turns_project",
		"idx_project_events_project",
		"idx_project_events_session",
		"idx_project_events_turn",
		"idx_project_events_target",
	} {
		var count int
		if err := db.sql.QueryRow(
			`SELECT COUNT(1) FROM sqlite_master WHERE type = 'index' AND name = ?`,
			index,
		).Scan(&count); err != nil || count != 1 {
			t.Fatalf("index %s count=%d err=%v", index, count, err)
		}
	}
	replyCols, err := db.tableColumns("replies")
	if err != nil || !replyCols["turn_id"] {
		t.Fatalf("replies.turn_id missing: cols=%v err=%v", replyCols, err)
	}
	var text string
	var turnID sql.NullString
	if err := db.sql.QueryRow(
		`SELECT text, turn_id FROM replies WHERE id = 'reply-1'`,
	).Scan(&text, &turnID); err != nil {
		t.Fatalf("legacy reply missing: %v", err)
	}
	if text != "legacy prompt" || turnID.Valid {
		t.Fatalf("legacy reply changed: text=%q turn=%v", text, turnID)
	}
	var version int
	if err := db.sql.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != schemaVersion {
		t.Fatalf("user_version=%d err=%v, want %d", version, err, schemaVersion)
	}
}

func TestAgentTurnCreateIdempotentAndLifecycle(t *testing.T) {
	db := newTestDB(t)
	seedTurnProject(t, db, "p1", "s1")
	store := NewAgentTurnStore(db)

	turn, created, err := store.Create(AgentTurn{
		ProjectID:       "p1",
		SessionID:       "s1",
		ClientRequestID: "request-1",
		AgentType:       "codex",
		PromptText:      "create three tasks",
	})
	if err != nil || !created {
		t.Fatalf("Create: created=%v err=%v", created, err)
	}
	if turn.Status != AgentTurnQueued || turn.ID == "" {
		t.Fatalf("queued turn = %+v", turn)
	}
	if err := store.SetReplyLinks(turn.ID, "reply-user", ""); err != nil {
		t.Fatalf("Set initiating reply: %v", err)
	}
	same, created, err := store.Create(AgentTurn{
		ProjectID:       "p1",
		SessionID:       "s1",
		ClientRequestID: "request-1",
		PromptText:      "create three tasks",
	})
	if err != nil || created || same.ID != turn.ID || same.PromptText != turn.PromptText {
		t.Fatalf("idempotent Create: same=%+v created=%v err=%v", same, created, err)
	}
	if _, _, err := store.Create(AgentTurn{
		ProjectID:       "p1",
		SessionID:       "s1",
		ClientRequestID: "request-1",
		PromptText:      "must not replace original",
	}); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting idempotent Create err=%v, want ErrIdempotencyConflict", err)
	}

	running, err := store.Transition(turn.ID, AgentTurnTransition{Status: AgentTurnRunning})
	if err != nil || running.StartedAt == nil {
		t.Fatalf("start: turn=%+v err=%v", running, err)
	}
	done, err := store.Transition(turn.ID, AgentTurnTransition{
		Status:           AgentTurnCompleted,
		FinalAnswer:      "created",
		RuntimeRecordID:  "s1",
		RuntimeRequestID: turn.ID,
		PromptMessageID:  turn.ID,
		StopReason:       "end_turn",
		TerminalSource:   "live_runtime",
		LastEventSeq:     42,
	})
	if err == nil {
		err = store.SetReplyLinks(turn.ID, "", "reply-agent")
	}
	if err == nil {
		done, _, err = store.Get(turn.ID)
	}
	if err != nil || done.CompletedAt == nil || done.FinalAnswer != "created" ||
		done.RuntimeRequestID != turn.ID || done.PromptMessageID != turn.ID ||
		done.StopReason != "end_turn" || done.TerminalSource != "live_runtime" ||
		done.LastEventSeq != 42 || done.InitiatingReplyID != "reply-user" ||
		done.FinalReplyID != "reply-agent" {
		t.Fatalf("complete: turn=%+v err=%v", done, err)
	}
	if _, err := store.Transition(turn.ID, AgentTurnTransition{Status: AgentTurnFailed}); !errors.Is(err, ErrInvalidTurnTransition) {
		t.Fatalf("terminal transition err=%v, want ErrInvalidTurnTransition", err)
	}

	events, err := NewProjectEventStore(db).List(ProjectEventListOptions{
		ProjectID: "p1",
		TurnID:    turn.ID,
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("List events: %v", err)
	}
	if len(events.Items) != 3 {
		t.Fatalf("lifecycle events=%d, want 3", len(events.Items))
	}
	got := map[string]bool{}
	for _, event := range events.Items {
		got[event.EventType] = true
	}
	for _, want := range []string{"turn.queue", "turn.start", "turn.complete"} {
		if !got[want] {
			t.Errorf("missing lifecycle event %s: %+v", want, events.Items)
		}
	}
}

func TestRecoverInterruptedTurns(t *testing.T) {
	db := newTestDB(t)
	seedTurnProject(t, db, "p1", "s1")
	store := NewAgentTurnStore(db)

	running, _, err := store.Create(AgentTurn{
		ProjectID: "p1", SessionID: "s1", ClientRequestID: "running",
	})
	if err != nil {
		t.Fatalf("Create running: %v", err)
	}
	if _, err := store.Transition(running.ID, AgentTurnTransition{Status: AgentTurnRunning}); err != nil {
		t.Fatalf("Transition running: %v", err)
	}
	queued, _, err := store.Create(AgentTurn{
		ProjectID: "p1", SessionID: "s1", ClientRequestID: "queued",
	})
	if err != nil {
		t.Fatalf("Create queued: %v", err)
	}

	failed, cancelled, err := store.RecoverInterrupted("backend_restarted", "restart")
	if err != nil || failed != 1 || cancelled != 1 {
		t.Fatalf("RecoverInterrupted: failed=%d cancelled=%d err=%v", failed, cancelled, err)
	}
	gotRunning, _, _ := store.Get(running.ID)
	gotQueued, _, _ := store.Get(queued.ID)
	if gotRunning.Status != AgentTurnFailed || gotRunning.ErrorCode != "backend_restarted" {
		t.Fatalf("running recovery = %+v", gotRunning)
	}
	if gotQueued.Status != AgentTurnCancelled || gotQueued.ErrorCode != "backend_restarted" {
		t.Fatalf("queued recovery = %+v", gotQueued)
	}

	failed, cancelled, err = store.RecoverInterrupted("backend_restarted", "restart")
	if err != nil || failed != 0 || cancelled != 0 {
		t.Fatalf("idempotent recovery: failed=%d cancelled=%d err=%v", failed, cancelled, err)
	}
}

func TestAgentTurnOneRunningAndFIFO(t *testing.T) {
	db := newTestDB(t)
	seedTurnProject(t, db, "p1", "s1")
	store := NewAgentTurnStore(db)
	first, _, _ := store.Create(AgentTurn{
		ID: "turn-a", ProjectID: "p1", SessionID: "s1",
		ClientRequestID: "a", CreatedAt: time.Now().UTC().Add(-time.Minute),
	})
	second, _, _ := store.Create(AgentTurn{
		ID: "turn-b", ProjectID: "p1", SessionID: "s1",
		ClientRequestID: "b", CreatedAt: time.Now().UTC(),
	})
	head, ok, err := store.NextQueued("s1")
	if err != nil || !ok || head.ID != first.ID {
		t.Fatalf("FIFO head=%+v ok=%v err=%v", head, ok, err)
	}
	queued, err := store.QueuedBySession("s1")
	if err != nil || len(queued) != 2 || queued[0].ID != first.ID || queued[1].ID != second.ID {
		t.Fatalf("QueuedBySession=%+v err=%v", queued, err)
	}
	if _, err := store.Transition(first.ID, AgentTurnTransition{Status: AgentTurnRunning}); err != nil {
		t.Fatalf("start first: %v", err)
	}
	if _, err := store.Transition(second.ID, AgentTurnTransition{Status: AgentTurnRunning}); err == nil {
		t.Fatal("second running Turn unexpectedly succeeded")
	}
	running, ok, err := store.RunningBySession("s1")
	if err != nil || !ok || running.ID != first.ID {
		t.Fatalf("RunningBySession=%+v ok=%v err=%v", running, ok, err)
	}
}

func TestAgentTurnProjectValidationAndPagination(t *testing.T) {
	db := newTestDB(t)
	seedTurnProject(t, db, "p1", "s1")
	if err := db.EnsureProject("p2", "p2", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	store := NewAgentTurnStore(db)
	if _, _, err := store.Create(AgentTurn{
		ProjectID: "p2", SessionID: "s1", PromptText: "cross project",
	}); !errors.Is(err, ErrProjectMismatch) {
		t.Fatalf("cross-project err=%v, want ErrProjectMismatch", err)
	}

	base := time.Now().UTC().Add(-time.Hour)
	for i, id := range []string{"a", "b", "c"} {
		if _, _, err := store.Create(AgentTurn{
			ID: id, ProjectID: "p1", SessionID: "s1",
			ClientRequestID: id, CreatedAt: base.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("Create %s: %v", id, err)
		}
	}
	first, err := store.List(AgentTurnListOptions{ProjectID: "p1", Limit: 2})
	if err != nil || len(first.Items) != 2 || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("first page=%+v err=%v", first, err)
	}
	second, err := store.List(AgentTurnListOptions{
		ProjectID: "p1", Limit: 2, Cursor: first.NextCursor,
	})
	if err != nil || len(second.Items) != 1 || second.HasMore {
		t.Fatalf("second page=%+v err=%v", second, err)
	}
	if _, err := store.List(AgentTurnListOptions{
		ProjectID: "p1", Cursor: "not-a-cursor",
	}); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("invalid cursor err=%v", err)
	}
}

func TestReplyTurnIDRoundTripAndLegacyNull(t *testing.T) {
	db := newTestDB(t)
	seedTurnProject(t, db, "p1", "s1")
	taskStore := NewTaskStore(db)
	ws := t.TempDir()
	if err := db.EnsureProject("p-task", "p-task", ws); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := taskStore.Save(ws, &TasksConfig{Tasks: []Task{{
		ID: "task-1", Title: "turn replies", Status: TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
		Replies: []Reply{
			{ID: "legacy", Text: "old", Mode: ModePureComment, CreatedAt: now},
			{ID: "new", Text: "new", TurnID: "turn-1", Mode: ModeFollowUp, CreatedAt: now.Add(time.Second)},
		},
	}}}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	task, ok, err := taskStore.GetTask("task-1")
	if err != nil || !ok {
		t.Fatalf("GetTask: ok=%v err=%v", ok, err)
	}
	if len(task.Replies) != 2 || task.Replies[0].TurnID != "" || task.Replies[1].TurnID != "turn-1" {
		t.Fatalf("reply turn IDs = %+v", task.Replies)
	}
}
