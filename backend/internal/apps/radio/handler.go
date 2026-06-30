package radio

import (
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

// Handler serves the radio HTTP surface: episode CRUD, pipeline trigger, and
// audio streaming (HTTP range for <audio>). It is constructed by NewHandler and
// wired by the orchestrator (see DOC below); the app NEVER edits the central
// router.
//
// ── ROUTER WIRING (one line each, for the orchestrator) ──────────────────────
//
//	radioHandler := radio.NewHandler()
//	mux.Handle("/api/radio/", radioHandler)            // episode CRUD + pipeline
//	mux.Handle("/api/radio/audio/", radioHandler)      // streaming (range)
//
// A single mux.Handle("/api/radio/", radioHandler) is enough — the handler
// internally routes /api/radio/audio/ for streaming.
type Handler struct {
	app *App
}

// NewHandler returns a Handler over the package singleton app (wired by Init).
func NewHandler() *Handler { return &Handler{app: app} }

// NewHandlerWith returns a Handler over a specific app (used by tests).
func NewHandlerWith(a *App) *Handler { return &Handler{app: a} }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.app == nil {
		http.Error(w, "radio app not initialised", http.StatusServiceUnavailable)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/radio")

	switch {
	case strings.HasPrefix(path, "/audio/"):
		h.serveAudio(w, r, strings.TrimPrefix(path, "/audio/"))
	case path == "/episodes" || path == "/episodes/":
		h.handleEpisodes(w, r)
	case strings.HasPrefix(path, "/episodes/"):
		h.handleEpisodeItem(w, r, strings.TrimPrefix(path, "/episodes/"))
	default:
		http.NotFound(w, r)
	}
}

// GET /api/radio/episodes        → {episodes:[...]}
// POST /api/radio/episodes       → create {title, sourceUrl, workspace} → episode
func (h *Handler) handleEpisodes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		eps, err := h.app.store.ListEpisodes()
		if err != nil {
			httpErr(w, err)
			return
		}
		if eps == nil {
			eps = []Episode{}
		}
		writeJSON(w, map[string]any{"episodes": eps})
	case http.MethodPost:
		var body struct {
			Title     string `json:"title"`
			SourceURL string `json:"sourceUrl"`
			Workspace string `json:"workspace"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if body.Workspace == "" {
			http.Error(w, "workspace is required", http.StatusBadRequest)
			return
		}
		ep, err := h.app.store.CreateEpisode(body.Workspace, body.Title, body.SourceURL)
		if err != nil {
			httpErr(w, err)
			return
		}
		writeJSON(w, ep)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// GET  /api/radio/episodes/{id}          → {episode, tasks:[...]}
// POST /api/radio/episodes/{id}/pipeline → start the 3-stage pipeline
func (h *Handler) handleEpisodeItem(w http.ResponseWriter, r *http.Request, rest string) {
	parts := strings.SplitN(strings.Trim(rest, "/"), "/", 2)
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.Error(w, "bad episode id", http.StatusBadRequest)
		return
	}

	if len(parts) == 2 && parts[1] == "pipeline" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		ws, ok, err := h.app.store.GetWorkspace(id)
		if err != nil {
			httpErr(w, err)
			return
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		ids, err := h.app.StartPipeline(id, ws)
		if err != nil {
			httpErr(w, err)
			return
		}
		writeJSON(w, map[string]any{"taskIds": ids})
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ep, ok, err := h.app.store.GetEpisode(id)
	if err != nil {
		httpErr(w, err)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	// Reverse binding seam: list the pipeline tasks for inline stage state.
	tasks, _ := h.app.api.ListTasksForBusiness(businessRef(id))
	if tasks == nil {
		tasks = []meta.Task{}
	}
	writeJSON(w, map[string]any{"episode": ep, "tasks": tasks})
}

// serveAudio streams the audio artifact for an episode with HTTP range support
// (Accept-Ranges) so <audio> can seek. Path: /api/radio/audio/{episodeId}.
func (h *Handler) serveAudio(w http.ResponseWriter, r *http.Request, rest string) {
	id, err := strconv.ParseInt(strings.Trim(rest, "/"), 10, 64)
	if err != nil {
		http.Error(w, "bad episode id", http.StatusBadRequest)
		return
	}
	ep, ok, err := h.app.store.GetEpisode(id)
	if err != nil {
		httpErr(w, err)
		return
	}
	if !ok || ep.AudioPath == "" {
		http.NotFound(w, r)
		return
	}
	ws, _, err := h.app.store.GetWorkspace(id)
	if err != nil || ws == "" {
		http.NotFound(w, r)
		return
	}
	// Resolve the artifact path safely under the workspace.
	abs := ep.AudioPath
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(ws, ep.AudioPath)
	}
	abs = filepath.Clean(abs)
	if !strings.HasPrefix(abs, filepath.Clean(ws)) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	w.Header().Set("Accept-Ranges", "bytes")
	// http.ServeFile handles Range, If-Modified-Since, content-type by extension.
	http.ServeFile(w, r, abs)
}

func httpErr(w http.ResponseWriter, err error) {
	log.Printf("[radio] http error: %v", err)
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[radio] json encode: %v", err)
	}
}
