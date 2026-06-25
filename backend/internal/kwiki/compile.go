package kwiki

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// compile is the lightweight "編譯知識" step: from raw text it derives a short
// summary, a handful of candidate concepts, and auto tags. It is intentionally
// dumb heuristics, no NLP/vector deps (简单优先) — a smarter agent-backed
// compiler can replace it later behind the same signature.

const (
	summaryMaxLen = 200 // runes
	maxConcepts   = 8
	maxAutoTags   = 6
)

var (
	// A "concept" candidate: a CamelCase/TitleCase word, an ALLCAPS acronym, a
	// quoted phrase, or a run of CJK characters. Good enough to surface the
	// salient nouns a human would tag.
	conceptRe = regexp.MustCompile(`"([^"]{2,40})"|“([^”]{2,40})”|\b([A-Z][a-zA-Z0-9]+(?:[A-Z][a-zA-Z0-9]+)+)\b|\b([A-Z]{2,8})\b|([\p{Han}]{2,12})`)
	// Hashtag-style explicit tags in the text: #foo, #市场.
	hashtagRe = regexp.MustCompile(`#([\p{L}\p{N}_\-]{2,30})`)
)

// summarize returns the first sentence/line of text, trimmed to summaryMaxLen
// runes.
func summarize(text string) string {
	t := strings.TrimSpace(text)
	if t == "" {
		return ""
	}
	// First non-empty line.
	for _, line := range strings.Split(t, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			t = line
			break
		}
	}
	// Cut at the first sentence terminator (inclusive) if it is reasonably early.
	if i := strings.IndexAny(t, "。.!?！？"); i > 0 && i < summaryMaxLen {
		_, size := utf8.DecodeRuneInString(t[i:])
		t = t[:i+size]
	}
	r := []rune(t)
	if len(r) > summaryMaxLen {
		return strings.TrimSpace(string(r[:summaryMaxLen])) + "…"
	}
	return t
}

// extractConcepts pulls candidate concepts out of text, deduped (case-folded)
// and capped at maxConcepts, preserving first-seen order.
func extractConcepts(text string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range conceptRe.FindAllStringSubmatch(text, -1) {
		var c string
		for _, g := range m[1:] {
			if g != "" {
				c = strings.TrimSpace(g)
				break
			}
		}
		if c == "" {
			continue
		}
		key := strings.ToLower(c)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, c)
		if len(out) >= maxConcepts {
			break
		}
	}
	return out
}

// extractTags collects explicit #hashtags from text plus a lowercased,
// slugified subset of concepts, merged (deduped) and capped.
func extractTags(text string, concepts []string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(t string) {
		t = slugifyTag(t)
		if t == "" || seen[t] {
			return
		}
		seen[t] = true
		out = append(out, t)
	}
	for _, m := range hashtagRe.FindAllStringSubmatch(text, -1) {
		add(m[1])
	}
	for _, c := range concepts {
		if len(out) >= maxAutoTags {
			break
		}
		add(c)
	}
	if len(out) > maxAutoTags {
		out = out[:maxAutoTags]
	}
	return out
}

// mergeTags unions two tag lists (slugified, deduped) and sorts for stable
// frontmatter output.
func mergeTags(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range append(append([]string{}, a...), b...) {
		t = slugifyTag(t)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// slugifyTag normalizes a tag to lowercase, spaces/punct → hyphen, CJK kept.
func slugifyTag(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastHyphen := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			b.WriteRune(r)
			lastHyphen = false
		case r == '-' || r == '_':
			b.WriteRune(r)
			lastHyphen = false
		default:
			if !lastHyphen && b.Len() > 0 {
				b.WriteByte('-')
				lastHyphen = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
