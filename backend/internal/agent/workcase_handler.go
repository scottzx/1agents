package agent

// WorkCase REST surface (task #322, docs/architecture/enterprise-foundation-v1.0.0.md
// §4.3). WorkCase is the kernel-owned long-running business matter between
// domain objects and Tasks:
//
//	GET    /api/agent/work-cases?workspace_id=X[&status=S]   list
//	POST   /api/agent/work-cases                             create
//	GET    /api/agent/work-cases/{id}                        detail incl. links
//	PATCH  /api/agent/work-cases/{id}                        partial edit (expectedVersion)
//	DELETE /api/agent/work-cases/{id}?workspace_id=X         delete
//	POST   /api/agent/work-cases/{id}/transition             lifecycle move (expectedVersion)
//	POST   /api/agent/work-cases/{id}/links                  link task|session|artifact
//	DELETE /api/agent/work-cases/{id}/links?kind=&target_id=&expected_version=
//	GET    /api/agent/work-cases/{id}/tasks                  tasks by Case
//	GET    /api/agent/work-cases/{id}/runs                   task runs by Case
//	GET    /api/agent/work-cases/{id}/events                 audit events targeting the Case
//
// Every mutation requires expectedVersion (optimistic concurrency, §5.1) and
// appends an immutable ProjectEvent (target_type "work_case") in the same
// transaction as the state change.

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/scottzx/1Agents/backend/internal/domainref"
	"github.com/scottzx/1Agents/backend/internal/meta"
)

// caseEventSnapshot is the event after-state recorded for WorkCase mutations.
func caseEventSnapshot(c meta.WorkCase) map[string]any {
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

// requireWorkCaseStore answers 503-style (500 here, matching the task-run
// store pattern) when the meta DB was unavailable at handler construction.
func (h *Handler) requireWorkCaseStore(w http.ResponseWriter) bool {
	if h.workCaseStore == nil {
		http.Error(w, "work case store is unavailable", http.StatusInternalServerError)
		return false
	}
	return true
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
		var body struct {
			WorkspaceID     string                 `json:"workspace_id"`
			Title           string                 `json:"title"`
			Objective       string                 `json:"objective"`
			CaseDefinition  string                 `json:"caseDefinition"`
			Owner           string                 `json:"owner"`
			CreatedBy       string                 `json:"createdBy"`
			CurrentPhase    string                 `json:"currentPhase"`
			PrimarySubject  string                 `json:"primarySubject"`
			SubjectRefs     []string               `json:"subjectRefs"`
			Participants    []meta.CaseParticipant `json:"participants"`
			ExpectedCloseAt *time.Time             `json:"expectedCloseAt"`
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
		newCase := meta.WorkCase{
			ID:              newID(),
			Title:           body.Title,
			Objective:       body.Objective,
			CaseDefinition:  body.CaseDefinition,
			Owner:           body.Owner,
			CreatedBy:       body.CreatedBy,
			CurrentPhase:    body.CurrentPhase,
			PrimarySubject:  body.PrimarySubject,
			SubjectRefs:     body.SubjectRefs,
			Participants:    body.Participants,
			ExpectedCloseAt: body.ExpectedCloseAt,
		}
		if newCase.CreatedBy == "" {
			newCase.CreatedBy = mutationCtx.ActorName
		}
		event := mutationEvent(mutationCtx, "work_case", newCase.ID, "create", nil, nil)
		created, err := h.workCaseStore.Create(projectID, newCase, event)
		if err != nil {
			writeWorkCaseError(w, err)
			return
		}
		event.After, _ = json.Marshal(caseEventSnapshot(created))
		writeMutationJSON(w, created, event)

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
		event := mutationEvent(mutationCtx, "work_case", id, "delete", nil, nil)
		if err := h.workCaseStore.Delete(projectID, id, event); err != nil {
			writeWorkCaseError(w, err)
			return
		}
		writeMutationJSON(w, map[string]any{"ok": true}, event)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleWorkCasePatch applies partial field edits under optimistic
// concurrency. Status is never changed here — lifecycle moves go through
// /transition.
func (h *Handler) handleWorkCasePatch(w http.ResponseWriter, r *http.Request, id string) {
	var body struct {
		ExpectedVersion int                     `json:"expectedVersion"`
		Title           *string                 `json:"title,omitempty"`
		Objective       *string                 `json:"objective,omitempty"`
		CaseDefinition  *string                 `json:"caseDefinition,omitempty"`
		Owner           *string                 `json:"owner,omitempty"`
		CurrentPhase    *string                 `json:"currentPhase,omitempty"`
		PrimarySubject  *string                 `json:"primarySubject,omitempty"`
		SubjectRefs     *[]string               `json:"subjectRefs,omitempty"`
		Participants    *[]meta.CaseParticipant `json:"participants,omitempty"`
		ExpectedCloseAt *time.Time              `json:"expectedCloseAt,omitempty"`
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
	before := caseEventSnapshot(current)
	event := mutationEvent(mutationCtx, "work_case", id, "update", before, nil)
	updated, err := h.workCaseStore.Update(current.WorkspaceID, id, meta.WorkCasePatch{
		Title:           body.Title,
		Objective:       body.Objective,
		CaseDefinition:  body.CaseDefinition,
		Owner:           body.Owner,
		CurrentPhase:    body.CurrentPhase,
		PrimarySubject:  body.PrimarySubject,
		SubjectRefs:     body.SubjectRefs,
		Participants:    body.Participants,
		ExpectedCloseAt: body.ExpectedCloseAt,
	}, body.ExpectedVersion, event)
	if err != nil {
		writeWorkCaseError(w, err)
		return
	}
	event.After, _ = json.Marshal(caseEventSnapshot(updated))
	writeMutationJSON(w, updated, event)
}

// handleWorkCaseTransition moves the case through the generic lifecycle
// (open/suspended/closed/cancelled). Terminal states never move again.
func (h *Handler) handleWorkCaseTransition(w http.ResponseWriter, r *http.Request, id string) {
	var body struct {
		Status          string `json:"status"`
		ExpectedVersion int    `json:"expectedVersion"`
		Reason          string `json:"reason"`
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
	event := mutationEvent(mutationCtx, "work_case", id, "transition",
		map[string]any{"status": current.Status, "version": current.Version},
		map[string]any{"status": to, "reason": body.Reason})
	updated, err := h.workCaseStore.Transition(
		current.WorkspaceID, id, to, body.Reason, body.ExpectedVersion, event)
	if err != nil {
		writeWorkCaseError(w, err)
		return
	}
	event.After, _ = json.Marshal(caseEventSnapshot(updated))
	writeMutationJSON(w, updated, event)
}

// handleWorkCaseLinkCreate links a task, session or artifact ref to the case.
func (h *Handler) handleWorkCaseLinkCreate(w http.ResponseWriter, r *http.Request, id string) {
	var body struct {
		Kind            string `json:"kind"`
		TargetID        string `json:"targetId"`
		ExpectedVersion int    `json:"expectedVersion"`
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
	event := mutationEvent(mutationCtx, "work_case", id, "link", nil,
		map[string]any{"kind": kind, "targetId": body.TargetID})
	updated, err := h.workCaseStore.Link(
		current.WorkspaceID, id, kind, body.TargetID, body.ExpectedVersion, event)
	if err != nil {
		writeWorkCaseError(w, err)
		return
	}
	event.After, _ = json.Marshal(caseEventSnapshot(updated))
	writeMutationJSON(w, updated, event)
}

// handleWorkCaseLinkDelete removes one association edge from the case.
func (h *Handler) handleWorkCaseLinkDelete(w http.ResponseWriter, r *http.Request, id string) {
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
	event := mutationEvent(mutationCtx, "work_case", id, "unlink", nil,
		map[string]any{"kind": kind, "targetId": targetID})
	updated, err := h.workCaseStore.Unlink(
		current.WorkspaceID, id, kind, targetID, expectedVersion, event)
	if err != nil {
		writeWorkCaseError(w, err)
		return
	}
	event.After, _ = json.Marshal(caseEventSnapshot(updated))
	writeMutationJSON(w, updated, event)
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

// handleWorkCaseEvents lists the immutable audit events targeting the case.
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

// writeWorkCaseError maps store/contract errors to HTTP statuses:
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
