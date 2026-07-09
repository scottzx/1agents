package ingest

// push_http.go is the inbound-push receiver — the mirror of the pull sync path.
// Local agents POST their processed records to /api/data/push/{source}/{kind};
// each lands verbatim in bronze (source_records, the retention hook) and then
// governance runs so any declared silver/gold table refreshes. There is no cursor
// and no schedule: the agent decides when to push. Auth reuses the per-source
// bearer store as a shared secret (a push source has no outbound token to collide
// with), presented as Authorization: Bearer <key> or X-Push-Token.

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/scottzx/1Agents/backend/internal/sources"
)

// maxPushBytes caps one push body (larger than the connector paste limit — an
// agent may batch many processed records).
const maxPushBytes = 8 << 20

// HandlePushInfo serves GET /api/sources/{source}/push — the push metadata the UI
// needs to render a push source: each push kind with its declared table schema and
// dedup key. The data endpoint itself is POST /api/data/push/{source}/{kind}.
func (h *Handler) HandlePushInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	source := sourceFromAction(r.URL.Path, "push")
	if source == "" || !sources.IsPushSource(source) {
		http.NotFound(w, r)
		return
	}
	kinds := []map[string]any{}
	for _, d := range sources.RESTKinds(source) {
		if d.Transport != "push" {
			continue
		}
		kinds = append(kinds, map[string]any{
			"kind":     d.Kind,
			"label":    d.Label,
			"uidField": d.UIDField,
			"schema":   d.Schema,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": source, "kinds": kinds})
}

// HandlePush serves POST /api/data/push/{source}/{kind}[?collection=].
func (h *Handler) HandlePush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/data/push/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		http.Error(w, "path must be /api/data/push/{source}/{kind}", http.StatusBadRequest)
		return
	}
	source, kind := parts[0], parts[1]
	desc, ok := sources.PushDescriptorFor(source, kind)
	if !ok {
		http.Error(w, "unknown push source/kind", http.StatusNotFound)
		return
	}
	accountID := h.manifestAccountID(source)
	if !h.pushAuthorized(source, accountID, r) {
		http.Error(w, "unauthorized: set a push key for this source and present it as Bearer", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxPushBytes))
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	items, err := decodePushItems(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	collection := r.URL.Query().Get("collection")
	recs, rejects := sources.BuildPushRecords(desc, collection, items)
	if len(rejects) > 0 {
		// Atomic contract: any schema violation rejects the whole batch so the agent
		// fixes and re-pushes rather than landing a partial set.
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"received": len(items), "committed": 0, "rejects": rejects,
		})
		return
	}
	changed, err := h.bronze.CommitPage(source, accountID, recs, sources.Cursor{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Flow the freshly-landed rows up the medallion (silver/gold), same as a pull.
	// Each governance step is cursor-incremental, so this only shapes what changed.
	governance := map[string]any{}
	h.afterSyncSilver(governance)
	writeJSON(w, http.StatusOK, map[string]any{
		"received": len(items), "committed": changed, "governance": governance,
	})
}

// pushAuthorized checks the request carries the source's configured push key. A
// push source with no key set rejects everything (you must set a secret first).
func (h *Handler) pushAuthorized(source, accountID string, r *http.Request) bool {
	want, ok, _ := sources.LoadBearerToken(source, accountID)
	if !ok || want == "" {
		return false
	}
	got := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if got == "" {
		got = strings.TrimSpace(r.Header.Get("X-Push-Token"))
	}
	return got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// decodePushItems accepts either a single JSON object or a JSON array of objects.
func decodePushItems(body []byte) ([]json.RawMessage, error) {
	b := bytes.TrimSpace(body)
	if len(b) == 0 {
		return nil, fmt.Errorf("empty body")
	}
	if b[0] == '[' {
		var arr []json.RawMessage
		if err := json.Unmarshal(b, &arr); err != nil {
			return nil, fmt.Errorf("invalid JSON array")
		}
		if len(arr) == 0 {
			return nil, fmt.Errorf("empty array")
		}
		return arr, nil
	}
	var obj json.RawMessage
	if err := json.Unmarshal(b, &obj); err != nil {
		return nil, fmt.Errorf("invalid JSON")
	}
	return []json.RawMessage{obj}, nil
}
