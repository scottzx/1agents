package research

import (
	"context"
	"encoding/xml"
	"fmt"
	"strings"
)

// RSSSource is a定时 pull 源 that turns an RSS/Atom feed into research Items.
// 网络抓取被抽到 fetchFn 上 (默认 nil → 报错)，这样单测可以注入一段离线 XML，
// 不依赖实时网络 (符合范围要求：接口 + 占位实现，可测)。
type RSSSource struct {
	// SourceName labels the source in provenance ("rss:hn", "rss:producthunt").
	SourceName string
	// FeedURL is the feed location, passed to fetchFn and carried as item URL when
	// an entry has none.
	FeedURL string
	// Domain routes ingested pages (market/work/…); empty → kwiki defaults "misc".
	Domain string
	// FetchXML returns the raw feed bytes. Injected so production wires a real
	// HTTP getter while tests pass a fixed feed. nil → Fetch errors.
	FetchXML func(ctx context.Context, url string) ([]byte, error)
}

func (s *RSSSource) Name() string {
	if s.SourceName != "" {
		return s.SourceName
	}
	return "rss"
}

// Fetch pulls the feed via FetchXML and parses it into Items. Supports both RSS
// 2.0 (<channel><item>) and Atom (<feed><entry>) with a single permissive struct.
func (s *RSSSource) Fetch(ctx context.Context) ([]Item, error) {
	if s.FetchXML == nil {
		return nil, fmt.Errorf("research: RSSSource %q has no FetchXML", s.Name())
	}
	raw, err := s.FetchXML(ctx, s.FeedURL)
	if err != nil {
		return nil, err
	}
	return s.parse(raw)
}

// feedXML is a permissive union of RSS 2.0 and Atom shapes; xml.Unmarshal fills
// whichever fields are present.
type feedXML struct {
	// RSS 2.0: rss > channel > item
	Channel struct {
		Items []feedEntry `xml:"item"`
	} `xml:"channel"`
	// Atom: feed > entry
	Entries []feedEntry `xml:"entry"`
}

type feedEntry struct {
	Title       string `xml:"title"`
	Description string `xml:"description"`
	Summary     string `xml:"summary"`
	Content     string `xml:"content"`
	GUID        string `xml:"guid"`
	ID          string `xml:"id"`
	// RSS uses a text <link>; Atom uses <link href="..."/>. Capture both.
	Link     string `xml:"link"`
	LinkHref string `xml:"-"`
}

func (s *RSSSource) parse(raw []byte) ([]Item, error) {
	var feed feedXML
	if err := xml.Unmarshal(raw, &feed); err != nil {
		return nil, fmt.Errorf("research: parse feed %q: %w", s.Name(), err)
	}
	entries := feed.Channel.Items
	if len(entries) == 0 {
		entries = feed.Entries
	}

	items := make([]Item, 0, len(entries))
	for _, e := range entries {
		title := strings.TrimSpace(e.Title)
		if title == "" {
			continue // an entry with no title can't seed a wiki page
		}
		body := firstNonEmpty(e.Content, e.Description, e.Summary)
		link := firstNonEmpty(e.Link, e.LinkHref)
		id := firstNonEmpty(e.GUID, e.ID, link)
		text := title
		if body != "" {
			text = title + "\n\n" + strings.TrimSpace(body)
		}
		items = append(items, Item{
			ID:     id,
			Title:  title,
			Text:   text,
			URL:    link,
			Domain: s.Domain,
		})
	}
	return items, nil
}

// StaticSource is a fixed in-memory源 — used by the 市场榜单 (Top50) demo wiring
// and by tests. It returns its Items verbatim.
type StaticSource struct {
	SourceName string
	Items      []Item
}

func (s *StaticSource) Name() string {
	if s.SourceName != "" {
		return s.SourceName
	}
	return "static"
}

func (s *StaticSource) Fetch(context.Context) ([]Item, error) { return s.Items, nil }

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
