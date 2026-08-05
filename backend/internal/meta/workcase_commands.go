package meta

// WorkCase command handlers (#323, docs/architecture/enterprise-foundation-v1.0.0.md
// §5 D3): the ONLY write path for WorkCase state. Web, Agent, Function,
// Human, IM and external API all dispatch these registered commands through
// the commandbus.Gateway, so every case mutation carries an actor, an
// idempotency key, expectedVersion and an execution audit row — and no
// entry point can advance a case (or its phase) by writing the store
// directly.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/scottzx/1Agents/backend/internal/commandbus"
	"github.com/scottzx/1Agents/backend/internal/domainref"
)

// WorkCase command contract names (schemaVersion 1).
const (
	CommandWorkCaseCreate     = "workcase.create"
	CommandWorkCaseUpdate     = "workcase.update"
	CommandWorkCaseTransition = "workcase.transition"
	CommandWorkCaseDelete     = "workcase.delete"
	CommandWorkCaseLink       = "workcase.link"
	CommandWorkCaseUnlink     = "workcase.unlink"
	CommandWorkCaseSetPhase   = "workcase.set_phase"
)

// caseEventSchemaVersion is the schema version of the work_case fact payload
// (caseSnapshot shape) carried by Outbox Events (#324, §5.3).
const caseEventSchemaVersion = 1

// caseCommandKinds are the actor kinds allowed to mutate cases in general.
// Individual commands tighten this (delete is user/human only; terminal
// transitions require a user or human decision, §8: 关键人工决定不可被
// Agent 静默覆盖).
var caseCommandKinds = []commandbus.ActorKind{
	commandbus.ActorUser, commandbus.ActorAgent, commandbus.ActorFunction,
	commandbus.ActorHuman, commandbus.ActorSystem,
}

// RegisterWorkCaseCommands registers the WorkCase command handlers on g.
// Called once at server startup; duplicate registration fails loudly.
func RegisterWorkCaseCommands(g *commandbus.Gateway, s *WorkCaseStore) error {
	descriptors := []commandbus.Descriptor{
		{
			Contract:       CommandWorkCaseCreate,
			SchemaVersions: []int{1},
			AllowedKinds:   caseCommandKinds,
			Handler:        s.handleCreateCase,
		},
		{
			Contract:       CommandWorkCaseUpdate,
			SchemaVersions: []int{1},
			AllowedKinds:   caseCommandKinds,
			Handler:        s.handleUpdateCase,
		},
		{
			Contract:       CommandWorkCaseTransition,
			SchemaVersions: []int{1},
			AllowedKinds:   caseCommandKinds,
			Authorize: func(cmd commandbus.Command) error {
				var p struct {
					Status string `json:"status"`
				}
				// Malformed payloads are rejected later with
				// CodeInvalidPayload by the handler itself.
				if err := cmd.PayloadObject(&p); err != nil || p.Status == "" {
					return nil
				}
				to := CaseStatus(p.Status)
				if to.Terminal() &&
					cmd.Actor.Kind != commandbus.ActorUser &&
					cmd.Actor.Kind != commandbus.ActorHuman {
					return commandbus.NewError(commandbus.CodePermissionDenied,
						"terminal transition to %q requires a user or human actor; %q may only suspend or reopen",
						to, cmd.Actor.Kind)
				}
				return nil
			},
			Handler: s.handleTransitionCase,
		},
		{
			Contract:       CommandWorkCaseDelete,
			SchemaVersions: []int{1},
			// Destructive: agents and functions may not delete cases.
			AllowedKinds: []commandbus.ActorKind{commandbus.ActorUser, commandbus.ActorHuman},
			Handler:      s.handleDeleteCase,
		},
		{
			Contract:       CommandWorkCaseLink,
			SchemaVersions: []int{1},
			AllowedKinds:   caseCommandKinds,
			Handler:        s.handleLinkCase,
		},
		{
			Contract:       CommandWorkCaseUnlink,
			SchemaVersions: []int{1},
			AllowedKinds:   caseCommandKinds,
			Handler:        s.handleUnlinkCase,
		},
		{
			Contract:       CommandWorkCaseSetPhase,
			SchemaVersions: []int{1},
			AllowedKinds:   caseCommandKinds,
			Handler:        s.handleSetPhaseCase,
		},
	}
	for _, d := range descriptors {
		if err := g.Register(d); err != nil {
			return err
		}
	}
	return nil
}

// ── payload shapes ──────────────────────────────────────────────────────────

type caseCreatePayload struct {
	CaseID          string            `json:"caseId"`
	Title           string            `json:"title"`
	Objective       string            `json:"objective"`
	CaseDefinition  string            `json:"caseDefinition"`
	Owner           string            `json:"owner"`
	CreatedBy       string            `json:"createdBy"`
	CurrentPhase    string            `json:"currentPhase"`
	PrimarySubject  string            `json:"primarySubject"`
	SubjectRefs     []string          `json:"subjectRefs"`
	Participants    []CaseParticipant `json:"participants"`
	ExpectedCloseAt *time.Time        `json:"expectedCloseAt"`
}

type caseUpdatePayload struct {
	CaseID          string             `json:"caseId"`
	Title           *string            `json:"title"`
	Objective       *string            `json:"objective"`
	CaseDefinition  *string            `json:"caseDefinition"`
	Owner           *string            `json:"owner"`
	PrimarySubject  *string            `json:"primarySubject"`
	SubjectRefs     *[]string          `json:"subjectRefs"`
	Participants    *[]CaseParticipant `json:"participants"`
	ExpectedCloseAt *time.Time         `json:"expectedCloseAt"`
}

type caseTransitionPayload struct {
	CaseID string `json:"caseId"`
	Status string `json:"status"`
	Reason string `json:"reason"`
}

type caseRefPayload struct {
	CaseID string `json:"caseId"`
}

type caseLinkPayload struct {
	CaseID   string `json:"caseId"`
	Kind     string `json:"kind"`
	TargetID string `json:"targetId"`
}

type caseSetPhasePayload struct {
	CaseID       string `json:"caseId"`
	CurrentPhase string `json:"currentPhase"`
}

// ── handlers ────────────────────────────────────────────────────────────────

func (s *WorkCaseStore) handleCreateCase(ctx context.Context, cmd commandbus.Command, tx *sql.Tx) (commandbus.Result, error) {
	var p caseCreatePayload
	if err := cmd.PayloadObject(&p); err != nil {
		return commandbus.Result{}, err
	}
	if p.Title == "" {
		return commandbus.Result{}, commandbus.NewError(commandbus.CodeInvalidPayload, "payload: title is required")
	}
	c := WorkCase{
		ID:              p.CaseID,
		Title:           p.Title,
		Objective:       p.Objective,
		CaseDefinition:  p.CaseDefinition,
		Owner:           p.Owner,
		CreatedBy:       p.CreatedBy,
		CurrentPhase:    p.CurrentPhase,
		PrimarySubject:  p.PrimarySubject,
		SubjectRefs:     p.SubjectRefs,
		Participants:    p.Participants,
		ExpectedCloseAt: p.ExpectedCloseAt,
	}
	if c.CreatedBy == "" {
		c.CreatedBy = cmd.Actor.Name
	}
	created, err := s.CreateInTx(tx, cmd.WorkspaceID, c)
	if err != nil {
		return commandbus.Result{}, classifyCaseError(err)
	}
	event := caseCommandEvent(cmd, created.ID, "create", nil, caseSnapshot(created))
	stored, err := appendProjectEventTx(tx, event, false)
	if err != nil {
		return commandbus.Result{}, classifyCaseError(err)
	}
	return caseResult(created, stored.ID, "create")
}

func (s *WorkCaseStore) handleUpdateCase(ctx context.Context, cmd commandbus.Command, tx *sql.Tx) (commandbus.Result, error) {
	var p caseUpdatePayload
	if err := cmd.PayloadObject(&p); err != nil {
		return commandbus.Result{}, err
	}
	if p.CaseID == "" {
		return commandbus.Result{}, commandbus.NewError(commandbus.CodeInvalidPayload, "payload: caseId is required")
	}
	if cmd.ExpectedVersion <= 0 {
		return commandbus.Result{}, commandbus.NewError(commandbus.CodeInvalidPayload, "expectedVersion is required for workcase.update")
	}
	before, err := loadCaseTx(tx, cmd.WorkspaceID, p.CaseID)
	if err != nil {
		return commandbus.Result{}, classifyCaseError(err)
	}
	updated, err := s.UpdateInTx(tx, cmd.WorkspaceID, p.CaseID, WorkCasePatch{
		Title:           p.Title,
		Objective:       p.Objective,
		CaseDefinition:  p.CaseDefinition,
		Owner:           p.Owner,
		PrimarySubject:  p.PrimarySubject,
		SubjectRefs:     p.SubjectRefs,
		Participants:    p.Participants,
		ExpectedCloseAt: p.ExpectedCloseAt,
	}, cmd.ExpectedVersion)
	if err != nil {
		return commandbus.Result{}, classifyCaseError(err)
	}
	event := caseCommandEvent(cmd, updated.ID, "update", caseSnapshot(before), caseSnapshot(updated))
	stored, err := appendProjectEventTx(tx, event, false)
	if err != nil {
		return commandbus.Result{}, classifyCaseError(err)
	}
	return caseResult(updated, stored.ID, "update")
}

func (s *WorkCaseStore) handleTransitionCase(ctx context.Context, cmd commandbus.Command, tx *sql.Tx) (commandbus.Result, error) {
	var p caseTransitionPayload
	if err := cmd.PayloadObject(&p); err != nil {
		return commandbus.Result{}, err
	}
	if p.CaseID == "" {
		return commandbus.Result{}, commandbus.NewError(commandbus.CodeInvalidPayload, "payload: caseId is required")
	}
	to := CaseStatus(p.Status)
	if !to.Valid() {
		return commandbus.Result{}, commandbus.NewError(commandbus.CodeInvalidPayload,
			"payload: status must be one of open|suspended|closed|cancelled")
	}
	if cmd.ExpectedVersion <= 0 {
		return commandbus.Result{}, commandbus.NewError(commandbus.CodeInvalidPayload, "expectedVersion is required for workcase.transition")
	}
	before, err := loadCaseTx(tx, cmd.WorkspaceID, p.CaseID)
	if err != nil {
		return commandbus.Result{}, classifyCaseError(err)
	}
	updated, err := s.TransitionInTx(tx, cmd.WorkspaceID, p.CaseID, to, p.Reason, cmd.ExpectedVersion)
	if err != nil {
		return commandbus.Result{}, classifyCaseError(err)
	}
	event := caseCommandEvent(cmd, updated.ID, "transition", caseSnapshot(before), caseSnapshot(updated))
	stored, err := appendProjectEventTx(tx, event, false)
	if err != nil {
		return commandbus.Result{}, classifyCaseError(err)
	}
	return caseResult(updated, stored.ID, "transition")
}

func (s *WorkCaseStore) handleDeleteCase(ctx context.Context, cmd commandbus.Command, tx *sql.Tx) (commandbus.Result, error) {
	var p caseRefPayload
	if err := cmd.PayloadObject(&p); err != nil {
		return commandbus.Result{}, err
	}
	if p.CaseID == "" {
		return commandbus.Result{}, commandbus.NewError(commandbus.CodeInvalidPayload, "payload: caseId is required")
	}
	before, err := loadCaseTx(tx, cmd.WorkspaceID, p.CaseID)
	if err != nil {
		return commandbus.Result{}, classifyCaseError(err)
	}
	if err := s.DeleteInTx(tx, cmd.WorkspaceID, p.CaseID); err != nil {
		return commandbus.Result{}, classifyCaseError(err)
	}
	event := caseCommandEvent(cmd, p.CaseID, "delete", caseSnapshot(before), nil)
	stored, err := appendProjectEventTx(tx, event, false)
	if err != nil {
		return commandbus.Result{}, classifyCaseError(err)
	}
	payload, _ := json.Marshal(map[string]any{"ok": true, "id": p.CaseID})
	return commandbus.Result{
		Version:            before.Version,
		EventID:            stored.ID,
		TargetID:           p.CaseID,
		Payload:            payload,
		EventType:          "work_case.delete",
		EventSchemaVersion: caseEventSchemaVersion,
		SubjectRef:         caseSubjectRef(cmd.WorkspaceID, p.CaseID),
	}, nil
}

func (s *WorkCaseStore) handleLinkCase(ctx context.Context, cmd commandbus.Command, tx *sql.Tx) (commandbus.Result, error) {
	var p caseLinkPayload
	if err := cmd.PayloadObject(&p); err != nil {
		return commandbus.Result{}, err
	}
	if p.CaseID == "" || p.TargetID == "" {
		return commandbus.Result{}, commandbus.NewError(commandbus.CodeInvalidPayload, "payload: caseId and targetId are required")
	}
	kind := CaseLinkKind(p.Kind)
	if !kind.Valid() {
		return commandbus.Result{}, commandbus.NewError(commandbus.CodeInvalidPayload, "payload: kind must be one of task|session|artifact")
	}
	if cmd.ExpectedVersion <= 0 {
		return commandbus.Result{}, commandbus.NewError(commandbus.CodeInvalidPayload, "expectedVersion is required for workcase.link")
	}
	before, err := loadCaseTx(tx, cmd.WorkspaceID, p.CaseID)
	if err != nil {
		return commandbus.Result{}, classifyCaseError(err)
	}
	updated, err := s.LinkInTx(tx, cmd.WorkspaceID, p.CaseID, kind, p.TargetID, cmd.ExpectedVersion)
	if err != nil {
		return commandbus.Result{}, classifyCaseError(err)
	}
	event := caseCommandEvent(cmd, updated.ID, "link", caseSnapshot(before), caseSnapshot(updated))
	stored, err := appendProjectEventTx(tx, event, false)
	if err != nil {
		return commandbus.Result{}, classifyCaseError(err)
	}
	return caseResult(updated, stored.ID, "link")
}

func (s *WorkCaseStore) handleUnlinkCase(ctx context.Context, cmd commandbus.Command, tx *sql.Tx) (commandbus.Result, error) {
	var p caseLinkPayload
	if err := cmd.PayloadObject(&p); err != nil {
		return commandbus.Result{}, err
	}
	if p.CaseID == "" || p.TargetID == "" {
		return commandbus.Result{}, commandbus.NewError(commandbus.CodeInvalidPayload, "payload: caseId and targetId are required")
	}
	kind := CaseLinkKind(p.Kind)
	if !kind.Valid() {
		return commandbus.Result{}, commandbus.NewError(commandbus.CodeInvalidPayload, "payload: kind must be one of task|session|artifact")
	}
	if cmd.ExpectedVersion <= 0 {
		return commandbus.Result{}, commandbus.NewError(commandbus.CodeInvalidPayload, "expectedVersion is required for workcase.unlink")
	}
	before, err := loadCaseTx(tx, cmd.WorkspaceID, p.CaseID)
	if err != nil {
		return commandbus.Result{}, classifyCaseError(err)
	}
	updated, err := s.UnlinkInTx(tx, cmd.WorkspaceID, p.CaseID, kind, p.TargetID, cmd.ExpectedVersion)
	if err != nil {
		return commandbus.Result{}, classifyCaseError(err)
	}
	event := caseCommandEvent(cmd, updated.ID, "unlink", caseSnapshot(before), caseSnapshot(updated))
	stored, err := appendProjectEventTx(tx, event, false)
	if err != nil {
		return commandbus.Result{}, classifyCaseError(err)
	}
	return caseResult(updated, stored.ID, "unlink")
}

func (s *WorkCaseStore) handleSetPhaseCase(ctx context.Context, cmd commandbus.Command, tx *sql.Tx) (commandbus.Result, error) {
	var p caseSetPhasePayload
	if err := cmd.PayloadObject(&p); err != nil {
		return commandbus.Result{}, err
	}
	if p.CaseID == "" {
		return commandbus.Result{}, commandbus.NewError(commandbus.CodeInvalidPayload, "payload: caseId is required")
	}
	if cmd.ExpectedVersion <= 0 {
		return commandbus.Result{}, commandbus.NewError(commandbus.CodeInvalidPayload, "expectedVersion is required for workcase.set_phase")
	}
	before, err := loadCaseTx(tx, cmd.WorkspaceID, p.CaseID)
	if err != nil {
		return commandbus.Result{}, classifyCaseError(err)
	}
	updated, err := s.SetPhaseInTx(tx, cmd.WorkspaceID, p.CaseID, p.CurrentPhase, cmd.ExpectedVersion)
	if err != nil {
		return commandbus.Result{}, classifyCaseError(err)
	}
	event := caseCommandEvent(cmd, updated.ID, "update", caseSnapshot(before), caseSnapshot(updated))
	stored, err := appendProjectEventTx(tx, event, false)
	if err != nil {
		return commandbus.Result{}, classifyCaseError(err)
	}
	return caseResult(updated, stored.ID, "update")
}

// ── shared helpers ──────────────────────────────────────────────────────────

// caseCommandEvent builds the immutable domain audit event for one case
// command, attributed to the command's actor (§5.1 actor/Command 可审计).
func caseCommandEvent(cmd commandbus.Command, caseID, operation string, before, after any) ProjectEvent {
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	return ProjectEvent{
		ID:                   newID(),
		ProjectID:            cmd.WorkspaceID,
		CorrelationID:        cmd.CorrelationID,
		TurnID:               cmd.Actor.TurnID,
		SessionID:            cmd.Actor.SessionID,
		TaskRunID:            cmd.Actor.TaskRunID,
		ActorKind:            string(cmd.Actor.Kind),
		ActorName:            cmd.Actor.Name,
		Origin:               cmd.Actor.Origin,
		EventType:            "work_case." + operation,
		TargetType:           "work_case",
		TargetID:             caseID,
		Operation:            operation,
		Before:               beforeJSON,
		After:                afterJSON,
		Status:               ProjectEventSucceeded,
		AllowUnprojectedTurn: cmd.Actor.AllowUnprojectedTurn,
	}
}

// caseSnapshot is the event before/after projection of a case.
func caseSnapshot(c WorkCase) map[string]any {
	return map[string]any{
		"id":             c.ID,
		"workspaceId":    c.WorkspaceID,
		"title":          c.Title,
		"caseDefinition": c.CaseDefinition,
		"status":         c.Status,
		"currentPhase":   c.CurrentPhase,
		"owner":          c.Owner,
		"primarySubject": c.PrimarySubject,
		"version":        c.Version,
	}
}

// caseResult packages a mutated case into the command result envelope,
// including the Outbox Event envelope fields (#324, §5.3): the gateway
// appends the delivery row from them inside the command transaction.
func caseResult(c WorkCase, eventID, operation string) (commandbus.Result, error) {
	payload, err := json.Marshal(c)
	if err != nil {
		return commandbus.Result{}, commandbus.WrapError(commandbus.CodeInternal, err, "marshal case result: %v", err)
	}
	return commandbus.Result{
		Version:            c.Version,
		EventID:            eventID,
		TargetID:           c.ID,
		Payload:            payload,
		EventType:          "work_case." + operation,
		EventSchemaVersion: caseEventSchemaVersion,
		SubjectRef:         caseSubjectRef(c.WorkspaceID, c.ID),
	}, nil
}

// caseSubjectRef renders the canonical CaseRef of the subject object the
// event is about (§5.3 subjectRef).
func caseSubjectRef(workspaceID, caseID string) string {
	ref, err := domainref.NewCaseRef(workspaceID, caseID, 0)
	if err != nil {
		return ""
	}
	return ref.String()
}

// classifyCaseError maps store sentinels onto stable command error codes so
// the gateway audit and the HTTP layer see one error taxonomy (§5:
// 领域拒绝、版本冲突、非法 payload 各有其码).
func classifyCaseError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, ErrCaseVersionConflict):
		return commandbus.WrapError(commandbus.CodeVersionConflict, err, "%v", err)
	case errors.Is(err, ErrInvalidProjectEvent),
		errors.Is(err, ErrInvalidCaseTransition),
		errors.Is(err, ErrDuplicate),
		errors.Is(err, ErrNotFound),
		errors.Is(err, ErrProjectMismatch):
		return commandbus.WrapError(commandbus.CodeDomainRejected, err, "%v", err)
	}
	if c, ok := domainref.CodeOf(err); ok && c == domainref.CodeInvalidRef {
		return commandbus.WrapError(commandbus.CodeInvalidPayload, err, "%v", err)
	}
	return commandbus.WrapError(commandbus.CodeInternal, err, "%v", err)
}
