package agent

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type ActiveBridge struct {
	SessionID     string
	WorkspacePath string
	mu            sync.Mutex
	ClientConn    *websocket.Conn
	ServerConn    *websocket.Conn
	MsgChan       chan WsMessage
	IsDone        bool
}

type AcpxClient struct {
	serverPort int
	mu         sync.Mutex
	bridges    map[string]*ActiveBridge
}

func NewAcpxClient(serverPort int) *AcpxClient {
	return &AcpxClient{
		serverPort: serverPort,
		bridges:    make(map[string]*ActiveBridge),
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
	ToolCallID      string          `json:"toolCallId,omitempty"`
	IsError         bool            `json:"isError,omitempty"`
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

	c.mu.Lock()
	if c.bridges == nil {
		c.bridges = make(map[string]*ActiveBridge)
	}

	bridge, exists := c.bridges[sessionId]
	if exists {
		log.Printf("[acpx_client] Reconnecting client to existing active bridge for session: %s", sessionId)
		bridge.mu.Lock()
		if bridge.ClientConn != nil {
			_ = bridge.ClientConn.Close() // Close old client connection
		}
		bridge.ClientConn = clientConn
		bridge.mu.Unlock()
		c.mu.Unlock()

		// Send ensure_session again so the bridge-server updates its WS connection
		ensureMsg := WsMessage{
			Action:          "ensure_session",
			SessionID:       sessionId,
			WorkspacePath:   workspacePath,
			AgentType:       agentType,
			AcpSessionID:    acpSessionID,
			ResumeSessionID: acpSessionID,
			SystemContext:   systemContext,
		}
		bridge.mu.Lock()
		if bridge.ServerConn != nil {
			_ = bridge.ServerConn.WriteJSON(ensureMsg)
		}
		bridge.mu.Unlock()

		// Start reading from the new client connection and forwarding to the existing server connection
		c.readFromClientLoop(bridge, clientConn)
		return
	}

	// Create new bridge
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
		c.mu.Unlock()
		_ = clientConn.Close()
		return
	}

	bridge = &ActiveBridge{
		SessionID:     sessionId,
		WorkspacePath: workspacePath,
		ClientConn:    clientConn,
		ServerConn:    serverConn,
		MsgChan:       make(chan WsMessage, 100),
	}
	c.bridges[sessionId] = bridge
	c.mu.Unlock()

	// 1. Initialize session on bridge-server
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
		c.cleanupBridge(sessionId)
		_ = serverConn.Close()
		_ = clientConn.Close()
		return
	}

	// Start server connection reader loop
	go c.readFromServerLoop(bridge, scheduler, tasksStore, chatStore, taskId)

	// Start write helper loop for writing to the active client connection
	go c.writeToClientLoop(bridge)

	// Read from client and forward to server connection
	c.readFromClientLoop(bridge, clientConn)
}

func (c *AcpxClient) readFromServerLoop(bridge *ActiveBridge, scheduler *Scheduler, tasksStore *TasksStore, chatStore *Store, taskId string) {
	defer func() {
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
		var msg WsMessage
		err := bridge.ServerConn.ReadJSON(&msg)
		if err != nil {
			log.Printf("[acpx_client] Read from server failed for session %s: %v", bridge.SessionID, err)
			break
		}

		// Intercept and update status
		if msg.Event == "session_ready" && msg.AgentSessionID != "" {
			if chatStore != nil {
				if err := chatStore.UpdateACP(bridge.SessionID, msg.AgentSessionID); err != nil {
					log.Printf("[acpx_client] UpdateACP(%s, %s) failed: %v", bridge.SessionID, msg.AgentSessionID, err)
				} else {
					log.Printf("[acpx_client] Persisted acpSessionId=%s for chat session %s", msg.AgentSessionID, bridge.SessionID)
				}
			}
		} else if msg.Event == "done" {
			log.Printf("[acpx_client] Turn done for session %s. Intercepted summary: %s", bridge.SessionID, msg.Summary)
			c.handleTaskSessionDone(bridge.WorkspacePath, taskId, bridge.SessionID, msg.Summary, tasksStore)
			scheduler.Lock.Release(bridge.WorkspacePath)
		} else if msg.Event == "error" {
			log.Printf("[acpx_client] Intercepted turn error for session %s: %s", bridge.SessionID, msg.Message)
			c.handleTaskSessionError(bridge.WorkspacePath, taskId, bridge.SessionID, msg.Message, tasksStore)
			scheduler.Lock.Release(bridge.WorkspacePath)
		}

		// Send to client write channel
		bridge.mu.Lock()
		if !bridge.IsDone {
			select {
			case bridge.MsgChan <- msg:
			default:
				log.Printf("[acpx_client] MsgChan full, dropping message for session %s", bridge.SessionID)
			}
		}
		bridge.mu.Unlock()
	}
}

func (c *AcpxClient) writeToClientLoop(bridge *ActiveBridge) {
	for msg := range bridge.MsgChan {
		bridge.mu.Lock()
		clientConn := bridge.ClientConn
		bridge.mu.Unlock()

		if clientConn != nil {
			if err := clientConn.WriteJSON(msg); err != nil {
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
		var msg WsMessage
		err := clientConn.ReadJSON(&msg)
		if err != nil {
			log.Printf("[acpx_client] Read from client connection failed for session %s: %v", bridge.SessionID, err)
			break
		}

		bridge.mu.Lock()
		serverConn := bridge.ServerConn
		isDone := bridge.IsDone
		bridge.mu.Unlock()

		if isDone {
			break
		}

		if serverConn != nil {
			if err := serverConn.WriteJSON(msg); err != nil {
				log.Printf("[acpx_client] Forward client message to server failed for session %s: %v", bridge.SessionID, err)
				break
			}
		}
	}
}

func (c *AcpxClient) cleanupBridge(sessionId string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.bridges != nil {
		delete(c.bridges, sessionId)
	}
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
