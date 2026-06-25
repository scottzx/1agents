package research

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/kwiki"
)

func newStore(t *testing.T) *kwiki.Store {
	t.Helper()
	s, err := kwiki.Open(t.TempDir())
	if err != nil {
		t.Fatalf("kwiki.Open: %v", err)
	}
	return s
}

// End-to-end: a static源 ingests into kwiki (L1), and the one item matching the
// deep-dive rule gets an L2 card; the non-matching one does not.
func TestRunIngestsAndDeepDivesMatchingItems(t *testing.T) {
	store := newStore(t)
	src := &StaticSource{
		SourceName: "top50",
		Items: []Item{
			{ID: "a", Title: "某榜单常规更新", Text: "排名小幅波动", Domain: "market"},
			{ID: "b", Title: "出现未见过的产品形态 X", Text: "一个全新品类的 AI 硬件", Domain: "market", Tags: []string{"竞品"}},
		},
	}
	p := &Pipeline{
		Store:          store,
		Source:         src,
		Browser:        StubBrowser{},
		Rule:           KeywordRule("未见过的产品形态"),
		Role:           "market-analyst",
		RolePrompt:     "你是市场分析师，产出 why-分析。",
		DeepDiveBudget: 5,
	}

	res, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Ingested) != 2 {
		t.Fatalf("expected 2 ingested pages, got %d (%v)", len(res.Ingested), res.Ingested)
	}
	if len(res.Cards) != 1 {
		t.Fatalf("expected 1 deep-dive card, got %d", len(res.Cards))
	}
	card := res.Cards[0]
	if card.Title != "出现未见过的产品形态 X" {
		t.Errorf("card title = %q", card.Title)
	}
	if !strings.Contains(card.Source, "top50") || !strings.Contains(card.Source, "market-analyst") {
		t.Errorf("card source missing provenance: %q", card.Source)
	}
	for _, want := range []string{"## 触发条目", "## 调研证据", "market-analyst", "why-分析"} {
		if !strings.Contains(card.Body, want) {
			t.Errorf("card body missing %q:\n%s", want, card.Body)
		}
	}

	// Pages actually landed in the wiki (L1 ingest via kwiki).
	pages, err := store.Pages()
	if err != nil {
		t.Fatalf("Pages: %v", err)
	}
	if len(pages) != 2 {
		t.Fatalf("expected 2 wiki pages, got %d", len(pages))
	}
}

// The deep-dive budget caps L2 calls per Run; over-budget matches are reported
// as skipped, not silently dropped (RFC §9.2 限流).
func TestDeepDiveBudgetLimitsCalls(t *testing.T) {
	store := newStore(t)
	src := &StaticSource{Items: []Item{
		{ID: "1", Title: "命中 alpha", Text: "alpha"},
		{ID: "2", Title: "命中 beta", Text: "alpha"},
		{ID: "3", Title: "命中 gamma", Text: "alpha"},
	}}
	calls := 0
	p := &Pipeline{
		Store:  store,
		Source: src,
		Browser: FuncBrowser(func(context.Context, string, string) (string, error) {
			calls++
			return "evidence", nil
		}),
		Rule:           KeywordRule("alpha"),
		DeepDiveBudget: 2,
	}
	res, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if calls != 2 {
		t.Errorf("expected 2 browser calls (budget), got %d", calls)
	}
	if len(res.Cards) != 2 {
		t.Errorf("expected 2 cards, got %d", len(res.Cards))
	}
	if len(res.DeepDiveSkipped) != 1 || res.DeepDiveSkipped[0] != "命中 gamma" {
		t.Errorf("expected gamma skipped, got %v", res.DeepDiveSkipped)
	}
}

// Zero budget means pure ingest, no deep dive even on a match.
func TestZeroBudgetSkipsDeepDive(t *testing.T) {
	store := newStore(t)
	src := &StaticSource{Items: []Item{{ID: "1", Title: "命中", Text: "alpha"}}}
	p := &Pipeline{
		Store:          store,
		Source:         src,
		Browser:        StubBrowser{},
		Rule:           KeywordRule("alpha"),
		DeepDiveBudget: 0,
	}
	res, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Cards) != 0 {
		t.Errorf("expected no cards at zero budget, got %d", len(res.Cards))
	}
	if len(res.DeepDiveSkipped) != 1 {
		t.Errorf("expected 1 skipped, got %v", res.DeepDiveSkipped)
	}
}

// A browser error during deep dive is non-fatal: a card noting the failure is
// produced and ingest of later items continues.
func TestDeepDiveBrowserErrorIsNonFatal(t *testing.T) {
	store := newStore(t)
	src := &StaticSource{Items: []Item{{ID: "1", Title: "命中", Text: "alpha"}}}
	p := &Pipeline{
		Store:  store,
		Source: src,
		Browser: FuncBrowser(func(context.Context, string, string) (string, error) {
			return "", errors.New("网络超时")
		}),
		Rule:           KeywordRule("alpha"),
		DeepDiveBudget: 1,
	}
	res, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("Run should not fail on browser error: %v", err)
	}
	if len(res.Cards) != 1 || !strings.Contains(res.Cards[0].Body, "未完成") {
		t.Errorf("expected a failure-note card, got %+v", res.Cards)
	}
}

func TestRunRequiresStoreAndSource(t *testing.T) {
	if _, err := (&Pipeline{}).Run(context.Background()); err == nil {
		t.Error("expected error with nil store")
	}
	if _, err := (&Pipeline{Store: newStore(t)}).Run(context.Background()); err == nil {
		t.Error("expected error with nil source")
	}
}

const sampleRSS = `<?xml version="1.0"?>
<rss version="2.0"><channel>
  <title>Demo Feed</title>
  <item>
    <title>新品类 AI 硬件登场</title>
    <description>一家初创发布了全新形态的设备</description>
    <link>https://example.com/a</link>
    <guid>guid-a</guid>
  </item>
  <item>
    <title>常规市场快讯</title>
    <description>无新意</description>
    <link>https://example.com/b</link>
  </item>
</channel></rss>`

func TestRSSSourceParsesFeed(t *testing.T) {
	src := &RSSSource{
		SourceName: "rss:demo",
		FeedURL:    "https://example.com/feed",
		Domain:     "market",
		FetchXML: func(context.Context, string) ([]byte, error) {
			return []byte(sampleRSS), nil
		},
	}
	items, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Title != "新品类 AI 硬件登场" {
		t.Errorf("title = %q", items[0].Title)
	}
	if items[0].URL != "https://example.com/a" || items[0].ID != "guid-a" {
		t.Errorf("link/id = %q / %q", items[0].URL, items[0].ID)
	}
	if items[0].Domain != "market" {
		t.Errorf("domain = %q", items[0].Domain)
	}
	if !strings.Contains(items[0].Text, "全新形态") {
		t.Errorf("text missing description: %q", items[0].Text)
	}
	// Item without guid falls back to link as id.
	if items[1].ID != "https://example.com/b" {
		t.Errorf("fallback id = %q", items[1].ID)
	}
}

func TestRSSSourceNoFetcherErrors(t *testing.T) {
	src := &RSSSource{SourceName: "rss:x"}
	if _, err := src.Fetch(context.Background()); err == nil {
		t.Error("expected error when FetchXML is nil")
	}
}

// RSS feed → pipeline → kwiki, the integration the acceptance criteria call for
// (至少一个定时源自动入 Inbox 并完成 L1 分类).
func TestRSSSourceFeedsPipeline(t *testing.T) {
	store := newStore(t)
	src := &RSSSource{
		SourceName: "rss:demo",
		Domain:     "market",
		FetchXML:   func(context.Context, string) ([]byte, error) { return []byte(sampleRSS), nil },
	}
	p := &Pipeline{
		Store:          store,
		Source:         src,
		Browser:        StubBrowser{},
		Rule:           KeywordRule("新品类"),
		DeepDiveBudget: 3,
	}
	res, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Ingested) != 2 {
		t.Fatalf("expected 2 ingested, got %d", len(res.Ingested))
	}
	if len(res.Cards) != 1 {
		t.Fatalf("expected 1 card from the matching item, got %d", len(res.Cards))
	}
}
