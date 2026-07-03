package sourcecli

import (
	"encoding/json"
	"net/http"
	"strings"
)

// Handler serves the CLI-lifecycle status for the 数据源 cards.
//
// Routes (both under the "/api/sources/cli/" subtree):
//
//	GET  /api/sources/cli/{tool}/status   → cached CLIStatus
//	POST /api/sources/cli/{tool}/recheck  → force a fresh probe, return CLIStatus
type Handler struct {
	mgr *Manager
}

// NewHandler wires a Handler over mgr with the source CLIs pre-registered
// (lark-cli for 飞书, agently-cli for Agent Mail).
func NewHandler() *Handler {
	mgr := NewManager(0)
	mgr.Register(NewLarkTool("", nil))
	mgr.Register(NewAgentlyTool("", nil))
	return &Handler{mgr: mgr}
}

// HandleCLI routes /api/sources/cli/{tool}/{action}.
func (h *Handler) HandleCLI(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/sources/cli/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) != 2 || parts[0] == "" {
		http.Error(w, "expected /api/sources/cli/{tool}/{status|recheck}", http.StatusBadRequest)
		return
	}
	tool, action := parts[0], parts[1]

	var (
		st    CLIStatus
		known bool
	)
	switch action {
	case "status":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		st, known = h.mgr.Status(r.Context(), tool)
	case "recheck":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		st, known = h.mgr.Recheck(r.Context(), tool)
	default:
		http.Error(w, "unknown action", http.StatusNotFound)
		return
	}
	if !known {
		http.Error(w, "unknown tool", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
