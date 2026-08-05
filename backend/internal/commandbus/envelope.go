// Package commandbus implements the C0 write contract frozen by
// docs/architecture/enterprise-foundation-v1.0.0.md (§5 D3, §8, D6): every
// state mutation — from Web, Agent, Function, Human, IM or external API —
// enters the same versioned Command envelope, is authorized against the
// actor's permission context, is de-duplicated by idempotencyKey, is guarded
// by expectedVersion optimistic concurrency, and is recorded in an execution
// audit trail (actor, command, target, result, duration). Successful
// commands that produce a fact append an Outbox Event delivery row in the
// same transaction (see package outbox), so the notification commits
// atomically with the state change.
//
// C0 scope (design §5.3): in-process typed handling, a stable envelope, a
// handler registry and tests. No network message bus. Domain owners register
// their command handlers here; callers must never write owner tables
// directly.
//
// This package is L1 kernel infrastructure: it imports only the standard
// library. Domain packages (meta) register handlers; it never imports them.
package commandbus

import (
	"encoding/json"
	"regexp"
)

// ActorKind enumerates who may issue a command. Web requests resolve to
// "user", agent sessions to "agent"; "function", "human" and "system" cover
// the non-interactive executors. IM and external API arrive through their
// channel origin but act as one of these kinds.
type ActorKind string

const (
	ActorUser     ActorKind = "user"
	ActorAgent    ActorKind = "agent"
	ActorFunction ActorKind = "function"
	ActorHuman    ActorKind = "human"
	ActorSystem   ActorKind = "system"
)

// Valid reports whether k is a known actor kind.
func (k ActorKind) Valid() bool {
	switch k {
	case ActorUser, ActorAgent, ActorFunction, ActorHuman, ActorSystem:
		return true
	}
	return false
}

// Actor is the authenticated permission context attached to every command
// (§5.3 actor 与权限上下文). It is always server-derived from session/turn
// state — never accepted from arbitrary request fields. C0 keeps the model
// minimal (kind-based policies per command); full RBAC is deferred (§10).
type Actor struct {
	Kind      ActorKind `json:"kind"`
	Name      string    `json:"name"`
	SessionID string    `json:"sessionId,omitempty"`
	TurnID    string    `json:"turnId,omitempty"`
	TaskRunID string    `json:"taskRunId,omitempty"`
	// Origin names the entry channel: http, cli, mcp, im, api, scheduler...
	Origin string `json:"origin,omitempty"`
	// CorrelationID ties the command to the surrounding conversation/turn.
	CorrelationID string `json:"correlationId,omitempty"`
	// AllowUnprojectedTurn is a trusted server-derived flag (live 1ACP turn
	// already validated) mirroring meta.MutationContext.AuthoritativeTurn.
	AllowUnprojectedTurn bool `json:"-"`
}

// Command is the unified write envelope (§5.3 公共字段). Every state change
// across Web, Agent, Function, Human, IM and external API is expressed as a
// Command and dispatched through one Gateway.
type Command struct {
	// Contract names the registered command, e.g. "workcase.transition".
	Contract string `json:"contract"`
	// SchemaVersion pins the payload contract version the caller speaks.
	SchemaVersion int `json:"schemaVersion"`
	// WorkspaceID scopes the command to one workspace (== projects.id).
	WorkspaceID string `json:"workspaceId"`
	// Actor is the authenticated permission context.
	Actor Actor `json:"actor"`
	// CorrelationID / CausationID carry causality for audit (§5.3).
	CorrelationID string `json:"correlationId,omitempty"`
	CausationID   string `json:"causationId,omitempty"`
	// IdempotencyKey de-duplicates retries: a repeated submission with the
	// same workspace+key replays the stored result instead of re-executing.
	IdempotencyKey string `json:"idempotencyKey"`
	// ExpectedVersion is the optimistic-concurrency token required by
	// commands mutating an existing object (0 for creation commands).
	ExpectedVersion int `json:"expectedVersion,omitempty"`
	// TargetID optionally names the subject object (e.g. the case id) so
	// rejections are auditable against their target before dispatch.
	TargetID string `json:"targetId,omitempty"`
	// Payload carries the owner-defined command body.
	Payload json.RawMessage `json:"payload,omitempty"`
}

// contractPattern constrains command names to stable lowercase identifiers
// with a dot separator ("workcase.transition"), mirroring domainref idents.
var contractPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*(\.[a-z0-9][a-z0-9_-]*)+$`)

const maxIdempotencyKeyLen = 200

// Validate checks the envelope invariants shared by every command (§5.3).
// Payload content is validated by the owning handler; the gateway only
// requires it to be valid JSON (absent payloads become {}).
func (c Command) Validate() error {
	if !contractPattern.MatchString(c.Contract) {
		return NewError(CodeInvalidPayload,
			"contract %q must be a dotted lowercase identifier (e.g. workcase.transition)", c.Contract)
	}
	if c.SchemaVersion < 1 {
		return NewError(CodeInvalidPayload, "schemaVersion must be >= 1, got %d", c.SchemaVersion)
	}
	if c.WorkspaceID == "" {
		return NewError(CodeInvalidPayload, "workspaceId is required")
	}
	if !c.Actor.Kind.Valid() {
		return NewError(CodeInvalidPayload, "actor kind %q is not one of user|agent|function|human|system", c.Actor.Kind)
	}
	if c.Actor.Name == "" {
		return NewError(CodeInvalidPayload, "actor name is required")
	}
	if c.IdempotencyKey == "" {
		return NewError(CodeInvalidPayload, "idempotencyKey is required")
	}
	if len(c.IdempotencyKey) > maxIdempotencyKeyLen {
		return NewError(CodeInvalidPayload, "idempotencyKey exceeds %d bytes", maxIdempotencyKeyLen)
	}
	if c.ExpectedVersion < 0 {
		return NewError(CodeInvalidPayload, "expectedVersion must be >= 0, got %d", c.ExpectedVersion)
	}
	if len(c.Payload) == 0 {
		return nil
	}
	if !json.Valid(c.Payload) {
		return NewError(CodeInvalidPayload, "payload is not valid JSON")
	}
	return nil
}

// PayloadObject unmarshals the payload into dst, treating an absent payload
// as an empty object. Handlers call this first and convert failures to
// CodeInvalidPayload.
func (c Command) PayloadObject(dst any) error {
	raw := c.Payload
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return NewError(CodeInvalidPayload, "payload: %v", err)
	}
	return nil
}

// Result outcome markers.
const (
	// ResultSucceeded — the handler executed and committed the change.
	ResultSucceeded = "succeeded"
	// ResultReplayed — an identical idempotencyKey already committed; the
	// stored result is returned and no effect was produced this time.
	ResultReplayed = "replayed"
)

// Result is what a dispatched command returns (§5.2 CommandResult):
// the outcome, the new object version, the produced domain event id and the
// owner-defined result payload.
type Result struct {
	Status   string          `json:"status"`
	Version  int             `json:"version,omitempty"`
	EventID  string          `json:"eventId,omitempty"`
	TargetID string          `json:"targetId,omitempty"`
	Payload  json.RawMessage `json:"payload,omitempty"`
	// EventType, EventSchemaVersion and SubjectRef complete the Outbox Event
	// envelope (§5.3) of the produced fact. Whenever EventID is set the
	// gateway requires all three and appends the outbox delivery row
	// atomically inside the command transaction — a handler that produces an
	// event but omits its envelope fails the whole command.
	EventType          string `json:"eventType,omitempty"`
	EventSchemaVersion int    `json:"eventSchemaVersion,omitempty"`
	SubjectRef         string `json:"subjectRef,omitempty"`
}
