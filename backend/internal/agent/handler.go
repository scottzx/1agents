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
	"strconv"
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
	catalog    *CatalogStore
	// selfBaseURL is this daemon's own loopback HTTP base (e.g.
	// http://127.0.0.1:8080), injected into the AI Project Manager's
	// task-tool MCP subprocess so it can call back into the task API.
	selfBaseURL string
}

// NewHandler returns a Handler backed by stores and client. selfBaseURL is the
// daemon's own loopback HTTP base used by the PM task-tool MCP subprocess.
func NewHandler(store *Store, tasksStore *TasksStore, acpxClient *AcpxClient, scheduler *Scheduler, catalog *CatalogStore, selfBaseURL string) *Handler {
	return &Handler{
		store:       store,
		tasksStore:  tasksStore,
		acpxClient:  acpxClient,
		scheduler:   scheduler,
		catalog:     catalog,
		selfBaseURL: selfBaseURL,
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

// HandleAgentCatalog serves GET /api/agent/catalog.
//
// Returns the cached per-host install + capability snapshot for every real
// agent application. A ?refresh=1 query forces a fresh PATH re-probe before
// responding (the manual refresh endpoint).
func (h *Handler) HandleAgentCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var statuses []AgentStatus
	if r.URL.Query().Get("refresh") != "" {
		statuses = h.catalog.Scan()
	} else {
		statuses = h.catalog.Snapshot()
	}
	writeJSON(w, statuses)
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
			Archived       *bool   `json:"archived,omitempty"`
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
		if body.Archived != nil {
			if err := h.store.SetArchived(id, *body.Archived); err != nil {
				if errors.Is(err, ErrNotFound) {
					http.Error(w, "session not found", http.StatusNotFound)
					return
				}
				log.Printf("[agent] set archived %s: %v", id, err)
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
	// The sidebar lists active sessions only; the 会话 archive view passes
	// include_archived=1 to also surface soft-deleted (archived) sessions.
	includeArchived := r.URL.Query().Get("include_archived") == "1"
	recs, err := h.store.ListByWorkspace(wsID, includeArchived)
	if err != nil {
		log.Printf("[agent] list for %s: %v", wsID, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if recs == nil {
		recs = []ChatSessionRecord{}
	}

	// Headless auto-run sessions execute silently in the backend; keep them
	// out of the sidebar so an AI-executed task doesn't spawn a chat box.
	// They stay resolvable by id (Get), so "查看详情" can still resume them.
	filtered := recs[:0]
	for _, rec := range recs {
		if rec.Role == SessionRoleAuto {
			continue
		}
		filtered = append(filtered, rec)
	}
	recs = filtered

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
		Role:        body.Role,
	}
	// AI Project Manager sessions default to approve-all: the task tools are
	// already hard-locked to this project via env injection, so auto-approving
	// keeps the conversation flowing instead of stalling on a permission prompt
	// for every create_task/update_task. The user can still switch the mode
	// manually afterwards (persisted via set_permission_mode).
	if isProjectManagerRole(rec.Role) {
		rec.PermissionMode = "approve-all"
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
			Type               string       `json:"type"`
			Recurrence         *Recurrence  `json:"recurrence"`
			MaxRetries         *int         `json:"maxRetries"`
			ScheduleType       ScheduleType `json:"scheduleType"`
			ScheduledAt        *time.Time   `json:"scheduledAt"`
			PlannedStart       *time.Time   `json:"plannedStart"`
			PlannedEnd         *time.Time   `json:"plannedEnd"`
			DependsOn          []string     `json:"dependsOn"`
			Links              []TaskLink   `json:"links"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if body.WorkspaceID == "" || body.Title == "" {
			http.Error(w, "workspace_id and title are required", http.StatusBadRequest)
			return
		}
		// assignee selects the executing agent; empty falls back to
		// DefaultAgentType at run time (runner.go). Reject unknown values so a
		// typo from the PM tool surfaces instead of silently defaulting.
		if body.Assignee != "" && !IsSupportedAgentType(body.Assignee) {
			http.Error(w, "unknown assignee agent type: "+body.Assignee, http.StatusBadRequest)
			return
		}
		wsPath, err := h.resolveWorkspacePath(body.WorkspaceID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
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
			Type:               TaskType(body.Type),
			Recurrence:         body.Recurrence,
			MaxRetries:         maxRetries,
			ScheduleType:       body.ScheduleType,
			ScheduledAt:        body.ScheduledAt,
			PlannedStart:       body.PlannedStart,
			PlannedEnd:         body.PlannedEnd,
			DependsOn:          body.DependsOn,
			Links:              body.Links,
			CreatedAt:          time.Now().UTC(),
			UpdatedAt:          time.Now().UTC(),
			Replies:            []Reply{},
			Sessions:           []SessionMetadata{},
		}
		if newTask.ScheduleType == "" {
			newTask.ScheduleType = ScheduleTypeImmediate
		}

		if err := h.tasksStore.Mutate(wsPath, func(cfg *TasksConfig) bool {
			cfg.Tasks = append(cfg.Tasks, newTask)
			return true
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Keep the roadmap's milestone list complete: a task may name a
		// brand-new milestone that has no metadata row yet.
		if newTask.Milestone != "" {
			_ = h.tasksStore.EnsureMilestone(wsPath, newTask.Milestone)
		}
		// Save assigns the short number (#N) on the stored row, so re-fetch
		// rather than returning the pre-save copy.
		saved, _, _ := h.tasksStore.GetTask(newTask.ID)
		writeJSON(w, saved)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleTaskResolve resolves a permalink reference to a task:
//
//	GET /api/agent/tasks/resolve?project={name|id}&number={n}
//	  → {workspaceId, task}   (404 when the project or number is unknown)
//
// The frontend uses it to turn `#N` / `项目名#N` references and
// /{project}/tasks/{number} deep links into an in-app task view. project may be
// a display name or a project id; number is the per-project short id (#N).
func (h *Handler) HandleTaskResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	project := r.URL.Query().Get("project")
	numStr := r.URL.Query().Get("number")
	number, err := strconv.Atoi(numStr)
	if project == "" || err != nil || number <= 0 {
		http.Error(w, "project and a positive number are required", http.StatusBadRequest)
		return
	}
	task, workspaceID, ok, err := h.tasksStore.ResolveByNumber(project, number)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	// Prefer the workspace-registry id (what the frontend keys workspaces by)
	// over the raw meta project id. The two are normally identical (project id
	// == workspace id), but a project created lazily before its workspace is
	// synced gets a random id; mapping the task's path back to the registry
	// keeps deep-link navigation working regardless.
	if wsID := h.workspaceIDForPath(task.WorkspacePath); wsID != "" {
		workspaceID = wsID
	}
	writeJSON(w, map[string]any{"workspaceId": workspaceID, "task": task})
}

// workspaceIDForPath reverse-maps an absolute workspace path to its registry
// workspace id, or "" when no workspace owns that path.
func (h *Handler) workspaceIDForPath(path string) string {
	if path == "" {
		return ""
	}
	cfg, err := workspace.NewHandler().LoadWorkspacesConfig()
	if err != nil {
		return ""
	}
	for _, ws := range cfg.Workspaces {
		if ws.Path == path {
			return ws.ID
		}
	}
	return ""
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
		Title              *string      `json:"title,omitempty"`
		Description        *string      `json:"description,omitempty"`
		IssueState         *string      `json:"issueState,omitempty"`
		Status             *string      `json:"status,omitempty"`
		AcceptanceCriteria *string      `json:"acceptanceCriteria,omitempty"`
		Priority           *string      `json:"priority,omitempty"`
		Assignee           *string      `json:"assignee,omitempty"`
		Labels             *[]string    `json:"labels,omitempty"`
		ParentID           *string      `json:"parentId,omitempty"`
		Milestone          *string      `json:"milestone,omitempty"`
		Sprint             *string      `json:"sprint,omitempty"`
		Type               *string      `json:"type,omitempty"`
		Recurrence         **Recurrence `json:"recurrence,omitempty"`
		MaxRetries         *int         `json:"maxRetries,omitempty"`
		PlannedStart       *time.Time   `json:"plannedStart,omitempty"`
		PlannedEnd         *time.Time   `json:"plannedEnd,omitempty"`
		Links              *[]TaskLink  `json:"links,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.Title != nil && strings.TrimSpace(*body.Title) == "" {
		http.Error(w, "title must not be empty", http.StatusBadRequest)
		return
	}
	if body.IssueState != nil {
		state := IssueState(*body.IssueState)
		if state != IssueOpen && state != IssueClosed {
			http.Error(w, "issueState must be open or closed", http.StatusBadRequest)
			return
		}
	}
	if body.Status != nil {
		// Manual status changes are limited to terminal states the scheduler
		// skips (completed/cancelled). Runnable states (pending/queued/running)
		// stay scheduler-owned, so the Kanban board can never arm execution by
		// drag — only retire a card.
		switch TaskStatus(*body.Status) {
		case TaskStatusCompleted, TaskStatusCancelled:
		default:
			http.Error(w, "status may only be set to completed or cancelled", http.StatusBadRequest)
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
	if body.Assignee != nil && *body.Assignee != "" && !IsSupportedAgentType(*body.Assignee) {
		http.Error(w, "unknown assignee agent type: "+*body.Assignee, http.StatusBadRequest)
		return
	}

	// Whole-config load/mutate/save (same path the CLI uses), so a single
	// PATCH can touch any mix of fields atomically. Mutate serializes the
	// cycle so a concurrent scheduler/runner Save can't clobber the edit.
	existing, ok, err := h.tasksStore.GetTask(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	found := false
	if err := h.tasksStore.Mutate(existing.WorkspacePath, func(cfg *TasksConfig) bool {
		var target *Task
		for i := range cfg.Tasks {
			if cfg.Tasks[i].ID == id {
				target = &cfg.Tasks[i]
				break
			}
		}
		if target == nil {
			return false
		}
		found = true

		if body.Title != nil {
			target.Title = strings.TrimSpace(*body.Title)
		}
		if body.Description != nil {
			target.Description = *body.Description
		}
		if body.IssueState != nil {
			target.IssueState = IssueState(*body.IssueState)
		}
		if body.Status != nil {
			target.Status = TaskStatus(*body.Status)
			if target.Status == TaskStatusCompleted && target.CompletedAt == nil {
				now := time.Now().UTC()
				target.CompletedAt = &now
			}
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
		if body.Type != nil {
			target.Type = TaskType(*body.Type)
		}
		if body.Links != nil {
			target.Links = *body.Links
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
		return true
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	if body.Milestone != nil && *body.Milestone != "" {
		_ = h.tasksStore.EnsureMilestone(existing.WorkspacePath, *body.Milestone)
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
		Text       string `json:"text"`
		Mode       string `json:"mode"`
		InReplyTo  string `json:"inReplyTo"`
		SessionRef string `json:"sessionRef"`
		Author     string `json:"author"`
		AgentType  string `json:"agentType"`
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
		Author:     Author{Kind: "user", Name: authorName},
		AgentType:  body.AgentType,
		Text:       body.Text,
		InReplyTo:  body.InReplyTo,
		SessionRef: body.SessionRef,
		Mode:       mode,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, reply)
}

// ── Milestones REST API ────────────────────────────────────────────────────

// HandleMilestonesRoot handles GET and POST /api/agent/milestones.
//
//	GET  /api/agent/milestones?workspace_id=…  → roadmap-ordered list w/ counts
//	POST /api/agent/milestones                 → create a milestone
func (h *Handler) HandleMilestonesRoot(w http.ResponseWriter, r *http.Request) {
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
		list, err := h.tasksStore.ListMilestones(wsPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, list)

	case http.MethodPost:
		var body struct {
			WorkspaceID   string     `json:"workspace_id"`
			Name          string     `json:"name"`
			Description   string     `json:"description"`
			TargetDate    *time.Time `json:"targetDate"`
			PredecessorID string     `json:"predecessorId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if body.WorkspaceID == "" || strings.TrimSpace(body.Name) == "" {
			http.Error(w, "workspace_id and name are required", http.StatusBadRequest)
			return
		}
		wsPath, err := h.resolveWorkspacePath(body.WorkspaceID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		ms, err := h.tasksStore.CreateMilestone(wsPath, strings.TrimSpace(body.Name), body.Description, body.TargetDate, body.PredecessorID)
		if err != nil {
			if errors.Is(err, ErrMilestoneExists) {
				http.Error(w, "milestone name already exists", http.StatusConflict)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, ms)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleMilestonesItem handles the milestone sub-resources:
//
//	PATCH  /api/agent/milestones/{id}     → edit name / description / target date
//	DELETE /api/agent/milestones/{id}     → remove (tasks fall back to 未分组)
//	POST   /api/agent/milestones/reorder  → set positions from an ordered id list
//
// All require workspace_id (query for PATCH/DELETE, body for reorder) so the
// store resolves the owning project.
func (h *Handler) HandleMilestonesItem(w http.ResponseWriter, r *http.Request) {
	const prefix = "/api/agent/milestones/"
	rest := r.URL.Path[len(prefix):]
	if rest == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	if rest == "reorder" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			WorkspaceID string   `json:"workspace_id"`
			Order       []string `json:"order"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if body.WorkspaceID == "" {
			http.Error(w, "workspace_id is required", http.StatusBadRequest)
			return
		}
		wsPath, err := h.resolveWorkspacePath(body.WorkspaceID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if err := h.tasksStore.ReorderMilestones(wsPath, body.Order); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		list, err := h.tasksStore.ListMilestones(wsPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, list)
		return
	}

	if i := indexByte(rest, '/'); i >= 0 {
		http.Error(w, "unsupported sub-path", http.StatusNotFound)
		return
	}
	id := rest

	switch r.Method {
	case http.MethodPatch:
		var body struct {
			WorkspaceID   string      `json:"workspace_id"`
			Name          *string     `json:"name,omitempty"`
			Description   *string     `json:"description,omitempty"`
			TargetDate    **time.Time `json:"targetDate,omitempty"`
			PredecessorID *string     `json:"predecessorId,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if body.WorkspaceID == "" {
			http.Error(w, "workspace_id is required", http.StatusBadRequest)
			return
		}
		if body.Name != nil && strings.TrimSpace(*body.Name) == "" {
			http.Error(w, "name must not be empty", http.StatusBadRequest)
			return
		}
		wsPath, err := h.resolveWorkspacePath(body.WorkspaceID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		patch := MilestonePatch{Description: body.Description, TargetDate: body.TargetDate, PredecessorID: body.PredecessorID}
		if body.Name != nil {
			trimmed := strings.TrimSpace(*body.Name)
			patch.Name = &trimmed
		}
		ms, err := h.tasksStore.UpdateMilestone(wsPath, id, patch)
		if err != nil {
			switch {
			case errors.Is(err, ErrMilestoneExists):
				http.Error(w, "milestone name already exists", http.StatusConflict)
			case errors.Is(err, ErrNotFound):
				http.Error(w, "milestone not found", http.StatusNotFound)
			default:
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
		writeJSON(w, ms)

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
		if err := h.tasksStore.DeleteMilestone(wsPath, id); err != nil {
			if errors.Is(err, ErrNotFound) {
				http.Error(w, "milestone not found", http.StatusNotFound)
				return
			}
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
	var sessionRole string
	if rec, ok, err := h.store.Get(sessionId); err == nil && ok {
		acpSessionID = rec.AcpSessionID
		sessionRole = rec.Role
	}

	var systemContext string
	var mcpServers json.RawMessage
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

		// Resume-id fallback: if the chat index record is gone (e.g. the user
		// deleted the sidebar session) but the triggering reply recorded the
		// agent's resume id, resume from that. The agent keeps the transcript
		// locally (Claude ~/.claude/projects/…, Codex its own dir), so "查看详情"
		// re-renders history by id instead of starting a fresh run.
		if acpSessionID == "" && replyID != "" {
			for i := range targetTask.Replies {
				if targetTask.Replies[i].ID == replyID && targetTask.Replies[i].AcpSessionID != "" {
					acpSessionID = targetTask.Replies[i].AcpSessionID
					break
				}
			}
		}

		// Issue background injection (issue-model §9): description + the
		// full reply timeline, injected only when this is a NEW session.
		// Resumed sessions already carry their own conversation history.
		if acpSessionID == "" {
			if targetTask.Type == TaskTypeDiscussion {
				// A discussion-linked session is a PM conversation, NOT an
				// executor: the agent acts as PM (create_task / create_discussion)
				// with the discussion thread as background. Its user prompts and
				// final replies are recorded back to this discussion's timeline
				// (writeUserReply / writeAgentReply, keyed on task_id).
				systemContext = buildPMSystemPrompt(h.workspaceName(wsID), wsID) + "\n\n" + buildIssueBackground(targetTask, wsPath)
			} else {
				systemContext = buildIssueBackground(targetTask, wsPath)
			}
		}
		if targetTask.Type == TaskTypeDiscussion {
			mcpServers = h.buildPMMcpServers(wsID)
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

		// Execution-start vs. read-only resume: a non-empty acpSessionID means
		// this open is resuming an existing agent session (e.g. "查看详情" on a
		// finished run) — it must NOT acquire the workspace lock, flip the task
		// to running, or re-execute. Only a genuinely new session (acpSessionID
		// empty) starts execution. Discussions are never executed, so a
		// discussion-linked PM conversation skips this entirely.
		if targetTask.Type != TaskTypeDiscussion && acpSessionID == "" && targetTask.Status != TaskStatusRunning {
			// Try to acquire the execution lock
			if !h.scheduler.Lock.TryAcquire(wsPath, taskId) {
				// If already occupied, return 409 conflict
				http.Error(w, "Another session is already running in this workspace", http.StatusConflict)
				return
			}
			now := time.Now().UTC()
			if err := h.tasksStore.Mutate(wsPath, func(cfg *TasksConfig) bool {
				for i := range cfg.Tasks {
					t := &cfg.Tasks[i]
					if t.ID != taskId {
						continue
					}
					t.Status = TaskStatusRunning
					t.StartedAt = &now
					t.UpdatedAt = now
					// Update or create session metadata
					sessionExists := false
					for j := range t.Sessions {
						if t.Sessions[j].ID == sessionId {
							t.Sessions[j].Status = SessionStatusRunning
							sessionExists = true
							break
						}
					}
					if !sessionExists {
						t.Sessions = append(t.Sessions, SessionMetadata{
							ID:        sessionId,
							Kind:      SessionKindChat,
							Name:      "智能体排查与修复",
							AgentType: agentType,
							Status:    SessionStatusRunning,
							CreatedAt: now,
						})
					}
					return true
				}
				return false
			}); err != nil {
				log.Printf("[agent] mark task %s running: %v", taskId, err)
			}
		}
		log.Printf("[agent] Bridging Chat UI WebSocket for task %s, session %s", taskId, sessionId)
	} else if isProjectManagerRole(sessionRole) {
		// AI Project Manager session (pm, or pmo in the default workspace):
		// inject the PM system prompt (new sessions only — resumed ones already
		// carry their history) and a task-tool MCP server locked to this workspace.
		if acpSessionID == "" {
			systemContext = buildPMSystemPrompt(h.workspaceName(wsID), wsID)
		}
		mcpServers = h.buildPMMcpServers(wsID)
		log.Printf("[agent] Bridging AI Project Manager WebSocket for session %s (workspace %s)", sessionId, wsID)
	} else {
		log.Printf("[agent] Bridging Chat UI WebSocket for session %s (no task)", sessionId)
	}

	h.acpxClient.Bridge(w, r, wsPath, taskId, sessionId, agentType, systemContext, mcpServers, h.scheduler, h.tasksStore, h.store, acpSessionID, replyID)
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
	// Mirror Claude Code's cwd → project-dir slug exactly: every
	// non-alphanumeric rune (including '.', '_', '/') becomes '-'. Keeping
	// '.'/'_' here (as a previous version did) makes paths like
	// "/Users/x/.coze" or ".../smart_cups" slug to "-x-.coze"/"smart_cups"
	// instead of Claude's "-x--coze"/"smart-cups", so the session .jsonl is
	// never found and the AI title never resolves.
	var sb strings.Builder
	for _, r := range path {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
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
