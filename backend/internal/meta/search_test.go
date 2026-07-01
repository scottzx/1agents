package meta

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSearchHandler(t *testing.T) {
	db := newTestDB(t)
	ws := t.TempDir()
	now := time.Now().UTC()

	if err := db.EnsureProject("proj-1", "My Project", ws); err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	ts := NewTaskStore(db)
	if err := ts.Save(ws, &TasksConfig{Tasks: []Task{
		{ID: "t1", Title: "Fix login bug", Status: TaskStatusPending, CreatedAt: now, UpdatedAt: now},
		{ID: "t2", Title: "Write docs", Description: "cover the login flow", Status: TaskStatusPending, CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second)},
	}}); err != nil {
		t.Fatalf("Save tasks: %v", err)
	}

	ss := NewSessionStore(db)
	if err := ss.Add(ChatSessionRecord{ID: "s1", WorkspaceID: "proj-1", Name: "login troubleshooting", AgentType: "claudecode", CreatedAt: now}); err != nil {
		t.Fatalf("Add session: %v", err)
	}
	if err := ss.Add(ChatSessionRecord{ID: "s2", WorkspaceID: "proj-1", Name: "unrelated chat", AgentType: "claudecode", CreatedAt: now}); err != nil {
		t.Fatalf("Add session: %v", err)
	}

	call := func(q string) (tasks, sessions []map[string]any) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/search?q="+q, nil)
		rec := httptest.NewRecorder()
		SearchHandler(db)(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var out struct {
			Tasks    []map[string]any `json:"tasks"`
			Sessions []map[string]any `json:"sessions"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return out.Tasks, out.Sessions
	}

	// "login" hits both tasks (title + description) and one session (name).
	tasks, sessions := call("login")
	if len(tasks) != 2 {
		t.Fatalf("login tasks = %d, want 2", len(tasks))
	}
	if len(sessions) != 1 || sessions[0]["id"] != "s1" {
		t.Fatalf("login sessions = %+v, want [s1]", sessions)
	}
	// Project name is joined in.
	if tasks[0]["project_name"] != "My Project" {
		t.Fatalf("project_name = %v, want My Project", tasks[0]["project_name"])
	}

	// #number jump: "#2" resolves the task numbered 2.
	tasks, _ = call("%232") // %23 = '#'
	found := false
	for _, tk := range tasks {
		if tk["number"] == float64(2) {
			found = true
		}
	}
	if !found {
		t.Fatalf("#2 search did not return task number 2: %+v", tasks)
	}

	// Blank query short-circuits to empty (no error).
	tasks, sessions = call("")
	if len(tasks) != 0 || len(sessions) != 0 {
		t.Fatalf("blank query returned results: tasks=%d sessions=%d", len(tasks), len(sessions))
	}
}
