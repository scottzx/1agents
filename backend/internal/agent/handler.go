package agent

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/scottzx/1Agents/backend/internal/workspace"
)

// Handler exposes the REST surface for the chat session and task index.
type Handler struct {
	store      *Store
	tasksStore *TasksStore
	acpxClient *AcpxClient
	scheduler  *Scheduler
}

// NewHandler returns a Handler backed by stores and client.
func NewHandler(store *Store, tasksStore *TasksStore, acpxClient *AcpxClient, scheduler *Scheduler) *Handler {
	return &Handler{
		store:      store,
		tasksStore: tasksStore,
		acpxClient: acpxClient,
		scheduler:  scheduler,
	}
}

// resolveWorkspacePath resolves workspaceID to its absolute physical path on host
func (h *Handler) resolveWorkspacePath(workspaceID string) (string, error) {
	wsHandler := workspace.NewHandler()
	cfg, err := wsHandler.LoadWorkspacesConfig()
	if err != nil {
		return "", err
	}
	for _, ws := range cfg.Workspaces {
		if ws.ID == workspaceID {
			return ws.Path, nil
		}
	}
	return "", fmt.Errorf("workspace not found: %s", workspaceID)
}

// HandleAgentTypes serves GET /api/agent/agent-types
func (h *Handler) HandleAgentTypes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, SupportedAgentTypes)
}

// HandleSessionsRoot handles /api/agent/sessions (root, no trailing slash).
func (h *Handler) HandleSessionsRoot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.list(w, r)
	case http.MethodPost:
		h.create(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleSessionsItem handles /api/agent/sessions/{id} (with trailing slash).
func (h *Handler) HandleSessionsItem(w http.ResponseWriter, r *http.Request) {
	const prefix = "/api/agent/sessions/"
	id := r.URL.Path[len(prefix):]
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	if i := indexByte(id, '/'); i >= 0 {
		http.Error(w, "unsupported sub-path", http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		rec, ok, err := h.store.Get(id)
		if err != nil {
			log.Printf("[agent] get %s: %v", id, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}
		writeJSON(w, rec)
	case http.MethodDelete:
		if err := h.store.Delete(id); err != nil {
			if errors.Is(err, ErrNotFound) {
				http.Error(w, "session not found", http.StatusNotFound)
				return
			}
			log.Printf("[agent] delete %s: %v", id, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	wsID := r.URL.Query().Get("workspace_id")
	if wsID == "" {
		http.Error(w, "workspace_id query parameter is required", http.StatusBadRequest)
		return
	}
	recs, err := h.store.ListByWorkspace(wsID)
	if err != nil {
		log.Printf("[agent] list for %s: %v", wsID, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if recs == nil {
		recs = []ChatSessionRecord{}
	}
	writeJSON(w, recs)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var body IndexRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.WorkspaceID == "" || body.AgentType == "" || body.CcProject == "" ||
		body.CcSessionID == "" || body.SessionKey == "" {
		http.Error(w, "workspace_id, agent_type, cc_project, cc_session_id and session_key are required", http.StatusBadRequest)
		return
	}
	rec := ChatSessionRecord{
		ID:          newID(),
		WorkspaceID: body.WorkspaceID,
		Name:        body.Name,
		AgentType:   body.AgentType,
		CcProject:   body.CcProject,
		CcSessionID: body.CcSessionID,
		SessionKey:  body.SessionKey,
	}
	if err := h.store.Add(rec); err != nil {
		if errors.Is(err, ErrDuplicate) {
			http.Error(w, "session with this id already exists", http.StatusConflict)
			return
		}
		log.Printf("[agent] add: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, rec)
}

// ── Tasks REST API ─────────────────────────────────────────────────────────

// HandleTasksRoot handles GET and POST /api/agent/tasks
func (h *Handler) HandleTasksRoot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		wsID := r.URL.Query().Get("workspace_id")
		if wsID == "" {
			http.Error(w, "workspace_id query parameter is required", http.StatusBadRequest)
			return
		}
		wsPath, err := h.resolveWorkspacePath(wsID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		cfg, err := h.tasksStore.Load(wsPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, cfg.Tasks)

	case http.MethodPost:
		var body struct {
			WorkspaceID  string       `json:"workspace_id"`
			Title        string       `json:"title"`
			ScheduleType ScheduleType `json:"scheduleType"`
			ScheduledAt  *time.Time   `json:"scheduledAt"`
			DependsOn    []string     `json:"dependsOn"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if body.WorkspaceID == "" || body.Title == "" {
			http.Error(w, "workspace_id and title are required", http.StatusBadRequest)
			return
		}
		wsPath, err := h.resolveWorkspacePath(body.WorkspaceID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		cfg, err := h.tasksStore.Load(wsPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		newTask := Task{
			ID:           newID(),
			Title:        body.Title,
			Status:       TaskStatusPending,
			ScheduleType: body.ScheduleType,
			ScheduledAt:  body.ScheduledAt,
			DependsOn:    body.DependsOn,
			CreatedAt:    time.Now().UTC(),
			UpdatedAt:    time.Now().UTC(),
			Sessions:     []SessionMetadata{},
		}
		if newTask.ScheduleType == "" {
			newTask.ScheduleType = ScheduleTypeImmediate
		}

		cfg.Tasks = append(cfg.Tasks, newTask)
		if err := h.tasksStore.Save(wsPath, cfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, newTask)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleTasksItem handles DELETE /api/agent/tasks/{id}
func (h *Handler) HandleTasksItem(w http.ResponseWriter, r *http.Request) {
	const prefix = "/api/agent/tasks/"
	id := r.URL.Path[len(prefix):]
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodDelete:
		wsID := r.URL.Query().Get("workspace_id")
		if wsID == "" {
			http.Error(w, "workspace_id query parameter is required", http.StatusBadRequest)
			return
		}
		wsPath, err := h.resolveWorkspacePath(wsID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		cfg, err := h.tasksStore.Load(wsPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		idx := -1
		for i, t := range cfg.Tasks {
			if t.ID == id {
				idx = i
				break
			}
		}
		if idx == -1 {
			http.Error(w, "task not found", http.StatusNotFound)
			return
		}

		cfg.Tasks = append(cfg.Tasks[:idx], cfg.Tasks[idx+1:]...)
		if err := h.tasksStore.Save(wsPath, cfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"ok": true})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleChatWs handles WebSocket connections at /api/agent/chat/ws
func (h *Handler) HandleChatWs(w http.ResponseWriter, r *http.Request) {
	wsID := r.URL.Query().Get("workspace_id")
	taskId := r.URL.Query().Get("task_id")
	sessionId := r.URL.Query().Get("session_id")
	agentType := r.URL.Query().Get("agent_type")

	if wsID == "" || sessionId == "" || agentType == "" {
		http.Error(w, "workspace_id, session_id, and agent_type query parameters are required", http.StatusBadRequest)
		return
	}

	wsPath, err := h.resolveWorkspacePath(wsID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	var systemContext string
	if taskId != "" {
		// 1. Load tasks configuration to find task and aggregate prior summaries
		cfg, err := h.tasksStore.Load(wsPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var targetTask *Task
		for i := range cfg.Tasks {
			if cfg.Tasks[i].ID == taskId {
				targetTask = &cfg.Tasks[i]
				break
			}
		}

		if targetTask == nil {
			http.Error(w, "task not found", http.StatusNotFound)
			return
		}

		// Context Chaining: Aggregate prior completed session summaries
		if len(targetTask.Sessions) > 0 {
			var historyLines []string
			historyLines = append(historyLines, fmt.Sprintf("[Task Context History]\nThe user is working on the task: %q.", targetTask.Title))
			historyLines = append(historyLines, "Previous sessions have already achieved the following:")
			count := 1
			for _, s := range targetTask.Sessions {
				if s.Summary != "" {
					historyLines = append(historyLines, fmt.Sprintf("- Session %d (%s): %s", count, s.AgentType, s.Summary))
					count++
				}
			}
			historyLines = append(historyLines, "Please continue the task from here, focusing on any requested adjustments.")
			systemContext = strings.Join(historyLines, "\n")
		}

		// Check state concurrency lock
		if targetTask.Status != TaskStatusRunning {
			// Try to acquire the execution lock
			if !h.scheduler.Lock.TryAcquire(wsPath, taskId) {
				// If already occupied, return 409 conflict
				http.Error(w, "Another session is already running in this workspace", http.StatusConflict)
				return
			}
			// Update task state to running
			targetTask.Status = TaskStatusRunning
			now := time.Now().UTC()
			targetTask.StartedAt = &now
			targetTask.UpdatedAt = now

			// Update or create session metadata
			sessionExists := false
			for i := range targetTask.Sessions {
				if targetTask.Sessions[i].ID == sessionId {
					targetTask.Sessions[i].Status = SessionStatusRunning
					sessionExists = true
					break
				}
			}
			if !sessionExists {
				targetTask.Sessions = append(targetTask.Sessions, SessionMetadata{
					ID:        sessionId,
					Kind:      SessionKindChat,
					Name:      "智能体排查与修复",
					AgentType: agentType,
					Status:    SessionStatusRunning,
					CreatedAt: now,
				})
			}

			_ = h.tasksStore.Save(wsPath, cfg)
		}
		log.Printf("[agent] Bridging Chat UI WebSocket for task %s, session %s", taskId, sessionId)
	} else {
		log.Printf("[agent] Bridging Chat UI WebSocket for session %s (no task)", sessionId)
	}

	h.acpxClient.Bridge(w, r, wsPath, taskId, sessionId, agentType, systemContext, h.scheduler, h.tasksStore)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[agent] json encode: %v", err)
	}
}

// newID returns a random 16-byte hex string.
func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "agent-fallback-id"
	}
	return hex.EncodeToString(b[:])
}

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}
