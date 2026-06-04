package terminal

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/scottzx/1Agents/backend/internal/config"
)

// TmuxWindow represents a single tmux window parsed from list-windows output.
type TmuxWindow struct {
	Index       int    `json:"index"`
	Name        string `json:"name"`
	Active      bool   `json:"active"`
	WorkspaceID string `json:"workspaceId"`
	Cwd         string `json:"cwd"`
	Status      string `json:"status"`      // e.g. "idle", "busy", "shell", "waiting", ""
	WaitingFor  string `json:"waitingFor"`  // e.g. "permission prompt", ""
	Agent       string `json:"agent"`       // e.g. "claude", "antigravity", ""
	PanePID     int    `json:"-"`
}

// CreateRequest is the body for POST /api/terminal/create.
type CreateRequest struct {
	WorkspaceID string `json:"workspaceId"`
	Cwd         string `json:"cwd"`
}

// KillRequest is the body for POST /api/terminal/kill.
type KillRequest struct {
	WindowIndex int `json:"windowIndex"`
}

// SwitchRequest is the body for POST /api/terminal/switch.
type SwitchRequest struct {
	WindowIndex int `json:"windowIndex"`
}

// Handler manages tmux terminal windows via HTTP API.
type Handler struct {
	session     string
	mu          sync.RWMutex
	mockWindows []TmuxWindow
	agyCache    map[int]string
}

// NewHandler creates a terminal Handler.
func NewHandler(cfg *config.Config) *Handler {
	return &Handler{
		session:     cfg.TmuxSession,
		mockWindows: make([]TmuxWindow, 0),
		agyCache:    make(map[int]string),
	}
}

// ── POST /api/terminal/create ──────────────────────────────────────────────

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.WorkspaceID == "" {
		http.Error(w, "workspaceId is required", http.StatusBadRequest)
		return
	}

	if h.session == "" {
		h.mu.Lock()
		defer h.mu.Unlock()

		var win *TmuxWindow
		for i := range h.mockWindows {
			if h.mockWindows[i].WorkspaceID == req.WorkspaceID {
				win = &h.mockWindows[i]
				break
			}
		}

		if win == nil {
			newWin := TmuxWindow{
				Index:       len(h.mockWindows),
				Name:        fmt.Sprintf("%s_%d", req.WorkspaceID, len(h.mockWindows)+1),
				Active:      true,
				WorkspaceID: req.WorkspaceID,
				Cwd:         req.Cwd,
			}
			h.mockWindows = append(h.mockWindows, newWin)
			win = &h.mockWindows[len(h.mockWindows)-1]
		}

		for i := range h.mockWindows {
			h.mockWindows[i].Active = (h.mockWindows[i].Index == win.Index)
		}

		writeJSON(w, http.StatusCreated, win)
		return
	}

	// Ensure tmux session exists; create one in detached mode if needed
	h.ensureSession()

	// Find next available window number for this workspace
	nextNum := h.nextWindowNum(req.WorkspaceID)
	winName := fmt.Sprintf("%s_%d", req.WorkspaceID, nextNum)

	// If the only window is the placeholder, rename it instead of creating new one
	windows, _ := h.listWindows()
	if len(windows) == 1 && !strings.Contains(windows[0].Name, "_") {
		renameCmd := exec.Command("tmux", "rename-window", "-t", h.session+":0", winName)
		if out, err := renameCmd.CombinedOutput(); err != nil {
			log.Printf("[terminal] rename placeholder error: %v (output: %s)", err, string(out))
		} else {
			// Synchronize shell directory to workspace Cwd
			if req.Cwd != "" {
				cdCmd := exec.Command("tmux", "send-keys", "-t", h.session+":0", fmt.Sprintf("cd %q && clear", req.Cwd), "C-m")
				_ = cdCmd.Run()
			}
			// Switch to the renamed window
			h.selectWindow(0)
			win := &TmuxWindow{Index: 0, Name: winName, Active: true, WorkspaceID: req.WorkspaceID}
			writeJSON(w, http.StatusCreated, win)
			return
		}
	}

	args := []string{"new-window", "-a", "-t", h.session, "-n", winName}
	if req.Cwd != "" {
		args = append(args, "-c", req.Cwd)
	}

	cmd := exec.Command("tmux", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[terminal] create window error: %v (output: %s)", err, string(out))
		http.Error(w, "failed to create window: "+string(out), http.StatusInternalServerError)
		return
	}

	// Get the index of the newly created window
	win, err := h.findWindowByName(winName)
	if err != nil {
		log.Printf("[terminal] find window after create error: %v", err)
		http.Error(w, "window created but failed to locate it", http.StatusInternalServerError)
		return
	}

	// Switch to the new window
	h.selectWindow(win.Index)

	writeJSON(w, http.StatusCreated, win)
}

// ── GET /api/terminal/list ─────────────────────────────────────────────────

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
	
		if h.session == "" {
			h.mu.RLock()
			defer h.mu.RUnlock()
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"windows": h.mockWindows,
				"session": "",
			})
			return
		}
	
		// Do NOT auto-create tmux session — only list if session already exists.
	// User must manually create a session via /api/terminal/create first.
	if !h.sessionExists() {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"windows": []TmuxWindow{},
			"session": h.session,
		})
		return
	}

	windows, err := h.listWindows()
	if err != nil {
		log.Printf("[terminal] list error: %v", err)
		http.Error(w, "failed to list windows: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"windows": windows,
		"session": h.session,
	})
}

// ── POST /api/terminal/kill ────────────────────────────────────────────────

func (h *Handler) Kill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req KillRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if h.session == "" {
		h.mu.Lock()
		defer h.mu.Unlock()

		if len(h.mockWindows) <= 1 {
			http.Error(w, "cannot kill the last terminal window", http.StatusBadRequest)
			return
		}

		idx := -1
		for i, win := range h.mockWindows {
			if win.Index == req.WindowIndex {
				idx = i
				break
			}
		}

		if idx == -1 {
			http.Error(w, "window not found", http.StatusNotFound)
			return
		}

		h.mockWindows = append(h.mockWindows[:idx], h.mockWindows[idx+1:]...)

		activeFound := false
		for i := range h.mockWindows {
			h.mockWindows[i].Index = i
			if h.mockWindows[i].Active {
				activeFound = true
			}
		}
		if !activeFound && len(h.mockWindows) > 0 {
			h.mockWindows[0].Active = true
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		return
	}

	windows, err := h.listWindows()
	if err != nil {
		http.Error(w, "failed to list windows: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if len(windows) <= 1 {
		http.Error(w, "cannot kill the last terminal window", http.StatusBadRequest)
		return
	}

	// Check if the target window index exists
	exists := false
	for _, w := range windows {
		if w.Index == req.WindowIndex {
			exists = true
			break
		}
	}
	if !exists {
		http.Error(w, "window not found", http.StatusNotFound)
		return
	}

	cmd := exec.Command("tmux", "kill-window", "-t", fmt.Sprintf("%s:%d", h.session, req.WindowIndex))
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[terminal] kill window error: %v (output: %s)", err, string(out))
		http.Error(w, "failed to kill window: "+string(out), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

// ── POST /api/terminal/switch ──────────────────────────────────────────────

func (h *Handler) Switch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SwitchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if h.session == "" {
		h.mu.Lock()
		defer h.mu.Unlock()

		found := false
		for i := range h.mockWindows {
			if h.mockWindows[i].Index == req.WindowIndex {
				found = true
				h.mockWindows[i].Active = true
			} else {
				h.mockWindows[i].Active = false
			}
		}

		if !found {
			http.Error(w, "window not found", http.StatusNotFound)
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		return
	}

	if err := h.selectWindow(req.WindowIndex); err != nil {
		log.Printf("[terminal] switch error: %v", err)
		http.Error(w, "failed to switch window: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

// ── tmux command helpers ───────────────────────────────────────────────────

func (h *Handler) sessionExists() bool {
	return exec.Command("tmux", "has-session", "-t", h.session).Run() == nil
}

// ensureSession creates the tmux session in detached mode if it doesn't exist.
// This is needed because ttyd only spawns tmux inside its PTY when a WebSocket
// client connects, but the API needs the session available before that.
func (h *Handler) ensureSession() {
	// Enable mouse mode globally first so any session inherits it
	_ = exec.Command("tmux", "set-option", "-g", "mouse", "on").Run()

	if h.sessionExists() {
		// Just in case, ensure mouse mode is on for this session
		_ = exec.Command("tmux", "set-option", "-t", h.session, "mouse", "on").Run()
		return
	}
	log.Printf("[terminal] creating tmux session '%s' in detached mode", h.session)
	_ = exec.Command("tmux", "new-session", "-d", "-s", h.session, "-n", "p").Run()
	_ = exec.Command("tmux", "set-option", "-t", h.session, "mouse", "on").Run()
}

// nextWindowNum finds the next available N for workspace_<N> naming.
func (h *Handler) nextWindowNum(workspaceID string) int {
	windows, err := h.listWindows()
	if err != nil {
		return 1
	}

	prefix := workspaceID + "_"
	maxN := 0
	for _, w := range windows {
		if strings.HasPrefix(w.Name, prefix) {
			rest := strings.TrimPrefix(w.Name, prefix)
			if n, err := strconv.Atoi(rest); err == nil && n > maxN {
				maxN = n
			}
		}
	}
	return maxN + 1
}



func (h *Handler) listWindows() ([]TmuxWindow, error) {
	parentMap, cmdMap, err := getProcessDetails()
	if err != nil {
		log.Printf("[terminal] getProcessDetails error: %v", err)
	}

	claudeSessions, err := getClaudeSessions()
	if err != nil {
		log.Printf("[terminal] getClaudeSessions error: %v", err)
	}

	home, _ := os.UserHomeDir()

	h.mu.Lock()
	// Clean up dead PIDs from agyCache
	for pid := range h.agyCache {
		if _, exists := cmdMap[pid]; !exists {
			delete(h.agyCache, pid)
		}
	}

	// Find uncached PIDs
	var uncachedAgyPIDs []int
	for pid, cmdLine := range cmdMap {
		cmdLower := strings.ToLower(cmdLine)
		if strings.Contains(cmdLower, "agy") || strings.Contains(cmdLower, "antigravity") {
			if _, cached := h.agyCache[pid]; !cached {
				uncachedAgyPIDs = append(uncachedAgyPIDs, pid)
			}
		}
	}

	// Query lsof for uncached PIDs and update cache
	if len(uncachedAgyPIDs) > 0 {
		newAgyConvIDs := getAgyConversationIDs(uncachedAgyPIDs)
		for pid, convID := range newAgyConvIDs {
			h.agyCache[pid] = convID
		}
	}

	// Clone caches to avoid holding lock during file I/O
	agyConvIDs := make(map[int]string)
	for pid, convID := range h.agyCache {
		agyConvIDs[pid] = convID
	}
	h.mu.Unlock()

	format := "#{window_index}|#{window_name}|#{?window_active,1,0}|#{pane_pid}"
	cmd := exec.Command("tmux", "list-windows", "-t", h.session, "-F", format)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("tmux list-windows: %w", err)
	}

	var windows []TmuxWindow
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 4)
		if len(parts) != 4 {
			continue
		}
		idx, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		name := parts[1]
		active := parts[2] == "1"
		panePID, err := strconv.Atoi(parts[3])
		if err != nil {
			panePID = 0
		}

		// Parse workspace ID from name: "{workspaceId}_{n}"
		wsID := name
		if lastUnderscore := strings.LastIndex(name, "_"); lastUnderscore > 0 {
			wsID = name[:lastUnderscore]
		}

		status := ""
		waitingFor := ""
		agent := ""

		// 1. Try to find a Claude session match first
		if panePID > 0 && len(claudeSessions) > 0 && len(parentMap) > 0 {
			for _, cs := range claudeSessions {
				if cs.PID == panePID || isAncestor(parentMap, cs.PID, panePID) {
					status = cs.Status
					waitingFor = cs.WaitingFor
					agent = cs.Agent
					if agent == "" {
						agent = "claude"
					}
					break
				}
			}
		}

		if agent == "" && panePID > 0 && len(parentMap) > 0 && len(cmdMap) > 0 {
			for pid, cmdLine := range cmdMap {
				if pid == panePID || isAncestor(parentMap, pid, panePID) {
					cmdLower := strings.ToLower(cmdLine)
					if strings.Contains(cmdLower, "agy") || strings.Contains(cmdLower, "antigravity") {
						agent = "antigravity"
						status = "idle"
						if convID, ok := agyConvIDs[pid]; ok && convID != "" {
							status, waitingFor = getAgyStatus(home, convID)
						}
						break
					} else if strings.Contains(cmdLower, "codex") {
						agent = "codex"
						status = ""
						break
					} else if strings.Contains(cmdLower, "claude") && !strings.Contains(cmdLower, "daemon") {
						agent = "claude"
						status = "idle"
						break
					}
				}
			}
		}

		windows = append(windows, TmuxWindow{
			Index:       idx,
			Name:        name,
			Active:      active,
			WorkspaceID: wsID,
			Status:      status,
			WaitingFor:  waitingFor,
			Agent:       agent,
			PanePID:     panePID,
		})
	}
	return windows, nil
}

func (h *Handler) findWindowByName(name string) (*TmuxWindow, error) {
	windows, err := h.listWindows()
	if err != nil {
		return nil, err
	}
	for _, w := range windows {
		if w.Name == name {
			return &w, nil
		}
	}
	return nil, fmt.Errorf("window %q not found", name)
}

func (h *Handler) selectWindow(index int) error {
	cmd := exec.Command("tmux", "select-window", "-t", fmt.Sprintf("%s:%d", h.session, index))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("select-window: %s (output: %s)", err, string(out))
	}
	return nil
}

// MouseRequest is the request body for POST /api/terminal/mouse.
type MouseRequest struct {
	Mouse bool `json:"mouse"`
}

// GetMouse returns whether the tmux session currently has mouse mode enabled.
func (h *Handler) GetMouse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.session == "" {
		writeJSON(w, http.StatusOK, map[string]interface{}{"mouse": false})
		return
	}

	if !h.sessionExists() {
		writeJSON(w, http.StatusOK, map[string]interface{}{"mouse": false})
		return
	}

	cmd := exec.Command("tmux", "show-options", "-t", h.session, "mouse")
	out, err := cmd.Output()
	enabled := false
	if err == nil {
		outStr := strings.TrimSpace(string(out))
		if strings.Contains(outStr, "on") {
			enabled = true
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"mouse": enabled})
}

// SetMouse configures the tmux session to enable or disable mouse mode.
func (h *Handler) SetMouse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req MouseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if h.session == "" {
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "mouse": req.Mouse})
		return
	}

	h.ensureSession()

	val := "off"
	if req.Mouse {
		val = "on"
	}

	// Set specifically on the session and globally so new windows inherit it
	_ = exec.Command("tmux", "set-option", "-t", h.session, "mouse", val).Run()
	_ = exec.Command("tmux", "set-option", "-g", "mouse", val).Run()

	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "mouse": req.Mouse})
}

// ── helpers ────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[terminal] json encode error: %v", err)
	}
}
