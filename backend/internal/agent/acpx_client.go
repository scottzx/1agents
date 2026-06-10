package agent

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type AcpxClient struct {
	serverPort int
}

func NewAcpxClient(serverPort int) *AcpxClient {
	return &AcpxClient{
		serverPort: serverPort,
	}
}

type WsMessage struct {
	Action          string          `json:"action,omitempty"`
	Event           string          `json:"event,omitempty"`
	SessionID       string          `json:"sessionId,omitempty"`
	WorkspacePath   string          `json:"workspacePath,omitempty"`
	AgentType       string          `json:"agentType,omitempty"`
	CCSessionID     string          `json:"ccSessionId,omitempty"`
	AcpSessionID    string          `json:"acpSessionId,omitempty"`
	SystemContext   string          `json:"systemContext,omitempty"`
	Text            string          `json:"text,omitempty"`
	RequestId       string          `json:"requestId,omitempty"`
	Behavior        string          `json:"behavior,omitempty"`
	ToolName        string          `json:"toolName,omitempty"`
	Arguments       json.RawMessage `json:"arguments,omitempty"`
	Summary         string          `json:"summary,omitempty"`
	Items           json.RawMessage `json:"items,omitempty"`
	Messages        json.RawMessage `json:"messages,omitempty"`
	Code            string          `json:"code,omitempty"`
	Message         string          `json:"message,omitempty"`
	Type            string          `json:"type,omitempty"`
	ResumeSessionID string          `json:"resumeSessionId,omitempty"`
	AgentSessionID  string          `json:"agentSessionId,omitempty"`
}

func (c *AcpxClient) Bridge(w http.ResponseWriter, r *http.Request, workspacePath, taskId, sessionId, agentType, systemContext string, scheduler *Scheduler, tasksStore *TasksStore, chatStore *Store, acpSessionID string) {
	clientConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[acpx_client] upgrade failed: %v", err)
		return
	}
	defer clientConn.Close()

	serverURL := fmt.Sprintf("ws://127.0.0.1:%d", c.serverPort)
	log.Printf("[acpx_client] Dialing bridge-server at %s", serverURL)

	serverConn, _, err := websocket.DefaultDialer.Dial(serverURL, nil)
	if err != nil {
		log.Printf("[acpx_client] Dial bridge-server failed: %v", err)
		_ = clientConn.WriteJSON(WsMessage{
			Event:     "error",
			SessionID: sessionId,
			Code:      "SERVER_UNAVAILABLE",
			Message:   "ACP microservice is unavailable. Please make sure it is running.",
		})
		return
	}
	defer serverConn.Close()

	// 1. Initialize session on bridge-server. If we have a previously-
	// recorded agent session id (e.g. Claude Code's UUID) for this chat,
	// pass it as resumeSessionId so the agent reuses the same JSONL
	// instead of starting a brand-new session.
	ensureMsg := WsMessage{
		Action:          "ensure_session",
		SessionID:       sessionId,
		WorkspacePath:   workspacePath,
		AgentType:       agentType,
		AcpSessionID:    acpSessionID,
		ResumeSessionID: acpSessionID,
		SystemContext:   systemContext,
	}
	if err := serverConn.WriteJSON(ensureMsg); err != nil {
		log.Printf("[acpx_client] Failed to send ensure_session: %v", err)
		return
	}

	// 2. Start bridging goroutines
	doneCh := make(chan struct{})

	// Read from bridge-server, intercept, and send to client
	go func() {
		defer close(doneCh)
		defer scheduler.Lock.Release(workspacePath)

		for {
			var msg WsMessage
			err := serverConn.ReadJSON(&msg)
			if err != nil {
				log.Printf("[acpx_client] Read from server failed: %v", err)
				break
			}

			// Intercept and update tasks.json status
			if msg.Event == "session_ready" && msg.AgentSessionID != "" {
				// First session_ready: the bridge-server reports the
				// agent-managed session id (e.g. Claude Code's UUID,
				// which is the JSONL filename on disk). Persist it so
				// future opens can resume the same session.
				if chatStore != nil {
					if err := chatStore.UpdateACP(sessionId, msg.AgentSessionID); err != nil {
						log.Printf("[acpx_client] UpdateACP(%s, %s) failed: %v", sessionId, msg.AgentSessionID, err)
					} else {
						log.Printf("[acpx_client] Persisted acpSessionId=%s for chat session %s", msg.AgentSessionID, sessionId)
					}
				}
			} else if msg.Event == "done" {
				log.Printf("[acpx_client] Turn done. Intercepted summary: %s", msg.Summary)
				c.handleTaskSessionDone(workspacePath, taskId, sessionId, msg.Summary, tasksStore)
				scheduler.Lock.Release(workspacePath)
			} else if msg.Event == "error" {
				log.Printf("[acpx_client] Intercepted turn error: %s", msg.Message)
				c.handleTaskSessionError(workspacePath, taskId, sessionId, msg.Message, tasksStore)
				scheduler.Lock.Release(workspacePath)
			}

			// Forward to client
			if err := clientConn.WriteJSON(msg); err != nil {
				log.Printf("[acpx_client] Forward to client failed: %v", err)
				break
			}
		}
	}()

	// Read from client and forward to bridge-server
	go func() {
		for {
			var msg WsMessage
			err := clientConn.ReadJSON(&msg)
			if err != nil {
				log.Printf("[acpx_client] Read from client failed: %v", err)
				break
			}

			// Forward to bridge-server
			if err := serverConn.WriteJSON(msg); err != nil {
				log.Printf("[acpx_client] Forward to server failed: %v", err)
				break
			}
		}
		// Close server connection to stop the other goroutine
		serverConn.Close()
	}()

	// Wait for bridging to end
	<-doneCh
	log.Printf("[acpx_client] Bridge closed for session: %s", sessionId)
}

func (c *AcpxClient) handleTaskSessionDone(workspacePath, taskId, sessionId, summary string, tasksStore *TasksStore) {
	cfg, err := tasksStore.Load(workspacePath)
	if err != nil {
		return
	}

	now := time.Now().UTC()
	updated := false

	for i := range cfg.Tasks {
		task := &cfg.Tasks[i]
		if task.ID == taskId {
			// Update task status and summary
			task.Status = TaskStatusCompleted
			task.CompletedAt = &now
			task.Summary = summary
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
			updated = true
			break
		}
	}

	if updated {
		_ = tasksStore.Save(workspacePath, cfg)
	}
}

func (c *AcpxClient) handleTaskSessionError(workspacePath, taskId, sessionId, errMsg string, tasksStore *TasksStore) {
	cfg, err := tasksStore.Load(workspacePath)
	if err != nil {
		return
	}

	now := time.Now().UTC()
	updated := false

	for i := range cfg.Tasks {
		task := &cfg.Tasks[i]
		if task.ID == taskId {
			task.Status = TaskStatusFailed
			task.UpdatedAt = now

			for j := range task.Sessions {
				sess := &task.Sessions[j]
				if sess.ID == sessionId {
					sess.Status = SessionStatusIdle
					sess.Summary = "Error: " + errMsg
					break
				}
			}
			updated = true
			break
		}
	}

	if updated {
		_ = tasksStore.Save(workspacePath, cfg)
	}
}
