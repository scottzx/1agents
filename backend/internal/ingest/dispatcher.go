package ingest

import (
	"fmt"
	"sort"
	"time"

	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/taskapi"
)

// taskDispatcher is the concrete Dispatcher: it turns sync requests into
// work-order function tasks and reads run history back from the tasks table. All
// timing (immediate + interval) lives in the work-order scheduler — this type
// only creates tasks, never ticks.
type taskDispatcher struct {
	api       *taskapi.API
	store     *meta.TaskStore
	workspace string // system host workspace path
}

// NewDispatcher builds a Dispatcher over the work-order API + task store, hosting
// tasks in workspacePath (the provisioned system workspace).
func NewDispatcher(api *taskapi.API, store *meta.TaskStore, workspacePath string) Dispatcher {
	return &taskDispatcher{api: api, store: store, workspace: workspacePath}
}

func (d *taskDispatcher) dispatch(source, kind, title, desc string) (string, error) {
	return d.api.DispatchTask("", taskapi.DispatchSpec{
		Title:         title,
		Description:   desc,
		Executor:      meta.TaskExecutorFunction,
		FunctionType:  "sources." + source + ".sync",
		BusinessRef:   businessRef(source, kind),
		WorkspacePath: d.workspace,
		Priority:      "medium",
	})
}

// SyncNow dispatches a one-off immediate sync task (no recurrence).
func (d *taskDispatcher) SyncNow(source, kind, collection string) (string, error) {
	return d.dispatch(source, kind, fmt.Sprintf("同步 %s（手动）", kind), "数据源手动增量同步。")
}

// EnsureRecurring makes sure exactly one live interval-recurring task exists for
// (source, kind). It is idempotent: if a non-terminal task with a recurrence
// rule already exists for the business_ref, it leaves it alone; otherwise it
// dispatches a new task and stamps the interval recurrence onto it.
func (d *taskDispatcher) EnsureRecurring(source, kind string, everyMinutes int) error {
	if everyMinutes < 1 {
		everyMinutes = 60
	}
	ref := businessRef(source, kind)
	existing, err := d.store.ListTasksByBusinessRef(ref)
	if err != nil {
		return err
	}
	for _, t := range existing {
		if t.Recurrence != nil && !isTerminal(t.Status) {
			return nil // already scheduled
		}
	}
	id, err := d.dispatch(source, kind, fmt.Sprintf("同步 %s（定时）", kind),
		fmt.Sprintf("数据源定时增量同步，每 %d 分钟一次。", everyMinutes))
	if err != nil {
		return err
	}
	// DispatchSpec carries no recurrence field, so stamp it on after creation. The
	// scheduler respawns the next instance on completion (nextOccurrence: interval).
	return d.store.Mutate(d.workspace, func(cfg *meta.TasksConfig) bool {
		for i := range cfg.Tasks {
			if cfg.Tasks[i].ID == id {
				cfg.Tasks[i].Recurrence = &meta.Recurrence{Freq: "interval", EveryMinutes: everyMinutes}
				return true
			}
		}
		return false
	})
}

// History returns prior sync runs for a source across all known kinds, newest
// first.
func (d *taskDispatcher) History(source string) ([]SyncRun, error) {
	var runs []SyncRun
	for _, kind := range knownKinds(source) {
		tasks, err := d.store.ListTasksByBusinessRef(businessRef(source, kind))
		if err != nil {
			return nil, err
		}
		for _, t := range tasks {
			runs = append(runs, SyncRun{
				TaskID:      t.ID,
				Kind:        kind,
				Status:      string(t.Status),
				Result:      t.Result,
				CreatedAt:   t.CreatedAt.Format(time.RFC3339),
				CompletedAt: formatPtr(t.CompletedAt),
			})
		}
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].CreatedAt > runs[j].CreatedAt })
	return runs, nil
}

// Schedules returns the live periodic-sync trigger state for a source, one row
// per kind that has any work-order task. The armed interval task (recurrence +
// non-terminal) supplies the current status + next trigger; the newest terminal
// task supplies the last-run summary.
func (d *taskDispatcher) Schedules(source string) ([]ScheduleRow, error) {
	var rows []ScheduleRow
	for _, kind := range knownKinds(source) {
		tasks, err := d.store.ListTasksByBusinessRef(businessRef(source, kind))
		if err != nil {
			return nil, err
		}
		if len(tasks) == 0 {
			continue
		}
		row := ScheduleRow{Kind: kind}
		var last *meta.Task
		for i := range tasks {
			t := &tasks[i]
			if t.Recurrence != nil && !isTerminal(t.Status) {
				row.Recurring = true
				row.Status = string(t.Status)
				row.NextRunAt = scheduleTrigger(t)
			}
			if isTerminal(t.Status) && (last == nil || t.CreatedAt.After(last.CreatedAt)) {
				last = t
			}
		}
		if last != nil {
			row.LastStatus = string(last.Status)
			if last.CompletedAt != nil {
				row.LastRunAt = last.CompletedAt.Format(time.RFC3339)
			} else {
				row.LastRunAt = last.CreatedAt.Format(time.RFC3339)
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// scheduleTrigger is a recurring task's next fire time: ScheduledAt, else the
// PlannedStart automation trigger (per meta.Task doc), else empty.
func scheduleTrigger(t *meta.Task) string {
	if t.ScheduledAt != nil {
		return t.ScheduledAt.Format(time.RFC3339)
	}
	if t.PlannedStart != nil {
		return t.PlannedStart.Format(time.RFC3339)
	}
	return ""
}

func isTerminal(s meta.TaskStatus) bool {
	return s == meta.TaskStatusCompleted || s == meta.TaskStatusFailed || s == meta.TaskStatusCancelled
}

func formatPtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}
