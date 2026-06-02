package workspace

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/chenhg5/cc-connect/config"
	"github.com/chenhg5/cc-connect/core"
)

var configDir string

func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	configDir = filepath.Join(home, ".1agents")
}

const configFile = "workspaces_dir.json"

// Workspace represents a single workspace entry.
type Workspace struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	Status      string `json:"status"`
	TerminalDir string `json:"terminalDir,omitempty"`
	ChatChannel string `json:"chatChannel,omitempty"`
}

// WorkspacesConfig is the top-level structure stored in workspaces_dir.json.
type WorkspacesConfig struct {
	Workspaces []Workspace `json:"workspaces"`
}

type Handler struct {
	tmuxSession string
}

func NewHandler(tmuxSession ...string) *Handler {
	session := ""
	if len(tmuxSession) > 0 {
		session = tmuxSession[0]
	}
	return &Handler{tmuxSession: session}
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

// LoadWorkspacesConfig loads and returns the current WorkspacesConfig.
func (h *Handler) LoadWorkspacesConfig() (*WorkspacesConfig, error) {
	return h.loadConfig()
}

// SaveWorkspacesConfig saves the provided WorkspacesConfig.
func (h *Handler) SaveWorkspacesConfig(cfg *WorkspacesConfig) error {
	return h.saveConfig(cfg)
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

	// Dynamically register this workspace as a CC-Connect project
	projName := ws.Name
	if projName == "" {
		projName = ws.ID
	}
	if config.ConfigPath != "" {
		err = config.AddPlatformToProject(projName, config.PlatformConfig{
			Type: "bridge",
		}, ws.Path, "claudecode")
		if err != nil {
			log.Printf("[workspace] ccconnect add project error: %v", err)
		} else {
			log.Printf("[workspace] Dynamically registered CC-Connect project %s at path %s", projName, ws.Path)
			
			// Trigger cc-connect to hot restart itself and reload the configuration!
			select {
			case core.RestartCh <- core.RestartRequest{}:
				log.Println("[workspace] Successfully requested CC-Connect process hot restart for configuration reload")
			default:
				log.Println("[workspace] CC-Connect hot restart already pending")
			}
		}
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
	wsToDelete := cfg.Workspaces[idx]
	cfg.Workspaces = append(cfg.Workspaces[:idx], cfg.Workspaces[idx+1:]...)
	if err := h.saveConfig(cfg); err != nil {
		log.Printf("[workspace] save error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Dynamically remove this workspace from CC-Connect projects config
	projName := wsToDelete.Name
	if projName == "" {
		projName = wsToDelete.ID
	}
	if config.ConfigPath != "" {
		err = config.RemoveProject(projName)
		if err != nil {
			log.Printf("[workspace] ccconnect remove project error: %v", err)
		} else {
			log.Printf("[workspace] Dynamically removed CC-Connect project %s", projName)
			
			// Trigger cc-connect to hot restart itself and reload the configuration!
			select {
			case core.RestartCh <- core.RestartRequest{}:
				log.Println("[workspace] Successfully requested CC-Connect process hot restart for configuration reload")
			default:
				log.Println("[workspace] CC-Connect hot restart already pending")
			}
		}
	}

	// Clean up tmux windows associated with this workspace
	if h.tmuxSession != "" {
		if exec.Command("tmux", "has-session", "-t", h.tmuxSession).Run() == nil {
			cmd := exec.Command("tmux", "list-windows", "-t", h.tmuxSession, "-F", "#{window_index}|#{window_name}")
			if out, err := cmd.Output(); err == nil {
				lines := strings.Split(strings.TrimSpace(string(out)), "\n")
				var windowsToKill []int
				var totalWindows int
				for _, line := range lines {
					if line == "" {
						continue
					}
					totalWindows++
					parts := strings.SplitN(line, "|", 2)
					if len(parts) != 2 {
						continue
					}
					idx, err1 := strconv.Atoi(parts[0])
					name := parts[1]
					if err1 != nil {
						continue
					}
					
					// Parse workspace ID from name: "{workspaceId}_{n}" or "{workspaceId}"
					wsID := name
					if lastUnderscore := strings.LastIndex(name, "_"); lastUnderscore > 0 {
						wsID = name[:lastUnderscore]
					}
					
					if wsID == id {
						windowsToKill = append(windowsToKill, idx)
					}
				}
				
				if len(windowsToKill) > 0 {
					// If we are about to kill all windows, create a placeholder "p" first to keep session alive
					if len(windowsToKill) >= totalWindows {
						_ = exec.Command("tmux", "new-window", "-t", h.tmuxSession, "-n", "p").Run()
					}
					
					// Kill target windows
					for _, idx := range windowsToKill {
						log.Printf("[workspace] Killing tmux window %d for deleted workspace %s", idx, id)
						_ = exec.Command("tmux", "kill-window", "-t", fmt.Sprintf("%s:%d", h.tmuxSession, idx)).Run()
					}
				}
			}
		}
	}

	writeJSON(w, map[string]interface{}{"ok": true})
}

// PickDirectory handles POST /api/workspace/pick-directory.
// It opens a native OS folder picker dialog and returns the selected path.
func (h *Handler) PickDirectory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path, err := pickDirectory()
	if err != nil {
		if isUserCancel(err) {
			writeJSON(w, map[string]string{"path": ""})
			return
		}
		log.Printf("[workspace] pick-directory error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"path": path})
}

func pickDirectory() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		return pickDirectoryDarwin()
	case "linux":
		return pickDirectoryLinux()
	default:
		return "", fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func pickDirectoryDarwin() (string, error) {
	script := `try
		POSIX path of (choose folder with prompt "选择工作空间目录")
	end try`
	cmd := exec.Command("osascript", "-e", script)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func pickDirectoryLinux() (string, error) {
	cmd := exec.Command("zenity", "--file-selection", "--directory", "--title=选择工作空间目录")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func isUserCancel(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "User canceled") ||
		strings.Contains(s, "canceled") ||
		strings.Contains(s, "exit status 1")
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[workspace] json encode error: %v", err)
	}
}

// ListDirectories handles GET /api/workspace/list-directories
func (h *Handler) ListDirectories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	pathParam := r.URL.Query().Get("path")
	pathParam = expandTilde(pathParam)
	var targetPath string

	if pathParam == "" || pathParam == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Printf("[workspace] os.UserHomeDir failed: %v", err)
			// Try manual environment lookups as a fallback for user home directory
			if h := os.Getenv("HOME"); h != "" {
				home = h
			} else if u := os.Getenv("USER"); u != "" {
				candidate := "/home/" + u
				if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
					home = candidate
				}
			}

			// If still empty or failed to find a valid directory, fall back to system root
			if home == "" {
				if runtime.GOOS == "windows" {
					drive := os.Getenv("SystemDrive")
					if drive != "" {
						home = drive + "\\"
					} else {
						home = "C:\\"
					}
				} else {
					home = "/"
				}
				log.Printf("[workspace] Falling back to system root directory: %s", home)
			}
		}
		targetPath = home
	} else {
		abs, err := filepath.Abs(pathParam)
		if err != nil {
			http.Error(w, "invalid path: "+err.Error(), http.StatusBadRequest)
			return
		}
		targetPath = abs
	}

	entries, err := os.ReadDir(targetPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type DirEntry struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}

	directories := []DirEntry{}
	for _, e := range entries {
		if e.IsDir() {
			name := e.Name()
			if name == "." || name == ".." {
				continue
			}
			directories = append(directories, DirEntry{
				Name: name,
				Path: filepath.Join(targetPath, name),
			})
		}
	}

	parentPath := filepath.Dir(targetPath)
	if parentPath == targetPath {
		parentPath = ""
	}

	writeJSON(w, map[string]any{
		"currentPath": targetPath,
		"parentPath":  parentPath,
		"directories": directories,
	})
}

// expandTilde expands a ~ prefix to the user's home directory.
func expandTilde(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~"+string(os.PathSeparator)) {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

