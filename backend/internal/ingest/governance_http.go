package ingest

import (
	"net/http"
	"strconv"

	"github.com/scottzx/1Agents/backend/internal/data"
)

// governance_http.go exposes the 数据治理 DAG + execution log to the frontend: the
// declarative steps (SQL / Python) with their upstream→output edges, each step's
// watermark + last run, the full run log, and a manual re-run. Built-in Go gold
// governors aren't declarative steps, so they surface only as leaf nodes when a
// step reads them.

type govStep struct {
	Name      string              `json:"name"`
	Lang      string              `json:"lang"` // sql | python
	Upstreams []string            `json:"upstreams"`
	Output    string              `json:"output"`
	Domain    string              `json:"domain,omitempty"`
	Watermark int64               `json:"watermark"`
	LastRun   *data.GovernanceRun `json:"lastRun,omitempty"`
}

type govNode struct {
	Table  string `json:"table"`
	IsStep bool   `json:"isStep"` // produced by a step (vs a leaf source table)
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

// governanceSteps flattens the registered SQL + Python steps with live watermark
// and last-run status.
func (h *Handler) governanceSteps() []govStep {
	steps := make([]govStep, 0, len(h.manifestGold)+len(h.manifestScript))
	for _, s := range h.manifestGold {
		steps = append(steps, h.govStepView(s.Name, "sql", s.Upstreams, s.Output, s.Domain))
	}
	for _, s := range h.manifestScript {
		steps = append(steps, h.govStepView(s.Name, "python", s.Upstreams, s.Output, s.Domain))
	}
	return steps
}

func (h *Handler) govStepView(name, lang string, upstreams []string, output, domain string) govStep {
	wm, _ := h.silver.GovernCursor("dag", name, "")
	v := govStep{Name: name, Lang: lang, Upstreams: upstreams, Output: output, Domain: domain, Watermark: wm}
	if lr, ok, _ := h.silver.LastGovernanceRun(name); ok {
		v.LastRun = &lr
	}
	return v
}

// buildDAG derives graph nodes (tables) + edges (upstream→output) from the steps.
func buildDAG(steps []govStep) ([]govNode, []govEdge) {
	stepOut := map[string]string{} // table → domain, marks tables produced by a step
	for _, s := range steps {
		if s.Output != "" {
			stepOut[s.Output] = s.Domain
		}
	}
	seen := map[string]bool{}
	var nodes []govNode
	add := func(tbl, domain string, isStep bool) {
		if tbl == "" || seen[tbl] {
			return
		}
		seen[tbl] = true
		nodes = append(nodes, govNode{Table: tbl, IsStep: isStep, Domain: domain})
	}
	var edges []govEdge
	for _, s := range steps {
		add(s.Output, s.Domain, true)
		for _, up := range s.Upstreams {
			domain, isStep := stepOut[up]
			add(up, domain, isStep)
			edges = append(edges, govEdge{From: up, To: s.Output})
		}
	}
	return nodes, edges
}

// HandleGovernanceDAG: GET /api/data/governance — the declarative DAG + per-step
// watermark/last-run, for the 数据治理 依赖关系 view.
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

// HandleGovernanceRun: POST /api/data/governance/run — re-run the whole governance
// DAG now (records fresh runs into the log) and return the updated steps.
func (h *Handler) HandleGovernanceRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.runManifestSilver()
	writeJSON(w, http.StatusOK, h.governanceSteps())
}
