package workspace

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Curated soul personas surfaced in the assistant-create picker (POC subset).
// The full library (4000+) is being ingested separately — see issue #368.
//
// Only the curated files (all under thedaviddias/) are embedded; the noisy
// auto-generated apeirography set (2.3M) is intentionally left out of the binary.
//
//go:embed presets/souls/curated.json
//go:embed presets/souls/thedaviddias/*.md
var soulsFS embed.FS

// soulManifestEntry mirrors one object in presets/souls/curated.json.
type soulManifestEntry struct {
	Ref       string `json:"ref"`
	File      string `json:"file"` // relative to presets/souls/
	TitleZh   string `json:"titleZh"`
	TitleEn   string `json:"titleEn"`
	SummaryZh string `json:"summaryZh"`
	SummaryEn string `json:"summaryEn"`
}

// SoulPreset is one curated persona as returned to the frontend, already
// localized to the requested language.
type SoulPreset struct {
	Ref     string `json:"ref"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Content string `json:"content"` // full SOUL.md markdown (for preview)
}

// soulWorkspaceFile is the per-assistant persona file. Its content is injected as
// the system prompt for the assistant's general chats (see agent chat handler).
const soulWorkspaceFile = "SOUL.md"

// loadSoulManifest parses the embedded curated.json.
func loadSoulManifest() ([]soulManifestEntry, error) {
	raw, err := soulsFS.ReadFile("presets/souls/curated.json")
	if err != nil {
		return nil, err
	}
	var doc struct {
		Souls []soulManifestEntry `json:"souls"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse souls manifest: %w", err)
	}
	return doc.Souls, nil
}

// soulContent returns the embedded markdown body for a manifest entry.
func soulContent(e soulManifestEntry) (string, error) {
	raw, err := soulsFS.ReadFile("presets/souls/" + e.File)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// listCuratedSouls returns the curated presets localized to language ("zh"|"en";
// anything non-"en" falls back to Chinese, matching the frontend default).
func listCuratedSouls(language string) ([]SoulPreset, error) {
	entries, err := loadSoulManifest()
	if err != nil {
		return nil, err
	}
	zh := language != "en"
	out := make([]SoulPreset, 0, len(entries))
	for _, e := range entries {
		content, err := soulContent(e)
		if err != nil {
			continue // a curated file that didn't embed is skipped, not fatal
		}
		p := SoulPreset{Ref: e.Ref, Content: content}
		if zh {
			p.Title, p.Summary = e.TitleZh, e.SummaryZh
		} else {
			p.Title, p.Summary = e.TitleEn, e.SummaryEn
		}
		out = append(out, p)
	}
	return out, nil
}

// seedSoulToWorkspace writes the chosen preset's markdown to <ws>/SOUL.md. An
// empty ref is a no-op (the "blank persona" choice — no file, no injection). An
// unknown ref is an error so the caller can surface it. An existing SOUL.md is
// overwritten (create-time seeding owns the file).
func seedSoulToWorkspace(workspacePath, ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}
	entries, err := loadSoulManifest()
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Ref != ref {
			continue
		}
		content, err := soulContent(e)
		if err != nil {
			return err
		}
		return writeWorkspaceSoul(workspacePath, content)
	}
	return fmt.Errorf("unknown soul preset %q", ref)
}

// seedSoulAsPrimaryAgent materializes a curated soul as the workspace's primary
// team agent: it writes the persona to <ws>/.claude/agents/<ref>.md (wrapped in
// a Claude-native subagent frontmatter header derived from the manifest entry)
// and records it as the primary in <ws>/.agents/team.json. An empty ref is a
// no-op — the 空人设 choice: no agent, no team, no injection. Returns the agent
// file name (<ref>.md) that was written.
func seedSoulAsPrimaryAgent(workspacePath, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", nil
	}
	entries, err := loadSoulManifest()
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if e.Ref != ref {
			continue
		}
		content, err := soulContent(e)
		if err != nil {
			return "", err
		}
		file := ref + ".md"
		dir := filepath.Join(workspacePath, ".claude", "agents")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(dir, file), []byte(renderAgentMarkdown(e, content)), 0o644); err != nil {
			return "", err
		}
		if err := WriteTeam(workspacePath, Team{Primary: file, Members: []string{file}}); err != nil {
			return "", err
		}
		// Convenience mirror for the generic .agents convention; non-fatal — the
		// agent file + team.json are the source of truth.
		_ = linkAgentsAgents(workspacePath)
		return file, nil
	}
	return "", fmt.Errorf("unknown soul preset %q", ref)
}

// renderAgentMarkdown wraps a soul's body in a Claude-native subagent frontmatter
// header (name + description) so the same file doubles as a .claude/agents/*.md
// agent. A body that already carries frontmatter is returned unchanged.
func renderAgentMarkdown(e soulManifestEntry, body string) string {
	if strings.HasPrefix(strings.TrimLeft(body, "\ufeff \t\r\n"), "---") {
		return body
	}
	desc := e.SummaryEn
	if desc == "" {
		desc = e.SummaryZh
	}
	desc = strings.ReplaceAll(strings.ReplaceAll(desc, "\n", " "), "\"", "'")
	return fmt.Sprintf("---\nname: %s\ndescription: \"%s\"\n---\n\n%s", e.Ref, desc, strings.TrimSpace(body))
}

// ReadWorkspaceSoul returns the workspace's SOUL.md content, or "" when absent.
// Exported for the agent chat handler, which prepends it to the session's system
// prompt (the persona injection point).
func ReadWorkspaceSoul(workspacePath string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(workspacePath, soulWorkspaceFile))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(raw), nil
}

// writeWorkspaceSoul writes (or overwrites) the workspace's SOUL.md. Writing an
// empty/whitespace-only body removes the file, so "clear the persona" leaves no
// dangling empty prompt to inject.
func writeWorkspaceSoul(workspacePath, content string) error {
	path := filepath.Join(workspacePath, soulWorkspaceFile)
	if strings.TrimSpace(content) == "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(workspacePath, 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
