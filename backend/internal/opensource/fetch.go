package opensource

import "fmt"

// Fetcher is the injectable seam for sourcing candidates from the outside world
// (GitHub search API today; other registries later). Keeping it an interface
// means the pipeline is unit-testable with a static fixture and never depends on
// live network — same shape as meta.InboxSource.
type Fetcher interface {
	// Search returns candidate repos matching query (e.g. a GitHub search
	// expression). limit caps the result count (≤0 means "fetcher default").
	Search(query string, limit int) ([]Candidate, error)
}

// StaticFetcher is the test/seed implementation: it returns a fixed candidate
// slice, ignoring the query. Used in tests and as the default offline source so
// the pipeline runs without a network.
type StaticFetcher struct {
	Candidates []Candidate
}

// Search returns the static candidates (clamped to limit when positive).
func (f StaticFetcher) Search(_ string, limit int) ([]Candidate, error) {
	if limit > 0 && limit < len(f.Candidates) {
		return append([]Candidate(nil), f.Candidates[:limit]...), nil
	}
	return append([]Candidate(nil), f.Candidates...), nil
}

// GitHubFetcher is the real-network placeholder. Wiring the GitHub search API
// (auth, pagination, rate limits, license enrichment) is intentionally out of
// scope for #138's minimal slice — the type exists so a follow-up can implement
// Search without touching the pipeline. Token is left for that implementation.
type GitHubFetcher struct {
	// Token is the GitHub API token used once Search is implemented.
	Token string
}

// Search is not yet implemented; it returns an explicit error so callers fall
// back to StaticFetcher (or surface the gap) rather than silently getting zero
// candidates. See #138 follow-up for the live implementation.
func (f GitHubFetcher) Search(query string, limit int) ([]Candidate, error) {
	return nil, fmt.Errorf("opensource: GitHubFetcher.Search not implemented (network fetch is a #138 follow-up); inject a StaticFetcher for offline runs")
}
