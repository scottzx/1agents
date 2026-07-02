package ingest

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/scottzx/1Agents/backend/internal/feishu"
	"github.com/scottzx/1Agents/backend/internal/meta"
)

// CollectionView is one crawlable kind for the config UI: the stored crawl
// policy merged with the catalog descriptor's static metadata (domain/label/
// implemented), so the frontend can render the full roadmap with per-kind toggles.
type CollectionView struct {
	meta.SourceCollectionConfig
	Domain      string `json:"domain"`
	Label       string `json:"label"`
	Implemented bool   `json:"implemented"`
	PerChat     bool   `json:"perChat"`
	Configured  bool   `json:"configured"` // false → showing defaults, never saved
}

// HandleCollections serves /api/sources/{source}/collections.
//
//	GET → the source's crawlable kinds (catalog ⨝ stored config)
//	PUT → upsert one kind's crawl policy (body: SourceCollectionConfig)
func (h *Handler) HandleCollections(w http.ResponseWriter, r *http.Request) {
	source := sourceFromPath(r.URL.Path)
	if source == "" {
		http.Error(w, "source required", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		h.getCollections(w, source)
	case http.MethodPut:
		h.putCollection(w, r, source)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) getCollections(w http.ResponseWriter, source string) {
	// Only Feishu has a catalog today; other sources return their stored config
	// verbatim (no roadmap overlay).
	if source != feishu.Source {
		list, err := h.cfg.List(source)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		views := make([]CollectionView, 0, len(list))
		for _, c := range list {
			views = append(views, CollectionView{SourceCollectionConfig: c, Configured: true})
		}
		writeJSON(w, http.StatusOK, views)
		return
	}

	views := []CollectionView{}
	for _, d := range feishu.Catalog() {
		cfg, ok, err := h.cfg.Get(source, d.Kind)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		views = append(views, CollectionView{
			SourceCollectionConfig: cfg,
			Domain:                 d.Domain,
			Label:                  d.Label,
			Implemented:            d.Implemented,
			PerChat:                d.PerChat,
			Configured:             ok,
		})
	}
	writeJSON(w, http.StatusOK, views)
}

func (h *Handler) putCollection(w http.ResponseWriter, r *http.Request, source string) {
	var body meta.SourceCollectionConfig
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	body.Source = source
	if body.Kind == "" {
		http.Error(w, "kind required", http.StatusBadRequest)
		return
	}
	// Guard against enabling a not-yet-wired kind (catalog Implemented=false).
	if source == feishu.Source && body.Enabled {
		if d := feishu.DescriptorFor(body.Kind); d == nil || !d.Implemented {
			http.Error(w, "kind not available for crawling yet", http.StatusBadRequest)
			return
		}
	}
	if err := h.cfg.Upsert(body); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Keep the periodic sync task in step with the new cadence (best-effort:
	// dispatcher may be unset in tests / early boot).
	if h.disp != nil && body.Enabled {
		_ = h.disp.EnsureRecurring(source, body.Kind, body.IncrementalMinutes)
	}
	cfg, _, err := h.cfg.Get(source, body.Kind)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// sourceFromPath extracts {source} from /api/sources/{source}/collections.
func sourceFromPath(path string) string {
	rest := strings.TrimPrefix(path, "/api/sources/")
	parts := strings.Split(rest, "/")
	if len(parts) < 2 || parts[1] != "collections" {
		return ""
	}
	return parts[0]
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
