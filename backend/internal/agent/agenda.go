package agent

import (
	"net/http"
	"time"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

// calendarItem is a normalized, source-agnostic entry for the personal agenda
// calendar (#192). The agenda unifies three sources across all workspaces —
// personal reminders, scheduled/recurring project tasks, and milestone target
// dates — into one shape the frontend renders and routes by `kind`.
type calendarItem struct {
	ID            string      `json:"id"`
	Kind          string      `json:"kind"` // reminder | task | milestone
	Title         string      `json:"title"`
	Date          time.Time   `json:"date"`    // anchor day (and time when HasTime)
	HasTime       bool        `json:"hasTime"` // Date carries a real time-of-day, not a fallback
	Status        string      `json:"status,omitempty"`
	Recurrence    *Recurrence `json:"recurrence,omitempty"`
	WorkspaceID   string      `json:"workspaceId"`
	WorkspaceName string      `json:"workspaceName,omitempty"`
	Number        int         `json:"number,omitempty"`
	Milestone     string      `json:"milestone,omitempty"`
	TaskType      string      `json:"taskType,omitempty"`
}

// HandleAgendaRoot serves GET /api/agent/agenda — the unified personal agenda
// aggregated across ALL workspaces. Scope is global by design, so it takes no
// workspace_id (unlike the per-workspace tasks/milestones endpoints).
func (h *Handler) HandleAgendaRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	projects, err := h.tasksStore.ListProjects()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items := []calendarItem{}
	for _, p := range projects {
		// Include active projects and system-owned workspaces (数据源同步 host).
		// Archived / killed / any other status stays out — those are dormant.
		if p.Status != meta.ProjectStatusActive && p.Status != meta.ProjectStatusSystem {
			continue
		}
		// Tasks → personal reminders + scheduled/recurring project tasks.
		if cfg, err := h.tasksStore.Load(p.WorkspacePath); err == nil {
			for i := range cfg.Tasks {
				if it, ok := taskToCalendarItem(&cfg.Tasks[i], p); ok {
					items = append(items, it)
				}
			}
		}
		// Milestones with a target date → date markers.
		if ms, err := h.tasksStore.ListMilestones(p.WorkspacePath); err == nil {
			for _, m := range ms {
				if m.TargetDate == nil {
					continue
				}
				items = append(items, calendarItem{
					ID:            m.ID,
					Kind:          "milestone",
					Title:         m.Name,
					Date:          *m.TargetDate,
					WorkspaceID:   p.ID,
					WorkspaceName: p.Name,
				})
			}
		}
	}
	writeJSON(w, items)
}

// taskToCalendarItem classifies one task into an agenda item, or returns
// ok=false when it doesn't belong on the calendar. Personal reminders
// (assignee=user) always appear; project tasks appear only when they carry a
// time anchor and are still open. Discussions and unadopted AI suggestions are
// never agenda items.
func taskToCalendarItem(t *Task, p Project) (calendarItem, bool) {
	if t.Type == TaskTypeDiscussion || t.Source == TaskSourceAgent {
		return calendarItem{}, false
	}
	// A human task (assignee=user or executor=human) is a personal to-do / gate
	// the user must act on — surface it on the agenda regardless of which field
	// declared it (one human lane).
	isReminder := isHumanTask(t)
	if isReminder {
		if t.Status == TaskStatusCancelled {
			return calendarItem{}, false
		}
	} else {
		if t.IssueState == IssueClosed {
			return calendarItem{}, false
		}
		if t.ScheduledAt == nil && t.Recurrence == nil && t.PlannedStart == nil {
			return calendarItem{}, false // a plain backlog task is not a calendar item
		}
	}
	// Anchor on the explicit schedule/plan time; fall back to creation day so an
	// undated personal todo still lands somewhere. Only scheduledAt is a real
	// clock time (reminders); plannedStart is a date-level PM planning anchor, so
	// it sets the day but not HasTime (avoids a noisy 00:00 on every project task).
	anchor := t.CreatedAt
	hasTime := false
	if t.ScheduledAt != nil {
		anchor = *t.ScheduledAt
		hasTime = true
	} else if t.PlannedStart != nil {
		anchor = *t.PlannedStart
	}
	kind := "task"
	if isReminder {
		kind = "reminder"
	}
	return calendarItem{
		ID:            t.ID,
		Kind:          kind,
		Title:         t.Title,
		Date:          anchor,
		HasTime:       hasTime,
		Status:        string(t.Status),
		Recurrence:    t.Recurrence,
		WorkspaceID:   p.ID,
		WorkspaceName: p.Name,
		Number:        t.Number,
		Milestone:     t.Milestone,
		TaskType:      string(t.Type),
	}, true
}
