package mcptasks

import (
	"bufio"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// runStdio feeds reqs (one JSON-RPC line each) through mcptasks.Run() over real
// os.Stdin/os.Stdout pipes, with env wired to a fake daemon, and returns the
// raw response lines. lockedTask, when non-empty, sets ONEAGENTS_TASK_ID.
func runStdio(t *testing.T, lockedTask string, reqs []string) []string {
	t.Helper()

	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/agent/tasks" && r.Method == http.MethodGet:
			w.Write([]byte(twoTasks()))
		case r.URL.Path == "/api/agent/tasks/t1" && r.Method == http.MethodGet:
			w.Write([]byte(`{"id":"t1","number":1,"title":"A","status":"pending","type":"task","description":"do the thing","acceptanceCriteria":"it works"}`))
		case strings.HasPrefix(r.URL.Path, "/api/agent/tasks/") && r.Method == http.MethodPatch:
			w.Write([]byte(`{"id":"t1","status":"completed"}`))
		default:
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, 400)
		}
	}))
	t.Cleanup(daemon.Close)

	t.Setenv("ONEAGENTS_BASE_URL", daemon.URL)
	t.Setenv("ONEAGENTS_WORKSPACE_ID", "ws1")
	t.Setenv("ONEAGENTS_INTERNAL_TOKEN", "tok")
	if lockedTask != "" {
		t.Setenv("ONEAGENTS_TASK_ID", lockedTask)
	}

	// Swap os.Stdin/os.Stdout for pipes around Run().
	inR, inW, _ := os.Pipe()
	outR, outW, _ := os.Pipe()
	origIn, origOut := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = inR, outW
	t.Cleanup(func() { os.Stdin, os.Stdout = origIn, origOut })

	go func() {
		w := bufio.NewWriter(inW)
		for _, line := range reqs {
			w.WriteString(line)
			w.WriteByte('\n')
		}
		w.Flush()
		inW.Close() // EOF ends Run()'s loop
	}()

	done := make(chan []byte)
	go func() {
		var buf bytes.Buffer
		buf.ReadFrom(outR)
		done <- buf.Bytes()
	}()

	if err := Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	outW.Close()
	raw := <-done

	var lines []string
	for _, l := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

func TestInterfacePMScope(t *testing.T) {
	reqs := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"get_task","arguments":{"id":"t1"}}}`,
	}
	resp := runStdio(t, "", reqs)
	t.Log("───────── PM (project-wide) scope ─────────")
	for i, req := range reqs {
		t.Logf("→ IN  : %s", req)
		if i < len(resp) {
			t.Logf("← OUT : %s", resp[i])
		}
	}
	if len(resp) != 3 {
		t.Fatalf("expected 3 responses, got %d", len(resp))
	}
	// PM advertises the create_* tools and get_task returns the full body.
	if !strings.Contains(resp[1], `"name":"create_task"`) || !strings.Contains(resp[1], `"name":"create_milestone"`) {
		t.Error("PM tools/list should include create_task and create_milestone")
	}
	if !strings.Contains(resp[2], "acceptanceCriteria") {
		t.Error("PM get_task should return the full task body")
	}
}

func TestInterfaceExecutorScope(t *testing.T) {
	reqs := []string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"create_task","arguments":{"title":"sneak"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"get_task","arguments":{"id":"t2"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"update_task","arguments":{"id":"t1","status":"completed"}}}`,
	}
	resp := runStdio(t, "t1", reqs)
	t.Log("───────── Executor scope (ONEAGENTS_TASK_ID=t1) ─────────")
	for i, req := range reqs {
		t.Logf("→ IN  : %s", req)
		if i < len(resp) {
			t.Logf("← OUT : %s", resp[i])
		}
	}
	if len(resp) != 4 {
		t.Fatalf("expected 4 responses, got %d", len(resp))
	}
	if strings.Contains(resp[0], `"name":"create_task"`) || strings.Contains(resp[0], `"name":"create_milestone"`) {
		t.Error("executor tools/list must NOT advertise create_* tools")
	}
	if !strings.Contains(resp[1], `"isError":true`) { // create_task blocked
		t.Error("executor create_task should be rejected")
	}
	if !strings.Contains(resp[2], `"isError":true`) { // foreign get_task rejected
		t.Error("executor get_task on a foreign id should be rejected")
	}
	if strings.Contains(resp[3], `"isError":true`) { // own update_task allowed
		t.Error("executor update_task on its own task should succeed")
	}
}
