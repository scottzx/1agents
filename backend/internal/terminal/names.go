package terminal

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
)

// sessionNamesConfig is the persisted shape of ~/.1agents/session_names.json.
// Keys are tmux window names (e.g. "wsId_1"); values are the user's free-form
// display labels. The file lives alongside workspaces_dir.json so renames
// survive backend restarts.
type sessionNamesConfig struct {
	Names map[string]string `json:"names"`
}

// getSessionNamesPath returns the absolute path to session_names.json.
func getSessionNamesPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".1agents", "session_names.json")
}

// loadSessionNames returns the persisted names map. A missing file yields an
// empty map with no error. A corrupt file is logged and treated as empty — we
// never destroy user data on a parse failure.
func loadSessionNames() (map[string]string, error) {
	path := getSessionNamesPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	var cfg sessionNamesConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Printf("[terminal] session_names.json is corrupt, ignoring: %v", err)
		return map[string]string{}, nil
	}
	if cfg.Names == nil {
		cfg.Names = map[string]string{}
	}
	return cfg.Names, nil
}

// saveSessionNames writes the map atomically. Caller must hold h.nameMu.
func saveSessionNames(names map[string]string) error {
	dir := filepath.Dir(getSessionNamesPath())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(sessionNamesConfig{Names: names}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(getSessionNamesPath(), data, 0o644)
}

// SetSessionName upserts the display name for the given tmux window name. An
// empty name deletes the entry (reset to default). The full read-modify-write
// cycle is serialized via h.nameMu so a concurrent poll from listWindows
// cannot observe a torn state.
func (h *Handler) SetSessionName(windowName, name string) error {
	h.nameMu.Lock()
	defer h.nameMu.Unlock()

	names, err := loadSessionNames()
	if err != nil {
		return err
	}
	if name == "" {
		delete(names, windowName)
	} else {
		names[windowName] = name
	}
	return saveSessionNames(names)
}

// DeleteSessionName removes the persisted name for the given window. Safe to
// call as a best-effort step before killing a tmux window — errors are
// returned to the caller for logging but should not block the kill itself.
func (h *Handler) DeleteSessionName(windowName string) error {
	h.nameMu.Lock()
	defer h.nameMu.Unlock()

	names, err := loadSessionNames()
	if err != nil {
		return err
	}
	delete(names, windowName)
	return saveSessionNames(names)
}

// ApplySessionNames overlays the persisted custom names onto the given
// windows. Returns the same slice with each window's CustomName field set
// from the on-disk map (empty string when no custom name exists).
func (h *Handler) ApplySessionNames(windows []TmuxWindow) []TmuxWindow {
	names, err := loadSessionNames()
	if err != nil {
		log.Printf("[terminal] loadSessionNames error: %v", err)
		return windows
	}
	for i := range windows {
		windows[i].CustomName = names[windows[i].Name]
	}
	return windows
}
