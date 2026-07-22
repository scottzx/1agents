package meta

import (
	"fmt"
	"strings"
	"time"
)

// PMOStore is the PMO 跨项目对话式需求分发层 (#61): the layer that sits between
// Inbox 收口 (#60) and the per-project requirement pool. Where Inbox is passive
// intake, the PMO is the cross-project dispatcher — it takes a clarified idea
// (often an inbox item) and writes it into a specific project's requirement pool
// as a
// tasks.type = requirement card. From there the project's AI Project Manager
// (#49/#50) schedules it. The PMO never schedules; it only dispatches.
//
// This adds NO new task schema. A dispatched requirement is a normal Task with
// Type=requirement living in the target project, carrying a dispatched-from
// backlink label to its originating inbox_item so the "what did this become"
// trail survives (Inbox #60 items have no task row to Links-target). Routing to
// a project reuses the project registry: the caller names a projectID, and the
// requirement is written into that project's workspace through the shared
// TaskStore.
type PMOStore struct {
	tasks *TaskStore
	inbox *InboxStore
}

// NewPMOStore returns a PMOStore sharing the given task store (so #N numbering
// and the per-workspace write lock are reused) and inbox store (so a dispatched
// item is marked read, closing the intake loop).
func NewPMOStore(tasks *TaskStore, inbox *InboxStore) *PMOStore {
	return &PMOStore{tasks: tasks, inbox: inbox}
}

// dispatchedFromLabel records the originating inbox_item id on a requirement
// dispatched out of the Inbox, since Inbox #60 items have no task row to
// Links-target. Format: "dispatched-from:<inboxItemID>".
const dispatchedFromLabel = "dispatched-from:"

// DispatchResult is what Dispatch returns: the requirement card as it now lives
// in the target project, plus the resolved target project for the caller's
// convenience (the PM that will schedule it).
type DispatchResult struct {
	Requirement Task    `json:"requirement"`
	Project     Project `json:"project"`
}

// Dispatch writes a requirement into projectID's requirement pool — the core
// PMO 分发 step (#61 Phase A). title is required. description carries the body /
// 验收说明 the PMO clarified in conversation. fromInbox, when set, is the
// originating inbox_item id: it is stamped as a dispatched-from backlink label,
// and the inbox item is flipped to read so it leaves the unread queue (the
// intake → dispatch loop closes). Returns ErrNotFound when projectID is unknown.
func (s *PMOStore) Dispatch(projectID, title, description, priority, fromInbox string) (DispatchResult, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return DispatchResult{}, fmt.Errorf("meta: requirement title is required")
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return DispatchResult{}, fmt.Errorf("meta: target projectID is required")
	}
	if projectID == PersonalProjectID {
		// A legacy personal bucket (removed feature) is never a dispatch target.
		return DispatchResult{}, fmt.Errorf("meta: cannot dispatch into the personal bucket")
	}

	project, ok, err := s.tasks.db.GetProject(projectID)
	if err != nil {
		return DispatchResult{}, err
	}
	if !ok {
		return DispatchResult{}, ErrNotFound
	}

	now := time.Now().UTC()
	req := Task{
		ID:           newID(),
		Title:        title,
		Description:  description,
		Type:         ItemTypeRequirement,
		Priority:     Priority(priority),
		IssueState:   IssueOpen,
		Status:       TaskStatusPending,
		ScheduleType: ScheduleTypeImmediate,
		CreatedBy:    "agent", // dispatched by the PMO, not hand-built by the user
		CreatedAt:    now,
		UpdatedAt:    now,
		Replies:      []Reply{},
		Sessions:     []SessionMetadata{},
	}
	if ref := strings.TrimSpace(fromInbox); ref != "" {
		req.Labels = append(req.Labels, dispatchedFromLabel+ref)
	}

	if err := s.tasks.Mutate(project.WorkspacePath, func(cfg *TasksConfig) bool {
		cfg.Tasks = append(cfg.Tasks, req)
		return true
	}); err != nil {
		return DispatchResult{}, err
	}

	// Close the intake loop: a dispatched inbox item leaves the unread queue.
	// Best-effort — a missing item must not fail the dispatch (the requirement
	// is already written).
	if ref := strings.TrimSpace(fromInbox); ref != "" && s.inbox != nil {
		if _, err := s.inbox.SetStatus(ref, InboxStatusRead); err != nil && err != ErrNotFound {
			return DispatchResult{}, err
		}
	}

	saved, _, err := s.tasks.GetTask(req.ID)
	if err != nil {
		return DispatchResult{}, err
	}
	return DispatchResult{Requirement: saved, Project: project}, nil
}

// DispatchTarget is one selectable project the PMO can dispatch into.
type DispatchTarget struct {
	ProjectID     string `json:"projectId"`
	Name          string `json:"name"`
	WorkspacePath string `json:"workspacePath"`
}

// Targets lists the projects the PMO may dispatch requirements into: every
// active project (a legacy personal bucket, if present, is excluded). This is
// the cross-project menu the PMO conversation picks a target from.
func (s *PMOStore) Targets() ([]DispatchTarget, error) {
	projects, err := s.tasks.db.ListProjectsByStatus(ProjectStatusActive)
	if err != nil {
		return nil, err
	}
	out := []DispatchTarget{}
	for _, p := range projects {
		if p.ID == PersonalProjectID {
			continue
		}
		out = append(out, DispatchTarget{
			ProjectID:     p.ID,
			Name:          p.Name,
			WorkspacePath: p.WorkspacePath,
		})
	}
	return out, nil
}
