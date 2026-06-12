package agent

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
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
		if rec.AcpSessionID != "" {
			name := rec.Name
			if name == "" || name == "聊天会话" || name == "新建会话" || strings.HasPrefix(name, "Chat") || strings.HasSuffix(name, "会话") {
				if wsPath, err := h.resolveWorkspacePath(rec.WorkspaceID); err == nil {
					if title := resolveAcpSessionTitle(wsPath, rec.AcpSessionID, name); title != "" && title != name {
						rec.Name = title
						go func(id, newName string) {
							_ = h.store.UpdateName(id, newName)
						}(rec.ID, title)
					}
				}
			}
		}
		writeJSON(w, rec)
	case http.MethodPatch:
		// PATCH body: { "permission_mode": "approve-reads" | "approve-all" | "deny-all" }
		// Used by the Composer's permission-mode toggle. Validates the
		// enum to keep bad client data out of the JSON store (since the
		// bridge-server later trusts this string).
		var body struct {
			PermissionMode *string `json:"permission_mode,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if body.PermissionMode != nil {
			mode := *body.PermissionMode
			if !isValidPermissionMode(mode) {
				http.Error(w, "permission_mode must be approve-reads, approve-all, or deny-all", http.StatusBadRequest)
				return
			}
			if err := h.store.UpdatePermissionMode(id, mode); err != nil {
				if errors.Is(err, ErrNotFound) {
					http.Error(w, "session not found", http.StatusNotFound)
					return
				}
				log.Printf("[agent] update permission_mode %s: %v", id, err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		rec, ok, err := h.store.Get(id)
		if err != nil {
			log.Printf("[agent] get %s after patch: %v", id, err)
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

	var wsPath string
	if len(recs) > 0 {
		if path, err := h.resolveWorkspacePath(wsID); err == nil {
			wsPath = path
		}
	}

	for i := range recs {
		rec := &recs[i]
		if rec.AcpSessionID != "" {
			name := rec.Name
			if name == "" || name == "聊天会话" || name == "新建会话" || strings.HasPrefix(name, "Chat") || strings.HasSuffix(name, "会话") {
				if title := resolveAcpSessionTitle(wsPath, rec.AcpSessionID, name); title != "" && title != name {
					rec.Name = title
					go func(id, newName string) {
						_ = h.store.UpdateName(id, newName)
					}(rec.ID, title)
				}
			}
		}
	}

	writeJSON(w, recs)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var body IndexRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	// Only the workspace and agent type are mandatory. The cc_* fields and
	// session_key identify the cc-connect / IM side and stay empty for
	// ACP-only sessions (e.g. task timeline sessions, which talk to the
	// agent purely through the chat WS bridge).
	if body.WorkspaceID == "" || body.AgentType == "" {
		http.Error(w, "workspace_id and agent_type are required", http.StatusBadRequest)
		return
	}
	rec := ChatSessionRecord{
		ID:          newID(),
		WorkspaceID: body.WorkspaceID,
		Name:        body.Name,
		AgentType:   body.AgentType,
		TaskID:      body.TaskID,
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
			WorkspaceID        string       `json:"workspace_id"`
			Title              string       `json:"title"`
			Description        string       `json:"description"`
			AcceptanceCriteria string       `json:"acceptanceCriteria"`
			Priority           string       `json:"priority"`
			Assignee           string       `json:"assignee"`
			Labels             []string     `json:"labels"`
			ParentID           string       `json:"parentId"`
			Milestone          string       `json:"milestone"`
			Sprint             string       `json:"sprint"`
			Recurrence         *Recurrence  `json:"recurrence"`
			MaxRetries         *int         `json:"maxRetries"`
			ScheduleType       ScheduleType `json:"scheduleType"`
			ScheduledAt        *time.Time   `json:"scheduledAt"`
			PlannedStart       *time.Time   `json:"plannedStart"`
			PlannedEnd         *time.Time   `json:"plannedEnd"`
			DependsOn          []string     `json:"dependsOn"`
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

		maxRetries := 1
		if body.MaxRetries != nil && *body.MaxRetries >= 0 {
			maxRetries = *body.MaxRetries
		}
		newTask := Task{
			ID:                 newID(),
			Title:              body.Title,
			Description:        body.Description,
			AcceptanceCriteria: body.AcceptanceCriteria,
			IssueState:         IssueOpen,
			Status:             TaskStatusPending,
			Priority:           Priority(body.Priority),
			Assignee:           body.Assignee,
			Labels:             body.Labels,
			ParentID:           body.ParentID,
			Milestone:          body.Milestone,
			Sprint:             body.Sprint,
			Recurrence:         body.Recurrence,
			MaxRetries:         maxRetries,
			ScheduleType:       body.ScheduleType,
			ScheduledAt:        body.ScheduledAt,
			PlannedStart:       body.PlannedStart,
			PlannedEnd:         body.PlannedEnd,
			DependsOn:          body.DependsOn,
			CreatedAt:          time.Now().UTC(),
			UpdatedAt:          time.Now().UTC(),
			Replies:            []Reply{},
			Sessions:           []SessionMetadata{},
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

// HandleTasksItem handles /api/agent/tasks/{id} and its sub-resources:
//
//	GET    /api/agent/tasks/{id}          → single task incl. description + replies
//	PATCH  /api/agent/tasks/{id}          → edit description / toggle issue state
//	DELETE /api/agent/tasks/{id}          → remove task (legacy, needs workspace_id)
//	POST   /api/agent/tasks/{id}/replies  → append a user reply to the timeline
func (h *Handler) HandleTasksItem(w http.ResponseWriter, r *http.Request) {
	const prefix = "/api/agent/tasks/"
	rest := r.URL.Path[len(prefix):]
	if rest == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	id := rest
	sub := ""
	if i := indexByte(rest, '/'); i >= 0 {
		id, sub = rest[:i], rest[i+1:]
	}

	if sub == "replies" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.handleTaskReplyCreate(w, r, id)
		return
	}
	if sub != "" {
		http.Error(w, "unsupported sub-path", http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		task, ok, err := h.tasksStore.GetTask(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "task not found", http.StatusNotFound)
			return
		}
		writeJSON(w, task)

	case http.MethodPatch:
		h.handleTaskPatch(w, r, id)

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

// handleTaskPatch applies partial edits to issue and PM fields. Only fields
// present in the body are touched.
func (h *Handler) handleTaskPatch(w http.ResponseWriter, r *http.Request, id string) {
	var body struct {
		Description        *string      `json:"description,omitempty"`
		IssueState         *string      `json:"issueState,omitempty"`
		AcceptanceCriteria *string      `json:"acceptanceCriteria,omitempty"`
		Priority           *string      `json:"priority,omitempty"`
		Assignee           *string      `json:"assignee,omitempty"`
		Labels             *[]string    `json:"labels,omitempty"`
		ParentID           *string      `json:"parentId,omitempty"`
		Milestone          *string      `json:"milestone,omitempty"`
		Sprint             *string      `json:"sprint,omitempty"`
		Recurrence         **Recurrence `json:"recurrence,omitempty"`
		MaxRetries         *int         `json:"maxRetries,omitempty"`
		PlannedStart       *time.Time   `json:"plannedStart,omitempty"`
		PlannedEnd         *time.Time   `json:"plannedEnd,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.IssueState != nil {
		state := IssueState(*body.IssueState)
		if state != IssueOpen && state != IssueClosed {
			http.Error(w, "issueState must be open or closed", http.StatusBadRequest)
			return
		}
	}
	if body.Priority != nil {
		switch Priority(*body.Priority) {
		case PriorityUrgent, PriorityHigh, PriorityMedium, PriorityLow:
		default:
			http.Error(w, "priority must be urgent, high, medium or low", http.StatusBadRequest)
			return
		}
	}

	// Whole-config load/mutate/save (same path the CLI uses), so a single
	// PATCH can touch any mix of fields atomically.
	existing, ok, err := h.tasksStore.GetTask(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	cfg, err := h.tasksStore.Load(existing.WorkspacePath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var target *Task
	for i := range cfg.Tasks {
		if cfg.Tasks[i].ID == id {
			target = &cfg.Tasks[i]
			break
		}
	}
	if target == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	if body.Description != nil {
		target.Description = *body.Description
	}
	if body.IssueState != nil {
		target.IssueState = IssueState(*body.IssueState)
	}
	if body.AcceptanceCriteria != nil {
		target.AcceptanceCriteria = *body.AcceptanceCriteria
	}
	if body.Priority != nil {
		target.Priority = Priority(*body.Priority)
	}
	if body.Assignee != nil {
		target.Assignee = *body.Assignee
	}
	if body.Labels != nil {
		target.Labels = *body.Labels
	}
	if body.ParentID != nil {
		target.ParentID = *body.ParentID
	}
	if body.Milestone != nil {
		target.Milestone = *body.Milestone
	}
	if body.Sprint != nil {
		target.Sprint = *body.Sprint
	}
	if body.Recurrence != nil {
		target.Recurrence = *body.Recurrence
	}
	if body.MaxRetries != nil && *body.MaxRetries >= 0 {
		target.MaxRetries = *body.MaxRetries
	}
	if body.PlannedStart != nil {
		target.PlannedStart = body.PlannedStart
	}
	if body.PlannedEnd != nil {
		target.PlannedEnd = body.PlannedEnd
	}
	target.UpdatedAt = time.Now().UTC()

	if err := h.tasksStore.Save(existing.WorkspacePath, cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	task, ok, err := h.tasksStore.GetTask(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	writeJSON(w, task)
}

// handleTaskReplyCreate appends a user reply to the task timeline.
// Closed-issue semantics (issue-model decision H): pure comments are
// allowed, opening or following up sessions is rejected with 422.
func (h *Handler) handleTaskReplyCreate(w http.ResponseWriter, r *http.Request, id string) {
	var body struct {
		Text      string `json:"text"`
		Mode      string `json:"mode"`
		InReplyTo string `json:"inReplyTo"`
		Author    string `json:"author"`
		AgentType string `json:"agentType"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(body.Text) == "" {
		http.Error(w, "text is required", http.StatusBadRequest)
		return
	}
	mode := ReplyMode(body.Mode)
	if mode == "" {
		mode = ModePureComment
	}
	if mode != ModeNewSession && mode != ModeFollowUp && mode != ModePureComment {
		http.Error(w, "mode must be new, follow_up or pure_comment", http.StatusBadRequest)
		return
	}

	task, ok, err := h.tasksStore.GetTask(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	if task.IssueState == IssueClosed && mode != ModePureComment {
		http.Error(w, "issue is closed: reopen it before starting sessions", http.StatusUnprocessableEntity)
		return
	}

	authorName := body.Author
	if authorName == "" {
		authorName = "user"
	}
	reply, err := h.tasksStore.AppendReply(id, Reply{
		Author:    Author{Kind: "user", Name: authorName},
		AgentType: body.AgentType,
		Text:      body.Text,
		InReplyTo: body.InReplyTo,
		Mode:      mode,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, reply)
}

// HandleChatWs handles WebSocket connections at /api/agent/chat/ws
func (h *Handler) HandleChatWs(w http.ResponseWriter, r *http.Request) {
	wsID := r.URL.Query().Get("workspace_id")
	taskId := r.URL.Query().Get("task_id")
	sessionId := r.URL.Query().Get("session_id")
	agentType := r.URL.Query().Get("agent_type")
	// reply_id links this session to the timeline reply that triggered it
	// (issue-model §7.2); optional for sessions outside any task.
	replyID := r.URL.Query().Get("reply_id")

	if wsID == "" || sessionId == "" || agentType == "" {
		http.Error(w, "workspace_id, session_id, and agent_type query parameters are required", http.StatusBadRequest)
		return
	}

	wsPath, err := h.resolveWorkspacePath(wsID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Look up the 1agents-side chat record early: a previously-recorded
	// agent session id (e.g. Claude Code's UUID) means this is a resume,
	// which both skips the background injection (issue-model decision G)
	// and is passed to the bridge as resumeSessionId.
	var acpSessionID string
	if rec, ok, err := h.store.Get(sessionId); err == nil && ok {
		acpSessionID = rec.AcpSessionID
	}

	var systemContext string
	if taskId != "" {
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

		// Issue background injection (issue-model §9): description + the
		// full reply timeline, injected only when this is a NEW session.
		// Resumed sessions already carry their own conversation history.
		if acpSessionID == "" {
			systemContext = buildIssueBackground(targetTask, wsPath)
		}

		// Link the triggering reply to this session (Reply.SessionRef) and
		// the chat record to the task (sessions.task_id, which also powers
		// the sidebar badge).
		if replyID != "" {
			if err := h.tasksStore.SetReplySession(replyID, sessionId); err != nil {
				log.Printf("[agent] SetReplySession(%s, %s): %v", replyID, sessionId, err)
			}
		}
		if err := h.store.UpdateTask(sessionId, taskId); err != nil && !errors.Is(err, ErrNotFound) {
			log.Printf("[agent] UpdateTask(%s, %s): %v", sessionId, taskId, err)
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

	h.acpxClient.Bridge(w, r, wsPath, taskId, sessionId, agentType, systemContext, h.scheduler, h.tasksStore, h.store, acpSessionID, replyID)
}

// buildIssueBackground renders the issue-model §9 plain-text background
// block: task header, Markdown description, and the full reply timeline in
// chronological order. Injected as a single system message before the
// user's first request in a new session.
func buildIssueBackground(t *Task, wsPath string) string {
	var b strings.Builder
	b.WriteString("=== ISSUE BACKGROUND ===\n")
	fmt.Fprintf(&b, "Task ID: %s\n", t.ID)
	fmt.Fprintf(&b, "Title: %s\n", t.Title)
	issueState := t.IssueState
	if issueState == "" {
		issueState = IssueOpen
	}
	fmt.Fprintf(&b, "Issue State: %s\n", issueState)
	fmt.Fprintf(&b, "Workflow Status: %s\n", t.Status)
	fmt.Fprintf(&b, "Workspace: %s\n", wsPath)
	if t.Description != "" {
		fmt.Fprintf(&b, "\nDescription:\n%s\n", t.Description)
	}
	if t.AcceptanceCriteria != "" {
		fmt.Fprintf(&b, "\n=== ACCEPTANCE CRITERIA ===\n%s\n", t.AcceptanceCriteria)
	}
	if len(t.Replies) > 0 {
		fmt.Fprintf(&b, "\nReplies (chronological, %d entries):\n---\n", len(t.Replies))
		for i, rp := range t.Replies {
			who := rp.Author.Kind
			if rp.Author.Kind == "agent" {
				agentLabel := rp.AgentType
				if agentLabel == "" {
					agentLabel = rp.Author.Name
				}
				if rp.SessionRef != "" {
					who = fmt.Sprintf("agent (%s, session #%s)", agentLabel, rp.SessionRef)
				} else {
					who = fmt.Sprintf("agent (%s)", agentLabel)
				}
			} else if rp.Author.Name != "" {
				who = rp.Author.Name
			}
			fmt.Fprintf(&b, "[%d] %s @ %s\n", i+1, who, rp.CreatedAt.UTC().Format(time.RFC3339))
			for _, line := range strings.Split(rp.Text, "\n") {
				fmt.Fprintf(&b, "    %s\n", line)
			}
			if i < len(t.Replies)-1 {
				b.WriteString("\n")
			}
		}
		b.WriteString("---\n")
	}
	b.WriteString("End of background.")
	return b.String()
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

// isValidPermissionMode mirrors the bridge-server's accepted mode strings.
// Kept here (not in types.go) because it's only consumed by the PATCH
// validator above.
func isValidPermissionMode(mode string) bool {
	switch mode {
	case "approve-reads", "approve-all", "deny-all":
		return true
	default:
		return false
	}
}

func getProjectSlug(path string) string {
	var sb strings.Builder
	for _, r := range path {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('-')
		}
	}
	return sb.String()
}

func resolveAcpSessionTitle(workspacePath, acpSessionID, defaultName string) string {
	if acpSessionID == "" {
		return defaultName
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return defaultName
	}
	slug := getProjectSlug(workspacePath)
	jsonlPath := filepath.Join(home, ".claude", "projects", slug, acpSessionID+".jsonl")

	file, err := os.Open(jsonlPath)
	if err != nil {
		return defaultName
	}
	defer file.Close()

	var resolvedTitle string
	var foundSlug string

	dec := json.NewDecoder(file)
	for {
		var line map[string]any
		if err := dec.Decode(&line); err != nil {
			break
		}
		if title, ok := line["aiTitle"].(string); ok && title != "" {
			resolvedTitle = title
		}
		if slg, ok := line["slug"].(string); ok && slg != "" {
			foundSlug = slg
		}
	}

	if resolvedTitle != "" {
		return resolvedTitle
	}
	if foundSlug != "" {
		return foundSlug
	}
	return defaultName
}
