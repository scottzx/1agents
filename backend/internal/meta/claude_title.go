package meta

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// ResolveClaudeSessionName searches ~/.claude/projects/*/sessions-index.json
// for the given acpSessionID, and returns the firstPrompt as the session title.
// (Moved verbatim from internal/agent/store.go.)
func ResolveClaudeSessionName(acpSessionID string) (string, error) {
	if acpSessionID == "" {
		return "", nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	projectsDir := filepath.Join(home, ".claude", "projects")

	// Read projects directory
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	type SessionEntry struct {
		SessionID   string `json:"sessionId"`
		FirstPrompt string `json:"firstPrompt"`
	}
	type SessionsIndex struct {
		Entries []SessionEntry `json:"entries"`
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		indexPath := filepath.Join(projectsDir, entry.Name(), "sessions-index.json")
		data, err := os.ReadFile(indexPath)
		if err != nil {
			continue
		}

		var idx SessionsIndex
		if err := json.Unmarshal(data, &idx); err != nil {
			continue
		}

		for _, item := range idx.Entries {
			if item.SessionID == acpSessionID {
				title := item.FirstPrompt
				title = cleanSessionTitle(title)
				return title, nil
			}
		}
	}

	return "", nil
}

func cleanSessionTitle(prompt string) string {
	// Strip HTML tags like <ide_opened_file>...</ide_opened_file> or <command-message>...</command-message>
	for {
		start := strings.Index(prompt, "<")
		if start == -1 {
			break
		}
		end := strings.Index(prompt[start:], ">")
		if end == -1 {
			break
		}
		prompt = prompt[:start] + prompt[start+end+1:]
	}

	prompt = strings.TrimSpace(prompt)
	// Replace newlines/multiple spaces with a single space
	prompt = strings.ReplaceAll(prompt, "\r", "")
	prompt = strings.ReplaceAll(prompt, "\n", " ")
	for strings.Contains(prompt, "  ") {
		prompt = strings.ReplaceAll(prompt, "  ", " ")
	}

	// Truncate to N characters
	const maxLen = 60
	runes := []rune(prompt)
	if len(runes) > maxLen {
		return string(runes[:maxLen-3]) + "..."
	}
	return prompt
}
