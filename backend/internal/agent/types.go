package agent

import "time"

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

// ChatSessionRecord is the 1agents-side index of a chat session.
//
// A chat session is a tuple (cc-connect session, 1agents uuid). The actual
// conversation lives in cc-connect; this record is just metadata that the
// sidebar uses to list "my chat sessions" alongside terminal sessions.
//
// Fields map 1:1 to the JSON shape returned by /api/agent/sessions:
//   {id, workspace_id, name, agent_type, cc_project, cc_session_id, session_key, created_at, last_event_at}
type ChatSessionRecord struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	Name        string    `json:"name"`
	AgentType   AgentType `json:"agent_type"`
	CcProject   string    `json:"cc_project"`
	CcSessionID string    `json:"cc_session_id"`
	SessionKey  string    `json:"session_key"`
	CreatedAt   time.Time `json:"created_at"`
	LastEventAt time.Time `json:"last_event_at,omitempty"`
}

// IndexRequest is the body of POST /api/agent/sessions.
//
// The frontend creates the cc-connect session FIRST, then calls this
// endpoint to register the mapping. This keeps 1agents out of the
// cc-connect session lifecycle (no coupling, no race).
type IndexRequest struct {
	WorkspaceID string    `json:"workspace_id" binding:"required"`
	Name        string    `json:"name"`
	AgentType   AgentType `json:"agent_type" binding:"required"`
	CcProject   string    `json:"cc_project" binding:"required"`
	CcSessionID string    `json:"cc_session_id" binding:"required"`
	SessionKey  string    `json:"session_key" binding:"required"`
}

// fileConfig is the top-level structure persisted to disk.
type fileConfig struct {
	Sessions []ChatSessionRecord `json:"sessions"`
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
	CreatedAt time.Time     `json:"createdAt"`
}

type Task struct {
	ID            string            `json:"id"`
	Title         string            `json:"title"`
	Status        TaskStatus        `json:"status"`
	ScheduleType  ScheduleType      `json:"scheduleType"`
	ScheduledAt   *time.Time        `json:"scheduledAt"`
	DependsOn     []string          `json:"dependsOn"`
	CreatedAt     time.Time         `json:"createdAt"`
	UpdatedAt     time.Time         `json:"updatedAt"`
	StartedAt     *time.Time        `json:"startedAt,omitempty"`
	CompletedAt   *time.Time        `json:"completedAt,omitempty"`
	Summary       string            `json:"summary,omitempty"`
	Sessions      []SessionMetadata `json:"sessions"`
	WorkspacePath string            `json:"-"`
}

type TasksConfig struct {
	Tasks []Task `json:"tasks"`
}

