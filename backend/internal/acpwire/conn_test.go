package acpwire

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestACPChatRoundTrip(t *testing.T) {
	errors := make(chan error, 1)
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/agents/codex" {
			t.Errorf("path: %s", r.URL.Path)
		}
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			errors <- err
			return
		}
		defer ws.Close()
		read := func(method string) object {
			t.Helper()
			var f object
			if err := ws.ReadJSON(&f); err != nil {
				errors <- err
				return nil
			}
			if f["jsonrpc"] != "2.0" || f["method"] != method || f["action"] != nil {
				t.Errorf("unexpected wire message: %v", f)
			}
			return f
		}
		reply := func(f object, result object) {
			if err := ws.WriteJSON(object{"jsonrpc": "2.0", "id": f["id"], "result": result}); err != nil {
				errors <- err
			}
		}
		init := read("initialize")
		reply(init, object{"protocolVersion": 1})
		create := read("session/new")
		if obj(create["params"])["cwd"] != "/tmp" {
			t.Error("cwd missing")
		}
		reply(create, object{"sessionId": "wire-session"})
		prompt := read("session/prompt")
		p := obj(prompt["params"])
		if p["sessionId"] != "wire-session" {
			t.Error("session mapping missing")
		}
		if len(p["prompt"].([]any)) != 2 {
			t.Error("attachment dropped")
		}
		_ = ws.WriteJSON(object{"jsonrpc": "2.0", "id": "permission-1", "method": "session/request_permission", "params": object{"sessionId": "wire-session", "toolCall": object{"toolCallId": "t1", "title": "Write"}, "options": []any{object{"optionId": "yes", "kind": "allow_once", "name": "Allow"}}, "_meta": object{"1agents": object{"requestId": "logical-1"}}}})
		var approval object
		if err := ws.ReadJSON(&approval); err != nil {
			errors <- err
			return
		}
		if approval["id"] != "permission-1" || obj(obj(approval["result"])["outcome"])["optionId"] != "yes" {
			t.Errorf("bad approval response: %v", approval)
		}
		_ = ws.WriteJSON(object{"jsonrpc": "2.0", "method": "session/update", "params": object{"sessionId": "wire-session", "update": object{"sessionUpdate": "agent_message_chunk", "content": object{"type": "text", "text": "hello"}}}})
		reply(prompt, object{"stopReason": "end_turn"})
		errors <- nil
	}))
	defer server.Close()
	c, err := Dial("ws"+strings.TrimPrefix(server.URL, "http"), "codex")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err = c.WriteJSON(object{"action": "ensure_session", "sessionId": "local-session", "workspacePath": "/tmp", "agentType": "codex"}); err != nil {
		t.Fatal(err)
	}
	var event object
	if err = c.ReadJSON(&event); err != nil {
		t.Fatal(err)
	}
	if event["event"] != "session_ready" || event["sessionId"] != "local-session" || event["agentSessionId"] != "wire-session" {
		t.Fatalf("ready: %v", event)
	}
	if err = c.WriteJSON(object{"action": "prompt", "sessionId": "local-session", "text": "hi", "attachments": []any{object{"mediaType": "image/png", "data": "AA=="}}}); err != nil {
		t.Fatal(err)
	}
	if err = c.ReadJSON(&event); err != nil {
		t.Fatal(err)
	}
	if event["event"] != "permission_request" {
		t.Fatalf("permission: %v", event)
	}
	if err = c.WriteJSON(object{"action": "respond_permission", "sessionId": "local-session", "requestId": event["requestId"], "behavior": "allow_once"}); err != nil {
		t.Fatal(err)
	}
	if err = c.ReadJSON(&event); err != nil {
		t.Fatal(err)
	}
	if event["event"] != "text_delta" || event["text"] != "hello" {
		t.Fatalf("text: %v", event)
	}
	if err = c.ReadJSON(&event); err != nil {
		t.Fatal(err)
	}
	if event["event"] != "done" {
		t.Fatalf("done: %v", event)
	}
	if err = <-errors; err != nil {
		t.Fatal(err)
	}
}

func TestDecodeManagedTerminalIsNotDuplicated(t *testing.T) {
	c := &Conn{pending: map[string]pending{key("p"): {action: "prompt", local: "local"}}, locals: map[string]string{"wire": "local"}, interactive: map[string]interaction{}}
	event, err := c.decode(object{"jsonrpc": "2.0", "method": "_1agents/events/turn_terminal", "params": object{"sessionId": "wire", "turnId": "turn", "status": "completed"}})
	if err != nil || event["event"] != "turn_terminal" || event["sessionId"] != "local" {
		t.Fatalf("terminal: %v %v", event, err)
	}
	event, err = c.decode(object{"jsonrpc": "2.0", "id": "p", "result": object{"stopReason": "end_turn", "_meta": object{"1agents": object{"turnId": "turn"}}}})
	if err != nil || event != nil {
		t.Fatalf("duplicate terminal: %v %v", event, err)
	}
}

func TestDecodeExtensionsPreservesViewFields(t *testing.T) {
	c := &Conn{locals: map[string]string{"wire": "local"}, interactive: map[string]interaction{}}
	var frame object
	_ = json.Unmarshal([]byte(`{"jsonrpc":"2.0","method":"_1agents/events/turn_sync","params":{"sessionId":"wire","sequence":4,"queued":[{"turnId":"t"}],"future":{"keep":true}}}`), &frame)
	got, err := c.decode(frame)
	if err != nil || got["event"] != "turn_sync" || got["future"] == nil || got["queued"] == nil {
		t.Fatalf("lost extension state: %v %v", got, err)
	}
}
