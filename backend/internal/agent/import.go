package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ImportSubagent imports an existing Claude Code subagent file into the
// user-level role repo (~/.1agents/roles/<name>.md). Claude Code subagents are
// already YAML-frontmatter + markdown, so the file is parsed by the same
// parseRoleMarkdown loader and re-emitted in our schema. The two format
// differences we normalize:
//
//   - Claude Code's `tools:` is a comma-separated string ("Read, Edit, Bash");
//     ours is a structured allow/deny list. A string tools field is mapped to
//     RoleTools.Allow.
//   - Claude Code has no `engine`; we default it to claude-code so the imported
//     role is chat-ready against the default engine.
//
// It returns the written path. A role that doesn't parse (no frontmatter / no
// name) is rejected rather than written.
func ImportSubagent(raw []byte) (string, error) {
	tpl, err := parseImportableRole(raw)
	if err != nil {
		return "", err
	}
	if tpl.Engine == "" {
		tpl.Engine = "claude-code"
	}

	dir := userRolesDir()
	if dir == "" {
		return "", fmt.Errorf("cannot resolve user roles dir")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dir, err)
	}
	dst := filepath.Join(dir, tpl.Name+".md")
	if err := os.WriteFile(dst, renderRoleMarkdown(tpl), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", dst, err)
	}
	return dst, nil
}

// ImportSkill imports an existing Claude Code / superpowers SKILL.md into the
// user-level skill repo (~/.1agents/skills/<name>/SKILL.md). The format is
// already ours, so the file is parsed and written verbatim under the
// directory-per-skill convention. Returns the written path.
func ImportSkill(raw []byte) (string, error) {
	sk, err := parseSkillMarkdown(raw)
	if err != nil {
		return "", err
	}
	dir := userSkillsDir()
	if dir == "" {
		return "", fmt.Errorf("cannot resolve user skills dir")
	}
	skillDir := filepath.Join(dir, sk.Name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", skillDir, err)
	}
	dst := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(dst, raw, 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", dst, err)
	}
	return dst, nil
}

// parseImportableRole parses a role template the same way the loader does, but
// also tolerates Claude Code's comma-separated string `tools:` field, which the
// strict RoleTools struct can't unmarshal. When the structured parse fails on
// tools, it retries after rewriting a scalar tools line into an allow list.
func parseImportableRole(raw []byte) (*RoleTemplate, error) {
	if tpl, err := parseRoleMarkdown(raw); err == nil {
		return tpl, nil
	}
	// Retry: rewrite a scalar `tools:` line to the allow-list shape.
	rewritten, changed := rewriteScalarTools(raw)
	if !changed {
		// Re-run to surface the original error.
		return parseRoleMarkdown(raw)
	}
	return parseRoleMarkdown(rewritten)
}

// rewriteScalarTools turns a Claude Code-style `tools: Read, Edit, Bash` line
// inside the frontmatter into our structured form:
//
//	tools:
//	  allow: [Read, Edit, Bash]
//
// Returns the rewritten bytes and whether a rewrite happened. Only the first
// top-level `tools:` scalar in the frontmatter is rewritten.
func rewriteScalarTools(raw []byte) ([]byte, bool) {
	content := strings.TrimPrefix(string(raw), bomPrefix)
	if !strings.HasPrefix(strings.TrimSpace(content), "---") {
		return raw, false
	}
	lines := strings.Split(content, "\n")
	for i, ln := range lines {
		trimmed := strings.TrimRight(ln, "\r")
		rest, ok := strings.CutPrefix(trimmed, "tools:")
		if !ok {
			continue
		}
		val := strings.TrimSpace(rest)
		// Already structured (block mapping) or empty — leave alone.
		if val == "" || strings.HasPrefix(val, "[") {
			return raw, false
		}
		var items []string
		for _, p := range strings.Split(val, ",") {
			if p = strings.TrimSpace(p); p != "" {
				items = append(items, p)
			}
		}
		if len(items) == 0 {
			return raw, false
		}
		lines[i] = "tools:\n  allow: [" + strings.Join(items, ", ") + "]"
		return []byte(strings.Join(lines, "\n")), true
	}
	return raw, false
}

// renderRoleMarkdown serializes a RoleTemplate back to our YAML-frontmatter +
// markdown format. Only the fields we model are emitted; secrets are never
// included (the schema carries none). Used by ImportSubagent.
func renderRoleMarkdown(tpl *RoleTemplate) []byte {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "name: %s\n", tpl.Name)
	if tpl.Description != "" {
		fmt.Fprintf(&b, "description: %s\n", tpl.Description)
	}
	if tpl.Engine != "" {
		fmt.Fprintf(&b, "engine: %s\n", tpl.Engine)
	}
	if tpl.Model != "" {
		fmt.Fprintf(&b, "model: %s\n", tpl.Model)
	}
	if tpl.PermissionMode != "" {
		fmt.Fprintf(&b, "permission_mode: %s\n", tpl.PermissionMode)
	}
	if tpl.EffortLevel != "" {
		fmt.Fprintf(&b, "effort_level: %s\n", tpl.EffortLevel)
	}
	if len(tpl.Tools.Allow) > 0 || len(tpl.Tools.Deny) > 0 {
		b.WriteString("tools:\n")
		if len(tpl.Tools.Allow) > 0 {
			fmt.Fprintf(&b, "  allow: [%s]\n", strings.Join(tpl.Tools.Allow, ", "))
		}
		if len(tpl.Tools.Deny) > 0 {
			fmt.Fprintf(&b, "  deny: [%s]\n", strings.Join(tpl.Tools.Deny, ", "))
		}
	}
	if len(tpl.Skills) > 0 {
		fmt.Fprintf(&b, "skills: [%s]\n", strings.Join(tpl.Skills, ", "))
	}
	if len(tpl.McpServers) > 0 {
		fmt.Fprintf(&b, "mcp_servers: [%s]\n", strings.Join(tpl.McpServers, ", "))
	}
	b.WriteString("---\n")
	if tpl.Prompt != "" {
		b.WriteString(tpl.Prompt)
		if !strings.HasSuffix(tpl.Prompt, "\n") {
			b.WriteByte('\n')
		}
	}
	return []byte(b.String())
}
