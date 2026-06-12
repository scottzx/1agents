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

// Project is one managed workspace directory. Project ID equals the
// workspace ID from the workspace registry, so the two concepts stay 1:1.
type Project struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	WorkspacePath string    `json:"workspacePath"`
	Status        string    `json:"status"` // active | archived
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
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
	// "approve-all", "deny-all". Empty value means "use the bridge-server's
	// global default".
	PermissionMode string `json:"permission_mode,omitempty"`
}

type ScheduleType string

const (
	ScheduleTypeImmediate ScheduleType = "immediate"
	ScheduleTypeScheduled ScheduleType = "scheduled"
)

type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusQueued    TaskStatus = "queued"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
	TaskStatusBlocked   TaskStatus = "blocked"
)

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
	Description  string       `json:"description"`          // issue-model: Markdown body; ALSO the agent's work instruction
	IssueState   IssueState   `json:"issueState"`           // issue-model: open | closed
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
	Priority  Priority `json:"priority,omitempty"`  // urgent|high|medium|low
	Assignee  string   `json:"assignee,omitempty"`  // executing agent type; empty = claudecode
	Labels    []string `json:"labels,omitempty"`
	CreatedBy string   `json:"createdBy,omitempty"` // user | agent | scheduler
	ParentID  string   `json:"parentId,omitempty"`  // one-level hierarchy; subtasks gate the parent
	Milestone string   `json:"milestone,omitempty"`
	// ── PM fields (schema v3) ──
	// Sprint is a free-text label (e.g. "Sprint 23", "2026-Q2-S1") used to
	// group tasks into iterations; empty for un-sprinted tasks and for any
	// v2 row that pre-dates the column.
	Sprint string `json:"sprint,omitempty"`

	// ── automation fields (schema v2) ──
	AcceptanceCriteria string      `json:"acceptanceCriteria,omitempty"` // injected; agent self-checks before completing
	Recurrence         *Recurrence `json:"recurrence,omitempty"`         // nil = one-shot
	MaxRetries         int         `json:"maxRetries"`                   // auto-retry budget on failure (default 1)
	RetryCount         int         `json:"retryCount,omitempty"`
	TimeoutMinutes     int         `json:"timeoutMinutes,omitempty"` // 0 = runner default idle timeout

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
