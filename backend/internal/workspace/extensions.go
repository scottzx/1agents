package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"github.com/scottzx/1Agents/backend/internal/harnesskit"
)

type extensionClient interface {
	EnsureProject(context.Context, string) error
	ScanAndSync(context.Context) error
	ListExtensions(context.Context, harnesskit.ListExtensionsFilter) ([]harnesskit.Extension, error)
	InstallToAgent(context.Context, string, string, string, string) (string, error)
	DeleteExtension(context.Context, string) error
	UpdateExtension(context.Context, string) error
}

type WorkspaceExtensionStatus struct {
	ExtensionID  string `json:"extensionId"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Path         string `json:"path,omitempty"`
	File         string `json:"file,omitempty"`
	State        string `json:"state"`
	SourceAgent  string `json:"sourceAgent,omitempty"`
	CanUpdate    bool   `json:"canUpdate"`
	UpdateReason string `json:"updateReason,omitempty"`
}

type AvailableExtension struct {
	ExtensionID       string `json:"extensionId"`
	Name              string `json:"name"`
	Description       string `json:"description"`
	SourceAgent       string `json:"sourceAgent,omitempty"`
	Installed         bool   `json:"installed"`
	CanInstall        bool   `json:"canInstall"`
	UnsupportedReason string `json:"unsupportedReason,omitempty"`
}

func (h *Handler) extensionWorkspace(id string) (Workspace, error) {
	cfg, err := h.loadConfig()
	if err != nil {
		return Workspace{}, err
	}
	for _, ws := range cfg.Workspaces {
		if ws.ID == id {
			return ws, nil
		}
	}
	return Workspace{}, fmt.Errorf("workspace not found")
}

func (h *Handler) requireExtensionClient() (extensionClient, error) {
	if h.extensions == nil {
		return nil, &harnesskit.APIError{
			StatusCode: http.StatusServiceUnavailable,
			Kind:       "harnesskit_unavailable",
			Message:    "HarnessKit runtime is not configured for workspace extensions",
		}
	}
	return h.extensions, nil
}

func (h *Handler) prepareProject(ctx context.Context, ws Workspace) (extensionClient, string, error) {
	client, err := h.requireExtensionClient()
	if err != nil {
		return nil, "", err
	}
	if err := client.EnsureProject(ctx, ws.Path); err != nil {
		return nil, "", err
	}
	agent, err := harnesskit.ResolveAgentMapping(ws.DefaultAgent)
	if err != nil {
		// Agent mapping error is non-fatal for project preparation (listing, status check, etc.).
		// Callers requiring a valid HarnessKit deployment agent (e.g. installExtension) validate agent != "".
		return client, "", nil
	}
	return client, agent, nil
}

func projectFilter(kind, path string) harnesskit.ListExtensionsFilter {
	return harnesskit.ListExtensionsFilter{Kind: kind, ScopeType: "project", ScopePath: path}
}

func extensionBelongsToProject(ext harnesskit.Extension, kind, projectPath string) bool {
	return ext.Kind == kind && ext.Scope.Type == "project" &&
		samePath(ext.Scope.Path, projectPath)
}

func samePath(left, right string) bool {
	leftPath, leftErr := filepath.Abs(left)
	rightPath, rightErr := filepath.Abs(right)
	if leftErr == nil {
		left = filepath.Clean(leftPath)
	}
	if rightErr == nil {
		right = filepath.Clean(rightPath)
	}
	return strings.EqualFold(left, right)
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func relativeExtensionPath(workspacePath string, ext harnesskit.Extension) string {
	if ext.SourcePath == nil || strings.TrimSpace(*ext.SourcePath) == "" {
		return ""
	}
	rel, err := filepath.Rel(workspacePath, *ext.SourcePath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	return filepath.ToSlash(rel)
}

func extensionStatus(workspacePath string, ext harnesskit.Extension) WorkspaceExtensionStatus {
	sourceAgent := ""
	if len(ext.Agents) > 0 {
		sourceAgent = ext.Agents[0]
	}
	canUpdate := ext.Source.Origin == "git" || ext.Source.Origin == "registry" || ext.Source.FromManifest
	reason := ""
	if !canUpdate {
		reason = "This project extension has no verified upstream update source; edit it in place and reindex instead."
	}
	path := relativeExtensionPath(workspacePath, ext)
	file := filepath.Base(path)
	if ext.Kind == "subagent" && path != "" {
		// Preserve the native Agent-relative path in team.json so personas also
		// work for non-Claude adapters. Legacy basename entries remain readable.
		file = path
	}
	return WorkspaceExtensionStatus{
		ExtensionID:  ext.ID,
		Name:         ext.Name,
		Description:  ext.Description,
		Path:         path,
		File:         file,
		State:        map[bool]string{true: "active", false: "disabled"}[ext.Enabled],
		SourceAgent:  sourceAgent,
		CanUpdate:    canUpdate,
		UpdateReason: reason,
	}
}

func (h *Handler) listProjectExtensions(ctx context.Context, ws Workspace, kind string) ([]WorkspaceExtensionStatus, error) {
	client, _, err := h.prepareProject(ctx, ws)
	if err != nil {
		return nil, err
	}
	rows, err := client.ListExtensions(ctx, projectFilter(kind, ws.Path))
	if err != nil {
		return nil, err
	}
	result := make([]WorkspaceExtensionStatus, 0, len(rows))
	for _, ext := range rows {
		if extensionBelongsToProject(ext, kind, ws.Path) {
			result = append(result, extensionStatus(ws.Path, ext))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return result, nil
}

func (h *Handler) listAvailableExtensions(ctx context.Context, ws Workspace, kind string) ([]AvailableExtension, error) {
	client, _, err := h.prepareProject(ctx, ws)
	if err != nil {
		return nil, err
	}
	projectRows, err := client.ListExtensions(ctx, projectFilter(kind, ws.Path))
	if err != nil {
		return nil, err
	}
	installedNames := make(map[string]bool, len(projectRows))
	for _, ext := range projectRows {
		if extensionBelongsToProject(ext, kind, ws.Path) {
			installedNames[strings.ToLower(ext.Name)] = true
		}
	}
	globalRows, err := client.ListExtensions(ctx, harnesskit.ListExtensionsFilter{
		Kind: kind, ScopeType: "global",
	})
	if err != nil {
		return nil, err
	}
	result := make([]AvailableExtension, 0, len(globalRows))
	for _, ext := range globalRows {
		if ext.Kind != kind || ext.Scope.Type != "global" {
			continue
		}
		sourceAgent := ""
		if len(ext.Agents) > 0 {
			sourceAgent = ext.Agents[0]
		}
		installed := installedNames[strings.ToLower(ext.Name)]
		result = append(result, AvailableExtension{
			ExtensionID: ext.ID,
			Name:        ext.Name,
			Description: ext.Description,
			SourceAgent: sourceAgent,
			Installed:   installed,
			CanInstall:  !installed,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return result, nil
}

func (h *Handler) installExtension(ctx context.Context, ws Workspace, kind, extensionID string) (string, WorkspaceExtensionStatus, error) {
	client, agent, err := h.prepareProject(ctx, ws)
	if err != nil {
		return "", WorkspaceExtensionStatus{}, err
	}
	if agent == "" {
		if _, err := harnesskit.ResolveAgentMapping(ws.DefaultAgent); err != nil {
			return "", WorkspaceExtensionStatus{}, &harnesskit.APIError{
				StatusCode: http.StatusUnprocessableEntity,
				Kind:       "agent_extension_unsupported",
				Message:    err.Error(),
			}
		}
	}
	globalRows, err := client.ListExtensions(ctx, harnesskit.ListExtensionsFilter{Kind: kind, ScopeType: "global"})
	if err != nil {
		return "", WorkspaceExtensionStatus{}, err
	}
	found := false
	for _, ext := range globalRows {
		if ext.ID == extensionID && ext.Kind == kind && ext.Scope.Type == "global" {
			found = true
			break
		}
	}
	if !found {
		return "", WorkspaceExtensionStatus{}, &harnesskit.APIError{
			StatusCode: http.StatusNotFound,
			Kind:       "extension_not_found",
			Message:    "The requested global HarnessKit extension does not exist",
		}
	}
	targetID, err := client.InstallToAgent(ctx, extensionID, agent, ws.Name, ws.Path)
	if err != nil {
		return "", WorkspaceExtensionStatus{}, err
	}
	rows, err := client.ListExtensions(ctx, projectFilter(kind, ws.Path))
	if err != nil {
		return "", WorkspaceExtensionStatus{}, err
	}
	for _, ext := range rows {
		if ext.ID == targetID && extensionBelongsToProject(ext, kind, ws.Path) {
			return targetID, extensionStatus(ws.Path, ext), nil
		}
	}
	return targetID, WorkspaceExtensionStatus{ExtensionID: targetID, State: "active"}, nil
}

func (h *Handler) findProjectExtension(ctx context.Context, ws Workspace, kind, extensionID string) (extensionClient, harnesskit.Extension, error) {
	client, _, err := h.prepareProject(ctx, ws)
	if err != nil {
		return nil, harnesskit.Extension{}, err
	}
	rows, err := client.ListExtensions(ctx, projectFilter(kind, ws.Path))
	if err != nil {
		return nil, harnesskit.Extension{}, err
	}
	for _, ext := range rows {
		if ext.ID == extensionID && extensionBelongsToProject(ext, kind, ws.Path) {
			return client, ext, nil
		}
	}
	return nil, harnesskit.Extension{}, &harnesskit.APIError{
		StatusCode: http.StatusNotFound,
		Kind:       "project_extension_not_found",
		Message:    "The extension is not installed in this workspace",
	}
}

func writeExtensionError(w http.ResponseWriter, err error) {
	var apiErr *harnesskit.APIError
	if errors.As(err, &apiErr) {
		status := apiErr.StatusCode
		if status < 400 || status > 599 {
			status = http.StatusBadGateway
		}
		writeJSONStatus(w, status, map[string]any{"error": apiErr.Kind, "message": apiErr.Message})
		return
	}
	writeJSONStatus(w, http.StatusInternalServerError, map[string]any{
		"error":   "workspace_extension_error",
		"message": err.Error(),
	})
}

func decodeExtensionRequest(r *http.Request) (string, string, error) {
	var body struct {
		ID          string `json:"id"`
		ExtensionID string `json:"extensionId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return "", "", err
	}
	if strings.TrimSpace(body.ID) == "" || strings.TrimSpace(body.ExtensionID) == "" {
		return "", "", fmt.Errorf("id and extensionId are required")
	}
	return strings.TrimSpace(body.ID), strings.TrimSpace(body.ExtensionID), nil
}
