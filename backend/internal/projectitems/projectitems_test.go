package projectitems

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
	// 10 legacy + 6 Workspace Inbox + 7 Feature Catalog tools.
	if len(tools) != 23 {
		t.Fatalf("expected 23 tools, got %d", len(tools))
	}
	got := map[string]bool{}
	for _, tl := range tools {
		name, _ := tl.(map[string]any)["name"].(string)
		got[name] = true
	}
	for _, want := range []string{
		"check_inbox", "get_mail", "accept_mail", "archive_mail", "list_mail_targets", "send_mail",
		"list_feature_catalog", "create_feature_node", "update_feature_node",
		"move_feature_node", "link_feature_item", "unlink_feature_item",
		"batch_feature_catalog",
	} {
		if !got[want] {
			t.Errorf("PM tools/list missing mail tool %q; got %v", want, got)
		}
	}
}

// A default conversation session (no task lock, no role) exposes the full PM
// tool set — create_reminder is gone; a personal todo is a create_project_item with
// assignee='user', which maps dueAt → a scheduled trigger (the old reminder
// mapping, now unified into create_project_item).
func TestConversationExposesPMToolsAndPersonalTask(t *testing.T) {
	var gotBody map[string]any
	s, buf, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/agent/project-items" {
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
	if !names["create_project_item"] || !names["create_milestone"] || !names["create_discussion"] {
		t.Errorf("conversation should expose the PM tool set, got %v", names)
	}
	if names["create_reminder"] {
		t.Error("create_reminder must no longer exist")
	}
	if names["submit_review"] {
		t.Error("submit_review is verifier-only, not advertised project-wide")
	}

	// A personal todo: create_project_item with assignee='user' + dueAt → scheduled.
	env = call(t, s, buf, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"create_project_item","arguments":{"title":"交报告","assignee":"user","dueAt":"2026-06-25T15:00:00+08:00"}}}`)
	if _, isErr := resultText(t, env); isErr {
		t.Fatalf("create_project_item returned error: %v", env)
	}
	if gotBody["assignee"] != "user" {
		t.Errorf("assignee = %v, want user", gotBody["assignee"])
	}
	if gotBody["scheduleType"] != "scheduled" || gotBody["scheduledAt"] != "2026-06-25T15:00:00+08:00" {
		t.Errorf("schedule fields = %v / %v", gotBody["scheduleType"], gotBody["scheduledAt"])
	}
}

func TestCreateMilestoneToolForwardsBumpOnly(t *testing.T) {
	var gotBody map[string]any
	s, buf, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/agent/milestones" {
			http.Error(w, "unexpected", http.StatusBadRequest)
			return
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.Write([]byte(`{"id":"m1","name":"0.1.0","version":"0.1.0"}`))
	})

	env := call(t, s, buf, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"create_milestone","arguments":{"bump":"minor","description":"foundation"}}}`)
	if text, isErr := resultText(t, env); isErr {
		t.Fatalf("create_milestone returned error: %s", text)
	}
	if gotBody["workspace_id"] != "ws1" || gotBody["bump"] != "minor" ||
		gotBody["description"] != "foundation" {
		t.Fatalf("milestone request body = %v", gotBody)
	}
	if _, ok := gotBody["name"]; ok {
		t.Fatalf("milestone request must not contain legacy name: %v", gotBody)
	}
	if _, ok := gotBody["predecessorId"]; ok {
		t.Fatalf("milestone request must not contain predecessorId: %v", gotBody)
	}

	env = call(t, s, buf, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"create_milestone","arguments":{"name":"legacy"}}}`)
	if text, isErr := resultText(t, env); !isErr || !strings.Contains(text, "bump") {
		t.Fatalf("legacy MCP shape should fail with bump guidance: %v", env)
	}
}

func TestMilestoneCLIUsesBumpAndRejectsDeprecatedName(t *testing.T) {
	var gotBody map[string]any
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/agent/milestones" {
			http.Error(w, "unexpected", http.StatusBadRequest)
			return
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.Write([]byte(`{"id":"m1","name":"0.0.1","version":"0.0.1"}`))
	}))
	defer api.Close()

	home := t.TempDir()
	agentsDir := filepath.Join(home, ".1agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	listenAddr := strings.TrimPrefix(api.URL, "http://")
	if err := os.WriteFile(
		filepath.Join(agentsDir, "daemon.json"),
		[]byte(`{"listen_addr":"`+listenAddr+`"}`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ONEAGENTS_HOME", home)
	t.Setenv("ONEAGENTS_WORKSPACE_ID", "ws-cli")

	oldStdout := os.Stdout
	readOut, writeOut, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writeOut
	code := RunCLI([]string{
		"milestones", "create", "--bump", "patch", "--description", "hotfix",
	})
	_ = writeOut.Close()
	os.Stdout = oldStdout
	output, _ := io.ReadAll(readOut)
	_ = readOut.Close()
	if code != 0 {
		t.Fatalf("milestones create --bump exit = %d", code)
	}
	if !strings.Contains(string(output), `"version": "0.0.1"`) {
		t.Fatalf("milestones create did not return server version: %s", output)
	}
	if gotBody["workspace_id"] != "ws-cli" || gotBody["bump"] != "patch" ||
		gotBody["description"] != "hotfix" {
		t.Fatalf("CLI milestone request body = %v", gotBody)
	}
	if _, ok := gotBody["name"]; ok {
		t.Fatalf("CLI request contains deprecated name: %v", gotBody)
	}
	if code := RunCLI([]string{
		"milestones", "create", "--name", "legacy",
	}); code == 0 {
		t.Fatal("deprecated --name unexpectedly succeeded")
	}
}

func writeTestDaemonFile(t *testing.T, apiURL string) {
	t.Helper()
	home := t.TempDir()
	agentsDir := filepath.Join(home, ".1agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(agentsDir, "daemon.json"),
		[]byte(`{"listen_addr":"`+strings.TrimPrefix(apiURL, "http://")+`"}`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ONEAGENTS_HOME", home)
}

func TestFeatureCatalogCLIMapsAllVerbsAndLocksWorkspace(t *testing.T) {
	type request struct {
		method string
		path   string
		query  string
		body   map[string]any
	}
	var requests []request
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		requests = append(requests, request{
			method: r.Method, path: r.URL.Path, query: r.URL.RawQuery, body: body,
		})
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/agent/feature-catalog" && r.Method == http.MethodGet {
			w.Write([]byte(`{"nodes":[],"links":[]}`))
			return
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer api.Close()
	writeTestDaemonFile(t, api.URL)
	t.Setenv("ONEAGENTS_WORKSPACE_ID", "locked-workspace")

	calls := [][]string{
		{"list"},
		{"create", "--kind", "module", "--title", "Root"},
		{"update", "n1", "--title", "Renamed"},
		{"move", "n1", "--parent", "n2", "--position", "3"},
		{"link", "n1", "--item", "req1", "--relation", "source"},
		{"unlink", "n1", "--item", "req1", "--relation", "source"},
		{"batch", "--json", `[{"op":"create","clientRef":"root","kind":"module","title":"Root"}]`},
	}
	for _, args := range calls {
		if code := RunFeatureCatalogCLI(args); code != 0 {
			t.Fatalf("feature-catalog %v exit = %d", args, code)
		}
	}
	if len(requests) != len(calls) {
		t.Fatalf("requests = %d, want %d: %+v", len(requests), len(calls), requests)
	}
	want := []struct{ method, path string }{
		{http.MethodGet, "/api/agent/feature-catalog"},
		{http.MethodPost, "/api/agent/feature-catalog"},
		{http.MethodPatch, "/api/agent/feature-catalog/n1"},
		{http.MethodPatch, "/api/agent/feature-catalog/n1"},
		{http.MethodPost, "/api/agent/feature-catalog/n1/items"},
		{http.MethodDelete, "/api/agent/feature-catalog/n1/items/req1"},
		{http.MethodPost, "/api/agent/feature-catalog/batch"},
	}
	for i := range want {
		if requests[i].method != want[i].method || requests[i].path != want[i].path {
			t.Fatalf("request %d = %s %s, want %s %s", i, requests[i].method, requests[i].path, want[i].method, want[i].path)
		}
		if i != 0 && i != 5 && requests[i].body["workspace_id"] != "locked-workspace" {
			t.Fatalf("request %d workspace not locked: %+v", i, requests[i].body)
		}
	}
	if !strings.Contains(requests[0].query, "workspace_id=locked-workspace") ||
		!strings.Contains(requests[5].query, "workspace_id=locked-workspace") {
		t.Fatalf("query workspace lock missing: list=%q unlink=%q", requests[0].query, requests[5].query)
	}
}

func TestFeatureCatalogCLIResolvesProjectFromCWDAndReportsAPIFailure(t *testing.T) {
	workspace := t.TempDir()
	nested := filepath.Join(workspace, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	var listWorkspace string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/workspace/list":
			_ = json.NewEncoder(w).Encode([]wsRecord{{ID: "cwd-workspace", Name: "cwd", Path: workspace}})
		case "/api/agent/feature-catalog":
			listWorkspace = r.URL.Query().Get("workspace_id")
			http.Error(w, "catalog unavailable", http.StatusConflict)
		default:
			http.NotFound(w, r)
		}
	}))
	defer api.Close()
	writeTestDaemonFile(t, api.URL)
	t.Setenv("ONEAGENTS_WORKSPACE_ID", "")
	oldCWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(nested); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldCWD)

	if code := RunFeatureCatalogCLI([]string{"list"}); code == 0 {
		t.Fatal("non-200 feature-catalog response unexpectedly succeeded")
	}
	if listWorkspace != "cwd-workspace" {
		t.Fatalf("cwd resolved workspace = %q, want cwd-workspace", listWorkspace)
	}
}

func TestFeatureCatalogMCPToolsForwardLockedRequestsAndFailures(t *testing.T) {
	var seen []string
	s, buf, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path)
		var body map[string]any
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		if r.Method != http.MethodGet && r.Method != http.MethodDelete &&
			body["workspace_id"] != "ws1" {
			http.Error(w, "workspace not locked", http.StatusForbidden)
			return
		}
		if r.URL.Path == "/api/agent/feature-catalog/batch" {
			http.Error(w, "atomic validation failed", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	})
	calls := []string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_feature_catalog","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"create_feature_node","arguments":{"kind":"module","title":"Root"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"update_feature_node","arguments":{"id":"n1","title":"Renamed"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"move_feature_node","arguments":{"id":"n1","position":1}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"link_feature_item","arguments":{"featureId":"n1","itemId":"req","relation":"source"}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"unlink_feature_item","arguments":{"featureId":"n1","itemId":"req","relation":"source"}}}`,
	}
	for _, request := range calls {
		if text, isErr := resultText(t, call(t, s, buf, request)); isErr {
			t.Fatalf("feature MCP call failed: %s", text)
		}
	}
	env := call(t, s, buf, `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"batch_feature_catalog","arguments":{"operations":[{"op":"create","clientRef":"root","kind":"module","title":"Root"}]}}}`)
	if text, isErr := resultText(t, env); !isErr || !strings.Contains(text, "atomic validation failed") {
		t.Fatalf("batch failure not surfaced as MCP tool error: %v", env)
	}
	want := []string{
		"GET /api/agent/feature-catalog",
		"POST /api/agent/feature-catalog",
		"PATCH /api/agent/feature-catalog/n1",
		"PATCH /api/agent/feature-catalog/n1",
		"POST /api/agent/feature-catalog/n1/items",
		"DELETE /api/agent/feature-catalog/n1/items/req",
		"POST /api/agent/feature-catalog/batch",
	}
	if strings.Join(seen, "\n") != strings.Join(want, "\n") {
		t.Fatalf("MCP routes:\n%s\nwant:\n%s", strings.Join(seen, "\n"), strings.Join(want, "\n"))
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
	var gotSessionID, gotSessionToken, gotOrigin string
	s, buf, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/agent/project-items" {
			gotAuth = r.Header.Get("Authorization")
			gotSessionID = r.Header.Get("X-OneAgents-Session-ID")
			gotSessionToken = r.Header.Get("X-OneAgents-Session-Token")
			gotOrigin = r.Header.Get("X-OneAgents-Origin")
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &gotBody)
			w.Write([]byte(`{"id":"t9","number":9,"title":"New","status":"pending","dependsOn":["t1"],"sessionId":"session-1","turnId":"turn-1","eventId":"event-1"}`))
			return
		}
		http.Error(w, "unexpected", 400)
	})
	s.api.sessionID = "session-1"
	s.api.sessionToken = "signed"
	s.api.origin = "mcp"

	env := call(t, s, buf, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"create_project_item","arguments":{"title":"New","dependsOn":["t1"],"type":"requirement"}}}`)
	text, isErr := resultText(t, env)
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	for _, want := range []string{`"sessionId": "session-1"`, `"turnId": "turn-1"`, `"eventId": "event-1"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("MCP create response missing %s: %s", want, text)
		}
	}
	if gotAuth != "Bearer tok" {
		t.Fatalf("auth header = %q", gotAuth)
	}
	if gotSessionID != "session-1" || gotSessionToken != "signed" || gotOrigin != "mcp" {
		t.Fatalf("attribution headers: session=%q token=%q origin=%q", gotSessionID, gotSessionToken, gotOrigin)
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
		if r.Method == http.MethodPost && r.URL.Path == "/api/agent/project-items" {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &gotBody)
			w.Write([]byte(`{"id":"t9","number":9,"title":"New","status":"pending"}`))
			return
		}
		http.Error(w, "unexpected", 400)
	})

	env := call(t, s, buf, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"create_project_item","arguments":{"title":"New","assignee":"codex","githubAssignees":["alice","bob"],"githubRepo":"o/r","githubKind":"issue","githubNumber":74}}}`)
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

func TestCreateTaskForwardsFeatureContext(t *testing.T) {
	var gotBody map[string]any
	s, buf, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/agent/project-items" {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &gotBody)
			w.Write([]byte(`{"id":"t10","number":10,"title":"Deliver feature","status":"pending"}`))
			return
		}
		http.Error(w, "unexpected", http.StatusBadRequest)
	})

	env := call(t, s, buf, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"create_project_item","arguments":{"title":"Deliver feature","type":"task","acceptanceCriteria":"done","featureId":"feature-1","links":[{"target":"requirement-1","rel":"relates"}]}}}`)
	if text, isErr := resultText(t, env); isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	if gotBody["featureId"] != "feature-1" {
		t.Fatalf("featureId not forwarded: %v", gotBody)
	}
	links, _ := gotBody["links"].([]any)
	if len(links) != 1 {
		t.Fatalf("top-level requirement link not forwarded: %v", gotBody["links"])
	}
}

func TestCreateTaskCLIForwardsFeatureContext(t *testing.T) {
	var gotBody map[string]any
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/agent/project-items" {
			http.Error(w, "unexpected", http.StatusBadRequest)
			return
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.Write([]byte(`{"id":"t11","number":11,"title":"CLI delivery","status":"pending"}`))
	}))
	defer api.Close()
	writeTestDaemonFile(t, api.URL)
	t.Setenv("ONEAGENTS_WORKSPACE_ID", "feature-workspace")

	code := RunCLI([]string{
		"create",
		"--json",
		`{"title":"CLI delivery","type":"task","acceptanceCriteria":"done","featureId":"feature-2","links":[{"target":"requirement-2","rel":"relates"}]}`,
	})
	if code != 0 {
		t.Fatalf("CLI create exit = %d", code)
	}
	if gotBody["workspace_id"] != "feature-workspace" || gotBody["featureId"] != "feature-2" {
		t.Fatalf("feature-scoped CLI body = %v", gotBody)
	}
	links, _ := gotBody["links"].([]any)
	if len(links) != 1 {
		t.Fatalf("CLI top-level requirement link not forwarded: %v", gotBody["links"])
	}
}

func TestGetTaskRejectsForeignId(t *testing.T) {
	s, buf, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/agent/project-items" {
			w.Write([]byte(twoTasks()))
			return
		}
		t.Errorf("get_project_item must not hit single-task endpoint for a foreign id (path %s)", r.URL.Path)
	})
	env := call(t, s, buf, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"get_project_item","arguments":{"id":"foreign"}}}`)
	text, isErr := resultText(t, env)
	if !isErr {
		t.Fatalf("expected lock error, got: %s", text)
	}
}

func TestGetTaskGraphMapsToGraphEndpoint(t *testing.T) {
	const graphJSON = `{"outgoing":[{"rel":"relates","task":{"id":"t9","number":9}}],"incoming":[]}`
	s, buf, _ := newTaskScopedServer(t, "t1", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/agent/project-items/t1/graph" {
			t.Errorf("get_project_item_graph hit wrong path %s", r.URL.Path)
		}
		w.Write([]byte(graphJSON))
	})
	env := call(t, s, buf, `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"get_project_item_graph","arguments":{"id":"t1"}}}`)
	text, isErr := resultText(t, env)
	if isErr {
		t.Fatalf("get_project_item_graph errored: %s", text)
	}
	if text != graphJSON {
		t.Fatalf("get_project_item_graph body = %s, want passthrough of graph JSON", text)
	}

	// A foreign id is rejected before any API call (task-scoped lock).
	s2, buf2, _ := newTaskScopedServer(t, "t1", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("get_project_item_graph must not hit API for a foreign id (path %s)", r.URL.Path)
	})
	env = call(t, s2, buf2, `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"get_project_item_graph","arguments":{"id":"other"}}}`)
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
		t.Errorf("create_project_item must not hit the API in a task-scoped session (path %s)", r.URL.Path)
	})
	env := call(t, s, buf, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"create_project_item","arguments":{"title":"X"}}}`)
	if text, isErr := resultText(t, env); !isErr {
		t.Fatalf("expected task-scope rejection, got: %s", text)
	}
}

func TestTaskScopedRejectsForeignTask(t *testing.T) {
	s, buf, _ := newTaskScopedServer(t, "t1", func(w http.ResponseWriter, r *http.Request) {
		// A foreign id must be rejected before any single-task API call.
		t.Errorf("must not hit API for a foreign id (path %s)", r.URL.Path)
	})
	// get_project_item on the other task
	env := call(t, s, buf, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"get_project_item","arguments":{"id":"t2"}}}`)
	if text, isErr := resultText(t, env); !isErr {
		t.Fatalf("get_project_item foreign: expected rejection, got: %s", text)
	}
	// update_project_item on the other task
	env = call(t, s, buf, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"update_project_item","arguments":{"id":"t2","status":"completed"}}}`)
	if text, isErr := resultText(t, env); !isErr {
		t.Fatalf("update_project_item foreign: expected rejection, got: %s", text)
	}
}

func TestTaskScopedListFiltersToSelf(t *testing.T) {
	s, buf, _ := newTaskScopedServer(t, "t1", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(twoTasks()))
	})
	env := call(t, s, buf, `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"list_project_items","arguments":{}}}`)
	text, isErr := resultText(t, env)
	if isErr {
		t.Fatalf("unexpected error: %s", text)
	}
	var payload struct {
		Count int `json:"count"`
		Tasks []struct {
			ID string `json:"id"`
		} `json:"items"`
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
		if r.Method == http.MethodPatch && r.URL.Path == "/api/agent/project-items/t1" {
			patched = true
			w.Write([]byte(`{"id":"t1","priority":"high"}`))
			return
		}
		http.Error(w, "unexpected", 400)
	})
	// A non-status edit on the executor's own task is allowed.
	env := call(t, s, buf, `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"update_project_item","arguments":{"id":"t1","priority":"high"}}}`)
	if text, isErr := resultText(t, env); isErr {
		t.Fatalf("own-task update should succeed, got error: %s", text)
	}
	if !patched {
		t.Fatal("expected PATCH /api/agent/project-items/t1")
	}
}

// TestExecutorCannotSelfReportCompleted is the #132 guardrail: an executor
// (default task scope) may not write status=completed — completion is driven
// by the artifact/verification path, not by the agent self-reporting done. The
// PATCH must never reach the API.
func TestExecutorCannotSelfReportCompleted(t *testing.T) {
	s, buf, _ := newTaskScopedServer(t, "t1", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("update_project_item(status=completed) must not hit the API in executor scope (path %s)", r.URL.Path)
	})
	env := call(t, s, buf, `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"update_project_item","arguments":{"id":"t1","status":"completed"}}}`)
	if text, isErr := resultText(t, env); !isErr {
		t.Fatalf("executor status=completed should be rejected, got: %s", text)
	}
}

// TestExecutorCanCancelOwnTask confirms the guardrail is narrow: an executor may
// still give up by setting status=cancelled (this is not a false completion).
func TestExecutorCanCancelOwnTask(t *testing.T) {
	var patched bool
	s, buf, _ := newTaskScopedServer(t, "t1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && r.URL.Path == "/api/agent/project-items/t1" {
			patched = true
			w.Write([]byte(`{"id":"t1","status":"cancelled"}`))
			return
		}
		http.Error(w, "unexpected", 400)
	})
	env := call(t, s, buf, `{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"update_project_item","arguments":{"id":"t1","status":"cancelled"}}}`)
	if text, isErr := resultText(t, env); isErr {
		t.Fatalf("executor status=cancelled should succeed, got error: %s", text)
	}
	if !patched {
		t.Fatal("expected PATCH /api/agent/project-items/t1 for cancel")
	}
}

// TestExecutorUpdateTaskDoesNotAdvertiseCompleted asserts the executor-scoped
// update_project_item schema offers only `cancelled` as a settable status, so the agent
// is never told to try a completion the server will reject (#132).
func TestExecutorUpdateTaskDoesNotAdvertiseCompleted(t *testing.T) {
	s, buf, _ := newTaskScopedServer(t, "t1", func(w http.ResponseWriter, r *http.Request) {})
	env := call(t, s, buf, `{"jsonrpc":"2.0","id":9,"method":"tools/list"}`)
	res, _ := env["result"].(map[string]any)
	tools, _ := res["tools"].([]any)
	var found bool
	for _, tl := range tools {
		m, _ := tl.(map[string]any)
		if m["name"] != "update_project_item" {
			continue
		}
		found = true
		schema, _ := m["inputSchema"].(map[string]any)
		props, _ := schema["properties"].(map[string]any)
		statusProp, _ := props["status"].(map[string]any)
		enum, _ := statusProp["enum"].([]any) // JSON round-trip → []any
		if len(enum) != 1 || enum[0] != "cancelled" {
			t.Fatalf("executor update_project_item status enum = %v, want [cancelled]", statusProp["enum"])
		}
	}
	if !found {
		t.Fatal("update_project_item not advertised in executor scope")
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
	if len(got) != 4 || !got["list_project_items"] || !got["get_project_item"] || !got["get_project_item_graph"] || !got["submit_review"] {
		t.Fatalf("verifier scope tools = %v, want {list_project_items, get_project_item, get_project_item_graph, submit_review}", got)
	}
	if got["update_project_item"] {
		t.Error("update_project_item must NOT be advertised to a verifier (hard read-only)")
	}
	// #205: verifier must not see write-class mail tools
	for _, deny := range []string{"send_mail", "accept_mail", "archive_mail"} {
		if got[deny] {
			t.Errorf("verifier must NOT have write mail tool %q", deny)
		}
	}
}

// ── Workspace Inbox mail tools (#205) ───────────────────────────────────────

func TestMailToolsSendCheckAccept(t *testing.T) {
	// Simulate A (ws1) send_mail → deliver to B (ws-b); B would check/accept via
	// separate sessions. Here we assert wire shapes on one MCP server locked to ws1.
	var deliverBody map[string]any
	s, buf, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/inbox/deliver":
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &deliverBody)
			w.Write([]byte(`{"id":"m1","workspaceId":"ws-b","source":"agent","fromWorkspaceId":"ws1","title":"竞品摘要","status":"unread"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/inbox":
			if r.URL.Query().Get("workspaceId") != "ws1" {
				http.Error(w, "wrong workspace", 400)
				return
			}
			w.Write([]byte(`{"items":[{"id":"m0","workspaceId":"ws1","title":"本箱信","status":"unread"}],"unread":1}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/inbox/m0/accept":
			var acc map[string]any
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &acc)
			if acc["workspaceId"] != "ws1" {
				http.Error(w, "accept must pin current workspace", 400)
				return
			}
			w.Write([]byte(`{"task":{"id":"req1","type":"requirement","title":"本箱信","labels":["dispatched-from:m0"]}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/inbox/targets":
			w.Write([]byte(`{"targets":[{"projectId":"ws-b","name":"判定助理"}]}`))
		default:
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, 400)
		}
	})

	// list targets
	env := call(t, s, buf, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_mail_targets","arguments":{}}}`)
	if text, isErr := resultText(t, env); isErr {
		t.Fatalf("list_mail_targets: %s", text)
	}

	// send: from must be forced to ws1, source=agent
	env = call(t, s, buf, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"send_mail","arguments":{"toWorkspaceId":"ws-b","title":"竞品摘要","content":"对方上了 X","fromRef":"collector"}}}`)
	if text, isErr := resultText(t, env); isErr {
		t.Fatalf("send_mail: %s", text)
	}
	if deliverBody["workspaceId"] != "ws-b" {
		t.Fatalf("deliver target = %v, want ws-b", deliverBody["workspaceId"])
	}
	if deliverBody["fromWorkspaceId"] != "ws1" {
		t.Fatalf("from forced to current ws, got %v", deliverBody["fromWorkspaceId"])
	}
	if deliverBody["source"] != "agent" {
		t.Fatalf("source = %v, want agent", deliverBody["source"])
	}

	// check own inbox
	env = call(t, s, buf, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"check_inbox","arguments":{}}}`)
	text, isErr := resultText(t, env)
	if isErr {
		t.Fatalf("check_inbox: %s", text)
	}
	if !strings.Contains(text, "m0") {
		t.Fatalf("check_inbox should list m0: %s", text)
	}

	// accept
	env = call(t, s, buf, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"accept_mail","arguments":{"id":"m0"}}}`)
	if text, isErr := resultText(t, env); isErr {
		t.Fatalf("accept_mail: %s", text)
	} else if !strings.Contains(text, "requirement") {
		t.Fatalf("accept should return requirement: %s", text)
	}
}

func TestVerifierBlocksSendMail(t *testing.T) {
	s, buf, _ := newVerifierScopedServer(t, "t1", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("send_mail must not hit API in verifier scope: %s", r.URL.Path)
	})
	env := call(t, s, buf, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"send_mail","arguments":{"toWorkspaceId":"x","title":"nope"}}}`)
	if text, isErr := resultText(t, env); !isErr {
		t.Fatalf("expected verifier send_mail rejection, got: %s", text)
	}
}

func TestVerifierScopeBlocksUpdateTask(t *testing.T) {
	s, buf, _ := newVerifierScopedServer(t, "t1", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("update_project_item must not hit the API in verifier scope (path %s)", r.URL.Path)
	})
	env := call(t, s, buf, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"update_project_item","arguments":{"id":"t1","status":"completed"}}}`)
	if text, isErr := resultText(t, env); !isErr {
		t.Fatalf("expected verifier update_project_item rejection, got: %s", text)
	}
}

func TestVerifierSubmitReviewPostsVerdict(t *testing.T) {
	var posted bool
	s, buf, _ := newVerifierScopedServer(t, "t1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/agent/project-items/t1/review" {
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
		t.Fatal("expected POST /api/agent/project-items/t1/review")
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
