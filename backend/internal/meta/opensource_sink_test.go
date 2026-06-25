package meta

import (
	"testing"
	"time"

	"github.com/scottzx/1Agents/backend/internal/opensource"
)

func sampleProposal() opensource.Proposal {
	cfg := opensource.DefaultReviewConfig()
	cfg.Now = func() time.Time { return time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC) }
	return opensource.Evaluate(cfg, opensource.Candidate{
		FullName:    "anthropics/claude-agent-sdk",
		URL:         "https://github.com/anthropics/claude-agent-sdk",
		Description: "llm agent skill workflow orchestration",
		License:     "MIT",
		Stars:       8000,
		PushedAt:    cfg.Now(),
	})
}

func TestInboxProposalSink_Submit(t *testing.T) {
	store := newTestInboxStore(t)
	sink := NewInboxProposalSink(store)

	if err := sink.Submit(sampleProposal()); err != nil {
		t.Fatalf("submit: %v", err)
	}
	items, err := store.List(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	it := items[0]
	if it.Source != InboxSourceMisc {
		t.Errorf("source = %q, want misc", it.Source)
	}
	if it.URL != "https://github.com/anthropics/claude-agent-sdk" {
		t.Errorf("url = %q", it.URL)
	}
	hasTag := func(tag string) bool {
		for _, x := range it.Tags {
			if x == tag {
				return true
			}
		}
		return false
	}
	if !hasTag("opensource-absorb") || !hasTag(string(opensource.DecisionMerge)) {
		t.Errorf("tags missing expected entries: %v", it.Tags)
	}
}

func TestInboxProposalSink_DedupByURL(t *testing.T) {
	store := newTestInboxStore(t)
	sink := NewInboxProposalSink(store)
	p := sampleProposal()

	if err := sink.Submit(p); err != nil {
		t.Fatal(err)
	}
	if err := sink.Submit(p); err != nil {
		t.Fatal(err)
	}
	items, err := store.List(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("dedup failed: got %d items, want 1", len(items))
	}
}
