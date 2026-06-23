package mcptasks

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// toolDefs is the static list returned by tools/list. Schemas are intentionally
// small — none expose a workspace/project parameter, so the agent is locked to
// the env-injected workspace.
var toolDefs = []map[string]any{
	{
		"name":        "list_tasks",
		"description": "List tasks in the current project. Optionally filter by status (pending, queued, running, completed, failed, cancelled, blocked) and/or type (task, requirement, bug). Returns a compact summary of each task including its short number, status, and dependencies.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status": map[string]any{"type": "string", "description": "Optional status filter."},
				"type":   map[string]any{"type": "string", "description": "Optional type filter: task, requirement, bug, or discussion."},
			},
		},
	},
	{
		"name":        "get_task",
		"description": "Fetch the full details of a single task in the current project by its id, including description, acceptance criteria, and dependencies.",
		"inputSchema": map[string]any{
			"type":     "object",
			"required": []string{"id"},
			"properties": map[string]any{
				"id": map[string]any{"type": "string", "description": "The task id."},
			},
		},
	},
	{
		"name":        "list_milestones",
		"description": "List the project's milestones in roadmap order, each with its target date, position, and task counts (total and completed). Use this to plan and group decomposed work into stages.",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
	},
	{
		"name":        "create_milestone",
		"description": "Create a new milestone (roadmap stage) in the current project. It is appended to the end of the roadmap. Assign tasks to it by passing its name as the `milestone` field of create_task / update_task. Pass predecessorId to place it after another milestone — milestones sharing a predecessor branch in parallel on the roadmap.",
		"inputSchema": map[string]any{
			"type":     "object",
			"required": []string{"name"},
			"properties": map[string]any{
				"name":          map[string]any{"type": "string", "description": "Milestone name; must be unique within the project."},
				"description":   map[string]any{"type": "string"},
				"targetDate":    map[string]any{"type": "string", "description": "Optional target/due date, RFC3339 (e.g. 2026-07-01T00:00:00Z)."},
				"predecessorId": map[string]any{"type": "string", "description": "Optional id of the predecessor (parent) milestone; empty makes it a root."},
			},
		},
	},
	{
		"name":        "update_milestone",
		"description": "Update a milestone by id. Renaming cascades to every task assigned to it. Only the fields you pass are changed. Set predecessorId to re-parent it on the roadmap (empty string makes it a root).",
		"inputSchema": map[string]any{
			"type":     "object",
			"required": []string{"id"},
			"properties": map[string]any{
				"id":            map[string]any{"type": "string"},
				"name":          map[string]any{"type": "string"},
				"description":   map[string]any{"type": "string"},
				"targetDate":    map[string]any{"type": "string", "description": "Target/due date, RFC3339."},
				"predecessorId": map[string]any{"type": "string", "description": "Predecessor (parent) milestone id; empty makes it a root."},
			},
		},
	},
	{
		"name":        "create_task",
		"description": "Create a new task in the current project. Use dependsOn to express ordering when decomposing a PRD/Epic into dependent subtasks (pass the ids returned by earlier create_task calls). type defaults to 'task'; use 'requirement' or 'bug' for the requirement pool.",
		"inputSchema": map[string]any{
			"type":     "object",
			"required": []string{"title"},
			"properties": map[string]any{
				"title":              map[string]any{"type": "string"},
				"description":        map[string]any{"type": "string", "description": "The work instruction for the executing agent."},
				"acceptanceCriteria": map[string]any{"type": "string", "description": "Concrete, checkable criteria for the task to be accepted as done."},
				"type":               map[string]any{"type": "string", "enum": []string{"task", "requirement", "bug"}},
				"priority":           map[string]any{"type": "string", "enum": []string{"urgent", "high", "medium", "low"}},
				"milestone":          map[string]any{"type": "string"},
				"assignee":           map[string]any{"type": "string", "description": "Executing agent type for this task, e.g. 'claudecode' or 'codex' (whichever agents are installed). Set this to run different tasks on different agents. Empty defaults to claudecode."},
				"dependsOn":          map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Ids of tasks this one depends on."},
			},
		},
	},
	{
		"name":        "create_discussion",
		"description": "Create a discussion post in the current project. A discussion is a free-form conceptual/directional record with NO clear deliverable — it never gets scheduled or executed. Use this for pure discussion that isn't yet a concrete requirement/bug/task; once a clear, deliverable goal emerges, create a requirement/bug/task instead.",
		"inputSchema": map[string]any{
			"type":     "object",
			"required": []string{"title"},
			"properties": map[string]any{
				"title":       map[string]any{"type": "string"},
				"description": map[string]any{"type": "string", "description": "The discussion body (Markdown supported)."},
			},
		},
	},
	{
		"name":        "update_task",
		"description": "Update fields of an existing task in the current project. status may only be set to 'completed' or 'cancelled' (runnable states stay scheduler-owned). Only the fields you pass are changed.",
		"inputSchema": map[string]any{
			"type":     "object",
			"required": []string{"id"},
			"properties": map[string]any{
				"id":                 map[string]any{"type": "string"},
				"status":             map[string]any{"type": "string", "enum": []string{"completed", "cancelled"}},
				"description":        map[string]any{"type": "string"},
				"acceptanceCriteria": map[string]any{"type": "string"},
				"priority":           map[string]any{"type": "string", "enum": []string{"urgent", "high", "medium", "low"}},
				"milestone":          map[string]any{"type": "string"},
				"type":               map[string]any{"type": "string", "enum": []string{"task", "requirement", "bug"}},
				"assignee":           map[string]any{"type": "string", "description": "Executing agent type, e.g. 'claudecode' or 'codex'. Empty defaults to claudecode."},
			},
		},
	},
}

// taskScopedTools is the tool subset advertised and accepted in a task-locked
// (executor) session: read its own task, list (filtered to itself), and update
// its own task. The PM-only create_*/milestone tools are withheld so the lock
// cannot be sidestepped by creating sibling tasks. See #50.
var taskScopedTools = map[string]bool{
	"list_tasks":  true,
	"get_task":    true,
	"update_task": true,
}

// listedTools returns the tools advertised by tools/list: the full set for a
// project-wide PM session, or the narrowed task-scoped subset when locked.
func (s *server) listedTools() []map[string]any {
	if s.taskID == "" {
		return toolDefs
	}
	out := make([]map[string]any, 0, len(taskScopedTools))
	for _, d := range toolDefs {
		if name, _ := d["name"].(string); taskScopedTools[name] {
			out = append(out, d)
		}
	}
	return out
}

// idInScope reports whether id is addressable in this session: exactly the
// locked task when task-scoped, otherwise any task in the locked workspace.
func (s *server) idInScope(id string) bool {
	if s.taskID != "" {
		return id == s.taskID
	}
	return s.idInWorkspace(id)
}

// task is the subset of the daemon's task JSON the tools surface.
type task struct {
	ID                 string   `json:"id"`
	Number             int      `json:"number"`
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	Status             string   `json:"status"`
	Type               string   `json:"type"`
	Priority           string   `json:"priority"`
	Milestone          string   `json:"milestone"`
	AcceptanceCriteria string   `json:"acceptanceCriteria"`
	DependsOn          []string `json:"dependsOn"`
	IssueState         string   `json:"issueState"`
}

func (s *server) onToolCall(params json.RawMessage) map[string]any {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return toolErr("invalid tool call params: " + err.Error())
	}
	if s.taskID != "" && !taskScopedTools[p.Name] {
		return toolErr(fmt.Sprintf("tool %q is not available in this task-scoped session", p.Name))
	}
	switch p.Name {
	case "list_tasks":
		return s.toolListTasks(p.Arguments)
	case "get_task":
		return s.toolGetTask(p.Arguments)
	case "list_milestones":
		return s.toolListMilestones()
	case "create_milestone":
		return s.toolCreateMilestone(p.Arguments)
	case "update_milestone":
		return s.toolUpdateMilestone(p.Arguments)
	case "create_task":
		return s.toolCreateTask(p.Arguments)
	case "create_discussion":
		return s.toolCreateDiscussion(p.Arguments)
	case "update_task":
		return s.toolUpdateTask(p.Arguments)
	default:
		return toolErr("unknown tool: " + p.Name)
	}
}

// listTasks fetches every task in the locked workspace.
func (s *server) listTasks() ([]task, error) {
	q := url.Values{"workspace_id": {s.workspaceID}}
	status, body, err := s.api.do("GET", "/api/agent/tasks", q, nil)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("list tasks failed (%d): %s", status, strings.TrimSpace(string(body)))
	}
	var tasks []task
	if err := json.Unmarshal(body, &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (s *server) toolListTasks(args json.RawMessage) map[string]any {
	var a struct {
		Status string `json:"status"`
		Type   string `json:"type"`
	}
	_ = json.Unmarshal(args, &a)

	tasks, err := s.listTasks()
	if err != nil {
		return toolErr(err.Error())
	}
	out := make([]task, 0, len(tasks))
	for _, t := range tasks {
		if s.taskID != "" && t.ID != s.taskID {
			continue // task-locked: only the bound task is visible
		}
		if a.Status != "" && t.Status != a.Status {
			continue
		}
		if a.Type != "" && t.Type != a.Type {
			continue
		}
		t.Description = "" // keep the summary compact
		t.AcceptanceCriteria = ""
		out = append(out, t)
	}
	return toolJSON(map[string]any{"count": len(out), "tasks": out})
}

func (s *server) toolGetTask(args json.RawMessage) map[string]any {
	var a struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(args, &a); err != nil || a.ID == "" {
		return toolErr("id is required")
	}
	if !s.idInScope(a.ID) {
		return toolErr("task not accessible in this session: " + a.ID)
	}
	status, body, err := s.api.do("GET", "/api/agent/tasks/"+url.PathEscape(a.ID), nil, nil)
	if err != nil {
		return toolErr(err.Error())
	}
	if status != 200 {
		return toolErr(fmt.Sprintf("get task failed (%d): %s", status, strings.TrimSpace(string(body))))
	}
	return toolText(string(body))
}

func (s *server) toolListMilestones() map[string]any {
	q := url.Values{"workspace_id": {s.workspaceID}}
	status, body, err := s.api.do("GET", "/api/agent/milestones", q, nil)
	if err != nil {
		return toolErr(err.Error())
	}
	if status != 200 {
		return toolErr(fmt.Sprintf("list milestones failed (%d): %s", status, strings.TrimSpace(string(body))))
	}
	var milestones []json.RawMessage
	if err := json.Unmarshal(body, &milestones); err != nil {
		return toolText(string(body))
	}
	return toolJSON(map[string]any{"count": len(milestones), "milestones": json.RawMessage(body)})
}

func (s *server) toolCreateMilestone(args json.RawMessage) map[string]any {
	var a struct {
		Name          string `json:"name"`
		Description   string `json:"description"`
		TargetDate    string `json:"targetDate"`
		PredecessorID string `json:"predecessorId"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return toolErr("invalid arguments: " + err.Error())
	}
	if strings.TrimSpace(a.Name) == "" {
		return toolErr("name is required")
	}
	body := map[string]any{
		"workspace_id":  s.workspaceID,
		"name":          a.Name,
		"description":   a.Description,
		"predecessorId": a.PredecessorID,
	}
	if a.TargetDate != "" {
		body["targetDate"] = a.TargetDate
	}
	status, resp, err := s.api.do("POST", "/api/agent/milestones", nil, body)
	if err != nil {
		return toolErr(err.Error())
	}
	if status != 200 {
		return toolErr(fmt.Sprintf("create milestone failed (%d): %s", status, strings.TrimSpace(string(resp))))
	}
	return toolText(string(resp))
}

func (s *server) toolUpdateMilestone(args json.RawMessage) map[string]any {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(args, &raw); err != nil {
		return toolErr("invalid arguments: " + err.Error())
	}
	idRaw, ok := raw["id"]
	if !ok {
		return toolErr("id is required")
	}
	var id string
	if err := json.Unmarshal(idRaw, &id); err != nil || id == "" {
		return toolErr("id is required")
	}
	patch := map[string]any{"workspace_id": s.workspaceID}
	for _, f := range []string{"name", "description", "targetDate", "predecessorId"} {
		if v, ok := raw[f]; ok {
			patch[f] = v
		}
	}
	if len(patch) == 1 {
		return toolErr("no updatable fields provided")
	}
	status, resp, err := s.api.do("PATCH", "/api/agent/milestones/"+url.PathEscape(id), nil, patch)
	if err != nil {
		return toolErr(err.Error())
	}
	if status != 200 {
		return toolErr(fmt.Sprintf("update milestone failed (%d): %s", status, strings.TrimSpace(string(resp))))
	}
	return toolText(string(resp))
}

func (s *server) toolCreateTask(args json.RawMessage) map[string]any {
	var a struct {
		Title              string   `json:"title"`
		Description        string   `json:"description"`
		AcceptanceCriteria string   `json:"acceptanceCriteria"`
		Type               string   `json:"type"`
		Priority           string   `json:"priority"`
		Milestone          string   `json:"milestone"`
		Assignee           string   `json:"assignee"`
		DependsOn          []string `json:"dependsOn"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return toolErr("invalid arguments: " + err.Error())
	}
	if strings.TrimSpace(a.Title) == "" {
		return toolErr("title is required")
	}
	body := map[string]any{
		"workspace_id":       s.workspaceID,
		"title":              a.Title,
		"description":        a.Description,
		"acceptanceCriteria": a.AcceptanceCriteria,
		"type":               a.Type,
		"priority":           a.Priority,
		"milestone":          a.Milestone,
		"assignee":           a.Assignee,
		"dependsOn":          a.DependsOn,
	}
	status, resp, err := s.api.do("POST", "/api/agent/tasks", nil, body)
	if err != nil {
		return toolErr(err.Error())
	}
	if status != 200 {
		return toolErr(fmt.Sprintf("create task failed (%d): %s", status, strings.TrimSpace(string(resp))))
	}
	var created task
	if err := json.Unmarshal(resp, &created); err != nil {
		return toolText(string(resp))
	}
	return toolJSON(map[string]any{
		"ok":        true,
		"id":        created.ID,
		"number":    created.Number,
		"title":     created.Title,
		"status":    created.Status,
		"dependsOn": created.DependsOn,
	})
}

func (s *server) toolCreateDiscussion(args json.RawMessage) map[string]any {
	var a struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return toolErr("invalid arguments: " + err.Error())
	}
	if strings.TrimSpace(a.Title) == "" {
		return toolErr("title is required")
	}
	body := map[string]any{
		"workspace_id": s.workspaceID,
		"title":        a.Title,
		"description":  a.Description,
		"type":         "discussion",
	}
	status, resp, err := s.api.do("POST", "/api/agent/tasks", nil, body)
	if err != nil {
		return toolErr(err.Error())
	}
	if status != 200 {
		return toolErr(fmt.Sprintf("create discussion failed (%d): %s", status, strings.TrimSpace(string(resp))))
	}
	var created task
	if err := json.Unmarshal(resp, &created); err != nil {
		return toolText(string(resp))
	}
	return toolJSON(map[string]any{
		"ok":     true,
		"id":     created.ID,
		"number": created.Number,
		"title":  created.Title,
		"type":   created.Type,
	})
}

func (s *server) toolUpdateTask(args json.RawMessage) map[string]any {
	// Decode into a generic map so only the fields the caller supplied are
	// forwarded (the PATCH endpoint applies partial edits).
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(args, &raw); err != nil {
		return toolErr("invalid arguments: " + err.Error())
	}
	idRaw, ok := raw["id"]
	if !ok {
		return toolErr("id is required")
	}
	var id string
	if err := json.Unmarshal(idRaw, &id); err != nil || id == "" {
		return toolErr("id is required")
	}
	if !s.idInScope(id) {
		return toolErr("task not accessible in this session: " + id)
	}

	patch := map[string]json.RawMessage{}
	for _, f := range []string{"status", "description", "acceptanceCriteria", "priority", "milestone", "type", "assignee"} {
		if v, ok := raw[f]; ok {
			patch[f] = v
		}
	}
	if len(patch) == 0 {
		return toolErr("no updatable fields provided")
	}
	status, resp, err := s.api.do("PATCH", "/api/agent/tasks/"+url.PathEscape(id), nil, patch)
	if err != nil {
		return toolErr(err.Error())
	}
	if status != 200 {
		return toolErr(fmt.Sprintf("update task failed (%d): %s", status, strings.TrimSpace(string(resp))))
	}
	return toolText(string(resp))
}

// idInWorkspace reports whether id belongs to the locked workspace. The
// single-task and PATCH endpoints are addressable by global id, so this guard
// keeps the workspace lock honest for get/update.
func (s *server) idInWorkspace(id string) bool {
	tasks, err := s.listTasks()
	if err != nil {
		return false
	}
	for _, t := range tasks {
		if t.ID == id {
			return true
		}
	}
	return false
}
