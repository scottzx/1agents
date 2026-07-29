package agent

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

func parsePageLimit(r *http.Request) (int, error) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return 0, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return 0, meta.ErrInvalidCursor
	}
	return limit, nil
}

func (h *Handler) queryProjectID(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	wsID := r.URL.Query().Get("workspace_id")
	if wsID == "" {
		http.Error(w, "workspace_id query parameter is required", http.StatusBadRequest)
		return "", "", false
	}
	wsPath, err := h.resolveWorkspacePath(wsID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return "", "", false
	}
	projectID, err := h.tasksStore.ProjectIDForPath(wsPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return "", "", false
	}
	if projectID == "" {
		projectID = wsID
	}
	return wsID, projectID, true
}

func (h *Handler) validateActivityScope(wsID, projectID, sessionID, turnID string) error {
	if sessionID != "" {
		session, ok, err := h.store.Get(sessionID)
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		if session.WorkspaceID != wsID && session.WorkspaceID != projectID {
			return meta.ErrProjectMismatch
		}
	}
	if turnID != "" {
		turn, ok, err := h.turnStore.Get(turnID)
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		if turn.ProjectID != projectID {
			return meta.ErrProjectMismatch
		}
		if sessionID != "" && turn.SessionID != sessionID {
			return meta.ErrProjectMismatch
		}
	}
	return nil
}

func writeReadScopeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, meta.ErrInvalidCursor), errors.Is(err, meta.ErrInvalidProjectEvent):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, meta.ErrProjectMismatch):
		http.Error(w, err.Error(), http.StatusForbidden)
	case errors.Is(err, meta.ErrNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// HandleTurns serves the persisted Turn history for a Project or Session.
func (h *Handler) HandleTurns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	wsID, projectID, ok := h.queryProjectID(w, r)
	if !ok {
		return
	}
	sessionID := r.URL.Query().Get("session_id")
	if err := h.validateActivityScope(wsID, projectID, sessionID, ""); err != nil {
		writeReadScopeError(w, err)
		return
	}
	limit, err := parsePageLimit(r)
	if err != nil {
		writeReadScopeError(w, err)
		return
	}
	status := meta.AgentTurnStatus(r.URL.Query().Get("status"))
	switch status {
	case "", meta.AgentTurnQueued, meta.AgentTurnRunning, meta.AgentTurnCompleted,
		meta.AgentTurnFailed, meta.AgentTurnCancelled:
	default:
		http.Error(w, "invalid Turn status", http.StatusBadRequest)
		return
	}
	page, err := h.turnStore.List(meta.AgentTurnListOptions{
		ProjectID: projectID,
		SessionID: sessionID,
		Status:    status,
		Cursor:    r.URL.Query().Get("cursor"),
		Limit:     limit,
	})
	if err != nil {
		writeReadScopeError(w, err)
		return
	}
	writeJSON(w, page)
}

// HandleProjectActivity serves the aggregated ProjectActivityEntry projection.
// Project, Session, Turn and ProjectItem dimensions are query filters over the
// same stable cursor contract.
func (h *Handler) HandleProjectActivity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	wsID, projectID, ok := h.queryProjectID(w, r)
	if !ok {
		return
	}
	sessionID := r.URL.Query().Get("session_id")
	turnID := r.URL.Query().Get("turn_id")
	if err := h.validateActivityScope(wsID, projectID, sessionID, turnID); err != nil {
		writeReadScopeError(w, err)
		return
	}
	limit, err := parsePageLimit(r)
	if err != nil {
		writeReadScopeError(w, err)
		return
	}
	status := meta.ProjectEventStatus(r.URL.Query().Get("status"))
	switch status {
	case "", meta.ProjectEventSucceeded, meta.ProjectEventRejected, meta.ProjectEventFailed:
	default:
		http.Error(w, "invalid Event status", http.StatusBadRequest)
		return
	}
	if h.activityStore == nil {
		http.Error(w, "activity store is unavailable", http.StatusInternalServerError)
		return
	}
	page, err := h.activityStore.List(meta.ProjectActivityListOptions{
		ProjectID:        projectID,
		SessionID:        sessionID,
		TurnID:           turnID,
		TargetType:       r.URL.Query().Get("target_type"),
		TargetID:         r.URL.Query().Get("target_id"),
		Status:           status,
		Origin:           r.URL.Query().Get("origin"),
		IncludeLifecycle: r.URL.Query().Get("include_lifecycle") == "1",
		Cursor:           r.URL.Query().Get("cursor"),
		Limit:            limit,
	})
	if err != nil {
		writeReadScopeError(w, err)
		return
	}
	writeJSON(w, page)
}
