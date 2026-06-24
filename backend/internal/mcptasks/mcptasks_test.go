package mcptasks

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newTestServer wires an mcptasks server to a fake task API and captures its
// stdout into buf.
func newTestServer(t *testing.T, h http.HandlerFunc) (*server, *bytes.Buffer, *httptest.Server) {
	t.Helper()
	api := httptest.NewServer(h)
	t.Cleanup(api.Close)
	buf := &bytes.Buffer{}
	s := &server{
		api:         &apiClient{baseURL: api.URL, token: "tok", http: &http.Client{Timeout: 5 * time.Second}},
		workspaceID: "ws1",
		out:         bufio.NewWriter(buf),
	}
	return s, buf, api
}

// call feeds one JSON-RPC line and returns the decoded response envelope.
func call(t *testing.T, s *server, buf *bytes.Buffer, line string) map[string]any {
	t.Helper()
	buf.Reset()
	s.handleLine([]byte(line))
	if buf.Len() == 0 {
		return nil
	}
	var env map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &env); err != nil {
		t.Fatalf("decode response %q: %v", buf.String(), err)
	}
	return env
}

// resultText extracts the first text block of a tools/call result.
func resultText(t *testing.T, env map[string]any) (string, bool) {
	t.Helper()
	res, ok := env["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result in %v", env)
	}
	isErr, _ := res["isError"].(bool)
	content, _ := res["content"].([]any)
	if len(content) == 0 {
		return "", isErr
	}
	first, _ := content[0].(map[string]any)
	text, _ := first["text"].(string)
	return text, isErr
}

func twoTasks() string {
	return `[{"id":"t1","number":1,"title":"A","status":"pending","type":"task"},
	         {"id":"t2","number":2,"title":"B","status":"completed","type":"bug","milestone":"M1"}]`
}

func TestInitializeAndToolsList(t *testing.T) {
	s, buf, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {})

	env := call(t, s, buf, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`)
	res, _ := env["result"].(map[string]any)
	if res["protocolVersion"] != "2024-11-05" {
		t.Fatalf("protocolVersion = %v", res["protocolVersion"])
	}

	env = call(t, s, buf, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	res, _ = env["result"].(map[string]any)
	tools, _ := res["tools"].([]any)
	if len(tools) != 8 {
		t.Fatalf("expected 8 tools, got %d", len(tools))
	}
}

func TestNotificationProducesNoResponse(t *testing.T) {
	s, buf, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {})
	if env := call(t, s, buf, `{"jsonrpc":"2.0","method":"notifications/initialized"}`); env != nil {
		t.Fatalf("notification should produce no response, got %v", env)
	}
}

func TestCreateTaskMapsToPost(t *testing.T) {
	var gotBody map[string]any
	var gotAuth string
	s, buf, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/agent/tasks" {
			gotAuth = r.Header.Get("Authorization")
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &gotBody)
			w.Write([]byte(`{"id":"t9","number":9,"title":"New","status":"pending","dependsOn":["t1"]}`))
			return
		}
		http.Error(w, "unexpected", 400)
	})

	env := call(t, s, buf, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"create_task","arguments":{"title":"New","dependsOn":["t1"],"type":"requirement"}}}`)
	text, isErr := resultText(t, env)
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	if gotAuth != "Bearer tok" {
		t.Fatalf("auth header = %q", gotAuth)
	}
	if gotBody["workspace_id"] != "ws1" {
		t.Fatalf("workspace_id not locked: %v", gotBody["workspace_id"])
	}
	if gotBody["title"] != "New" || gotBody["type"] != "requirement" {
		t.Fatalf("body mismatch: %v", gotBody)
	}
	deps, _ := gotBody["dependsOn"].([]any)
	if len(deps) != 1 || deps[0] != "t1" {
		t.Fatalf("dependsOn mismatch: %v", gotBody["dependsOn"])
	}
}

func TestGetTaskRejectsForeignId(t *testing.T) {
	s, buf, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/agent/tasks" {
			w.Write([]byte(twoTasks()))
			return
		}
		t.Errorf("get_task must not hit single-task endpoint for a foreign id (path %s)", r.URL.Path)
	})
	env := call(t, s, buf, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"get_task","arguments":{"id":"foreign"}}}`)
	text, isErr := resultText(t, env)
	if !isErr {
		t.Fatalf("expected lock error, got: %s", text)
	}
}

// ── task-scoped (executor) lock, #50 ────────────────────────────────────────

// newTaskScopedServer is newTestServer with the session locked to taskID.
func newTaskScopedServer(t *testing.T, taskID string, h http.HandlerFunc) (*server, *bytes.Buffer, *httptest.Server) {
	t.Helper()
	s, buf, api := newTestServer(t, h)
	s.taskID = taskID
	return s, buf, api
}

func TestTaskScopedToolsListIsNarrowed(t *testing.T) {
	s, buf, _ := newTaskScopedServer(t, "t1", func(w http.ResponseWriter, r *http.Request) {})
	env := call(t, s, buf, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	res, _ := env["result"].(map[string]any)
	tools, _ := res["tools"].([]any)
	if len(tools) != 3 {
		t.Fatalf("expected 3 task-scoped tools, got %d", len(tools))
	}
	for _, tl := range tools {
		name, _ := tl.(map[string]any)["name"].(string)
		if !taskScopedTools[name] {
			t.Errorf("unexpected tool advertised in task scope: %q", name)
		}
	}
}

func TestTaskScopedBlocksPMTools(t *testing.T) {
	s, buf, _ := newTaskScopedServer(t, "t1", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("create_task must not hit the API in a task-scoped session (path %s)", r.URL.Path)
	})
	env := call(t, s, buf, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"create_task","arguments":{"title":"X"}}}`)
	if text, isErr := resultText(t, env); !isErr {
		t.Fatalf("expected task-scope rejection, got: %s", text)
	}
}

func TestTaskScopedRejectsForeignTask(t *testing.T) {
	s, buf, _ := newTaskScopedServer(t, "t1", func(w http.ResponseWriter, r *http.Request) {
		// A foreign id must be rejected before any single-task API call.
		t.Errorf("must not hit API for a foreign id (path %s)", r.URL.Path)
	})
	// get_task on the other task
	env := call(t, s, buf, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"get_task","arguments":{"id":"t2"}}}`)
	if text, isErr := resultText(t, env); !isErr {
		t.Fatalf("get_task foreign: expected rejection, got: %s", text)
	}
	// update_task on the other task
	env = call(t, s, buf, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"update_task","arguments":{"id":"t2","status":"completed"}}}`)
	if text, isErr := resultText(t, env); !isErr {
		t.Fatalf("update_task foreign: expected rejection, got: %s", text)
	}
}

func TestTaskScopedListFiltersToSelf(t *testing.T) {
	s, buf, _ := newTaskScopedServer(t, "t1", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(twoTasks()))
	})
	env := call(t, s, buf, `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"list_tasks","arguments":{}}}`)
	text, isErr := resultText(t, env)
	if isErr {
		t.Fatalf("unexpected error: %s", text)
	}
	var payload struct {
		Count int `json:"count"`
		Tasks []struct {
			ID string `json:"id"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("decode: %v (%s)", err, text)
	}
	if payload.Count != 1 || payload.Tasks[0].ID != "t1" {
		t.Fatalf("task-scoped list should return only t1, got: %s", text)
	}
}

func TestTaskScopedAllowsOwnUpdate(t *testing.T) {
	var patched bool
	s, buf, _ := newTaskScopedServer(t, "t1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && r.URL.Path == "/api/agent/tasks/t1" {
			patched = true
			w.Write([]byte(`{"id":"t1","status":"completed"}`))
			return
		}
		http.Error(w, "unexpected", 400)
	})
	env := call(t, s, buf, `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"update_task","arguments":{"id":"t1","status":"completed"}}}`)
	if text, isErr := resultText(t, env); isErr {
		t.Fatalf("own-task update should succeed, got error: %s", text)
	}
	if !patched {
		t.Fatal("expected PATCH /api/agent/tasks/t1")
	}
}

func TestListMilestones(t *testing.T) {
	// list_milestones now proxies GET /api/agent/milestones (first-class
	// milestone entities) rather than aggregating tasks client-side.
	s, buf, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/agent/milestones" {
			w.Write([]byte(`[{"id":"m1","name":"M1","position":0,"total":2,"completed":1,
				"targetDate":"2026-07-01T00:00:00Z","createdAt":"2026-06-01T00:00:00Z","updatedAt":"2026-06-01T00:00:00Z"}]`))
			return
		}
		w.Write([]byte(twoTasks()))
	})
	env := call(t, s, buf, `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"list_milestones","arguments":{}}}`)
	text, isErr := resultText(t, env)
	if isErr {
		t.Fatalf("unexpected error: %s", text)
	}
	var payload struct {
		Count      int `json:"count"`
		Milestones []struct {
			Name      string `json:"name"`
			Total     int    `json:"total"`
			Completed int    `json:"completed"`
		} `json:"milestones"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("decode: %v (%s)", err, text)
	}
	if payload.Count != 1 || payload.Milestones[0].Name != "M1" || payload.Milestones[0].Completed != 1 {
		t.Fatalf("milestone passthrough wrong: %s", text)
	}
}
