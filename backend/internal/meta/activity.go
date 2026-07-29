package meta

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// ProjectActivityTarget is the compact, privacy-safe object reference exposed
// by the activity projection. Event snapshots remain in the audit store.
type ProjectActivityTarget struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	Operation string `json:"operation"`
}

// ProjectActivityEntry deterministically groups mutation Events by Turn first,
// then correlation id, then a single Event.
type ProjectActivityEntry struct {
	ID            string                  `json:"id"`
	ProjectID     string                  `json:"projectId"`
	GroupKind     string                  `json:"groupKind"`
	TurnID        string                  `json:"turnId,omitempty"`
	CorrelationID string                  `json:"correlationId,omitempty"`
	SessionID     string                  `json:"sessionId,omitempty"`
	ActorKind     string                  `json:"actorKind"`
	ActorName     string                  `json:"actorName,omitempty"`
	Origin        string                  `json:"origin"`
	Status        ProjectEventStatus      `json:"status"`
	Summary       string                  `json:"summary"`
	Count         int                     `json:"count"`
	EventIDs      []string                `json:"eventIds"`
	Targets       []ProjectActivityTarget `json:"targets"`
	CreatedAt     time.Time               `json:"createdAt"`
}

type ProjectActivityListOptions struct {
	ProjectID        string
	SessionID        string
	TurnID           string
	TargetType       string
	TargetID         string
	Status           ProjectEventStatus
	Origin           string
	IncludeLifecycle bool
	Cursor           string
	Limit            int
}

type ProjectActivityPage struct {
	Items      []ProjectActivityEntry `json:"items"`
	NextCursor string                 `json:"nextCursor,omitempty"`
	HasMore    bool                   `json:"hasMore"`
}

type ProjectActivityStore struct {
	db *DB
}

func NewProjectActivityStore(db *DB) *ProjectActivityStore {
	return &ProjectActivityStore{db: db}
}

func (s *ProjectActivityStore) List(opts ProjectActivityListOptions) (ProjectActivityPage, error) {
	if opts.ProjectID == "" {
		return ProjectActivityPage{}, fmt.Errorf("%w: project_id is required", ErrInvalidProjectEvent)
	}
	limit, err := normalizePageLimit(opts.Limit)
	if err != nil {
		return ProjectActivityPage{}, err
	}
	cursor, err := decodeStoreCursor(opts.Cursor)
	if err != nil {
		return ProjectActivityPage{}, err
	}

	var query strings.Builder
	query.WriteString(`SELECT ` + projectEventCols + ` FROM project_events WHERE project_id = ?`)
	args := []any{opts.ProjectID}
	if !opts.IncludeLifecycle && opts.TargetType != "turn" {
		query.WriteString(` AND target_type != 'turn'`)
	}
	if opts.SessionID != "" {
		query.WriteString(` AND session_id = ?`)
		args = append(args, opts.SessionID)
	}
	if opts.TurnID != "" {
		query.WriteString(` AND turn_id = ?`)
		args = append(args, opts.TurnID)
	}
	if opts.TargetType != "" {
		query.WriteString(` AND target_type = ?`)
		args = append(args, opts.TargetType)
	}
	if opts.TargetID != "" {
		query.WriteString(` AND target_id = ?`)
		args = append(args, opts.TargetID)
	}
	if opts.Status != "" {
		query.WriteString(` AND status = ?`)
		args = append(args, opts.Status)
	}
	if opts.Origin != "" {
		query.WriteString(` AND origin = ?`)
		args = append(args, opts.Origin)
	}
	query.WriteString(` ORDER BY created_at DESC, id DESC`)

	rows, err := s.db.sql.Query(query.String(), args...)
	if err != nil {
		return ProjectActivityPage{}, err
	}
	defer rows.Close()

	groups := map[string]*ProjectActivityEntry{}
	for rows.Next() {
		event, err := scanProjectEvent(rows)
		if err != nil {
			return ProjectActivityPage{}, err
		}
		key, kind := activityGroupKey(event)
		entry := groups[key]
		if entry == nil {
			entry = &ProjectActivityEntry{
				ID:            key,
				ProjectID:     event.ProjectID,
				GroupKind:     kind,
				TurnID:        event.TurnID,
				CorrelationID: event.CorrelationID,
				SessionID:     event.SessionID,
				ActorKind:     event.ActorKind,
				ActorName:     event.ActorName,
				Origin:        event.Origin,
				Status:        event.Status,
				CreatedAt:     event.CreatedAt,
			}
			groups[key] = entry
		}
		entry.Count++
		entry.EventIDs = append(entry.EventIDs, event.ID)
		entry.Targets = append(entry.Targets, ProjectActivityTarget{
			Type: event.TargetType, ID: event.TargetID, Operation: event.Operation,
		})
		if event.CreatedAt.After(entry.CreatedAt) {
			entry.CreatedAt = event.CreatedAt
			entry.SessionID = event.SessionID
			entry.ActorKind = event.ActorKind
			entry.ActorName = event.ActorName
			entry.Origin = event.Origin
		}
		entry.Status = aggregateActivityStatus(entry.Status, event.Status)
	}
	if err := rows.Err(); err != nil {
		return ProjectActivityPage{}, err
	}

	items := make([]ProjectActivityEntry, 0, len(groups))
	for _, entry := range groups {
		entry.Summary = summarizeActivity(entry.Targets)
		items = append(items, *entry)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ID > items[j].ID
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	if cursor.At != "" {
		filtered := items[:0]
		for _, entry := range items {
			at := timeToStr(entry.CreatedAt)
			if at < cursor.At || (at == cursor.At && entry.ID < cursor.ID) {
				filtered = append(filtered, entry)
			}
		}
		items = filtered
	}

	page := ProjectActivityPage{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.HasMore = true
		last := page.Items[len(page.Items)-1]
		page.NextCursor = encodeStoreCursor(last.CreatedAt, last.ID)
	}
	return page, nil
}

func activityGroupKey(event ProjectEvent) (string, string) {
	if event.TurnID != "" {
		return "turn:" + event.TurnID, "turn"
	}
	if event.CorrelationID != "" {
		return "correlation:" + event.CorrelationID, "correlation"
	}
	return "event:" + event.ID, "event"
}

func aggregateActivityStatus(current, next ProjectEventStatus) ProjectEventStatus {
	if current == ProjectEventFailed || next == ProjectEventFailed {
		return ProjectEventFailed
	}
	if current == ProjectEventRejected || next == ProjectEventRejected {
		return ProjectEventRejected
	}
	return ProjectEventSucceeded
}

func summarizeActivity(targets []ProjectActivityTarget) string {
	if len(targets) == 0 {
		return "项目发生变更"
	}
	for _, target := range targets {
		if target.Type == "project_item" && target.Operation == "complete" {
			return "项目项已通过完成审计"
		}
	}
	for _, target := range targets {
		if target.Type == "verification" {
			if target.Operation == "fail" {
				return "TaskRun 核验未通过"
			}
			if target.Operation == "complete" {
				return "TaskRun 核验通过并完成"
			}
		}
	}
	for _, target := range targets {
		if target.Type != "task_run" {
			continue
		}
		switch target.Operation {
		case "complete":
			return "TaskRun 执行完成"
		case "fail":
			return "TaskRun 执行失败"
		case "cancel":
			return "TaskRun 已取消"
		case "start":
			return "TaskRun 已开始"
		}
	}
	first := targets[0]
	same := true
	for _, target := range targets[1:] {
		if target.Type != first.Type || target.Operation != first.Operation {
			same = false
			break
		}
	}
	if !same {
		return fmt.Sprintf("执行 %d 项项目操作", len(targets))
	}
	n := len(targets)
	switch first.Type + "." + first.Operation {
	case "project_item.create":
		if n == 1 {
			return "创建 1 个 Task"
		}
		return fmt.Sprintf("创建 %d 个 Tasks", n)
	case "project_item.update":
		return fmt.Sprintf("更新 %d 个项目项", n)
	case "project_item.close":
		return fmt.Sprintf("关闭 %d 个项目项", n)
	case "project_item.reopen":
		return fmt.Sprintf("重新打开 %d 个项目项", n)
	case "milestone.create":
		return fmt.Sprintf("创建 %d 个里程碑", n)
	case "milestone.update":
		return fmt.Sprintf("更新 %d 个里程碑", n)
	case "milestone.delete":
		return fmt.Sprintf("删除 %d 个里程碑", n)
	case "dependency.link":
		return fmt.Sprintf("新增 %d 条依赖", n)
	case "dependency.unlink":
		return fmt.Sprintf("移除 %d 条依赖", n)
	default:
		return fmt.Sprintf("执行 %d 项项目操作", n)
	}
}
