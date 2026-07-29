package projectitems

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
		"name":        "list_project_items",
		"description": "List project items (需求/缺陷/任务/讨论) in the current project. Optionally filter by status (pending, queued, running, completed, failed, cancelled, blocked) and/or type (task, requirement, bug). Returns a compact summary of each item including its short number, status, and dependencies.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status": map[string]any{"type": "string", "description": "Optional status filter."},
				"type":   map[string]any{"type": "string", "description": "Optional type filter: task, requirement, bug, or discussion."},
			},
		},
	},
	{
		"name":        "get_project_item",
		"description": "Fetch the full details of a single project item in the current project by its id, including description, acceptance criteria, and dependencies.",
		"inputSchema": map[string]any{
			"type":     "object",
			"required": []string{"id"},
			"properties": map[string]any{
				"id": map[string]any{"type": "string", "description": "The task id."},
			},
		},
	},
	{
		"name":        "get_project_item_graph",
		"description": "Fetch the cross-reference knowledge graph around a project item (#136): `outgoing` are the items it references (via `#N` mentions or explicit links), `incoming` are the items that reference it (backlinks). Walk this to trace why an item exists — e.g. from a task up to the requirement/bug it implements. Each edge carries the relation (relates/closes) and the peer item's id, number, title, type, and status.",
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
		"description": "List the project's milestones in roadmap order, each with its target date, position, and executable-task counts (total/completed; requirements/bugs and cancelled excluded). Use this to plan and group decomposed work into stages.",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
	},
	{
		"name":        "create_milestone",
		"description": "Create a new milestone (roadmap stage) in the current project. It is appended to the end of the roadmap. Assign tasks to it by passing its name as the `milestone` field of create_project_item / update_project_item. Pass predecessorId to place it after another milestone — milestones sharing a predecessor branch in parallel on the roadmap.",
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
		"name":        "create_project_item",
		"description": "Create a new project item (type task/requirement/bug) in the current project. Use dependsOn to express ordering when decomposing a PRD/Epic into dependent subtasks (pass the ids returned by earlier create_project_item calls). type defaults to 'task'; use 'requirement' or 'bug' for the requirement pool. IMPORTANT: an executable task (type 'task') MUST include acceptanceCriteria — without it the task is held as 未就绪 (not_ready) and never enters the scheduler queue. 任务归口 (#68): an executable task in a real project MUST also trace to a requirement or bug — either set parentId to the source requirement, pass its id in the `links` field (rel 'relates') right here at creation, or reference the source's #N in the description (e.g. \"实现 #5 ...\", which auto-creates the same relates link); otherwise it is held as not_ready until sourced. Subtasks inherit their parent's sourcing. Personal-bucket tasks (no project) are exempt.\n\nDecomposition flow: once the user has agreed on a top-level requirement, you may break it down into sub-requirements and executable tasks and schedule them directly — small sub-items do NOT need a separate user-confirmation round. When every task decomposed under a requirement (its ParentID children) reaches a terminal state, the requirement auto-closes.\n\nA personal reminder/todo/deadline for the USER is just a task with assignee='user': it is never dispatched to an agent, it lives on the user's calendar until they mark it done, and dueAt/recurrence schedule it. Use assignee='user' (with dueAt for a deadline) when the user asks you to remember something.\n\nGitHub mapping (#74): title/description/type/milestone map to native GitHub Issue fields; priority maps to a GitHub Projects v2 custom field (not an Issue field). NOTE the two distinct assignee dimensions: `assignee` is WHO executes this task — either an AI agent type (claudecode/codex, dispatched by the scheduler) or 'user' (a human/personal task, never dispatched) — and is local-only; `githubAssignees` are GitHub login names (issue.assignees[].login) for human collaborators. The github* reference fields (githubRepo/githubKind/githubNumber/githubNodeId/githubUrl/githubState/lastSyncedAt) are the sync anchor to a GitHub Issue/PR — normally backfilled by the sync pass, accept-only here, and not something you set when authoring a task.",
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
				"assignee":            map[string]any{"type": "string", "description": "WHO executes this task. An AGENT type (e.g. 'claudecode' or 'codex', whichever are installed) is dispatched by the scheduler. 'user' makes it a personal/human task: never dispatched, lives on the user's calendar until they mark it done (use with dueAt/recurrence for reminders/deadlines). LOCAL ONLY — not a GitHub user. Empty defaults to claudecode. For GitHub users, use githubAssignees."},
				"githubAssignees":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "GitHub login names (issue.assignees[].login) — human collaborators on the mapped Issue/PR. Distinct from `assignee` (the executing agent). Sync field."},
				"verifier":            map[string]any{"type": "string", "description": "Optional reviewing agent type. When set (and acceptanceCriteria is non-empty), after the executor finishes a verifier of this type auto-checks the output against the criteria; the task only completes when every criterion passes, otherwise it re-executes. Empty = no verification."},
				"verifierCount":       map[string]any{"type": "integer", "description": "Optional. How many independent verifiers form an adversarial review panel (#131). >1 runs that many separate verifier passes that each judge the output; the panel decides by threshold. Default/0/1 = a single verifier."},
				"verifyPassThreshold": map[string]any{"type": "integer", "description": "Optional. How many of the verifierCount verdicts must pass for the panel to accept the output. 0 = simple majority (⌊N/2⌋+1). Set equal to verifierCount to require unanimity."},
				"dependsOn":           map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Ids of tasks this one depends on."},
				"links": map[string]any{
					"type":        "array",
					"description": "Peer cross-reference links to other tasks, set here at creation so you don't need a follow-up update_project_item. Use this to trace an executable task back to the requirement/bug it implements (任务归口 #68): pass {target: <the source requirement's id>, rel: 'relates'}. Referencing the source's #N in the description auto-creates the same 'relates' link, so links and #N are one and the same relation — pick whichever is convenient.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"target": map[string]any{"type": "string", "description": "The peer task's id (e.g. the requirement returned by an earlier create_project_item)."},
							"rel":    map[string]any{"type": "string", "enum": []string{"relates"}, "description": "Relation kind. 'relates' is a plain cross-reference / sourcing link."},
						},
						"required": []string{"target"},
					},
				},
				"dueAt": map[string]any{"type": "string", "description": "Optional due/trigger time, RFC3339 (e.g. '2026-06-25T15:00:00+08:00'). For a personal task (assignee='user') this is the calendar deadline/reminder time. Omit for an undated item."},
				"recurrence": map[string]any{
					"type":        "object",
					"description": "Optional repeat rule for a personal (assignee='user') task. Omit for a one-off.",
					"properties": map[string]any{
						"freq":       map[string]any{"type": "string", "enum": []string{"daily", "weekly", "monthly", "yearly"}},
						"interval":   map[string]any{"type": "integer", "description": "Every N periods (default 1), e.g. every 2 weeks."},
						"weekday":    map[string]any{"type": "integer", "description": "0=Sunday…6=Saturday (single-day weekly)."},
						"daysOfWeek": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": "Multi-day weekly, e.g. [6,0] for every Sat & Sun."},
						"monthday":   map[string]any{"type": "integer", "description": "1–31 (absolute monthly/yearly day)."},
						"weekIndex":  map[string]any{"type": "integer", "description": "Relative month/year: 1..4, or -1 for last, combined with daysOfWeek (e.g. first Monday)."},
						"month":      map[string]any{"type": "integer", "description": "1–12 (yearly only)."},
						"at":         map[string]any{"type": "string", "description": "'HH:MM' local time; defaults to midnight."},
						"until":      map[string]any{"type": "string", "description": "Stop after this date (RFC3339 or 'YYYY-MM-DD')."},
						"count":      map[string]any{"type": "integer", "description": "Stop after this many occurrences."},
					},
				},
				"checklist": map[string]any{
					"type":        "array",
					"description": "Optional embedded checklist — ordered progress items the executor ticks off. Each item: {text, done?}.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"text": map[string]any{"type": "string"},
							"done": map[string]any{"type": "boolean"},
						},
						"required": []string{"text"},
					},
				},
				"githubRepo":   map[string]any{"type": "string", "description": "Sync anchor: 'owner/repo' of the bound GitHub object. Normally backfilled by sync."},
				"githubKind":   map[string]any{"type": "string", "enum": []string{"issue", "pr"}, "description": "Sync anchor: whether the bound GitHub object is an issue or a pr."},
				"githubNumber": map[string]any{"type": "integer", "description": "Sync anchor: the remote #N (per-repo), distinct from the local task number."},
				"githubNodeId": map[string]any{"type": "string", "description": "Sync anchor: GraphQL global node id (needed by the Projects v2 API)."},
				"githubUrl":    map[string]any{"type": "string", "description": "Sync anchor: the object's html_url."},
				"githubState":  map[string]any{"type": "string", "description": "Sync anchor: remote open/closed state, for conflict detection."},
				"lastSyncedAt": map[string]any{"type": "string", "description": "Sync anchor: RFC3339 timestamp of the last successful GitHub sync."},
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
		"name":        "update_project_item",
		"description": "Update fields of an existing project item in the current project. Two distinct 'done' dimensions: `status` is the TASK lifecycle (only 'completed'/'cancelled' settable; runnable states stay scheduler-owned) — use it for executable tasks; `issueState` is the issue OPEN/CLOSED state — this is how you close a **requirement or bug** (a non-executable issue item whose 'done' is issueState='closed', not status). Set issueState='closed' to close/archive an item on the board, 'open' to reopen. Only the fields you pass are changed. See create_project_item for the GitHub field mapping and the `assignee` (executing agent, local) vs `githubAssignees` (GitHub logins) distinction. The github* reference fields are the sync anchor and are normally written by the sync pass.",
		"inputSchema": map[string]any{
			"type":     "object",
			"required": []string{"id"},
			"properties": map[string]any{
				"id":                 map[string]any{"type": "string"},
				"status":             map[string]any{"type": "string", "enum": []string{"completed", "cancelled"}, "description": "Task lifecycle (executable tasks). Only 'completed'/'cancelled' are settable here."},
				"issueState":         map[string]any{"type": "string", "enum": []string{"open", "closed"}, "description": "Issue open/closed state. Set 'closed' to close a requirement/bug (their 'done' is closing, not status='completed'); 'open' to reopen. Independent of status."},
				"description":        map[string]any{"type": "string"},
				"acceptanceCriteria": map[string]any{"type": "string", "description": "Concrete, checkable acceptance criteria. Fill this in to move an executable task out of the not_ready hold and into the scheduler queue."},
				"priority":           map[string]any{"type": "string", "enum": []string{"urgent", "high", "medium", "low"}, "description": "Local scheduling priority (maps to a Projects v2 field, not a native Issue field)."},
				"milestone":          map[string]any{"type": "string"},
				"type":               map[string]any{"type": "string", "enum": []string{"task", "requirement", "bug"}},
				"assignee":           map[string]any{"type": "string", "description": "Executing AGENT type, e.g. 'claudecode' or 'codex' (local only, NOT a GitHub user). Empty defaults to claudecode."},
				"githubAssignees":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "GitHub login names (human collaborators). Distinct from `assignee`. Sync field."},
				"verifier":           map[string]any{"type": "string", "description": "Reviewing agent type for post-execution verification; empty disables verification. See create_project_item."},
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
		"description": "Submit your verification verdict for the task under review (verifier role only). Report one result per acceptance criterion: pass=true if the executor's output meets it, pass=false with a concrete comment on what is missing. The server marks the task completed only when EVERY criterion passes; otherwise it sends the task back for another execution round (or fails it once the review budget is exhausted). Set needsHuman=true instead when the blocker is NOT something the executor can fix by retrying — it needs a human design, architecture, or tradeoff decision — and explain what the human must decide in summary; the server escalates the task to a human rather than looping. You do not set the task status yourself — this verdict drives it.",
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
				"needsHuman": map[string]any{"type": "boolean", "description": "Set true to escalate to a human instead of rejecting: the artifact needs a design/architecture/tradeoff decision that retrying the executor won't resolve. Explain what the human must decide in summary. Does not consume the review budget."},
				"summary":    map[string]any{"type": "string", "description": "Optional overall note recorded with the verdict; when needsHuman=true, state plainly what needs human judgement."},
			},
		},
	},
}

// complete_human_project_item + Workspace Inbox mail tools (appended at init).
func init() {
	toolDefs = append(toolDefs, map[string]any{
		"name":        "complete_human_project_item",
		"description": "Mark a human-executor project item (executor=human, status=awaiting_human) as completed, optionally recording a decision payload. Calling this tool unlocks all downstream items that depend on it. Use this to record human decisions (e.g. 'approved', 'selected option B') and advance the pipeline.",
		"inputSchema": map[string]any{
			"type":     "object",
			"required": []string{"id"},
			"properties": map[string]any{
				"id":      map[string]any{"type": "string", "description": "The human task id to complete."},
				"payload": map[string]any{"type": "string", "description": "Optional JSON payload recording the human decision (e.g. '{\"choice\":\"B\"}')."},
				"summary": map[string]any{"type": "string", "description": "Optional free-text summary of the decision."},
			},
		},
	})
	// Workspace Inbox mail tools (#202 Phase2 / #205). PM scope only — not
	// added to executorScopedTools / verifierScopedTools, so task-locked
	// sessions never see write mail tools. from on send_mail is forced by the
	// server to the env-injected workspace.
	toolDefs = append(toolDefs,
		map[string]any{
			"name":        "check_inbox",
			"description": "List mail in the current Workspace Inbox. Default: non-archived items. Pass status='unread' for unread only, or includeArchived=true to include archived. Use this at session start or when the user asks you to check the inbox.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"status":          map[string]any{"type": "string", "enum": []string{"unread", "read", "all"}, "description": "Optional filter. 'unread' / 'read' filter after list; 'all' (or omit) returns non-archived by default."},
					"includeArchived": map[string]any{"type": "boolean", "description": "When true, include archived items."},
				},
			},
		},
		map[string]any{
			"name":        "get_mail",
			"description": "Fetch full details of one inbox item in the current Workspace Inbox by id.",
			"inputSchema": map[string]any{
				"type":     "object",
				"required": []string{"id"},
				"properties": map[string]any{
					"id": map[string]any{"type": "string", "description": "Inbox item id."},
				},
			},
		},
		map[string]any{
			"name":        "accept_mail",
			"description": "Adopt an inbox item as a requirement in the current Workspace (reuses PMO Dispatch: type=requirement + dispatched-from backlink, mail marked read). Optionally override title/description/priority.",
			"inputSchema": map[string]any{
				"type":     "object",
				"required": []string{"id"},
				"properties": map[string]any{
					"id":          map[string]any{"type": "string", "description": "Inbox item id to accept."},
					"title":       map[string]any{"type": "string", "description": "Optional title override; defaults from the mail."},
					"description": map[string]any{"type": "string", "description": "Optional description override; defaults from summary/content/url."},
					"priority":    map[string]any{"type": "string", "enum": []string{"urgent", "high", "medium", "low"}},
				},
			},
		},
		map[string]any{
			"name":        "archive_mail",
			"description": "Archive an inbox item in the current Workspace Inbox (status→archived; never deleted). Optional reason is recorded in the tool response only.",
			"inputSchema": map[string]any{
				"type":     "object",
				"required": []string{"id"},
				"properties": map[string]any{
					"id":     map[string]any{"type": "string", "description": "Inbox item id."},
					"reason": map[string]any{"type": "string", "description": "Optional archive reason for the transcript."},
				},
			},
		},
		map[string]any{
			"name":        "list_mail_targets",
			"description": "List Workspaces that can receive mail (active projects/workforce, excluding personal bucket). Use before send_mail to pick toWorkspaceId.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
		},
		map[string]any{
			"name":        "send_mail",
			"description": "Deliver an envelope to another Workspace's Inbox. The sender (fromWorkspaceId) is forced to the current workspace; source is always 'agent'. This is the only allowed cross-workspace write — it only inserts an inbox row on the target, never mutates their ProjectItems.",
			"inputSchema": map[string]any{
				"type":     "object",
				"required": []string{"toWorkspaceId", "title"},
				"properties": map[string]any{
					"toWorkspaceId": map[string]any{"type": "string", "description": "Recipient Workspace id (from list_mail_targets)."},
					"title":         map[string]any{"type": "string"},
					"content":       map[string]any{"type": "string"},
					"url":           map[string]any{"type": "string"},
					"summary":       map[string]any{"type": "string"},
					"tags":          map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"fromRef":       map[string]any{"type": "string", "description": "Optional producer label (role name / function id)."},
				},
			},
		},
	)
}

// reviewVerifier is "verifier" — the ONEAGENTS_TASK_ROLE value selecting the
// hard read-only review scope.
const reviewVerifier = "verifier"

// executorScopedTools is the tool subset advertised and accepted in a task-
// locked executor session: read its own task, list (filtered to itself), and
// update its own task. The PM-only create_*/milestone tools are withheld so the
// lock cannot be sidestepped by creating sibling tasks. See #50.
var executorScopedTools = map[string]bool{
	"list_project_items":          true,
	"get_project_item":            true,
	"get_project_item_graph":      true,
	"update_project_item":         true,
	"complete_human_project_item": true,
}

// verifierScopedTools is the hard read-only review subset: read the task, list
// (filtered to itself), and submit a verdict. update_project_item is deliberately
// absent — a verifier judges, it never edits the task. See #50.
var verifierScopedTools = map[string]bool{
	"list_project_items":     true,
	"get_project_item":       true,
	"get_project_item_graph": true,
	"submit_review":          true,
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
		// #132: in executor scope, update_project_item must not advertise `completed` —
		// an executor cannot self-report done (the run-finish artifact event does).
		// Swap in the cancel-only variant so the agent isn't told to try a status
		// the server will reject.
		if name == "update_project_item" && s.taskRole != reviewVerifier {
			out = append(out, executorUpdateTaskDef)
			continue
		}
		out = append(out, d)
	}
	return out
}

// executorUpdateTaskDef is the executor-scoped advertisement of update_project_item: the
// settable status is narrowed to `cancelled` only, since completion is driven by
// the artifact/verification path, not by the agent (#132).
var executorUpdateTaskDef = map[string]any{
	"name":        "update_project_item",
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
	SessionID          string   `json:"sessionId"`
	TurnID             string   `json:"turnId"`
	EventID            string   `json:"eventId"`
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
	switch p.Name {
	case "list_project_items":
		return s.toolListTasks(p.Arguments)
	case "get_project_item":
		return s.toolGetTask(p.Arguments)
	case "get_project_item_graph":
		return s.toolGetTaskGraph(p.Arguments)
	case "list_milestones":
		return s.toolListMilestones()
	case "create_milestone":
		return s.toolCreateMilestone(p.Arguments)
	case "update_milestone":
		return s.toolUpdateMilestone(p.Arguments)
	case "create_project_item":
		return s.toolCreateTask(p.Arguments)
	case "create_discussion":
		return s.toolCreateDiscussion(p.Arguments)
	case "update_project_item":
		return s.toolUpdateTask(p.Arguments)
	case "submit_review":
		return s.toolSubmitReview(p.Arguments)
	case "complete_human_project_item":
		return s.toolCompleteHumanTask(p.Arguments)
	case "check_inbox":
		return s.toolCheckInbox(p.Arguments)
	case "get_mail":
		return s.toolGetMail(p.Arguments)
	case "accept_mail":
		return s.toolAcceptMail(p.Arguments)
	case "archive_mail":
		return s.toolArchiveMail(p.Arguments)
	case "list_mail_targets":
		return s.toolListMailTargets()
	case "send_mail":
		return s.toolSendMail(p.Arguments)
	default:
		return toolErr("unknown tool: " + p.Name)
	}
}

// cl returns a workspace-scoped Client view of this server's transport, so the
// MCP handlers and the CLI (cli.go) share one request-construction core.
func (s *server) cl() *Client {
	return &Client{api: s.api, workspaceID: s.workspaceID}
}

// listTasks fetches every task in the locked workspace.
func (s *server) listTasks() ([]task, error) {
	return s.cl().ListTasks()
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
	return toolJSON(map[string]any{"count": len(out), "items": out})
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
	status, body, err := s.cl().GetTask(a.ID)
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
	status, body, err := s.cl().GetTaskGraph(a.ID)
	if err != nil {
		return toolErr(err.Error())
	}
	if status != 200 {
		return toolErr(fmt.Sprintf("get task graph failed (%d): %s", status, strings.TrimSpace(string(body))))
	}
	return toolText(string(body))
}

func (s *server) toolListMilestones() map[string]any {
	status, body, err := s.cl().ListMilestones()
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
	var a CreateMilestoneArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return toolErr("invalid arguments: " + err.Error())
	}
	if strings.TrimSpace(a.Name) == "" {
		return toolErr("name is required")
	}
	status, resp, err := s.cl().CreateMilestone(a)
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
	status, resp, err := s.cl().UpdateMilestone(id, patch)
	if err != nil {
		return toolErr(err.Error())
	}
	if status != 200 {
		return toolErr(fmt.Sprintf("update milestone failed (%d): %s", status, strings.TrimSpace(string(resp))))
	}
	return toolText(string(resp))
}

func (s *server) toolCreateTask(args json.RawMessage) map[string]any {
	var a CreateTaskArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return toolErr("invalid arguments: " + err.Error())
	}
	if strings.TrimSpace(a.Title) == "" {
		return toolErr("title is required")
	}
	status, resp, err := s.cl().CreateTask(a)
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
		"sessionId": created.SessionID,
		"turnId":    created.TurnID,
		"eventId":   created.EventID,
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
	status, resp, err := s.cl().CreateDiscussion(a.Title, a.Description)
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
		"ok":        true,
		"id":        created.ID,
		"number":    created.Number,
		"title":     created.Title,
		"type":      created.Type,
		"sessionId": created.SessionID,
		"turnId":    created.TurnID,
		"eventId":   created.EventID,
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
	for _, f := range updatableItemFields {
		if v, ok := raw[f]; ok {
			patch[f] = v
		}
	}
	if len(patch) == 0 {
		return toolErr("no updatable fields provided")
	}
	status, resp, err := s.cl().UpdateTask(id, patch)
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
		NeedsHuman bool   `json:"needsHuman"`
		Summary    string `json:"summary"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return toolErr("invalid arguments: " + err.Error())
	}
	if len(a.Criteria) == 0 {
		return toolErr("criteria is required: report a result for each acceptance criterion")
	}
	body := map[string]any{
		"criteria":   a.Criteria,
		"needsHuman": a.NeedsHuman,
		"summary":    a.Summary,
	}
	status, resp, err := s.api.do("POST", "/api/agent/project-items/"+url.PathEscape(s.taskID)+"/review", nil, body)
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
	ok, err := s.cl().InWorkspace(id)
	return err == nil && ok
}

// toolCompleteHumanTask marks a human-executor task (status=awaiting_human) as
// completed. The caller may provide an optional JSON payload recording the
// decision and a free-text summary. On success, the scheduler's next tick
// releases all downstream tasks that depended on this one (#324).
func (s *server) toolCompleteHumanTask(args json.RawMessage) map[string]any {
	var a struct {
		ID      string `json:"id"`
		Payload string `json:"payload"`
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal(args, &a); err != nil || a.ID == "" {
		return toolErr("id is required")
	}
	if !s.idInWorkspace(a.ID) {
		return toolErr("task not found in this workspace: " + a.ID)
	}
	body := map[string]any{
		"status":  "completed",
		"summary": a.Summary,
	}
	if a.Payload != "" {
		body["result"] = a.Payload
	}
	status, resp, err := s.api.do("PATCH", "/api/agent/project-items/"+url.PathEscape(a.ID), nil, body)
	if err != nil {
		return toolErr(err.Error())
	}
	if status != 200 {
		return toolErr(fmt.Sprintf("complete_human_project_item failed (%d): %s", status, strings.TrimSpace(string(resp))))
	}
	return toolText("human task " + a.ID + " completed")
}

// ── Workspace Inbox mail tools (#205) ───────────────────────────────────────

func (s *server) toolCheckInbox(args json.RawMessage) map[string]any {
	var a struct {
		Status          string `json:"status"`
		IncludeArchived bool   `json:"includeArchived"`
	}
	_ = json.Unmarshal(args, &a)
	status, body, err := s.cl().ListInbox(a.IncludeArchived)
	if err != nil {
		return toolErr(err.Error())
	}
	if status != 200 {
		return toolErr(fmt.Sprintf("check_inbox failed (%d): %s", status, strings.TrimSpace(string(body))))
	}
	var payload struct {
		Items  []json.RawMessage `json:"items"`
		Unread int               `json:"unread"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return toolText(string(body))
	}
	// Optional status filter client-side (daemon list is workspace-scoped).
	want := strings.ToLower(strings.TrimSpace(a.Status))
	if want == "" || want == "all" {
		return toolJSON(map[string]any{
			"count":  len(payload.Items),
			"unread": payload.Unread,
			"items":  payload.Items,
		})
	}
	filtered := make([]json.RawMessage, 0, len(payload.Items))
	for _, raw := range payload.Items {
		var it struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(raw, &it); err != nil {
			continue
		}
		if it.Status == want {
			filtered = append(filtered, raw)
		}
	}
	return toolJSON(map[string]any{
		"count":  len(filtered),
		"unread": payload.Unread,
		"items":  filtered,
	})
}

func (s *server) toolGetMail(args json.RawMessage) map[string]any {
	var a struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(args, &a); err != nil || strings.TrimSpace(a.ID) == "" {
		return toolErr("id is required")
	}
	status, body, err := s.cl().GetInboxItem(a.ID)
	if err != nil {
		return toolErr(err.Error())
	}
	if status != 200 {
		return toolErr(fmt.Sprintf("get_mail failed (%d): %s", status, strings.TrimSpace(string(body))))
	}
	return toolText(string(body))
}

func (s *server) toolAcceptMail(args json.RawMessage) map[string]any {
	var a struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Priority    string `json:"priority"`
	}
	if err := json.Unmarshal(args, &a); err != nil || strings.TrimSpace(a.ID) == "" {
		return toolErr("id is required")
	}
	status, body, err := s.cl().AcceptMail(a.ID, a.Title, a.Description, a.Priority)
	if err != nil {
		return toolErr(err.Error())
	}
	if status != 200 {
		return toolErr(fmt.Sprintf("accept_mail failed (%d): %s", status, strings.TrimSpace(string(body))))
	}
	return toolText(string(body))
}

func (s *server) toolArchiveMail(args json.RawMessage) map[string]any {
	var a struct {
		ID     string `json:"id"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(args, &a); err != nil || strings.TrimSpace(a.ID) == "" {
		return toolErr("id is required")
	}
	status, body, err := s.cl().ArchiveMail(a.ID)
	if err != nil {
		return toolErr(err.Error())
	}
	if status != 200 {
		return toolErr(fmt.Sprintf("archive_mail failed (%d): %s", status, strings.TrimSpace(string(body))))
	}
	if strings.TrimSpace(a.Reason) == "" {
		return toolText(string(body))
	}
	return toolJSON(map[string]any{
		"item":   json.RawMessage(body),
		"reason": a.Reason,
	})
}

func (s *server) toolListMailTargets() map[string]any {
	status, body, err := s.cl().ListMailTargets()
	if err != nil {
		return toolErr(err.Error())
	}
	if status != 200 {
		return toolErr(fmt.Sprintf("list_mail_targets failed (%d): %s", status, strings.TrimSpace(string(body))))
	}
	return toolText(string(body))
}

func (s *server) toolSendMail(args json.RawMessage) map[string]any {
	var a SendMailArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return toolErr("invalid arguments: " + err.Error())
	}
	if strings.TrimSpace(a.ToWorkspaceID) == "" {
		return toolErr("toWorkspaceId is required")
	}
	if strings.TrimSpace(a.Title) == "" && strings.TrimSpace(a.Content) == "" && strings.TrimSpace(a.URL) == "" {
		return toolErr("title, content or url is required")
	}
	// Client.SendMail forces fromWorkspaceId=current and source=agent.
	status, body, err := s.cl().SendMail(a)
	if err != nil {
		return toolErr(err.Error())
	}
	if status != 200 {
		return toolErr(fmt.Sprintf("send_mail failed (%d): %s", status, strings.TrimSpace(string(body))))
	}
	return toolText(string(body))
}
