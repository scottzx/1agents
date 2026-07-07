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
	if len(tools) != 10 {
		t.Fatalf("expected 10 tools, got %d", len(tools))
	}
}

// A default conversation session (no task lock, no role) exposes the full PM
// tool set — create_reminder is gone; a personal todo is a create_task with
// assignee='user', which maps dueAt → a scheduled trigger (the old reminder
// mapping, now unified into create_task).
func TestConversationExposesPMToolsAndPersonalTask(t *testing.T) {
	var gotBody map[string]any
	s, buf, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/agent/tasks" {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &gotBody)
			w.Write([]byte(`{"id":"r1","number":7,"title":"交报告","status":"pending"}`))
			return
		}
		w.Write([]byte(`[]`))
	})
	// No taskRole, no taskID: the project-wide conversation surface.

	env := call(t, s, buf, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	res, _ := env["result"].(map[string]any)
	tools, _ := res["tools"].([]any)
	names := map[string]bool{}
	for _, tv := range tools {
		if m, ok := tv.(map[string]any); ok {
			if n, _ := m["name"].(string); n != "" {
				names[n] = true
			}
		}
	}
	if !names["create_task"] || !names["create_milestone"] || !names["create_discussion"] {
		t.Errorf("conversation should expose the PM tool set, got %v", names)
	}
	if names["create_reminder"] {
		t.Error("create_reminder must no longer exist")
	}
	if names["submit_review"] {
		t.Error("submit_review is verifier-only, not advertised project-wide")
	}

	// A personal todo: create_task with assignee='user' + dueAt → scheduled.
	env = call(t, s, buf, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"create_task","arguments":{"title":"交报告","assignee":"user","dueAt":"2026-06-25T15:00:00+08:00"}}}`)
	if _, isErr := resultText(t, env); isErr {
		t.Fatalf("create_task returned error: %v", env)
	}
	if gotBody["assignee"] != "user" {
		t.Errorf("assignee = %v, want user", gotBody["assignee"])
	}
	if gotBody["scheduleType"] != "scheduled" || gotBody["scheduledAt"] != "2026-06-25T15:00:00+08:00" {
		t.Errorf("schedule fields = %v / %v", gotBody["scheduleType"], gotBody["scheduledAt"])
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

// TestCreateTaskForwardsGithubFields covers the #74 GitHub mapping inputs:
// githubAssignees and the github* sync anchors must reach the POST body when
// provided, and assignee (executing agent) must stay a separate field.
func TestCreateTaskForwardsGithubFields(t *testing.T) {
	var gotBody map[string]any
	s, buf, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/agent/tasks" {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &gotBody)
			w.Write([]byte(`{"id":"t9","number":9,"title":"New","status":"pending"}`))
			return
		}
		http.Error(w, "unexpected", 400)
	})

	env := call(t, s, buf, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"create_task","arguments":{"title":"New","assignee":"codex","githubAssignees":["alice","bob"],"githubRepo":"o/r","githubKind":"issue","githubNumber":74}}}`)
	if text, isErr := resultText(t, env); isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	if gotBody["assignee"] != "codex" {
		t.Fatalf("assignee (executing agent) lost: %v", gotBody["assignee"])
	}
	ga, _ := gotBody["githubAssignees"].([]any)
	if len(ga) != 2 || ga[0] != "alice" || ga[1] != "bob" {
		t.Fatalf("githubAssignees mismatch: %v", gotBody["githubAssignees"])
	}
	if gotBody["githubRepo"] != "o/r" || gotBody["githubKind"] != "issue" {
		t.Fatalf("github ref fields mismatch: %v", gotBody)
	}
	// githubNumber is JSON, so a float64 after decode.
	if n, _ := gotBody["githubNumber"].(float64); n != 74 {
		t.Fatalf("githubNumber mismatch: %v", gotBody["githubNumber"])
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

func TestGetTaskGraphMapsToGraphEndpoint(t *testing.T) {
	const graphJSON = `{"outgoing":[{"rel":"relates","task":{"id":"t9","number":9}}],"incoming":[]}`
	s, buf, _ := newTaskScopedServer(t, "t1", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/agent/tasks/t1/graph" {
			t.Errorf("get_task_graph hit wrong path %s", r.URL.Path)
		}
		w.Write([]byte(graphJSON))
	})
	env := call(t, s, buf, `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"get_task_graph","arguments":{"id":"t1"}}}`)
	text, isErr := resultText(t, env)
	if isErr {
		t.Fatalf("get_task_graph errored: %s", text)
	}
	if text != graphJSON {
		t.Fatalf("get_task_graph body = %s, want passthrough of graph JSON", text)
	}

	// A foreign id is rejected before any API call (task-scoped lock).
	s2, buf2, _ := newTaskScopedServer(t, "t1", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("get_task_graph must not hit API for a foreign id (path %s)", r.URL.Path)
	})
	env = call(t, s2, buf2, `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"get_task_graph","arguments":{"id":"other"}}}`)
	if text, isErr := resultText(t, env); !isErr {
		t.Fatalf("expected lock rejection for foreign id, got: %s", text)
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
	if len(tools) != 5 {
		t.Fatalf("expected 5 task-scoped tools, got %d", len(tools))
	}
	for _, tl := range tools {
		name, _ := tl.(map[string]any)["name"].(string)
		if !executorScopedTools[name] {
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
			w.Write([]byte(`{"id":"t1","priority":"high"}`))
			return
		}
		http.Error(w, "unexpected", 400)
	})
	// A non-status edit on the executor's own task is allowed.
	env := call(t, s, buf, `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"update_task","arguments":{"id":"t1","priority":"high"}}}`)
	if text, isErr := resultText(t, env); isErr {
		t.Fatalf("own-task update should succeed, got error: %s", text)
	}
	if !patched {
		t.Fatal("expected PATCH /api/agent/tasks/t1")
	}
}

// TestExecutorCannotSelfReportCompleted is the #132 guardrail: an executor
// (default task scope) may not write status=completed — completion is driven
// by the artifact/verification path, not by the agent self-reporting done. The
// PATCH must never reach the API.
func TestExecutorCannotSelfReportCompleted(t *testing.T) {
	s, buf, _ := newTaskScopedServer(t, "t1", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("update_task(status=completed) must not hit the API in executor scope (path %s)", r.URL.Path)
	})
	env := call(t, s, buf, `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"update_task","arguments":{"id":"t1","status":"completed"}}}`)
	if text, isErr := resultText(t, env); !isErr {
		t.Fatalf("executor status=completed should be rejected, got: %s", text)
	}
}

// TestExecutorCanCancelOwnTask confirms the guardrail is narrow: an executor may
// still give up by setting status=cancelled (this is not a false completion).
func TestExecutorCanCancelOwnTask(t *testing.T) {
	var patched bool
	s, buf, _ := newTaskScopedServer(t, "t1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && r.URL.Path == "/api/agent/tasks/t1" {
			patched = true
			w.Write([]byte(`{"id":"t1","status":"cancelled"}`))
			return
		}
		http.Error(w, "unexpected", 400)
	})
	env := call(t, s, buf, `{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"update_task","arguments":{"id":"t1","status":"cancelled"}}}`)
	if text, isErr := resultText(t, env); isErr {
		t.Fatalf("executor status=cancelled should succeed, got error: %s", text)
	}
	if !patched {
		t.Fatal("expected PATCH /api/agent/tasks/t1 for cancel")
	}
}

// TestExecutorUpdateTaskDoesNotAdvertiseCompleted asserts the executor-scoped
// update_task schema offers only `cancelled` as a settable status, so the agent
// is never told to try a completion the server will reject (#132).
func TestExecutorUpdateTaskDoesNotAdvertiseCompleted(t *testing.T) {
	s, buf, _ := newTaskScopedServer(t, "t1", func(w http.ResponseWriter, r *http.Request) {})
	env := call(t, s, buf, `{"jsonrpc":"2.0","id":9,"method":"tools/list"}`)
	res, _ := env["result"].(map[string]any)
	tools, _ := res["tools"].([]any)
	var found bool
	for _, tl := range tools {
		m, _ := tl.(map[string]any)
		if m["name"] != "update_task" {
			continue
		}
		found = true
		schema, _ := m["inputSchema"].(map[string]any)
		props, _ := schema["properties"].(map[string]any)
		statusProp, _ := props["status"].(map[string]any)
		enum, _ := statusProp["enum"].([]any) // JSON round-trip → []any
		if len(enum) != 1 || enum[0] != "cancelled" {
			t.Fatalf("executor update_task status enum = %v, want [cancelled]", statusProp["enum"])
		}
	}
	if !found {
		t.Fatal("update_task not advertised in executor scope")
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

// ── verifier scope (hard read-only + submit_review), #50 ────────────────────

// newVerifierScopedServer locks the session to taskID as a verifier.
func newVerifierScopedServer(t *testing.T, taskID string, h http.HandlerFunc) (*server, *bytes.Buffer, *httptest.Server) {
	t.Helper()
	s, buf, api := newTaskScopedServer(t, taskID, h)
	s.taskRole = "verifier"
	return s, buf, api
}

func TestVerifierScopeToolsList(t *testing.T) {
	s, buf, _ := newVerifierScopedServer(t, "t1", func(w http.ResponseWriter, r *http.Request) {})
	env := call(t, s, buf, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	res, _ := env["result"].(map[string]any)
	tools, _ := res["tools"].([]any)
	got := map[string]bool{}
	for _, tl := range tools {
		name, _ := tl.(map[string]any)["name"].(string)
		got[name] = true
	}
	if len(got) != 4 || !got["list_tasks"] || !got["get_task"] || !got["get_task_graph"] || !got["submit_review"] {
		t.Fatalf("verifier scope tools = %v, want {list_tasks, get_task, get_task_graph, submit_review}", got)
	}
	if got["update_task"] {
		t.Error("update_task must NOT be advertised to a verifier (hard read-only)")
	}
}

func TestVerifierScopeBlocksUpdateTask(t *testing.T) {
	s, buf, _ := newVerifierScopedServer(t, "t1", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("update_task must not hit the API in verifier scope (path %s)", r.URL.Path)
	})
	env := call(t, s, buf, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"update_task","arguments":{"id":"t1","status":"completed"}}}`)
	if text, isErr := resultText(t, env); !isErr {
		t.Fatalf("expected verifier update_task rejection, got: %s", text)
	}
}

func TestVerifierSubmitReviewPostsVerdict(t *testing.T) {
	var posted bool
	s, buf, _ := newVerifierScopedServer(t, "t1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/agent/tasks/t1/review" {
			posted = true
			w.Write([]byte(`{"id":"t1","status":"completed"}`))
			return
		}
		http.Error(w, "unexpected", 400)
	})
	env := call(t, s, buf, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"submit_review","arguments":{"criteria":[{"criterion":"c","pass":true}],"summary":"ok"}}}`)
	if text, isErr := resultText(t, env); isErr {
		t.Fatalf("submit_review should succeed, got error: %s", text)
	}
	if !posted {
		t.Fatal("expected POST /api/agent/tasks/t1/review")
	}
}

func TestVerifierSubmitReviewRequiresCriteria(t *testing.T) {
	s, buf, _ := newVerifierScopedServer(t, "t1", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("must not hit API with empty criteria (path %s)", r.URL.Path)
	})
	env := call(t, s, buf, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"submit_review","arguments":{"criteria":[]}}}`)
	if text, isErr := resultText(t, env); !isErr {
		t.Fatalf("empty criteria should be rejected, got: %s", text)
	}
}

func TestExecutorScopeBlocksSubmitReview(t *testing.T) {
	// Default task-scope is executor: submit_review is not in its surface.
	s, buf, _ := newTaskScopedServer(t, "t1", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("submit_review must not hit the API in executor scope (path %s)", r.URL.Path)
	})
	env := call(t, s, buf, `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"submit_review","arguments":{"criteria":[{"criterion":"c","pass":true}]}}}`)
	if text, isErr := resultText(t, env); !isErr {
		t.Fatalf("executor submit_review should be rejected, got: %s", text)
	}
}
