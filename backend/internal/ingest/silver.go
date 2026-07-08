package ingest

import (
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"
	"time"

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
			Name:         g.Name,
			Interpreter:  interp,
			Script:       script,
			InputSQL:     g.InputSQL,
			IncrCol:      g.Incremental.Column,
			Output:       g.Output,
			CreateSQL:    g.CreateSQL,
			Conflict:     g.Conflict,
			Upstreams:    g.Upstreams,
			Domain:       g.Domain,
			Requirements: g.Requirements,
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

// silver.go wires the governance DAG into ingestion. It runs after every sync
// (silver/gold must never lag bronze) and is exposed as manual re-run endpoints
// for the 数据治理 view. Every step is idempotent + cursor-incremental, so running
// opportunistically after each sync only shapes the records that sync changed.

// runGovernance runs the whole governance DAG once, in order — built-in Go
// governors (silver source-cleaning, then gold fusion/resolution), then the
// declarative manifest steps (generic bronze→silver, SQL silver→gold, Python
// silver→gold) — recording each step to the execution log. Returns rows-written
// per output table. Best-effort: a step error is logged, never fails the sync
// (the next run resumes from the unchanged cursor).
func (h *Handler) runGovernance() map[string]int {
	written := map[string]int{}
	base := h.governanceRecorder()
	rec := func(r govern.RunRecord) {
		base(r)
		if r.Status == "success" && r.Output != "" {
			written[r.Output] += r.Rows
		}
	}

	// ① built-in Go governors (silver + gold), each a first-class recorded step.
	if err := govern.RunBuiltin(h.bronze, h.silver, rec); err != nil {
		log.Printf("[ingest] builtin governance: %v", err)
	}
	// ② manifest generic bronze→silver (REST/CLI sources land into their table).
	for _, spec := range h.manifestSilver {
		h.recordManifestSilver(rec, spec)
	}
	// ③ manifest declarative silver→gold — SQL then Python steps.
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
	return written
}

// recordManifestSilver runs one generic bronze→silver landing and logs it as a
// step (so a manifest REST/CLI source's silver table shows in the DAG + log).
func (h *Handler) recordManifestSilver(rec govern.RunRecorder, spec govern.ManifestSilverSpec) {
	start := time.Now()
	n, err := govern.SilverManifest(h.bronze, h.silver, spec)
	r := govern.RunRecord{Step: spec.Table, Output: spec.Table, Lang: "manifest", Upstreams: []string{"bronze:" + spec.Source}, Rows: n, DurationMs: time.Since(start).Milliseconds(), Status: "success"}
	if err != nil {
		r.Status, r.Err = "failed", err.Error()
		log.Printf("[ingest] manifest silver %s/%s: %v", spec.Source, spec.Kind, err)
	}
	rec(r)
}

// runGovernanceStep re-runs a single step by name, across all executors (built-in
// Go / manifest silver / SQL / Python). When rebuild is set, a declarative step's
// output is truncated + its cursor reset first, for a clean rebuild of union/dedup
// tables (built-in shared tables are only cursor-reset, never truncated). Returns
// whether a step with that name was found.
func (h *Handler) runGovernanceStep(name string, rebuild bool) (bool, error) {
	rec := h.governanceRecorder()
	if _, found, err := govern.RunBuiltinStep(h.bronze, h.silver, name, rec); found {
		return true, err
	}
	for _, spec := range h.manifestSilver {
		if spec.Table == name {
			h.recordManifestSilver(rec, spec)
			return true, nil
		}
	}
	for _, s := range h.manifestGold {
		if s.Name == name {
			if rebuild {
				if err := h.rebuildStep(s.Output, name); err != nil {
					return true, err
				}
			}
			start := time.Now()
			n, err := govern.RunSQLStep(h.silver, s)
			rec(govern.RunRecord{Step: s.Name, Output: s.Output, Lang: "sql", Upstreams: s.Upstreams, Rows: n, DurationMs: time.Since(start).Milliseconds(), Status: statusOf(err), Err: errStr(err)})
			return true, err
		}
	}
	for _, s := range h.manifestScript {
		if s.Name == name {
			if rebuild {
				if err := h.rebuildStep(s.Output, name); err != nil {
					return true, err
				}
			}
			start := time.Now()
			n, err := govern.RunScriptStep(h.silver, s)
			rec(govern.RunRecord{Step: s.Name, Output: s.Output, Lang: "python", Upstreams: s.Upstreams, Rows: n, DurationMs: time.Since(start).Milliseconds(), Status: statusOf(err), Err: errStr(err)})
			return true, err
		}
	}
	return false, nil
}

// rebuildStep clears a declarative step's output table + resets its cursor, so the
// next run recomputes it from scratch (删除/rebuild — for union/dedup tables whose
// upsert can't retract rows that no longer belong).
func (h *Handler) rebuildStep(output, name string) error {
	if err := h.silver.TruncateGovernanceOutput(output); err != nil {
		return err
	}
	return h.silver.SaveGovernCursor("dag", name, "", 0)
}

func statusOf(err error) string {
	if err != nil {
		return "failed"
	}
	return "success"
}

func errStr(err error) string {
	if err != nil {
		return err.Error()
	}
	return ""
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

// afterSyncSilver runs the governance DAG and folds a compact summary into a sync
// task's result map, so the work-order result shows what was normalized. Errors
// are logged, not propagated (a normalize hiccup never fails a good sync).
func (h *Handler) afterSyncSilver(result map[string]any) {
	result["governance"] = h.runGovernance()
}

// HandleRunSilver: POST /api/data/silver/run → run the whole governance DAG now and
// return rows-written per output table. The manual "重新治理" control.
func (h *Handler) HandleRunSilver(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(h.runGovernance())
}
