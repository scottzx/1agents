package meta

import (
	"encoding/json"
	"log"
	"net/http"
)

// ProjectsHandler serves the project registry API:
//
//	GET  /api/projects → list all projects (most recently updated first)
//	POST /api/projects → register/refresh a project {id?, name, path}
func ProjectsHandler(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			projects, err := db.ListProjects()
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

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[meta] json encode: %v", err)
	}
}
