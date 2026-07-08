package govern

// script_step.go is the Python (subprocess) governance executor — the escape hatch
// for transforms SQL can't express (nested-array aggregation, vCard/RRULE parsing,
// messy JSON). The framework owns schema, cursor, DDL and the upsert; the script is
// a pure row transform: it reads input rows as NDJSON on stdin and emits output rows
// as NDJSON on stdout. It gets a minimal env (PATH/LANG only — no token, no DB path),
// a timeout, and never touches the database directly. Python is the one interpreter:
// python3 is ubiquitous, runs a .py with zero build, and its stdlib covers the messy
// cases without dependencies.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/scottzx/1Agents/backend/internal/data"
)

// scriptTimeout bounds one script step's run.
const scriptTimeout = 2 * time.Minute

// ScriptStep is one declarative transform whose body is an external script. InputSQL
// selects the rows to process (any join over data.db, filtered WHERE <IncrCol> > :since);
// each row is streamed to the script; the script's emitted rows are upserted into
// Output on the Conflict key.
type ScriptStep struct {
	Name        string
	Interpreter string   // default "python3"
	Script      string   // absolute path to the script file
	InputSQL    string   // SELECT ... WHERE <IncrCol> > :since (multi-upstream join allowed)
	IncrCol     string   // watermark column present in each input row
	Output      string   // output table (validated identifier)
	CreateSQL   string   // CREATE TABLE IF NOT EXISTS ...
	Conflict    []string // ON CONFLICT(...) columns for the upsert
	Upstreams   []string // for dependency ordering
	Domain      string   // viewer domain for Output
}

func (s ScriptStep) validate() error {
	if s.Name == "" || s.Script == "" || s.Output == "" || len(s.Conflict) == 0 {
		return fmt.Errorf("govern: script step %q needs name/script/output/conflict", s.Name)
	}
	ids := append([]string{s.Output, s.IncrCol}, s.Conflict...)
	for _, id := range ids {
		if id != "" && !identRe.MatchString(id) {
			return fmt.Errorf("govern: script step %s: bad identifier %q", s.Name, id)
		}
	}
	return nil
}

// RunScriptStep streams the step's incremental input rows through the script and
// upserts the emitted rows into Output, then advances the watermark. Returns the
// number of output rows written.
func RunScriptStep(dst *data.Store, s ScriptStep) (int, error) {
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
	inSQL := strings.ReplaceAll(s.InputSQL, ":since", strconv.FormatInt(since, 10))
	inRows, maxIncr, err := queryRowsAsMaps(db, inSQL, s.IncrCol, since)
	if err != nil {
		return 0, fmt.Errorf("govern: %s inputSQL: %w", s.Name, err)
	}
	if len(inRows) == 0 {
		return 0, nil
	}

	// Feed input rows to the script as NDJSON on stdin.
	var stdin bytes.Buffer
	for _, r := range inRows {
		b, err := json.Marshal(r)
		if err != nil {
			return 0, err
		}
		stdin.Write(b)
		stdin.WriteByte('\n')
	}
	interp := s.Interpreter
	if interp == "" {
		interp = "python3"
	}
	ctx, cancel := context.WithTimeout(context.Background(), scriptTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, interp, s.Script)
	cmd.Stdin = &stdin
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "LANG=C.UTF-8"} // no secrets, no DB path
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("govern: %s script: %w: %s", s.Name, err, strings.TrimSpace(stderr.String()))
	}

	// Upsert the emitted NDJSON rows.
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, line := range bytes.Split(out, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var row map[string]any
		if err := json.Unmarshal(line, &row); err != nil {
			_ = tx.Rollback()
			return n, fmt.Errorf("govern: %s emitted bad json: %w", s.Name, err)
		}
		if err := upsertMap(tx, s.Output, row, s.Conflict); err != nil {
			_ = tx.Rollback()
			return n, err
		}
		n++
	}
	if err := tx.Commit(); err != nil {
		return n, err
	}
	if s.IncrCol != "" {
		if err := dst.SaveGovernCursor("dag", s.Name, "", maxIncr); err != nil {
			return n, err
		}
	}
	return n, nil
}

// RunScriptSteps runs script steps in dependency order.
func RunScriptSteps(dst *data.Store, steps []ScriptStep) error {
	order, err := topoOrder(len(steps), func(i int) ([]string, string) {
		return steps[i].Upstreams, steps[i].Output
	})
	if err != nil {
		return err
	}
	for _, i := range order {
		if _, err := RunScriptStep(dst, steps[i]); err != nil {
			return err
		}
	}
	return nil
}

// queryRowsAsMaps runs query and returns each row as a column→value map (bytes are
// decoded to strings so they JSON-encode as text), plus the max value seen in incrCol.
func queryRowsAsMaps(db *sql.DB, query, incrCol string, since int64) ([]map[string]any, int64, error) {
	rows, err := db.Query(query)
	if err != nil {
		return nil, since, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, since, err
	}
	maxIncr := since
	var out []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, since, err
		}
		m := make(map[string]any, len(cols))
		for i, c := range cols {
			m[c] = normalizeSQLValue(vals[i])
			if c == incrCol {
				if iv, ok := vals[i].(int64); ok && iv > maxIncr {
					maxIncr = iv
				}
			}
		}
		out = append(out, m)
	}
	return out, maxIncr, rows.Err()
}

// normalizeSQLValue turns a driver value into a JSON-friendly one ([]byte → string).
func normalizeSQLValue(v any) any {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return v
}

// upsertMap upserts one emitted row into table on the conflict key. Emitted column
// names are validated (whitelist) since they come from the script; nested values are
// JSON-encoded to text.
func upsertMap(tx *sql.Tx, table string, row map[string]any, conflict []string) error {
	if !identRe.MatchString(table) {
		return fmt.Errorf("govern: bad output table %q", table)
	}
	cols := make([]string, 0, len(row))
	for c := range row {
		if !identRe.MatchString(c) {
			return fmt.Errorf("govern: script emitted bad column %q", c)
		}
		cols = append(cols, c)
	}
	sort.Strings(cols)

	conflictSet := map[string]bool{}
	for _, c := range conflict {
		conflictSet[c] = true
	}
	ph := make([]string, len(cols))
	args := make([]any, len(cols))
	var sets []string
	for i, c := range cols {
		ph[i] = "?"
		args[i] = scalarArg(row[c])
		if !conflictSet[c] {
			sets = append(sets, c+"=excluded."+c)
		}
	}
	action := "DO NOTHING"
	if len(sets) > 0 {
		action = "DO UPDATE SET " + strings.Join(sets, ", ")
	}
	stmt := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) ON CONFLICT(%s) %s",
		table, strings.Join(cols, ", "), strings.Join(ph, ", "), strings.Join(conflict, ", "), action)
	_, err := tx.Exec(stmt, args...)
	return err
}

// scalarArg keeps scalars as-is and JSON-encodes nested values to text.
func scalarArg(v any) any {
	switch v.(type) {
	case map[string]any, []any:
		b, _ := json.Marshal(v)
		return string(b)
	default:
		return v
	}
}

// topoOrder returns node indices in dependency order (a node whose upstream is
// another node's output comes after it). deps(i) yields (upstreams, output).
func topoOrder(n int, deps func(i int) (upstreams []string, output string)) ([]int, error) {
	producer := map[string]int{}
	ups := make([][]string, n)
	for i := 0; i < n; i++ {
		u, out := deps(i)
		ups[i] = u
		if out != "" {
			producer[out] = i
		}
	}
	indeg := make([]int, n)
	adj := make([][]int, n)
	for i := 0; i < n; i++ {
		for _, up := range ups[i] {
			if p, ok := producer[up]; ok && p != i {
				adj[p] = append(adj[p], i)
				indeg[i]++
			}
		}
	}
	queue := make([]int, 0, n)
	for i := 0; i < n; i++ {
		if indeg[i] == 0 {
			queue = append(queue, i)
		}
	}
	out := make([]int, 0, n)
	for len(queue) > 0 {
		x := queue[0]
		queue = queue[1:]
		out = append(out, x)
		for _, m := range adj[x] {
			indeg[m]--
			if indeg[m] == 0 {
				queue = append(queue, m)
			}
		}
	}
	if len(out) != n {
		return nil, fmt.Errorf("govern: cycle in script steps")
	}
	return out, nil
}
