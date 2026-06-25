package kwiki

import (
	"regexp"
	"sort"
	"strings"
)

// mdLinkRe matches inline Markdown links [text](target). Only the target is
// captured; we filter to intra-wiki .md targets in Lint.
var mdLinkRe = regexp.MustCompile(`\[[^\]]*\]\(([^)]+)\)`)

// Lint walks every wiki page and reports:
//   - broken links: a [..](slug.md) link to a wiki page that does not exist;
//   - orphans: pages no other page links to (the generated index.md is ignored
//     as a link source, so index entries do not rescue a page from orphanhood).
//
// External links (http/https, mailto, anchors) and links into other layers are
// ignored — Lint only knows the wiki layer.
func (s *Store) Lint() (LintReport, error) {
	pages, err := s.Pages()
	if err != nil {
		return LintReport{}, err
	}
	exists := make(map[string]bool, len(pages))
	for _, p := range pages {
		exists[p.Slug] = true
	}

	report := LintReport{BrokenLinks: []BrokenLink{}, Orphans: []string{}}
	linkedTo := map[string]bool{}

	for _, p := range pages {
		for _, m := range mdLinkRe.FindAllStringSubmatch(p.Body, -1) {
			target := strings.TrimSpace(m[1])
			slug, ok := wikiLinkSlug(target)
			if !ok {
				continue // external / non-wiki link
			}
			if slug == p.Slug {
				continue // self-link doesn't rescue from orphanhood
			}
			if exists[slug] {
				linkedTo[slug] = true
			} else {
				report.BrokenLinks = append(report.BrokenLinks, BrokenLink{Page: p.Slug, Target: slug})
			}
		}
	}

	for _, p := range pages {
		if !linkedTo[p.Slug] {
			report.Orphans = append(report.Orphans, p.Slug)
		}
	}

	sort.Slice(report.BrokenLinks, func(i, j int) bool {
		if report.BrokenLinks[i].Page != report.BrokenLinks[j].Page {
			return report.BrokenLinks[i].Page < report.BrokenLinks[j].Page
		}
		return report.BrokenLinks[i].Target < report.BrokenLinks[j].Target
	})
	sort.Strings(report.Orphans)
	return report, nil
}

// wikiLinkSlug returns the wiki slug a Markdown link target points to, if it is
// an intra-wiki link. It accepts "slug.md" and "slug" but rejects external
// schemes, anchors, and paths into other directories.
func wikiLinkSlug(target string) (slug string, ok bool) {
	if target == "" {
		return "", false
	}
	if strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") || strings.HasPrefix(target, "#") {
		return "", false
	}
	// Strip any anchor fragment.
	if i := strings.IndexByte(target, '#'); i >= 0 {
		target = target[:i]
	}
	// Reject links that traverse directories (other layers).
	if strings.ContainsAny(target, "/\\") {
		return "", false
	}
	target = strings.TrimSuffix(target, ".md")
	if target == "" || target == indexFile || target == "index" {
		return "", false
	}
	return target, true
}
