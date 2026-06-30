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

import "time"

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
	TerminalDir  string    `json:"terminalDir,omitempty"`
	ChatChannel  string    `json:"chatChannel,omitempty"`
	DefaultAgent string    `json:"defaultAgent,omitempty"`
	Builtin      bool      `json:"builtin,omitempty"`
	Position     int       `json:"position,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
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
type ChatSessionRecord struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Name        string `json:"name"`
	AgentType   string `json:"agent_type"`
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
	AcpSessionID string    `json:"acp_session_id,omitempty"`
	SessionKey   string    `json:"session_key"`
	CreatedAt    time.Time `json:"created_at"`
	LastEventAt  time.Time `json:"last_event_at,omitempty"`
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
}

type ScheduleType string

const (
	ScheduleTypeImmediate ScheduleType = "immediate"
	ScheduleTypeScheduled ScheduleType = "scheduled"
)

// TaskType is the GitHub-style issue discriminator. Requirement/bug/discussion
// cards live in the same tasks table as normal tasks; the "需求池"/"讨论" views
// filter by type. A discussion is a free-form conceptual record (no deliverable)
// that never participates in scheduling — see the scheduler's ready loop.
type TaskType string

const (
	TaskTypeTask        TaskType = "task"
	TaskTypeRequirement TaskType = "requirement"
	TaskTypeBug         TaskType = "bug"
	TaskTypeDiscussion  TaskType = "discussion"
)

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
	Pass      bool              `json:"pass"`
	Criteria  []CriterionResult `json:"criteria,omitempty"`
	Summary   string            `json:"summary,omitempty"`
	Attempt   int               `json:"attempt"`
	Verifier  string            `json:"verifier,omitempty"` // agent type that judged
	CreatedAt time.Time         `json:"createdAt"`
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
type Recurrence struct {
	Freq     string `json:"freq"` // daily | weekly | monthly
	Weekday  int    `json:"weekday,omitempty"`
	Monthday int    `json:"monthday,omitempty"`
	At       string `json:"at,omitempty"`
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
	AcpSessionID string    `json:"acpSessionId,omitempty"` // raw agent UUID
	InReplyTo    string    `json:"inReplyTo,omitempty"`    // target reply.id for follow-ups
	Mode         ReplyMode `json:"mode"`
	CreatedAt    time.Time `json:"createdAt"`
}

type Task struct {
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
	// Type is the issue discriminator (task | requirement | bug). Empty/"" is
	// treated as "task" for any pre-v4 row.
	Type TaskType `json:"type,omitempty"`
	// ── relations (schema v5) ──
	// Number is the per-project short id (#N), assigned on first save and
	// stable thereafter; 0 means un-numbered (pre-v5 rows are backfilled).
	Number int `json:"number,omitempty"`
	// Links are GitHub-style peer cross-references to other tasks. They drive
	// indexing/navigation; "closes" links also auto-close their target.
	Links []TaskLink `json:"links,omitempty"`
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
	MaxRetries         int         `json:"maxRetries"`                   // auto-retry budget on failure (default 1)
	RetryCount         int         `json:"retryCount,omitempty"`
	TimeoutMinutes     int         `json:"timeoutMinutes,omitempty"` // 0 = runner default idle timeout

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

type TasksConfig struct {
	Tasks []Task `json:"tasks"`
}

// Milestone is a first-class roadmap stage (schema v7). Its identity is the
// pair (ProjectID, Name): tasks still link to a milestone through the existing
// Task.Milestone *string* column, so the milestones table only stores the
// extra metadata (target date, ordering, description) that a bare label can't
// carry. Renaming a milestone cascades to its tasks' Milestone field, so the
// name stays a valid join key. The "current/past/future" distinction is NOT
// stored — it is derived from Position + task completion at read time.
type Milestone struct {
	ID          string     `json:"id"`
	ProjectID   string     `json:"-"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	TargetDate  *time.Time `json:"targetDate,omitempty"`
	Position    int        `json:"position"`
	// PredecessorID is the optional parent milestone (前置里程碑). Milestones
	// sharing a predecessor fork into parallel branches on the roadmap; an
	// empty value makes the milestone a root. Empty when the parent is unset.
	PredecessorID string    `json:"predecessorId,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
	// Total / Completed are computed by joining tasks on Name at list time
	// (not persisted), so the roadmap can render a progress bar per stage.
	Total     int `json:"total"`
	Completed int `json:"completed"`
}

// InboxItem is one piece of captured external context in the Inbox 收口层 (#60):
// the most-upstream layer that aggregates manual captures, IM forwards, emails,
// RSS and misc into a single intake list. Items are never deleted — Status
// flips to "archived" so the trail of what each item became survives. PMO
// dispatch metadata (#61) is intentionally absent here.
type InboxItem struct {
	ID string `json:"id"`
	// Source is the intake channel: manual / im / email / rss / misc.
	Source string `json:"source"`
	// Title / Content / URL hold the raw captured material.
	Title   string `json:"title"`
	Content string `json:"content,omitempty"`
	URL     string `json:"url,omitempty"`
	// Summary / Tags are optional enrichment (left empty by MVP manual capture).
	Summary string   `json:"summary,omitempty"`
	Tags    []string `json:"tags,omitempty"`
	// Status is unread / read / archived.
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Inbox item sources and statuses.
const (
	InboxSourceManual = "manual"
	InboxSourceIM     = "im"
	InboxSourceEmail  = "email"
	InboxSourceRSS    = "rss"
	InboxSourceMisc   = "misc"

	InboxStatusUnread   = "unread"
	InboxStatusRead     = "read"
	InboxStatusArchived = "archived"
)
