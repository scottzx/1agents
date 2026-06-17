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
	if len(tools) != 7 {
		t.Fatalf("expected 7 tools, got %d", len(tools))
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
