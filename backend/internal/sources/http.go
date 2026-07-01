// http.go exposes the bronze store read-only for the 数据源管理 (data-source
// management) UI: a per-(source,kind) rollup for the overview cards, and a
// tabular record list for the 多维表格 (bitable) detail view. Read-only —
// pulling/governance stay on their own paths. Records are the raw pulled data;
// for vCard contacts we also surface parsed fields so the grid has real columns.
package sources

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/scottzx/1Agents/backend/internal/icloud"
)

// Handler serves the read-only bronze API over the default sync.db.
type Handler struct {
	store *Store
}

// NewHandlerDefault wires a Handler from the default sync.db bronze store.
func NewHandlerDefault() (*Handler, error) {
	st, err := OpenDefault()
	if err != nil {
		return nil, err
	}
	return &Handler{store: st}, nil
}

// HandleSummary: GET /api/sources/summary → per-(source,kind) rollup for the
// overview cards.
func (h *Handler) HandleSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sums, err := h.store.Summaries()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, sums)
}

// Field is one native property of a record, key = the source's own field name
// (vCard FN/TEL/EMAIL/…). No schema is imposed here — the frontend builds
// columns from whatever keys appear, so the same viewer serves contacts today
// and todos/calendars later. Governance (not this endpoint) is where fields get
// normalized; the viewer stays faithful to the raw record.
type Field struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// SourceRecordRow is one bronze record for the grid: the generic envelope plus
// its native fields in source order (repeats preserved, e.g. two TELs).
type SourceRecordRow struct {
	UID         string  `json:"uid"`
	Collection  string  `json:"collection"`
	ETag        string  `json:"etag"`
	ContentType string  `json:"contentType"`
	Deleted     bool    `json:"deleted"`
	FetchedAt   int64   `json:"fetchedAt"`
	Fields      []Field `json:"fields"`
	Preview     string  `json:"preview"`
}

// HandleRecords: GET /api/sources/records?source=&kind=&limit= → the bronze
// records for a (source, kind) as grid rows with native fields.
func (h *Handler) HandleRecords(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	source, kind := q.Get("source"), q.Get("kind")
	if source == "" || kind == "" {
		http.Error(w, "source and kind required", http.StatusBadRequest)
		return
	}
	limit := 0
	if v := q.Get("limit"); v != "" {
		limit, _ = strconv.Atoi(v)
	}
	recs, err := h.store.ListRecords(source, kind, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rows := make([]SourceRecordRow, 0, len(recs))
	for _, rec := range recs {
		rows = append(rows, SourceRecordRow{
			UID: rec.UID, Collection: rec.Collection, ETag: rec.ETag,
			ContentType: rec.ContentType, Deleted: rec.Deleted, FetchedAt: rec.FetchedAt,
			Fields: recordFields(rec.ContentType, kind, rec.Payload), Preview: preview(rec.Payload),
		})
	}
	writeJSON(w, http.StatusOK, rows)
}

// recordFields parses a raw payload into native (key, value) fields by content
// type. vCard is handled today; other formats (iCal, JSON) plug in here as
// their pullers land in bronze. Unknown formats fall back to a single raw
// field so nothing is silently dropped.
func recordFields(contentType, kind, payload string) []Field {
	if strings.Contains(contentType, "vcard") || kind == KindContact {
		props := icloud.VCardProps(payload)
		out := make([]Field, 0, len(props))
		for _, p := range props {
			out = append(out, Field{Key: p[0], Value: capValue(p[1])})
		}
		return out
	}
	return []Field{{Key: "payload", Value: capValue(payload)}}
}

// capValue bounds a single field value so a stray blob (e.g. a base64 PHOTO)
// can't bloat the response; the full raw stays in bronze.
func capValue(s string) string {
	const max = 500
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}

// preview is a short, single-line snippet of a raw payload for the grid.
func preview(payload string) string {
	s := strings.Join(strings.Fields(payload), " ")
	if len(s) > 160 {
		return s[:160]
	}
	return s
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
