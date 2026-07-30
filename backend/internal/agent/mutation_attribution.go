package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/scottzx/1Agents/backend/internal/localtoken"
	"github.com/scottzx/1Agents/backend/internal/meta"
)

var errUntrustedSessionAttribution = errors.New("untrusted Session attribution")

func (h *Handler) resolveMutationContext(r *http.Request, projectID string) (meta.MutationContext, error) {
	if r.Header.Get("X-OneAgents-Turn-ID") != "" {
		return meta.MutationContext{}, fmt.Errorf("%w: turnId is server-derived", meta.ErrInvalidProjectEvent)
	}
	origin := r.Header.Get("X-OneAgents-Origin")
	switch origin {
	case "cli", "mcp":
	default:
		origin = "http"
	}
	sessionID := r.Header.Get("X-OneAgents-Session-ID")
	sessionToken := r.Header.Get("X-OneAgents-Session-Token")
	if sessionID == "" {
		if sessionToken != "" {
			return meta.MutationContext{}, errUntrustedSessionAttribution
		}
		return meta.MutationContext{
			ProjectID: projectID,
			ActorKind: "user",
			ActorName: "user",
			Origin:    origin,
		}, nil
	}
	if !localtoken.ValidateSessionToken(sessionID, sessionToken) {
		return meta.MutationContext{}, errUntrustedSessionAttribution
	}
	rec, ok, err := h.store.Get(sessionID)
	if err != nil {
		return meta.MutationContext{}, err
	}
	if !ok {
		return meta.MutationContext{}, meta.ErrNotFound
	}
	if rec.WorkspaceID != projectID {
		return meta.MutationContext{}, meta.ErrProjectMismatch
	}
	var turn meta.AgentTurn
	ok = false
	authoritative := false
	if h.acpxClient != nil {
		turn, ok, authoritative = h.acpxClient.authoritativeRunningTurn(sessionID)
	}
	if !authoritative {
		if h.turnStore == nil {
			return meta.MutationContext{}, fmt.Errorf("Turn state is unavailable")
		}
		turn, ok, err = h.turnStore.RunningBySession(sessionID)
		if err != nil {
			return meta.MutationContext{}, err
		}
	}
	if !ok {
		return meta.MutationContext{}, meta.ErrTurnNotRunning
	}
	if turn.ProjectID != projectID {
		return meta.MutationContext{}, meta.ErrProjectMismatch
	}
	return meta.MutationContext{
		ProjectID:         projectID,
		ActorKind:         "agent",
		ActorName:         turn.AgentType,
		SessionID:         sessionID,
		TurnID:            turn.ID,
		CorrelationID:     turn.ID,
		Origin:            origin,
		AuthoritativeTurn: authoritative,
	}, nil
}

func writeMutationContextError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errUntrustedSessionAttribution):
		http.Error(w, err.Error(), http.StatusUnauthorized)
	case errors.Is(err, meta.ErrProjectMismatch):
		http.Error(w, err.Error(), http.StatusForbidden)
	case errors.Is(err, meta.ErrTurnNotRunning):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, meta.ErrNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, meta.ErrInvalidProjectEvent):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func mutationEvent(ctx meta.MutationContext, targetType, targetID, operation string, before, after any) meta.ProjectEvent {
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	return meta.ProjectEvent{
		ID:                   newID(),
		ProjectID:            ctx.ProjectID,
		CorrelationID:        ctx.CorrelationID,
		TurnID:               ctx.TurnID,
		SessionID:            ctx.SessionID,
		TaskRunID:            ctx.TaskRunID,
		ActorKind:            ctx.ActorKind,
		ActorName:            ctx.ActorName,
		Origin:               ctx.Origin,
		EventType:            targetType + "." + operation,
		TargetType:           targetType,
		TargetID:             targetID,
		Operation:            operation,
		Before:               beforeJSON,
		After:                afterJSON,
		Status:               meta.ProjectEventSucceeded,
		AllowUnprojectedTurn: ctx.AuthoritativeTurn,
	}
}

func taskEventSnapshot(task ProjectItem) map[string]any {
	return map[string]any{
		"id":         task.ID,
		"number":     task.Number,
		"title":      task.Title,
		"type":       task.Type,
		"status":     task.Status,
		"issueState": task.IssueState,
		"priority":   task.Priority,
		"assignee":   task.Assignee,
		"milestone":  task.Milestone,
		"parentId":   task.ParentID,
		"dependsOn":  task.DependsOn,
	}
}

func writeMutationJSON(w http.ResponseWriter, value any, event meta.ProjectEvent) {
	raw, err := json.Marshal(value)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var response map[string]any
	if err := json.Unmarshal(raw, &response); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	response["eventId"] = event.ID
	if event.SessionID != "" {
		response["sessionId"] = event.SessionID
	}
	if event.TurnID != "" {
		response["turnId"] = event.TurnID
	}
	writeJSON(w, response)
}
