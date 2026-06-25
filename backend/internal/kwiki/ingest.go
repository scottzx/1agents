package kwiki

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Ingest compiles one Inbox item into the wiki: it writes the raw text under
// raw/, derives summary/concepts/tags, writes (or updates) the wiki page under
// wiki/, appends an .ingested.json provenance record, and regenerates the
// wiki/index.md navigation. It returns the resulting page (with its slug).
//
// Re-ingesting an item with the same slug overwrites its raw + wiki files and
// preserves the page's original Created time.
func (s *Store) Ingest(item InboxItem) (WikiPage, error) {
	if strings.TrimSpace(item.Text) == "" {
		return WikiPage{}, fmt.Errorf("kwiki: ingest: empty text")
	}
	now := item.CapturedAt
	if now.IsZero() {
		now = time.Now()
	}
	title := item.Title
	if title == "" {
		title = deriveTitle(item.Text)
	}
	slug := deriveSlug(item.ID, title)
	domain := item.Domain
	if domain == "" {
		domain = "misc"
	}

	// 1. Persist raw capture.
	if err := os.WriteFile(s.rawPath(slug+".md"), []byte(item.Text), 0o644); err != nil {
		return WikiPage{}, fmt.Errorf("kwiki: write raw: %w", err)
	}

	// 2. Compile knowledge.
	concepts := extractConcepts(item.Text)
	summary := summarize(item.Text)
	tags := mergeTags(item.Tags, extractTags(item.Text, concepts))
	tags = mergeTags(tags, []string{domain}) // domain is always a tag

	created := now
	if existing, err := s.loadPage(slug); err == nil && !existing.Created.IsZero() {
		created = existing.Created
	}

	page := WikiPage{
		Slug:     slug,
		Title:    title,
		Domain:   domain,
		Source:   item.Source,
		Tags:     tags,
		Concepts: concepts,
		Summary:  summary,
		Created:  created,
		Updated:  now,
		Body:     renderBody(title, summary, concepts, item.Text),
	}
	data, err := page.Marshal()
	if err != nil {
		return WikiPage{}, err
	}
	if err := os.WriteFile(s.wikiPath(slug+".md"), data, 0o644); err != nil {
		return WikiPage{}, fmt.Errorf("kwiki: write wiki page: %w", err)
	}

	// 3. Provenance + 4. index.
	if err := s.appendIngestRecord(IngestRecord{
		ItemID:     item.ID,
		Slug:       slug,
		Title:      title,
		Source:     item.Source,
		Domain:     domain,
		Tags:       tags,
		IngestedAt: now,
	}); err != nil {
		return WikiPage{}, err
	}
	if err := s.RebuildIndex(); err != nil {
		return WikiPage{}, err
	}
	return page, nil
}

// renderBody assembles the wiki page Markdown body: summary, concept list, and
// the original captured text under a fenced heading.
func renderBody(title, summary string, concepts []string, raw string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", title)
	if summary != "" {
		fmt.Fprintf(&b, "%s\n\n", summary)
	}
	if len(concepts) > 0 {
		b.WriteString("## 概念\n\n")
		for _, c := range concepts {
			fmt.Fprintf(&b, "- %s\n", c)
		}
		b.WriteString("\n")
	}
	b.WriteString("## 原文\n\n")
	b.WriteString(strings.TrimSpace(raw))
	b.WriteString("\n")
	return b.String()
}

// deriveTitle takes the first non-empty line of text, trimmed to a short label.
func deriveTitle(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(line, "#> "))
		if line == "" {
			continue
		}
		r := []rune(line)
		if len(r) > 60 {
			return strings.TrimSpace(string(r[:60]))
		}
		return line
	}
	return "untitled"
}

// deriveSlug prefers a slugified item ID, else slugifies the title. Falls back
// to a timestamp-ish constant only when both are empty (callers always pass
// text, so title is never empty in practice).
func deriveSlug(id, title string) string {
	if s := slugifyTag(id); s != "" {
		return s
	}
	if s := slugifyTag(title); s != "" {
		return s
	}
	return "untitled"
}

// FileBack appends a conversation insight onto an existing wiki page's body and
// bumps its Updated time. Used to回写 discussion takeaways into the wiki. It
// errors if the page does not exist.
func (s *Store) FileBack(slug, insight string) error {
	insight = strings.TrimSpace(insight)
	if insight == "" {
		return fmt.Errorf("kwiki: fileback: empty insight")
	}
	page, err := s.loadPage(slug)
	if err != nil {
		return fmt.Errorf("kwiki: fileback: load %s: %w", slug, err)
	}
	now := time.Now()
	stamp := now.Format("2006-01-02")
	body := strings.TrimRight(page.Body, "\n")
	if !strings.Contains(body, "## 洞见回写") {
		body += "\n\n## 洞见回写\n"
	}
	body += fmt.Sprintf("\n- (%s) %s\n", stamp, insight)
	page.Body = body
	page.Updated = now

	data, err := page.Marshal()
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.wikiPath(slug+".md"), data, 0o644); err != nil {
		return fmt.Errorf("kwiki: fileback: write: %w", err)
	}
	return nil
}
