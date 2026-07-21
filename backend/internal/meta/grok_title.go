package meta

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// ResolveGrokSessionTitle reads Grok Build's auto-generated session title from
// ~/.grok/sessions/<url-encoded-cwd>/<acpSessionID>/summary.json.
//
// Grok stores the absolute cwd as a single path segment percent-encoded so that
// '/' becomes "%2F" (url.PathEscape). Prefer the workspacePath-keyed lookup;
// when that misses (empty path, cwd drift, or symlinks), walk every cwd bucket
// for a matching session id — same strategy as ResolveClaudeSessionName.
//
// Prefer generated_title, fall back to session_summary. Titles are cleaned with
// the shared cleanSessionTitle helper (HTML strip + 60-char truncate).
func ResolveGrokSessionTitle(workspacePath, acpSessionID string) (string, error) {
	if acpSessionID == "" {
		return "", nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	sessionsRoot := filepath.Join(home, ".grok", "sessions")

	if workspacePath != "" {
		// Grok encodes the absolute cwd as one path segment via percent-encoding.
		enc := url.PathEscape(workspacePath)
		if title := readGrokSummaryTitle(filepath.Join(sessionsRoot, enc, acpSessionID, "summary.json")); title != "" {
			return title, nil
		}
	}

	entries, err := os.ReadDir(sessionsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if title := readGrokSummaryTitle(filepath.Join(sessionsRoot, entry.Name(), acpSessionID, "summary.json")); title != "" {
			return title, nil
		}
	}
	return "", nil
}

func readGrokSummaryTitle(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var summary struct {
		GeneratedTitle string `json:"generated_title"`
		SessionSummary string `json:"session_summary"`
	}
	if err := json.Unmarshal(data, &summary); err != nil {
		return ""
	}
	title := strings.TrimSpace(summary.GeneratedTitle)
	if title == "" {
		title = strings.TrimSpace(summary.SessionSummary)
	}
	if title == "" {
		return ""
	}
	return cleanSessionTitle(title)
}
