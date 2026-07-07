package data

import (
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// gold_promote.go activates the dormant todos.linked_task_id: it turns a fused
// external to-do into a task (an agent work-order or a personal human todo). The
// to-do stays the source mirror; the task is the execution instance; the two are
// linked so a re-fuse never re-promotes. The task body is built from the FULL
// field-complete silver row (title/body/due/importance/categories +
// recurrence_std/checklist_items), matching the superset principle.

// PromotableTodo is a gold to-do resolved for promotion: its identity + already-
// linked task (if any) + the task-create body assembled from its silver fields.
type PromotableTodo struct {
	ID           string
	Source       string
	ExternalID   string
	LinkedTaskID string         // non-empty when already promoted (idempotency)
	Body         map[string]any // POST /api/agent/tasks body (minus workspace_id/assignee)
}

// TodoForPromotion loads a gold to-do by id, reads its field-complete silver
// row, and assembles the task-create body (identity fields set by the caller).
// Returns ok=false when the gold row is missing.
func (s *Store) TodoForPromotion(id string) (PromotableTodo, bool, error) {
	var source, externalID, linked string
	err := s.sql.QueryRow(
		`SELECT source, external_id, linked_task_id FROM todos WHERE id = ?`, id).
		Scan(&source, &externalID, &linked)
	if err == sql.ErrNoRows {
		return PromotableTodo{}, false, nil
	}
	if err != nil {
		return PromotableTodo{}, false, err
	}
	_, m, err := s.silverRowFields(silverTableFor("todos", source), externalID, "")
	if err != nil {
		return PromotableTodo{}, false, err
	}
	return PromotableTodo{
		ID:           id,
		Source:       source,
		ExternalID:   externalID,
		LinkedTaskID: linked,
		Body:         buildPromotionBody(m),
	}, true, nil
}

// LinkTodoTask records the promoted task id on the gold to-do (the promote-back
// write that keeps a re-fuse from re-promoting — UpsertTodos preserves it).
func (s *Store) LinkTodoTask(id, taskID string) error {
	_, err := s.sql.Exec(`UPDATE todos SET linked_task_id = ? WHERE id = ?`, taskID, id)
	return err
}

// buildPromotionBody maps a silver to-do row (raw column map) to a task-create
// body. Superset carry-over: title, body→description, importance→priority,
// categories→labels, due_at→scheduledAt, recurrence_std→recurrence,
// checklist_items→checklist.
func buildPromotionBody(m map[string]string) map[string]any {
	body := map[string]any{
		"title":       m["title"],
		"description": m["body"],
		"priority":    mapImportance(m["importance"]),
		"type":        "task",
	}
	if labels := parseStringArray(m["categories"]); len(labels) > 0 {
		body["labels"] = labels
	}
	if due := epochMsToRFC3339(m["due_at"]); due != "" {
		body["scheduleType"] = "scheduled"
		body["scheduledAt"] = due
	}
	if std := strings.TrimSpace(m["recurrence_std"]); std != "" && std != "null" {
		body["recurrence"] = json.RawMessage(std)
	}
	if cl := convertChecklist(m["checklist_items"]); len(cl) > 0 {
		body["checklist"] = cl
	}
	return body
}

// mapImportance maps MS To Do importance to our task priority. Unknown → medium.
func mapImportance(imp string) string {
	switch strings.ToLower(strings.TrimSpace(imp)) {
	case "high":
		return "high"
	case "low":
		return "low"
	default:
		return "medium"
	}
}

func parseStringArray(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" || s == "[]" {
		return nil
	}
	var out []string
	if json.Unmarshal([]byte(s), &out) != nil {
		return nil
	}
	return out
}

// epochMsToRFC3339 converts an epoch-millisecond string to RFC3339, or "" when
// empty/zero/unparseable.
func epochMsToRFC3339(s string) string {
	ms, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
}

// convertChecklist maps MS Graph checklistItems ([{displayName,isChecked}]) to
// our ChecklistItem shape ([{text,done}]).
func convertChecklist(s string) []map[string]any {
	s = strings.TrimSpace(s)
	if s == "" || s == "[]" {
		return nil
	}
	var items []struct {
		DisplayName string `json:"displayName"`
		IsChecked   bool   `json:"isChecked"`
	}
	if json.Unmarshal([]byte(s), &items) != nil {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		if strings.TrimSpace(it.DisplayName) == "" {
			continue
		}
		out = append(out, map[string]any{"text": it.DisplayName, "done": it.IsChecked})
	}
	return out
}
