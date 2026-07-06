package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Team is the per-project agent-team manifest at <ws>/.agents/team.json.
//
// Primary names the agent (a <ws>/.claude/agents/<name>.md file) that drives the
// default conversation. An EMPTY Primary means no persona is injected — used by
// project-level folders that don't want a fixed personality; the user picks an
// expert per conversation from the input-box dropdown instead. Assistant folders
// default to a non-empty Primary (seeded at creation from the chosen soul).
//
// Members lists the agents offered in that expert picker (file names <name>.md).
type Team struct {
	Primary string   `json:"primary"`
	Members []string `json:"members"`
}

// teamManifestPath is <ws>/.agents/team.json (sits beside the .agents/agents
// symlink that already points into .claude/agents).
func teamManifestPath(workspacePath string) string {
	return filepath.Join(workspacePath, ".agents", "team.json")
}

// ReadTeam loads the manifest. A missing file is not an error — it returns a
// zero Team (no primary, no members), matching a project that predates the team
// model or one that was never given a manifest.
func ReadTeam(workspacePath string) (Team, error) {
	var t Team
	raw, err := os.ReadFile(teamManifestPath(workspacePath))
	if err != nil {
		if os.IsNotExist(err) {
			return t, nil
		}
		return t, err
	}
	if err := json.Unmarshal(raw, &t); err != nil {
		return t, err
	}
	return t, nil
}

// WriteTeam persists the manifest to <ws>/.agents/team.json, creating .agents/
// if needed.
func WriteTeam(workspacePath string, t Team) error {
	dir := filepath.Join(workspacePath, ".agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(teamManifestPath(workspacePath), raw, 0o644)
}

// ResolveAgentPersona returns the system-prompt persona for a conversation.
//
// agentRef (optional) is an explicit expert pick from the input-box dropdown;
// when empty the project's primary agent is used. The persona is the BODY of the
// chosen <ws>/.claude/agents/<name>.md, with any Claude-native YAML frontmatter
// stripped.
//
// It returns "" (NOT an error — "no persona, no injection") when there is no
// pick, no primary, and no legacy SOUL.md. Back-compat: a workspace with no
// team.json but a SOUL.md (assistants created before the team model) still
// injects the SOUL.md body.
func ResolveAgentPersona(workspacePath, agentRef string) (string, error) {
	ref := strings.TrimSpace(agentRef)
	if ref == "" {
		team, err := ReadTeam(workspacePath)
		if err != nil {
			return "", err
		}
		ref = strings.TrimSpace(team.Primary)
	}
	if ref == "" {
		// No explicit pick and no primary: fall back to the legacy SOUL.md, or
		// inject nothing (empty primary is a valid "no personality" choice).
		return ReadWorkspaceSoul(workspacePath)
	}
	file, err := workspaceAgentFile(workspacePath, ref)
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	return agentPersonaBody(string(raw)), nil
}

// agentPersonaBody strips a leading Claude-native YAML frontmatter block
// (a first "---" line through the next "---" line) and returns the trimmed
// markdown body — the persona/system prompt. Content without frontmatter is
// returned trimmed as-is.
func agentPersonaBody(md string) string {
	s := strings.TrimLeft(md, "\ufeff \t\r\n")
	if strings.HasPrefix(s, "---\n") || strings.HasPrefix(s, "---\r\n") {
		lines := strings.SplitAfter(s, "\n")
		for i := 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "---" {
				return strings.TrimSpace(strings.Join(lines[i+1:], ""))
			}
		}
	}
	return strings.TrimSpace(md)
}
