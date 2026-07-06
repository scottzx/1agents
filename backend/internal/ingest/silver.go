package ingest

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/scottzx/1Agents/backend/internal/govern"
)

// silver.go wires the bronze→silver transform (internal/govern) into ingestion:
// it runs after every sync (silver should never lag bronze) and is exposed as a
// manual re-run endpoint for the 数据归一 viewer. The transform is idempotent and
// cursor-incremental, so running it opportunistically after each sync only
// shapes the records that sync just changed.

// runSilver shapes any newly-synced bronze into the silver tables. Best-effort:
// callers log the error but never fail the sync task on it — a silver hiccup must
// not roll back a good bronze pull (silver re-runs safely next time).
func (h *Handler) runSilver() (govern.SilverStats, error) {
	return govern.Silver(h.bronze, h.silver)
}

// runGold fuses silver into gold (identity resolution + threads + messages).
// Runs on the same data.db, after silver, so gold never lags the just-cleaned
// silver. Idempotent and cursor-incremental like silver.
func (h *Handler) runGold() (govern.GoldStats, error) {
	return govern.Gold(h.silver)
}

// afterSyncSilver runs the silver→gold pipeline and folds a compact summary into
// a sync task's result map, so the work-order result shows what was normalized.
// Errors are logged, not propagated (a normalize hiccup never fails a good sync).
func (h *Handler) afterSyncSilver(result map[string]any) {
	stats, err := h.runSilver()
	if err != nil {
		log.Printf("[ingest] silver after sync: %v", err)
		return
	}
	result["silver"] = map[string]int{
		"contacts": stats.Contacts,
		"messages": stats.Messages,
		"events":   stats.Events,
		"todos":    stats.Todos,
	}
	gold, err := h.runGold()
	if err != nil {
		log.Printf("[ingest] gold after sync: %v", err)
		return
	}
	result["gold"] = goldSummary(gold)
}

// HandleRunSilver: POST /api/data/silver/run → run the full bronze→silver→gold
// pipeline now and return the per-stage counts. The manual "重新清洗" control
// behind the 数据归一 viewer.
func (h *Handler) HandleRunSilver(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	stats, err := h.runSilver()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	gold, err := h.runGold()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"contacts": stats.Contacts,
		"messages": stats.Messages,
		"events":   stats.Events,
		"todos":    stats.Todos,
		"gold":     goldSummary(gold),
	})
}

func goldSummary(g govern.GoldStats) map[string]int {
	return map[string]int{
		"threads":  g.Threads,
		"messages": g.Messages,
		"contacts": g.Contacts,
	}
}
