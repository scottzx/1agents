// Package roundtable implements Agents 圆桌脑暴 (design.md): multi-session
// orchestration with fixed roster seats, state machine, and persistence.
//
// Slice 1: room + seats factory, drafting_brief state, create/get/list APIs.
// Slice 2: R1 referee session chat + Brief confirm → waiting_r2.
// Slice 3: R2 five panelists isolated parallel speech + referee Summary₂ → waiting_r3.
// Slice 4: R3 resume same acp_session_id + public context + Summary₃ → done.
package roundtable

import "time"

// RoomState is the room-level orchestration state (design §5.1).
type RoomState string

const (
	StateDraftingBrief RoomState = "drafting_brief"
	StateWaitingR2     RoomState = "waiting_r2"
	StateSummarizingR2 RoomState = "summarizing_r2"
	StateWaitingR3     RoomState = "waiting_r3"
	StateSummarizingR3 RoomState = "summarizing_r3"
	StateDone          RoomState = "done"
	StateFailed        RoomState = "failed"
)

// Role is a fixed seat role (design §3).
type Role string

const (
	RoleReferee Role = "referee"
	RoleMarket  Role = "market"
	RoleProduct Role = "product"
	RoleEng     Role = "eng"
	RoleOps     Role = "ops"
	RoleFinance Role = "finance"
)

// DefaultRoster is the MVP fixed seating order (referee + five panelists).
var DefaultRoster = []Role{
	RoleReferee,
	RoleMarket,
	RoleProduct,
	RoleEng,
	RoleOps,
	RoleFinance,
}

// PanelistRoles is the fixed five R2/R3 speaking seats (excludes referee).
var PanelistRoles = []Role{
	RoleMarket,
	RoleProduct,
	RoleEng,
	RoleOps,
	RoleFinance,
}

// IsPanelist reports whether role is one of the five function seats.
func IsPanelist(r Role) bool {
	switch r {
	case RoleMarket, RoleProduct, RoleEng, RoleOps, RoleFinance:
		return true
	default:
		return false
	}
}

// RoleSlug is the short suffix used in app workspace ids (app-rt-<room>-<slug>).
func RoleSlug(r Role) string {
	switch r {
	case RoleReferee:
		return "ref"
	case RoleMarket:
		return "mkt"
	case RoleProduct:
		return "prd"
	case RoleEng:
		return "eng"
	case RoleOps:
		return "ops"
	case RoleFinance:
		return "fin"
	default:
		return string(r)
	}
}

// RoleLabel is the Chinese display name for UI.
func RoleLabel(r Role) string {
	switch r {
	case RoleReferee:
		return "裁判"
	case RoleMarket:
		return "市场"
	case RoleProduct:
		return "产品"
	case RoleEng:
		return "研发"
	case RoleOps:
		return "运营"
	case RoleFinance:
		return "财务"
	default:
		return string(r)
	}
}

// SeatStatus is per-seat runtime status (MVP: ready until later slices drive speaking/failed).
type SeatStatus string

const (
	SeatReady    SeatStatus = "ready"
	SeatSpeaking SeatStatus = "speaking"
	SeatDone     SeatStatus = "done"
	SeatFailed   SeatStatus = "failed"
	SeatSkipped  SeatStatus = "skipped"
)

// ProductKind optionally tags the domain of the brief.
type ProductKind string

const (
	ProductSoftware ProductKind = "software"
	ProductHardware ProductKind = "hardware"
	ProductHybrid   ProductKind = "hybrid"
)

// Turn kind values (design §5.3).
const (
	TurnKindChat    = "chat"
	TurnKindSpeech  = "speech"
	TurnKindSummary = "summary"
	TurnKindSystem  = "system"
)

// TurnSeatUser is the seat_id value for user-authored turns (design §5.3: seat_id | "user").
const TurnSeatUser = "user"

// Brief is the confirmed R1 output (design §4 / §5.3). Optional until R1 completes.
type Brief struct {
	Title           string      `json:"title"`
	Question        string      `json:"question"`
	Constraints     string      `json:"constraints"`
	SuccessCriteria string      `json:"success_criteria"`
	ProductKind     ProductKind `json:"product_kind,omitempty"`
}

// BriefStatus is the lifecycle of an immutable BriefVersion.
type BriefStatus string

const (
	BriefStatusDraft      BriefStatus = "draft"
	BriefStatusProposed   BriefStatus = "proposed"
	BriefStatusConfirmed  BriefStatus = "confirmed"
	BriefStatusSuperseded BriefStatus = "superseded"
)

// BriefProposer identifies who authored a BriefVersion. Referee versions are
// proposals only; confirmation is a separate user-only operation.
type BriefProposer string

const (
	BriefProposerUser    BriefProposer = "user"
	BriefProposerReferee BriefProposer = "referee"
)

// BriefVersion is one immutable content snapshot. Lifecycle metadata may
// change when the version is confirmed or superseded; Content never changes.
type BriefVersion struct {
	RoomID       string        `json:"room_id"`
	Version      int           `json:"version"`
	Status       BriefStatus   `json:"status"`
	Content      Brief         `json:"content"`
	ProposedBy   BriefProposer `json:"proposed_by"`
	SourceTurnID string        `json:"source_turn_id,omitempty"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
	ConfirmedAt  *time.Time    `json:"confirmed_at,omitempty"`
}

// RunStatus is the durable lifecycle of an asynchronous R2/R3 execution.
type RunStatus string

const (
	RunQueued        RunStatus = "queued"
	RunRunning       RunStatus = "running"
	RunSummarizing   RunStatus = "summarizing"
	RunCompleted     RunStatus = "completed"
	RunPartialFailed RunStatus = "partial_failed"
	RunFailed        RunStatus = "failed"
	RunCanceled      RunStatus = "canceled"
)

// RunErrorScope tells clients which recovery controls are safe to offer.
type RunErrorScope string

const (
	RunErrorNone    RunErrorScope = ""
	RunErrorRoom    RunErrorScope = "room"
	RunErrorSeat    RunErrorScope = "seat"
	RunErrorSummary RunErrorScope = "summary"
)

// RoundRun is the idempotent execution record for one room round.
type RoundRun struct {
	ID             string        `json:"id"`
	RoomID         string        `json:"room_id"`
	Round          int           `json:"round"`
	Status         RunStatus     `json:"status"`
	IdempotencyKey string        `json:"idempotency_key"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
	StartedAt      *time.Time    `json:"started_at,omitempty"`
	FinishedAt     *time.Time    `json:"finished_at,omitempty"`
	Error          string        `json:"error,omitempty"`
	ErrorScope     RunErrorScope `json:"error_scope,omitempty"`
}

// RoundProgress is projected from durable per-run seat execution records.
type RoundProgress struct {
	Completed    int      `json:"completed"`
	Total        int      `json:"total"`
	ActiveRoles  []string `json:"active_roles"`
	FailedRoles  []string `json:"failed_roles"`
	SkippedRoles []string `json:"skipped_roles"`
}

// RoundEvent is an append-only, room-scoped event. Seq is the reconnect
// cursor: clients request events after the last sequence they persisted.
type RoundEvent struct {
	Seq       int64     `json:"seq"`
	RoomID    string    `json:"room_id"`
	RunID     string    `json:"run_id"`
	Round     int       `json:"round"`
	Kind      string    `json:"kind"` // run | seat | summary
	Status    string    `json:"status"`
	Role      Role      `json:"role,omitempty"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Room is one roundtable session (design §5.3).
type Room struct {
	ID                    string        `json:"id"`
	Title                 string        `json:"title"`
	State                 RoomState     `json:"state"`
	Brief                 *Brief        `json:"brief,omitempty"` // legacy projection of CurrentBrief.Content
	CurrentBriefVersion   int           `json:"current_brief_version,omitempty"`
	ConfirmedBriefVersion int           `json:"confirmed_brief_version,omitempty"`
	R2BriefVersion        int           `json:"r2_brief_version,omitempty"`
	CurrentBrief          *BriefVersion `json:"current_brief,omitempty"`
	ConfirmedBrief        *BriefVersion `json:"confirmed_brief,omitempty"`
	R2Brief               *BriefVersion `json:"r2_brief,omitempty"`
	SummaryR2             string        `json:"summary_r2,omitempty"`
	SummaryR3             string        `json:"summary_r3,omitempty"`
	CreatedAt             time.Time     `json:"created_at"`
	UpdatedAt             time.Time     `json:"updated_at"`
	Seats                 []Seat        `json:"seats,omitempty"`
	Phase                 string        `json:"phase"`
	PhaseStatus           string        `json:"phase_status"`
	NextAction            string        `json:"next_action"`
	AvailableActions      []string      `json:"available_actions"`
	Progress              RoundProgress `json:"progress"`
	ActiveRun             *RoundRun     `json:"active_run,omitempty"`
	// Turns is the main timeline (content_text only); loaded on GetRoom / ListTurns.
	Turns []Turn `json:"turns,omitempty"`
}

// Seat is one role session binding (design §5.3).
type Seat struct {
	ID          string `json:"id"`
	RoomID      string `json:"room_id"`
	Role        Role   `json:"role"`
	AgentType   string `json:"agent_type"` // MVP: grok-build
	WorkspaceID string `json:"workspace_id"`
	// SessionID is the 1agents ChatSessionRecord id (sidebar / chat WS).
	SessionID string `json:"session_id,omitempty"`
	// AcpSessionID is the agent harness session id (1acp resume).
	AcpSessionID string     `json:"acp_session_id,omitempty"`
	Status       SeatStatus `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
}

// Turn is one main-timeline entry (design §5.3).
// content_text is the only field bound to the default timeline UI.
type Turn struct {
	ID          string    `json:"id"`
	RoomID      string    `json:"room_id"`
	Round       int       `json:"round"`             // 1|2|3
	SeatID      string    `json:"seat_id,omitempty"` // seat uuid or TurnSeatUser
	Kind        string    `json:"kind"`              // chat | speech | summary | system
	ContentText string    `json:"content_text"`
	ProcessRef  string    `json:"process_ref,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}
