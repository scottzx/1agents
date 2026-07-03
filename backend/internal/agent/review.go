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

// effectiveVerifierCount is how many independent verifiers judge each cycle.
// 0/1 = the classic single-verifier flow; >1 is an adversarial panel.
func effectiveVerifierCount(t *Task) int {
	if t.VerifierCount > 1 {
		return t.VerifierCount
	}
	return 1
}

// effectivePassThreshold is how many of the panel's verdicts must pass for the
// artifact to be accepted. Unset (0) defaults to a simple majority (⌊N/2⌋+1).
// Clamped to [1, N] so a misconfigured value can't make the gate impossible or
// trivial.
func effectivePassThreshold(t *Task) int {
	n := effectiveVerifierCount(t)
	thr := t.VerifyPassThreshold
	if thr <= 0 {
		thr = n/2 + 1
	}
	if thr > n {
		thr = n
	}
	if thr < 1 {
		thr = 1
	}
	return thr
}

// criteriaPass reports whether a verifier's per-criterion verdict is an overall
// pass: at least one criterion and every one of them passing.
func criteriaPass(criteria []CriterionResult) bool {
	if len(criteria) == 0 {
		return false
	}
	for _, c := range criteria {
		if !c.Pass {
			return false
		}
	}
	return true
}

// aggregatePanel folds the cycle's accumulated per-verifier verdicts into a
// single panel verdict. Overall Pass is server-computed as "≥ threshold
// verifiers passed". The merged Criteria union keeps the dissenting verifiers'
// comments so the re-execution sees what each panelist objected to. See #131.
func aggregatePanel(pool []ReviewVerdict, threshold, attempt int, now time.Time) ReviewVerdict {
	// Single-verifier panel = the classic flow: pass through the lone verdict
	// untouched so its timeline output is identical to pre-#131 behaviour.
	if len(pool) == 1 {
		v := pool[0]
		v.Attempt = attempt
		v.CreatedAt = now
		return v
	}
	passCount := 0
	needsHumanCount := 0
	for _, v := range pool {
		if v.Pass {
			passCount++
		}
		if v.NeedsHuman {
			needsHumanCount++
		}
	}
	panelPass := passCount >= threshold
	// Escalation diverts only the reject path: a pass consensus still ships,
	// but absent one, a single credible "needs human" beats a pointless
	// executor re-run. NeedsHuman and Pass are mutually exclusive on the
	// aggregate (Pass wins).
	panelNeedsHuman := !panelPass && needsHumanCount > 0

	var merged []CriterionResult
	var summaries []string
	for i, v := range pool {
		label := v.Verifier
		if label == "" {
			label = fmt.Sprintf("verifier#%d", i+1)
		}
		for _, c := range v.Criteria {
			cc := c
			cc.Criterion = fmt.Sprintf("[%s] %s", label, c.Criterion)
			merged = append(merged, cc)
		}
		if strings.TrimSpace(v.Summary) != "" {
			summaries = append(summaries, fmt.Sprintf("- %s: %s", label, strings.TrimSpace(v.Summary)))
		}
	}
	summary := fmt.Sprintf("对抗式核验:%d/%d 通过(阈值 %d)", passCount, len(pool), threshold)
	if needsHumanCount > 0 {
		summary += fmt.Sprintf(",%d 判定需人工介入", needsHumanCount)
	}
	if len(summaries) > 0 {
		summary += "\n" + strings.Join(summaries, "\n")
	}
	return ReviewVerdict{
		Pass:       panelPass,
		NeedsHuman: panelNeedsHuman,
		Criteria:   merged,
		Summary:    summary,
		Attempt:    attempt,
		Verifier:   fmt.Sprintf("panel(%d)", len(pool)),
		CreatedAt:  now,
	}
}

// applyReviewVerdict records a verifier's per-criterion verdict on the task
// under review and drives its state machine (#50):
//
//   - every criterion passes  → completed (绿灯)
//   - verifier flags needsHuman → awaiting_human (升级人工), budget untouched
//   - some criterion fails, budget left → back to pending (re-execute), so the
//     next executor run sees the rejection in its injected timeline background
//   - some fails, budget exhausted → failed (报异常, terminal)
//
// needsHuman is the verifier's explicit escalation route (借鉴路): it wins over
// a rejection but not over a pass consensus, and never counts toward pass — so
// an escalating verdict can't accidentally complete the task.
//
// The verdict is authoritative: the verifier reports per-criterion results, the
// server computes overall pass and the transition. The task must currently be
// running (a verification in progress); a call in any other state is rejected
// so a duplicate/late submit can't re-transition. Returns the updated task.
func applyReviewVerdict(store *TasksStore, wsPath, taskID string, criteria []CriterionResult, needsHuman bool, summary, verifier string) (*Task, error) {
	now := time.Now().UTC()
	// An escalating verdict never counts as a pass, even if the verifier also
	// happened to mark every criterion — the route it chose is authoritative.
	pass := criteriaPass(criteria) && !needsHuman

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
		n := effectiveVerifierCount(t)

		// Each verifier in the panel submits independently. Accumulate this
		// verifier's verdict and wait for the rest before deciding (#131). For the
		// classic single-verifier flow (n == 1) the panel completes immediately.
		t.ReviewPool = append(t.ReviewPool, ReviewVerdict{
			Pass:       pass,
			NeedsHuman: needsHuman,
			Criteria:   criteria,
			Summary:    summary,
			Attempt:    cycle,
			Verifier:   verifier,
			CreatedAt:  now,
		})

		// In a multi-verifier panel each panelist's individual verdict lands on
		// the timeline as it arrives. The single-verifier flow keeps its one
		// aggregate reply (appended at the tail) to stay byte-identical to pre-#131.
		if n > 1 {
			t.Replies = append(t.Replies, Reply{
				Author:    Author{Kind: "agent", Name: verifier},
				AgentType: verifier,
				Text:      renderVerdictReply(t.ReviewPool[len(t.ReviewPool)-1]),
				Mode:      ModePureComment,
				CreatedAt: now,
			})
		}

		if len(t.ReviewPool) < n {
			// Panel still gathering verdicts: stay under review (running) so the
			// next verifier's submit is accepted. Record progress and return.
			t.UpdatedAt = now
			t.Replies = append(t.Replies, Reply{
				Author:    Author{Kind: "agent", Name: "panel"},
				Text:      fmt.Sprintf("对抗式核验进行中:已收 %d/%d 份裁决,阈值 %d", len(t.ReviewPool), n, effectivePassThreshold(t)),
				Mode:      ModePureComment,
				CreatedAt: now,
			})
			cp := *t
			result = &cp
			return true
		}

		// Panel complete: aggregate the pool into one authoritative verdict,
		// clear it, and drive the same state machine the single-verifier flow uses.
		verdict := aggregatePanel(t.ReviewPool, effectivePassThreshold(t), cycle, now)
		t.ReviewPool = nil

		// Record the verdict before transitioning so the verify-failed /
		// verify-needs-human rules can read its summary off the task (#133).
		t.Review = &verdict

		switch {
		case verdict.Pass:
			t.Status = TaskStatusCompleted
			t.CompletedAt = &now
			t.Summary = fmt.Sprintf("核验通过(第 %d 轮)", cycle)
		case verdict.NeedsHuman:
			// verify-needs-human → 升级人工 (借鉴路): park at awaiting_human via
			// the declarative engine. Does NOT consume review budget — it's an
			// escalation, not a rejection, so ReviewCount stays put and the task
			// is not eligible for a re-execution round.
			t.Summary = fmt.Sprintf("核验判定需人工介入(第 %d 轮),已升级等待人工决策", cycle)
			ev := TaskEvent{Kind: EventVerifyNeedsHuman, Task: *t, Signals: DeriveSignals(*t), At: now}
			applyEventActions(t, DefaultEventEngine().Evaluate(ev), now)
		default:
			t.ReviewCount = cycle
			if cycle < effectiveReviewMax(t) {
				// verify-failed → 自动重派执行者(带失败上下文) (#133): the requeue
				// (status→pending, StartedAt cleared) and the failure-context
				// note are driven by the declarative engine, not hardwired here.
				t.Summary = fmt.Sprintf("核验未通过(第 %d 轮),已重排执行", cycle)
				ev := TaskEvent{Kind: EventVerifyFailed, Task: *t, Signals: DeriveSignals(*t), At: now}
				applyEventActions(t, DefaultEventEngine().Evaluate(ev), now)
			} else {
				t.Status = TaskStatusFailed
				t.Summary = fmt.Sprintf("核验未通过且复核预算耗尽(%d/%d)", cycle, effectiveReviewMax(t))
			}
		}
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
	switch {
	case v.Pass:
		headline = "✅ 通过"
	case v.NeedsHuman:
		headline = "⚠️ 需人工介入"
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
