package agent

import "github.com/scottzx/1Agents/backend/internal/meta"

// AgentType is an agent engine name accepted by the backend. Most types are
// registered in cc-connect; ACP-native web-chat agents may be driven directly
// through modules/1acp instead.
type AgentType = string

const (
	AgentTypeClaudecode    AgentType = "claudecode"
	AgentTypeCodex         AgentType = "codex"
	AgentTypeGrokBuild     AgentType = "grok-build"
	AgentTypeDeepSeekBuild AgentType = "deepseek-build"
	AgentTypeAcp           AgentType = "acp"
	AgentTypeGemini        AgentType = "gemini"
	AgentTypeCursor        AgentType = "cursor"
	AgentTypeDevin         AgentType = "devin"
	AgentTypeIflow         AgentType = "iflow"
	AgentTypeKimi          AgentType = "kimi"
	AgentTypeOpencode      AgentType = "opencode"
	AgentTypePi            AgentType = "pi"
	AgentTypeQoder         AgentType = "qoder"
	AgentTypeTmux          AgentType = "tmux"
)

// SupportedAgentTypes is the canonical list served by /api/agent/agent-types.
// It includes both cc-connect plugins and ACP-native web-chat agents.
var SupportedAgentTypes = []AgentType{
	AgentTypeClaudecode,
	AgentTypeCodex,
	AgentTypeGrokBuild,
	AgentTypeDeepSeekBuild,
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

// IsSupportedAgentType reports whether t is one of SupportedAgentTypes.
func IsSupportedAgentType(t AgentType) bool {
	for _, a := range SupportedAgentTypes {
		if a == t {
			return true
		}
	}
	return false
}

// Model types live in internal/meta (the SQLite metadata layer) so the
// server handlers and the CLI share one definition; the aliases below keep
// this package's existing code and the wire JSON shapes unchanged.
type (
	ChatSessionRecord = meta.ChatSessionRecord
	ScheduleType      = meta.ScheduleType
	TaskStatus        = meta.TaskStatus
	ItemType          = meta.ItemType
	TaskType          = meta.ItemType // transitional alias; prefer ItemType
	TaskSource        = meta.TaskSource
	TaskExecutor      = meta.TaskExecutor
	TaskTargetSpec    = meta.TaskTargetSpec
	IssueState        = meta.IssueState
	SessionKind       = meta.SessionKind
	SessionStatus     = meta.SessionStatus
	SessionMetadata   = meta.SessionMetadata
	Author            = meta.Author
	ReplyMode         = meta.ReplyMode
	Reply             = meta.Reply
	// ProjectItem is the primary board-row type (table project_items; #197).
	// Task is a transitional alias — prefer ProjectItem in new code.
	ProjectItem     = meta.ProjectItem
	Task            = meta.Task
	TasksConfig     = meta.TasksConfig
	Milestone       = meta.Milestone
	Project         = meta.Project
	MilestonePatch  = meta.MilestonePatch
	TaskLink        = meta.TaskLink
	LinkRel         = meta.LinkRel
	LinkGraph       = meta.LinkGraph
	LinkEdge        = meta.LinkEdge
	Priority        = meta.Priority
	Recurrence      = meta.Recurrence
	ChecklistItem   = meta.ChecklistItem
	WorkspaceRef    = meta.WorkspaceRef
	ReviewVerdict   = meta.ReviewVerdict
	CriterionResult = meta.CriterionResult
)

// PriorityRank re-exports the scheduler ordering helper.
var PriorityRank = meta.PriorityRank

// Label/field policy-signal layer (#134): scheduler and future orchestration
// read these instead of re-parsing labels/fields.
var (
	DeriveSignals   = meta.DeriveSignals
	SplitLabels     = meta.SplitLabels
	IsReservedLabel = meta.IsReservedLabel
)

// PolicySignals is the normalized triggers/policy view of a task.
type PolicySignals = meta.PolicySignals

// Frontmatter helpers (card content is YAML-frontmatter Markdown).
var (
	SplitFrontmatter      = meta.SplitFrontmatter
	FrontmatterAcceptance = meta.FrontmatterAcceptance
	RenderCardDoc         = meta.RenderCardDoc
)

const (
	ScheduleTypeImmediate = meta.ScheduleTypeImmediate
	ScheduleTypeScheduled = meta.ScheduleTypeScheduled

	TaskStatusPending       = meta.TaskStatusPending
	TaskStatusQueued        = meta.TaskStatusQueued
	TaskStatusRunning       = meta.TaskStatusRunning
	TaskStatusCompleted     = meta.TaskStatusCompleted
	TaskStatusFailed        = meta.TaskStatusFailed
	TaskStatusCancelled     = meta.TaskStatusCancelled
	TaskStatusBlocked       = meta.TaskStatusBlocked
	TaskStatusNotReady      = meta.TaskStatusNotReady
	TaskStatusPendingReview = meta.TaskStatusPendingReview
	TaskStatusAwaitingHuman = meta.TaskStatusAwaitingHuman

	TaskExecutorAgent    = meta.TaskExecutorAgent
	TaskExecutorFunction = meta.TaskExecutorFunction
	TaskExecutorHuman    = meta.TaskExecutorHuman

	IssueOpen   = meta.IssueOpen
	IssueClosed = meta.IssueClosed

	ItemTypeTask        = meta.ItemTypeTask
	ItemTypeRequirement = meta.ItemTypeRequirement
	ItemTypeBug         = meta.ItemTypeBug
	ItemTypeDiscussion  = meta.ItemTypeDiscussion

	TaskTypeTask        = meta.ItemTypeTask // transitional aliases
	TaskTypeRequirement = meta.ItemTypeRequirement
	TaskTypeBug         = meta.ItemTypeBug
	TaskTypeDiscussion  = meta.ItemTypeDiscussion

	TaskSourceAgent = meta.TaskSourceAgent

	AssigneeUser = meta.AssigneeUser

	LinkCloses  = meta.LinkCloses
	LinkRelates = meta.LinkRelates

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
	ProfileID   string    `json:"profile_id,omitempty"`
	// TaskID is the optional issue-model soft link; set when the session is
	// spawned from a task timeline so the sidebar badge shows immediately.
	TaskID string `json:"task_id"`
	// cc_* / session_key identify the cc-connect (IM) side; empty for
	// ACP-only sessions.
	CcProject   string `json:"cc_project"`
	CcSessionID string `json:"cc_session_id"`
	SessionKey  string `json:"session_key"`
	// Role marks a special-purpose session ("pm" = AI Project Manager).
	// Empty for an ordinary chat. See meta.ChatSessionRecord.Role.
	Role string `json:"role"`
	// Ephemeral marks a "单次对话" (oneshot) session: no real project workspace.
	// The server allocates workspace_id=oneshot and a disposable cwd under
	// /tmp/1agents-chat/<random>. Also accepted when workspace_id is already
	// "oneshot".
	Ephemeral bool `json:"ephemeral,omitempty"`
}
