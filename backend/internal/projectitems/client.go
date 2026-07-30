package projectitems

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Client is a workspace-scoped wrapper over the daemon's
// /api/agent/project-items* (and /api/agent/milestones*) HTTP API. Both the MCP
// server handlers (tools.go) and the Bash CLI (cli.go) go through it, so request
// construction — and therefore the daemon-side business logic it triggers (归口
// sourcing #68, per-project #N numbering, type discriminator, issueState
// open/closed, auto-close, links parsing) — is identical across both surfaces.
//
// Client carries NO session/task scope: workspace injection is the only
// isolation it applies. The executor/verifier task-lock rules (#50, #132) live
// entirely in the MCP *server and never touch Client — the CLI simply has no
// such scope.
type Client struct {
	api         *apiClient
	workspaceID string
}

// NewClient builds a Client with its own HTTP transport. token may be empty
// (loopback requests bypass auth unless a tunnel is active).
func NewClient(baseURL, workspaceID, token string) *Client {
	sessionID := os.Getenv("ONEAGENTS_SESSION_ID")
	sessionToken := os.Getenv("ONEAGENTS_SESSION_TOKEN")
	if token == "" && sessionID != "" {
		token = sessionToken
	}
	return &Client{
		api: &apiClient{
			baseURL:      baseURL,
			token:        token,
			sessionID:    sessionID,
			sessionToken: sessionToken,
			origin:       "cli",
			http:         &http.Client{Timeout: 30 * time.Second},
		},
		workspaceID: workspaceID,
	}
}

// CreateTaskArgs mirrors the create_project_item tool arguments and the CLI's
// `create --json` payload. Fields map 1:1 to the REST create body.
type CreateTaskArgs struct {
	Title               string   `json:"title"`
	Description         string   `json:"description"`
	AcceptanceCriteria  string   `json:"acceptanceCriteria"`
	Type                string   `json:"type"`
	Priority            string   `json:"priority"`
	Milestone           string   `json:"milestone"`
	FeatureID           string   `json:"featureId"`
	Assignee            string   `json:"assignee"`
	Verifier            string   `json:"verifier"`
	VerifierCount       int      `json:"verifierCount"`
	VerifyPassThreshold int      `json:"verifyPassThreshold"`
	DependsOn           []string `json:"dependsOn"`
	// Links are peer cross-references (归口), forwarded verbatim ([]{target, rel}).
	Links json.RawMessage `json:"links"`
	// Personal (assignee='user') scheduling: dueAt → scheduled trigger,
	// recurrence → repeat rule.
	DueAt      string          `json:"dueAt"`
	Recurrence json.RawMessage `json:"recurrence"`
	Checklist  json.RawMessage `json:"checklist"`
	// GitHub Issue/PR sync mapping (#74).
	GithubAssignees []string `json:"githubAssignees"`
	GithubRepo      string   `json:"githubRepo"`
	GithubKind      string   `json:"githubKind"`
	GithubNumber    int      `json:"githubNumber"`
	GithubNodeId    string   `json:"githubNodeId"`
	GithubUrl       string   `json:"githubUrl"`
	GithubState     string   `json:"githubState"`
	LastSyncedAt    string   `json:"lastSyncedAt"`
}

// CreateMilestoneArgs mirrors the create_milestone tool arguments.
type CreateMilestoneArgs struct {
	Bump        string `json:"bump"`
	Description string `json:"description"`
	TargetDate  string `json:"targetDate"`
}

// updatableItemFields is the PATCH whitelist shared by the MCP update tool and
// the CLI update verb — only these keys are forwarded to the daemon.
var updatableItemFields = []string{
	"status", "issueState", "description", "acceptanceCriteria", "priority", "milestone", "type", "assignee", "verifier",
	"githubAssignees", "githubRepo", "githubKind", "githubNumber", "githubNodeId", "githubUrl", "githubState", "lastSyncedAt", "featureId",
}

// ListTasks fetches every project item in the workspace (unfiltered).
func (c *Client) ListTasks() ([]task, error) {
	q := url.Values{"workspace_id": {c.workspaceID}}
	status, body, err := c.api.do("GET", "/api/agent/project-items", q, nil)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("list failed (%d): %s", status, strings.TrimSpace(string(body)))
	}
	var tasks []task
	if err := json.Unmarshal(body, &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

// GetTask returns the raw daemon JSON for a single item.
func (c *Client) GetTask(id string) (int, []byte, error) {
	return c.api.do("GET", "/api/agent/project-items/"+url.PathEscape(id), nil, nil)
}

// GetTaskGraph returns the raw cross-reference graph JSON for an item.
func (c *Client) GetTaskGraph(id string) (int, []byte, error) {
	return c.api.do("GET", "/api/agent/project-items/"+url.PathEscape(id)+"/graph", nil, nil)
}

// CreateTask builds the REST create body (workspace-scoped) and POSTs it.
func (c *Client) CreateTask(a CreateTaskArgs) (int, []byte, error) {
	body := map[string]any{
		"workspace_id":       c.workspaceID,
		"title":              a.Title,
		"description":        a.Description,
		"acceptanceCriteria": a.AcceptanceCriteria,
		"type":               a.Type,
		"priority":           a.Priority,
		"milestone":          a.Milestone,
		"assignee":           a.Assignee,
		"verifier":           a.Verifier,
		"dependsOn":          a.DependsOn,
	}
	if a.FeatureID != "" {
		body["featureId"] = a.FeatureID
	}
	if a.VerifierCount > 0 {
		body["verifierCount"] = a.VerifierCount
	}
	if a.VerifyPassThreshold > 0 {
		body["verifyPassThreshold"] = a.VerifyPassThreshold
	}
	if strings.TrimSpace(a.DueAt) != "" {
		body["scheduleType"] = "scheduled"
		body["scheduledAt"] = a.DueAt
	}
	if len(a.Recurrence) > 0 && string(a.Recurrence) != "null" {
		body["recurrence"] = a.Recurrence
	}
	if len(a.Links) > 0 && string(a.Links) != "null" {
		body["links"] = a.Links
	}
	if len(a.Checklist) > 0 && string(a.Checklist) != "null" {
		body["checklist"] = a.Checklist
	}
	if len(a.GithubAssignees) > 0 {
		body["githubAssignees"] = a.GithubAssignees
	}
	if a.GithubRepo != "" {
		body["githubRepo"] = a.GithubRepo
	}
	if a.GithubKind != "" {
		body["githubKind"] = a.GithubKind
	}
	if a.GithubNumber != 0 {
		body["githubNumber"] = a.GithubNumber
	}
	if a.GithubNodeId != "" {
		body["githubNodeId"] = a.GithubNodeId
	}
	if a.GithubUrl != "" {
		body["githubUrl"] = a.GithubUrl
	}
	if a.GithubState != "" {
		body["githubState"] = a.GithubState
	}
	if a.LastSyncedAt != "" {
		body["lastSyncedAt"] = a.LastSyncedAt
	}
	return c.api.do("POST", "/api/agent/project-items", nil, body)
}

// CreateDiscussion POSTs a type=discussion item.
func (c *Client) CreateDiscussion(title, desc string) (int, []byte, error) {
	body := map[string]any{
		"workspace_id": c.workspaceID,
		"title":        title,
		"description":  desc,
		"type":         "discussion",
	}
	return c.api.do("POST", "/api/agent/project-items", nil, body)
}

// UpdateTask PATCHes an item with the caller-supplied (already whitelisted)
// partial fields.
func (c *Client) UpdateTask(id string, patch map[string]json.RawMessage) (int, []byte, error) {
	return c.api.do("PATCH", "/api/agent/project-items/"+url.PathEscape(id), nil, patch)
}

// ListMilestones returns the raw milestones JSON for the workspace.
func (c *Client) ListMilestones() (int, []byte, error) {
	q := url.Values{"workspace_id": {c.workspaceID}}
	return c.api.do("GET", "/api/agent/milestones", q, nil)
}

// CreateMilestone builds the REST milestone body (workspace-scoped) and POSTs it.
func (c *Client) CreateMilestone(a CreateMilestoneArgs) (int, []byte, error) {
	body := map[string]any{
		"workspace_id": c.workspaceID,
		"bump":         a.Bump,
		"description":  a.Description,
	}
	if a.TargetDate != "" {
		body["targetDate"] = a.TargetDate
	}
	return c.api.do("POST", "/api/agent/milestones", nil, body)
}

// UpdateMilestone PATCHes a milestone. patch already carries workspace_id.
func (c *Client) UpdateMilestone(id string, patch map[string]any) (int, []byte, error) {
	return c.api.do("PATCH", "/api/agent/milestones/"+url.PathEscape(id), nil, patch)
}

// ListFeatureCatalog returns the normalized feature nodes and item links for
// this client's locked workspace.
func (c *Client) ListFeatureCatalog() (int, []byte, error) {
	q := url.Values{"workspace_id": {c.workspaceID}}
	return c.api.do("GET", "/api/agent/feature-catalog", q, nil)
}

func (c *Client) FeatureCatalogGantt() (int, []byte, error) {
	q := url.Values{"workspace_id": {c.workspaceID}}
	return c.api.do("GET", "/api/agent/feature-catalog/gantt", q, nil)
}

func (c *Client) FeatureCatalogExport(format string) (int, []byte, error) {
	q := url.Values{"workspace_id": {c.workspaceID}, "format": {format}}
	return c.api.do("GET", "/api/agent/feature-catalog/export", q, nil)
}

// CreateFeatureNode POSTs one module or feature point.
func (c *Client) CreateFeatureNode(input map[string]any) (int, []byte, error) {
	body := map[string]any{}
	for key, value := range input {
		body[key] = value
	}
	body["workspace_id"] = c.workspaceID
	return c.api.do("POST", "/api/agent/feature-catalog", nil, body)
}

// UpdateFeatureNode applies the same PATCH used by Web, including moves when
// parentId and/or position is present.
func (c *Client) UpdateFeatureNode(id string, patch map[string]any) (int, []byte, error) {
	body := map[string]any{}
	for key, value := range patch {
		body[key] = value
	}
	body["workspace_id"] = c.workspaceID
	return c.api.do("PATCH", "/api/agent/feature-catalog/"+url.PathEscape(id), nil, body)
}

func (c *Client) LinkFeatureItem(featureID, itemID, relation string) (int, []byte, error) {
	return c.api.do(
		"POST",
		"/api/agent/feature-catalog/"+url.PathEscape(featureID)+"/items",
		nil,
		map[string]any{
			"workspace_id": c.workspaceID,
			"itemId":       itemID,
			"relation":     relation,
		},
	)
}

func (c *Client) UnlinkFeatureItem(featureID, itemID, relation string) (int, []byte, error) {
	q := url.Values{
		"workspace_id": {c.workspaceID},
		"relation":     {relation},
	}
	return c.api.do(
		"DELETE",
		"/api/agent/feature-catalog/"+url.PathEscape(featureID)+"/items/"+url.PathEscape(itemID),
		q,
		nil,
	)
}

// BatchFeatureCatalog sends operations unchanged except for injecting the
// locked workspace. The daemon's FeatureCatalogStore owns reference resolution,
// validation, and the single transaction.
func (c *Client) BatchFeatureCatalog(operations json.RawMessage) (int, []byte, error) {
	return c.api.do("POST", "/api/agent/feature-catalog/batch", nil, map[string]any{
		"workspace_id": c.workspaceID,
		"operations":   operations,
	})
}

// ResolveNumber maps a per-project short number (the `#N` shown by `list`) to the
// item's UUID via the daemon's resolve endpoint, scoped to this client's
// workspace. Returns a not-found error when the workspace has no such number.
func (c *Client) ResolveNumber(number int) (string, error) {
	q := url.Values{"project": {c.workspaceID}, "number": {strconv.Itoa(number)}}
	status, body, err := c.api.do("GET", "/api/agent/project-items/resolve", q, nil)
	if err != nil {
		return "", err
	}
	if status != 200 {
		return "", fmt.Errorf("resolve #%d failed (%d): %s", number, status, strings.TrimSpace(string(body)))
	}
	var r struct {
		Task struct {
			ID string `json:"id"`
		} `json:"task"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return "", err
	}
	if r.Task.ID == "" {
		return "", fmt.Errorf("resolve #%d: no id in response", number)
	}
	return r.Task.ID, nil
}

// InWorkspace reports whether id belongs to the client's workspace — the guard
// that keeps id-addressable get/update/graph honest under project isolation.
func (c *Client) InWorkspace(id string) (bool, error) {
	tasks, err := c.ListTasks()
	if err != nil {
		return false, err
	}
	for _, t := range tasks {
		if t.ID == id {
			return true, nil
		}
	}
	return false, nil
}

// ── Workspace Inbox (#202 / #205) ───────────────────────────────────────────
// Mail tools proxy /api/inbox*. The Client always injects the locked workspace
// as the current box (list/get/accept/archive) or as fromWorkspaceId (send).

// ListInbox returns this workspace's inbox items. includeArchived=true includes
// archived rows (maps to ?archived=1).
func (c *Client) ListInbox(includeArchived bool) (int, []byte, error) {
	q := url.Values{"workspaceId": {c.workspaceID}}
	if includeArchived {
		q.Set("archived", "1")
	}
	return c.api.do("GET", "/api/inbox", q, nil)
}

// GetInboxItem returns one mail, scoped so the daemon rejects rows not owned
// by this workspace.
func (c *Client) GetInboxItem(id string) (int, []byte, error) {
	q := url.Values{"workspaceId": {c.workspaceID}}
	return c.api.do("GET", "/api/inbox/"+url.PathEscape(id), q, nil)
}

// AcceptMail adopts an inbox item as a requirement in this workspace.
func (c *Client) AcceptMail(id, title, description, priority string) (int, []byte, error) {
	body := map[string]any{"workspaceId": c.workspaceID}
	if title != "" {
		body["title"] = title
	}
	if description != "" {
		body["description"] = description
	}
	if priority != "" {
		body["priority"] = priority
	}
	return c.api.do("POST", "/api/inbox/"+url.PathEscape(id)+"/accept", nil, body)
}

// ArchiveMail archives an inbox item in this workspace.
func (c *Client) ArchiveMail(id string) (int, []byte, error) {
	q := url.Values{"workspaceId": {c.workspaceID}}
	return c.api.do("POST", "/api/inbox/"+url.PathEscape(id)+"/archive", q, nil)
}

// ListMailTargets returns workspaces that can receive mail.
func (c *Client) ListMailTargets() (int, []byte, error) {
	return c.api.do("GET", "/api/inbox/targets", nil, nil)
}

// SendMailArgs is the agent-facing send payload. From is forced by Client.
type SendMailArgs struct {
	ToWorkspaceID string   `json:"toWorkspaceId"`
	Title         string   `json:"title"`
	Content       string   `json:"content"`
	URL           string   `json:"url"`
	Summary       string   `json:"summary"`
	Tags          []string `json:"tags"`
	FromRef       string   `json:"fromRef"`
}

// SendMail delivers an envelope to target workspace. fromWorkspaceId is always
// this Client's workspace; source is always "agent". Callers cannot override.
func (c *Client) SendMail(a SendMailArgs) (int, []byte, error) {
	body := map[string]any{
		"workspaceId":     a.ToWorkspaceID,
		"source":          "agent",
		"fromWorkspaceId": c.workspaceID,
		"title":           a.Title,
		"content":         a.Content,
		"url":             a.URL,
		"summary":         a.Summary,
		"tags":            a.Tags,
		"fromRef":         a.FromRef,
	}
	return c.api.do("POST", "/api/inbox/deliver", nil, body)
}

// DeliverEnvelopeArgs is the function/data_source deliver path (#206): full
// envelope write via POST /api/inbox/deliver. Unlike SendMail, source is free
// (function|data_source|email|…) and fromWorkspaceId is optional.
type DeliverEnvelopeArgs struct {
	ToWorkspaceID   string   `json:"toWorkspaceId"`
	Source          string   `json:"source"`
	FromWorkspaceID string   `json:"fromWorkspaceId"`
	FromRef         string   `json:"fromRef"`
	Title           string   `json:"title"`
	Content         string   `json:"content"`
	URL             string   `json:"url"`
	Summary         string   `json:"summary"`
	Tags            []string `json:"tags"`
}

// DeliverEnvelope POSTs a full envelope (function / data_source / email producers).
// Empty ToWorkspaceID defaults to the Client's locked workspace.
func (c *Client) DeliverEnvelope(a DeliverEnvelopeArgs) (int, []byte, error) {
	to := strings.TrimSpace(a.ToWorkspaceID)
	if to == "" {
		to = c.workspaceID
	}
	body := map[string]any{
		"workspaceId":     to,
		"source":          a.Source,
		"fromWorkspaceId": a.FromWorkspaceID,
		"fromRef":         a.FromRef,
		"title":           a.Title,
		"content":         a.Content,
		"url":             a.URL,
		"summary":         a.Summary,
		"tags":            a.Tags,
	}
	return c.api.do("POST", "/api/inbox/deliver", nil, body)
}

// SetMailStatus flips inbox item status: archive | read | unread.
func (c *Client) SetMailStatus(id, action string) (int, []byte, error) {
	q := url.Values{"workspaceId": {c.workspaceID}}
	return c.api.do("POST", "/api/inbox/"+url.PathEscape(id)+"/"+url.PathEscape(action), q, nil)
}
