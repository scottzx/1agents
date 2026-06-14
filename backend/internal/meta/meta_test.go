package meta

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "meta.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestProjectsEnsureAndList(t *testing.T) {
	db := newTestDB(t)
	if err := db.EnsureProject("ws1", "Project One", "/tmp/p1"); err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	// Re-ensure with a new name → updates, no duplicate.
	if err := db.EnsureProject("ws1", "Renamed", "/tmp/p1"); err != nil {
		t.Fatalf("EnsureProject again: %v", err)
	}
	all, err := db.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(all) != 1 || all[0].Name != "Renamed" || all[0].WorkspacePath != "/tmp/p1" {
		t.Fatalf("unexpected projects: %+v", all)
	}
	p, ok, err := db.GetProject("ws1")
	if err != nil || !ok || p.ID != "ws1" {
		t.Fatalf("GetProject: ok=%v err=%v p=%+v", ok, err, p)
	}
}

func TestSessionStoreCRUD(t *testing.T) {
	db := newTestDB(t)
	s := NewSessionStore(db)

	rec := ChatSessionRecord{
		ID:          "abc",
		WorkspaceID: "ws1",
		Name:        "first",
		AgentType:   "claudecode",
		CcProject:   "ws1__claudecode",
		CcSessionID: "cc-1",
		SessionKey:  "chatui:ws1:cc-1",
	}
	if err := s.Add(rec); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := s.Add(rec); err != ErrDuplicate {
		t.Fatalf("duplicate add: got %v, want ErrDuplicate", err)
	}

	got, ok, err := s.Get("abc")
	if err != nil || !ok {
		t.Fatalf("Get: ok=%v err=%v", ok, err)
	}
	if got.Name != "first" || got.WorkspaceID != "ws1" || got.CreatedAt.IsZero() {
		t.Fatalf("Get returned wrong record: %+v", got)
	}

	if err := s.UpdateName("abc", "renamed"); err != nil {
		t.Fatalf("UpdateName: %v", err)
	}
	if err := s.UpdatePermissionMode("abc", "approve-all"); err != nil {
		t.Fatalf("UpdatePermissionMode: %v", err)
	}
	if err := s.UpdateTask("abc", "task-1"); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if err := s.Touch("abc"); err != nil {
		t.Fatalf("Touch: %v", err)
	}
	got, _, _ = s.Get("abc")
	if got.Name != "renamed" || got.PermissionMode != "approve-all" ||
		got.TaskID != "task-1" || got.LastEventAt.IsZero() {
		t.Fatalf("updates not persisted: %+v", got)
	}

	// UpdateACP only sets when empty.
	if err := s.UpdateACP("abc", "uuid-1"); err != nil {
		t.Fatalf("UpdateACP: %v", err)
	}
	if err := s.UpdateACP("abc", "uuid-2"); err != nil {
		t.Fatalf("UpdateACP second: %v", err)
	}
	got, _, _ = s.Get("abc")
	if got.AcpSessionID != "uuid-1" {
		t.Fatalf("AcpSessionID = %q, want uuid-1 (first write wins)", got.AcpSessionID)
	}

	if err := s.Delete("abc"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := s.Delete("abc"); err != ErrNotFound {
		t.Fatalf("delete missing: got %v, want ErrNotFound", err)
	}
	if _, ok, _ := s.Get("abc"); ok {
		t.Fatalf("record still found after delete")
	}
}

func TestSessionListNewestFirst(t *testing.T) {
	db := newTestDB(t)
	s := NewSessionStore(db)
	base := time.Now().UTC().Add(-time.Hour)
	for i, id := range []string{"a", "b", "c"} {
		_ = s.Add(ChatSessionRecord{
			ID:          id,
			WorkspaceID: "ws",
			CreatedAt:   base.Add(time.Duration(i) * time.Minute),
		})
	}
	all, err := s.ListByWorkspace("ws")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 3 || all[0].ID != "c" || all[2].ID != "a" {
		t.Fatalf("wrong order: %+v", all)
	}
	none, _ := s.ListByWorkspace("nope")
	if len(none) != 0 {
		t.Fatalf("expected empty list for unknown workspace")
	}
}

func TestTaskStoreLoadSaveRoundTrip(t *testing.T) {
	db := newTestDB(t)
	s := NewTaskStore(db)
	ws := t.TempDir()

	now := time.Now().UTC().Truncate(time.Millisecond)
	planned := now.Add(24 * time.Hour)
	cfg := &TasksConfig{Tasks: []Task{
		{
			ID:           "t1",
			Title:        "first",
			Description:  "body **md**",
			Status:       TaskStatusPending,
			ScheduleType: ScheduleTypeImmediate,
			PlannedStart: &now,
			PlannedEnd:   &planned,
			DependsOn:    []string{"t0", "tX"},
			CreatedAt:    now,
			UpdatedAt:    now,
			Replies: []Reply{
				{Author: Author{Kind: "user", Name: "scott"}, Text: "先调研", Mode: ModeNewSession, SessionRef: "sess-1"},
			},
			Sessions: []SessionMetadata{
				{ID: "sess-1", Kind: SessionKindChat, Name: "调研", AgentType: "claudecode", Status: SessionStatusRunning, CreatedAt: now},
			},
		},
		{ID: "t2", Title: "second", Status: TaskStatusCompleted, CreatedAt: now.Add(time.Second), UpdatedAt: now},
	}}
	if err := s.Save(ws, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := s.Load(ws)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Tasks) != 2 {
		t.Fatalf("loaded %d tasks, want 2", len(loaded.Tasks))
	}
	t1 := loaded.Tasks[0]
	if t1.ID != "t1" || t1.Title != "first" || t1.Description != "body **md**" {
		t.Fatalf("t1 wrong: %+v", t1)
	}
	if t1.IssueState != IssueOpen {
		t.Fatalf("IssueState = %q, want open default", t1.IssueState)
	}
	if t1.PlannedStart == nil || !t1.PlannedStart.Equal(now) || t1.PlannedEnd == nil {
		t.Fatalf("planned times lost: %+v %+v", t1.PlannedStart, t1.PlannedEnd)
	}
	if len(t1.DependsOn) != 2 || t1.DependsOn[0] != "t0" || t1.DependsOn[1] != "tX" {
		t.Fatalf("deps wrong: %v", t1.DependsOn)
	}
	if len(t1.Replies) != 1 || t1.Replies[0].Text != "先调研" || t1.Replies[0].ID == "" {
		t.Fatalf("replies wrong: %+v", t1.Replies)
	}
	if len(t1.Sessions) != 1 || t1.Sessions[0].ID != "sess-1" ||
		t1.Sessions[0].Status != SessionStatusRunning {
		t.Fatalf("sessions wrong: %+v", t1.Sessions)
	}
	// ReplyIDs reverse index: the reply references sess-1.
	if len(t1.Sessions[0].ReplyIDs) != 1 || t1.Sessions[0].ReplyIDs[0] != t1.Replies[0].ID {
		t.Fatalf("ReplyIDs wrong: %+v", t1.Sessions[0].ReplyIDs)
	}
	if t1.WorkspacePath != ws {
		t.Fatalf("WorkspacePath = %q, want %q", t1.WorkspacePath, ws)
	}

	// Whole-config replace: dropping t2 deletes it.
	cfg.Tasks = cfg.Tasks[:1]
	if err := s.Save(ws, cfg); err != nil {
		t.Fatalf("Save 2: %v", err)
	}
	loaded, _ = s.Load(ws)
	if len(loaded.Tasks) != 1 || loaded.Tasks[0].ID != "t1" {
		t.Fatalf("t2 not deleted: %+v", loaded.Tasks)
	}
}

func TestTaskStoreIssueMutations(t *testing.T) {
	db := newTestDB(t)
	s := NewTaskStore(db)
	ws := t.TempDir()
	now := time.Now().UTC()
	if err := s.Save(ws, &TasksConfig{Tasks: []Task{
		{ID: "t1", Title: "x", Status: TaskStatusPending, CreatedAt: now, UpdatedAt: now},
	}}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	rp, err := s.AppendReply("t1", Reply{Author: Author{Kind: "user", Name: "scott"}, Text: "hello"})
	if err != nil {
		t.Fatalf("AppendReply: %v", err)
	}
	if rp.ID == "" || rp.Mode != ModePureComment {
		t.Fatalf("reply defaults missing: %+v", rp)
	}
	rp2, err := s.AppendReply("t1", Reply{Author: Author{Kind: "agent", Name: "claude"}, Text: "done", Mode: ModeFollowUp})
	if err != nil {
		t.Fatalf("AppendReply 2: %v", err)
	}
	if _, err := s.AppendReply("missing", Reply{Text: "x"}); err != ErrNotFound {
		t.Fatalf("AppendReply missing task: got %v, want ErrNotFound", err)
	}

	if err := s.UpdateDescription("t1", "new body"); err != nil {
		t.Fatalf("UpdateDescription: %v", err)
	}
	if err := s.SetIssueState("t1", IssueClosed); err != nil {
		t.Fatalf("SetIssueState: %v", err)
	}

	task, ok, err := s.GetTask("t1")
	if err != nil || !ok {
		t.Fatalf("GetTask: ok=%v err=%v", ok, err)
	}
	if task.Description != "new body" || task.IssueState != IssueClosed {
		t.Fatalf("mutations not persisted: %+v", task)
	}
	if len(task.Replies) != 2 || task.Replies[0].ID != rp.ID || task.Replies[1].ID != rp2.ID {
		t.Fatalf("timeline wrong: %+v", task.Replies)
	}
	if task.WorkspacePath != ws {
		t.Fatalf("GetTask workspace path = %q, want %q", task.WorkspacePath, ws)
	}
}

func TestLegacyTasksImport(t *testing.T) {
	db := newTestDB(t)
	s := NewTaskStore(db)
	ws := t.TempDir()

	legacyDir := filepath.Join(ws, ".1agents")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := TasksConfig{Tasks: []Task{
		{ID: "old1", Title: "legacy task", Status: TaskStatusCompleted,
			DependsOn: []string{"old0"},
			Sessions: []SessionMetadata{
				{ID: "ls1", Kind: SessionKindChat, Name: "legacy sess", AgentType: "claudecode", Status: SessionStatusIdle, Summary: "did things"},
			},
			CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
	}}
	data, _ := json.Marshal(legacy)
	if err := os.WriteFile(filepath.Join(legacyDir, "tasks.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := s.Load(ws)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Tasks) != 1 || loaded.Tasks[0].ID != "old1" {
		t.Fatalf("legacy import failed: %+v", loaded.Tasks)
	}
	if loaded.Tasks[0].Sessions[0].Summary != "did things" {
		t.Fatalf("legacy session metadata lost: %+v", loaded.Tasks[0].Sessions)
	}
	if _, err := os.Stat(filepath.Join(legacyDir, "tasks.json")); !os.IsNotExist(err) {
		t.Fatalf("legacy file not renamed")
	}
	if _, err := os.Stat(filepath.Join(legacyDir, "tasks.json.migrated")); err != nil {
		t.Fatalf("migrated backup missing: %v", err)
	}
	// Second load: no double import.
	loaded, _ = s.Load(ws)
	if len(loaded.Tasks) != 1 {
		t.Fatalf("double import: %d tasks", len(loaded.Tasks))
	}
}

func TestMigrateLegacySessions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ONEAGENTS_HOME", home)
	db := newTestDB(t)

	dir := filepath.Join(home, ".1agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := map[string]any{"sessions": []ChatSessionRecord{
		{ID: "s1", WorkspaceID: "ws1", Name: "legacy chat", AgentType: "claudecode",
			CcSessionID: "cc1", SessionKey: "k1", CreatedAt: time.Now().UTC()},
	}}
	data, _ := json.Marshal(legacy)
	if err := os.WriteFile(filepath.Join(dir, "agent-sessions.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := db.MigrateLegacy([]WorkspaceRef{{ID: "ws1", Name: "One", Path: t.TempDir()}}); err != nil {
		t.Fatalf("MigrateLegacy: %v", err)
	}

	s := NewSessionStore(db)
	rec, ok, err := s.Get("s1")
	if err != nil || !ok || rec.Name != "legacy chat" {
		t.Fatalf("session not migrated: ok=%v err=%v rec=%+v", ok, err, rec)
	}
	if _, ok, _ := db.GetProject("ws1"); !ok {
		t.Fatalf("project not created from workspace ref")
	}
	if _, err := os.Stat(filepath.Join(dir, "agent-sessions.json.migrated")); err != nil {
		t.Fatalf("legacy sessions file not renamed: %v", err)
	}
	// Idempotent rerun.
	if err := db.MigrateLegacy(nil); err != nil {
		t.Fatalf("MigrateLegacy rerun: %v", err)
	}
}

func TestTaskV2FieldsRoundTrip(t *testing.T) {
	db := newTestDB(t)
	s := NewTaskStore(db)
	ws := t.TempDir()
	now := time.Now().UTC()

	cfg := &TasksConfig{Tasks: []Task{{
		ID: "parent-1", Title: "父任务", Status: TaskStatusPending,
		Priority: PriorityUrgent, Assignee: "codex",
		Labels: []string{"infra", "高风险"}, CreatedBy: "scheduler",
		Milestone:          "v1.0",
		AcceptanceCriteria: "hello.txt 存在且内容为 hello",
		Recurrence:         &Recurrence{Freq: "weekly", Weekday: 1, At: "09:00"},
		MaxRetries:         3, RetryCount: 1, TimeoutMinutes: 20,
		CreatedAt: now, UpdatedAt: now,
	}, {
		ID: "child-1", Title: "子任务", Status: TaskStatusPending,
		ParentID:  "parent-1",
		CreatedAt: now.Add(time.Second), UpdatedAt: now,
	}}}
	if err := s.Save(ws, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := s.Load(ws)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	p := loaded.Tasks[0]
	if p.Priority != PriorityUrgent || p.Assignee != "codex" || p.CreatedBy != "scheduler" ||
		p.Milestone != "v1.0" || p.AcceptanceCriteria == "" ||
		p.MaxRetries != 3 || p.RetryCount != 1 || p.TimeoutMinutes != 20 {
		t.Fatalf("v2 fields lost: %+v", p)
	}
	if len(p.Labels) != 2 || p.Labels[1] != "高风险" {
		t.Fatalf("labels lost: %v", p.Labels)
	}
	if p.Recurrence == nil || p.Recurrence.Freq != "weekly" || p.Recurrence.Weekday != 1 || p.Recurrence.At != "09:00" {
		t.Fatalf("recurrence lost: %+v", p.Recurrence)
	}
	if loaded.Tasks[1].ParentID != "parent-1" {
		t.Fatalf("parent link lost: %+v", loaded.Tasks[1])
	}
	// Defaults applied on save for the child.
	if loaded.Tasks[1].Priority != PriorityMedium || loaded.Tasks[1].CreatedBy != "user" {
		t.Fatalf("defaults not applied: %+v", loaded.Tasks[1])
	}
}

func TestSchemaV1ToV2Upgrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.db")

	// Build a v1 database by hand (schema v1 + one legacy-shaped task row).
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(schemaV1); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO projects (id, name, workspace_path, created_at, updated_at)
		VALUES ('ws1', 'P', '/tmp/v1ws', '2026-06-01T00:00:00Z', '2026-06-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO tasks (id, project_id, title, status, created_at, updated_at)
		VALUES ('t1', 'ws1', '老任务', 'completed', '2026-06-01T00:00:00Z', '2026-06-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`PRAGMA user_version = 1`); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	// Reopen through meta.Open → v2 migration must run and data survive.
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open after v1: %v", err)
	}
	defer db.Close()
	task, ok, err := NewTaskStore(db).GetTask("t1")
	if err != nil || !ok {
		t.Fatalf("GetTask after upgrade: ok=%v err=%v", ok, err)
	}
	if task.Title != "老任务" || task.Status != TaskStatusCompleted {
		t.Fatalf("v1 data lost: %+v", task)
	}
	if task.Priority != PriorityMedium || task.MaxRetries != 1 {
		t.Fatalf("v2 defaults missing: priority=%q maxRetries=%d", task.Priority, task.MaxRetries)
	}
}

func TestTaskSprintFieldRoundTrip(t *testing.T) {
	db := newTestDB(t)
	s := NewTaskStore(db)
	ws := t.TempDir()
	now := time.Now().UTC()

	cfg := &TasksConfig{Tasks: []Task{{
		ID: "sprinted", Title: "本冲刺", Status: TaskStatusPending,
		Sprint:    "Sprint 23",
		CreatedAt: now, UpdatedAt: now,
	}, {
		ID: "unsprinted", Title: "非冲刺", Status: TaskStatusPending,
		CreatedAt: now.Add(time.Second), UpdatedAt: now,
	}}}
	if err := s.Save(ws, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := s.Load(ws)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Tasks) != 2 {
		t.Fatalf("loaded %d tasks, want 2", len(loaded.Tasks))
	}
	if loaded.Tasks[0].Sprint != "Sprint 23" {
		t.Fatalf("sprint lost: %q", loaded.Tasks[0].Sprint)
	}
	if loaded.Tasks[1].Sprint != "" {
		t.Fatalf("unsprinted task should have empty sprint, got %q", loaded.Tasks[1].Sprint)
	}
}

func TestSchemaV2ToV3Upgrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.db")

	// Build a v2 database by hand: apply v1 + v2 SQL, insert a legacy task
	// (the v2 row was written before the sprint column existed), and pin
	// user_version=2 so reopen runs the v3 migration.
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(schemaV1); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(schemaV2); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO projects (id, name, workspace_path, created_at, updated_at)
		VALUES ('ws1', 'P', '/tmp/v2ws', '2026-06-01T00:00:00Z', '2026-06-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO tasks (id, project_id, title, status, created_at, updated_at,
			priority, assignee, labels, created_by, parent_id, milestone,
			acceptance_criteria, recurrence, max_retries, retry_count, timeout_minutes)
		VALUES ('legacy', 'ws1', 'v2 老任务', 'completed',
			'2026-06-01T00:00:00Z', '2026-06-01T00:00:00Z',
			'medium', '', '[]', 'user', '', '', '', '', 1, 0, 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`PRAGMA user_version = 2`); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	// Reopen through meta.Open → the v3+v4 migrations must run, data must
	// survive, and the legacy row's sprint/type should take their defaults.
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open after v2: %v", err)
	}
	defer db.Close()

	var got int
	if err := db.sql.QueryRow("PRAGMA user_version").Scan(&got); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if got != schemaVersion {
		t.Fatalf("user_version = %d, want %d", got, schemaVersion)
	}

	store := NewTaskStore(db)
	task, ok, err := store.GetTask("legacy")
	if err != nil || !ok {
		t.Fatalf("GetTask after upgrade: ok=%v err=%v", ok, err)
	}
	if task.Title != "v2 老任务" || task.Status != TaskStatusCompleted {
		t.Fatalf("v2 data lost: %+v", task)
	}
	if task.Sprint != "" {
		t.Fatalf("legacy v2 row should have empty sprint, got %q", task.Sprint)
	}
	if task.Type != TaskTypeTask {
		t.Fatalf("legacy row type = %q, want default 'task'", task.Type)
	}

	// A new task written after the upgrade can opt into a sprint.
	ws := t.TempDir()
	now := time.Now().UTC()
	if err := store.Save(ws, &TasksConfig{Tasks: []Task{{
		ID: "new", Title: "v3 新任务", Status: TaskStatusPending,
		Sprint: "Sprint 24", CreatedAt: now, UpdatedAt: now,
	}}}); err != nil {
		t.Fatalf("Save after upgrade: %v", err)
	}
	loaded, err := store.Load(ws)
	if err != nil {
		t.Fatalf("Load after upgrade: %v", err)
	}
	if len(loaded.Tasks) != 1 || loaded.Tasks[0].Sprint != "Sprint 24" {
		t.Fatalf("post-upgrade sprint round-trip failed: %+v", loaded.Tasks)
	}
}
