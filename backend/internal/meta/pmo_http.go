package meta

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// PMODispatchHandler serves the PMO 跨项目分发 API (#61):
//
//	GET  /api/pmo/dispatch → list dispatch targets (active projects)
//	POST /api/pmo/dispatch → dispatch a requirement into a project's pool
//	                         {projectId, title, description?, priority?, fromInbox?}
//
// fromInbox is the optional originating inbox_item id: it is recorded as a
// dispatched-from backlink label on the requirement and flips that inbox item
// to read, closing the intake → dispatch loop.
func PMODispatchHandler(store *PMOStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			targets, err := store.Targets()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]any{"targets": targets})

		case http.MethodPost:
			var body struct {
				ProjectID   string `json:"projectId"`
				Title       string `json:"title"`
				Description string `json:"description"`
				Priority    string `json:"priority"`
				FromInbox   string `json:"fromInbox"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid request body", http.StatusBadRequest)
				return
			}
			if strings.TrimSpace(body.ProjectID) == "" || strings.TrimSpace(body.Title) == "" {
				http.Error(w, "projectId and title are required", http.StatusBadRequest)
				return
			}
			res, err := store.Dispatch(body.ProjectID, body.Title, body.Description, body.Priority, body.FromInbox)
			if errors.Is(err, ErrNotFound) {
				http.Error(w, "target project not found", http.StatusNotFound)
				return
			}
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			writeJSON(w, res)

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}
