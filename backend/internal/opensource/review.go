package opensource

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// ReviewConfig tunes the absorb review heuristics. The zero value is unusable;
// use DefaultReviewConfig and override fields as needed.
type ReviewConfig struct {
	// Keywords are the project-direction vocabulary; a candidate's
	// description/topics matching these raise the fit score. Lower-case.
	Keywords []string
	// MinScore is the combined-score floor below which a candidate is ignored.
	MinScore int
	// MergeScore is the combined-score floor above which a mergeable-licensed
	// candidate is proposed for merge (rather than borrow).
	MergeScore int
	// Now is the clock used for staleness; nil defaults to time.Now.
	Now func() time.Time
}

// DefaultReviewConfig is the project's absorb policy: keywords reflect the
// 1Agents direction (agent / skill / inbox / orchestration …); thresholds keep
// the bar high so merge proposals are rare and deliberate.
func DefaultReviewConfig() ReviewConfig {
	return ReviewConfig{
		Keywords: []string{
			"agent", "agents", "llm", "claude", "skill", "inbox",
			"orchestration", "workflow", "automation", "knowledge", "rag",
			"terminal", "cli", "prompt", "mcp",
		},
		MinScore:   40,
		MergeScore: 70,
		Now:        time.Now,
	}
}

func (c ReviewConfig) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// ReviewCandidate scores a candidate's license, fit, and quality. It is pure
// (no I/O) so it is trivially testable.
func ReviewCandidate(cfg ReviewConfig, c Candidate) Review {
	var reasons []string

	lic := ClassifyLicense(c.License)
	reasons = append(reasons, fmt.Sprintf("协议 %s → %s", strings.TrimSpace(c.License), lic))

	fit, fitReasons := scoreFit(cfg, c)
	reasons = append(reasons, fitReasons...)

	quality, qualReasons := scoreQuality(cfg, c)
	reasons = append(reasons, qualReasons...)

	return Review{License: lic, Fit: fit, Quality: quality, Reasons: reasons}
}

// scoreFit measures direction alignment: keyword hits across description and
// topics. Each distinct matched keyword adds weight, capped at 100.
func scoreFit(cfg ReviewConfig, c Candidate) (int, []string) {
	hay := strings.ToLower(c.Description + " " + strings.Join(c.Topics, " ") + " " + c.FullName)
	matched := map[string]bool{}
	for _, kw := range cfg.Keywords {
		kw = strings.ToLower(strings.TrimSpace(kw))
		if kw != "" && strings.Contains(hay, kw) {
			matched[kw] = true
		}
	}
	if len(matched) == 0 {
		return 0, []string{"契合度 0：未命中方向关键词"}
	}
	// 30 base for the first hit + 20 per additional distinct hit, capped.
	score := 30 + (len(matched)-1)*20
	if score > 100 {
		score = 100
	}
	keys := make([]string, 0, len(matched))
	for k := range matched {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return score, []string{fmt.Sprintf("契合度 %d：命中关键词 [%s]", score, strings.Join(keys, ", "))}
}

// scoreQuality is a popularity+health heuristic: stars (log-ish buckets),
// fork ratio is ignored (noisy), with penalties for archived/stale repos.
func scoreQuality(cfg ReviewConfig, c Candidate) (int, []string) {
	var reasons []string
	score := 0

	switch {
	case c.Stars >= 5000:
		score += 60
	case c.Stars >= 1000:
		score += 45
	case c.Stars >= 200:
		score += 30
	case c.Stars >= 50:
		score += 15
	default:
		score += 5
	}
	reasons = append(reasons, fmt.Sprintf("质量基线：%d stars", c.Stars))

	if c.Forks >= 100 {
		score += 20
	} else if c.Forks >= 20 {
		score += 10
	}

	if c.Archived {
		score -= 40
		reasons = append(reasons, "质量扣分：仓库已 archived")
	}
	if !c.PushedAt.IsZero() {
		age := cfg.now().Sub(c.PushedAt)
		if age > 2*365*24*time.Hour {
			score -= 30
			reasons = append(reasons, "质量扣分：超过 2 年无提交")
		} else if age > 365*24*time.Hour {
			score -= 15
			reasons = append(reasons, "质量扣分：超过 1 年无提交")
		}
	}

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	reasons = append(reasons, fmt.Sprintf("质量 %d", score))
	return score, reasons
}

// Decide turns a review into a decision under cfg's thresholds:
//
//   - score < MinScore                         → ignore (off-direction / low quality)
//   - license incompatible                     → ignore (can't even safely借鉴 wholesale)
//   - license mergeable AND score ≥ MergeScore → merge
//   - otherwise                                → borrow (idea worth taking, source not vendored)
func Decide(cfg ReviewConfig, r Review) Decision {
	score := r.Score()
	if r.License == LicenseIncompatible {
		return DecisionIgnore
	}
	if score < cfg.MinScore {
		return DecisionIgnore
	}
	if r.License.CanMerge() && score >= cfg.MergeScore {
		return DecisionMerge
	}
	return DecisionBorrow
}

// Evaluate is the full per-candidate pipeline: review → decide → proposal.
func Evaluate(cfg ReviewConfig, c Candidate) Proposal {
	r := ReviewCandidate(cfg, c)
	return Proposal{Candidate: c, Review: r, Decision: Decide(cfg, r)}
}
