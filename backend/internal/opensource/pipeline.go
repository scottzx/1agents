package opensource

import (
	"fmt"
	"sort"
	"strings"
)

// ProposalSink receives evaluated proposals. It is an interface so the pipeline
// does not import the Inbox store directly (core stays decoupled, like
// cc-connect's registries) — meta wires a concrete sink. Tests use a recorder.
type ProposalSink interface {
	// Submit lands one proposal. Implementations dedup by Candidate.FullName.
	Submit(p Proposal) error
}

// Pipeline runs fetch → evaluate → sink. It is the package's top-level entry,
// holding the policy (cfg), the source (fetcher), and the destination (sink).
type Pipeline struct {
	Cfg     ReviewConfig
	Fetcher Fetcher
	Sink    ProposalSink
}

// RunResult summarizes one pipeline run for diagnostics / API responses.
type RunResult struct {
	// Evaluated is every proposal produced (including ignored ones).
	Evaluated []Proposal
	// Submitted counts proposals actually handed to the sink (ignored ones are
	// not submitted — the Inbox should not fill with rejects).
	Submitted int
}

// Run fetches candidates for query, evaluates each, and submits the non-ignore
// proposals to the sink. Ignored proposals are still returned in Evaluated for
// auditing but are not landed. Proposals are returned sorted by score desc so
// the strongest absorb targets are first.
func (p Pipeline) Run(query string, limit int) (RunResult, error) {
	if p.Fetcher == nil {
		return RunResult{}, fmt.Errorf("opensource: pipeline has no fetcher")
	}
	cands, err := p.Fetcher.Search(query, limit)
	if err != nil {
		return RunResult{}, fmt.Errorf("opensource: fetch: %w", err)
	}

	var res RunResult
	for _, c := range cands {
		prop := Evaluate(p.Cfg, c)
		res.Evaluated = append(res.Evaluated, prop)
		if prop.Decision == DecisionIgnore {
			continue
		}
		if p.Sink != nil {
			if err := p.Sink.Submit(prop); err != nil {
				return res, fmt.Errorf("opensource: submit %s: %w", c.FullName, err)
			}
		}
		res.Submitted++
	}

	sort.SliceStable(res.Evaluated, func(i, j int) bool {
		return res.Evaluated[i].Review.Score() > res.Evaluated[j].Review.Score()
	})
	return res, nil
}

// Title renders a one-line Inbox title for a proposal.
func (p Proposal) Title() string {
	return fmt.Sprintf("[开源吸收·%s] %s", p.Decision, p.Candidate.FullName)
}

// Summary renders a one-line summary (decision + score + license).
func (p Proposal) Summary() string {
	return fmt.Sprintf("决策 %s · 评分 %d (契合 %d/质量 %d) · 协议 %s",
		p.Decision, p.Review.Score(), p.Review.Fit, p.Review.Quality, p.Review.License)
}

// Body renders the full proposal as markdown for the Inbox item content: repo
// meta, the decision, and the review reasons a human reads to accept/reject.
func (p Proposal) Body() string {
	var b strings.Builder
	c := p.Candidate
	fmt.Fprintf(&b, "# 开源项目吸收提案：%s\n\n", c.FullName)
	if c.Description != "" {
		fmt.Fprintf(&b, "%s\n\n", c.Description)
	}
	fmt.Fprintf(&b, "- 决策：**%s**\n", p.Decision)
	fmt.Fprintf(&b, "- 综合评分：%d（契合度 %d / 质量 %d）\n", p.Review.Score(), p.Review.Fit, p.Review.Quality)
	fmt.Fprintf(&b, "- 协议：%s（%s）\n", strings.TrimSpace(c.License), p.Review.License)
	if c.URL != "" {
		fmt.Fprintf(&b, "- 仓库：%s\n", c.URL)
	}
	fmt.Fprintf(&b, "- Stars/Forks：%d / %d\n", c.Stars, c.Forks)
	if len(p.Review.Reasons) > 0 {
		b.WriteString("\n## 评审依据\n\n")
		for _, r := range p.Review.Reasons {
			fmt.Fprintf(&b, "- %s\n", r)
		}
	}
	b.WriteString("\n> 本提案由开源吸收管线 (#138) 自动生成，需人工评审后决定是否合并/借鉴。\n")
	return b.String()
}
