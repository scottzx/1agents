// http.go exposes the silver layer read-only for the 数据归一 (data
// normalization) viewer: a per-(domain,source) rollup for the overview and a
// tabular record list for one domain's 多维表格. Read-only — the bronze→silver
// transform runs on its own path (internal/govern, triggered by internal/ingest).
package data

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// Handler serves the read-only silver/gold API over the default data.db, plus
// the one write path — promoting a fused to-do into a task.
type Handler struct {
	store *Store
	// selfBaseURL is this daemon's own loopback HTTP base (e.g.
	// http://127.0.0.1:8080). Promotion posts to the full-featured
	// POST /api/agent/project-items over loopback so it keeps package data decoupled
	// from the task store.
	selfBaseURL string
}

// NewHandlerDefault wires a Handler from the default data.db silver store.
func NewHandlerDefault() (*Handler, error) {
	st, err := OpenDefault()
	if err != nil {
		return nil, err
	}
	return &Handler{store: st}, nil
}

// SetSelfBaseURL injects the loopback base used by todo promotion.
func (h *Handler) SetSelfBaseURL(u string) { h.selfBaseURL = u }

// HandleSummary: GET /api/data/summary → per-(domain,source) silver rollup.
func (h *Handler) HandleSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sums, err := h.store.SilverSummary()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, sums)
}

// HandleRecords: GET /api/data/records?domain=&source=&limit= → one domain's
// conformed silver rows as grid rows. source (optional) scopes to one source.
func (h *Handler) HandleRecords(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	domain := q.Get("domain")
	if domain == "" {
		http.Error(w, "domain required", http.StatusBadRequest)
		return
	}
	limit := 0
	if v := q.Get("limit"); v != "" {
		limit, _ = strconv.Atoi(v)
	}
	rows, err := h.store.ListSilver(domain, q.Get("source"), limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

// HandleGoldSummary: GET /api/data/gold/summary → per-domain fused rollup.
func (h *Handler) HandleGoldSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sums, err := h.store.GoldSummary()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, sums)
}

// HandleGoldRecords: GET /api/data/gold/records?domain=&limit= → one fused
// domain's rows (contacts|messages|events) as grid rows.
func (h *Handler) HandleGoldRecords(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	domain := q.Get("domain")
	if domain == "" {
		http.Error(w, "domain required", http.StatusBadRequest)
		return
	}
	limit := 0
	if v := q.Get("limit"); v != "" {
		limit, _ = strconv.Atoi(v)
	}
	rows, err := h.store.ListGold(domain, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

// HandlePromoteTodo: POST /api/data/gold/todos/promote {id, workspaceId,
// assignee} → create a task from the fused to-do and link it back. assignee is
// "user" (a personal human todo, never dispatched) or an agent type (scheduled
// for execution). Idempotent: an already-linked to-do returns its existing task.
func (h *Handler) HandlePromoteTodo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID          string `json:"id"`
		WorkspaceID string `json:"workspaceId"`
		Assignee    string `json:"assignee"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.ID) == "" || strings.TrimSpace(req.WorkspaceID) == "" {
		http.Error(w, "id and workspaceId are required", http.StatusBadRequest)
		return
	}
	if h.selfBaseURL == "" {
		http.Error(w, "promotion unavailable: task API base not configured", http.StatusServiceUnavailable)
		return
	}
	todo, ok, err := h.store.TodoForPromotion(req.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "todo not found", http.StatusNotFound)
		return
	}
	// Idempotent: never create a second task for an already-promoted to-do.
	if todo.LinkedTaskID != "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "taskId": todo.LinkedTaskID, "alreadyLinked": true})
		return
	}

	body := todo.Body
	body["workspace_id"] = req.WorkspaceID
	if a := strings.TrimSpace(req.Assignee); a != "" {
		body["assignee"] = a
	} else {
		body["assignee"] = "user" // default: a personal human todo mirror
	}
	payload, err := json.Marshal(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp, err := http.Post(h.selfBaseURL+"/api/agent/project-items", "application/json", bytes.NewReader(payload))
	if err != nil {
		http.Error(w, "create task: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		http.Error(w, "create task failed: "+strings.TrimSpace(string(respBody)), http.StatusBadGateway)
		return
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &created); err != nil || created.ID == "" {
		http.Error(w, "create task returned no id", http.StatusBadGateway)
		return
	}
	if err := h.store.LinkTodoTask(req.ID, created.ID); err != nil {
		http.Error(w, "link todo: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "taskId": created.ID})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
