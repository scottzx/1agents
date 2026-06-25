package meta

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// InboxHandler serves the Inbox 收口层 API (#60):
//
//	GET  /api/inbox?archived=1 → {items, unread} (archived items excluded unless archived=1)
//	POST /api/inbox            → manual capture {source?, title, content?, url?, tags?}
//
// InboxItemHandler serves per-item status changes, routed under /api/inbox/:
//
//	POST /api/inbox/{id}/archive → flip to archived (data retained)
//	POST /api/inbox/{id}/read    → flip to read
//	POST /api/inbox/{id}/unread  → flip back to unread
func InboxHandler(store *InboxStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			items, err := store.List(r.URL.Query().Get("archived") == "1")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			unread, err := store.UnreadCount()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]any{"items": items, "unread": unread})

		case http.MethodPost:
			var body struct {
				Source  string   `json:"source"`
				Title   string   `json:"title"`
				Content string   `json:"content"`
				URL     string   `json:"url"`
				Tags    []string `json:"tags"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid request body", http.StatusBadRequest)
				return
			}
			if strings.TrimSpace(body.Title) == "" && strings.TrimSpace(body.Content) == "" &&
				strings.TrimSpace(body.URL) == "" {
				http.Error(w, "title, content or url is required", http.StatusBadRequest)
				return
			}
			item, err := store.Capture(InboxItem{
				Source:  body.Source,
				Title:   body.Title,
				Content: body.Content,
				URL:     body.URL,
				Tags:    body.Tags,
			})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, item)

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// InboxItemHandler handles /api/inbox/{id}/{action} where action is
// archive / read / unread. POST only.
func InboxItemHandler(store *InboxStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// Path: /api/inbox/{id}/{action}
		rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/inbox/"), "/")
		parts := strings.Split(rest, "/")
		if len(parts) != 2 || parts[0] == "" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		id, action := parts[0], parts[1]
		var status string
		switch action {
		case "archive":
			status = InboxStatusArchived
		case "read":
			status = InboxStatusRead
		case "unread":
			status = InboxStatusUnread
		default:
			http.Error(w, "unknown action", http.StatusNotFound)
			return
		}
		item, err := store.SetStatus(id, status)
		if errors.Is(err, ErrNotFound) {
			http.Error(w, "inbox item not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, item)
	}
}
