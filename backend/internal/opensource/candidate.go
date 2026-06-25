package opensource

import "time"

// Candidate is one open-source project surfaced for absorption review. It is
// repo *metadata* only — the absorber never clones or executes anything. Fields
// mirror the GitHub repo shape so a real Fetcher can fill them from the search
// API, but the type is fetch-agnostic (a manual Inbox capture can hand-fill it).
type Candidate struct {
	// FullName is "owner/repo" — the stable identifier used for dedup/provenance.
	FullName string
	// URL is the repo's HTML URL.
	URL string
	// Description is the repo's short blurb (the primary fit signal).
	Description string
	// License is the raw license string (SPDX id or loose name) as reported by
	// the source; classified via ClassifyLicense.
	License string
	// Stars / Forks are popularity signals feeding the quality heuristic.
	Stars int
	Forks int
	// Topics are repo topic tags (secondary fit signal).
	Topics []string
	// Language is the primary language (informational; not scored).
	Language string
	// Archived marks a read-only/abandoned repo — a quality penalty.
	Archived bool
	// PushedAt is the last push time; staleness feeds the quality heuristic. Zero
	// means unknown (no staleness penalty applied).
	PushedAt time.Time
}

// Decision is the absorb pipeline's verdict for a candidate.
type Decision string

const (
	// DecisionMerge: permissive license + strong fit/quality → propose vendoring
	// the source (still human-reviewed before any code lands).
	DecisionMerge Decision = "merge"
	// DecisionBorrow: worth learning from, but license forbids merge OR the score
	// is middling → propose借鉴 idea, not source.
	DecisionBorrow Decision = "borrow"
	// DecisionIgnore: off-direction / low quality / incompatible license.
	DecisionIgnore Decision = "ignore"
)

// Review is the scored assessment of a candidate before a decision is made.
type Review struct {
	// License is the classified merge-ability of the candidate's license.
	License License
	// Fit is the project-direction alignment score in [0,100].
	Fit int
	// Quality is the code-quality/health heuristic score in [0,100].
	Quality int
	// Reasons are human-readable bullet points explaining the scores; they become
	// the proposal Rationale a reviewer reads.
	Reasons []string
}

// Score is the combined fit+quality score in [0,100] (simple mean).
func (r Review) Score() int { return (r.Fit + r.Quality) / 2 }

// Proposal is the reviewed 吸收提案: a candidate + its review + the decision. It
// is the absorber's only output — it lands in the Inbox for a human to accept,
// mirroring #188's "transform into a proposal, never auto-apply" stance.
type Proposal struct {
	Candidate Candidate
	Review    Review
	Decision  Decision
}
