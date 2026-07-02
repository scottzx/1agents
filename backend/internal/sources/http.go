// http.go exposes the bronze store read-only for the 数据源管理 (data-source
// management) UI: a per-(source,kind) rollup for the overview cards, and a
// tabular record list for the 多维表格 (bitable) detail view. Read-only —
// pulling/governance stay on their own paths. Records are the raw pulled data;
// for vCard contacts we also surface parsed fields so the grid has real columns.
package sources

import (
	"encoding/json"
	"net/http"
	"sort"
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
	account := q.Get("account") // optional: scope to one account_id (源为中心)
	limit := 0
	if v := q.Get("limit"); v != "" {
		limit, _ = strconv.Atoi(v)
	}
	recs, err := h.store.ListRecords(source, account, kind, limit)
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
// type, so the 多维表格 viewer gets real columns instead of one opaque blob.
// vCard → parsed props; JSON → one field per top-level key (Feishu et al.);
// unknown formats fall back to a single raw field so nothing is silently dropped.
// This is a viewer-side projection only — bronze still holds the verbatim
// payload, and governance (gold) is a separate step.
func recordFields(contentType, kind, payload string) []Field {
	if strings.Contains(contentType, "vcard") || kind == KindContact {
		props := icloud.VCardProps(payload)
		out := make([]Field, 0, len(props))
		for _, p := range props {
			out = append(out, Field{Key: p[0], Value: capValue(p[1])})
		}
		return out
	}
	if strings.Contains(contentType, "json") {
		if fields, ok := jsonFields(payload); ok {
			return fields
		}
	}
	return []Field{{Key: "payload", Value: capValue(payload)}}
}

// jsonFields flattens a JSON object's top-level keys into table columns: scalars
// become their string form, nested objects/arrays their compact JSON. Keys are
// sorted for a stable column order. ok=false when the payload isn't a JSON
// object (caller falls back to the raw field).
func jsonFields(payload string) (fields []Field, ok bool) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &m); err != nil {
		return nil, false
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fields = make([]Field, 0, len(keys))
	for _, k := range keys {
		fields = append(fields, Field{Key: k, Value: capValue(jsonScalar(m[k]))})
	}
	return fields, true
}

// jsonScalar renders a JSON value for a cell: a string unquoted, anything else
// (number/bool/object/array/null) as its literal JSON text.
func jsonScalar(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return strings.TrimSpace(string(raw))
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
