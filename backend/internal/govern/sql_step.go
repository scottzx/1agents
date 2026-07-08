package govern

// sql_step.go is the config-driven silver→gold governance layer: a step is a piece
// of SQL that reads N upstream tables (any join, all inside data.db — no ATTACH)
// and writes/updates/batch-upserts an output table. It is the general form of what
// the built-in Go gold governors do by hand, but declared in a connector manifest
// instead of Go — so a new fusion/aggregation rule is pure config.
//
// Each step is cursor-incremental (a "dag"-stage GovernCursor over its driving
// upstream's watermark column) and idempotent (the body's ON CONFLICT expresses
// insert / update / upsert). Steps run in dependency order (a step whose upstream
// is another step's output runs after it). v1 is SQL-only; a Python/TS subprocess
// executor is the next extension.

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/scottzx/1Agents/backend/internal/data"
)

// SQLStep is one declarative silver→gold transform.
type SQLStep struct {
	Name      string   // unique; also the cursor key
	Upstreams []string // tables read (for dependency ordering); the body does the joins
	Output    string   // table written (validated identifier)
	Domain    string   // viewer domain for Output ("" ⇒ not browsable)
	CreateSQL string   // CREATE TABLE IF NOT EXISTS ... (trusted; manifest author = operator)
	Body      string   // INSERT ... SELECT ... WHERE <col> > :since ... ON CONFLICT ... (:since is bound)
	IncrTable string   // driving upstream table for the watermark (validated identifier)
	IncrCol   string   // watermark column on IncrTable, int epoch ms (validated identifier)
}

func (s SQLStep) validate() error {
	if s.Name == "" {
		return fmt.Errorf("govern: sql step needs a name")
	}
	for _, id := range []string{s.Output, s.IncrTable, s.IncrCol} {
		if id != "" && !identRe.MatchString(id) {
			return fmt.Errorf("govern: sql step %s: bad identifier %q", s.Name, id)
		}
	}
	return nil
}

// RunSQLStep runs one step: ensure its output table, execute the body with :since
// bound to the stored watermark, then advance the watermark to the newest driving
// upstream row. Returns rows affected.
func RunSQLStep(dst *data.Store, s SQLStep) (int, error) {
	if err := s.validate(); err != nil {
		return 0, err
	}
	db := dst.SQL()
	if s.CreateSQL != "" {
		if _, err := db.Exec(s.CreateSQL); err != nil {
			return 0, fmt.Errorf("govern: %s createSQL: %w", s.Name, err)
		}
	}
	since, err := dst.GovernCursor("dag", s.Name, "")
	if err != nil {
		return 0, err
	}
	// :since is our own int64 cursor — inline it (driver-agnostic, no injection risk).
	body := strings.ReplaceAll(s.Body, ":since", strconv.FormatInt(since, 10))
	res, err := db.Exec(body)
	if err != nil {
		return 0, fmt.Errorf("govern: %s body: %w", s.Name, err)
	}
	n, _ := res.RowsAffected()

	if s.IncrTable != "" && s.IncrCol != "" {
		var wm int64
		if err := db.QueryRow(
			"SELECT COALESCE(MAX("+s.IncrCol+"), ?) FROM "+s.IncrTable, since,
		).Scan(&wm); err != nil {
			return int(n), fmt.Errorf("govern: %s watermark: %w", s.Name, err)
		}
		if err := dst.SaveGovernCursor("dag", s.Name, "", wm); err != nil {
			return int(n), err
		}
	}
	return int(n), nil
}

// RunSQLSteps runs steps in dependency order (producers before consumers). A
// step's error stops the batch (the caller logs it; the next sync re-runs from the
// unchanged cursor).
func RunSQLSteps(dst *data.Store, steps []SQLStep) error {
	ordered, err := topoSortSteps(steps)
	if err != nil {
		return err
	}
	for _, s := range ordered {
		if _, err := RunSQLStep(dst, s); err != nil {
			return err
		}
	}
	return nil
}

// topoSortSteps orders steps so a step whose upstream is another step's output runs
// after that producer. Steps reading only source tables (silver) have no incoming
// edges. A cycle is an error.
func topoSortSteps(steps []SQLStep) ([]SQLStep, error) {
	producer := map[string]int{}
	for i, s := range steps {
		if s.Output != "" {
			producer[s.Output] = i
		}
	}
	indeg := make([]int, len(steps))
	adj := make([][]int, len(steps))
	for i, s := range steps {
		for _, up := range s.Upstreams {
			if p, ok := producer[up]; ok && p != i {
				adj[p] = append(adj[p], i)
				indeg[i]++
			}
		}
	}
	queue := make([]int, 0, len(steps))
	for i := range steps {
		if indeg[i] == 0 {
			queue = append(queue, i)
		}
	}
	out := make([]SQLStep, 0, len(steps))
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		out = append(out, steps[n])
		for _, m := range adj[n] {
			indeg[m]--
			if indeg[m] == 0 {
				queue = append(queue, m)
			}
		}
	}
	if len(out) != len(steps) {
		return nil, fmt.Errorf("govern: cycle in governance steps")
	}
	return out, nil
}
