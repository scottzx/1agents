package meta

import "strings"

// Task cards author their content as YAML-frontmatter Markdown: a leading
// `---` fenced block of machine-recognizable keys (acceptance, …) followed by
// the free-form body (background / process / expected result). Frontmatter is
// the single source of truth for those structured keys — the legacy
// AcceptanceCriteria column is only a fallback for pre-frontmatter rows.
//
// We parse only the tiny subset we author (inline scalar, `- ` list, and `|`/`>`
// block scalar for the keys we care about), so no YAML dependency is pulled in.

// SplitFrontmatter splits a document into its raw frontmatter block (without the
// `---` fences) and the body. When the doc has no leading `---` fence, it
// returns ("", doc).
func SplitFrontmatter(doc string) (frontmatter, body string) {
	s := strings.TrimPrefix(doc, "\uFEFF") // tolerate a leading BOM
	if !strings.HasPrefix(s, "---\n") && !strings.HasPrefix(s, "---\r\n") {
		return "", doc
	}
	// Drop the opening fence line.
	nl := strings.IndexByte(s, '\n')
	rest := s[nl+1:]
	lines := strings.Split(rest, "\n")
	for i, ln := range lines {
		if strings.TrimRight(ln, "\r") == "---" {
			fm := strings.Join(lines[:i], "\n")
			bd := strings.Join(lines[i+1:], "\n")
			return fm, strings.TrimLeft(bd, "\r\n")
		}
	}
	// No closing fence — treat the whole thing as body (malformed frontmatter).
	return "", doc
}

// FrontmatterAcceptance extracts the `acceptance` key from a document's
// frontmatter as plain text. List items are rendered as "- item" lines; inline
// and block scalars are returned as-is. Returns "" when absent.
func FrontmatterAcceptance(doc string) string {
	fm, _ := SplitFrontmatter(doc)
	if fm == "" {
		return ""
	}
	lines := strings.Split(fm, "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], "\r")
		rest, ok := topLevelKey(line, "acceptance")
		if !ok {
			continue
		}
		rest = strings.TrimSpace(rest)
		// Inline scalar: `acceptance: 文本`
		if rest != "" && rest != "|" && rest != ">" && rest != "|-" && rest != ">-" {
			return strings.TrimSpace(strings.Trim(rest, `"'`))
		}
		// Block scalar or list: collect the indented lines that follow.
		var items []string
		var block []string
		for j := i + 1; j < len(lines); j++ {
			ln := strings.TrimRight(lines[j], "\r")
			if strings.TrimSpace(ln) == "" {
				if rest == "" { // a blank line ends a list
					break
				}
				block = append(block, "")
				continue
			}
			if !isIndented(ln) { // dedent back to top level ends the value
				break
			}
			trimmed := strings.TrimSpace(ln)
			if item, isItem := strings.CutPrefix(trimmed, "- "); isItem {
				items = append(items, "- "+strings.TrimSpace(strings.Trim(item, `"'`)))
			} else {
				block = append(block, trimmed)
			}
		}
		if len(items) > 0 {
			return strings.Join(items, "\n")
		}
		return strings.TrimSpace(strings.Join(block, "\n"))
	}
	return ""
}

// topLevelKey reports whether line is an unindented `key:` mapping and returns
// the value part after the colon.
func topLevelKey(line, key string) (value string, ok bool) {
	if line == "" || line[0] == ' ' || line[0] == '\t' || line[0] == '-' {
		return "", false
	}
	prefix := key + ":"
	if !strings.HasPrefix(line, prefix) {
		return "", false
	}
	return line[len(prefix):], true
}

func isIndented(s string) bool {
	return strings.HasPrefix(s, " ") || strings.HasPrefix(s, "\t")
}
