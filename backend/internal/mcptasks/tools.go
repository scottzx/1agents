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
		"name":        "get_task_graph",
		"description": "Fetch the cross-reference knowledge graph around a task (#136): `outgoing` are the tasks it references (via `#N` mentions or explicit links), `incoming` are the tasks that reference it (backlinks). Walk this to trace why a task exists — e.g. from a task up to the requirement/bug it implements. Each edge carries the relation (relates/closes) and the peer task's id, number, title, type, and status.",
		"inputSchema": map[string]any{
			"type":     "object",
			"required": []string{"id"},
			"properties": map[string]any{
				"id": map[string]any{"type": "string", "description": "The task id whose references and backlinks to fetch."},
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
		"description": "Create a new task in the current project. Use dependsOn to express ordering when decomposing a PRD/Epic into dependent subtasks (pass the ids returned by earlier create_task calls). type defaults to 'task'; use 'requirement' or 'bug' for the requirement pool. IMPORTANT: an executable task (type 'task') MUST include acceptanceCriteria — without it the task is held as 未就绪 (not_ready) and never enters the scheduler queue. 任务归口 (#68): an executable task in a real project MUST also trace to a requirement or bug — reference the source's #N in the description (e.g. \"实现 #5 ...\", which auto-creates a relates link) or it is likewise held as not_ready until sourced. Subtasks inherit their parent's sourcing. Personal-bucket tasks (no project) are exempt.\n\nGitHub mapping (#74): title/description/type/milestone map to native GitHub Issue fields; priority maps to a GitHub Projects v2 custom field (not an Issue field). NOTE the two distinct assignee dimensions: `assignee` is the executing AI agent type (claudecode/codex) and is local-only; `githubAssignees` are GitHub login names (issue.assignees[].login) for human collaborators. The github* reference fields (githubRepo/githubKind/githubNumber/githubNodeId/githubUrl/githubState/lastSyncedAt) are the sync anchor to a GitHub Issue/PR — normally backfilled by the sync pass, accept-only here, and not something you set when authoring a task.",
		"inputSchema": map[string]any{
			"type":     "object",
			"required": []string{"title"},
			"properties": map[string]any{
				"title":               map[string]any{"type": "string"},
				"description":         map[string]any{"type": "string", "description": "The work instruction for the executing agent. Maps to GitHub issue.body."},
				"acceptanceCriteria":  map[string]any{"type": "string", "description": "Required for executable tasks: concrete, checkable criteria for the task to be accepted as done. A task without acceptance criteria is held as not_ready and never scheduled."},
				"type":                map[string]any{"type": "string", "enum": []string{"task", "requirement", "bug"}, "description": "Issue discriminator. Maps to GitHub Issue Types / a label."},
				"priority":            map[string]any{"type": "string", "enum": []string{"urgent", "high", "medium", "low"}, "description": "Local scheduling priority. No native GitHub Issue field — maps to a Projects v2 custom field."},
				"milestone":           map[string]any{"type": "string", "description": "Maps to GitHub milestone (matched/created by title)."},
				"assignee":            map[string]any{"type": "string", "description": "Executing AGENT type for this task, e.g. 'claudecode' or 'codex' (whichever agents are installed). LOCAL ONLY — this is NOT a GitHub user. Empty defaults to claudecode. For GitHub users, use githubAssignees."},
				"githubAssignees":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "GitHub login names (issue.assignees[].login) — human collaborators on the mapped Issue/PR. Distinct from `assignee` (the executing agent). Sync field."},
				"verifier":            map[string]any{"type": "string", "description": "Optional reviewing agent type. When set (and acceptanceCriteria is non-empty), after the executor finishes a verifier of this type auto-checks the output against the criteria; the task only completes when every criterion passes, otherwise it re-executes. Empty = no verification."},
				"verifierCount":       map[string]any{"type": "integer", "description": "Optional. How many independent verifiers form an adversarial review panel (#131). >1 runs that many separate verifier passes that each judge the output; the panel decides by threshold. Default/0/1 = a single verifier."},
				"verifyPassThreshold": map[string]any{"type": "integer", "description": "Optional. How many of the verifierCount verdicts must pass for the panel to accept the output. 0 = simple majority (⌊N/2⌋+1). Set equal to verifierCount to require unanimity."},
				"dependsOn":           map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Ids of tasks this one depends on."},
				"githubRepo":          map[string]any{"type": "string", "description": "Sync anchor: 'owner/repo' of the bound GitHub object. Normally backfilled by sync."},
				"githubKind":          map[string]any{"type": "string", "enum": []string{"issue", "pr"}, "description": "Sync anchor: whether the bound GitHub object is an issue or a pr."},
				"githubNumber":        map[string]any{"type": "integer", "description": "Sync anchor: the remote #N (per-repo), distinct from the local task number."},
				"githubNodeId":        map[string]any{"type": "string", "description": "Sync anchor: GraphQL global node id (needed by the Projects v2 API)."},
				"githubUrl":           map[string]any{"type": "string", "description": "Sync anchor: the object's html_url."},
				"githubState":         map[string]any{"type": "string", "description": "Sync anchor: remote open/closed state, for conflict detection."},
				"lastSyncedAt":        map[string]any{"type": "string", "description": "Sync anchor: RFC3339 timestamp of the last successful GitHub sync."},
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
		"description": "Update fields of an existing task in the current project. status may only be set to 'completed' or 'cancelled' (runnable states stay scheduler-owned). Only the fields you pass are changed. See create_task for the GitHub field mapping and the `assignee` (executing agent, local) vs `githubAssignees` (GitHub logins) distinction. The github* reference fields are the sync anchor and are normally written by the sync pass.",
		"inputSchema": map[string]any{
			"type":     "object",
			"required": []string{"id"},
			"properties": map[string]any{
				"id":                 map[string]any{"type": "string"},
				"status":             map[string]any{"type": "string", "enum": []string{"completed", "cancelled"}},
				"description":        map[string]any{"type": "string"},
				"acceptanceCriteria": map[string]any{"type": "string", "description": "Concrete, checkable acceptance criteria. Fill this in to move an executable task out of the not_ready hold and into the scheduler queue."},
				"priority":           map[string]any{"type": "string", "enum": []string{"urgent", "high", "medium", "low"}, "description": "Local scheduling priority (maps to a Projects v2 field, not a native Issue field)."},
				"milestone":          map[string]any{"type": "string"},
				"type":               map[string]any{"type": "string", "enum": []string{"task", "requirement", "bug"}},
				"assignee":           map[string]any{"type": "string", "description": "Executing AGENT type, e.g. 'claudecode' or 'codex' (local only, NOT a GitHub user). Empty defaults to claudecode."},
				"githubAssignees":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "GitHub login names (human collaborators). Distinct from `assignee`. Sync field."},
				"verifier":           map[string]any{"type": "string", "description": "Reviewing agent type for post-execution verification; empty disables verification. See create_task."},
				"githubRepo":         map[string]any{"type": "string", "description": "Sync anchor: 'owner/repo'."},
				"githubKind":         map[string]any{"type": "string", "enum": []string{"issue", "pr"}, "description": "Sync anchor: issue or pr."},
				"githubNumber":       map[string]any{"type": "integer", "description": "Sync anchor: remote #N."},
				"githubNodeId":       map[string]any{"type": "string", "description": "Sync anchor: GraphQL node id."},
				"githubUrl":          map[string]any{"type": "string", "description": "Sync anchor: html_url."},
				"githubState":        map[string]any{"type": "string", "description": "Sync anchor: remote open/closed."},
				"lastSyncedAt":       map[string]any{"type": "string", "description": "Sync anchor: RFC3339 last-synced timestamp."},
			},
		},
	},
	{
		"name":        "submit_review",
		"description": "Submit your verification verdict for the task under review (verifier role only). Report one result per acceptance criterion: pass=true if the executor's output meets it, pass=false with a concrete comment on what is missing. The server marks the task completed only when EVERY criterion passes; otherwise it sends the task back for another execution round (or fails it once the review budget is exhausted). You do not set the task status yourself — this verdict drives it.",
		"inputSchema": map[string]any{
			"type":     "object",
			"required": []string{"criteria"},
			"properties": map[string]any{
				"criteria": map[string]any{
					"type":        "array",
					"description": "Per-criterion judgement; report every acceptance criterion.",
					"items": map[string]any{
						"type":     "object",
						"required": []string{"criterion", "pass"},
						"properties": map[string]any{
							"criterion": map[string]any{"type": "string", "description": "The acceptance criterion being judged (quote/paraphrase it)."},
							"pass":      map[string]any{"type": "boolean", "description": "Whether the executor's output satisfies this criterion."},
							"comment":   map[string]any{"type": "string", "description": "Required when pass=false: what is missing and how to fix it."},
						},
					},
				},
				"summary": map[string]any{"type": "string", "description": "Optional overall note recorded with the verdict."},
			},
		},
	},
	{
		"name":        "create_reminder",
		"description": "Record a personal reminder / todo / deadline for the USER in the current project. The executor is the user themselves — it is NEVER dispatched to or run by an agent; it just lives on the user's calendar until they mark it done. Use this (not create_task) when the user explicitly asks you to remember a todo, note a deadline, or remind them of something. Set dueAt for a deadline/scheduled reminder; add recurrence for a repeating one.",
		"inputSchema": map[string]any{
			"type":     "object",
			"required": []string{"title"},
			"properties": map[string]any{
				"title": map[string]any{"type": "string", "description": "Short reminder text, e.g. 'Submit the quarterly report'."},
				"note":  map[string]any{"type": "string", "description": "Optional extra detail (Markdown supported)."},
				"dueAt": map[string]any{"type": "string", "description": "Optional due/trigger time, RFC3339 (e.g. '2026-06-25T15:00:00+08:00'). Omit for an undated todo."},
				"recurrence": map[string]any{
					"type":        "object",
					"description": "Optional repeat rule. Omit for a one-off reminder.",
					"properties": map[string]any{
						"freq":     map[string]any{"type": "string", "enum": []string{"daily", "weekly", "monthly"}},
						"weekday":  map[string]any{"type": "integer", "description": "0=Sunday…6=Saturday (for weekly)."},
						"monthday": map[string]any{"type": "integer", "description": "1–31 (for monthly)."},
						"at":       map[string]any{"type": "string", "description": "'HH:MM' local time; defaults to midnight."},
					},
				},
			},
		},
	},
}

// reviewVerifier is "verifier" — the ONEAGENTS_TASK_ROLE value selecting the
// hard read-only review scope.
const reviewVerifier = "verifier"

// reminderRole is "reminder" — the ONEAGENTS_TASK_ROLE value for a general
// (non-PM) chat session: project-wide but narrowed to recording personal
// reminders (#192). Unlike executor/verifier it is not task-locked.
const reminderRole = "reminder"

// reminderScopedTools is the surface for a reminder-role session: record a
// personal reminder and read the current list (for dedup/context). All other
// PM tools are withheld so a general chat agent cannot create project tasks,
// milestones, or discussions.
var reminderScopedTools = map[string]bool{
	"create_reminder": true,
	"list_tasks":      true,
}

// executorScopedTools is the tool subset advertised and accepted in a task-
// locked executor session: read its own task, list (filtered to itself), and
// update its own task. The PM-only create_*/milestone tools are withheld so the
// lock cannot be sidestepped by creating sibling tasks. See #50.
var executorScopedTools = map[string]bool{
	"list_tasks":     true,
	"get_task":       true,
	"get_task_graph": true,
	"update_task":    true,
}

// verifierScopedTools is the hard read-only review subset: read the task, list
// (filtered to itself), and submit a verdict. update_task is deliberately
// absent — a verifier judges, it never edits the task. See #50.
var verifierScopedTools = map[string]bool{
	"list_tasks":     true,
	"get_task":       true,
	"get_task_graph": true,
	"submit_review":  true,
}

// scopedTools returns the allowed tool set for the current task-locked role:
// verifier → hard read-only + submit_review; otherwise the executor surface.
func (s *server) scopedTools() map[string]bool {
	if s.taskRole == reviewVerifier {
		return verifierScopedTools
	}
	return executorScopedTools
}

// listedTools returns the tools advertised by tools/list: the PM set for a
// project-wide session, or the narrowed task-scoped subset when locked. The
// PM set excludes submit_review (it is meaningless without a task lock and a
// verifier role); the scoped sets allow-list explicitly.
func (s *server) listedTools() []map[string]any {
	out := make([]map[string]any, 0, len(toolDefs))
	if s.taskID == "" {
		if s.taskRole == reminderRole {
			for _, d := range toolDefs {
				if name, _ := d["name"].(string); reminderScopedTools[name] {
					out = append(out, d)
				}
			}
			return out
		}
		for _, d := range toolDefs {
			if name, _ := d["name"].(string); name == "submit_review" {
				continue
			}
			out = append(out, d)
		}
		return out
	}
	allowed := s.scopedTools()
	for _, d := range toolDefs {
		name, _ := d["name"].(string)
		if !allowed[name] {
			continue
		}
		// #132: in executor scope, update_task must not advertise `completed` —
		// an executor cannot self-report done (the run-finish artifact event does).
		// Swap in the cancel-only variant so the agent isn't told to try a status
		// the server will reject.
		if name == "update_task" && s.taskRole != reviewVerifier {
			out = append(out, executorUpdateTaskDef)
			continue
		}
		out = append(out, d)
	}
	return out
}

// executorUpdateTaskDef is the executor-scoped advertisement of update_task: the
// settable status is narrowed to `cancelled` only, since completion is driven by
// the artifact/verification path, not by the agent (#132).
var executorUpdateTaskDef = map[string]any{
	"name":        "update_task",
	"description": "Update fields of your assigned task. NOTE: you cannot mark it completed — finish your run and the system routes it to verification/completion based on the artifact you produced. status may only be set to 'cancelled' (give up, with a reason). Only the fields you pass are changed.",
	"inputSchema": map[string]any{
		"type":     "object",
		"required": []string{"id"},
		"properties": map[string]any{
			"id":                 map[string]any{"type": "string"},
			"status":             map[string]any{"type": "string", "enum": []string{"cancelled"}, "description": "Only 'cancelled' is settable by an executor; completion is artifact/verification-driven."},
			"description":        map[string]any{"type": "string"},
			"acceptanceCriteria": map[string]any{"type": "string"},
			"priority":           map[string]any{"type": "string", "enum": []string{"urgent", "high", "medium", "low"}},
			"milestone":          map[string]any{"type": "string"},
			"type":               map[string]any{"type": "string", "enum": []string{"task", "requirement", "bug"}},
			"assignee":           map[string]any{"type": "string"},
			"verifier":           map[string]any{"type": "string"},
		},
	},
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
	if s.taskID != "" && !s.scopedTools()[p.Name] {
		return toolErr(fmt.Sprintf("tool %q is not available in this task-scoped session", p.Name))
	}
	if s.taskID == "" && s.taskRole == reminderRole && !reminderScopedTools[p.Name] {
		return toolErr(fmt.Sprintf("tool %q is not available in this reminder-scoped session", p.Name))
	}
	switch p.Name {
	case "list_tasks":
		return s.toolListTasks(p.Arguments)
	case "get_task":
		return s.toolGetTask(p.Arguments)
	case "get_task_graph":
		return s.toolGetTaskGraph(p.Arguments)
	case "list_milestones":
		return s.toolListMilestones()
	case "create_milestone":
		return s.toolCreateMilestone(p.Arguments)
	case "update_milestone":
		return s.toolUpdateMilestone(p.Arguments)
	case "create_task":
		return s.toolCreateTask(p.Arguments)
	case "create_reminder":
		return s.toolCreateReminder(p.Arguments)
	case "create_discussion":
		return s.toolCreateDiscussion(p.Arguments)
	case "update_task":
		return s.toolUpdateTask(p.Arguments)
	case "submit_review":
		return s.toolSubmitReview(p.Arguments)
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

func (s *server) toolGetTaskGraph(args json.RawMessage) map[string]any {
	var a struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(args, &a); err != nil || a.ID == "" {
		return toolErr("id is required")
	}
	if !s.idInScope(a.ID) {
		return toolErr("task not accessible in this session: " + a.ID)
	}
	status, body, err := s.api.do("GET", "/api/agent/tasks/"+url.PathEscape(a.ID)+"/graph", nil, nil)
	if err != nil {
		return toolErr(err.Error())
	}
	if status != 200 {
		return toolErr(fmt.Sprintf("get task graph failed (%d): %s", status, strings.TrimSpace(string(body))))
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
		// GitHub Issue/PR mapping (#74). githubAssignees is the human-collaborator
		// dimension (distinct from assignee); the github* refs are the sync anchor.
		GithubAssignees []string `json:"githubAssignees"`
		GithubRepo      string   `json:"githubRepo"`
		GithubKind      string   `json:"githubKind"`
		GithubNumber    int      `json:"githubNumber"`
		GithubNodeId    string   `json:"githubNodeId"`
		GithubUrl       string   `json:"githubUrl"`
		GithubState     string   `json:"githubState"`
		LastSyncedAt    string   `json:"lastSyncedAt"`
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
		"verifier":           a.Verifier,
		"dependsOn":          a.DependsOn,
	}
	if a.VerifierCount > 0 {
		body["verifierCount"] = a.VerifierCount
	}
	if a.VerifyPassThreshold > 0 {
		body["verifyPassThreshold"] = a.VerifyPassThreshold
	}
	// Forward GitHub mapping fields only when provided, so a normal create never
	// writes empty anchors over a row a future sync pass might populate.
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

// toolCreateReminder records a personal reminder/todo for the user (#192). It is
// a task pinned to assignee="user" so the scheduler never runs it; an optional
// dueAt becomes the scheduled trigger time and recurrence makes it repeat.
func (s *server) toolCreateReminder(args json.RawMessage) map[string]any {
	var a struct {
		Title      string          `json:"title"`
		Note       string          `json:"note"`
		DueAt      string          `json:"dueAt"`
		Recurrence json.RawMessage `json:"recurrence"`
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
		"description":  a.Note,
		"type":         "task",
		"assignee":     "user", // executor is the user; scheduler skips it
	}
	if strings.TrimSpace(a.DueAt) != "" {
		body["scheduleType"] = "scheduled"
		body["scheduledAt"] = a.DueAt
	}
	if len(a.Recurrence) > 0 && string(a.Recurrence) != "null" {
		body["recurrence"] = a.Recurrence
	}
	status, resp, err := s.api.do("POST", "/api/agent/tasks", nil, body)
	if err != nil {
		return toolErr(err.Error())
	}
	if status != 200 {
		return toolErr(fmt.Sprintf("create reminder failed (%d): %s", status, strings.TrimSpace(string(resp))))
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

	// #132: an executor cannot self-report completion. Task completion is driven
	// by an artifact event (the run finishing → done event → pending_review /
	// verifier verdict), never by the agent writing status=completed. Finishing
	// the run *is* the "I'm done" signal; the state machine decides from there.
	// A task-scoped executor may still set status=cancelled (a genuine give-up),
	// but completed is reserved for the artifact/verification path.
	if s.taskID != "" && s.taskRole != reviewVerifier {
		if v, ok := raw["status"]; ok {
			var st string
			_ = json.Unmarshal(v, &st)
			if st == "completed" {
				return toolErr("an executor cannot mark its task completed — finish your run and the system routes it to verification/completion based on the artifact. You may only set status=cancelled here.")
			}
		}
	}

	patch := map[string]json.RawMessage{}
	for _, f := range []string{"status", "description", "acceptanceCriteria", "priority", "milestone", "type", "assignee", "verifier",
		"githubAssignees", "githubRepo", "githubKind", "githubNumber", "githubNodeId", "githubUrl", "githubState", "lastSyncedAt"} {
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

// toolSubmitReview posts the verifier's verdict to the daemon's review
// endpoint, which computes overall pass (all criteria) and drives the task's
// state machine (completed / re-execute / failed). Verifier scope only; the
// task is always the env-locked one, so no id parameter is accepted.
func (s *server) toolSubmitReview(args json.RawMessage) map[string]any {
	if s.taskID == "" {
		return toolErr("submit_review is only available in a task-scoped verifier session")
	}
	var a struct {
		Criteria []struct {
			Criterion string `json:"criterion"`
			Pass      bool   `json:"pass"`
			Comment   string `json:"comment"`
		} `json:"criteria"`
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return toolErr("invalid arguments: " + err.Error())
	}
	if len(a.Criteria) == 0 {
		return toolErr("criteria is required: report a result for each acceptance criterion")
	}
	body := map[string]any{
		"criteria": a.Criteria,
		"summary":  a.Summary,
	}
	status, resp, err := s.api.do("POST", "/api/agent/tasks/"+url.PathEscape(s.taskID)+"/review", nil, body)
	if err != nil {
		return toolErr(err.Error())
	}
	if status != 200 {
		return toolErr(fmt.Sprintf("submit review failed (%d): %s", status, strings.TrimSpace(string(resp))))
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
