package feishusync

import (
	"strings"
	"time"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

// Provider is the value stored in a task's external-sync provider slot for
// Feishu Bitable bindings. It distinguishes Bitable sync state from the GitHub
// sync fields (#74) that live on the same Task.
const Provider = "feishu-bitable"

// Bitable column names. These are the human-readable header labels the field
// mapping (issue #101 draft) targets. They are constants so the mapping and the
// table-creation helper stay in agreement.
const (
	ColNumber      = "任务编号"
	ColTitle       = "任务标题"
	ColStatus      = "状态"
	ColPriority    = "优先级"
	ColAssignee    = "负责 Agent"
	ColLabels      = "标签"
	ColMilestone   = "里程碑"
	ColSprint      = "Sprint"
	ColType        = "类型"
	ColPlannedStart = "计划开始"
	ColPlannedEnd   = "计划结束"
	ColDescription  = "描述"
	ColUpdatedAt    = "最后更新"
	// ColRecordRef is a hidden helper column NOT used for matching (matching is
	// by record_id). It carries the local task id so a human inspecting the
	// Bitable can trace a row back to a task.
	ColRecordRef = "本地任务ID"
)

// readOnlyColumns are columns the local side owns; a remote edit to any of them
// is ignored on pull. Per the field-mapping draft, 任务编号 and 最后更新 are
// read-only mirrors.
var readOnlyColumns = map[string]bool{
	ColNumber:    true,
	ColUpdatedAt: true,
	ColRecordRef: true,
}

// TaskToFields renders the syncable subset of a task into Bitable column
// values. Dates are emitted as epoch-millis numbers (Feishu date column format).
func TaskToFields(t meta.Task) map[string]interface{} {
	f := map[string]interface{}{
		ColNumber:    t.Number,
		ColTitle:     t.Title,
		ColStatus:    string(t.Status),
		ColPriority:  string(t.Priority),
		ColAssignee:  t.Assignee,
		ColLabels:    append([]string(nil), t.Labels...),
		ColMilestone: t.Milestone,
		ColSprint:    t.Sprint,
		ColType:      string(t.Type),
		ColDescription: t.Description,
		ColRecordRef:   t.ID,
		ColUpdatedAt:   t.UpdatedAt.UnixMilli(),
	}
	if t.PlannedStart != nil {
		f[ColPlannedStart] = t.PlannedStart.UnixMilli()
	}
	if t.PlannedEnd != nil {
		f[ColPlannedEnd] = t.PlannedEnd.UnixMilli()
	}
	return f
}

// ApplyRecordToTask writes the writable Bitable fields of rec onto t (which is
// the local task identified by the record's external id). Read-only columns and
// columns absent from rec are left untouched. Returns true if any field
// changed. This is the pull direction: remote → local, writable fields only.
func ApplyRecordToTask(t *meta.Task, rec Record) bool {
	changed := false
	set := func(cur *string, col string) {
		if readOnlyColumns[col] {
			return
		}
		v, ok := rec.Fields[col]
		if !ok {
			return
		}
		s := asText(v)
		if s != *cur {
			*cur = s
			changed = true
		}
	}
	set(&t.Title, ColTitle)
	set((*string)(&t.Status), ColStatus)
	set((*string)(&t.Priority), ColPriority)
	set(&t.Assignee, ColAssignee)
	set(&t.Milestone, ColMilestone)
	set(&t.Sprint, ColSprint)
	set((*string)(&t.Type), ColType)
	set(&t.Description, ColDescription)

	if v, ok := rec.Fields[ColLabels]; ok {
		labels := asStringSlice(v)
		if !equalStrings(labels, t.Labels) {
			t.Labels = labels
			changed = true
		}
	}
	if changed {
		t.UpdatedAt = time.Now().UTC()
	}
	return changed
}

// asText flattens a Bitable field value to a plain string. Feishu returns text
// columns either as a string or as a list of segment objects
// ([{"text": "...", "type": "text"}]); we collapse both to text.
func asText(v interface{}) string {
	switch x := v.(type) {
	case string:
		return x
	case []interface{}:
		var b strings.Builder
		for _, seg := range x {
			if m, ok := seg.(map[string]interface{}); ok {
				if txt, ok := m["text"].(string); ok {
					b.WriteString(txt)
					continue
				}
			}
			if s, ok := seg.(string); ok {
				b.WriteString(s)
			}
		}
		return b.String()
	case map[string]interface{}:
		if txt, ok := x["text"].(string); ok {
			return txt
		}
		return ""
	case nil:
		return ""
	default:
		return ""
	}
}

// asStringSlice flattens a multi-select / list field to a string slice.
func asStringSlice(v interface{}) []string {
	switch x := v.(type) {
	case []string:
		return append([]string(nil), x...)
	case []interface{}:
		out := make([]string, 0, len(x))
		for _, item := range x {
			if s := asText(item); s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		if x == "" {
			return nil
		}
		return []string{x}
	default:
		return nil
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
