package agent

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// bomPrefix is the UTF-8 byte-order mark some editors prepend; stripped before
// frontmatter parsing. Built from bytes to keep the BOM out of this source file.
var bomPrefix = string([]byte{0xEF, 0xBB, 0xBF})

// RoleTools is the allow/deny tool policy from a role template's frontmatter.
// Parsed and stored now; wiring it to the engine (allowedTools/disallowedTools)
// is deferred until the 1acp transport carries those fields — see #137 plan.
type RoleTools struct {
	Allow []string `yaml:"allow"`
	Deny  []string `yaml:"deny"`
}

// RoleTemplate is one agent role definition: structured fields from YAML
// frontmatter plus the markdown body as the system prompt. It replaces the
// hardcoded PM persona/MCP builders with a file-driven configuration (#137).
//
// Secrets never live here: the template only names which MCP servers to attach
// (McpServers); credentials are injected server-side in Go (see
// buildMcpServersFromRole).
type RoleTemplate struct {
	Name           string    `yaml:"name"`
	Description    string    `yaml:"description"`
	Engine         string    `yaml:"engine"` // hyphenated engine id; see engineToAgentType
	Model          string    `yaml:"model"`
	PermissionMode string    `yaml:"permission_mode"`
	EffortLevel    string    `yaml:"effort_level"`
	Tools          RoleTools `yaml:"tools"`
	Skills         []string  `yaml:"skills"`
	McpServers     []string  `yaml:"mcp_servers"`

	// Prompt is the markdown body after the frontmatter (the system prompt).
	Prompt string `yaml:"-"`

	// Resolution metadata, set by the loader (not from YAML).
	Source        string   `yaml:"-"` // "builtin" | "user" | "project"
	Path          string   `yaml:"-"` // on-disk path ("" for builtin); used by fork/restore
	Available     bool     `yaml:"-"` // engine installed / chat-ready
	Unavailable   string   `yaml:"-"` // human-readable reason when !Available
	MissingSkills []string `yaml:"-"` // skills named in Skills that didn't resolve
}

// parseRoleMarkdown splits a role template file into YAML frontmatter and a
// markdown body. Format:
//
//	---
//	name: pm
//	engine: claude-code
//	...
//	---
//	<markdown system prompt>
func parseRoleMarkdown(raw []byte) (*RoleTemplate, error) {
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

	// Skip past the closing "---" line to the body.
	body := rest[endIdx+len("\n---"):]
	if nl := strings.IndexByte(body, '\n'); nl >= 0 {
		body = body[nl+1:]
	} else {
		body = ""
	}

	var tpl RoleTemplate
	if err := yaml.Unmarshal([]byte(fmBlock), &tpl); err != nil {
		return nil, fmt.Errorf("invalid frontmatter: %w", err)
	}
	tpl.Prompt = strings.TrimSpace(body)
	if strings.TrimSpace(tpl.Name) == "" {
		return nil, fmt.Errorf("role template missing name")
	}
	return &tpl, nil
}

// engineToAgentType maps a template's hyphenated engine id (e.g. "claude-code")
// to the AgentType used internally (e.g. "claudecode"). An empty engine falls
// back to the default; an unknown engine passes through verbatim so the loader
// can mark it unavailable rather than silently rewriting it.
func engineToAgentType(engine string) AgentType {
	switch strings.ToLower(strings.TrimSpace(engine)) {
	case "":
		return DefaultAgentType
	case "claude-code", "claudecode", "claude":
		return AgentTypeClaudecode
	case "codex":
		return AgentTypeCodex
	case "cursor", "cursor-agent":
		return AgentTypeCursor
	case "gemini":
		return AgentTypeGemini
	case "devin":
		return AgentTypeDevin
	case "iflow":
		return AgentTypeIflow
	case "kimi":
		return AgentTypeKimi
	case "opencode":
		return AgentTypeOpencode
	case "pi":
		return AgentTypePi
	case "qoder":
		return AgentTypeQoder
	default:
		return AgentType(strings.TrimSpace(engine))
	}
}

// renderRolePrompt substitutes the per-session context placeholders in a role
// template's body. Only {{ProjectName}} and {{WorkspaceID}} are supported —
// just enough to reproduce buildPMSystemPrompt byte-for-byte.
func renderRolePrompt(tpl *RoleTemplate, projectName, workspaceID string) string {
	prompt := tpl.Prompt
	prompt = strings.ReplaceAll(prompt, "{{ProjectName}}", projectName)
	prompt = strings.ReplaceAll(prompt, "{{WorkspaceID}}", workspaceID)
	return prompt
}
