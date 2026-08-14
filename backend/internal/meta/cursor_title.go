package meta

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// ResolveCursorSessionTitle reads Cursor ACP's auto-generated session title
// from ~/.cursor/acp-sessions/<acpSessionID>/meta.json.
func ResolveCursorSessionTitle(acpSessionID string) (string, error) {
	if acpSessionID == "" {
		return "", nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(home, ".cursor", "acp-sessions", acpSessionID, "meta.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	var file struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return "", err
	}
	title := strings.TrimSpace(file.Title)
	if title == "" {
		return "", nil
	}
	return cleanSessionTitle(title), nil
}
