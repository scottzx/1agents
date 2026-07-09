package projectitems

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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
	return &Client{
		api:         &apiClient{baseURL: baseURL, token: token, http: &http.Client{Timeout: 30 * time.Second}},
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
	Name          string `json:"name"`
	Description   string `json:"description"`
	TargetDate    string `json:"targetDate"`
	PredecessorID string `json:"predecessorId"`
}

// updatableItemFields is the PATCH whitelist shared by the MCP update tool and
// the CLI update verb — only these keys are forwarded to the daemon.
var updatableItemFields = []string{
	"status", "issueState", "description", "acceptanceCriteria", "priority", "milestone", "type", "assignee", "verifier",
	"githubAssignees", "githubRepo", "githubKind", "githubNumber", "githubNodeId", "githubUrl", "githubState", "lastSyncedAt",
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
		"workspace_id":  c.workspaceID,
		"name":          a.Name,
		"description":   a.Description,
		"predecessorId": a.PredecessorID,
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
