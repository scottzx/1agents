package ingest

import (
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"

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
		// silver→gold steps declared inside the connector manifest (vendor-scoped).
		for _, g := range m.Governance {
			h.addGovernStep(g, m.Vendor, sources.ConnectorsDir())
		}
	}
}

// RegisterGovernanceManifests registers standalone governance DAGs (decoupled from
// any connector — 集成/治理解耦): cross-source steps that read any data.db table and
// write entity tables. Scripts resolve relative to the governance dir. Called once
// at startup.
func (h *Handler) RegisterGovernanceManifests(gms []sources.GovernanceManifest) {
	for _, gm := range gms {
		for _, g := range gm.Steps {
			src := g.Source
			if src == "" {
				src = gm.Name
			}
			g.Source = src
			h.addGovernStep(g, src, sources.GovernanceDir())
		}
	}
}

// addGovernStep turns one declared step into an SQL or Python governance step and
// registers its output table with the viewer. scriptBase resolves relative script
// paths (the connectors dir for vendor steps, the governance dir for standalone).
func (h *Handler) addGovernStep(g sources.ManifestStep, sourceTag, scriptBase string) {
	if g.Script != "" {
		script := g.Script
		if !filepath.IsAbs(script) {
			script = filepath.Join(scriptBase, script)
		}
		interp := g.Interpreter
		if interp == "" {
			interp = "python3"
		}
		h.manifestScript = append(h.manifestScript, govern.ScriptStep{
			Name:        g.Name,
			Interpreter: interp,
			Script:      script,
			InputSQL:    g.InputSQL,
			IncrCol:     g.Incremental.Column,
			Output:      g.Output,
			CreateSQL:   g.CreateSQL,
			Conflict:    g.Conflict,
			Upstreams:   g.Upstreams,
			Domain:      g.Domain,
		})
	} else {
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
	}
	if g.Output != "" && g.Domain != "" {
		data.RegisterViewerTable(g.Domain, sourceTag, g.Output)
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
	rec := h.governanceRecorder()
	if len(h.manifestGold) > 0 {
		if err := govern.RunSQLSteps(h.silver, h.manifestGold, rec); err != nil {
			log.Printf("[ingest] manifest gold: %v", err)
		}
	}
	if len(h.manifestScript) > 0 {
		if err := govern.RunScriptSteps(h.silver, h.manifestScript, rec); err != nil {
			log.Printf("[ingest] manifest script gold: %v", err)
		}
	}
}

// governanceRecorder returns a RunRecorder that appends each step run to the
// data.db execution log (数据治理 执行日志).
func (h *Handler) governanceRecorder() govern.RunRecorder {
	return func(r govern.RunRecord) {
		if err := h.silver.RecordGovernanceRun(data.GovernanceRun{
			Step: r.Step, OutputTable: r.Output, Lang: r.Lang,
			Status: r.Status, Rows: r.Rows, DurationMs: r.DurationMs, Error: r.Err,
		}); err != nil {
			log.Printf("[ingest] record governance run %s: %v", r.Step, err)
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
