package meta

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
)

// ProjectsHandler serves the project registry API:
//
//	GET  /api/projects             → list all projects (most recently updated first)
//	GET  /api/projects?status=...  → filter by status (active|archived|killed)
//	POST /api/projects             → register/refresh a project {id?, name, path}
func ProjectsHandler(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			var (
				projects []Project
				err      error
			)
			if status := r.URL.Query().Get("status"); status != "" {
				projects, err = db.ListProjectsByStatus(ProjectStatus(status))
			} else {
				projects, err = db.ListProjects()
			}
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, projects)

		case http.MethodPost:
			var body struct {
				ID   string `json:"id"`
				Name string `json:"name"`
				Path string `json:"path"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid request body", http.StatusBadRequest)
				return
			}
			if body.Name == "" || body.Path == "" {
				http.Error(w, "name and path are required", http.StatusBadRequest)
				return
			}
			if body.ID == "" {
				body.ID = NewID()
			}
			if err := db.EnsureProject(body.ID, body.Name, body.Path); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			p, _, err := db.GetProject(body.ID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, p)

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// ProjectActionHandler serves the project lifecycle actions (#141):
//
//	POST /api/projects/{id}/archive {reason?, note?} → 阶段性完成归档
//	POST /api/projects/{id}/close   {reason?, note?} → 竞品出现砍掉 (killed)
//	POST /api/projects/{id}/reopen                   → back to active
//
// archive defaults reason to "completed"; close defaults it to "superseded".
// Both keep all project data — only status/reason/timestamp change. The handler
// is mounted on the "/api/projects/" prefix.
func ProjectActionHandler(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		rest := strings.TrimPrefix(r.URL.Path, "/api/projects/")
		id, action, ok := strings.Cut(rest, "/")
		if !ok || id == "" || action == "" {
			http.Error(w, "expected /api/projects/{id}/{action}", http.StatusBadRequest)
			return
		}

		var body struct {
			Reason ArchiveReason `json:"reason"`
			Note   string        `json:"note"`
		}
		// Body is optional; ignore decode errors on an empty body.
		_ = json.NewDecoder(r.Body).Decode(&body)

		var err error
		switch action {
		case "archive":
			reason := body.Reason
			if reason == "" {
				reason = ArchiveReasonCompleted
			}
			err = db.ArchiveProject(id, ProjectStatusArchived, reason, body.Note)
		case "close":
			reason := body.Reason
			if reason == "" {
				reason = ArchiveReasonSuperseded
			}
			err = db.ArchiveProject(id, ProjectStatusKilled, reason, body.Note)
		case "reopen":
			err = db.ReopenProject(id)
		default:
			http.Error(w, "unknown action: "+action, http.StatusNotFound)
			return
		}
		if errors.Is(err, ErrNotFound) {
			http.Error(w, "project not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		p, _, err := db.GetProject(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, p)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[meta] json encode: %v", err)
	}
}
