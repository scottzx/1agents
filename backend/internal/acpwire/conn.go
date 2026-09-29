// Package acpwire connects the application's chat state to ACP v1 JSON-RPC.
// The application-facing messages are local view data, never network envelopes.
package acpwire

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type object = map[string]any
type pending struct {
	action, local string
	input         object
}
type interaction struct {
	id      any
	method  string
	options []any
}

// Conn serializes writes and translates the chat application's messages at the
// service boundary. Exactly one goroutine reads, as required by gorilla/websocket.
type Conn struct {
	*websocket.Conn
	mu          sync.Mutex
	next        uint64
	pending     map[string]pending
	sessions    map[string]string
	locals      map[string]string
	interactive map[string]interaction
	local       string
}

func Dial(address, agent string) (*Conn, error) {
	u, err := url.Parse(address)
	if err != nil {
		return nil, err
	}
	u.Path = "/agents/" + agent
	ws, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return nil, err
	}
	c := &Conn{Conn: ws, pending: map[string]pending{}, sessions: map[string]string{}, locals: map[string]string{}, interactive: map[string]interaction{}}
	_ = ws.SetReadDeadline(time.Now().Add(15 * time.Second))
	err = ws.WriteJSON(object{"jsonrpc": "2.0", "id": "init", "method": "initialize", "params": object{"protocolVersion": 1, "clientCapabilities": object{"_meta": object{"1agents": object{"version": 1}}}, "clientInfo": object{"name": "1agents", "version": "1"}}})
	if err == nil {
		var reply object
		err = ws.ReadJSON(&reply)
		if err == nil && (reply["jsonrpc"] != "2.0" || reply["id"] != "init" || reply["error"] != nil || obj(reply["result"])["protocolVersion"] != float64(1)) {
			err = fmt.Errorf("ACP initialization failed: %v", reply["error"])
		}
	}
	if err != nil {
		_ = ws.Close()
		return nil, err
	}
	_ = ws.SetReadDeadline(time.Time{})
	return c, nil
}
func obj(v any) object {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return object{}
}
func str(v any) string     { s, _ := v.(string); return s }
func key(v any) string     { b, _ := json.Marshal(v); return string(b) }
func meta(v object) object { return obj(obj(v["_meta"])["1agents"]) }
func (c *Conn) WriteJSON(value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.WriteMessage(websocket.TextMessage, raw)
}
func (c *Conn) WriteMessage(kind int, raw []byte) error {
	if kind != websocket.TextMessage {
		return fmt.Errorf("ACP requires JSON text messages")
	}
	var input object
	if err := json.Unmarshal(raw, &input); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	action, local := str(input["action"]), str(input["sessionId"])
	wire := c.sessions[local]
	if wire == "" {
		wire = local
	}
	if strings.HasPrefix(action, "respond_") {
		requestKey := str(input["requestId"])
		item, ok := c.interactive[requestKey]
		if !ok {
			return fmt.Errorf("interaction no longer pending: %s", requestKey)
		}
		result := object{}
		if action == "respond_permission" {
			behavior := str(input["behavior"])
			if behavior == "allow" {
				behavior = "allow_once"
			}
			if behavior == "deny" {
				behavior = "reject_once"
			}
			result["outcome"] = object{"outcome": "cancelled"}
			for _, option := range item.options {
				o := obj(option)
				if o["kind"] == behavior {
					result["outcome"] = object{"outcome": "selected", "optionId": o["optionId"]}
					break
				}
			}
		} else {
			for k, v := range input {
				if k != "action" && k != "sessionId" && k != "requestId" {
					result[k] = v
				}
			}
		}
		if err := c.Conn.WriteJSON(object{"jsonrpc": "2.0", "id": item.id, "result": result}); err != nil {
			return err
		}
		delete(c.interactive, requestKey)
		return nil
	}
	method := ""
	params := object{"sessionId": wire}
	switch action {
	case "ensure_session":
		c.local = local
		params = object{"cwd": input["workspacePath"], "mcpServers": []any{}}
		if servers, ok := input["mcpServers"]; ok {
			params["mcpServers"] = servers
		}
		custom := object{}
		for _, k := range []string{"systemContext", "permissionMode", "responsePolicy", "env", "launch"} {
			if v, ok := input[k]; ok {
				custom[k] = v
			}
		}
		params["_meta"] = object{"1agents": custom}
		if known := c.sessions[local]; known != "" {
			method = "session/resume"
			params["sessionId"] = known
		} else if saved := str(input["acpSessionId"]); saved != "" {
			method = "session/resume"
			params["sessionId"] = saved
		} else {
			method = "session/new"
		}
	case "prompt":
		method = "session/prompt"
		blocks := []any{object{"type": "text", "text": input["text"]}}
		if attachments, ok := input["attachments"].([]any); ok {
			for _, a := range attachments {
				m := obj(a)
				t := "image"
				if strings.HasPrefix(str(m["mediaType"]), "audio/") {
					t = "audio"
				}
				blocks = append(blocks, object{"type": t, "mimeType": m["mediaType"], "data": m["data"]})
			}
		}
		params["prompt"] = blocks
		custom := object{}
		for _, k := range []string{"turnId", "requestId", "turnManaged", "responsePolicy"} {
			if v, ok := input[k]; ok {
				custom[k] = v
			}
		}
		params["_meta"] = object{"1agents": custom}
	case "cancel_turn":
		return c.Conn.WriteJSON(object{"jsonrpc": "2.0", "method": "session/cancel", "params": params})
	case "close_session":
		method = "session/close"
	case "delete_session":
		method = "session/delete"
	case "close_all_sessions":
		method = "_1agents/sessions/close_all"
		params = object{}
	case "set_session_mode":
		method = "session/set_mode"
		params["modeId"] = obj(input["payload"])["modeId"]
	case "set_config_option":
		method = "session/set_config_option"
		params["configId"] = obj(input["payload"])["key"]
		params["value"] = obj(input["payload"])["value"]
	case "list_sessions":
		method = "session/list"
		delete(params, "sessionId")
		if cwd := input["workspacePath"]; cwd != nil {
			params["cwd"] = cwd
		}
	default:
		names := map[string]string{"get_history": "history", "set_permission_mode": "permission_mode", "cancel_queued": "cancel_queued", "fork_session": "fork", "delete_session": "delete", "authenticate": "authenticate", "logout": "logout"}
		name, ok := names[action]
		if !ok {
			return fmt.Errorf("unsupported ACP application command %q", action)
		}
		method = "_1agents/session/" + name
		for k, v := range input {
			if k != "action" && k != "sessionId" {
				params[k] = v
			}
		}
	}
	c.next++
	id := fmt.Sprintf("client-%d", c.next)
	c.pending[key(id)] = pending{action: action, local: local, input: params}
	if err := c.Conn.WriteJSON(object{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		delete(c.pending, key(id))
		return err
	}
	return nil
}
func (c *Conn) ReadJSON(value any) error {
	_, raw, err := c.ReadMessage()
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, value)
}
func (c *Conn) ReadMessage() (int, []byte, error) {
	for {
		_, raw, err := c.Conn.ReadMessage()
		if err != nil {
			return 0, nil, err
		}
		var frame object
		if err = json.Unmarshal(raw, &frame); err != nil {
			return 0, nil, err
		}
		if frame["jsonrpc"] != "2.0" {
			return 0, nil, fmt.Errorf("non-ACP message from service")
		}
		c.mu.Lock()
		event, err := c.decode(frame)
		c.mu.Unlock()
		if err != nil {
			return 0, nil, err
		}
		if event == nil {
			continue
		}
		encoded, err := json.Marshal(event)
		return websocket.TextMessage, encoded, err
	}
}
func (c *Conn) localID(wire string) string {
	if local := c.locals[wire]; local != "" {
		return local
	}
	if c.local != "" {
		return c.local
	}
	return wire
}
func (c *Conn) decode(frame object) (object, error) {
	method := str(frame["method"])
	if method != "" {
		p := obj(frame["params"])
		wire := str(p["sessionId"])
		local := c.localID(wire)
		if method == "session/request_permission" || strings.HasPrefix(method, "_x.ai/") {
			m := meta(p)
			requestKey := str(m["requestId"])
			if requestKey == "" {
				requestKey = str(p["requestId"])
			}
			if requestKey == "" {
				requestKey = key(frame["id"])
			}
			item := interaction{id: frame["id"], method: method}
			item.options, _ = p["options"].([]any)
			c.interactive[requestKey] = item
			event := object{}
			for k, v := range p {
				if k != "_meta" {
					event[k] = v
				}
			}
			for k, v := range m {
				event[k] = v
			}
			event["sessionId"] = local
			event["requestId"] = requestKey
			if method == "session/request_permission" {
				event["event"] = "permission_request"
				tool := obj(p["toolCall"])
				event["toolCallId"] = tool["toolCallId"]
				event["toolName"] = tool["title"]
				event["arguments"] = tool["rawInput"]
			} else {
				event["event"] = strings.TrimPrefix(method, "_x.ai/")
			}
			return event, nil
		}
		if method == "session/update" {
			u := obj(p["update"])
			event := meta(u)
			event["sessionId"] = local
			switch str(u["sessionUpdate"]) {
			case "agent_message_chunk", "agent_thought_chunk":
				event["event"] = "text_delta"
				event["text"] = obj(u["content"])["text"]
				event["type"] = "output"
				if u["sessionUpdate"] == "agent_thought_chunk" {
					event["type"] = "thought"
				}
			case "tool_call", "tool_call_update":
				if u["rawOutput"] != nil {
					event["event"] = "tool_result"
					event["text"] = u["rawOutput"]
					event["isError"] = u["status"] == "failed"
				} else {
					event["event"] = "tool_call"
					if title, ok := u["title"]; ok {
						event["toolName"] = title
					}
					event["arguments"] = u["rawInput"]
				}
				event["toolCallId"] = u["toolCallId"]
				event["status"] = u["status"]
			case "current_mode_update":
				event["event"] = "mode_changed"
				event["payload"] = object{"currentModeId": u["currentModeId"]}
			case "config_option_update":
				event["event"] = "session_meta"
				event["payload"] = object{"configOptions": u["configOptions"]}
			case "available_commands_update":
				event["event"] = "available_commands_update"
				event["payload"] = object{"availableCommands": u["availableCommands"]}
			case "plan":
				event["event"] = "plan"
				event["payload"] = object{"entries": u["entries"]}
			default:
				return nil, nil
			}
			return event, nil
		}
		if method == "_1agents/session/meta" {
			p["event"] = "session_meta"
			p["sessionId"] = local
			return p, nil
		}
		if strings.HasPrefix(method, "_1agents/events/") {
			name := strings.TrimPrefix(method, "_1agents/events/")
			p["sessionId"] = local
			if meta(p)["viewType"] == true || name == "sessions_list" || name == "session_forked" || name == "session_deleted" || name == "auth_completed" {
				p["type"] = name
			} else {
				p["event"] = name
			}
			if strings.HasSuffix(name, "_timeout") {
				delete(c.interactive, str(p["requestId"]))
			}
			return p, nil
		}
		if _, ok := frame["id"]; ok {
			return nil, c.Conn.WriteJSON(object{"jsonrpc": "2.0", "id": frame["id"], "error": object{"code": -32601, "message": "Method not found"}})
		}
		return nil, nil
	}
	id := key(frame["id"])
	request, ok := c.pending[id]
	if !ok {
		return nil, nil
	}
	delete(c.pending, id)
	if failure := obj(frame["error"]); len(failure) > 0 {
		// Explicit migration of sessions saved before ACP wire IDs existed. The
		// service's import method resumes that native ID under a new ACP identity.
		if request.action == "ensure_session" && failure["code"] == float64(-32001) {
			c.next++
			next := fmt.Sprintf("client-%d", c.next)
			c.pending[key(next)] = pending{action: "import_session", local: request.local, input: request.input}
			return nil, c.Conn.WriteJSON(object{"jsonrpc": "2.0", "id": next, "method": "_1agents/session/import", "params": request.input})
		}
		return object{"event": "error", "sessionId": request.local, "message": failure["message"], "code": obj(failure["data"])["code"]}, nil
	}
	result := obj(frame["result"])
	switch request.action {
	case "ensure_session", "import_session":
		wire := str(result["sessionId"])
		if wire == "" {
			wire = str(request.input["sessionId"])
		}
		c.sessions[request.local] = wire
		c.locals[wire] = request.local
		return object{"event": "session_ready", "sessionId": request.local, "agentSessionId": wire, "turnProtocolVersion": 3}, nil
	case "prompt":
		event := meta(result)
		if event["turnId"] != nil {
			return nil, nil
		} // managed turn_terminal already delivered
		event["event"] = "done"
		event["sessionId"] = request.local
		event["stopped"] = result["stopReason"] == "cancelled"
		return event, nil
	case "list_sessions":
		rows, _ := result["sessions"].([]any)
		sessions := make([]any, 0, len(rows))
		for _, row := range rows {
			s := obj(row)
			view := meta(s)
			for k, v := range (object{"id": s["sessionId"], "acpxRecordId": s["sessionId"], "acpSessionId": s["sessionId"], "cwd": s["cwd"], "name": s["title"], "createdAt": s["updatedAt"]}) {
				view[k] = v
			}
			sessions = append(sessions, view)
		}
		return object{"type": "sessions_list", "sessionId": request.local, "payload": object{"sessions": sessions}}, nil
	}
	return nil, nil
}
