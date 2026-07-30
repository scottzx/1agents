package harnesskit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

// Client is the authenticated, server-side HarnessKit command client. It is
// intentionally separate from the browser proxy: workspace paths and the
// daemon bearer token never cross the browser boundary.
type Client struct {
	runtime Runtime
	http    *http.Client
}

func NewClient(runtime Runtime) *Client {
	return &Client{
		runtime: runtime,
		http:    &http.Client{Timeout: 60 * time.Second},
	}
}

type APIError struct {
	StatusCode int
	Kind       string
	Message    string
}

func (e *APIError) Error() string {
	if e.Kind == "" {
		return e.Message
	}
	return e.Kind + ": " + e.Message
}

func IsConflict(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict
}

type Project struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
}

type ExtensionScope struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
	Path string `json:"path,omitempty"`
}

type ExtensionSource struct {
	Origin       string  `json:"origin"`
	URL          *string `json:"url"`
	Version      *string `json:"version"`
	CommitHash   *string `json:"commit_hash"`
	FromManifest bool    `json:"from_manifest"`
}

type Extension struct {
	ID          string          `json:"id"`
	Kind        string          `json:"kind"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Source      ExtensionSource `json:"source"`
	Agents      []string        `json:"agents"`
	Tags        []string        `json:"tags"`
	Enabled     bool            `json:"enabled"`
	SourcePath  *string         `json:"source_path"`
	InstallMeta json.RawMessage `json:"install_meta"`
	Scope       ExtensionScope  `json:"scope"`
}

type ListExtensionsFilter struct {
	Kind      string
	Agent     string
	ScopeType string
	ScopePath string
}

func (c *Client) AddProject(ctx context.Context, path string) (Project, error) {
	var out Project
	err := c.call(ctx, "add_project", map[string]string{"path": path}, &out)
	return out, err
}

// EnsureProject registers a project before workspace-scoped operations. A
// duplicate is success because add_project is deliberately idempotent at this
// facade boundary. The following scan makes native agent files visible to
// list/install/delete immediately.
func (c *Client) EnsureProject(ctx context.Context, path string) error {
	if _, err := c.AddProject(ctx, path); err != nil && !IsConflict(err) {
		return err
	}
	return c.ScanAndSync(ctx)
}

func (c *Client) ScanAndSync(ctx context.Context) error {
	var count int
	return c.call(ctx, "scan_and_sync", map[string]any{}, &count)
}

func (c *Client) ListExtensions(ctx context.Context, filter ListExtensionsFilter) ([]Extension, error) {
	body := map[string]string{}
	if filter.Kind != "" {
		body["kind"] = filter.Kind
	}
	if filter.Agent != "" {
		body["agent"] = filter.Agent
	}
	if filter.ScopeType != "" {
		body["scope_type"] = filter.ScopeType
	}
	if filter.ScopePath != "" {
		body["scope_path"] = filter.ScopePath
	}
	var out []Extension
	err := c.call(ctx, "list_extensions", body, &out)
	return out, err
}

func (c *Client) InstallToAgent(
	ctx context.Context,
	extensionID, targetAgent, projectName, projectPath string,
) (string, error) {
	body := map[string]any{
		"extension_id":    extensionID,
		"target_agent":    targetAgent,
		"hermes_category": nil,
		"target_scope": map[string]string{
			"type": "project",
			"name": projectName,
			"path": projectPath,
		},
	}
	var out string
	err := c.call(ctx, "install_to_agent", body, &out)
	return out, err
}

func (c *Client) DeleteExtension(ctx context.Context, id string) error {
	var out any
	return c.call(ctx, "delete_extension", map[string]string{"id": id}, &out)
}

func (c *Client) UpdateExtension(ctx context.Context, id string) error {
	var out json.RawMessage
	return c.call(ctx, "update_extension", map[string]string{"id": id}, &out)
}

func (c *Client) call(ctx context.Context, command string, payload, out any) error {
	if c == nil || c.runtime == nil {
		return &APIError{StatusCode: http.StatusServiceUnavailable, Kind: "harnesskit_unavailable", Message: "HarnessKit runtime is not configured"}
	}
	baseURL, token, ready := c.runtime.Endpoint()
	if !ready {
		return &APIError{StatusCode: http.StatusServiceUnavailable, Kind: "harnesskit_unavailable", Message: "HarnessKit is not ready"}
	}
	base, err := url.Parse(baseURL)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return &APIError{StatusCode: http.StatusServiceUnavailable, Kind: "harnesskit_unavailable", Message: "HarnessKit endpoint is invalid"}
	}
	if strings.TrimSpace(token) == "" {
		return &APIError{StatusCode: http.StatusServiceUnavailable, Kind: "harnesskit_unavailable", Message: "HarnessKit token is unavailable"}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode HarnessKit %s request: %w", command, err)
	}
	target := base.ResolveReference(&url.URL{Path: filepath.ToSlash("/api/" + command)})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("build HarnessKit %s request: %w", command, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return &APIError{StatusCode: http.StatusBadGateway, Kind: "harnesskit_upstream_error", Message: err.Error()}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("read HarnessKit %s response: %w", command, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var detail struct {
			Kind    string `json:"kind"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(body, &detail)
		if detail.Message == "" {
			detail.Message = strings.TrimSpace(string(body))
		}
		if detail.Message == "" {
			detail.Message = http.StatusText(resp.StatusCode)
		}
		return &APIError{StatusCode: resp.StatusCode, Kind: detail.Kind, Message: detail.Message}
	}
	if out == nil || len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode HarnessKit %s response: %w", command, err)
	}
	return nil
}
