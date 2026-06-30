package opensource

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestClassifyLicense(t *testing.T) {
	cases := []struct {
		raw  string
		want License
	}{
		{"MIT", LicenseMergeable},
		{"mit license", LicenseMergeable},
		{"Apache-2.0", LicenseMergeable},
		{"Apache 2.0", LicenseMergeable},
		{"BSD-3-Clause", LicenseMergeable},
		{"BSD", LicenseMergeable},
		{"ISC", LicenseMergeable},
		{"MPL-2.0", LicenseBorrowOnly},
		{"LGPL-3.0", LicenseBorrowOnly},
		{"GPL-3.0", LicenseIncompatible},
		{"GPL-2.0-only", LicenseIncompatible},
		{"AGPL-3.0-or-later", LicenseIncompatible},
		{"proprietary", LicenseIncompatible},
		{"", LicenseUnknown},
		{"SomeNovelLicense", LicenseUnknown},
	}
	for _, c := range cases {
		if got := ClassifyLicense(c.raw); got != c.want {
			t.Errorf("ClassifyLicense(%q) = %v, want %v", c.raw, got, c.want)
		}
	}
}

func TestCanMerge(t *testing.T) {
	if !LicenseMergeable.CanMerge() {
		t.Error("mergeable should CanMerge")
	}
	for _, l := range []License{LicenseUnknown, LicenseBorrowOnly, LicenseIncompatible} {
		if l.CanMerge() {
			t.Errorf("%v should not CanMerge", l)
		}
	}
}

func fixedNow() time.Time { return time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC) }

func cfgWithClock() ReviewConfig {
	c := DefaultReviewConfig()
	c.Now = fixedNow
	return c
}

func TestEvaluate_MergeStrongMitProject(t *testing.T) {
	c := Candidate{
		FullName:    "anthropics/claude-agent-sdk",
		URL:         "https://github.com/anthropics/claude-agent-sdk",
		Description: "SDK for building LLM agents with skill and workflow orchestration",
		License:     "MIT",
		Stars:       8000,
		Forks:       400,
		PushedAt:    fixedNow().Add(-30 * 24 * time.Hour),
	}
	p := Evaluate(cfgWithClock(), c)
	if p.Decision != DecisionMerge {
		t.Fatalf("decision = %v, want merge (score=%d, lic=%v)", p.Decision, p.Review.Score(), p.Review.License)
	}
	if p.Review.License != LicenseMergeable {
		t.Errorf("license = %v, want mergeable", p.Review.License)
	}
}

func TestEvaluate_BorrowWhenLicenseBlocksMerge(t *testing.T) {
	c := Candidate{
		FullName:    "someone/gpl-agent-framework",
		Description: "powerful agent automation workflow llm orchestration toolkit",
		License:     "GPL-3.0",
		Stars:       9000,
		PushedAt:    fixedNow(),
	}
	p := Evaluate(cfgWithClock(), c)
	// GPL is incompatible → ignore (can't even vendor wholesale).
	if p.Decision != DecisionIgnore {
		t.Fatalf("GPL decision = %v, want ignore", p.Decision)
	}
}

func TestEvaluate_BorrowWeakCopyleftHighFit(t *testing.T) {
	c := Candidate{
		FullName:    "someone/mpl-agent",
		Description: "agent skill workflow automation orchestration",
		License:     "MPL-2.0",
		Stars:       6000,
		PushedAt:    fixedNow(),
	}
	p := Evaluate(cfgWithClock(), c)
	if p.Decision != DecisionBorrow {
		t.Fatalf("MPL high-fit decision = %v, want borrow", p.Decision)
	}
}

func TestEvaluate_IgnoreOffDirection(t *testing.T) {
	c := Candidate{
		FullName:    "someone/recipe-book",
		Description: "a collection of dinner recipes",
		License:     "MIT",
		Stars:       3,
	}
	p := Evaluate(cfgWithClock(), c)
	if p.Decision != DecisionIgnore {
		t.Fatalf("off-direction decision = %v, want ignore", p.Decision)
	}
}

func TestScoreQuality_PenalizesStaleAndArchived(t *testing.T) {
	cfg := cfgWithClock()
	fresh := Candidate{Stars: 2000, PushedAt: fixedNow()}
	stale := Candidate{Stars: 2000, PushedAt: fixedNow().Add(-3 * 365 * 24 * time.Hour)}
	archived := Candidate{Stars: 2000, Archived: true, PushedAt: fixedNow()}

	qf, _ := scoreQuality(cfg, fresh)
	qs, _ := scoreQuality(cfg, stale)
	qa, _ := scoreQuality(cfg, archived)
	if !(qf > qs) {
		t.Errorf("stale (%d) should score below fresh (%d)", qs, qf)
	}
	if !(qf > qa) {
		t.Errorf("archived (%d) should score below fresh (%d)", qa, qf)
	}
}

// recorderSink captures submitted proposals for assertions.
type recorderSink struct {
	got []Proposal
	err error
}

func (s *recorderSink) Submit(p Proposal) error {
	if s.err != nil {
		return s.err
	}
	s.got = append(s.got, p)
	return nil
}

func TestPipeline_Run_SubmitsOnlyNonIgnore(t *testing.T) {
	fetcher := StaticFetcher{Candidates: []Candidate{
		{FullName: "a/merge", Description: "llm agent skill orchestration", License: "MIT", Stars: 9000, PushedAt: fixedNow()},
		{FullName: "b/recipes", Description: "dinner recipes", License: "MIT", Stars: 1},
		{FullName: "c/borrow", Description: "agent workflow automation", License: "MPL-2.0", Stars: 6000, PushedAt: fixedNow()},
	}}
	sink := &recorderSink{}
	p := Pipeline{Cfg: cfgWithClock(), Fetcher: fetcher, Sink: sink}

	res, err := p.Run("agent", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Evaluated) != 3 {
		t.Fatalf("evaluated = %d, want 3", len(res.Evaluated))
	}
	if res.Submitted != 2 {
		t.Fatalf("submitted = %d, want 2 (ignore the recipe repo)", res.Submitted)
	}
	if len(sink.got) != 2 {
		t.Fatalf("sink got %d, want 2", len(sink.got))
	}
	// Evaluated sorted by score desc.
	if res.Evaluated[0].Review.Score() < res.Evaluated[len(res.Evaluated)-1].Review.Score() {
		t.Error("evaluated not sorted by score desc")
	}
}

func TestPipeline_Run_NoFetcher(t *testing.T) {
	p := Pipeline{Cfg: DefaultReviewConfig()}
	if _, err := p.Run("x", 0); err == nil {
		t.Fatal("expected error with nil fetcher")
	}
}

func TestGitHubFetcher_Placeholder(t *testing.T) {
	_, err := GitHubFetcher{}.Search("agent", 5)
	if err == nil {
		t.Fatal("placeholder fetcher should return not-implemented error")
	}
	if !strings.Contains(err.Error(), "not implemented") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStaticFetcher_RespectsLimit(t *testing.T) {
	f := StaticFetcher{Candidates: []Candidate{{FullName: "a"}, {FullName: "b"}, {FullName: "c"}}}
	got, err := f.Search("", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("limit not respected: got %d", len(got))
	}
}

func TestProposal_BodyContainsKeyFields(t *testing.T) {
	p := Evaluate(cfgWithClock(), Candidate{
		FullName:    "x/y",
		URL:         "https://github.com/x/y",
		Description: "agent llm skill",
		License:     "MIT",
		Stars:       3000,
		PushedAt:    fixedNow(),
	})
	body := p.Body()
	for _, want := range []string{"x/y", "MIT", "https://github.com/x/y", "评审依据"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q\n%s", want, body)
		}
	}
}

func TestPipeline_Run_PropagatesSinkError(t *testing.T) {
	fetcher := StaticFetcher{Candidates: []Candidate{
		{FullName: "a/merge", Description: "llm agent skill", License: "MIT", Stars: 9000, PushedAt: fixedNow()},
	}}
	p := Pipeline{Cfg: cfgWithClock(), Fetcher: fetcher, Sink: &recorderSink{err: errors.New("boom")}}
	if _, err := p.Run("agent", 0); err == nil {
		t.Fatal("expected sink error to propagate")
	}
}
