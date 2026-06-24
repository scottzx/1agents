// Package mcptasks is a minimal Model Context Protocol (MCP) server exposed
// as the `1agents mcp-tasks` subcommand. It gives the in-app AI Project
// Manager chat session a small set of task-management tools, all locked to a
// single workspace.
//
// Transport is stdio with newline-delimited JSON-RPC 2.0 (the MCP stdio
// transport). The server holds no state of its own: every tool call proxies
// to the running daemon's existing HTTP task API. The target workspace is
// fixed by the ONEAGENTS_WORKSPACE_ID env var injected by the backend at
// bridge time, so the agent can neither read nor write any other project —
// the tools deliberately expose no workspace/project parameter.
//
// Environment (all injected by the backend, never by the agent):
//
//	ONEAGENTS_BASE_URL        e.g. http://127.0.0.1:8080
//	ONEAGENTS_WORKSPACE_ID    the locked workspace id
//	ONEAGENTS_TASK_ID         optional: locks the session to a single task
//	                          (executor scope, #50). When set, the tool surface
//	                          narrows to reading/updating just that task; the
//	                          PM-only create/milestone tools are withheld.
//	ONEAGENTS_TASK_ROLE       optional: "executor" (default when task-locked) or
//	                          "verifier". The verifier scope is hard read-only —
//	                          update_task is withheld and submit_review is added,
//	                          so a reviewer can only judge, never edit (#50).
//	ONEAGENTS_INTERNAL_TOKEN  loopback bearer accepted by authMiddleware
package mcptasks

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const protocolVersion = "2024-11-05"

// Run executes the stdio MCP loop until stdin closes.
func Run() error {
	baseURL := strings.TrimRight(os.Getenv("ONEAGENTS_BASE_URL"), "/")
	workspaceID := os.Getenv("ONEAGENTS_WORKSPACE_ID")
	token := os.Getenv("ONEAGENTS_INTERNAL_TOKEN")
	if baseURL == "" || workspaceID == "" {
		return fmt.Errorf("ONEAGENTS_BASE_URL and ONEAGENTS_WORKSPACE_ID are required")
	}

	s := &server{
		api: &apiClient{
			baseURL: baseURL,
			token:   token,
			http:    &http.Client{Timeout: 30 * time.Second},
		},
		workspaceID: workspaceID,
		taskID:      os.Getenv("ONEAGENTS_TASK_ID"),
		taskRole:    os.Getenv("ONEAGENTS_TASK_ROLE"),
		out:         bufio.NewWriter(os.Stdout),
	}
	return s.loop(os.Stdin)
}

// ── JSON-RPC plumbing ───────────────────────────────────────────────────────

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type server struct {
	api         *apiClient
	workspaceID string
	// taskID, when non-empty, locks the session to a single task (executor
	// scope, #50): get/update/list are confined to it and PM-only tools are
	// withheld. Empty means project-wide PM scope.
	taskID string
	// taskRole selects the task-locked tool surface: "verifier" is hard
	// read-only (no update_task, plus submit_review); anything else ("executor"
	// or unset) keeps the read+update_task executor surface. Ignored when
	// taskID is empty (PM scope).
	taskRole string
	out      *bufio.Writer
}

func (s *server) loop(in io.Reader) error {
	reader := bufio.NewReader(in)
	for {
		line, err := reader.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			s.handleLine(bytes.TrimSpace(line))
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

func (s *server) handleLine(line []byte) {
	var req rpcRequest
	if err := json.Unmarshal(line, &req); err != nil {
		// Cannot recover an id; emit a parse error per JSON-RPC.
		s.writeRaw(map[string]any{
			"jsonrpc": "2.0",
			"id":      nil,
			"error":   rpcError{Code: -32700, Message: "parse error"},
		})
		return
	}

	// Notifications (no id) get no response.
	isNotification := len(req.ID) == 0

	switch req.Method {
	case "initialize":
		s.reply(req.ID, s.onInitialize(req.Params))
	case "notifications/initialized", "notifications/cancelled":
		// no-op
	case "ping":
		if !isNotification {
			s.reply(req.ID, map[string]any{})
		}
	case "tools/list":
		s.reply(req.ID, map[string]any{"tools": s.listedTools()})
	case "tools/call":
		s.reply(req.ID, s.onToolCall(req.Params))
	default:
		if !isNotification {
			s.replyError(req.ID, -32601, "method not found: "+req.Method)
		}
	}
}

func (s *server) onInitialize(params json.RawMessage) map[string]any {
	version := protocolVersion
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if len(params) > 0 && json.Unmarshal(params, &p) == nil && p.ProtocolVersion != "" {
		version = p.ProtocolVersion
	}
	return map[string]any{
		"protocolVersion": version,
		"capabilities":    map[string]any{"tools": map[string]any{}},
		"serverInfo":      map[string]any{"name": "1agents-tasks", "version": "1.0.0"},
	}
}

func (s *server) reply(id json.RawMessage, result any) {
	s.writeRaw(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (s *server) replyError(id json.RawMessage, code int, msg string) {
	s.writeRaw(map[string]any{"jsonrpc": "2.0", "id": id, "error": rpcError{Code: code, Message: msg}})
}

func (s *server) writeRaw(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	s.out.Write(b)
	s.out.WriteByte('\n')
	s.out.Flush()
}

// ── tool result helpers ─────────────────────────────────────────────────────

// toolText builds a successful tools/call result carrying plain text.
func toolText(text string) map[string]any {
	return map[string]any{"content": []map[string]any{{"type": "text", "text": text}}}
}

// toolJSON marshals v and returns it as a text result (the MCP convention for
// structured tool output the model can read).
func toolJSON(v any) map[string]any {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return toolErr("failed to encode result: " + err.Error())
	}
	return toolText(string(b))
}

// toolErr returns a tool-level error (isError:true) so the model sees the
// failure instead of the whole call being rejected at the protocol layer.
func toolErr(msg string) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": msg}},
		"isError": true,
	}
}

// ── HTTP client ─────────────────────────────────────────────────────────────

type apiClient struct {
	baseURL string
	token   string
	http    *http.Client
}

func (c *apiClient) do(method, path string, query url.Values, body any) (int, []byte, error) {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, u, reader)
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return resp.StatusCode, data, err
}
