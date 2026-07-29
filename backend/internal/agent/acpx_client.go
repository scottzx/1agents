package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/scottzx/1Agents/backend/internal/localtoken"
	"github.com/scottzx/1Agents/backend/internal/meta"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

const (
	// bridgeIdleTimeout is how long a frontend-disconnected bridge may sit
	// idle before its agent subprocess is reaped. The conversation survives:
	// the user's next visit reconnects and resumes via the stored acpSessionId.
	bridgeIdleTimeout = 5 * time.Minute
	// bridgeReapSweepInterval is how often reapLoop scans for idle bridges.
	bridgeReapSweepInterval = time.Minute
)

// nativeSystemPromptAgents are agent types whose ACP adapter honors
// _meta.systemPrompt (claude-agent-acp natively), so
// the role/system context reaches them cleanly through ensure_session. ACP has
// no portable system-prompt field — session/new carries only cwd + mcpServers,
// and all prompt content is treated as the user message — so for every other
// agent the systemContext is instead merged into the first prompt of a fresh
// session (see readFromClientLoop). Resumed sessions never re-inject: the role
// is already in the replayed history.
var nativeSystemPromptAgents = map[string]bool{
	string(AgentTypeClaudecode): true,
}

type ActiveBridge struct {
	SessionID     string
	ProjectID     string
	WorkspacePath string
	mu            sync.Mutex
	ClientConn    *websocket.Conn
	ServerConn    *websocket.Conn
	// MsgChan carries raw server→client frames. The relay forwards bytes
	// verbatim (lossless): WsMessage is only a typed *peek* for the
	// interception branches, never re-serialized, so new bridge event
	// fields flow through without touching Go.
	MsgChan chan []byte
	IsDone  bool

	// LastActivityAt is bumped on every client- or server-side message.
	// The idle reaper (reapLoop) uses it, together with a nil ClientConn,
	// to tear down agent subprocesses whose frontend has gone away and
	// stayed quiet — the real process leak, since a dropped ClientConn
	// leaves the bridge (and its agent) alive to support reconnect.
	LastActivityAt time.Time

	// pendingSystemContext holds the role/system context to merge into the
	// first user prompt, for a fresh session on an agent that does not honor
	// _meta.systemPrompt. Consumed (cleared) once, on the first prompt. Empty
	// for native agents (they get it via _meta) and for resumed sessions (the
	// role is already in replayed history).
	pendingSystemContext string

	// Issue-model write-back state (issue-model §8). TaskID/AgentType are
	// fixed at bridge creation; ReplyID is the timeline reply that
	// triggered the current turn and is refreshed on client reconnects.
	TaskID    string
	AgentType string
	ReplyID   string
	// tasksStore lets the client-read loop record each user prompt back to
	// the task timeline (symmetric to writeAgentReply), so the conversation
	// is captured server-side regardless of which UI sent the prompt.
	tasksStore   *TasksStore
	turnStore    *meta.AgentTurnStore
	activeTurn   *TurnContext
	pendingTurns []*TurnContext
	// turnText accumulates the assistant's streamed output text for the
	// current turn; reset on each tool call so that at `done` it holds the
	// final assistant message (text after the last tool call).
	turnText []string
}

// TurnContext binds one persisted Turn to the exact prompt frame forwarded to
// the ACP runtime. Only activeTurn is ever sent; pendingTurns remains FIFO.
type TurnContext struct {
	Turn meta.AgentTurn
	Raw  []byte
}

// touch records that the session saw activity just now, resetting its idle clock.
func (b *ActiveBridge) touch() {
	b.mu.Lock()
	b.LastActivityAt = time.Now()
	b.mu.Unlock()
}

// appendTurnText accumulates one streamed output chunk. Returns true when
// this was the first chunk of the current assistant text block (used to
// bump last_event_at once per block rather than on every delta).
func (b *ActiveBridge) appendTurnText(text string) (firstChunk bool) {
	b.mu.Lock()
	firstChunk = len(b.turnText) == 0
	b.turnText = append(b.turnText, text)
	b.mu.Unlock()
	return firstChunk
}

// resetTurnText clears the accumulator (new turn, or a tool call ended the
// current assistant message).
func (b *ActiveBridge) resetTurnText() {
	b.mu.Lock()
	b.turnText = nil
	b.mu.Unlock()
}

// takeTurnText returns the accumulated text and clears the buffer.
func (b *ActiveBridge) takeTurnText() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.turnText) == 0 {
		return ""
	}
	var sb []byte
	for _, t := range b.turnText {
		sb = append(sb, t...)
	}
	b.turnText = nil
	return string(sb)
}

type AcpxClient struct {
	serverPort int
	mu         sync.Mutex
	bridges    map[string]*ActiveBridge
	turnStore  *meta.AgentTurnStore
}

func NewAcpxClient(serverPort int, stores ...*meta.AgentTurnStore) *AcpxClient {
	c := &AcpxClient{
		serverPort: serverPort,
		bridges:    make(map[string]*ActiveBridge),
	}
	if len(stores) > 0 {
		c.turnStore = stores[0]
	}
	if c.turnStore != nil {
		failed, cancelled, reconciled, err := recoverInterruptedTurns(
			c.turnStore,
			defaultRuntimeStateDir(),
		)
		if err != nil {
			log.Printf("[acpx_client] Recover interrupted Turns failed: %v", err)
		} else if failed > 0 || cancelled > 0 || reconciled > 0 {
			log.Printf("[acpx_client] Recovered interrupted Turns: failed=%d cancelled=%d reconciled=%d",
				failed, cancelled, reconciled)
		}
	}
	go c.reapLoop()
	return c
}

// reapLoop periodically frees bridges whose frontend has disconnected
// (ClientConn == nil) and stayed idle past bridgeIdleTimeout. That is the real
// leak: a dropped ClientConn intentionally leaves the bridge — and its agent
// subprocess — alive so a reconnect can re-attach, but nothing tears it down if
// the user never returns. Bridges with a live ClientConn are never reaped: the
// user is present, and reaping one would just make the frontend auto-reconnect
// and immediately respawn the agent. A reaped conversation is not lost — the
// reconnect path re-runs ensure_session with the stored acpSessionId, which the
// agent resumes from its on-disk history (ACP session/resume).
func (c *AcpxClient) reapLoop() {
	ticker := time.NewTicker(bridgeReapSweepInterval)
	defer ticker.Stop()
	for range ticker.C {
		c.mu.Lock()
		snapshot := make([]*ActiveBridge, 0, len(c.bridges))
		for _, b := range c.bridges {
			snapshot = append(snapshot, b)
		}
		c.mu.Unlock()

		now := time.Now()
		for _, bridge := range snapshot {
			bridge.mu.Lock()
			idle := bridge.ClientConn == nil && !bridge.IsDone &&
				now.Sub(bridge.LastActivityAt) > bridgeIdleTimeout
			bridge.mu.Unlock()
			if idle {
				c.reapBridge(bridge)
			}
		}
	}
}

// reapBridge frees one idle, disconnected bridge. It asks the bridge-server to
// close the session (tearing down the agent subprocess), then closes ServerConn
// — that unblocks readFromServerLoop, whose defer removes the bridge from the
// registry and releases the workspace lock.
func (c *AcpxClient) reapBridge(bridge *ActiveBridge) {
	bridge.mu.Lock()
	serverConn := bridge.ServerConn
	sessionID := bridge.SessionID
	bridge.mu.Unlock()
	if serverConn == nil {
		return
	}
	log.Printf("[acpx_client] Reaping idle disconnected session: %s", sessionID)
	_ = serverConn.WriteJSON(WsMessage{Action: "close_session", SessionID: sessionID})
	_ = serverConn.Close()
}

type WsMessage struct {
	Action           string `json:"action,omitempty"`
	Event            string `json:"event,omitempty"`
	SessionID        string `json:"sessionId,omitempty"`
	WorkspacePath    string `json:"workspacePath,omitempty"`
	AgentType        string `json:"agentType,omitempty"`
	CCSessionID      string `json:"ccSessionId,omitempty"`
	AcpSessionID     string `json:"acpSessionId,omitempty"`
	SystemContext    string `json:"systemContext,omitempty"`
	Text             string `json:"text,omitempty"`
	RequestId        string `json:"requestId,omitempty"`
	TurnID           string `json:"turnId,omitempty"`
	Sequence         int64  `json:"sequence,omitempty"`
	Status           string `json:"status,omitempty"`
	StopReason       string `json:"stopReason,omitempty"`
	FinalAnswer      string `json:"finalAnswer,omitempty"`
	RuntimeRequestID string `json:"runtimeRequestId,omitempty"`
	PromptMessageID  string `json:"promptMessageId,omitempty"`
	Error            *struct {
		Code    string `json:"code,omitempty"`
		Message string `json:"message,omitempty"`
	} `json:"error,omitempty"`
	Attachments json.RawMessage `json:"attachments,omitempty"`
	Behavior    string          `json:"behavior,omitempty"`
	ToolName    string          `json:"toolName,omitempty"`
	ToolCallID  string          `json:"toolCallId,omitempty"`
	IsError     bool            `json:"isError,omitempty"`
	Arguments   json.RawMessage `json:"arguments,omitempty"`
	Summary     string          `json:"summary,omitempty"`
	// Stopped marks a `done` event that ended because the user hit "停止"
	// (cancel_turn), not a natural finish. The turn's partial reply is still
	// recorded, but handleTaskSessionDone must NOT flip the task to Completed.
	Stopped         bool            `json:"stopped,omitempty"`
	Items           json.RawMessage `json:"items,omitempty"`
	Messages        json.RawMessage `json:"messages,omitempty"`
	Code            string          `json:"code,omitempty"`
	Message         string          `json:"message,omitempty"`
	Type            string          `json:"type,omitempty"`
	ResumeSessionID string          `json:"resumeSessionId,omitempty"`
	AgentSessionID  string          `json:"agentSessionId,omitempty"`
	Payload         json.RawMessage `json:"payload,omitempty"`
	// McpServers carries the per-session MCP server config (a JSON array of
	// ACP McpServer entries) forwarded to the bridge-server, which sets it on
	// sessionOptions.mcpServers. Used by the AI Project Manager session to
	// inject the project-locked task-tool server. The Go side only declares
	// the field so it survives the ReadJSON → WriteJSON round trip.
	McpServers json.RawMessage `json:"mcpServers,omitempty"`
	// Env is host-owned process environment for the ACP agent. It carries the
	// signed 1agents Session identity used by shell-invoked project-items CLI
	// calls; callers cannot supply it through the browser WebSocket.
	Env map[string]string `json:"env,omitempty"`
	// PermissionMode is the per-session policy ("approve-reads" /
	// "approve-all" / "deny-all"). Two paths use it:
	//   1. set on the initial ensure_session message so the bridge-server
	//      can seed activeSessions[sessionId].permissionMode from the
	//      persisted ChatSessionRecord value.
	//   2. carried by the set_permission_mode action from the client.
	// The bridge-server reads the value out of the appropriate WS message;
	// the Go side only needs to declare the JSON field so it survives the
	// ReadJSON → WriteJSON round trip in readFromClientLoop.
	PermissionMode string `json:"permissionMode,omitempty"`
}

func (c *AcpxClient) Bridge(w http.ResponseWriter, r *http.Request, projectID, workspacePath, taskId, sessionId, agentType, systemContext string, mcpServers json.RawMessage, scheduler *Scheduler, tasksStore *TasksStore, chatStore *Store, acpSessionID, replyID string) {
	clientConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[acpx_client] upgrade failed: %v", err)
		return
	}

	// systemContext reaches native agents via ensure_session's _meta; non-native
	// agents receive it merged into the first prompt instead (pendingSystemContext),
	// so don't also ship it as _meta — that would double-inject the role.
	metaSystemContext := systemContext
	if !nativeSystemPromptAgents[agentType] {
		metaSystemContext = ""
	}
	sessionToken := localtoken.SessionToken(sessionId)
	sessionEnv := map[string]string{
		"ONEAGENTS_SESSION_ID":    sessionId,
		"ONEAGENTS_SESSION_TOKEN": sessionToken,
	}
	mcpServers = injectSessionAttribution(mcpServers, sessionId, sessionToken)

	c.mu.Lock()
	if c.bridges == nil {
		c.bridges = make(map[string]*ActiveBridge)
	}

	bridge, exists := c.bridges[sessionId]
	if exists {
		log.Printf("[acpx_client] Reconnecting client to existing active bridge for session: %s", sessionId)
		bridge.mu.Lock()
		if bridge.ClientConn != nil {
			// Tell the old client it was taken over by a newer connection
			// BEFORE closing it. The frontend uses this to suppress its
			// auto-reconnect (which would otherwise ping-pong the bridge
			// back and forth between two tabs) and show a banner instead.
			_ = bridge.ClientConn.WriteJSON(WsMessage{
				Event:     "session_taken_over",
				SessionID: sessionId,
			})
			_ = bridge.ClientConn.Close() // Close old client connection
		}
		bridge.ClientConn = clientConn
		// A reconnect is activity: refresh the idle clock so a bridge that
		// briefly reconnects then drops again isn't reaped on stale time.
		bridge.LastActivityAt = time.Now()
		// A follow-up reply may re-enter an existing bridge: refresh the
		// reply linkage so the next agent write-back points at it.
		if replyID != "" {
			bridge.ReplyID = replyID
		}
		bridge.mu.Unlock()
		c.mu.Unlock()
		c.enqueueTurnSync(bridge)

		// Send ensure_session again so the bridge-server updates its WS connection
		// Also reseed permission policy in case the JSON store changed while
		// the bridge stayed alive across page reloads.
		var reconnectMode string
		if chatStore != nil {
			if rec, ok, err := chatStore.Get(sessionId); err == nil && ok {
				reconnectMode = rec.PermissionMode
			}
		}
		ensureMsg := WsMessage{
			Action:          "ensure_session",
			SessionID:       sessionId,
			WorkspacePath:   workspacePath,
			AgentType:       agentType,
			AcpSessionID:    acpSessionID,
			ResumeSessionID: acpSessionID,
			SystemContext:   metaSystemContext,
			McpServers:      mcpServers,
			Env:             sessionEnv,
			PermissionMode:  reconnectMode,
		}
		ensureStart := time.Now()
		bridge.mu.Lock()
		if bridge.ServerConn != nil {
			_ = bridge.ServerConn.WriteJSON(ensureMsg)
		}
		bridge.mu.Unlock()
		ensureDur := time.Since(ensureStart)
		log.Printf("[acpx_client] Reconnect ensure_session sent for %s (took %v)", sessionId, ensureDur)

		// Start reading from the new client connection and forwarding to the existing server connection
		c.readFromClientLoop(bridge, clientConn)
		return
	}

	// Create new bridge
	serverURL := fmt.Sprintf("ws://127.0.0.1:%d", c.serverPort)
	dialStart := time.Now()
	log.Printf("[acpx_client] Dialing bridge-server at %s", serverURL)

	serverConn, _, err := websocket.DefaultDialer.Dial(serverURL, nil)
	if err != nil {
		dialDur := time.Since(dialStart)
		log.Printf("[acpx_client] Dial bridge-server failed for %s: %v (took %v)", sessionId, err, dialDur)
		_ = clientConn.WriteJSON(WsMessage{
			Event:     "error",
			SessionID: sessionId,
			Code:      "SERVER_UNAVAILABLE",
			Message:   "ACP microservice is unavailable. Please make sure it is running.",
		})
		c.mu.Unlock()
		_ = clientConn.Close()
		return
	}
	dialDur := time.Since(dialStart)
	log.Printf("[acpx_client] Dial bridge-server succeeded for %s (took %v)", sessionId, dialDur)

	bridge = &ActiveBridge{
		SessionID:      sessionId,
		ProjectID:      projectID,
		WorkspacePath:  workspacePath,
		ClientConn:     clientConn,
		ServerConn:     serverConn,
		MsgChan:        make(chan []byte, 100),
		TaskID:         taskId,
		AgentType:      agentType,
		ReplyID:        replyID,
		tasksStore:     tasksStore,
		turnStore:      c.turnStore,
		LastActivityAt: time.Now(),
	}
	// Fresh session on an agent that can't take a system prompt over ACP:
	// stash the role/system context to merge into the first prompt. Native
	// agents get it via ensure_session's _meta; resumes (acpSessionID != "")
	// already carry it in replayed history.
	if systemContext != "" && acpSessionID == "" && !nativeSystemPromptAgents[agentType] {
		bridge.pendingSystemContext = systemContext
	}
	c.bridges[sessionId] = bridge
	c.mu.Unlock()

	// Seed the per-session permission policy from the persisted record so
	// the bridge-server gates handlePermissionRequestCallback on first
	// turn — same value the Composer mode toggle later overwrites via
	// set_permission_mode. Empty string means "use bridge-server default".
	var initialMode string
	if chatStore != nil {
		if rec, ok, err := chatStore.Get(sessionId); err == nil && ok {
			initialMode = rec.PermissionMode
		}
	}

	// 1. Initialize session on bridge-server
	ensureMsg := WsMessage{
		Action:          "ensure_session",
		SessionID:       sessionId,
		WorkspacePath:   workspacePath,
		AgentType:       agentType,
		AcpSessionID:    acpSessionID,
		ResumeSessionID: acpSessionID,
		SystemContext:   metaSystemContext,
		McpServers:      mcpServers,
		Env:             sessionEnv,
		PermissionMode:  initialMode,
	}
	ensureStart := time.Now()
	if err := serverConn.WriteJSON(ensureMsg); err != nil {
		ensureDur := time.Since(ensureStart)
		log.Printf("[acpx_client] Failed to send ensure_session for %s: %v (took %v)", sessionId, err, ensureDur)
		c.cleanupBridge(sessionId)
		_ = serverConn.Close()
		_ = clientConn.Close()
		return
	}
	ensureDur := time.Since(ensureStart)
	log.Printf("[acpx_client] ensure_session sent for %s (took %v)", sessionId, ensureDur)

	// Start server connection reader loop
	go c.readFromServerLoop(bridge, scheduler, tasksStore, chatStore, taskId)

	// Start write helper loop for writing to the active client connection
	go c.writeToClientLoop(bridge)

	// Every Browser ownership connection receives an authoritative durable
	// Turn projection, including the first connection after a backend restart.
	c.enqueueTurnSync(bridge)

	// Read from client and forward to server connection
	c.readFromClientLoop(bridge, clientConn)
}

func (c *AcpxClient) readFromServerLoop(bridge *ActiveBridge, scheduler *Scheduler, tasksStore *TasksStore, chatStore *Store, taskId string) {
	defer func() {
		// A server-side connection loss is different from a frontend WebSocket
		// disconnect: the ACP runtime can no longer own active work, so fail it
		// and cancel anything that was still queued.
		c.failOutstandingTurns(bridge, tasksStore, chatStore)
		bridge.mu.Lock()
		bridge.IsDone = true
		close(bridge.MsgChan)
		if bridge.ServerConn != nil {
			_ = bridge.ServerConn.Close()
		}
		bridge.mu.Unlock()

		// Cleanup registry entry
		c.cleanupBridge(bridge.SessionID)
		scheduler.Lock.Release(bridge.WorkspacePath)
		log.Printf("[acpx_client] Server connection reader loop finished for session: %s", bridge.SessionID)
	}()

	for {
		// Raw-bytes relay: read the frame verbatim and only *peek* into it
		// with the typed WsMessage for the interception branches below. The
		// frame forwarded to the client is the original bytes, so fields the
		// struct doesn't declare (session_meta payloads etc.) survive.
		_, raw, err := bridge.ServerConn.ReadMessage()
		if err != nil {
			log.Printf("[acpx_client] Read from server failed for session %s: %v", bridge.SessionID, err)
			break
		}
		bridge.touch()
		var msg WsMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			// Not JSON we understand — still forward verbatim; only the
			// interception below needs the parse.
			log.Printf("[acpx_client] Peek unmarshal failed for session %s (forwarding raw): %v", bridge.SessionID, err)
		}

		var nextTurn *TurnContext

		// Intercept and update status
		if msg.Event == "session_ready" && msg.AgentSessionID != "" {
			if chatStore != nil {
				if err := chatStore.UpdateACP(bridge.SessionID, msg.AgentSessionID); err != nil {
					log.Printf("[acpx_client] UpdateACP(%s, %s) failed: %v", bridge.SessionID, msg.AgentSessionID, err)
				} else {
					log.Printf("[acpx_client] Persisted acpSessionId=%s for chat session %s", msg.AgentSessionID, bridge.SessionID)
				}
			}
		} else if msg.Type == "sessions_list" {
			if chatStore != nil {
				var listPayload struct {
					Sessions []struct {
						ID           string `json:"id"`
						AcpxRecordId string `json:"acpxRecordId"`
						AcpSessionId string `json:"acpSessionId"`
						Cwd          string `json:"cwd"`
						Name         string `json:"name"`
						AgentCommand string `json:"agentCommand"`
						CreatedAt    string `json:"createdAt"`
						Status       string `json:"status"`
						Closed       bool   `json:"closed"`
					} `json:"sessions"`
				}
				if err := json.Unmarshal(msg.Payload, &listPayload); err == nil {
					filtered := make([]any, 0)
					for _, s := range listPayload.Sessions {
						if cleanPath(s.Cwd) == cleanPath(bridge.WorkspacePath) {
							rec, ok, err := chatStore.Get(s.ID)
							role := ""
							permissionMode := ""
							archivedAt := ""
							taskIdVal := ""
							ccProject := ""
							ccSessionId := ""
							if err == nil && ok {
								role = rec.Role
								permissionMode = rec.PermissionMode
								taskIdVal = rec.TaskID
								ccProject = rec.CcProject
								ccSessionId = rec.CcSessionID
								if !rec.ArchivedAt.IsZero() {
									archivedAt = rec.ArchivedAt.Format(time.RFC3339)
								}
							}

							filtered = append(filtered, map[string]any{
								"id":             s.ID,
								"workspaceId":    getWorkspaceId(chatStore, bridge.SessionID),
								"taskId":         taskIdVal,
								"name":           s.Name,
								"agentType":      getAgentTypeFromCommand(s.AgentCommand),
								"ccProject":      ccProject,
								"ccSessionId":    ccSessionId,
								"acpSessionId":   s.AcpSessionId,
								"sessionKey":     s.ID,
								"status":         s.Status,
								"createdAt":      s.CreatedAt,
								"role":           role,
								"permissionMode": permissionMode,
								"archivedAt":     archivedAt,
							})
						}
					}
					var outer map[string]any
					if err := json.Unmarshal(raw, &outer); err == nil {
						outer["payload"] = map[string]any{
							"sessions": filtered,
						}
						if rawUpdated, err := json.Marshal(outer); err == nil {
							raw = rawUpdated
						}
					}
				}
			}
		} else if msg.Type == "session_forked" {
			if chatStore != nil {
				var forkPayload struct {
					ParentSessionId string `json:"parentSessionId"`
					Session         struct {
						ID           string `json:"id"`
						AcpSessionId string `json:"acpSessionId"`
						Cwd          string `json:"cwd"`
						Name         string `json:"name"`
						AgentCommand string `json:"agentCommand"`
						CreatedAt    string `json:"createdAt"`
					} `json:"session"`
				}
				if err := json.Unmarshal(msg.Payload, &forkPayload); err == nil {
					if parentRec, ok, err := chatStore.Get(forkPayload.ParentSessionId); err == nil && ok {
						newRec := parentRec
						newRec.ID = forkPayload.Session.ID
						newRec.AcpSessionID = forkPayload.Session.AcpSessionId
						newRec.SessionKey = forkPayload.Session.ID
						newRec.Name = forkPayload.Session.Name
						newRec.AgentType = getAgentTypeFromCommand(forkPayload.Session.AgentCommand)
						newRec.CreatedAt = time.Now().UTC()
						newRec.LastEventAt = time.Now().UTC()
						newRec.ArchivedAt = time.Time{}

						if err := chatStore.Add(newRec); err != nil {
							log.Printf("[acpx_client] Failed to insert forked session to meta.db: %v", err)
						} else {
							log.Printf("[acpx_client] Persisted forked session %s (parent: %s) to meta.db", newRec.ID, forkPayload.ParentSessionId)
						}

						outer := make(map[string]any)
						if err := json.Unmarshal(raw, &outer); err == nil {
							if payloadMap, ok := outer["payload"].(map[string]any); ok {
								if sessionMap, ok := payloadMap["session"].(map[string]any); ok {
									sessionMap["workspaceId"] = parentRec.WorkspaceID
									sessionMap["taskId"] = parentRec.TaskID
									sessionMap["ccProject"] = parentRec.CcProject
									sessionMap["ccSessionId"] = parentRec.CcSessionID
									sessionMap["role"] = parentRec.Role
									sessionMap["permissionMode"] = parentRec.PermissionMode
								}
							}
							if rawUpdated, err := json.Marshal(outer); err == nil {
								raw = rawUpdated
							}
						}
					}
				}
			}
		} else if msg.Type == "session_deleted" {
			if chatStore != nil {
				targetSid := msg.SessionID
				if targetSid == "" {
					targetSid = bridge.SessionID
				}
				if err := chatStore.Delete(targetSid); err != nil {
					log.Printf("[acpx_client] Failed to delete session %s from meta.db: %v", targetSid, err)
				} else {
					log.Printf("[acpx_client] Deleted session %s from meta.db", targetSid)
				}
			}
		} else if msg.Event == "text_delta" {
			// Accumulate the assistant's streamed output for the issue
			// timeline write-back ('thought' chunks are not part of the
			// final message). First non-thought chunk of a text block also
			// bumps last_event_at so the sidebar sorts by last assistant
			// reply (newest first).
			if msg.Type != "thought" && msg.Text != "" {
				if first := bridge.appendTurnText(msg.Text); first && chatStore != nil {
					if err := chatStore.Touch(bridge.SessionID); err != nil {
						log.Printf("[acpx_client] Touch(%s) after assistant text: %v", bridge.SessionID, err)
					}
				}
			}
		} else if msg.Event == "tool_call" {
			// A tool call ends the current assistant text block; only text
			// after the LAST tool call is the final message (issue-model
			// decision A: full last assistant message).
			bridge.resetTurnText()
		} else if msg.Event == "turn_terminal" {
			log.Printf("[acpx_client] Turn terminal: session_id=%s turn_id=%s status=%s",
				bridge.SessionID, msg.TurnID, msg.Status)
			turnID := msg.TurnID
			var explicit bool
			var finishErr error
			raw, nextTurn, explicit, finishErr = c.finishActiveTurn(bridge, msg, raw, tasksStore, chatStore)
			if finishErr != nil {
				log.Printf("[acpx_client] Finish Turn %s for session %s failed: %v", turnID, bridge.SessionID, finishErr)
				break
			}
			if explicit {
				stopped := msg.Status == string(meta.AgentTurnCancelled)
				if msg.Status == string(meta.AgentTurnFailed) {
					errorText := msg.Message
					if msg.Error != nil {
						errorText = msg.Error.Message
					}
					c.handleTaskSessionError(bridge.WorkspacePath, taskId, bridge.SessionID, turnID, errorText, tasksStore)
				} else {
					c.handleTaskSessionDone(
						bridge.WorkspacePath, taskId, bridge.SessionID, turnID,
						msg.StopReason, stopped, tasksStore,
					)
				}
				scheduler.Lock.Release(bridge.WorkspacePath)
			}
		} else if msg.Event == "done" {
			log.Printf("[acpx_client] Turn done for session %s (stopped=%v). Intercepted summary: %s", bridge.SessionID, msg.Stopped, msg.Summary)
			if activeBridgeTurnID(bridge) == "" {
				writeAgentReply(bridge, tasksStore, chatStore)
				c.handleTaskSessionDone(bridge.WorkspacePath, taskId, bridge.SessionID, "", msg.Summary, msg.Stopped, tasksStore)
				scheduler.Lock.Release(bridge.WorkspacePath)
			}
		} else if msg.Event == "error" {
			// Generic errors include control/session failures and are not a Turn
			// terminal. Only turn_terminal may transition the active AgentTurn.
			log.Printf("[acpx_client] Non-terminal error: session_id=%s turn_id=%s code=%s message=%s",
				bridge.SessionID, msg.TurnID, msg.Code, msg.Message)
		}

		// Send the ORIGINAL bytes to the client write channel (lossless).
		bridge.mu.Lock()
		if !bridge.IsDone {
			select {
			case bridge.MsgChan <- raw:
			default:
				log.Printf("[acpx_client] MsgChan full, dropping %q message for session %s", msg.Event, bridge.SessionID)
			}
		}
		bridge.mu.Unlock()

		if nextTurn != nil {
			if err := c.startTurn(bridge, nextTurn); err != nil {
				log.Printf("[acpx_client] Start queued Turn %s failed: %v", nextTurn.Turn.ID, err)
				break
			}
		}
	}
}

func (c *AcpxClient) writeToClientLoop(bridge *ActiveBridge) {
	for raw := range bridge.MsgChan {
		bridge.mu.Lock()
		clientConn := bridge.ClientConn
		bridge.mu.Unlock()

		if clientConn != nil {
			if err := clientConn.WriteMessage(websocket.TextMessage, raw); err != nil {
				log.Printf("[acpx_client] Failed to write to client connection for session %s: %v", bridge.SessionID, err)
			}
		}
	}
}

func (c *AcpxClient) readFromClientLoop(bridge *ActiveBridge, clientConn *websocket.Conn) {
	defer func() {
		bridge.mu.Lock()
		if bridge.ClientConn == clientConn {
			bridge.ClientConn = nil
		}
		_ = clientConn.Close()
		bridge.mu.Unlock()
		log.Printf("[acpx_client] Client connection reader loop finished for session: %s", bridge.SessionID)
	}()

	for {
		// Raw-bytes relay (same as readFromServerLoop): peek with the typed
		// struct, forward the original bytes so client actions with fields the
		// struct doesn't declare reach the bridge intact.
		_, raw, err := clientConn.ReadMessage()
		if err != nil {
			log.Printf("[acpx_client] Read from client connection failed for session %s: %v", bridge.SessionID, err)
			break
		}
		bridge.touch()
		var msg WsMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			log.Printf("[acpx_client] Peek unmarshal of client message failed for session %s (forwarding raw): %v", bridge.SessionID, err)
		}

		// First prompt of a fresh session on a non-native agent: prepend the
		// role/system context so the agent receives it as one combined message
		// (no separate priming turn). The persisted prompt remains the user's
		// original text, without this system preamble.
		if msg.Action == "prompt" {
			bridge.mu.Lock()
			pending := bridge.pendingSystemContext
			bridge.pendingSystemContext = ""
			bridge.mu.Unlock()
			if pending != "" {
				raw = mergeSystemContextIntoPrompt(raw, pending)
			}
			handled, queueErr := c.queuePrompt(bridge, msg, raw)
			if queueErr != nil {
				log.Printf("[acpx_client] Queue prompt for session %s failed: %v", bridge.SessionID, queueErr)
				code := "TURN_ACCEPT_FAILED"
				if errors.Is(queueErr, meta.ErrIdempotencyConflict) {
					code = "IDEMPOTENCY_CONFLICT"
				}
				enqueueProtocolError(bridge, msg.RequestId, code, queueErr.Error())
				continue
			}
			if handled {
				continue
			}

			// Legacy compatibility for callers that do not provide a TurnStore.
			bridge.resetTurnText()
			writeUserReply(bridge, bridge.tasksStore, msg.Text)
		}

		if (msg.Action == "cancel_turn" || msg.Action == "cancel_queued_turn") && msg.TurnID != "" {
			handled, cancelErr := c.cancelPendingTurn(bridge, msg.TurnID)
			if cancelErr != nil {
				log.Printf("[acpx_client] Cancel queued Turn %s failed: %v", msg.TurnID, cancelErr)
			}
			if handled {
				continue
			}
		}

		bridge.mu.Lock()
		isDone := bridge.IsDone
		bridge.mu.Unlock()
		if isDone {
			break
		}

		if err := c.writeServerRaw(bridge, raw); err != nil {
			log.Printf("[acpx_client] Forward client message to server failed for session %s: %v", bridge.SessionID, err)
			break
		}
	}
}

// mergeSystemContextIntoPrompt prepends the pending role/system context to the
// prompt's text field, rewriting through map[string]any (never WsMessage) so
// fields the peek struct doesn't declare survive. On any parse failure the
// original bytes are returned unchanged — a lost preamble beats a lost prompt.
func mergeSystemContextIntoPrompt(raw []byte, pending string) []byte {
	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return raw
	}
	text, _ := generic["text"].(string)
	generic["text"] = pending + "\n\n" + text
	rewritten, err := json.Marshal(generic)
	if err != nil {
		return raw
	}
	return rewritten
}

// writeUserReply records a user prompt to the task timeline (issue-model §8,
// user side; mirror of writeAgentReply). SessionRef groups it under the
// session's branch. No-op for sessions outside a task or empty prompts.
func writeUserReply(bridge *ActiveBridge, tasksStore *TasksStore, text string, turnIDs ...string) string {
	bridge.mu.Lock()
	taskID := bridge.TaskID
	sessionID := bridge.SessionID
	bridge.mu.Unlock()

	if taskID == "" || strings.TrimSpace(text) == "" || tasksStore == nil {
		return ""
	}

	var turnID string
	if len(turnIDs) > 0 {
		turnID = turnIDs[0]
	}
	reply, err := tasksStore.AppendReply(taskID, Reply{
		Author:     Author{Kind: "user", Name: "user"},
		Text:       text,
		SessionRef: sessionID,
		TurnID:     turnID,
		Mode:       ModeFollowUp,
	})
	if err != nil {
		log.Printf("[acpx_client] AppendReply(user) for task %s failed: %v", taskID, err)
	} else {
		log.Printf("[acpx_client] User prompt written to task %s timeline (%d chars)", taskID, len(text))
	}
	return reply.ID
}

// writeAgentReply writes the turn's final assistant message back to the
// task timeline (issue-model §8: server-side interception, one reply per
// turn). Shared by the browser-bridged path and the headless TaskRunner.
// No-op for sessions outside a task or empty turns.
func writeAgentReply(bridge *ActiveBridge, tasksStore *TasksStore, chatStore *Store, turnIDs ...string) {
	text := bridge.takeTurnText()
	var turnID string
	if len(turnIDs) > 0 {
		turnID = turnIDs[0]
	}
	writeAgentReplyText(bridge, tasksStore, chatStore, turnID, "", text)
}

func writeAgentReplyText(
	bridge *ActiveBridge,
	tasksStore *TasksStore,
	chatStore *Store,
	turnID, initiatingReplyID, text string,
) string {
	bridge.mu.Lock()
	taskID := bridge.TaskID
	agentType := bridge.AgentType
	legacyReplyID := bridge.ReplyID
	bridge.mu.Unlock()

	if taskID == "" || strings.TrimSpace(text) == "" || tasksStore == nil {
		return ""
	}
	if initiatingReplyID == "" {
		initiatingReplyID = legacyReplyID
	}

	var acpSessionID string
	if chatStore != nil {
		if rec, ok, err := chatStore.Get(bridge.SessionID); err == nil && ok {
			acpSessionID = rec.AcpSessionID
		}
	}

	reply, err := tasksStore.AppendReply(taskID, Reply{
		Author:       Author{Kind: "agent", Name: agentType},
		AgentType:    agentType,
		Text:         text,
		SessionRef:   bridge.SessionID,
		TurnID:       turnID,
		AcpSessionID: acpSessionID,
		InReplyTo:    initiatingReplyID,
		Mode:         ModePureComment,
	})
	if err != nil {
		log.Printf("[acpx_client] AppendReply for task %s failed: %v", taskID, err)
	} else {
		log.Printf("[acpx_client] Agent reply written to task %s timeline (%d chars)", taskID, len(text))
	}
	return reply.ID
}

func (c *AcpxClient) cleanupBridge(sessionId string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.bridges != nil {
		delete(c.bridges, sessionId)
	}
}

func activeBridgeTurnID(bridge *ActiveBridge) string {
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	if bridge.activeTurn == nil {
		return ""
	}
	return bridge.activeTurn.Turn.ID
}

func (c *AcpxClient) handleTaskSessionDone(workspacePath, taskId, sessionId, turnID, summary string, stopped bool, tasksStore *TasksStore) {
	now := time.Now().UTC()
	existing, ok, err := tasksStore.GetTask(taskId)
	if err != nil || !ok {
		return
	}
	if existing.Type == ItemTypeDiscussion {
		stopped = true
	}
	runs := tasksStore.TaskRuns()
	run, hasRun, runErr := runs.RunningBySession(sessionId)
	if runErr == nil && !hasRun && existing.Type != ItemTypeDiscussion {
		run, runErr = runs.Create(workspacePath, meta.TaskRun{
			TaskID: taskId, SessionID: sessionId, OriginTurnID: turnID, Kind: meta.TaskRunExecution,
		})
		hasRun = runErr == nil
	}
	terminal := TaskStatusCompleted
	runStatus := meta.TaskRunCompleted
	var closedBy *meta.ClosedBy
	if stopped {
		terminal = existing.Status
		runStatus = meta.TaskRunCancelled
	} else if needsReview(&existing) {
		terminal = TaskStatusPendingReview
	} else if !hasRun {
		terminal = TaskStatusFailed
		runStatus = meta.TaskRunFailed
		summary = "completion audit unavailable: TaskRun was not created"
	} else {
		closedBy = &meta.ClosedBy{Kind: "runtime_evidence", Verdict: "passed"}
	}
	if hasRun {
		finished, finishErr := runs.Finish(run.ID, runStatus, []meta.CompletionEvidence{{
			Kind: "runtime_terminal", Summary: summary, SessionID: sessionId, TurnID: turnID,
		}}, nil, closedBy, "")
		if finishErr != nil {
			log.Printf("[acpx_client] finish TaskRun %s: %v", run.ID, finishErr)
			if terminal == TaskStatusCompleted {
				terminal = TaskStatusFailed
				summary = "completion audit failed: " + finishErr.Error()
				closedBy = nil
			}
		} else {
			closedBy = finished.ClosedBy
		}
	}
	_ = tasksStore.Mutate(workspacePath, func(cfg *TasksConfig) bool {
		for i := range cfg.Tasks {
			task := &cfg.Tasks[i]
			if task.ID == taskId {
				// A discussion is a PM conversation, never an executable task:
				// its turns must not flip it to completed. A user "停止"
				// (stopped) likewise ends the turn without completing the task.
				// Either way the agent's reply was already recorded by
				// writeAgentReply; here we only mark the session idle below.
				if task.Type != ItemTypeDiscussion && !stopped {
					task.Status = terminal
					if terminal == TaskStatusCompleted {
						task.CompletedAt = &now
						task.ClosedBy = closedBy
						task.Replies = append(task.Replies, Reply{
							Author: Author{Kind: "system", Name: "completion-gate"},
							Text:   fmt.Sprintf("完成审计：TaskRun `%s`，Evidence 已记录，Verdict=passed。", run.ID),
							Mode:   ModePureComment, CreatedAt: now,
						})
					}
					task.Summary = summary
				}
				task.UpdatedAt = now

				// Add or update session metadata
				sessionExists := false
				for j := range task.Sessions {
					sess := &task.Sessions[j]
					if sess.ID == sessionId {
						sess.Status = SessionStatusIdle
						sess.Summary = summary
						sessionExists = true
						break
					}
				}

				if !sessionExists {
					task.Sessions = append(task.Sessions, SessionMetadata{
						ID:        sessionId,
						Kind:      SessionKindChat,
						Name:      "智能体排查与修复",
						AgentType: "claudecode",
						Status:    SessionStatusIdle,
						Summary:   summary,
						CreatedAt: now,
					})
				}
				return true
			}
		}
		return false
	})
}

func (c *AcpxClient) handleTaskSessionError(workspacePath, taskId, sessionId, turnID, errMsg string, tasksStore *TasksStore) {
	now := time.Now().UTC()
	if existing, ok, _ := tasksStore.GetTask(taskId); ok && existing.Type != ItemTypeDiscussion {
		runs := tasksStore.TaskRuns()
		run, hasRun, _ := runs.RunningBySession(sessionId)
		if !hasRun {
			run, _ = runs.Create(workspacePath, meta.TaskRun{
				TaskID: taskId, SessionID: sessionId, OriginTurnID: turnID, Kind: meta.TaskRunExecution,
			})
		}
		if run.ID != "" {
			_, _ = runs.Finish(run.ID, meta.TaskRunFailed, []meta.CompletionEvidence{{
				Kind: "runtime_error", Summary: errMsg, SessionID: sessionId, TurnID: turnID,
			}}, nil, nil, errMsg)
		}
	}
	_ = tasksStore.Mutate(workspacePath, func(cfg *TasksConfig) bool {
		for i := range cfg.Tasks {
			task := &cfg.Tasks[i]
			if task.ID == taskId {
				// Discussions are PM conversations, not tasks — a turn error
				// must not mark the discussion failed.
				if task.Type != ItemTypeDiscussion {
					task.Status = TaskStatusFailed
				}
				task.UpdatedAt = now

				for j := range task.Sessions {
					sess := &task.Sessions[j]
					if sess.ID == sessionId {
						sess.Status = SessionStatusIdle
						sess.Summary = "Error: " + errMsg
						break
					}
				}
				return true
			}
		}
		return false
	})
}

func getWorkspaceId(store *Store, sessionID string) string {
	if store == nil {
		return ""
	}
	if rec, ok, err := store.Get(sessionID); err == nil && ok {
		return rec.WorkspaceID
	}
	return ""
}

func cleanPath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	return strings.ToLower(strings.TrimRight(p, "/"))
}

func getAgentTypeFromCommand(cmd string) string {
	if strings.Contains(strings.ToLower(cmd), "claude") {
		return "claudecode"
	}
	if strings.Contains(strings.ToLower(cmd), "codex") {
		return "codex"
	}
	return "claudecode"
}
