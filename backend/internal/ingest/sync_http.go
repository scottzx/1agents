package ingest

import (
	"encoding/json"
	"net/http"
	"strings"
)

// HandleSync serves POST /api/sources/{source}/sync — dispatch an immediate
// one-off sync task. Body: {"kind":"feishu_chat"}. Returns {"taskId":"..."}.
func (h *Handler) HandleSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	source := sourceFromAction(r.URL.Path, "sync")
	if source == "" {
		http.Error(w, "source required", http.StatusBadRequest)
		return
	}
	if h.disp == nil {
		http.Error(w, "sync dispatcher unavailable", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		Kind       string `json:"kind"`
		Collection string `json:"collection"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Kind == "" {
		http.Error(w, "kind required", http.StatusBadRequest)
		return
	}
	taskID, err := h.disp.SyncNow(source, body.Kind, body.Collection)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"taskId": taskID})
}

// HandleHistory serves GET /api/sources/{source}/history — prior sync runs
// (work-order tasks) newest first.
func (h *Handler) HandleHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	source := sourceFromAction(r.URL.Path, "history")
	if source == "" {
		http.Error(w, "source required", http.StatusBadRequest)
		return
	}
	if h.disp == nil {
		writeJSON(w, http.StatusOK, []SyncRun{})
		return
	}
	runs, err := h.disp.History(source)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

// sourceFromAction extracts {source} from /api/sources/{source}/{action}.
func sourceFromAction(path, action string) string {
	rest := strings.TrimPrefix(path, "/api/sources/")
	parts := strings.Split(rest, "/")
	if len(parts) < 2 || parts[1] != action {
		return ""
	}
	return parts[0]
}
