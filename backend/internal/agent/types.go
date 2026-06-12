package agent

import "github.com/scottzx/1Agents/backend/internal/meta"

// AgentType is the agent plugin name registered in cc-connect.
// Matches the import list in backend/internal/ccconnect/runner.go.
type AgentType = string

const (
	AgentTypeClaudecode AgentType = "claudecode"
	AgentTypeCodex      AgentType = "codex"
	AgentTypeAcp        AgentType = "acp"
	AgentTypeGemini     AgentType = "gemini"
	AgentTypeCursor     AgentType = "cursor"
	AgentTypeDevin      AgentType = "devin"
	AgentTypeIflow      AgentType = "iflow"
	AgentTypeKimi       AgentType = "kimi"
	AgentTypeOpencode   AgentType = "opencode"
	AgentTypePi         AgentType = "pi"
	AgentTypeQoder      AgentType = "qoder"
	AgentTypeTmux       AgentType = "tmux"
)

// SupportedAgentTypes is the canonical list served by /api/agent/agent-types.
// Must stay in sync with the blank imports in
// backend/internal/ccconnect/runner.go.
var SupportedAgentTypes = []AgentType{
	AgentTypeClaudecode,
	AgentTypeCodex,
	AgentTypeAcp,
	AgentTypeGemini,
	AgentTypeCursor,
	AgentTypeDevin,
	AgentTypeIflow,
	AgentTypeKimi,
	AgentTypeOpencode,
	AgentTypePi,
	AgentTypeQoder,
	AgentTypeTmux,
}

// DefaultAgentType is the agent used when a workspace has none configured.
const DefaultAgentType = AgentTypeClaudecode

// Model types live in internal/meta (the SQLite metadata layer) so the
// server handlers and the CLI share one definition; the aliases below keep
// this package's existing code and the wire JSON shapes unchanged.
type (
	ChatSessionRecord = meta.ChatSessionRecord
	ScheduleType      = meta.ScheduleType
	TaskStatus        = meta.TaskStatus
	IssueState        = meta.IssueState
	SessionKind       = meta.SessionKind
	SessionStatus     = meta.SessionStatus
	SessionMetadata   = meta.SessionMetadata
	Author            = meta.Author
	ReplyMode         = meta.ReplyMode
	Reply             = meta.Reply
	Task              = meta.Task
	TasksConfig       = meta.TasksConfig
	Priority          = meta.Priority
	Recurrence        = meta.Recurrence
	WorkspaceRef      = meta.WorkspaceRef
)

// PriorityRank re-exports the scheduler ordering helper.
var PriorityRank = meta.PriorityRank

const (
	ScheduleTypeImmediate = meta.ScheduleTypeImmediate
	ScheduleTypeScheduled = meta.ScheduleTypeScheduled

	TaskStatusPending   = meta.TaskStatusPending
	TaskStatusQueued    = meta.TaskStatusQueued
	TaskStatusRunning   = meta.TaskStatusRunning
	TaskStatusCompleted = meta.TaskStatusCompleted
	TaskStatusFailed    = meta.TaskStatusFailed
	TaskStatusCancelled = meta.TaskStatusCancelled
	TaskStatusBlocked   = meta.TaskStatusBlocked

	IssueOpen   = meta.IssueOpen
	IssueClosed = meta.IssueClosed

	PriorityUrgent = meta.PriorityUrgent
	PriorityHigh   = meta.PriorityHigh
	PriorityMedium = meta.PriorityMedium
	PriorityLow    = meta.PriorityLow

	SessionKindChat = meta.SessionKindChat

	SessionStatusIdle    = meta.SessionStatusIdle
	SessionStatusRunning = meta.SessionStatusRunning

	ModeNewSession  = meta.ModeNewSession
	ModeFollowUp    = meta.ModeFollowUp
	ModePureComment = meta.ModePureComment
)

// IndexRequest is the body of POST /api/agent/sessions.
//
// The frontend creates the cc-connect session FIRST, then calls this
// endpoint to register the mapping. This keeps 1agents out of the
// cc-connect session lifecycle (no coupling, no race).
type IndexRequest struct {
	WorkspaceID string    `json:"workspace_id" binding:"required"`
	Name        string    `json:"name"`
	AgentType   AgentType `json:"agent_type" binding:"required"`
	// TaskID is the optional issue-model soft link; set when the session is
	// spawned from a task timeline so the sidebar badge shows immediately.
	TaskID string `json:"task_id"`
	// cc_* / session_key identify the cc-connect (IM) side; empty for
	// ACP-only sessions.
	CcProject   string `json:"cc_project"`
	CcSessionID string `json:"cc_session_id"`
	SessionKey  string `json:"session_key"`
}
