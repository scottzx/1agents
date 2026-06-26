package meta

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// PersonalTasksHandler serves the Inbox 下游 Task 汇总层 API (#67):
//
//	GET  /api/personal-tasks → list personal (no-project) tasks
//	POST /api/personal-tasks → capture a lightweight personal task
//	                           {title, description?, fromInbox?}
//
// fromInbox is an optional originating inbox_item id, recorded as a backlink
// label so the "what did this inbox item become" trail survives.
func PersonalTasksHandler(store *PersonalStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			tasks, err := store.List()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]any{"tasks": tasks})

		case http.MethodPost:
			var body struct {
				Title       string `json:"title"`
				Description string `json:"description"`
				FromInbox   string `json:"fromInbox"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid request body", http.StatusBadRequest)
				return
			}
			if strings.TrimSpace(body.Title) == "" {
				http.Error(w, "title is required", http.StatusBadRequest)
				return
			}
			task, err := store.Capture(body.Title, body.Description, body.FromInbox)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, task)

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// PersonalTaskItemHandler serves the per-task actions under
// /api/personal-tasks/{id}/{action}. POST only.
//
//	POST /api/personal-tasks/{id}/incubate → 立项: promote to a new long-term
//	      project {projectName, workspacePath, milestones?}
//
// onIncubated, when non-nil, is invoked once after a successful 立项 with the
// freshly-created project. The project row is already a workspace (unified
// registry); the hook performs the non-storage side-effects — cc-connect bridge
// registration + agent guide files — that the workspace package owns (kept out
// of meta to avoid a meta→workspace import cycle). Best-effort: it must not fail
// the request, so its result is ignored.
func PersonalTaskItemHandler(store *PersonalStore, onIncubated func(Project)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/personal-tasks/"), "/")
		id, action, ok := strings.Cut(rest, "/")
		if !ok || id == "" || action == "" {
			http.Error(w, "expected /api/personal-tasks/{id}/{action}", http.StatusBadRequest)
			return
		}
		switch action {
		case "incubate":
			var body struct {
				ProjectName   string   `json:"projectName"`
				WorkspacePath string   `json:"workspacePath"`
				Milestones    []string `json:"milestones"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid request body", http.StatusBadRequest)
				return
			}
			if strings.TrimSpace(body.ProjectName) == "" || strings.TrimSpace(body.WorkspacePath) == "" {
				http.Error(w, "projectName and workspacePath are required", http.StatusBadRequest)
				return
			}
			res, err := store.Incubate(id, body.ProjectName, body.WorkspacePath, body.Milestones)
			if errors.Is(err, ErrNotFound) {
				http.Error(w, "personal task not found", http.StatusNotFound)
				return
			}
			if err != nil {
				// Validation failures (not a personal task / path taken) are 400.
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if onIncubated != nil {
				onIncubated(res.Project)
			}
			writeJSON(w, res)

		default:
			http.Error(w, "unknown action: "+action, http.StatusNotFound)
		}
	}
}
