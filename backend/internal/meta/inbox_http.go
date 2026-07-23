package meta

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// InboxHandler serves the Workspace Inbox list/capture API (#202 / #60):
//
//	GET  /api/inbox?workspaceId=&archived=1 → {items, unread}
//	POST /api/inbox                         → manual capture {workspaceId, source?, title, …}
//
// When workspaceId is omitted on GET, returns the global list (legacy); POST
// capture without workspaceId lands on the default assistant box.
func InboxHandler(store *InboxStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			wsID := strings.TrimSpace(r.URL.Query().Get("workspaceId"))
			includeArchived := r.URL.Query().Get("archived") == "1"
			var items []InboxItem
			var err error
			var unread int
			if wsID != "" {
				items, err = store.ListByWorkspace(wsID, includeArchived)
				if err == nil {
					unread, err = store.UnreadCountByWorkspace(wsID)
				}
			} else {
				items, err = store.List(includeArchived)
				if err == nil {
					unread, err = store.UnreadCount()
				}
			}
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]any{"items": items, "unread": unread})

		case http.MethodPost:
			var body struct {
				WorkspaceID string          `json:"workspaceId"`
				Source      string          `json:"source"`
				Title       string          `json:"title"`
				Content     string          `json:"content"`
				URL         string          `json:"url"`
				Summary     string          `json:"summary"`
				Tags        []string        `json:"tags"`
				Payload     json.RawMessage `json:"payload"`
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
				WorkspaceID: body.WorkspaceID,
				Source:      body.Source,
				Title:       body.Title,
				Content:     body.Content,
				URL:         body.URL,
				Summary:     body.Summary,
				Tags:        body.Tags,
				Payload:     body.Payload,
			})
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			writeJSON(w, item)

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// InboxDeliverHandler serves POST /api/inbox/deliver — the unified envelope
// write path (function / agent / human / channel). Body carries workspaceId
// (recipient), optional fromWorkspaceId / fromRef, and title|content|url.
func InboxDeliverHandler(store *InboxStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			WorkspaceID     string          `json:"workspaceId"`
			Source          string          `json:"source"`
			FromWorkspaceID string          `json:"fromWorkspaceId"`
			FromRef         string          `json:"fromRef"`
			Title           string          `json:"title"`
			Content         string          `json:"content"`
			URL             string          `json:"url"`
			Summary         string          `json:"summary"`
			Tags            []string        `json:"tags"`
			Payload         json.RawMessage `json:"payload"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		item, err := store.Deliver(InboxItem{
			WorkspaceID:     body.WorkspaceID,
			Source:          body.Source,
			FromWorkspaceID: body.FromWorkspaceID,
			FromRef:         body.FromRef,
			Title:           body.Title,
			Content:         body.Content,
			URL:             body.URL,
			Summary:         body.Summary,
			Tags:            body.Tags,
			Payload:         body.Payload,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, item)
	}
}

// InboxTargetsHandler serves GET /api/inbox/targets — workspaces that can
// receive mail (active projects / workforce, excluding personal bucket).
func InboxTargetsHandler(pmo *PMOStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if pmo == nil {
			http.Error(w, "pmo store not available", http.StatusServiceUnavailable)
			return
		}
		targets, err := pmo.Targets()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"targets": targets})
	}
}

// InboxItemHandler handles /api/inbox/{id} and /api/inbox/{id}/{action}:
//
//	GET  /api/inbox/{id}             → single item (?workspaceId= ownership filter)
//	POST /api/inbox/{id}/archive     → archive
//	POST /api/inbox/{id}/read        → mark read
//	POST /api/inbox/{id}/unread      → mark unread
//	POST /api/inbox/{id}/accept      → accept as requirement (reuses PMO Dispatch)
func InboxItemHandler(store *InboxStore, pmo *PMOStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/inbox/"), "/")
		// Sub-routes registered separately (deliver/targets) must not be claimed here
		// if they somehow fall through — but they are exact-path handlers.
		if rest == "" || rest == "deliver" || rest == "targets" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		parts := strings.Split(rest, "/")
		id := parts[0]
		if id == "" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		if len(parts) == 1 {
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			item, ok, err := store.Get(id)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if !ok {
				http.Error(w, "inbox item not found", http.StatusNotFound)
				return
			}
			if ws := strings.TrimSpace(r.URL.Query().Get("workspaceId")); ws != "" && item.WorkspaceID != ws {
				http.Error(w, "inbox item not found", http.StatusNotFound)
				return
			}
			writeJSON(w, item)
			return
		}

		if len(parts) != 2 {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		action := parts[1]
		switch action {
		case "archive", "read", "unread":
			var status string
			switch action {
			case "archive":
				status = InboxStatusArchived
			case "read":
				status = InboxStatusRead
			case "unread":
				status = InboxStatusUnread
			}
			if ws := strings.TrimSpace(r.URL.Query().Get("workspaceId")); ws != "" {
				item, ok, err := store.Get(id)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				if !ok || item.WorkspaceID != ws {
					http.Error(w, "inbox item not found", http.StatusNotFound)
					return
				}
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

		case "accept":
			if pmo == nil {
				http.Error(w, "pmo store not available", http.StatusServiceUnavailable)
				return
			}
			var body struct {
				WorkspaceID string `json:"workspaceId"`
				Title       string `json:"title"`
				Description string `json:"description"`
				Priority    string `json:"priority"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			wsID := strings.TrimSpace(body.WorkspaceID)
			if wsID == "" {
				wsID = strings.TrimSpace(r.URL.Query().Get("workspaceId"))
			}
			if wsID == "" {
				item, ok, err := store.Get(id)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				if !ok {
					http.Error(w, "inbox item not found", http.StatusNotFound)
					return
				}
				wsID = item.WorkspaceID
			}
			res, err := pmo.Accept(wsID, id, body.Title, body.Description, body.Priority)
			if errors.Is(err, ErrNotFound) {
				http.Error(w, "inbox item or workspace not found", http.StatusNotFound)
				return
			}
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			writeJSON(w, res)

		default:
			http.Error(w, "unknown action", http.StatusNotFound)
		}
	}
}
