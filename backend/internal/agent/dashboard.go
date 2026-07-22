package agent

import (
	"net/http"
	"time"

	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/workspace"
)

// Company cockpit (公司驾驶舱) Phase 1 — read-only cross-project aggregation.
//
// This endpoint powers the dashboard's PMO overview: it摊开 every project on one
// board and surfaces blocked / stalled projects so they jump out without
// reading. It is strictly read-only — it reads existing task + session metadata
// and never mutates the task schema or any write path (issue #127).

// stalledThreshold is how long a running project may go without any session
// activity before it is considered 停滞 (stalled). Approximated via the most
// recent session last_event_at, since the REST session record carries no live
// running flag (see issue #127).
const stalledThreshold = 30 * time.Minute

// activeThreshold approximates "agent 正在干活" — a session whose last_event_at
// is within this window counts as live/on-station.
const activeThreshold = 5 * time.Minute

// ProjectHealth is the derived per-project status shown on the cockpit. It is a
// coarser, board-level rollup than per-task TaskStatus.
type ProjectHealth string

const (
	HealthRunning ProjectHealth = "running" // has a running task, recently active
	HealthBlocked ProjectHealth = "blocked" // a task is blocked or failed
	HealthStalled ProjectHealth = "stalled" // running but no recent session activity
	HealthDone    ProjectHealth = "done"    // all tasks completed (可发射)
	HealthIdle    ProjectHealth = "idle"    // no executable tasks / pending only
)

// DashboardProject is one project card's worth of aggregated, read-only data.
type DashboardProject struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	DefaultAgent string        `json:"defaultAgent,omitempty"`
	Health       ProjectHealth `json:"health"`
	// ProgressPercent is completed tasks / total executable tasks (0–100).
	ProgressPercent int `json:"progressPercent"`
	TotalTasks      int `json:"totalTasks"`
	CompletedTasks  int `json:"completedTasks"`
	RunningTasks    int `json:"runningTasks"`
	BlockedTasks    int `json:"blockedTasks"`
	// ActiveSessions is the count of on-station chat sessions (last_event_at
	// within activeThreshold). AgentSessions is the total non-auto session count.
	ActiveSessions int `json:"activeSessions"`
	AgentSessions  int `json:"agentSessions"`
	// LastEventAt is the most recent session activity across the project, used
	// for stalled detection and "last active" display. Zero when no sessions.
	LastEventAt time.Time `json:"lastEventAt,omitempty"`
}

// DashboardSummary is the top-of-screen company health HUD.
type DashboardSummary struct {
	TotalProjects   int `json:"totalProjects"`
	RunningProjects int `json:"runningProjects"`
	BlockedProjects int `json:"blockedProjects"` // blocked + stalled — the number最该看见
	DoneProjects    int `json:"doneProjects"`
	ActiveAgents    int `json:"activeAgents"`   // sessions on-station across all projects
	DeliveredTasks  int `json:"deliveredTasks"` // total completed tasks today across projects
}

// DashboardResponse is the full read-only payload for GET /api/agent/dashboard.
type DashboardResponse struct {
	Summary     DashboardSummary   `json:"summary"`
	Projects    []DashboardProject `json:"projects"`
	GeneratedAt time.Time          `json:"generatedAt"`
}

// HandleDashboard serves GET /api/agent/dashboard — the company cockpit Phase 1
// cross-project aggregate. Read-only: no schema or write-path changes.
func (h *Handler) HandleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	wsHandler := workspace.NewHandler()
	cfg, err := wsHandler.LoadWorkspacesConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp := buildDashboard(cfg.Workspaces, h.tasksStore, h.store, time.Now().UTC())
	writeJSON(w, resp)
}

// buildDashboard is the pure aggregation core, split out so it can be unit
// tested without on-disk workspace config. It tolerates per-workspace load
// errors (a workspace with no meta yet simply contributes an idle card).
func buildDashboard(workspaces []workspace.Workspace, tasksStore *meta.TaskStore, sessionStore *meta.SessionStore, now time.Time) DashboardResponse {
	projects := make([]DashboardProject, 0, len(workspaces))
	var summary DashboardSummary

	for _, ws := range workspaces {
		if ws.Builtin {
			continue
		}
		p := DashboardProject{
			ID:           ws.ID,
			Name:         ws.Name,
			DefaultAgent: ws.DefaultAgent,
		}

		// ── tasks ──
		if tasksStore != nil {
			if tc, err := tasksStore.Load(ws.Path); err == nil && tc != nil {
				for _, t := range tc.Tasks {
					// Only executable tasks count toward the board rollup;
					// requirements/bugs/discussions are issue items, not work.
					if t.Type == meta.ItemTypeRequirement || t.Type == meta.ItemTypeBug || t.Type == meta.ItemTypeDiscussion {
						continue
					}
					// AI suggestions stay out of the board until adopted.
					if t.Source == meta.TaskSourceAgent {
						continue
					}
					p.TotalTasks++
					switch t.Status {
					case meta.TaskStatusCompleted:
						p.CompletedTasks++
					case meta.TaskStatusRunning, meta.TaskStatusQueued, meta.TaskStatusPendingReview:
						p.RunningTasks++
					case meta.TaskStatusBlocked, meta.TaskStatusFailed:
						p.BlockedTasks++
					}
				}
			}
		}
		if p.TotalTasks > 0 {
			p.ProgressPercent = p.CompletedTasks * 100 / p.TotalTasks
		}

		// ── sessions (花名册 + 在岗) ──
		if sessionStore != nil {
			if recs, err := sessionStore.ListByWorkspace(ws.ID, false); err == nil {
				for _, rec := range recs {
					if rec.Role == SessionRoleAuto {
						continue
					}
					p.AgentSessions++
					if !rec.LastEventAt.IsZero() && now.Sub(rec.LastEventAt) < activeThreshold {
						p.ActiveSessions++
					}
					if rec.LastEventAt.After(p.LastEventAt) {
						p.LastEventAt = rec.LastEventAt
					}
				}
			}
		}

		p.Health = deriveHealth(p, now)
		projects = append(projects, p)

		// ── company HUD rollup ──
		summary.TotalProjects++
		summary.ActiveAgents += p.ActiveSessions
		summary.DeliveredTasks += p.CompletedTasks
		switch p.Health {
		case HealthRunning:
			summary.RunningProjects++
		case HealthBlocked, HealthStalled:
			summary.BlockedProjects++
		case HealthDone:
			summary.DoneProjects++
		}
	}

	sortProjectsBySalience(projects)
	return DashboardResponse{Summary: summary, Projects: projects, GeneratedAt: now}
}

// deriveHealth rolls a project's task + session state into one board-level
// health. Order matters: blocked/failed dominates, then done, then the
// running-vs-stalled split based on recent session activity.
func deriveHealth(p DashboardProject, now time.Time) ProjectHealth {
	if p.BlockedTasks > 0 {
		return HealthBlocked
	}
	if p.TotalTasks > 0 && p.CompletedTasks == p.TotalTasks {
		return HealthDone
	}
	if p.RunningTasks > 0 {
		// Running tasks but no recent session heartbeat ⇒ stalled (停滞).
		if p.LastEventAt.IsZero() || now.Sub(p.LastEventAt) >= stalledThreshold {
			return HealthStalled
		}
		return HealthRunning
	}
	return HealthIdle
}

// healthSalience ranks health so the cards that 最该看见 float to the top:
// blocked > stalled > running > done > idle.
func healthSalience(hh ProjectHealth) int {
	switch hh {
	case HealthBlocked:
		return 0
	case HealthStalled:
		return 1
	case HealthRunning:
		return 2
	case HealthDone:
		return 3
	default:
		return 4
	}
}

// sortProjectsBySalience puts blocking / stalled projects on top (the cockpit's
// first job), then by recent activity, then by name for stable ordering.
func sortProjectsBySalience(projects []DashboardProject) {
	// insertion sort keeps it dependency-free and is fine for project counts.
	for i := 1; i < len(projects); i++ {
		j := i
		for j > 0 && lessSalient(projects[j], projects[j-1]) {
			projects[j], projects[j-1] = projects[j-1], projects[j]
			j--
		}
	}
}

func lessSalient(a, b DashboardProject) bool {
	sa, sb := healthSalience(a.Health), healthSalience(b.Health)
	if sa != sb {
		return sa < sb
	}
	if !a.LastEventAt.Equal(b.LastEventAt) {
		return a.LastEventAt.After(b.LastEventAt)
	}
	return a.Name < b.Name
}
