package meta

import (
	"encoding/json"
	"time"
)

type AgentTurnStatus string

const (
	AgentTurnQueued    AgentTurnStatus = "queued"
	AgentTurnRunning   AgentTurnStatus = "running"
	AgentTurnCompleted AgentTurnStatus = "completed"
	AgentTurnFailed    AgentTurnStatus = "failed"
	AgentTurnCancelled AgentTurnStatus = "cancelled"
)

func (s AgentTurnStatus) terminal() bool {
	return s == AgentTurnCompleted || s == AgentTurnFailed || s == AgentTurnCancelled
}

type AgentTurn struct {
	ID                 string          `json:"id"`
	ProjectID          string          `json:"projectId"`
	SessionID          string          `json:"sessionId"`
	ClientRequestID    string          `json:"clientRequestId,omitempty"`
	InitiatingReplyID  string          `json:"initiatingReplyId,omitempty"`
	AgentType          string          `json:"agentType,omitempty"`
	ProfileSnapshot    json.RawMessage `json:"profileSnapshot,omitempty"`
	Status             AgentTurnStatus `json:"status"`
	PromptText         string          `json:"promptText,omitempty"`
	RequestFingerprint string          `json:"requestFingerprint,omitempty"`
	FinalAnswer        string          `json:"finalAnswer,omitempty"`
	ErrorCode          string          `json:"errorCode,omitempty"`
	ErrorText          string          `json:"errorText,omitempty"`
	RuntimeRecordID    string          `json:"runtimeRecordId,omitempty"`
	RuntimeRequestID   string          `json:"runtimeRequestId,omitempty"`
	PromptMessageID    string          `json:"promptMessageId,omitempty"`
	FinalReplyID       string          `json:"finalReplyId,omitempty"`
	StopReason         string          `json:"stopReason,omitempty"`
	TerminalSource     string          `json:"terminalSource,omitempty"`
	LastEventSeq       int64           `json:"lastEventSeq,omitempty"`
	StartedAt          *time.Time      `json:"startedAt,omitempty"`
	CompletedAt        *time.Time      `json:"completedAt,omitempty"`
	CreatedAt          time.Time       `json:"createdAt"`
	UpdatedAt          time.Time       `json:"updatedAt"`
}

type AgentTurnTransition struct {
	Status           AgentTurnStatus
	FinalAnswer      string
	ErrorCode        string
	ErrorText        string
	RuntimeRecordID  string
	RuntimeRequestID string
	PromptMessageID  string
	FinalReplyID     string
	StopReason       string
	TerminalSource   string
	LastEventSeq     int64
	At               time.Time
}

type AgentTurnListOptions struct {
	ProjectID string
	SessionID string
	Status    AgentTurnStatus
	Cursor    string
	Limit     int
}

type AgentTurnPage struct {
	Items      []AgentTurn `json:"items"`
	NextCursor string      `json:"nextCursor,omitempty"`
	HasMore    bool        `json:"hasMore"`
}

type ProjectEventStatus string

const (
	ProjectEventSucceeded ProjectEventStatus = "succeeded"
	ProjectEventRejected  ProjectEventStatus = "rejected"
	ProjectEventFailed    ProjectEventStatus = "failed"
)

type ProjectEvent struct {
	ID            string             `json:"id"`
	ProjectID     string             `json:"projectId"`
	CorrelationID string             `json:"correlationId,omitempty"`
	TurnID        string             `json:"turnId,omitempty"`
	SessionID     string             `json:"sessionId,omitempty"`
	TaskRunID     string             `json:"taskRunId,omitempty"`
	ActorKind     string             `json:"actorKind"`
	ActorName     string             `json:"actorName,omitempty"`
	Origin        string             `json:"origin"`
	EventType     string             `json:"eventType"`
	TargetType    string             `json:"targetType"`
	TargetID      string             `json:"targetId"`
	Operation     string             `json:"operation"`
	Before        json.RawMessage    `json:"before,omitempty"`
	After         json.RawMessage    `json:"after,omitempty"`
	Status        ProjectEventStatus `json:"status"`
	ErrorCode     string             `json:"errorCode,omitempty"`
	ErrorText     string             `json:"errorText,omitempty"`
	Sequence      int64              `json:"sequence"`
	CreatedAt     time.Time          `json:"createdAt"`
	// AllowUnprojectedTurn is set only by a trusted live 1ACP mutation
	// context. It lets the immutable event commit when agent_turns is missing
	// or stale, without weakening the default DB-backed validation path.
	AllowUnprojectedTurn bool `json:"-"`
}

type ProjectEventListOptions struct {
	ProjectID  string
	SessionID  string
	TurnID     string
	TaskRunID  string
	TargetType string
	TargetID   string
	Status     ProjectEventStatus
	Origin     string
	Cursor     string
	Limit      int
}

type ProjectEventPage struct {
	Items      []ProjectEvent `json:"items"`
	NextCursor string         `json:"nextCursor,omitempty"`
	HasMore    bool           `json:"hasMore"`
}

// MutationContext is the trusted server-side provenance attached to a project
// mutation. Callers must derive it from authenticated Session/TaskRun state,
// never from arbitrary request fields.
type MutationContext struct {
	ProjectID     string
	ActorKind     string
	ActorName     string
	SessionID     string
	TurnID        string
	TaskRunID     string
	CorrelationID string
	Origin        string
	// AuthoritativeTurn means the live 1ACP Journal state already validated
	// this Turn. It is server-derived and never accepted from request input.
	AuthoritativeTurn bool
}
