package agent

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// Discussion 决策层 (#189). A Discussion is the decision layer that sits between
// Inbox 收口 (#60) and PMO 分发 (#61): a free-form conversation thread that may
// carry 0..N cards (L2 deep-dive products / agent-suggested cards #47). The card
// is context enrichment, not a precondition — a cardless discussion is just an
// idea conversation that the thread itself is refining.
//
// This layer adds NO new task schema: a Discussion is a Task with
// Type=discussion, cards attach as Links{Rel: relates} to other tasks, and a
// conclusion routes to a requirement / temp Task created through the existing
// task create path — always with a backlink (Links{Rel: relates}) to the
// discussion and any originating inbox_item. The #133 task-orchestration schema
// is deliberately untouched.

// discussionCardLabel prefixes a label that records a non-task card source on a
// discussion (e.g. an inbox_item id that has no task row yet, since Inbox #60 is
// not merged). Task-row cards are recorded as Links instead. Format:
// "discussion-card:<sourceRef>".
const discussionCardLabel = "discussion-card:"

// concludedFromLabel marks a requirement/task spawned out of a discussion
// conclusion, recording the originating inbox_item ref when there is no task row
// to link to. Format: "concluded-from:<sourceRef>".
const concludedFromLabel = "concluded-from:"

// attachCard wires a card onto a discussion. When sourceTaskID names an existing
// task in the workspace (an L2 deep-dive product or an agent-suggested card #47),
// the card is a bidirectional relates link. When it is empty but sourceRef is
// set, the card is an external (inbox_item) reference recorded as a label, since
// that source has no task row yet. Returns false if the discussion is not found.
func attachCard(cfg *TasksConfig, discussionID, sourceTaskID, sourceRef string) bool {
	disc := findTask(cfg, discussionID)
	if disc == nil {
		return false
	}
	now := time.Now().UTC()
	if sourceTaskID != "" {
		card := findTask(cfg, sourceTaskID)
		if card == nil {
			return false
		}
		addLink(disc, TaskLink{Target: sourceTaskID, Rel: LinkRelates})
		addLink(card, TaskLink{Target: discussionID, Rel: LinkRelates})
		card.UpdatedAt = now
	} else if sourceRef != "" {
		addLabel(disc, discussionCardLabel+sourceRef)
	} else {
		return false
	}
	disc.UpdatedAt = now
	return true
}

// findTask returns a pointer to the task with id in cfg, or nil.
func findTask(cfg *TasksConfig, id string) *Task {
	for i := range cfg.Tasks {
		if cfg.Tasks[i].ID == id {
			return &cfg.Tasks[i]
		}
	}
	return nil
}

// addLink appends l to t.Links unless an identical (target, rel) pair is already
// present (idempotent — attaching the same card twice is a no-op).
func addLink(t *Task, l TaskLink) {
	for _, existing := range t.Links {
		if existing.Target == l.Target && existing.Rel == l.Rel {
			return
		}
	}
	t.Links = append(t.Links, l)
}

// addLabel appends label to t.Labels unless already present.
func addLabel(t *Task, label string) {
	for _, existing := range t.Labels {
		if existing == label {
			return
		}
	}
	t.Labels = append(t.Labels, label)
}

// ── HTTP handlers ───────────────────────────────────────────────────────────

// HandleDiscussionsRoot serves POST /api/agent/discussions — create a discussion
// thread. Both #189 paths route here:
//
//   - 有卡: pass sourceTaskId (an inbox-item/agent-suggested task row) and/or
//     sourceRef (an external inbox_item id with no task row yet); the card is
//     attached on creation.
//   - 无卡: omit both — a boss-initiated idea thread the discussion refines.
//
// The discussion is a Task{Type: discussion}, created through the same store the
// PM tool uses (no scheduling — the scheduler skips discussions).
func (h *Handler) HandleDiscussionsRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		WorkspaceID  string `json:"workspace_id"`
		Title        string `json:"title"`
		Description  string `json:"description"`
		SourceTaskID string `json:"sourceTaskId"` // existing task row to attach as a card
		SourceRef    string `json:"sourceRef"`    // external inbox_item id (no task row)
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.WorkspaceID == "" || strings.TrimSpace(body.Title) == "" {
		http.Error(w, "workspace_id and title are required", http.StatusBadRequest)
		return
	}
	wsPath, err := h.resolveWorkspacePath(body.WorkspaceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	now := time.Now().UTC()
	disc := Task{
		ID:           newID(),
		Title:        strings.TrimSpace(body.Title),
		Description:  body.Description,
		Type:         TaskTypeDiscussion,
		IssueState:   IssueOpen,
		Status:       TaskStatusPending,
		ScheduleType: ScheduleTypeImmediate,
		CreatedAt:    now,
		UpdatedAt:    now,
		Replies:      []Reply{},
		Sessions:     []SessionMetadata{},
	}

	if err := h.tasksStore.Mutate(wsPath, func(cfg *TasksConfig) bool {
		cfg.Tasks = append(cfg.Tasks, disc)
		if body.SourceTaskID != "" || body.SourceRef != "" {
			// attachCard wires the card onto the just-appended discussion.
			attachCard(cfg, disc.ID, body.SourceTaskID, body.SourceRef)
		}
		return true
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	saved, _, _ := h.tasksStore.GetTask(disc.ID)
	writeJSON(w, saved)
}

// HandleDiscussionItem serves the discussion sub-resources:
//
//	POST /api/agent/discussions/{id}/cards     → attach a card (inbox item / #47 suggestion)
//	POST /api/agent/discussions/{id}/conclude  → 拍板: spawn requirement / temp Task
func (h *Handler) HandleDiscussionItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/agent/discussions/")
	id, action, ok := strings.Cut(rest, "/")
	if !ok || id == "" {
		http.Error(w, "discussion id and action are required", http.StatusBadRequest)
		return
	}
	switch action {
	case "cards":
		h.handleDiscussionAttachCard(w, r, id)
	case "conclude":
		h.handleDiscussionConclude(w, r, id)
	default:
		http.Error(w, "unknown discussion action: "+action, http.StatusNotFound)
	}
}

// handleDiscussionAttachCard attaches a card to an existing discussion — the
// path that brings an agent-suggested card (#47) into a discussion, or wires an
// inbox item / L2 deep-dive product on after the thread already exists.
func (h *Handler) handleDiscussionAttachCard(w http.ResponseWriter, r *http.Request, id string) {
	var body struct {
		SourceTaskID string `json:"sourceTaskId"`
		SourceRef    string `json:"sourceRef"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.SourceTaskID == "" && body.SourceRef == "" {
		http.Error(w, "sourceTaskId or sourceRef is required", http.StatusBadRequest)
		return
	}
	disc, ok, err := h.tasksStore.GetTask(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "discussion not found", http.StatusNotFound)
		return
	}
	if disc.Type != TaskTypeDiscussion {
		http.Error(w, "task is not a discussion", http.StatusBadRequest)
		return
	}
	attached := false
	if err := h.tasksStore.Mutate(disc.WorkspacePath, func(cfg *TasksConfig) bool {
		attached = attachCard(cfg, id, body.SourceTaskID, body.SourceRef)
		return attached
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !attached {
		http.Error(w, "card source not found in this workspace", http.StatusNotFound)
		return
	}
	saved, _, _ := h.tasksStore.GetTask(id)
	writeJSON(w, saved)
}

// handleDiscussionConclude turns a discussion conclusion into a requirement or a
// temp Task (#189 拍板出口), with a backlink to the discussion and any
// originating inbox_item. The new item is created through the same store as the
// PM/create path — no task schema change. Pass close=true to close the
// discussion once its conclusion is captured.
func (h *Handler) handleDiscussionConclude(w http.ResponseWriter, r *http.Request, id string) {
	var body struct {
		Title              string `json:"title"`
		Description        string `json:"description"`
		AcceptanceCriteria string `json:"acceptanceCriteria"`
		Type               string `json:"type"`      // requirement | task | bug
		Priority           string `json:"priority"`  // optional
		Assignee           string `json:"assignee"`  // optional executing agent type (temp Task)
		SourceRef          string `json:"sourceRef"` // originating inbox_item id, for the backlink label
		Close              bool   `json:"close"`     // close the discussion after concluding
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(body.Title) == "" {
		http.Error(w, "title is required", http.StatusBadRequest)
		return
	}
	// A conclusion routes only to the requirement pool or a temp Task — never a
	// discussion/bug-suggestion variant. Default to requirement (the work-domain
	// route from RFC §3.2); "task" produces a temp Task.
	typ := TaskType(body.Type)
	if typ == "" {
		typ = TaskTypeRequirement
	}
	if typ != TaskTypeRequirement && typ != TaskTypeTask && typ != TaskTypeBug {
		http.Error(w, "type must be requirement, task or bug", http.StatusBadRequest)
		return
	}
	if body.Assignee != "" && body.Assignee != AssigneeUser && !IsSupportedAgentType(body.Assignee) {
		http.Error(w, "unknown assignee agent type: "+body.Assignee, http.StatusBadRequest)
		return
	}

	disc, ok, err := h.tasksStore.GetTask(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "discussion not found", http.StatusNotFound)
		return
	}
	if disc.Type != TaskTypeDiscussion {
		http.Error(w, "task is not a discussion", http.StatusBadRequest)
		return
	}

	now := time.Now().UTC()
	spawned := Task{
		ID:                 newID(),
		Title:              strings.TrimSpace(body.Title),
		Description:        body.Description,
		AcceptanceCriteria: body.AcceptanceCriteria,
		Type:               typ,
		Priority:           Priority(body.Priority),
		Assignee:           body.Assignee,
		IssueState:         IssueOpen,
		Status:             TaskStatusPending,
		ScheduleType:       ScheduleTypeImmediate,
		// Backlink to the source discussion (#189: 带回链到来源 discussion).
		Links:     []TaskLink{{Target: id, Rel: LinkRelates}},
		CreatedAt: now,
		UpdatedAt: now,
		Replies:   []Reply{},
		Sessions:  []SessionMetadata{},
	}
	// Record the originating inbox_item ref (no task row to link to yet, Inbox
	// #60 unmerged) as a label backlink.
	if body.SourceRef != "" {
		spawned.Labels = append(spawned.Labels, concludedFromLabel+body.SourceRef)
	}

	if err := h.tasksStore.Mutate(disc.WorkspacePath, func(cfg *TasksConfig) bool {
		cfg.Tasks = append(cfg.Tasks, spawned)
		// Backlink the other way: the discussion relates to its outcome, and is
		// closed when requested (the decision is captured, the thread is done).
		d := findTask(cfg, id)
		if d != nil {
			addLink(d, TaskLink{Target: spawned.ID, Rel: LinkRelates})
			if body.Close {
				d.IssueState = IssueClosed
			}
			d.UpdatedAt = now
		}
		return true
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	saved, _, _ := h.tasksStore.GetTask(spawned.ID)
	writeJSON(w, saved)
}
