package agent

import (
	"fmt"
	"strings"
	"time"
)

// defaultReviewMaxAttempts caps the execute→verify retry loop when a task does
// not set ReviewMaxAttempts. After this many rejected verdicts a task fails
// terminally (报异常) instead of looping forever.
const defaultReviewMaxAttempts = 2

// needsReview reports whether a finished executor run should hand off to a
// verifier instead of completing. Verification requires both a configured
// Verifier agent and acceptance criteria to judge against — without criteria
// there is nothing to verify, so the task completes directly.
func needsReview(t *Task) bool {
	return strings.TrimSpace(t.Verifier) != "" && strings.TrimSpace(t.AcceptanceCriteria) != ""
}

// effectiveReviewMax returns the task's review-cycle budget, applying the
// default when unset.
func effectiveReviewMax(t *Task) int {
	if t.ReviewMaxAttempts > 0 {
		return t.ReviewMaxAttempts
	}
	return defaultReviewMaxAttempts
}

// reviewExhausted reports whether a failed task is terminal due to an exhausted
// review budget (so the scheduler's execution-retry must not requeue it).
func reviewExhausted(t *Task) bool {
	return t.Status == TaskStatusFailed && t.Review != nil && !t.Review.Pass &&
		t.ReviewCount >= effectiveReviewMax(t)
}

// applyReviewVerdict records a verifier's per-criterion verdict on the task
// under review and drives its state machine (#50):
//
//   - every criterion passes  → completed (绿灯)
//   - some criterion fails, budget left → back to pending (re-execute), so the
//     next executor run sees the rejection in its injected timeline background
//   - some fails, budget exhausted → failed (报异常, terminal)
//
// The verdict is authoritative: the verifier reports per-criterion results, the
// server computes overall pass and the transition. The task must currently be
// running (a verification in progress); a call in any other state is rejected
// so a duplicate/late submit can't re-transition. Returns the updated task.
func applyReviewVerdict(store *TasksStore, wsPath, taskID string, criteria []CriterionResult, summary, verifier string) (*Task, error) {
	now := time.Now().UTC()
	pass := len(criteria) > 0
	for _, c := range criteria {
		if !c.Pass {
			pass = false
			break
		}
	}

	var result *Task
	var stateErr error
	mutErr := store.Mutate(wsPath, func(cfg *TasksConfig) bool {
		var t *Task
		for i := range cfg.Tasks {
			if cfg.Tasks[i].ID == taskID {
				t = &cfg.Tasks[i]
				break
			}
		}
		if t == nil {
			stateErr = fmt.Errorf("task not found: %s", taskID)
			return false
		}
		if t.Status != TaskStatusRunning {
			stateErr = fmt.Errorf("task %s is not under review (status %s)", taskID, t.Status)
			return false
		}

		cycle := t.ReviewCount + 1
		verdict := ReviewVerdict{
			Pass:      pass,
			Criteria:  criteria,
			Summary:   summary,
			Attempt:   cycle,
			Verifier:  verifier,
			CreatedAt: now,
		}

		switch {
		case pass:
			t.Status = TaskStatusCompleted
			t.CompletedAt = &now
			t.Summary = fmt.Sprintf("核验通过(第 %d 轮)", cycle)
		default:
			t.ReviewCount = cycle
			if cycle < effectiveReviewMax(t) {
				t.Status = TaskStatusPending
				t.StartedAt = nil
				t.Summary = fmt.Sprintf("核验未通过(第 %d 轮),已重排执行", cycle)
			} else {
				t.Status = TaskStatusFailed
				t.Summary = fmt.Sprintf("核验未通过且复核预算耗尽(%d/%d)", cycle, effectiveReviewMax(t))
			}
		}
		t.Review = &verdict
		t.UpdatedAt = now
		t.Replies = append(t.Replies, Reply{
			Author:    Author{Kind: "agent", Name: verifier},
			AgentType: verifier,
			Text:      renderVerdictReply(verdict),
			Mode:      ModePureComment,
			CreatedAt: now,
		})

		// Copy out before Save reallocates the slice.
		cp := *t
		result = &cp
		return true
	})
	if mutErr != nil {
		return nil, mutErr
	}
	if stateErr != nil {
		return nil, stateErr
	}
	return result, nil
}

// renderVerdictReply formats a verdict as a Markdown timeline entry.
func renderVerdictReply(v ReviewVerdict) string {
	var b strings.Builder
	headline := "❌ 打回"
	if v.Pass {
		headline = "✅ 通过"
	}
	fmt.Fprintf(&b, "## 🔍 核验结果(第 %d 轮):%s\n\n", v.Attempt, headline)
	for _, c := range v.Criteria {
		mark := "❌"
		if c.Pass {
			mark = "✅"
		}
		line := fmt.Sprintf("- %s %s", mark, c.Criterion)
		if strings.TrimSpace(c.Comment) != "" {
			line += " — " + c.Comment
		}
		b.WriteString(line + "\n")
	}
	if strings.TrimSpace(v.Summary) != "" {
		b.WriteString("\n" + v.Summary + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
