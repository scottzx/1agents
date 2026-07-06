// http.go exposes the silver layer read-only for the 数据归一 (data
// normalization) viewer: a per-(domain,source) rollup for the overview and a
// tabular record list for one domain's 多维表格. Read-only — the bronze→silver
// transform runs on its own path (internal/govern, triggered by internal/ingest).
package data

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// Handler serves the read-only silver API over the default data.db.
type Handler struct {
	store *Store
}

// NewHandlerDefault wires a Handler from the default data.db silver store.
func NewHandlerDefault() (*Handler, error) {
	st, err := OpenDefault()
	if err != nil {
		return nil, err
	}
	return &Handler{store: st}, nil
}

// HandleSummary: GET /api/data/summary → per-(domain,source) silver rollup.
func (h *Handler) HandleSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sums, err := h.store.SilverSummary()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, sums)
}

// HandleRecords: GET /api/data/records?domain=&source=&limit= → one domain's
// conformed silver rows as grid rows. source (optional) scopes to one source.
func (h *Handler) HandleRecords(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	domain := q.Get("domain")
	if domain == "" {
		http.Error(w, "domain required", http.StatusBadRequest)
		return
	}
	limit := 0
	if v := q.Get("limit"); v != "" {
		limit, _ = strconv.Atoi(v)
	}
	rows, err := h.store.ListSilver(domain, q.Get("source"), limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
