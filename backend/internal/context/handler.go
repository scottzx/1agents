package context

import (
	"encoding/json"
	"log"
	"net/http"
)

// Handler manages the active workspace context, allowing the frontend
// to switch the file-system and git roots at runtime.
type Handler struct {
	fsHandler  FsRootSetter
	gitHandler GitRootSetter
}

// FsRootSetter is satisfied by *fs.Handler.
type FsRootSetter interface {
	SetRoot(path string) error
	Root() string
}

// GitRootSetter is satisfied by *git.Handler.
type GitRootSetter interface {
	SetRoot(path string) error
}

// NewHandler creates a context handler that can update both the fs and git roots.
func NewHandler(fs FsRootSetter, git GitRootSetter) *Handler {
	return &Handler{fsHandler: fs, gitHandler: git}
}

type setWorkspaceRequest struct {
	Path string `json:"path"`
}

// Set handles POST /api/context/set
// Switches the active workspace directory for both the file browser and git.
func (h *Handler) Set(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req setWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Path == "" {
		http.Error(w, "missing path", http.StatusBadRequest)
		return
	}

	if err := h.fsHandler.SetRoot(req.Path); err != nil {
		log.Printf("[context] fs set root error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := h.gitHandler.SetRoot(req.Path); err != nil {
		log.Printf("[context] git set root error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{
		"ok":   true,
		"path": h.fsHandler.Root(),
	})
}

// Get handles GET /api/context/get
// Returns the current workspace root for fs and git.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]string{"path": h.fsHandler.Root()})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[context] json encode error: %v", err)
	}
}
