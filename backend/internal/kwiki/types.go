package kwiki

import "time"

// InboxItem is one piece of captured external context handed to Ingest. It is a
// plain value object — kwiki does not own the Inbox (#60); the caller maps its
// own row into this struct. The minimum a raw capture needs is its origin text,
// where it came from, and when.
type InboxItem struct {
	// ID is a stable identifier from the caller's Inbox (optional). When empty,
	// Ingest derives a slug from Title/Source so the same item ingested twice
	// lands on the same wiki page.
	ID string `json:"id,omitempty"`
	// Title is a short human label; used for the slug and page heading. When
	// empty a title is derived from the first line of Text.
	Title string `json:"title,omitempty"`
	// Text is the raw captured content (article body, chat excerpt, …).
	Text string `json:"text"`
	// Source records where this came from: manual/im/email/rss/crawler/misc.
	Source string `json:"source,omitempty"`
	// Domain routes the page: work/market/personal/… (RFC §3.2). Defaults to
	// "misc". Also recorded as a frontmatter tag.
	Domain string `json:"domain,omitempty"`
	// Tags are caller-supplied tags merged with auto-extracted ones.
	Tags []string `json:"tags,omitempty"`
	// CapturedAt is when the item entered the Inbox. Defaults to now.
	CapturedAt time.Time `json:"capturedAt,omitempty"`
}

// WikiPage is the compiled-knowledge representation of an ingested item: a
// frontmatter header (tags/source/domain/summary) plus a Markdown body holding
// the extracted concepts and the original text.
type WikiPage struct {
	Slug     string    `yaml:"-"`
	Title    string    `yaml:"title"`
	Domain   string    `yaml:"domain,omitempty"`
	Source   string    `yaml:"source,omitempty"`
	Tags     []string  `yaml:"tags,omitempty"`
	Concepts []string  `yaml:"concepts,omitempty"`
	Summary  string    `yaml:"summary,omitempty"`
	Created  time.Time `yaml:"created,omitempty"`
	Updated  time.Time `yaml:"updated,omitempty"`
	// Body is the Markdown content after the frontmatter fence.
	Body string `yaml:"-"`
}

// IngestRecord is one line of .ingested.json provenance: which Inbox item was
// compiled into which wiki page, and when.
type IngestRecord struct {
	ItemID     string    `json:"itemId,omitempty"`
	Slug       string    `json:"slug"`
	Title      string    `json:"title"`
	Source     string    `json:"source,omitempty"`
	Domain     string    `json:"domain,omitempty"`
	Tags       []string  `json:"tags,omitempty"`
	IngestedAt time.Time `json:"ingestedAt"`
}

// LintReport is the result of Lint: broken intra-wiki links and orphan pages
// (pages no other page links to and that the index does not surface).
type LintReport struct {
	BrokenLinks []BrokenLink `json:"brokenLinks"`
	Orphans     []string     `json:"orphans"`
}

// OK reports whether the wiki passed lint clean (no broken links, no orphans).
func (r LintReport) OK() bool { return len(r.BrokenLinks) == 0 && len(r.Orphans) == 0 }

// BrokenLink is a Markdown link in Page that points at a wiki page (Target)
// which does not exist.
type BrokenLink struct {
	Page   string `json:"page"`
	Target string `json:"target"`
}
