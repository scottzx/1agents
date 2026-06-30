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
	"time"

	"github.com/chenhg5/cc-connect/config"
	"github.com/chenhg5/cc-connect/core"
	"github.com/scottzx/1Agents/backend/internal/meta"
)

func get1AgentsHome() string {
	if val := os.Getenv("ONEAGENTS_HOME"); val != "" {
		return val
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return home
}

// Workspace represents a single workspace entry.
type Workspace struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Path            string   `json:"path"`
	Status          string   `json:"status"`
	TerminalDir     string   `json:"terminalDir,omitempty"`
	ChatChannel     string   `json:"chatChannel,omitempty"`
	DefaultAgent    string   `json:"defaultAgent,omitempty"`
	Builtin         bool     `json:"builtin,omitempty"`
	// AvailableAgents is the allowlist of agent type slugs that may run in
	// this workspace (§325). Empty means unrestricted.
	AvailableAgents []string `json:"availableAgents,omitempty"`
}

// WorkspacesConfig is the top-level workspace registry shape. Its persistence
// has moved from workspaces_dir.json into meta.db's projects table (a project IS
// a workspace); the struct is kept for API/JSON compatibility with the many
// callers of LoadWorkspacesConfig.
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

// projectToWorkspace maps a meta project row to the workspace registry shape.
func projectToWorkspace(p meta.Project) Workspace {
	return Workspace{
		ID:              p.ID,
		Name:            p.Name,
		Path:            p.WorkspacePath,
		Status:          string(p.Status),
		TerminalDir:     p.TerminalDir,
		ChatChannel:     p.ChatChannel,
		DefaultAgent:    p.DefaultAgent,
		Builtin:         p.Builtin,
		AvailableAgents: p.AvailableAgents,
	}
}

// workspaceToProject maps a workspace into a meta project (write side).
func workspaceToProject(ws Workspace) meta.Project {
	return meta.Project{
		ID:              ws.ID,
		Name:            ws.Name,
		WorkspacePath:   ws.Path,
		TerminalDir:     ws.TerminalDir,
		ChatChannel:     ws.ChatChannel,
		DefaultAgent:    ws.DefaultAgent,
		Builtin:         ws.Builtin,
		AvailableAgents: ws.AvailableAgents,
	}
}

// loadConfig returns every workspace-backed project (any status, excluding the
// reserved personal bucket) in sidebar order — the faithful replacement for
// reading workspaces_dir.json. Status filtering (e.g. hiding archived projects
// from the sidebar) is applied by callers such as the List handler.
func (h *Handler) loadConfig() (*WorkspacesConfig, error) {
	db, err := meta.OpenDefault()
	if err != nil {
		return nil, err
	}
	projects, err := db.ListWorkspaceProjects()
	if err != nil {
		return nil, err
	}
	wss := make([]Workspace, 0, len(projects))
	for _, p := range projects {
		wss = append(wss, projectToWorkspace(p))
	}
	return &WorkspacesConfig{Workspaces: wss}, nil
}

// LoadWorkspacesConfig loads the workspace registry from meta.db. Returns all
// non-personal projects regardless of status (matching the legacy json), so
// id→path resolution still works for archived projects.
func (h *Handler) LoadWorkspacesConfig() (*WorkspacesConfig, error) {
	return h.loadConfig()
}

// SaveWorkspacesConfig reconciles the projects table to the given registry:
// upserts each workspace, deletes non-personal projects no longer present, and
// rewrites positions to match the given order. Kept for API compatibility; the
// CRUD handlers below write the table directly.
func (h *Handler) SaveWorkspacesConfig(cfg *WorkspacesConfig) error {
	db, err := meta.OpenDefault()
	if err != nil {
		return err
	}
	keep := make(map[string]bool, len(cfg.Workspaces))
	ids := make([]string, 0, len(cfg.Workspaces))
	for _, ws := range cfg.Workspaces {
		if err := db.EnsureWorkspaceProject(workspaceToProject(ws)); err != nil {
			return err
		}
		keep[ws.ID] = true
		ids = append(ids, ws.ID)
	}
	existing, err := db.ListWorkspaceProjects()
	if err != nil {
		return err
	}
	for _, p := range existing {
		if !keep[p.ID] {
			if err := db.DeleteProject(p.ID); err != nil {
				return err
			}
		}
	}
	return db.ReorderProjects(ids)
}

// EnsureDefaultWorkspace creates the built-in default workspace if it does not
// already exist. Called once at server startup so new installs skip onboarding.
func (h *Handler) EnsureDefaultWorkspace() error {
	db, err := meta.OpenDefault()
	if err != nil {
		return err
	}
	if _, ok, err := db.GetProject("default"); err != nil {
		return err
	} else if ok {
		return nil
	}
	homeDir := get1AgentsHome()
	defaultPath := filepath.Join(homeDir, ".1agents", "projects", "default")
	if err := os.MkdirAll(defaultPath, 0o755); err != nil {
		return fmt.Errorf("create default workspace dir: %w", err)
	}
	ensureProjectGuideFiles(defaultPath)
	if err := db.EnsureWorkspaceProject(meta.Project{
		ID:            "default",
		Name:          "对话",
		WorkspacePath: defaultPath,
		DefaultAgent:  "claudecode",
		Builtin:       true,
	}); err != nil {
		return err
	}
	log.Printf("[workspace] created built-in default workspace at %s", defaultPath)
	return nil
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
	// Sidebar shows only active workspaces — archived/killed projects (#141) drop
	// out of the registry view while staying queryable by id for the archive board.
	active := make([]Workspace, 0, len(cfg.Workspaces))
	for _, ws := range cfg.Workspaces {
		if ws.Status == "" || ws.Status == string(meta.ProjectStatusActive) {
			active = append(active, ws)
		}
	}
	writeJSON(w, active)
}

// RegisterIncubatedProject wires the cc-connect bridge + guide files for a
// project promoted via 立项 (the project row is already persisted by
// meta.Incubate as a workspace). Passed as the onIncubated hook to
// meta.PersonalTaskItemHandler so promotion gets a working bridge without meta
// importing the workspace package.
func (h *Handler) RegisterIncubatedProject(p meta.Project) {
	h.registerWorkspaceProject(projectToWorkspace(p))
}

// registerWorkspaceProject performs the side-effects of bringing a workspace
// online: agent guidance files + dynamic CC-Connect bridge registration + hot
// restart. Shared by Create and the 立项 (Incubate) hook so a promoted project
// gets a working bridge exactly like a hand-created workspace. Persisting the
// project row is the caller's job; this only does the non-storage side-effects.
func (h *Handler) registerWorkspaceProject(ws Workspace) {
	// Ensure the project has agent guidance files (CLAUDE.md / AGENTS.md).
	ensureProjectGuideFiles(ws.Path)

	// Dynamically register this workspace as a CC-Connect project.
	projName := ws.Name
	if projName == "" {
		projName = ws.ID
	}
	if config.ConfigPath != "" {
		if err := config.AddPlatformToProject(projName, config.PlatformConfig{
			Type: "bridge",
		}, ws.Path, "claudecode"); err != nil {
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

			// Wait for the hot restart to finish to avoid 502 Bad Gateway race condition on immediate redirect
			time.Sleep(1000 * time.Millisecond)
		}
	}
}

// Create handles POST /api/workspace/create
// Optional body field templateId: when set, Scaffold is called after the
// project row is persisted (creates artifact subdirs + domain tables + seeds
// project_config). The workspace is created even when Scaffold fails — the
// error is logged and returned in a non-fatal "scaffoldError" response field.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Workspace
		TemplateID string `json:"templateId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ws := body.Workspace
	// Backfill default agent if missing — matches the existing
	// "claudecode" default in handler.go:175 + the agent.DefaultAgentType
	// constant in internal/agent/types.go.
	if ws.DefaultAgent == "" {
		ws.DefaultAgent = "claudecode"
	}
	db, err := meta.OpenDefault()
	if err != nil {
		log.Printf("[workspace] db open error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if ws.ID == "temp" || ws.Path == "temp" {
		homeDir := get1AgentsHome()
		tempDir := filepath.Join(homeDir, "temp")
		_ = os.MkdirAll(tempDir, 0755)
		ws.Path = tempDir
	}
	if ws.ID == "" {
		ws.ID = meta.NewID()
	}
	// Check for duplicate ID
	if _, ok, err := db.GetProject(ws.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	} else if ok {
		http.Error(w, "workspace with this ID already exists", http.StatusConflict)
		return
	}
	if err := db.EnsureWorkspaceProject(workspaceToProject(ws)); err != nil {
		log.Printf("[workspace] save error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.registerWorkspaceProject(ws)

	// Optional template scaffold (#329).
	resp := map[string]interface{}{"ok": true, "workspace": ws}
	if body.TemplateID != "" {
		if _, sErr := scaffoldProject(body.TemplateID, ws.ID, ws.Path); sErr != nil {
			log.Printf("[workspace] scaffold template %s for project %s: %v", body.TemplateID, ws.ID, sErr)
			resp["scaffoldError"] = sErr.Error()
		} else {
			resp["scaffolded"] = true
		}
	}
	writeJSON(w, resp)
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
	db, err := meta.OpenDefault()
	if err != nil {
		log.Printf("[workspace] db open error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	existing, ok, err := db.GetProject(ws.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	if existing.Builtin {
		http.Error(w, "cannot modify built-in workspace", http.StatusForbidden)
		return
	}
	// builtin can never be set via update — pin it to the stored value.
	ws.Builtin = existing.Builtin
	// EnsureWorkspaceProject preserves the row's status and position on conflict.
	if err := db.EnsureWorkspaceProject(workspaceToProject(ws)); err != nil {
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
	if id == "default" {
		http.Error(w, "cannot delete built-in workspace", http.StatusForbidden)
		return
	}
	db, err := meta.OpenDefault()
	if err != nil {
		log.Printf("[workspace] db open error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wsToDelete, ok, err := db.GetProject(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	if wsToDelete.Builtin {
		http.Error(w, "cannot delete built-in workspace", http.StatusForbidden)
		return
	}
	if err := db.DeleteProject(id); err != nil {
		log.Printf("[workspace] delete error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Dynamically remove this workspace from CC-Connect projects config
	nameOrID := wsToDelete.Name
	if nameOrID == "" {
		nameOrID = wsToDelete.ID
	}
	projName := getCCProjectName(nameOrID, "claudecode")
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
			
			// Wait for the hot restart to finish to avoid 502 Bad Gateway race condition on immediate redirect
			time.Sleep(1000 * time.Millisecond)
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

// Reorder handles POST /api/workspace/reorder
func (h *Handler) Reorder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	db, err := meta.OpenDefault()
	if err != nil {
		log.Printf("[workspace] db open error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cfg, err := h.loadConfig()
	if err != nil {
		log.Printf("[workspace] load error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Build the full ordered id list: requested ids first (only those that
	// exist), then any remaining workspaces in their current order — mirroring
	// the previous "append unspecified at the end" semantics.
	exists := make(map[string]bool, len(cfg.Workspaces))
	for _, ws := range cfg.Workspaces {
		exists[ws.ID] = true
	}
	ordered := make([]string, 0, len(cfg.Workspaces))
	seen := make(map[string]bool, len(req.IDs))
	for _, id := range req.IDs {
		if exists[id] && !seen[id] {
			ordered = append(ordered, id)
			seen[id] = true
		}
	}
	for _, ws := range cfg.Workspaces {
		if !seen[ws.ID] {
			ordered = append(ordered, ws.ID)
		}
	}

	if err := db.ReorderProjects(ordered); err != nil {
		log.Printf("[workspace] reorder error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
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
	case "windows":
		return pickDirectoryWindows()
	default:
		return "", fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func pickDirectoryWindows() (string, error) {
	script := `Add-Type -AssemblyName System.Windows.Forms; $f = New-Object System.Windows.Forms.FolderBrowserDialog; $f.Description = "选择工作空间目录"; if ($f.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { $f.SelectedPath }`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
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

func getCCProjectName(workspaceName string, agentType string) string {
	var sb strings.Builder
	inInvalidSeq := false
	for _, r := range workspaceName {
		isValid := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
		if isValid {
			sb.WriteRune(r)
			inInvalidSeq = false
		} else {
			if !inInvalidSeq {
				sb.WriteRune('_')
				inInvalidSeq = true
			}
		}
	}
	slug := sb.String()
	if len(slug) > 32 {
		slug = slug[:32]
	}
	if slug == "" {
		slug = "ws"
	}
	return fmt.Sprintf("%s__%s", slug, agentType)
}

// CreateDirectory handles POST /api/workspace/create-directory
func (h *Handler) CreateDirectory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ParentPath string `json:"parentPath"`
		Name       string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		http.Error(w, "directory name cannot be empty", http.StatusBadRequest)
		return
	}
	if strings.Contains(req.Name, "/") || strings.Contains(req.Name, "\\") || req.Name == ".." || req.Name == "." {
		http.Error(w, "invalid directory name", http.StatusBadRequest)
		return
	}

	parent := expandTilde(req.ParentPath)
	if parent == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			http.Error(w, "unable to determine home directory", http.StatusInternalServerError)
			return
		}
		parent = home
	}
	parentAbs, err := filepath.Abs(parent)
	if err != nil {
		http.Error(w, "invalid parent path: "+err.Error(), http.StatusBadRequest)
		return
	}

	targetPath := filepath.Join(parentAbs, req.Name)
	targetPath = filepath.Clean(targetPath)
	parentAbs = filepath.Clean(parentAbs)

	// Verify path safety (prevent directory traversal)
	if !strings.HasPrefix(targetPath, parentAbs) {
		http.Error(w, "path traversal detected", http.StatusBadRequest)
		return
	}

	if err := os.MkdirAll(targetPath, 0755); err != nil {
		http.Error(w, "failed to create directory: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{
		"ok":   true,
		"path": targetPath,
	})
}

