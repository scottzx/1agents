package kwiki

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// appendIngestRecord appends one record to the .ingested.json log (a JSON
// array). The log is read-modify-write; the volumes here are Inbox-scale (not
// hot-path), so simplicity beats an append-only format.
func (s *Store) appendIngestRecord(rec IngestRecord) error {
	records, err := s.IngestLog()
	if err != nil {
		return err
	}
	records = append(records, rec)
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return fmt.Errorf("kwiki: marshal ingest log: %w", err)
	}
	if err := os.WriteFile(s.ingestedLogPath(), data, 0o644); err != nil {
		return fmt.Errorf("kwiki: write ingest log: %w", err)
	}
	return nil
}

// IngestLog returns the provenance records in .ingested.json (empty when the
// log does not exist yet).
func (s *Store) IngestLog() ([]IngestRecord, error) {
	data, err := os.ReadFile(s.ingestedLogPath())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("kwiki: read ingest log: %w", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, nil
	}
	var records []IngestRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("kwiki: parse ingest log: %w", err)
	}
	return records, nil
}

// RebuildIndex regenerates wiki/index.md from the current wiki pages: a flat
// list grouped by frontmatter tag, with each page linked by slug. This is the
// navigation surface (no vector store).
func (s *Store) RebuildIndex() error {
	pages, err := s.Pages()
	if err != nil {
		return err
	}

	// tag -> page slugs
	byTag := map[string][]WikiPage{}
	for _, p := range pages {
		tags := p.Tags
		if len(tags) == 0 {
			tags = []string{"untagged"}
		}
		for _, t := range tags {
			byTag[t] = append(byTag[t], p)
		}
	}
	tags := make([]string, 0, len(byTag))
	for t := range byTag {
		tags = append(tags, t)
	}
	sort.Strings(tags)

	var b strings.Builder
	b.WriteString("# Wiki 索引\n\n")
	fmt.Fprintf(&b, "_共 %d 页，自动生成，请勿手改。_\n\n", len(pages))
	if len(pages) == 0 {
		b.WriteString("(暂无页面)\n")
	}
	for _, t := range tags {
		fmt.Fprintf(&b, "## #%s\n\n", t)
		list := byTag[t]
		sort.Slice(list, func(i, j int) bool { return list[i].Title < list[j].Title })
		for _, p := range list {
			title := p.Title
			if title == "" {
				title = p.Slug
			}
			if p.Summary != "" {
				fmt.Fprintf(&b, "- [%s](%s.md) — %s\n", title, p.Slug, p.Summary)
			} else {
				fmt.Fprintf(&b, "- [%s](%s.md)\n", title, p.Slug)
			}
		}
		b.WriteString("\n")
	}
	if err := os.WriteFile(s.indexPath(), []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("kwiki: write index: %w", err)
	}
	return nil
}

// Pages loads every wiki page (excluding the generated index.md), sorted by
// slug.
func (s *Store) Pages() ([]WikiPage, error) {
	entries, err := os.ReadDir(s.wikiPath(""))
	if err != nil {
		return nil, fmt.Errorf("kwiki: read wiki dir: %w", err)
	}
	var pages []WikiPage
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".md") || name == indexFile {
			continue
		}
		slug := strings.TrimSuffix(name, ".md")
		p, err := s.loadPage(slug)
		if err != nil {
			return nil, err
		}
		pages = append(pages, p)
	}
	sort.Slice(pages, func(i, j int) bool { return pages[i].Slug < pages[j].Slug })
	return pages, nil
}
