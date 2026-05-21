package terminal

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"

	"github.com/scottzx/remote-agents/agent/internal/config"
)

// TmuxWindow represents a single tmux window parsed from list-windows output.
type TmuxWindow struct {
	Index      int    `json:"index"`
	Name       string `json:"name"`
	Active     bool   `json:"active"`
	WorkspaceID string `json:"workspaceId"`
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
	session string
}

// NewHandler creates a terminal Handler.
func NewHandler(cfg *config.Config) *Handler {
	return &Handler{session: cfg.TmuxSession}
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

	// Ensure tmux session exists; create one in detached mode if needed
	h.ensureSession()

	// Find next available window number for this workspace
	nextNum := h.nextWindowNum(req.WorkspaceID)
	winName := fmt.Sprintf("%s_%d", req.WorkspaceID, nextNum)

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

	h.ensureSession()
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
	if h.sessionExists() {
		return
	}
	log.Printf("[terminal] creating tmux session '%s' in detached mode", h.session)
	exec.Command("tmux", "new-session", "-d", "-s", h.session, "-n", "default").Run()
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
	format := "#{window_index}|#{window_name}|#{?window_active,1,0}"
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
		parts := strings.SplitN(line, "|", 3)
		if len(parts) != 3 {
			continue
		}
		idx, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		name := parts[1]
		active := parts[2] == "1"

		// Parse workspace ID from name: "{workspaceId}_{n}"
		wsID := name
		if lastUnderscore := strings.LastIndex(name, "_"); lastUnderscore > 0 {
			wsID = name[:lastUnderscore]
		}

		windows = append(windows, TmuxWindow{
			Index:       idx,
			Name:        name,
			Active:      active,
			WorkspaceID: wsID,
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

// ── helpers ────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[terminal] json encode error: %v", err)
	}
}
