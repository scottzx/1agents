package kwiki

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s
}

func TestOpenCreatesLayout(t *testing.T) {
	root := t.TempDir()
	if _, err := Open(root); err != nil {
		t.Fatalf("Open: %v", err)
	}
	for _, d := range []string{rawDir, wikiDir, outputDir} {
		if fi, err := os.Stat(filepath.Join(root, d)); err != nil || !fi.IsDir() {
			t.Errorf("expected dir %s, err=%v", d, err)
		}
	}
	if _, err := Open(""); err == nil {
		t.Error("Open(\"\") should error")
	}
}

func TestIngestProducesRawWikiLogIndex(t *testing.T) {
	s := newStore(t)
	item := InboxItem{
		ID:         "item-1",
		Title:      "OpenAI 发布 GPT-6",
		Text:       "OpenAI 今天发布了 GPT-6，号称推理能力大幅提升。#市场 #AI",
		Source:     "rss",
		Domain:     "market",
		Tags:       []string{"competitor"},
		CapturedAt: time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC),
	}
	page, err := s.Ingest(item)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	// Acceptance 1a: raw original is persisted.
	rawData, err := os.ReadFile(s.rawPath(page.Slug + ".md"))
	if err != nil {
		t.Fatalf("read raw: %v", err)
	}
	if string(rawData) != item.Text {
		t.Errorf("raw mismatch: %q", rawData)
	}

	// Acceptance 1b: wiki page compiled with summary/concepts/tags frontmatter.
	wikiData, err := os.ReadFile(s.wikiPath(page.Slug + ".md"))
	if err != nil {
		t.Fatalf("read wiki: %v", err)
	}
	doc := string(wikiData)
	if !strings.HasPrefix(doc, "---\n") {
		t.Errorf("wiki page missing frontmatter:\n%s", doc)
	}
	for _, want := range []string{"market", "competitor", "summary:", "concepts:"} {
		if !strings.Contains(doc, want) {
			t.Errorf("wiki frontmatter missing %q:\n%s", want, doc)
		}
	}
	if page.Summary == "" {
		t.Error("expected non-empty summary")
	}
	if len(page.Concepts) == 0 {
		t.Errorf("expected concepts, got none (page=%+v)", page)
	}
	// domain always becomes a tag
	if !containsStr(page.Tags, "market") {
		t.Errorf("domain not in tags: %v", page.Tags)
	}
	// explicit hashtag captured
	if !containsStr(page.Tags, "ai") {
		t.Errorf("hashtag #AI not captured: %v", page.Tags)
	}

	// Acceptance 1c: .ingested.json provenance record.
	log, err := s.IngestLog()
	if err != nil {
		t.Fatalf("IngestLog: %v", err)
	}
	if len(log) != 1 {
		t.Fatalf("expected 1 ingest record, got %d", len(log))
	}
	if log[0].ItemID != "item-1" || log[0].Slug != page.Slug {
		t.Errorf("bad record: %+v", log[0])
	}

	// Acceptance 2: index.md generated with tag nav + link.
	idx, err := os.ReadFile(s.indexPath())
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	idxStr := string(idx)
	if !strings.Contains(idxStr, "## #market") {
		t.Errorf("index missing tag heading:\n%s", idxStr)
	}
	if !strings.Contains(idxStr, "("+page.Slug+".md)") {
		t.Errorf("index missing page link:\n%s", idxStr)
	}
}

func TestIngestEmptyTextErrors(t *testing.T) {
	s := newStore(t)
	if _, err := s.Ingest(InboxItem{Text: "   "}); err == nil {
		t.Error("expected error on empty text")
	}
}

func TestReingestPreservesCreatedAndOverwrites(t *testing.T) {
	s := newStore(t)
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	first, err := s.Ingest(InboxItem{ID: "x", Title: "稳定标题", Text: "第一版内容", CapturedAt: t0})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	t1 := t0.Add(48 * time.Hour)
	second, err := s.Ingest(InboxItem{ID: "x", Title: "稳定标题", Text: "第二版内容更新", CapturedAt: t1})
	if err != nil {
		t.Fatalf("re-Ingest: %v", err)
	}
	if first.Slug != second.Slug {
		t.Errorf("slug changed on re-ingest: %s vs %s", first.Slug, second.Slug)
	}
	if !second.Created.Equal(t0) {
		t.Errorf("Created not preserved: got %v want %v", second.Created, t0)
	}
	if !second.Updated.Equal(t1) {
		t.Errorf("Updated not bumped: got %v want %v", second.Updated, t1)
	}
	// raw overwritten
	raw, _ := os.ReadFile(s.rawPath(second.Slug + ".md"))
	if string(raw) != "第二版内容更新" {
		t.Errorf("raw not overwritten: %q", raw)
	}
	// pages count stays 1 (overwrite, not append)
	pages, _ := s.Pages()
	if len(pages) != 1 {
		t.Errorf("expected 1 page after re-ingest, got %d", len(pages))
	}
	// but two provenance records (history is append-only)
	log, _ := s.IngestLog()
	if len(log) != 2 {
		t.Errorf("expected 2 ingest records, got %d", len(log))
	}
}

func TestFileBack(t *testing.T) {
	s := newStore(t)
	page, err := s.Ingest(InboxItem{ID: "p", Title: "健康趋势", Text: "体检报告：血压略高"})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if err := s.FileBack(page.Slug, "讨论结论：增加有氧运动"); err != nil {
		t.Fatalf("FileBack: %v", err)
	}
	if err := s.FileBack(page.Slug, "第二条洞见"); err != nil {
		t.Fatalf("FileBack 2: %v", err)
	}
	reloaded, err := s.loadPage(page.Slug)
	if err != nil {
		t.Fatalf("loadPage: %v", err)
	}
	if !strings.Contains(reloaded.Body, "## 洞见回写") {
		t.Errorf("missing fileback section:\n%s", reloaded.Body)
	}
	if !strings.Contains(reloaded.Body, "增加有氧运动") || !strings.Contains(reloaded.Body, "第二条洞见") {
		t.Errorf("filebacks not appended:\n%s", reloaded.Body)
	}
	if strings.Count(reloaded.Body, "## 洞见回写") != 1 {
		t.Errorf("expected single fileback heading, got %d", strings.Count(reloaded.Body, "## 洞见回写"))
	}
	if err := s.FileBack("does-not-exist", "x"); err == nil {
		t.Error("FileBack on missing page should error")
	}
	if err := s.FileBack(page.Slug, "  "); err == nil {
		t.Error("FileBack with empty insight should error")
	}
}

func TestLintBrokenLinkAndOrphan(t *testing.T) {
	s := newStore(t)
	// alpha links to beta (exists) and to ghost (missing).
	mustWritePage(t, s, "alpha", WikiPage{
		Slug:  "alpha",
		Title: "Alpha",
		Tags:  []string{"x"},
		Body:  "# Alpha\n\nsee [beta](beta.md) and [ghost](ghost.md) and [ext](https://e.com).\n",
	})
	mustWritePage(t, s, "beta", WikiPage{Slug: "beta", Title: "Beta", Tags: []string{"x"}, Body: "# Beta\n"})
	// gamma is linked by nobody -> orphan.
	mustWritePage(t, s, "gamma", WikiPage{Slug: "gamma", Title: "Gamma", Tags: []string{"x"}, Body: "# Gamma\n"})

	report, err := s.Lint()
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	if report.OK() {
		t.Fatal("expected lint to fail")
	}
	if len(report.BrokenLinks) != 1 || report.BrokenLinks[0].Page != "alpha" || report.BrokenLinks[0].Target != "ghost" {
		t.Errorf("bad broken links: %+v", report.BrokenLinks)
	}
	// alpha and gamma are orphans (nothing links to them); beta is linked.
	if !equalStrSet(report.Orphans, []string{"alpha", "gamma"}) {
		t.Errorf("bad orphans: %v", report.Orphans)
	}
}

func TestLintCleanWiki(t *testing.T) {
	s := newStore(t)
	mustWritePage(t, s, "a", WikiPage{Slug: "a", Title: "A", Body: "# A\n[b](b.md)\n"})
	mustWritePage(t, s, "b", WikiPage{Slug: "b", Title: "B", Body: "# B\n[a](a.md)\n"})
	report, err := s.Lint()
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	if !report.OK() {
		t.Errorf("expected clean lint, got %+v", report)
	}
}

func TestPageRoundTrip(t *testing.T) {
	in := WikiPage{
		Slug:     "rt",
		Title:    "标题: 带冒号",
		Domain:   "market",
		Source:   "rss",
		Tags:     []string{"a", "b"},
		Concepts: []string{"GPT", "推理"},
		Summary:  "一句话摘要",
		Created:  time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC),
		Body:     "# 标题\n\n正文内容\n",
	}
	data, err := in.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	out, err := parsePage(data)
	if err != nil {
		t.Fatalf("parsePage: %v", err)
	}
	if out.Title != in.Title || out.Domain != in.Domain || out.Summary != in.Summary {
		t.Errorf("scalar round-trip mismatch: %+v", out)
	}
	if !equalStrSet(out.Tags, in.Tags) || !equalStrSet(out.Concepts, in.Concepts) {
		t.Errorf("list round-trip mismatch: tags=%v concepts=%v", out.Tags, out.Concepts)
	}
	if strings.TrimSpace(out.Body) != strings.TrimSpace(in.Body) {
		t.Errorf("body round-trip mismatch: %q", out.Body)
	}
}

func TestSlugifyAndDerive(t *testing.T) {
	cases := map[string]string{
		"Hello World":  "hello-world",
		"  C++ / Go  ": "c-go",
		"#市场情报":        "市场情报",
	}
	for in, want := range cases {
		if got := slugifyTag(in); got != want {
			t.Errorf("slugifyTag(%q)=%q want %q", in, got, want)
		}
	}
	if got := deriveTitle("# 第一行标题\n第二行"); got != "第一行标题" {
		t.Errorf("deriveTitle = %q", got)
	}
	// ID wins over title for slug.
	if got := deriveSlug("ITEM-42", "随便"); got != "item-42" {
		t.Errorf("deriveSlug = %q", got)
	}
}

// --- helpers ---

func mustWritePage(t *testing.T, s *Store, slug string, p WikiPage) {
	t.Helper()
	data, err := p.Marshal()
	if err != nil {
		t.Fatalf("Marshal %s: %v", slug, err)
	}
	if err := os.WriteFile(s.wikiPath(slug+".md"), data, 0o644); err != nil {
		t.Fatalf("write %s: %v", slug, err)
	}
}

func containsStr(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func equalStrSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := map[string]int{}
	for _, x := range a {
		m[x]++
	}
	for _, x := range b {
		m[x]--
	}
	for _, v := range m {
		if v != 0 {
			return false
		}
	}
	return true
}
