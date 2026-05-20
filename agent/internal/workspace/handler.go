package workspace

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

const configDir = "/Users/scott/.remote-agents"
const configFile = "workspaces_dir.json"

// Workspace represents a single workspace entry.
type Workspace struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

// WorkspacesConfig is the top-level structure stored in workspaces_dir.json.
type WorkspacesConfig struct {
	Workspaces []Workspace `json:"workspaces"`
}

type Handler struct{}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) ensureConfigDir() error {
	return os.MkdirAll(configDir, 0o755)
}

func (h *Handler) getConfigPath() string {
	return filepath.Join(configDir, configFile)
}

func (h *Handler) loadConfig() (*WorkspacesConfig, error) {
	path := h.getConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &WorkspacesConfig{Workspaces: []Workspace{}}, nil
		}
		return nil, err
	}
	var cfg WorkspacesConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (h *Handler) saveConfig(cfg *WorkspacesConfig) error {
	if err := h.ensureConfigDir(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(h.getConfigPath(), data, 0o644)
}

// List handles GET /api/workspace/list
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cfg, err := h.loadConfig()
	if err != nil {
		log.Printf("[workspace] load error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, cfg.Workspaces)
}

// Create handles POST /api/workspace/create
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var ws Workspace
	if err := json.NewDecoder(r.Body).Decode(&ws); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cfg, err := h.loadConfig()
	if err != nil {
		log.Printf("[workspace] load error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Check for duplicate ID
	for _, existing := range cfg.Workspaces {
		if existing.ID == ws.ID {
			http.Error(w, "workspace with this ID already exists", http.StatusConflict)
			return
		}
	}
	cfg.Workspaces = append(cfg.Workspaces, ws)
	if err := h.saveConfig(cfg); err != nil {
		log.Printf("[workspace] save error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "workspace": ws})
}

// Update handles POST /api/workspace/update
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var ws Workspace
	if err := json.NewDecoder(r.Body).Decode(&ws); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cfg, err := h.loadConfig()
	if err != nil {
		log.Printf("[workspace] load error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	found := false
	for i, existing := range cfg.Workspaces {
		if existing.ID == ws.ID {
			cfg.Workspaces[i] = ws
			found = true
			break
		}
	}
	if !found {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	if err := h.saveConfig(cfg); err != nil {
		log.Printf("[workspace] save error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "workspace": ws})
}

// Delete handles DELETE /api/workspace/delete
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id query parameter", http.StatusBadRequest)
		return
	}
	cfg, err := h.loadConfig()
	if err != nil {
		log.Printf("[workspace] load error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	idx := -1
	for i, ws := range cfg.Workspaces {
		if ws.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	cfg.Workspaces = append(cfg.Workspaces[:idx], cfg.Workspaces[idx+1:]...)
	if err := h.saveConfig(cfg); err != nil {
		log.Printf("[workspace] save error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[workspace] json encode error: %v", err)
	}
}
