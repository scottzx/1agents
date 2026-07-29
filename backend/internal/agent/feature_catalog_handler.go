package agent

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

func (h *Handler) featureCatalogProject(w http.ResponseWriter, workspaceID string) (string, bool) {
	if workspaceID == "" {
		http.Error(w, "workspace_id is required", http.StatusBadRequest)
		return "", false
	}
	workspacePath, err := h.resolveWorkspacePath(workspaceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return "", false
	}
	projectID, err := h.tasksStore.ProjectIDForPath(workspacePath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return "", false
	}
	if projectID == "" {
		http.Error(w, "project not found", http.StatusNotFound)
		return "", false
	}
	return projectID, true
}

func writeFeatureCatalogError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, meta.ErrFeatureHasChildren):
		http.Error(w, "has_children", http.StatusConflict)
	case errors.Is(err, meta.ErrProjectMismatch):
		http.Error(w, err.Error(), http.StatusForbidden)
	case errors.Is(err, meta.ErrNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, meta.ErrFeatureInvalidKind),
		errors.Is(err, meta.ErrFeatureInvalidParent),
		errors.Is(err, meta.ErrFeatureMaxDepth),
		errors.Is(err, meta.ErrFeatureCycle),
		errors.Is(err, meta.ErrFeatureInvalidRelation),
		errors.Is(err, meta.ErrFeatureInvalidItemType),
		errors.Is(err, meta.ErrFeatureInvalidMilestone):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		writeMutationContextError(w, err)
	}
}

// HandleFeatureCatalogRoot handles GET/POST /api/agent/feature-catalog.
func (h *Handler) HandleFeatureCatalogRoot(w http.ResponseWriter, r *http.Request) {
	if h.featureStore == nil {
		http.Error(w, "feature catalog store is unavailable", http.StatusInternalServerError)
		return
	}
	switch r.Method {
	case http.MethodGet:
		projectID, ok := h.featureCatalogProject(w, r.URL.Query().Get("workspace_id"))
		if !ok {
			return
		}
		catalog, err := h.featureStore.List(projectID)
		if err != nil {
			writeFeatureCatalogError(w, err)
			return
		}
		writeJSON(w, catalog)
	case http.MethodPost:
		var body struct {
			WorkspaceID       string               `json:"workspace_id"`
			ParentID          string               `json:"parentId"`
			Kind              meta.FeatureNodeKind `json:"kind"`
			Title             string               `json:"title"`
			Description       string               `json:"description"`
			TargetMilestoneID string               `json:"targetMilestoneId"`
			Position          int                  `json:"position"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		projectID, ok := h.featureCatalogProject(w, body.WorkspaceID)
		if !ok {
			return
		}
		ctx, err := h.resolveMutationContext(r, projectID)
		if err != nil {
			writeMutationContextError(w, err)
			return
		}
		event := mutationEvent(ctx, "feature_node", "", "create", nil, nil)
		node, err := h.featureStore.Create(meta.FeatureNode{
			ProjectID: projectID, ParentID: body.ParentID, Kind: body.Kind,
			Title: body.Title, Description: body.Description,
			TargetMilestoneID: body.TargetMilestoneID, Position: body.Position,
		}, event)
		if err != nil {
			writeFeatureCatalogError(w, err)
			return
		}
		event.TargetID = node.ID
		writeMutationJSON(w, node, event)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleFeatureCatalogItem handles node mutation and item-link sub-resources.
func (h *Handler) HandleFeatureCatalogItem(w http.ResponseWriter, r *http.Request) {
	if h.featureStore == nil {
		http.Error(w, "feature catalog store is unavailable", http.StatusInternalServerError)
		return
	}
	const prefix = "/api/agent/feature-catalog/"
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, prefix), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "missing feature id", http.StatusBadRequest)
		return
	}
	if len(parts) == 1 && parts[0] == "batch" {
		h.handleFeatureCatalogBatch(w, r)
		return
	}
	if len(parts) == 1 && parts[0] == "gantt" {
		h.handleFeatureCatalogGantt(w, r)
		return
	}
	if len(parts) == 1 && parts[0] == "export" {
		h.handleFeatureCatalogExport(w, r)
		return
	}
	featureID := parts[0]
	if len(parts) == 1 {
		h.handleFeatureNode(w, r, featureID)
		return
	}
	if len(parts) == 2 && parts[1] == "milestone-diff" {
		h.handleFeatureMilestoneDiff(w, r, featureID)
		return
	}
	if len(parts) == 2 && parts[1] == "sync-milestone" {
		h.handleFeatureMilestoneSync(w, r, featureID)
		return
	}
	if parts[1] != "items" || len(parts) > 3 {
		http.Error(w, "unsupported sub-path", http.StatusNotFound)
		return
	}
	h.handleFeatureLink(w, r, featureID, parts[2:])
}

func (h *Handler) handleFeatureCatalogBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		WorkspaceID string                              `json:"workspace_id"`
		Operations  []meta.FeatureCatalogBatchOperation `json:"operations"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(body.Operations) == 0 {
		http.Error(w, "operations are required", http.StatusBadRequest)
		return
	}
	projectID, ok := h.featureCatalogProject(w, body.WorkspaceID)
	if !ok {
		return
	}
	ctx, err := h.resolveMutationContext(r, projectID)
	if err != nil {
		writeMutationContextError(w, err)
		return
	}
	results, err := h.featureStore.Batch(
		projectID,
		body.Operations,
		mutationEvent(ctx, "feature_node", "batch", "update", nil, nil),
	)
	if err != nil {
		writeFeatureCatalogError(w, err)
		return
	}
	response := map[string]any{"results": results}
	if ctx.SessionID != "" {
		response["sessionId"] = ctx.SessionID
	}
	if ctx.TurnID != "" {
		response["turnId"] = ctx.TurnID
	}
	writeJSON(w, response)
}

func (h *Handler) handleFeatureMilestoneDiff(w http.ResponseWriter, r *http.Request, featureID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	projectID, ok := h.featureCatalogProject(w, r.URL.Query().Get("workspace_id"))
	if !ok {
		return
	}
	preview, err := h.featureStore.PreviewMilestoneSync(projectID, featureID)
	if err != nil {
		writeFeatureCatalogError(w, err)
		return
	}
	writeJSON(w, preview)
}

func (h *Handler) handleFeatureMilestoneSync(w http.ResponseWriter, r *http.Request, featureID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	projectID, ok := h.featureCatalogProject(w, body.WorkspaceID)
	if !ok {
		return
	}
	ctx, err := h.resolveMutationContext(r, projectID)
	if err != nil {
		writeMutationContextError(w, err)
		return
	}
	event := mutationEvent(ctx, "feature_milestone", featureID, "sync", nil, nil)
	preview, err := h.featureStore.SyncMilestone(projectID, featureID, event)
	if err != nil {
		writeFeatureCatalogError(w, err)
		return
	}
	writeMutationJSON(w, preview, event)
}

func (h *Handler) handleFeatureNode(w http.ResponseWriter, r *http.Request, featureID string) {
	switch r.Method {
	case http.MethodPatch:
		var body struct {
			WorkspaceID       string  `json:"workspace_id"`
			ParentID          *string `json:"parentId"`
			Title             *string `json:"title"`
			Description       *string `json:"description"`
			TargetMilestoneID *string `json:"targetMilestoneId"`
			Position          *int    `json:"position"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		projectID, ok := h.featureCatalogProject(w, body.WorkspaceID)
		if !ok {
			return
		}
		ctx, err := h.resolveMutationContext(r, projectID)
		if err != nil {
			writeMutationContextError(w, err)
			return
		}
		operation := "update"
		if body.ParentID != nil || body.Position != nil {
			operation = "move"
		}
		event := mutationEvent(ctx, "feature_node", featureID, operation, nil, nil)
		node, err := h.featureStore.Update(projectID, featureID, meta.FeatureNodePatch{
			ParentID: body.ParentID, Title: body.Title, Description: body.Description,
			TargetMilestoneID: body.TargetMilestoneID, Position: body.Position,
		}, event)
		if err != nil {
			writeFeatureCatalogError(w, err)
			return
		}
		writeMutationJSON(w, node, event)
	case http.MethodDelete:
		projectID, ok := h.featureCatalogProject(w, r.URL.Query().Get("workspace_id"))
		if !ok {
			return
		}
		ctx, err := h.resolveMutationContext(r, projectID)
		if err != nil {
			writeMutationContextError(w, err)
			return
		}
		event := mutationEvent(ctx, "feature_node", featureID, "delete", nil, nil)
		if err := h.featureStore.Delete(projectID, featureID, event); err != nil {
			writeFeatureCatalogError(w, err)
			return
		}
		writeMutationJSON(w, map[string]any{"ok": true}, event)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleFeatureLink(w http.ResponseWriter, r *http.Request, featureID string, tail []string) {
	switch {
	case r.Method == http.MethodPost && len(tail) == 0:
		var body struct {
			WorkspaceID string                   `json:"workspace_id"`
			ItemID      string                   `json:"itemId"`
			Relation    meta.FeatureItemRelation `json:"relation"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		projectID, ok := h.featureCatalogProject(w, body.WorkspaceID)
		if !ok {
			return
		}
		ctx, err := h.resolveMutationContext(r, projectID)
		if err != nil {
			writeMutationContextError(w, err)
			return
		}
		targetID := strings.Join([]string{featureID, body.ItemID, string(body.Relation)}, ":")
		event := mutationEvent(ctx, "feature_link", targetID, "link", nil, nil)
		link, created, err := h.featureStore.LinkItem(
			projectID, featureID, body.ItemID, body.Relation, event,
		)
		if err != nil {
			writeFeatureCatalogError(w, err)
			return
		}
		if !created {
			writeJSON(w, link)
			return
		}
		writeMutationJSON(w, link, event)
	case r.Method == http.MethodDelete && len(tail) == 1:
		projectID, ok := h.featureCatalogProject(w, r.URL.Query().Get("workspace_id"))
		if !ok {
			return
		}
		relation := meta.FeatureItemRelation(r.URL.Query().Get("relation"))
		ctx, err := h.resolveMutationContext(r, projectID)
		if err != nil {
			writeMutationContextError(w, err)
			return
		}
		targetID := strings.Join([]string{featureID, tail[0], string(relation)}, ":")
		event := mutationEvent(ctx, "feature_link", targetID, "unlink", nil, nil)
		if err := h.featureStore.UnlinkItem(projectID, featureID, tail[0], relation, event); err != nil {
			writeFeatureCatalogError(w, err)
			return
		}
		writeMutationJSON(w, map[string]any{"ok": true}, event)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleFeatureCatalogGantt(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    projectID, ok := h.featureCatalogProject(w, r.URL.Query().Get("workspace_id"))
    if !ok {
        return
    }
    data, err := h.featureStore.GanttView(projectID)
    if err != nil {
        writeFeatureCatalogError(w, err)
        return
    }
    writeJSON(w, data)
}

func (h *Handler) handleFeatureCatalogExport(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    projectID, ok := h.featureCatalogProject(w, r.URL.Query().Get("workspace_id"))
    if !ok {
        return
    }
    format := r.URL.Query().Get("format")
    switch format {
    case "json":
        data, err := h.featureStore.ExportJSON(projectID)
        if err != nil {
            writeFeatureCatalogError(w, err)
            return
        }
        w.Header().Set("Content-Type", "application/json")
        w.Header().Set("Content-Disposition", `attachment; filename="feature-catalog.json"`)
        w.Write(data)
    case "markdown", "md", "":
        md, err := h.featureStore.ExportMarkdown(projectID)
        if err != nil {
            writeFeatureCatalogError(w, err)
            return
        }
        w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
        w.Header().Set("Content-Disposition", `attachment; filename="feature-catalog.md"`)
        w.Write([]byte(md))
    default:
        http.Error(w, "unsupported format: use json or markdown", http.StatusBadRequest)
    }
}
