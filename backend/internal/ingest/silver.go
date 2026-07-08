package ingest

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/scottzx/1Agents/backend/internal/data"
	"github.com/scottzx/1Agents/backend/internal/govern"
	"github.com/scottzx/1Agents/backend/internal/sources"
)

// RegisterManifestGovernance derives the generic bronze→silver specs from the
// connector manifests and registers each target table with the 数据归一 viewer.
// Idempotent per process start; called once during startup wiring. A collection
// without a silver.table is skipped (no viewer landing).
func (h *Handler) RegisterManifestGovernance(ms []sources.Manifest) {
	for _, m := range ms {
		for _, c := range m.Collections {
			table := c.Silver.Table
			if table == "" {
				continue
			}
			domain := c.Silver.Domain
			if domain == "" {
				domain = c.Domain
			}
			h.manifestSilver = append(h.manifestSilver, govern.ManifestSilverSpec{
				Source:  m.Vendor,
				Kind:    c.Kind,
				Table:   table,
				Domain:  domain,
				Promote: c.Silver.Promote,
			})
			data.RegisterViewerTable(domain, m.Vendor, table)
		}
		// silver→gold SQL steps (multi-upstream join / upsert, all in data.db).
		for _, g := range m.Governance {
			h.manifestGold = append(h.manifestGold, govern.SQLStep{
				Name:      g.Name,
				Upstreams: g.Upstreams,
				Output:    g.Output,
				Domain:    g.Domain,
				CreateSQL: g.CreateSQL,
				Body:      g.Body,
				IncrTable: g.Incremental.Table,
				IncrCol:   g.Incremental.Column,
			})
			if g.Output != "" && g.Domain != "" {
				data.RegisterViewerTable(g.Domain, m.Vendor, g.Output)
			}
		}
	}
}

// runManifestSilver lands every manifest source's newly-synced bronze into its
// generic silver table, then runs the declarative silver→gold SQL steps. Both are
// cursor-incremental + idempotent, so running after any sync only shapes changes.
func (h *Handler) runManifestSilver() {
	for _, spec := range h.manifestSilver {
		if _, err := govern.SilverManifest(h.bronze, h.silver, spec); err != nil {
			log.Printf("[ingest] manifest silver %s/%s: %v", spec.Source, spec.Kind, err)
		}
	}
	if len(h.manifestGold) > 0 {
		if err := govern.RunSQLSteps(h.silver, h.manifestGold); err != nil {
			log.Printf("[ingest] manifest gold: %v", err)
		}
	}
}

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
	h.runManifestSilver() // manifest REST sources land into their generic silver tables
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
	h.runManifestSilver()
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
		"events":   g.Events,
		"todos":    g.Todos,
	}
}
