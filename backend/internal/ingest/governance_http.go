package ingest

import (
	"net/http"
	"strconv"

	"github.com/scottzx/1Agents/backend/internal/data"
	"github.com/scottzx/1Agents/backend/internal/govern"
)

// governance_http.go exposes the 数据治理 DAG + execution log to the frontend: every
// governance step across all three executors — built-in Go governors (silver
// source-cleaning + gold fusion/resolution), generic manifest bronze→silver, and
// declarative SQL / Python silver→gold — with their upstream→output edges, each
// step's last run, the full run log, a per-step re-run, and a drill-in that reads
// any output table schema-free.

type govStep struct {
	Name      string              `json:"name"`
	Lang      string              `json:"lang"` // go | manifest | sql | python
	Tier      string              `json:"tier"` // silver | gold
	Upstreams []string            `json:"upstreams"`
	Output    string              `json:"output"`
	Domain    string              `json:"domain,omitempty"`
	Watermark int64               `json:"watermark"`
	LastRun   *data.GovernanceRun `json:"lastRun,omitempty"`
}

type govNode struct {
	Table  string `json:"table"`
	IsStep bool   `json:"isStep"` // produced by a step (vs a leaf source table)
	Layer  string `json:"layer"`  // bronze | silver | gold
	Domain string `json:"domain,omitempty"`
}

type govEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type govDAG struct {
	Steps []govStep `json:"steps"`
	Nodes []govNode `json:"nodes"`
	Edges []govEdge `json:"edges"`
}

// governanceSteps flattens every registered step (built-in Go + manifest silver +
// SQL + Python) with its last-run status. Order = medallion flow: built-in silver,
// built-in gold, manifest silver, then the declarative silver→gold steps.
func (h *Handler) governanceSteps() []govStep {
	var steps []govStep
	for _, s := range govern.BuiltinSteps() {
		steps = append(steps, h.govStepView(s.Name, "go", s.Tier, s.Upstreams, s.Output, s.Domain, false))
	}
	for _, spec := range h.manifestSilver {
		steps = append(steps, h.govStepView(spec.Table, "manifest", govern.TierSilver, []string{"bronze:" + spec.Source}, spec.Table, spec.Domain, false))
	}
	for _, s := range h.manifestGold {
		steps = append(steps, h.govStepView(s.Name, "sql", govern.TierGold, s.Upstreams, s.Output, s.Domain, true))
	}
	for _, s := range h.manifestScript {
		steps = append(steps, h.govStepView(s.Name, "python", govern.TierGold, s.Upstreams, s.Output, s.Domain, true))
	}
	return steps
}

// govStepView assembles one step's view. dagCursor=true reads the "dag"-stage
// watermark (declarative steps); built-in/manifest-silver steps track their
// progress in per-(source,kind) cursors, so they show 0 and rely on lastRun.
func (h *Handler) govStepView(name, lang, tier string, upstreams []string, output, domain string, dagCursor bool) govStep {
	v := govStep{Name: name, Lang: lang, Tier: tier, Upstreams: upstreams, Output: output, Domain: domain}
	if dagCursor {
		v.Watermark, _ = h.silver.GovernCursor("dag", name, "")
	}
	if lr, ok, _ := h.silver.LastGovernanceRun(name); ok {
		v.LastRun = &lr
	}
	return v
}

// buildDAG derives graph nodes (tables) + edges (upstream→output) from the steps,
// tagging each node's medallion layer for the dependency view.
func buildDAG(steps []govStep) ([]govNode, []govEdge) {
	stepOut := map[string]govStep{} // output table → its producing step
	for _, s := range steps {
		if s.Output != "" {
			stepOut[s.Output] = s
		}
	}
	layerOf := func(tbl string, isStep bool, tier string) string {
		if len(tbl) >= 7 && tbl[:7] == "bronze:" {
			return "bronze"
		}
		if isStep && tier != "" {
			return tier
		}
		if len(tbl) >= 7 && tbl[:7] == "silver_" {
			return "silver"
		}
		return "gold"
	}
	seen := map[string]bool{}
	var nodes []govNode
	add := func(tbl, domain, tier string, isStep bool) {
		if tbl == "" || seen[tbl] {
			return
		}
		seen[tbl] = true
		nodes = append(nodes, govNode{Table: tbl, IsStep: isStep, Layer: layerOf(tbl, isStep, tier), Domain: domain})
	}
	var edges []govEdge
	for _, s := range steps {
		add(s.Output, s.Domain, s.Tier, true)
		for _, up := range s.Upstreams {
			if prod, ok := stepOut[up]; ok {
				add(up, prod.Domain, prod.Tier, true)
			} else {
				add(up, "", "", false)
			}
			edges = append(edges, govEdge{From: up, To: s.Output})
		}
	}
	return nodes, edges
}

// HandleGovernanceDAG: GET /api/data/governance — the full DAG + per-step last-run,
// for the 数据治理 依赖关系 view.
func (h *Handler) HandleGovernanceDAG(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	steps := h.governanceSteps()
	nodes, edges := buildDAG(steps)
	writeJSON(w, http.StatusOK, govDAG{Steps: steps, Nodes: nodes, Edges: edges})
}

// HandleGovernanceRuns: GET /api/data/governance/runs?step=&limit= — the execution
// log (newest first; all steps when step is omitted).
func (h *Handler) HandleGovernanceRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	runs, err := h.silver.ListGovernanceRuns(r.URL.Query().Get("step"), limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

// HandleGovernanceRun: POST /api/data/governance/run?step=&rebuild= — re-run one
// step (when step= is given) or the whole DAG. rebuild=1 clears the step's output
// table + resets its cursor first (删除/rebuild, for union/dedup outputs). Returns
// the updated steps.
func (h *Handler) HandleGovernanceRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	step := r.URL.Query().Get("step")
	if step == "" {
		h.runGovernance()
		writeJSON(w, http.StatusOK, h.governanceSteps())
		return
	}
	rebuild := r.URL.Query().Get("rebuild") == "1" || r.URL.Query().Get("rebuild") == "true"
	found, err := h.runGovernanceStep(step, rebuild)
	if !found {
		http.Error(w, "unknown step: "+step, http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, h.governanceSteps())
}

// HandleGovernanceTable: GET /api/data/governance/table?name=&limit= — one output
// table's rows as schema-free grid rows, for the 数据治理 card drill-in.
func (h *Handler) HandleGovernanceTable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	rows, err := h.silver.ListTable(name, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}
