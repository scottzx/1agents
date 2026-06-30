package main

import (
	"regexp"
	"sort"
	"strings"
)

// Feature is one parsed feature point extracted from a commit/PR title.
type Feature struct {
	Type    string // conventional-commit type: feat, fix, refactor, ...
	Scope   string // optional scope inside the parens: feat(scope): ...
	Summary string // human-readable description with type/scope/PR stripped
	PR      int    // squash-merge PR number from the trailing "(#NNN)", 0 if absent
	Raw     string // the original commit subject line
}

// Group is a set of features sharing the same conventional-commit type,
// rendered as one section in the HTML page.
type Group struct {
	Type     string
	Title    string // localized display title for the type
	Features []Feature
}

// titleRE matches conventional-commit subjects like:
//
//	feat(task-engine): 执行结果即提案 (#131) (#260)
//	fix: 修复崩溃
//
// Group 1 = type, group 3 = optional scope, group 5 = description (may carry
// trailing "(#NNN)" markers which are stripped separately).
var titleRE = regexp.MustCompile(`^(\w+)(\(([^)]*)\))?(!)?:\s*(.+)$`)

// prRE matches a trailing squash-merge PR marker "(#260)". The last such marker
// on the line is treated as the PR number for the merge.
var prRE = regexp.MustCompile(`\(#(\d+)\)`)

// typeOrder defines section ordering in the rendered page.
var typeOrder = []string{"feat", "fix", "perf", "refactor", "docs", "test", "build", "ci", "chore", "style", "revert"}

// typeTitles maps conventional-commit types to their display section title.
var typeTitles = map[string]string{
	"feat":     "新功能",
	"fix":      "问题修复",
	"perf":     "性能优化",
	"refactor": "重构",
	"docs":     "文档",
	"test":     "测试",
	"build":    "构建",
	"ci":       "持续集成",
	"chore":    "杂项",
	"style":    "样式",
	"revert":   "回滚",
}

// ParseSubject parses a single commit subject line into a Feature.
// ok is false when the line is not a conventional-commit subject (e.g. a merge
// commit or free-form message); such lines are skipped by the generator.
func ParseSubject(subject string) (Feature, bool) {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return Feature{}, false
	}
	m := titleRE.FindStringSubmatch(subject)
	if m == nil {
		return Feature{}, false
	}
	desc := strings.TrimSpace(m[5])

	// Extract the trailing PR number, then strip every PR marker from the
	// description so nested squash markers don't clutter the summary.
	pr := 0
	if locs := prRE.FindAllStringSubmatch(desc, -1); len(locs) > 0 {
		pr = atoiSafe(locs[len(locs)-1][1])
	}
	summary := strings.TrimSpace(prRE.ReplaceAllString(desc, ""))

	return Feature{
		Type:    strings.ToLower(m[1]),
		Scope:   m[3],
		Summary: summary,
		PR:      pr,
		Raw:     subject,
	}, true
}

// ParseLog turns a list of raw commit subject lines into parsed Features,
// skipping any line that is not a conventional-commit subject.
func ParseLog(subjects []string) []Feature {
	var out []Feature
	for _, s := range subjects {
		if f, ok := ParseSubject(s); ok {
			out = append(out, f)
		}
	}
	return out
}

// GroupFeatures buckets features by conventional-commit type and returns the
// groups in a stable display order (typeOrder first, then any unknown types
// alphabetically). Empty groups are omitted.
func GroupFeatures(features []Feature) []Group {
	byType := map[string][]Feature{}
	for _, f := range features {
		byType[f.Type] = append(byType[f.Type], f)
	}

	var groups []Group
	seen := map[string]bool{}
	for _, t := range typeOrder {
		if fs, ok := byType[t]; ok {
			groups = append(groups, Group{Type: t, Title: titleFor(t), Features: fs})
			seen[t] = true
		}
	}
	// Unknown types, sorted for determinism.
	var rest []string
	for t := range byType {
		if !seen[t] {
			rest = append(rest, t)
		}
	}
	sort.Strings(rest)
	for _, t := range rest {
		groups = append(groups, Group{Type: t, Title: titleFor(t), Features: byType[t]})
	}
	return groups
}

func titleFor(t string) string {
	if title, ok := typeTitles[t]; ok {
		return title
	}
	return t
}

func atoiSafe(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}
