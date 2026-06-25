package agent

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Skill is one callable capability: YAML frontmatter (name/description) plus the
// markdown body (the skill's instructions). It is the "技能" object of the
// role≠skill model (RFC §4 决策 4): roles bind skills by name via RoleTemplate.Skills.
//
// The format is deliberately Claude Code / superpowers SKILL.md compatible
// (name + description frontmatter, markdown body), so existing skill files import
// verbatim. Extra Claude Code keys (allowed-tools, license, …) are tolerated and
// ignored rather than rejected.
type Skill struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`

	// Body is the markdown after the frontmatter (the skill instructions).
	Body string `yaml:"-"`

	// Source is set by the loader: "builtin" | "user" | "project".
	Source string `yaml:"-"`
}

// parseSkillMarkdown splits a SKILL.md into YAML frontmatter and a markdown body,
// mirroring parseRoleMarkdown. The only required field is name. Format:
//
//	---
//	name: deep-research
//	description: ...
//	---
//	<markdown instructions>
func parseSkillMarkdown(raw []byte) (*Skill, error) {
	content := strings.TrimSpace(strings.TrimPrefix(string(raw), bomPrefix))
	if !strings.HasPrefix(content, "---") {
		return nil, fmt.Errorf("missing frontmatter")
	}

	rest := content[3:]
	endIdx := strings.Index(rest, "\n---")
	if endIdx < 0 {
		return nil, fmt.Errorf("unterminated frontmatter")
	}
	fmBlock := rest[:endIdx]

	body := rest[endIdx+len("\n---"):]
	if nl := strings.IndexByte(body, '\n'); nl >= 0 {
		body = body[nl+1:]
	} else {
		body = ""
	}

	var sk Skill
	if err := yaml.Unmarshal([]byte(fmBlock), &sk); err != nil {
		return nil, fmt.Errorf("invalid frontmatter: %w", err)
	}
	sk.Body = strings.TrimSpace(body)
	if strings.TrimSpace(sk.Name) == "" {
		return nil, fmt.Errorf("skill missing name")
	}
	return &sk, nil
}
