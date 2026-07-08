package ingest

import (
	"io"
	"net/http"
	"strings"

	"github.com/scottzx/1Agents/backend/internal/sources"
)

// connectors_http.go is the 自定义连接器 surface: a manifest can be added from the
// UI (paste YAML) and takes effect immediately — no file drop, no restart. The
// POST endpoint validates + persists the manifest, then hot-registers it (vendor +
// descriptors + account + config + governance + recurring sync). Manifest sources
// are served by one source-agnostic catch-all (HandleManifestRoute) rather than
// per-vendor routes, so a hot-added vendor is reachable without touching the mux.

// AddConnector validates raw manifest YAML, persists it to the connectors dir, and
// hot-registers it at runtime. Returns the parsed manifest.
func (h *Handler) AddConnector(yamlBytes []byte) (sources.Manifest, error) {
	m, err := sources.ParseManifest(yamlBytes)
	if err != nil {
		return m, err
	}
	if err := sources.ValidateManifest(m); err != nil {
		return m, err
	}
	if err := sources.SaveManifest(m.Vendor, yamlBytes); err != nil {
		return m, err
	}
	// Runtime registration — the same steps server.go runs at startup, for one manifest.
	sources.RegisterManifest(m)
	h.RegisterManifestSyncFn(m.Vendor)
	if err := h.SeedManifestAccounts([]sources.Manifest{m}); err != nil {
		return m, err
	}
	if err := h.SeedManifestConfigs([]sources.Manifest{m}); err != nil {
		return m, err
	}
	h.RegisterManifestGovernance([]sources.Manifest{m})
	// Arm recurring sync for enabled collections (idempotent).
	if h.disp != nil {
		for _, c := range m.Collections {
			if c.Defaults.Enabled {
				_ = h.disp.EnsureRecurring(m.Vendor, c.Kind, c.Defaults.IncrementalMinutes)
			}
		}
	}
	return m, nil
}

// HandleConnectors serves /api/sources/connectors.
//
//	GET  → the installed connector manifests
//	POST → add a connector (body = manifest YAML), hot-registered immediately
func (h *Handler) HandleConnectors(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		ms, err := sources.LoadManifests()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, ms)
	case http.MethodPost:
		body, err := io.ReadAll(io.LimitReader(r.Body, 256<<10))
		if err != nil {
			http.Error(w, "read body", http.StatusBadRequest)
			return
		}
		m, err := h.AddConnector(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"vendor": m.Vendor, "label": m.Label, "collections": len(m.Collections)})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleManifestRoute is the source-agnostic catch-all for manifest REST sources,
// registered at /api/sources/ (built-in vendors keep their more-specific explicit
// routes, which win). It dispatches /api/sources/{source}/{action} to the shared
// handlers — so a hot-added vendor is reachable with no new mux registration.
func (h *Handler) HandleManifestRoute(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/sources/")
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 2 || !sources.IsRESTSource(parts[0]) {
		http.NotFound(w, r)
		return
	}
	switch parts[1] {
	case "collections":
		h.HandleCollections(w, r)
	case "sync":
		h.HandleSync(w, r)
	case "history":
		h.HandleHistory(w, r)
	case "schedules":
		h.HandleSchedules(w, r)
	case "bearer":
		h.HandleBearer(w, r)
	default:
		http.NotFound(w, r)
	}
}
