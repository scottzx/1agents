package agent

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// notify.go wires task lifecycle transitions to an out-of-band notifier
// (#129). When a task becomes blocked, fails, or hands off to verification
// (pending_review), the scheduler/runner emits a TaskNotification. The IM
// integration (internal/ccconnect) implements TaskNotifier to push an
// interactive card to Feishu/Slack and route the user's approve/reject tap
// back through ApplyIMDecision.
//
// The agent package deliberately knows nothing about cc-connect: it exposes a
// narrow interface and a registration hook, keeping the dependency arrow
// one-directional (ccconnect → agent), as in the rest of the bridge wiring.

// TaskNotifyKind labels the transition that produced a notification.
type TaskNotifyKind string

const (
	NotifyBlocked       TaskNotifyKind = "blocked"        // 受阻
	NotifyFailed        TaskNotifyKind = "failed"         // 失败
	NotifyPendingReview TaskNotifyKind = "pending_review" // 待验收
)

// TaskNotification is the immutable payload handed to a TaskNotifier. It
// carries just enough to render a card and route a write-back, with no
// pointer into the live task slice.
type TaskNotification struct {
	Kind          TaskNotifyKind
	WorkspacePath string
	WorkspaceID   string
	TaskID        string
	Number        int
	Title         string
	Summary       string
}

// TaskNotifier receives task-state notifications. Implementations push them to
// an external surface (IM). Notify must be non-blocking or cheap; callers
// invoke it from scheduler/runner goroutines.
type TaskNotifier interface {
	Notify(n TaskNotification)
}

var (
	notifierMu sync.RWMutex
	notifier   TaskNotifier
)

// SetTaskNotifier registers (or clears, with nil) the process-wide notifier.
func SetTaskNotifier(n TaskNotifier) {
	notifierMu.Lock()
	notifier = n
	notifierMu.Unlock()
}

// emitNotify forwards a notification to the registered notifier, if any.
func emitNotify(n TaskNotification) {
	notifierMu.RLock()
	cur := notifier
	notifierMu.RUnlock()
	if cur == nil {
		return
	}
	cur.Notify(n)
}

// IMDecision is the verdict a user taps on an IM card.
type IMDecision string

const (
	IMApprove IMDecision = "approve"
	IMReject  IMDecision = "reject"
)

// ApplyIMDecision writes back a user's one-tap IM verdict to the task, driving
// the same status machine a human would from the board. It is the single owner
// of the IM write-back transition so the IM layer stays free of task rules.
//
//	approve: blocked/failed → pending (re-queue); pending_review → completed
//	reject:  any of the three → cancelled
//
// Returns the updated task title/status for the IM layer to confirm. A no-op
// (task already moved on) returns a nil error and ok=false so the card can say
// the decision was stale.
func ApplyIMDecision(store *TasksStore, wsPath, taskID string, decision IMDecision) (title, newStatus string, ok bool, err error) {
	now := time.Now().UTC()
	mutErr := store.Mutate(wsPath, func(cfg *TasksConfig) bool {
		var t *Task
		for i := range cfg.Tasks {
			if cfg.Tasks[i].ID == taskID {
				t = &cfg.Tasks[i]
				break
			}
		}
		if t == nil {
			return false
		}
		// Only the three notified states are user-decidable. Anything else
		// means the task already advanced; leave it untouched (stale tap).
		switch t.Status {
		case TaskStatusBlocked, TaskStatusFailed, TaskStatusPendingReview:
		default:
			title = t.Title
			return false
		}

		switch decision {
		case IMApprove:
			if t.Status == TaskStatusPendingReview {
				t.Status = TaskStatusCompleted
				t.CompletedAt = &now
				t.Summary = "IM 一键批准:核验通过"
			} else {
				t.Status = TaskStatusPending
				t.StartedAt = nil
				t.Summary = "IM 一键批准:已重新排期执行"
			}
		case IMReject:
			t.Status = TaskStatusCancelled
			t.Summary = "IM 一键打回:已取消"
		default:
			return false
		}

		t.UpdatedAt = now
		t.Replies = append(t.Replies, Reply{
			Author:    Author{Kind: "user", Name: "IM"},
			Text:      fmt.Sprintf("IM 决策:%s → %s", decision, t.Status),
			Mode:      ModePureComment,
			CreatedAt: now,
		})
		title, newStatus, ok = t.Title, string(t.Status), true
		return true
	})
	if mutErr != nil {
		return "", "", false, mutErr
	}
	if ok && newStatus == string(TaskStatusCompleted) {
		task, auditErr := recordDerivedCompletion(
			store, wsPath, taskID,
			"manual_decision", "im_human_decision",
			"A user approved the pending review from an authenticated IM action.",
		)
		if auditErr != nil {
			return title, string(TaskStatusFailed), false, auditErr
		}
		newStatus = string(task.Status)
	}
	if ok {
		log.Printf("[notify] IM decision %s applied to task %s → %s", decision, taskID, newStatus)
	}
	return title, newStatus, ok, nil
}
