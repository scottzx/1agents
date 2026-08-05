package agent

// WorkCase REST surface (task #322, wired through the unified Command
// Gateway by #323; docs/architecture/enterprise-foundation-v1.0.0.md §4.3,
// §5 D3). WorkCase is the kernel-owned long-running business matter between
// domain objects and Tasks:
//
//	GET    /api/agent/work-cases?workspace_id=X[&status=S]   list
//	POST   /api/agent/work-cases                             create        → workcase.create
//	GET    /api/agent/work-cases/{id}                        detail incl. links
//	PATCH  /api/agent/work-cases/{id}                        partial edit  → workcase.update
//	DELETE /api/agent/work-cases/{id}?workspace_id=X         delete        → workcase.delete
//	POST   /api/agent/work-cases/{id}/transition             lifecycle     → workcase.transition
//	POST   /api/agent/work-cases/{id}/phase                  set phase     → workcase.set_phase
//	POST   /api/agent/work-cases/{id}/links                  link          → workcase.link
//	DELETE /api/agent/work-cases/{id}/links?kind=&target_id=&expected_version=
//	                                                       unlink          → workcase.unlink
//	GET    /api/agent/work-cases/{id}/tasks                  tasks by Case
//	GET    /api/agent/work-cases/{id}/runs                   task runs by Case
//	GET    /api/agent/work-cases/{id}/events                 domain events targeting the Case
//	GET    /api/agent/work-cases/{id}/commands               execution audit trail of the Case
//
// Every mutation is dispatched as a Command through commandbus.Gateway
// (#323): Web, Agent, Function, Human, IM and external API share one write
// path with actor attribution, idempotencyKey de-duplication, expectedVersion
// optimistic concurrency (409) and a full execution audit. Generic PATCH can
// neither transition status nor change currentPhase — phase advances only via
// the dedicated workcase.set_phase command (禁止直接修改 Case phase).
//
// Mutations accept the idempotency key as the Idempotency-Key header or the
// idempotencyKey body field; when absent a fresh key is generated (no
// de-duplication). Replaying a committed key returns the original result.

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/scottzx/1Agents/backend/internal/commandbus"
	"github.com/scottzx/1Agents/backend/internal/domainref"
	"github.com/scottzx/1Agents/backend/internal/meta"
)

// requireWorkCaseStore answers 500 (matching the task-run store pattern)
// when the meta DB was unavailable at handler construction.
func (h *Handler) requireWorkCaseStore(w http.ResponseWriter) bool {
	if h.workCaseStore == nil {
		http.Error(w, "work case store is unavailable", http.StatusInternalServerError)
		return false
	}
	return true
}

// requireCommandBus guards the mutation endpoints: without the gateway there
// is no authorized write path for case state (#323).
func (h *Handler) requireCommandBus(w http.ResponseWriter) bool {
	if h.commandBus == nil {
		http.Error(w, "command gateway is unavailable", http.StatusInternalServerError)
		return false
	}
	return true
}

// commandActor converts the trusted server-derived mutation context into the
// command permission context (§5.3 actor). Never taken from request fields.
func commandActor(ctx meta.MutationContext) commandbus.Actor {
	return commandbus.Actor{
		Kind:                 commandbus.ActorKind(ctx.ActorKind),
		Name:                 ctx.ActorName,
		SessionID:            ctx.SessionID,
		TurnID:               ctx.TurnID,
		TaskRunID:            ctx.TaskRunID,
		Origin:               ctx.Origin,
		CorrelationID:        ctx.CorrelationID,
		AllowUnprojectedTurn: ctx.AuthoritativeTurn,
	}
}

// requestIdempotencyKey resolves the caller-supplied idempotency key
// (header first, then body field) or generates a fresh one so retries of
// the same submission de-duplicate while new submissions never collide.
func requestIdempotencyKey(r *http.Request, bodyKey string) string {
	if k := r.Header.Get("Idempotency-Key"); k != "" {
		return k
	}
	if bodyKey != "" {
		return bodyKey
	}
	return newID()
}

// dispatchCaseCommand builds the unified envelope and runs it through the
// gateway, translating the outcome back to HTTP.
func (h *Handler) dispatchCaseCommand(w http.ResponseWriter, r *http.Request, contract string, mutationCtx meta.MutationContext, expectedVersion int, targetID, idempotencyKey string, payload map[string]any) {
	raw, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cmd := commandbus.Command{
		Contract:        contract,
		SchemaVersion:   1,
		WorkspaceID:     mutationCtx.ProjectID,
		Actor:           commandActor(mutationCtx),
		CorrelationID:   mutationCtx.CorrelationID,
		IdempotencyKey:  idempotencyKey,
		ExpectedVersion: expectedVersion,
		TargetID:        targetID,
		Payload:         raw,
	}
	result, err := h.commandBus.Dispatch(r.Context(), cmd)
	if err != nil {
		writeCommandError(w, err)
		return
	}
	writeCommandResult(w, result, mutationCtx)
}

// writeCommandResult renders a command outcome in the historical mutation
// response shape (entity fields + eventId/sessionId/turnId). A replayed
// submission returns the identical stored result plus "replayed": true.
func writeCommandResult(w http.ResponseWriter, result commandbus.Result, ctx meta.MutationContext) {
	response := map[string]any{}
	if len(result.Payload) > 0 {
		_ = json.Unmarshal(result.Payload, &response)
	}
	response["eventId"] = result.EventID
	if ctx.SessionID != "" {
		response["sessionId"] = ctx.SessionID
	}
	if ctx.TurnID != "" {
		response["turnId"] = ctx.TurnID
	}
	if result.Status == commandbus.ResultReplayed {
		response["replayed"] = true
	}
	writeJSON(w, response)
}

// writeCommandError maps command error codes to HTTP statuses with a
// machine-readable JSON body, so unregistered commands, missing permission,
// invalid payloads, version conflicts and domain rejections are each
// distinguishable (#323 acceptance: 不同错误码).
func writeCommandError(w http.ResponseWriter, err error) {
	code, _ := commandbus.CodeOf(err)
	status := http.StatusInternalServerError
	switch code {
	case commandbus.CodeUnknownCommand, commandbus.CodeInvalidPayload:
		status = http.StatusBadRequest
	case commandbus.CodePermissionDenied:
		status = http.StatusForbidden
	case commandbus.CodeVersionConflict:
		status = http.StatusConflict
	case commandbus.CodeDomainRejected:
		status = http.StatusBadRequest
		switch {
		case errors.Is(err, meta.ErrNotFound):
			status = http.StatusNotFound
		case errors.Is(err, meta.ErrProjectMismatch):
			status = http.StatusForbidden
		case errors.Is(err, meta.ErrDuplicate):
			status = http.StatusConflict
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code":    string(code),
		"message": err.Error(),
	})
}

// HandleWorkCasesRoot serves GET (list) and POST (create) on
// /api/agent/work-cases.
func (h *Handler) HandleWorkCasesRoot(w http.ResponseWriter, r *http.Request) {
	if !h.requireWorkCaseStore(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		wsID := r.URL.Query().Get("workspace_id")
		if wsID == "" {
			http.Error(w, "workspace_id query parameter is required", http.StatusBadRequest)
			return
		}
		wsPath, err := h.resolveWorkspacePath(wsID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		projectID, err := h.tasksStore.ProjectIDForPath(wsPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if projectID == "" {
			writeJSON(w, []meta.WorkCase{})
			return
		}
		status := meta.CaseStatus(r.URL.Query().Get("status"))
		if status != "" && !status.Valid() {
			http.Error(w, "unknown status filter: "+string(status), http.StatusBadRequest)
			return
		}
		cases, err := h.workCaseStore.List(projectID, status)
		if err != nil {
			writeWorkCaseError(w, err)
			return
		}
		writeJSON(w, cases)

	case http.MethodPost:
		if !h.requireCommandBus(w) {
			return
		}
		var body struct {
			WorkspaceID     string                 `json:"workspace_id"`
			IdempotencyKey  string                 `json:"idempotencyKey"`
			Title           string                 `json:"title"`
			Objective       string                 `json:"objective"`
			CaseDefinition  string                 `json:"caseDefinition"`
			Owner           string                 `json:"owner"`
			CreatedBy       string                 `json:"createdBy"`
			CurrentPhase    string                 `json:"currentPhase"`
			PrimarySubject  string                 `json:"primarySubject"`
			SubjectRefs     []string               `json:"subjectRefs"`
			Participants    []meta.CaseParticipant `json:"participants"`
			ExpectedCloseAt string                 `json:"expectedCloseAt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if body.WorkspaceID == "" || body.Title == "" {
			http.Error(w, "workspace_id and title are required", http.StatusBadRequest)
			return
		}
		wsPath, err := h.resolveWorkspacePath(body.WorkspaceID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		projectID, err := h.tasksStore.ProjectIDForPath(wsPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if projectID == "" {
			projectID = body.WorkspaceID
		}
		mutationCtx, err := h.resolveMutationContext(r, projectID)
		if err != nil {
			writeMutationContextError(w, err)
			return
		}
		caseID := newID()
		payload := map[string]any{
			"caseId":         caseID,
			"title":          body.Title,
			"objective":      body.Objective,
			"caseDefinition": body.CaseDefinition,
			"owner":          body.Owner,
			"createdBy":      body.CreatedBy,
			"currentPhase":   body.CurrentPhase,
			"primarySubject": body.PrimarySubject,
			"subjectRefs":    body.SubjectRefs,
			"participants":   body.Participants,
		}
		if body.ExpectedCloseAt != "" {
			payload["expectedCloseAt"] = body.ExpectedCloseAt
		}
		h.dispatchCaseCommand(w, r, meta.CommandWorkCaseCreate, mutationCtx, 0,
			caseID, requestIdempotencyKey(r, body.IdempotencyKey), payload)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleWorkCasesItem serves /api/agent/work-cases/{id} and its sub-resources
// (see the route table at the top of this file).
func (h *Handler) HandleWorkCasesItem(w http.ResponseWriter, r *http.Request) {
	if !h.requireWorkCaseStore(w) {
		return
	}
	const prefix = "/api/agent/work-cases/"
	rest := r.URL.Path[len(prefix):]
	if rest == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	id := rest
	sub := ""
	if i := indexByte(rest, '/'); i >= 0 {
		id, sub = rest[:i], rest[i+1:]
	}

	switch sub {
	case "":
		h.handleWorkCaseItem(w, r, id)
	case "transition":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.handleWorkCaseTransition(w, r, id)
	case "phase":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.handleWorkCaseSetPhase(w, r, id)
	case "links":
		switch r.Method {
		case http.MethodPost:
			h.handleWorkCaseLinkCreate(w, r, id)
		case http.MethodDelete:
			h.handleWorkCaseLinkDelete(w, r, id)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	case "tasks":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.handleWorkCaseTasks(w, id)
	case "runs":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.handleWorkCaseRuns(w, id)
	case "events":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.handleWorkCaseEvents(w, r, id)
	case "commands":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.handleWorkCaseCommandAudit(w, r, id)
	default:
		http.Error(w, "unsupported sub-path", http.StatusNotFound)
	}
}

// workCaseDetail is the item GET shape: the case plus its association edges.
type workCaseDetail struct {
	meta.WorkCase
	Links []meta.CaseLink `json:"links"`
}

func (h *Handler) handleWorkCaseItem(w http.ResponseWriter, r *http.Request, id string) {
	switch r.Method {
	case http.MethodGet:
		c, ok, err := h.workCaseStore.Get(id)
		if err != nil {
			writeWorkCaseError(w, err)
			return
		}
		if !ok {
			http.Error(w, "work case not found", http.StatusNotFound)
			return
		}
		links, err := h.workCaseStore.ListLinks(id)
		if err != nil {
			writeWorkCaseError(w, err)
			return
		}
		writeJSON(w, workCaseDetail{WorkCase: c, Links: links})

	case http.MethodPatch:
		h.handleWorkCasePatch(w, r, id)

	case http.MethodDelete:
		if !h.requireCommandBus(w) {
			return
		}
		wsID := r.URL.Query().Get("workspace_id")
		if wsID == "" {
			http.Error(w, "workspace_id query parameter is required", http.StatusBadRequest)
			return
		}
		wsPath, err := h.resolveWorkspacePath(wsID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		projectID, err := h.tasksStore.ProjectIDForPath(wsPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if projectID == "" {
			http.Error(w, "work case not found", http.StatusNotFound)
			return
		}
		if _, ok, err := h.workCaseStore.Get(id); err != nil {
			writeWorkCaseError(w, err)
			return
		} else if !ok {
			http.Error(w, "work case not found", http.StatusNotFound)
			return
		}
		mutationCtx, err := h.resolveMutationContext(r, projectID)
		if err != nil {
			writeMutationContextError(w, err)
			return
		}
		h.dispatchCaseCommand(w, r, meta.CommandWorkCaseDelete, mutationCtx, 0,
			id, requestIdempotencyKey(r, ""), map[string]any{"caseId": id})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleWorkCasePatch applies partial field edits via the workcase.update
// command. Status and currentPhase are rejected here outright: lifecycle
// moves go through /transition and phase advances only through /phase
// (#323: 禁止直接修改 Case phase).
func (h *Handler) handleWorkCasePatch(w http.ResponseWriter, r *http.Request, id string) {
	if !h.requireCommandBus(w) {
		return
	}
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	for _, banned := range []string{"currentPhase", "status"} {
		if _, present := raw[banned]; present {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": string(commandbus.CodeInvalidPayload),
				"message": banned + " cannot be changed through PATCH; use the " +
					"workcase.transition or workcase.set_phase command (/transition, /phase)",
			})
			return
		}
	}
	var head struct {
		ExpectedVersion int    `json:"expectedVersion"`
		IdempotencyKey  string `json:"idempotencyKey"`
	}
	if err := json.Unmarshal(raw["expectedVersion"], &head.ExpectedVersion); err != nil || head.ExpectedVersion <= 0 {
		http.Error(w, "expectedVersion is required", http.StatusBadRequest)
		return
	}
	current, ok, err := h.workCaseStore.Get(id)
	if err != nil {
		writeWorkCaseError(w, err)
		return
	}
	if !ok {
		http.Error(w, "work case not found", http.StatusNotFound)
		return
	}
	mutationCtx, err := h.resolveMutationContext(r, current.WorkspaceID)
	if err != nil {
		writeMutationContextError(w, err)
		return
	}
	delete(raw, "expectedVersion")
	delete(raw, "idempotencyKey")
	payload := map[string]any{"caseId": id}
	for k, v := range raw {
		payload[k] = json.RawMessage(v)
	}
	h.dispatchCaseCommand(w, r, meta.CommandWorkCaseUpdate, mutationCtx,
		head.ExpectedVersion, id, requestIdempotencyKey(r, head.IdempotencyKey), payload)
}

// handleWorkCaseTransition moves the case through the generic lifecycle
// (open/suspended/closed/cancelled) via the workcase.transition command.
// Terminal states never move again; terminal transitions require a user or
// human actor (关键人工决定不可被 Agent 静默覆盖).
func (h *Handler) handleWorkCaseTransition(w http.ResponseWriter, r *http.Request, id string) {
	if !h.requireCommandBus(w) {
		return
	}
	var body struct {
		Status          string `json:"status"`
		ExpectedVersion int    `json:"expectedVersion"`
		Reason          string `json:"reason"`
		IdempotencyKey  string `json:"idempotencyKey"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	to := meta.CaseStatus(body.Status)
	if !to.Valid() {
		http.Error(w, "status must be one of open|suspended|closed|cancelled", http.StatusBadRequest)
		return
	}
	if body.ExpectedVersion <= 0 {
		http.Error(w, "expectedVersion is required", http.StatusBadRequest)
		return
	}
	current, ok, err := h.workCaseStore.Get(id)
	if err != nil {
		writeWorkCaseError(w, err)
		return
	}
	if !ok {
		http.Error(w, "work case not found", http.StatusNotFound)
		return
	}
	mutationCtx, err := h.resolveMutationContext(r, current.WorkspaceID)
	if err != nil {
		writeMutationContextError(w, err)
		return
	}
	h.dispatchCaseCommand(w, r, meta.CommandWorkCaseTransition, mutationCtx,
		body.ExpectedVersion, id, requestIdempotencyKey(r, body.IdempotencyKey),
		map[string]any{"caseId": id, "status": body.Status, "reason": body.Reason})
}

// handleWorkCaseSetPhase is the ONLY HTTP path that changes currentPhase:
// the workcase.set_phase command under actor attribution, idempotency and
// optimistic concurrency (#323: 禁止直接修改 Case phase).
func (h *Handler) handleWorkCaseSetPhase(w http.ResponseWriter, r *http.Request, id string) {
	if !h.requireCommandBus(w) {
		return
	}
	var body struct {
		CurrentPhase    string `json:"currentPhase"`
		ExpectedVersion int    `json:"expectedVersion"`
		IdempotencyKey  string `json:"idempotencyKey"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.ExpectedVersion <= 0 {
		http.Error(w, "expectedVersion is required", http.StatusBadRequest)
		return
	}
	current, ok, err := h.workCaseStore.Get(id)
	if err != nil {
		writeWorkCaseError(w, err)
		return
	}
	if !ok {
		http.Error(w, "work case not found", http.StatusNotFound)
		return
	}
	mutationCtx, err := h.resolveMutationContext(r, current.WorkspaceID)
	if err != nil {
		writeMutationContextError(w, err)
		return
	}
	h.dispatchCaseCommand(w, r, meta.CommandWorkCaseSetPhase, mutationCtx,
		body.ExpectedVersion, id, requestIdempotencyKey(r, body.IdempotencyKey),
		map[string]any{"caseId": id, "currentPhase": body.CurrentPhase})
}

// handleWorkCaseLinkCreate links a task, session or artifact ref to the case
// via the workcase.link command.
func (h *Handler) handleWorkCaseLinkCreate(w http.ResponseWriter, r *http.Request, id string) {
	if !h.requireCommandBus(w) {
		return
	}
	var body struct {
		Kind            string `json:"kind"`
		TargetID        string `json:"targetId"`
		ExpectedVersion int    `json:"expectedVersion"`
		IdempotencyKey  string `json:"idempotencyKey"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.ExpectedVersion <= 0 {
		http.Error(w, "expectedVersion is required", http.StatusBadRequest)
		return
	}
	kind := meta.CaseLinkKind(body.Kind)
	if !kind.Valid() {
		http.Error(w, "kind must be one of task|session|artifact", http.StatusBadRequest)
		return
	}
	if body.TargetID == "" {
		http.Error(w, "targetId is required", http.StatusBadRequest)
		return
	}
	current, ok, err := h.workCaseStore.Get(id)
	if err != nil {
		writeWorkCaseError(w, err)
		return
	}
	if !ok {
		http.Error(w, "work case not found", http.StatusNotFound)
		return
	}
	mutationCtx, err := h.resolveMutationContext(r, current.WorkspaceID)
	if err != nil {
		writeMutationContextError(w, err)
		return
	}
	h.dispatchCaseCommand(w, r, meta.CommandWorkCaseLink, mutationCtx,
		body.ExpectedVersion, id, requestIdempotencyKey(r, body.IdempotencyKey),
		map[string]any{"caseId": id, "kind": body.Kind, "targetId": body.TargetID})
}

// handleWorkCaseLinkDelete removes one association edge from the case via
// the workcase.unlink command.
func (h *Handler) handleWorkCaseLinkDelete(w http.ResponseWriter, r *http.Request, id string) {
	if !h.requireCommandBus(w) {
		return
	}
	q := r.URL.Query()
	kind := meta.CaseLinkKind(q.Get("kind"))
	targetID := q.Get("target_id")
	if !kind.Valid() {
		http.Error(w, "kind must be one of task|session|artifact", http.StatusBadRequest)
		return
	}
	if targetID == "" {
		http.Error(w, "target_id is required", http.StatusBadRequest)
		return
	}
	expectedVersion, err := strconv.Atoi(q.Get("expected_version"))
	if err != nil || expectedVersion <= 0 {
		http.Error(w, "expected_version is required", http.StatusBadRequest)
		return
	}
	current, ok, err := h.workCaseStore.Get(id)
	if err != nil {
		writeWorkCaseError(w, err)
		return
	}
	if !ok {
		http.Error(w, "work case not found", http.StatusNotFound)
		return
	}
	mutationCtx, err := h.resolveMutationContext(r, current.WorkspaceID)
	if err != nil {
		writeMutationContextError(w, err)
		return
	}
	h.dispatchCaseCommand(w, r, meta.CommandWorkCaseUnlink, mutationCtx,
		expectedVersion, id, requestIdempotencyKey(r, ""),
		map[string]any{"caseId": id, "kind": string(kind), "targetId": targetID})
}

// handleWorkCaseTasks lists the tasks linked to the case (Task 可按 Case 查询).
func (h *Handler) handleWorkCaseTasks(w http.ResponseWriter, id string) {
	if _, ok, err := h.workCaseStore.Get(id); err != nil {
		writeWorkCaseError(w, err)
		return
	} else if !ok {
		http.Error(w, "work case not found", http.StatusNotFound)
		return
	}
	tasks, err := h.workCaseStore.ListTasksByCase(id)
	if err != nil {
		writeWorkCaseError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": tasks})
}

// handleWorkCaseRuns lists every execution/verification run of the case's
// tasks (TaskRun 可按 Case 查询).
func (h *Handler) handleWorkCaseRuns(w http.ResponseWriter, id string) {
	if _, ok, err := h.workCaseStore.Get(id); err != nil {
		writeWorkCaseError(w, err)
		return
	} else if !ok {
		http.Error(w, "work case not found", http.StatusNotFound)
		return
	}
	runs, err := h.workCaseStore.ListTaskRunsByCase(id)
	if err != nil {
		writeWorkCaseError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": runs})
}

// handleWorkCaseEvents lists the immutable domain audit events targeting the
// case.
func (h *Handler) handleWorkCaseEvents(w http.ResponseWriter, r *http.Request, id string) {
	if h.eventStore == nil {
		http.Error(w, "project event store is unavailable", http.StatusInternalServerError)
		return
	}
	current, ok, err := h.workCaseStore.Get(id)
	if err != nil {
		writeWorkCaseError(w, err)
		return
	}
	if !ok {
		http.Error(w, "work case not found", http.StatusNotFound)
		return
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	page, err := h.eventStore.List(meta.ProjectEventListOptions{
		ProjectID:  current.WorkspaceID,
		TargetType: "work_case",
		TargetID:   id,
		Limit:      limit,
	})
	if err != nil {
		writeWorkCaseError(w, err)
		return
	}
	writeJSON(w, page)
}

// handleWorkCaseCommandAudit lists the Command execution audit trail of the
// case: actor, command, target, result, error and duration of every dispatch
// attempt (#323 acceptance: Actor、Command、Case、结果和耗时可审计).
func (h *Handler) handleWorkCaseCommandAudit(w http.ResponseWriter, r *http.Request, id string) {
	if !h.requireCommandBus(w) {
		return
	}
	current, ok, err := h.workCaseStore.Get(id)
	if err != nil {
		writeWorkCaseError(w, err)
		return
	}
	if !ok {
		http.Error(w, "work case not found", http.StatusNotFound)
		return
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	items, err := h.commandBus.ListExecutions(commandbus.ExecutionFilter{
		WorkspaceID: current.WorkspaceID,
		TargetID:    id,
		Limit:       limit,
	})
	if err != nil {
		writeCommandError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

// writeWorkCaseError maps store/contract read errors to HTTP statuses:
// version conflicts and duplicate links → 409, invalid lifecycle moves and
// malformed refs → 400, project-scope violations → 403, missing → 404.
func writeWorkCaseError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, meta.ErrNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, meta.ErrProjectMismatch):
		http.Error(w, err.Error(), http.StatusForbidden)
	case errors.Is(err, meta.ErrCaseVersionConflict),
		errors.Is(err, meta.ErrDuplicate):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, meta.ErrInvalidCaseTransition),
		errors.Is(err, meta.ErrInvalidProjectEvent):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		if _, ok := domainref.CodeOf(err); ok {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
