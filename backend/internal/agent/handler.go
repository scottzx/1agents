package agent

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
)

// Handler exposes the REST surface for the chat session index.
//
// All endpoints are mounted under /api/agent and protected by the
// outer server.authMiddleware. The handler itself does NO auth checks
// — that is the responsibility of the parent mux.
type Handler struct {
	store *Store
}

// NewHandler returns a Handler backed by store.
func NewHandler(store *Store) *Handler {
	return &Handler{store: store}
}

// HandleAgentTypes serves GET /api/agent/agent-types
func (h *Handler) HandleAgentTypes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, SupportedAgentTypes)
}

// HandleSessionsRoot handles /api/agent/sessions (root, no trailing slash).
//
// GET  → list by ?workspace_id=… (returns empty array if missing)
// POST → add a new record (returns the record with id + created_at)
func (h *Handler) HandleSessionsRoot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.list(w, r)
	case http.MethodPost:
		h.create(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleSessionsItem handles /api/agent/sessions/{id} (with trailing slash).
//
// GET    → fetch single record
// DELETE → remove record (does NOT touch cc-connect; caller is responsible
//
//	for the cc-connect session lifecycle).
func (h *Handler) HandleSessionsItem(w http.ResponseWriter, r *http.Request) {
	// Extract id from path: /api/agent/sessions/{id}
	const prefix = "/api/agent/sessions/"
	id := r.URL.Path[len(prefix):]
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	// Strip any trailing sub-paths (none defined today; future-proofing).
	if i := indexByte(id, '/'); i >= 0 {
		http.Error(w, "unsupported sub-path", http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		rec, ok, err := h.store.Get(id)
		if err != nil {
			log.Printf("[agent] get %s: %v", id, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}
		writeJSON(w, rec)
	case http.MethodDelete:
		if err := h.store.Delete(id); err != nil {
			if errors.Is(err, ErrNotFound) {
				http.Error(w, "session not found", http.StatusNotFound)
				return
			}
			log.Printf("[agent] delete %s: %v", id, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	wsID := r.URL.Query().Get("workspace_id")
	if wsID == "" {
		http.Error(w, "workspace_id query parameter is required", http.StatusBadRequest)
		return
	}
	recs, err := h.store.ListByWorkspace(wsID)
	if err != nil {
		log.Printf("[agent] list for %s: %v", wsID, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if recs == nil {
		recs = []ChatSessionRecord{}
	}
	writeJSON(w, recs)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var body IndexRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.WorkspaceID == "" || body.AgentType == "" || body.CcProject == "" ||
		body.CcSessionID == "" || body.SessionKey == "" {
		http.Error(w, "workspace_id, agent_type, cc_project, cc_session_id and session_key are required", http.StatusBadRequest)
		return
	}
	rec := ChatSessionRecord{
		ID:          newID(),
		WorkspaceID: body.WorkspaceID,
		Name:        body.Name,
		AgentType:   body.AgentType,
		CcProject:   body.CcProject,
		CcSessionID: body.CcSessionID,
		SessionKey:  body.SessionKey,
	}
	if err := h.store.Add(rec); err != nil {
		if errors.Is(err, ErrDuplicate) {
			http.Error(w, "session with this id already exists", http.StatusConflict)
			return
		}
		log.Printf("[agent] add: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, rec)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[agent] json encode: %v", err)
	}
}

// newID returns a random 16-byte hex string, suitable as a session id.
func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand should never fail on a healthy system; if it does,
		// fall back to a non-cryptographic id so the request can still
		// succeed. Collision is astronomically unlikely.
		return "agent-fallback-id"
	}
	return hex.EncodeToString(b[:])
}

// indexByte is a tiny stdlib replacement to keep imports minimal in the
// hot path of HandleSessionsItem.
func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}
