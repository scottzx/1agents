// Package meta owns the global metadata database (~/.1agents/meta.db).
//
// It is the single persistence layer for projects, tasks (with their issue
// timeline), and chat-session index records — shared by the HTTP server and
// the CLI subcommands (both open the same SQLite file in WAL mode).
//
// Model types here were moved from internal/agent (which now aliases them)
// so that the wire JSON shapes stay byte-identical to the legacy JSON-file
// stores. See docs/features/project-model/design.md.
package meta

import (
	"encoding/json"
	"strings"
	"time"
)

// ProjectStatus is a project's lifecycle phase (issue #141). A project leaves
// the active view via two paths that share one terminal mechanism but carry a
// distinct reason: 阶段性完成归档 (archived) vs 竞品出现砍掉 (killed). Data is
// never deleted — archived/killed rows just drop out of the active list and
// stay visible in the archive view.
type ProjectStatus string

const (
	ProjectStatusActive   ProjectStatus = "active"
	ProjectStatusArchived ProjectStatus = "archived"
	ProjectStatusKilled   ProjectStatus = "killed"
	// ProjectStatusSystem marks a workspace owned by the platform itself, not by
	// the user (数据源同步宿主 __sources_sync__ is the first — Epic #359 phase 2).
	// The sidebar/registry still hides it (only "active" appears there), but the
	// scheduler schedules it and agenda/dashboard/task-bus views surface its
	// tasks explicitly, so periodic system work is visible without cluttering
	// the user's project list.
	ProjectStatusSystem ProjectStatus = "system"
)

// ArchiveReason records why a project left the active view. It is orthogonal to
// status so the UI can show "归档：完成" vs "关闭：竞品出现" without overloading the
// status enum. Empty while the project is active.
type ArchiveReason string

const (
	// ArchiveReasonCompleted — 阶段性完成 → 归档沉淀 (pairs with ProjectStatusArchived).
	ArchiveReasonCompleted ArchiveReason = "completed"
	// ArchiveReasonSuperseded — 竞品出现 / 大厂已做 → 判定无必要继续 → 砍掉
	// (pairs with ProjectStatusKilled).
	ArchiveReasonSuperseded ArchiveReason = "superseded"
)

// Project is one managed workspace directory. Project ID equals the
// workspace ID from the workspace registry, so the two concepts stay 1:1.
type Project struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	WorkspacePath string        `json:"workspacePath"`
	Status        ProjectStatus `json:"status"` // active | archived | killed
	// ArchiveReason explains why an archived/killed project closed; empty while
	// active. ArchiveNote is the optional free-text rationale captured at close
	// time. ArchivedAt is the close timestamp; nil while active.
	ArchiveReason ArchiveReason `json:"archiveReason,omitempty"`
	ArchiveNote   string        `json:"archiveNote,omitempty"`
	ArchivedAt    *time.Time    `json:"archivedAt,omitempty"`
	// Workspace registry fields (v15) absorbed from workspaces_dir.json. A project
	// IS a workspace; these carry the sidebar/terminal/chat metadata that used to
	// live in the json registry. Builtin marks the reserved default workspace;
	// Position drives sidebar order.
	TerminalDir  string `json:"terminalDir,omitempty"`
	ChatChannel  string `json:"chatChannel,omitempty"`
	DefaultAgent string `json:"defaultAgent,omitempty"`
	// DefaultProfileID is preferred over DefaultAgent for new task dispatch.
	// DefaultAgent remains readable for one compatibility cycle.
	DefaultProfileID string `json:"defaultProfileId,omitempty"`
	Builtin          bool   `json:"builtin,omitempty"`
	Position         int    `json:"position,omitempty"`
	// AvailableAgents is the allowlist of agent types that may run in this
	// workspace (e.g. ["claudecode", "codex"]). Empty means unrestricted.
	AvailableAgents []string `json:"availableAgents,omitempty"`
	// Kind splits workspaces into families:
	//   KindWorkforce ("workforce") — UI「助理」; light chat unit, optional PM shell
	//   KindProject   ("project")   — full project with kanban / milestones (default)
	//   KindTmp       ("tmp")       — UI「单次/临时对话」; real path, path may be hidden
	//   KindApp       ("app")       — app-owned disposable seats (e.g. 圆桌); hidden from
	//                                 sidebar 任务区 (workforce∪tmp) and 项目列表
	// Empty on legacy rows is treated as KindProject. Historical kind=assistant is
	// migrated to workforce on Open.
	Kind string `json:"kind,omitempty"`
	// Avatar is an optional image URL ("/avatars/presets/x.png" or an upload
	// under "/avatars/"). Rendered on the assistant card and sidebar folder icon.
	Avatar    string    `json:"avatar,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Workspace kind values persisted on projects.kind (Epic #184 §0.4 / #189 + tmp + app).
const (
	KindWorkforce = "workforce" // UI「助理」
	KindProject   = "project"   // UI「项目」
	KindTmp       = "tmp"       // UI「单次/临时对话」— real WorkspaceId + pwd; path may be hidden in UI
	KindApp       = "app"       // App-owned seats (圆桌等); not listed in sidebar 任务/项目
)

// NormalizeProjectKind maps empty/legacy kind strings to persisted values.
// kind=assistant becomes workforce. Unknown non-empty values default to project.
func NormalizeProjectKind(kind string) string {
	switch kind {
	case "", KindProject:
		return KindProject
	case KindWorkforce, "assistant":
		return KindWorkforce
	case KindTmp:
		return KindTmp
	case KindApp:
		return KindApp
	default:
		return KindProject
	}
}

// IsTmpKind reports whether kind is a temporary-dialogue workspace.
func IsTmpKind(kind string) bool {
	return NormalizeProjectKind(kind) == KindTmp
}

// IsAppKind reports whether kind is an app-owned disposable workspace (e.g. roundtable seats).
func IsAppKind(kind string) bool {
	return NormalizeProjectKind(kind) == KindApp
}

// ChatSessionRecord is the 1agents-side index of a chat session.
//
// A chat session is a tuple (cc-connect session, 1agents uuid). The actual
// conversation lives in cc-connect; this record is just metadata that the
// sidebar uses to list "my chat sessions" alongside terminal sessions.
//
// Fields map 1:1 to the JSON shape returned by /api/agent/sessions:
//
//	{id, workspace_id, name, agent_type, cc_project, cc_session_id, session_key, created_at, last_event_at}
//
// OneshotWorkspaceID is the frontend picker sentinel for「单次对话」only.
// Creating an ephemeral session mints a real projects row (kind=tmp) with a
// disposable path under /tmp/1agents-chat/; session.workspace_id points at that
// row. Legacy sessions may still use workspace_id=oneshot with session.cwd.
const OneshotWorkspaceID = "oneshot"

// IsOneshotWorkspaceID is true for the picker sentinel or tmp-workspace ids
// (prefix "tmp-"). Prefer checking projects.kind == tmp when a project row exists.
func IsOneshotWorkspaceID(id string) bool {
	return id == OneshotWorkspaceID || strings.HasPrefix(id, "tmp-") || strings.HasPrefix(id, "oneshot-")
}

type ChatSessionRecord struct {
	ID              string `json:"id"`
	WorkspaceID     string `json:"workspace_id"`
	Name            string `json:"name"`
	AgentType       string `json:"agent_type"`
	ProfileID       string `json:"profile_id,omitempty"`
	ProfileRevision int    `json:"profile_revision,omitempty"`
	// TaskID is the optional soft link to a task. Sessions spawned from a
	// task carry it; sidebar renders a task badge when set. Empty for
	// standalone sessions (no enforcement — issue-model decision 3).
	TaskID      string `json:"task_id,omitempty"`
	CcProject   string `json:"cc_project"`
	CcSessionID string `json:"cc_session_id"`
	// AcpSessionID is the agent-managed session id (e.g. Claude Code's
	// JSONL filename) — set on first session_ready from the bridge-server
	// and reused as resumeSessionId on subsequent opens. Independent of
	// CcSessionID, which only identifies the cc-connect / IM side.
	AcpSessionID string `json:"acp_session_id,omitempty"`
	SessionKey   string `json:"session_key"`
	// Cwd is the absolute agent working directory when it differs from the
	// workspace project path. Used by oneshot (单次对话) sessions that live
	// under /tmp/1agents-chat/<random>. Empty = resolve path from WorkspaceID.
	Cwd         string    `json:"cwd,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	LastEventAt time.Time `json:"last_event_at,omitempty"`
	// PermissionMode is the per-session permission policy forwarded to the
	// bridge-server (which gates handlePermissionRequestCallback). One of
	// "approve-reads" (default; auto-allow reads, prompt otherwise),
	// "approve-all", "deny-all", "auto" (decide per request by tool
	// source/risk; see internal/agent/permission). Empty value means "use the
	// bridge-server's global default".
	PermissionMode string `json:"permission_mode,omitempty"`
	// Role marks a special-purpose session. Empty for an ordinary chat. "pm"
	// is the in-app AI Project Manager: HandleChatWs injects a PM system
	// prompt plus a project-locked task-tool MCP server for these sessions.
	Role string `json:"role,omitempty"`
	// ArchivedAt is the soft-delete timestamp. Zero = active; non-zero means
	// the session was archived (closed from the sidebar). Archived sessions
	// drop out of the sidebar list but stay in the 会话 archive view.
	ArchivedAt time.Time `json:"archived_at,omitempty"`
	// UserNamed marks whether the session's name was set by the user (via
	// UpdateName) rather than by the AI title auto-resolution. When true, the
	// list/get endpoint must NOT overwrite the name with the AI title, so a
	// user rename such as "我的项目会话" (which happens to match the default
	// "会话"-suffix pattern) survives subsequent list calls.
	UserNamed bool `json:"user_named"`
}

type ScheduleType string

const (
	ScheduleTypeImmediate ScheduleType = "immediate"
	ScheduleTypeScheduled ScheduleType = "scheduled"
)

// ItemType is the GitHub-style issue discriminator. Requirement/bug/discussion
// cards live in the same project_items table as normal tasks; the "需求池"/"讨论"
// views filter by type. A discussion is a free-form conceptual record (no
// deliverable) that never participates in scheduling — see the scheduler's
// ready loop. Wire JSON values stay task|requirement|bug|discussion.
type ItemType string

const (
	ItemTypeTask        ItemType = "task"
	ItemTypeRequirement ItemType = "requirement"
	ItemTypeBug         ItemType = "bug"
	ItemTypeDiscussion  ItemType = "discussion"
)

// TaskType is a deprecated alias for ItemType (Epic #184 / #197). Prefer ItemType.
// Removal target: delete when M6 (#196) closes and remaining TaskType call sites
// are gone (tracked by #197). Wire values unchanged.
//
// Deprecated: use ItemType.
type TaskType = ItemType

const (
	TaskTypeTask        = ItemTypeTask
	TaskTypeRequirement = ItemTypeRequirement
	TaskTypeBug         = ItemTypeBug
	TaskTypeDiscussion  = ItemTypeDiscussion
)

// TaskExecutor is the executor role for a task (schema v20, #318).
type TaskExecutor string

const (
	// TaskExecutorAgent is the default: an AI agent runs the task via ACP.
	TaskExecutorAgent TaskExecutor = "agent"
	// TaskExecutorFunction routes to the in-process function registry (token≈0).
	TaskExecutorFunction TaskExecutor = "function"
	// TaskExecutorHuman holds the task in a decision queue until user action.
	TaskExecutorHuman TaskExecutor = "human"
)

// TaskTargetSpec is the dispatch spec embedded in Task.TaskTarget (JSON). It
// lets a task override the project defaults for which agent to run, in which
// directory, and with which MCP capabilities.
type TaskTargetSpec struct {
	// AgentType overrides Task.Assignee for dispatch (e.g. "claudecode").
	AgentType string `json:"agent,omitempty"`
	// ProfileID selects a concrete Runtime + Provider + Model profile.
	ProfileID string `json:"profile_id,omitempty"`
	// Cwd is the absolute working directory the agent should cd into. Defaults
	// to the project's WorkspacePath when empty.
	Cwd string `json:"cwd,omitempty"`
	// Capabilities is the list of MCP server names to inject for this task.
	Capabilities []string `json:"capabilities,omitempty"`
}

// TaskSource marks how a task entered the pool. The empty default means a
// normal task (user- or PM-created). "agent-suggested" marks an AI suggestion
// (issue #47, the spawn_task model): a self-contained work item an executing
// agent bubbled up. Suggestions are held out of the board/scheduler until a
// human 采纳 (adopt → clears Source so it becomes a normal task) or 忽略
// (dismiss → deletes it). Source is orthogonal to Type: a suggestion keeps its
// intended type (task/requirement/bug), so adoption is just clearing the flag.
type TaskSource string

const (
	TaskSourceAgent TaskSource = "agent-suggested"
)

// AssigneeUser is a sentinel Assignee value meaning the executor is the user
// themselves — a personal reminder / todo / DDL record (issue #192). Unlike a
// normal assignee (an executing agent type), the scheduler never runs these;
// they live on the calendar until the user marks them done.
const AssigneeUser = "user"

// LinkRel is the relation kind of a TaskLink. "closes" auto-closes the target
// when the source task completes (GitHub-style "fixes #N"); "relates" is a
// plain cross-reference kept for indexing/navigation only (never automatic).
type LinkRel string

const (
	LinkCloses  LinkRel = "closes"
	LinkRelates LinkRel = "relates"
)

// TaskLink is a peer cross-reference from one task to another. This is NOT
// hierarchy — subtasks use ParentID; links connect requirements/tasks/bugs as
// equals. Target holds the referenced task's hex id.
type TaskLink struct {
	Target string  `json:"target"`
	Rel    LinkRel `json:"rel"`
}

type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusQueued    TaskStatus = "queued"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
	TaskStatusBlocked   TaskStatus = "blocked"
	// TaskStatusNotReady marks an executable task that lacks acceptance criteria
	// (issue #135): a task with no "怎样算完成" cannot be loop-verified by the
	// agent, so the scheduler holds it out of the runnable queue and surfaces it
	// as 未就绪 until criteria are filled in. Like TaskStatusBlocked it is a
	// derived state the scheduler toggles, not a user-set one.
	TaskStatusNotReady TaskStatus = "not_ready"
	// TaskStatusPendingReview marks a task whose executor finished but which is
	// configured for verification (Task.Verifier set): the scheduler picks it up
	// and runs the verifier headlessly instead of completing it. See #50.
	TaskStatusPendingReview TaskStatus = "pending_review"
	// TaskStatusAwaitingHuman marks a task with executor=human that is waiting for
	// user action (click-done, dialog backfill, or MCP complete_human_project_item). Once
	// the user completes it, the scheduler's ready gate releases downstream tasks
	// that depend on this one — identical to completing any other task.
	TaskStatusAwaitingHuman TaskStatus = "awaiting_human"
)

// CriterionResult is the verifier's per-acceptance-criterion judgement.
type CriterionResult struct {
	Criterion string `json:"criterion"`
	Pass      bool   `json:"pass"`
	Comment   string `json:"comment,omitempty"`
}

// ReviewVerdict is the latest verification result a verifier submitted (via the
// submit_review MCP tool) for a task. Overall Pass is server-computed as "every
// criterion passed" — the verifier reports per-criterion results, the server
// decides done/loop/fail. Attempt mirrors Task.ReviewCount for display. See #50.
type ReviewVerdict struct {
	Pass bool `json:"pass"`
	// NeedsHuman is the verifier's explicit escalation route (#50 借鉴路): the
	// artifact can't be judged pass/fail by retrying the executor — it needs a
	// human design/architecture/tradeoff decision. Distinct from a rejection:
	// it drives the task to awaiting_human instead of consuming review budget.
	// Mutually exclusive with Pass (an escalating verdict never counts as pass).
	NeedsHuman bool              `json:"needsHuman,omitempty"`
	Criteria   []CriterionResult `json:"criteria,omitempty"`
	Summary    string            `json:"summary,omitempty"`
	Attempt    int               `json:"attempt"`
	Verifier   string            `json:"verifier,omitempty"` // agent type that judged
	CreatedAt  time.Time         `json:"createdAt"`
}

// CompletionEvidence is a privacy-safe proof reference retained on a TaskRun.
// It stores the fact and compact summary, never raw logs, environment values,
// credentials, or full command output.
type CompletionEvidence struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Summary   string    `json:"summary"`
	SessionID string    `json:"sessionId,omitempty"`
	TurnID    string    `json:"turnId,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// ClosedBy is the durable completion audit pointer exposed on a completed Task.
// The referenced TaskRun owns Evidence and the optional verifier Verdict.
type ClosedBy struct {
	Kind        string    `json:"kind"`
	TaskRunID   string    `json:"taskRunId"`
	TurnID      string    `json:"turnId,omitempty"`
	SessionID   string    `json:"sessionId,omitempty"`
	EvidenceIDs []string  `json:"evidenceIds"`
	Verdict     string    `json:"verdict"`
	ClosedAt    time.Time `json:"closedAt"`
}

// Priority drives scheduler ordering when several tasks are ready at once
// (Linear/Jira style; urgent runs first).
type Priority string

const (
	PriorityUrgent Priority = "urgent"
	PriorityHigh   Priority = "high"
	PriorityMedium Priority = "medium"
	PriorityLow    Priority = "low"
)

// PriorityRank maps a priority to its scheduling order (lower runs first).
// Unknown/empty values sort with medium.
func PriorityRank(p Priority) int {
	switch p {
	case PriorityUrgent:
		return 0
	case PriorityHigh:
		return 1
	case PriorityLow:
		return 3
	default:
		return 2
	}
}

// Recurrence is the simple-enum repeat rule (confirmed decision: no cron).
// Freq selects which extra field applies: weekly→Weekday (0=Sunday…6),
// monthly→Monthday (1–31, clamped to month length). At is "HH:MM" local.
//
// interval is the exception to the clock-time rule: it repeats every
// EveryMinutes after the previous run, ignoring At/Weekday/Monthday. It exists
// for machine-driven cadences — data-source incremental sync fires "every N
// minutes", not "daily at HH:MM" — so the work-order scheduler can own periodic
// ingestion instead of each syncer growing its own ticker.
// The extra fields (Interval/DaysOfWeek/WeekIndex/Until/Count) exist so the rule
// can losslessly hold what external sources express — MS Graph patterns and
// RFC5545 RRULE — e.g. "every Sat & Sun", "first Monday of the month", "10
// times", "until 2026-12-31". They are all optional; a rule that uses only the
// legacy Weekday/Monthday keeps working unchanged.
type Recurrence struct {
	Freq         string `json:"freq"`                 // daily | weekly | monthly | yearly | interval
	Interval     int    `json:"interval,omitempty"`   // every N periods (default 1)
	Weekday      int    `json:"weekday,omitempty"`    // weekly single-day (legacy)
	DaysOfWeek   []int  `json:"daysOfWeek,omitempty"` // weekly multi-day, 0=Sunday…6
	Monthday     int    `json:"monthday,omitempty"`   // monthly/yearly absolute day (1–31)
	WeekIndex    int    `json:"weekIndex,omitempty"`  // monthly/yearly relative: 1..4 / -1=last, with DaysOfWeek
	Month        int    `json:"month,omitempty"`      // yearly only: 1–12
	At           string `json:"at,omitempty"`
	EveryMinutes int    `json:"everyMinutes,omitempty"` // interval only: minutes between runs
	Until        string `json:"until,omitempty"`        // RFC3339/date: stop after this
	Count        int    `json:"count,omitempty"`        // stop after this many occurrences
}

// ChecklistItem is one entry of a task's embedded checklist — an ordered,
// individually-checkable sub-item held inside the task itself (distinct from a
// parent/child subtask). It gives an executing agent a progress ledger to tick
// off as it works, and a verifier a per-item list to confirm against. Modeled
// on Microsoft To Do's checklistItems; also the promotion target for external
// todos that carry one.
type ChecklistItem struct {
	Text string `json:"text"`
	Done bool   `json:"done,omitempty"`
}

// IssueState is the open/closed dimension layered on top of the workflow
// status (issue-model decision 1: dual status).
type IssueState string

const (
	IssueOpen   IssueState = "open"
	IssueClosed IssueState = "closed"
)

type SessionKind string

const (
	SessionKindChat SessionKind = "chat"
)

type SessionStatus string

const (
	SessionStatusIdle    SessionStatus = "idle"
	SessionStatusRunning SessionStatus = "running"
)

type SessionMetadata struct {
	ID        string        `json:"id"`
	Kind      SessionKind   `json:"kind"`
	Name      string        `json:"name"`
	AgentType string        `json:"agentType"`
	Status    SessionStatus `json:"status"`
	Summary   string        `json:"summary,omitempty"`
	// ReplyIDs is the reverse index of timeline replies that reference this
	// session (computed from replies.session_ref on load, not stored).
	ReplyIDs  []string  `json:"replyIds,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// Author identifies who wrote a timeline reply.
type Author struct {
	Kind string `json:"kind"` // "user" | "agent"
	Name string `json:"name"` // "scott" | "claude-opus-4-8" | ...
}

type ReplyMode string

const (
	ModeNewSession  ReplyMode = "new"          // reply opened a new session
	ModeFollowUp    ReplyMode = "follow_up"    // reply follows up an existing session
	ModePureComment ReplyMode = "pure_comment" // plain comment, no session action
)

// Reply is one entry on a task's issue timeline (issue-model §6).
type Reply struct {
	ID           string    `json:"id"`
	Author       Author    `json:"author"`
	AgentType    string    `json:"agentType,omitempty"`
	Text         string    `json:"text"`
	SessionRef   string    `json:"sessionRef,omitempty"`   // SessionMetadata.ID
	TurnID       string    `json:"turnId,omitempty"`       // AgentTurn.ID; empty for legacy replies
	AcpSessionID string    `json:"acpSessionId,omitempty"` // raw agent UUID
	InReplyTo    string    `json:"inReplyTo,omitempty"`    // target reply.id for follow-ups
	Mode         ReplyMode `json:"mode"`
	CreatedAt    time.Time `json:"createdAt"`
}

// ProjectItem is the primary board-row type, persisted in the project_items table
// (Epic #184 / #197 M6-1). This is the struct definition site; Task is only a
// transitional alias. Wire JSON field names stay stable.
// Prefer ProjectItem / ItemType in new code.
type ProjectItem struct {
	ID           string       `json:"id"`
	Title        string       `json:"title"`
	Description  string       `json:"description"` // issue-model: Markdown body; ALSO the agent's work instruction
	IssueState   IssueState   `json:"issueState"`  // issue-model: open | closed
	Status       TaskStatus   `json:"status"`
	ScheduleType ScheduleType `json:"scheduleType"`
	ScheduledAt  *time.Time   `json:"scheduledAt"`
	// PlannedStart / PlannedEnd are the PM scheduling fields shown in the
	// table view (project-model §2.2). PlannedStart doubles as the
	// automation trigger time when ScheduledAt is unset.
	PlannedStart *time.Time `json:"plannedStart,omitempty"`
	PlannedEnd   *time.Time `json:"plannedEnd,omitempty"`
	DependsOn    []string   `json:"dependsOn"`

	// ── PM fields (schema v2) ──
	Priority  Priority `json:"priority,omitempty"` // urgent|high|medium|low
	Assignee  string   `json:"assignee,omitempty"` // executing agent type; empty = claudecode
	Labels    []string `json:"labels,omitempty"`
	CreatedBy string   `json:"createdBy,omitempty"` // user | agent | scheduler
	ParentID  string   `json:"parentId,omitempty"`  // one-level hierarchy; subtasks gate the parent
	Milestone string   `json:"milestone,omitempty"`
	// ── PM fields (schema v3) ──
	// Sprint is a free-text label (e.g. "Sprint 23", "2026-Q2-S1") used to
	// group tasks into iterations; empty for un-sprinted tasks and for any
	// v2 row that pre-dates the column.
	Sprint string `json:"sprint,omitempty"`
	// ── PM fields (schema v4) ──
	// Type is the ItemType discriminator (task | requirement | bug | discussion).
	// Empty/"" is treated as ItemTypeTask for any pre-v4 row. Only ItemTypeTask
	// is scheduler-runnable; other types are board issues.
	Type ItemType `json:"type,omitempty"`
	// ── relations (schema v5) ──
	// Number is the per-project short id (#N), assigned on first save and
	// stable thereafter; 0 means un-numbered (pre-v5 rows are backfilled).
	Number int `json:"number,omitempty"`
	// Links are GitHub-style peer cross-references to other tasks. They drive
	// indexing/navigation; "closes" links also auto-close their target.
	Links []TaskLink `json:"links,omitempty"`
	// ── task kernel fields (schema v20, #318) ──
	// Executor is the role that carries out this task: "agent" (default, AI),
	// "function" (deterministic in-process handler), or "human" (decision gate).
	// Empty/missing rows behave identically to "agent" for full back-compat.
	Executor TaskExecutor `json:"executor,omitempty"`
	// BusinessRef is the opaque binding to an application domain object, e.g.
	// "crm:lead:42" or "media:clip:7". Nullable. Used by IssueTasksFromBusiness /
	// ListTasksForBusiness (binding seam, #321).
	BusinessRef string `json:"businessRef,omitempty"`
	// TaskTarget holds the dispatch spec for agent tasks: which agent type to
	// start, what cwd to run in, and which MCP capabilities to inject. Stored as
	// JSON; nil means "use project defaults" (workspacePath + Assignee).
	TaskTarget *TaskTargetSpec `json:"target,omitempty"`
	// Result is the task's terminal output, written by the runner or function
	// executor. Stored as JSON so callers can write domain-specific payloads.
	Result string `json:"result,omitempty"`
	// CostTokens is the total token expenditure for this task across all
	// execution + verification cycles. 0 for function/human tasks (near-zero
	// cost). Set by the runner on finish.
	CostTokens int64 `json:"costTokens,omitempty"`

	// ── suggestion source (schema v9) ──
	// Source marks an AI-suggested task (issue #47). Empty = normal task;
	// "agent-suggested" cards stay out of the board/scheduler until adopted.
	Source TaskSource `json:"source,omitempty"`
	// ── requirement/bug confirmation (schema v10) ──
	// UserConfirm marks a requirement/bug the user has confirmed as ready for
	// the PM to schedule (break down into executable tasks). Requirements and
	// bugs are non-executable issue items (open/closed only); the scheduler
	// never runs them directly, and the PM may only plan the confirmed ones.
	UserConfirm bool `json:"userConfirm,omitempty"`

	// ── GitHub Issue/PR sync mapping (schema v12, #74) ──
	// These store the anchor for the one GitHub object a task may be bound to.
	// This issue ships field/contract alignment only — there is no sync engine
	// yet, so the fields are populated by a future sync pass and surfaced
	// read-only. All default to empty/zero for pre-v12 rows.
	//
	// GithubRepo is "owner/repo" of the bound object's repository.
	GithubRepo string `json:"githubRepo,omitempty"`
	// GithubKind is "issue" or "pr" — which GitHub object this task maps to.
	GithubKind string `json:"githubKind,omitempty"`
	// GithubNumber is the remote #N (per-repo), distinct from the local Number.
	GithubNumber int `json:"githubNumber,omitempty"`
	// GithubNodeId is the GraphQL global node id, required by the Projects v2 API.
	GithubNodeId string `json:"githubNodeId,omitempty"`
	// GithubUrl is the object's html_url.
	GithubUrl string `json:"githubUrl,omitempty"`
	// GithubState is the remote open/closed state, kept for conflict detection
	// against the local IssueState.
	GithubState string `json:"githubState,omitempty"`
	// GithubAssignees are GitHub login names (issue.assignees[].login). This is
	// the human-collaborator dimension and is deliberately separate from
	// Assignee, which selects the executing agent type (claudecode/codex/…).
	GithubAssignees []string `json:"githubAssignees,omitempty"`
	// LastSyncedAt is the timestamp of the last successful sync with GitHub;
	// nil means never synced.
	LastSyncedAt *time.Time `json:"lastSyncedAt,omitempty"`

	// ── automation fields (schema v2) ──
	AcceptanceCriteria string      `json:"acceptanceCriteria,omitempty"` // injected; agent self-checks before completing
	Recurrence         *Recurrence `json:"recurrence,omitempty"`         // nil = one-shot
	// Checklist is the task's embedded ordered progress ledger (see
	// ChecklistItem). Empty for tasks without one. Serialized as a JSON column.
	Checklist      []ChecklistItem `json:"checklist,omitempty"`
	MaxRetries     int             `json:"maxRetries"` // auto-retry budget on failure (default 1)
	RetryCount     int             `json:"retryCount,omitempty"`
	TimeoutMinutes int             `json:"timeoutMinutes,omitempty"` // 0 = runner default idle timeout

	// ── verification fields (schema v9, #50) ──
	// Verifier is the agent type that reviews this task after the executor
	// finishes; empty = no verification (executor completion is final). When set
	// (and AcceptanceCriteria is non-empty), executor completion routes the task
	// to pending_review and the scheduler runs a headless verifier pass.
	Verifier string `json:"verifier,omitempty"`
	// ReviewMaxAttempts caps how many verification cycles a task may consume
	// before a rejected verdict becomes a terminal failure (报异常). 0 = the
	// effective default (defaultReviewMaxAttempts). Independent of MaxRetries,
	// which governs execution crashes.
	ReviewMaxAttempts int `json:"reviewMaxAttempts,omitempty"`
	// ReviewCount counts verification cycles that ended in rejection.
	ReviewCount int `json:"reviewCount,omitempty"`
	// Review holds the latest verdict (per-criterion results + overall pass).
	Review *ReviewVerdict `json:"review,omitempty"`
	// ClosedBy points to the TaskRun + compact evidence that actually crossed
	// the completion gate. An assistant final answer alone never populates it.
	ClosedBy *ClosedBy `json:"closedBy,omitempty"`

	// ── adversarial multi-verifier fields (schema v14, #131) ──
	// VerifierCount is how many independent verifiers judge each verification
	// cycle. >1 turns a single review into an adversarial panel: N verifiers each
	// submit their own verdict, and the panel decision is by threshold. 0/1 = the
	// classic single-verifier flow.
	VerifierCount int `json:"verifierCount,omitempty"`
	// VerifyPassThreshold is how many of the VerifierCount verdicts must pass for
	// the panel to accept the artifact. 0 = majority (⌊N/2⌋+1).
	VerifyPassThreshold int `json:"verifyPassThreshold,omitempty"`
	// ReviewPool accumulates the running cycle's per-verifier verdicts until the
	// panel is complete (len == VerifierCount), at which point it is aggregated
	// into Review and cleared. Empty between cycles.
	ReviewPool []ReviewVerdict `json:"reviewPool,omitempty"`

	CreatedAt     time.Time         `json:"createdAt"`
	UpdatedAt     time.Time         `json:"updatedAt"`
	StartedAt     *time.Time        `json:"startedAt,omitempty"`
	CompletedAt   *time.Time        `json:"completedAt,omitempty"`
	Summary       string            `json:"summary,omitempty"`
	Replies       []Reply           `json:"replies"`  // issue-model: chronological timeline
	Sessions      []SessionMetadata `json:"sessions"` // execution index (aggregated from sessions.task_id)
	WorkspacePath string            `json:"-"`
}

// Task is a deprecated alias for ProjectItem (Epic #197 / M6-1). Prefer ProjectItem.
// Removal target: delete when M6 (#196) closes and remaining Task call sites
// are gone (tracked by #197). Wire JSON is unchanged (same underlying type).
//
// Deprecated: use ProjectItem.
type Task = ProjectItem

// TasksConfig is the load/save aggregate for a workspace board (JSON key "tasks"
// kept for wire compatibility). Prefer iterating ProjectItem values.
type TasksConfig struct {
	Tasks []ProjectItem `json:"tasks"`
}

// FeatureNodeKind identifies the two node kinds in a project's feature
// catalog. Modules form the hierarchy; feature points are leaves.
type FeatureNodeKind string

const (
	FeatureNodeModule FeatureNodeKind = "module"
	FeatureNodePoint  FeatureNodeKind = "feature"
)

// FeatureNode is one persisted module or feature point in a project's feature
// catalog. Level, path, progress, and aggregated milestone data are derived at
// read time and therefore are intentionally absent from this storage contract.
type FeatureNode struct {
	ID                string                   `json:"id"`
	ProjectID         string                   `json:"-"`
	ParentID          string                   `json:"parentId,omitempty"`
	Kind              FeatureNodeKind          `json:"kind"`
	Title             string                   `json:"title"`
	Description       string                   `json:"description,omitempty"`
	Documents         []string                 `json:"documents,omitempty"`
	TargetMilestoneID string                   `json:"targetMilestoneId,omitempty"`
	Position          int                      `json:"position"`
	CreatedAt         time.Time                `json:"createdAt"`
	UpdatedAt         time.Time                `json:"updatedAt"`
	Progress          *FeatureProgress         `json:"progress,omitempty"`
	VersionCoverage   []FeatureVersionCoverage `json:"versionCoverage,omitempty"`
}

// FeatureVersionCoverage is the derived target-version distribution for the
// feature points below a module. It is never persisted.
type FeatureVersionCoverage struct {
	MilestoneID  string `json:"milestoneId"`
	Version      string `json:"version"`
	FeatureCount int    `json:"featureCount"`
}

// FeatureProgressStatus is derived from a feature's non-cancelled delivery
// tasks. It is never persisted, so task status remains the single source of
// truth.
type FeatureProgressStatus string

const (
	FeatureProgressUnplanned  FeatureProgressStatus = "unplanned"
	FeatureProgressPending    FeatureProgressStatus = "pending"
	FeatureProgressInProgress FeatureProgressStatus = "in_progress"
	FeatureProgressDelivered  FeatureProgressStatus = "delivered"
	FeatureProgressReplan     FeatureProgressStatus = "replan"
)

// FeatureProgress is the derived delivery/coverage read model for a feature
// point or a module subtree. ProgressPercent is nil when there is no valid
// delivery-task denominator, which keeps "未拆解"/"需要重新规划" distinct from
// a real zero-percent plan.
type FeatureProgress struct {
	Status            FeatureProgressStatus `json:"status"`
	ProgressPercent   *int                  `json:"progressPercent"`
	CompletedTasks    int                   `json:"completedTasks"`
	TotalTasks        int                   `json:"totalTasks"`
	CoveredFeatures   int                   `json:"coveredFeatures"`
	TotalFeatures     int                   `json:"totalFeatures"`
	UnplannedFeatures int                   `json:"unplannedFeatures"`
	ReplanFeatures    int                   `json:"replanFeatures"`
}

// FeatureItemRelation describes why a project item is linked to a feature
// point: source requirements/bugs explain why it exists, while delivery tasks
// implement it.
type FeatureItemRelation string

const (
	FeatureItemSource   FeatureItemRelation = "source"
	FeatureItemDelivery FeatureItemRelation = "delivery"
)

// FeatureItemLink is the many-to-many traceability edge between a feature
// point and a project item.
type FeatureItemLink struct {
	FeatureID string              `json:"featureId"`
	ItemID    string              `json:"itemId"`
	Relation  FeatureItemRelation `json:"relation"`
	CreatedAt time.Time           `json:"createdAt"`
}

// FeatureCatalog is the project-scoped read model returned by the feature
// catalog API. Nodes and traceability links remain normalized in storage.
type FeatureCatalog struct {
	Nodes []FeatureNode     `json:"nodes"`
	Links []FeatureItemLink `json:"links"`
}

type FeatureCatalogVersionKind string

const (
	FeatureCatalogVersionManual     FeatureCatalogVersionKind = "manual"
	FeatureCatalogVersionPreRestore FeatureCatalogVersionKind = "pre_restore"
)

// FeatureCatalogVersion is metadata only. SnapshotJSON is deliberately not
// exposed by list or mutation APIs.
type FeatureCatalogVersion struct {
	ID            string                    `json:"id"`
	ProjectID     string                    `json:"-"`
	Alias         string                    `json:"alias"`
	Kind          FeatureCatalogVersionKind `json:"kind"`
	SchemaVersion int                       `json:"schemaVersion"`
	NodeCount     int                       `json:"nodeCount"`
	LinkCount     int                       `json:"linkCount"`
	CreatedAt     time.Time                 `json:"createdAt"`
	UpdatedAt     time.Time                 `json:"updatedAt"`
}

type FeatureCatalogVersionPage struct {
	Items      []FeatureCatalogVersion `json:"items"`
	NextCursor string                  `json:"nextCursor,omitempty"`
	HasMore    bool                    `json:"hasMore"`
}

type FeatureCatalogRestoreWarning struct {
	FeatureID   string `json:"featureId"`
	ReferenceID string `json:"referenceId"`
	Kind        string `json:"kind"`
	Action      string `json:"action"`
}

type FeatureCatalogRestoreResult struct {
	RequestID                   string                         `json:"requestId"`
	TargetVersion               FeatureCatalogVersion          `json:"targetVersion"`
	SafetyVersion               FeatureCatalogVersion          `json:"safetyVersion"`
	RestoredNodeCount           int                            `json:"restoredNodeCount"`
	RestoredLinkCount           int                            `json:"restoredLinkCount"`
	SkippedLinkCount            int                            `json:"skippedLinkCount"`
	ClearedTargetMilestoneCount int                            `json:"clearedTargetMilestoneCount"`
	Warnings                    []FeatureCatalogRestoreWarning `json:"warnings"`
	WarningsTruncated           bool                           `json:"warningsTruncated"`
}

// FeatureMilestoneTaskDiff describes one delivery task whose milestone differs
// from its feature point's current target version.
type FeatureMilestoneTaskDiff struct {
	ID               string `json:"id"`
	Number           int    `json:"number,omitempty"`
	Title            string `json:"title"`
	CurrentMilestone string `json:"currentMilestone,omitempty"`
}

// FeatureMilestoneSyncPreview is returned both before and after an explicit
// sync. Tasks always contains the rows that would be (or were) changed.
type FeatureMilestoneSyncPreview struct {
	FeatureID         string                     `json:"featureId"`
	TargetMilestoneID string                     `json:"targetMilestoneId,omitempty"`
	TargetMilestone   string                     `json:"targetMilestone,omitempty"`
	TargetVersion     string                     `json:"targetVersion,omitempty"`
	Tasks             []FeatureMilestoneTaskDiff `json:"tasks"`
}

// Milestone is a first-class roadmap stage (schema v7). Its identity is the
// pair (ProjectID, Name): tasks still link to a milestone through the existing
// Task.Milestone *string* column, so the milestones table only stores the
// extra metadata (target date, ordering, description) that a bare label can't
// carry. Renaming a milestone cascades to its tasks' Milestone field, so the
// name stays a valid join key. The "current/past/future" distinction is NOT
// stored — it is derived from Position + task completion at read time.
type Milestone struct {
	ID        string `json:"id"`
	ProjectID string `json:"-"`
	Name      string `json:"name"`
	// Version is the canonical SemVer for system-generated milestones. It stays
	// empty for historical/free-name milestones so their task join key is never
	// rewritten during migration.
	Version     string     `json:"version,omitempty"`
	IsLegacy    bool       `json:"isLegacy,omitempty"`
	Description string     `json:"description,omitempty"`
	TargetDate  *time.Time `json:"targetDate,omitempty"`
	Position    int        `json:"position"`
	// PredecessorID is the optional parent milestone (前置里程碑). Milestones
	// sharing a predecessor fork into parallel branches on the roadmap; an
	// empty value makes the milestone a root. Empty when the parent is unset.
	PredecessorID string    `json:"predecessorId,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
	// Total / Completed are computed at list time (not persisted): only executable tasks (type=task).
	// Requirements/bugs/discussions and cancelled tasks are excluded.
	Total     int `json:"total"`
	Completed int `json:"completed"`
}

// InboxItem is one piece of captured context in a Workspace Inbox (#202 / #60):
// every row belongs to a recipient Workspace (workspace_id). Deliver is the
// unified write path (function / agent / human / channel). Items are never
// deleted — Status flips to "archived" so the trail of what each item became
// survives. Accept into a requirement pool reuses PMO Dispatch (#61).
type InboxItem struct {
	ID string `json:"id"`
	// WorkspaceID is the recipient Workspace (projects.id). Required on write.
	WorkspaceID string `json:"workspaceId"`
	// Source is the deliverer type: manual / agent / function / im / email /
	// rss / data_source / misc.
	Source string `json:"source"`
	// FromWorkspaceID is the sender Workspace when this is an in-org handoff.
	FromWorkspaceID string `json:"fromWorkspaceId,omitempty"`
	// FromRef is an optional producer id (function name / agent role / source id).
	FromRef string `json:"fromRef,omitempty"`
	// Title / Content / URL hold the raw captured material.
	Title   string `json:"title"`
	Content string `json:"content,omitempty"`
	URL     string `json:"url,omitempty"`
	// Summary / Tags are optional enrichment (left empty by MVP manual capture).
	Summary string   `json:"summary,omitempty"`
	Tags    []string `json:"tags,omitempty"`
	// Payload is optional JSON extension for structured deliverers.
	Payload json.RawMessage `json:"payload,omitempty"`
	// Status is unread / read / archived.
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Inbox item sources and statuses.
const (
	InboxSourceManual     = "manual"
	InboxSourceAgent      = "agent"
	InboxSourceFunction   = "function"
	InboxSourceIM         = "im"
	InboxSourceEmail      = "email"
	InboxSourceRSS        = "rss"
	InboxSourceDataSource = "data_source"
	InboxSourceMisc       = "misc"

	InboxStatusUnread   = "unread"
	InboxStatusRead     = "read"
	InboxStatusArchived = "archived"

	// DefaultInboxWorkspaceID is the builtin default assistant (总助/默认助理)
	// workspace id used to backfill legacy unscoped inbox rows.
	DefaultInboxWorkspaceID = "default"
)
