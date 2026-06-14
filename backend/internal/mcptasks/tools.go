package mcptasks

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
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
				"type":   map[string]any{"type": "string", "description": "Optional type filter: task, requirement, or bug."},
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
		"description": "List the milestones used in the current project with task counts (total and completed) for each. Useful for grouping decomposed work.",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
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
				"dependsOn":          map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Ids of tasks this one depends on."},
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
			},
		},
	},
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
	switch p.Name {
	case "list_tasks":
		return s.toolListTasks(p.Arguments)
	case "get_task":
		return s.toolGetTask(p.Arguments)
	case "list_milestones":
		return s.toolListMilestones()
	case "create_task":
		return s.toolCreateTask(p.Arguments)
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
	if !s.idInWorkspace(a.ID) {
		return toolErr("task not found in this project: " + a.ID)
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
	tasks, err := s.listTasks()
	if err != nil {
		return toolErr(err.Error())
	}
	type bucket struct {
		Milestone string `json:"milestone"`
		Total     int    `json:"total"`
		Completed int    `json:"completed"`
	}
	idx := map[string]*bucket{}
	order := []string{}
	for _, t := range tasks {
		if t.Milestone == "" {
			continue
		}
		b, ok := idx[t.Milestone]
		if !ok {
			b = &bucket{Milestone: t.Milestone}
			idx[t.Milestone] = b
			order = append(order, t.Milestone)
		}
		b.Total++
		if t.Status == "completed" {
			b.Completed++
		}
	}
	sort.Strings(order)
	out := make([]bucket, 0, len(order))
	for _, m := range order {
		out = append(out, *idx[m])
	}
	return toolJSON(map[string]any{"count": len(out), "milestones": out})
}

func (s *server) toolCreateTask(args json.RawMessage) map[string]any {
	var a struct {
		Title              string   `json:"title"`
		Description        string   `json:"description"`
		AcceptanceCriteria string   `json:"acceptanceCriteria"`
		Type               string   `json:"type"`
		Priority           string   `json:"priority"`
		Milestone          string   `json:"milestone"`
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
	if !s.idInWorkspace(id) {
		return toolErr("task not found in this project: " + id)
	}

	patch := map[string]json.RawMessage{}
	for _, f := range []string{"status", "description", "acceptanceCriteria", "priority", "milestone", "type"} {
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
