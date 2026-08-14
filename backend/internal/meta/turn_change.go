package meta

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// TurnChangeRecipeVersion is the independent invalidation key for 本轮资产变化.
// Bump only when the aggregation rules change (how tool_use becomes file ops).
// Global PRAGMA user_version must not be used as a recompute signal.
const TurnChangeRecipeVersion = 3

type TurnChangeOp string

const (
	TurnChangeAdded    TurnChangeOp = "added"
	TurnChangeDeleted  TurnChangeOp = "deleted"
	TurnChangeModified TurnChangeOp = "modified"
)

type TurnChangeSource string

const (
	TurnChangeLive        TurnChangeSource = "live"
	TurnChangeBackfill    TurnChangeSource = "backfill"
	TurnChangeUnavailable TurnChangeSource = "unavailable"
)

type TurnChangeFile struct {
	Path       string       `json:"path"`
	Op         TurnChangeOp `json:"op"`
	Tool       string       `json:"tool,omitempty"`
	ToolCallID string       `json:"toolCallId,omitempty"`
}

type TurnChangeReport struct {
	TurnID        string           `json:"turnId"`
	RecipeVersion int              `json:"recipeVersion"`
	AddedCount    int              `json:"addedCount"`
	DeletedCount  int              `json:"deletedCount"`
	ModifiedCount int              `json:"modifiedCount"`
	Files         []TurnChangeFile `json:"files"`
	Source        TurnChangeSource `json:"source"`
	ComputedAt    time.Time        `json:"computedAt"`
}

// NeedsRecompute reports whether this cached row (or a missing row) must be
// rebuilt from the current history snapshot. lastFinishedTurnID is the
// canonical agent_turns.id of the turn that just ended; empty means none.
func NeedsRecompute(report *TurnChangeReport, recipeVersion int, lastFinishedTurnID string) bool {
	if report == nil || report.TurnID == "" {
		return true
	}
	if lastFinishedTurnID != "" && report.TurnID == lastFinishedTurnID {
		return true
	}
	return report.RecipeVersion != recipeVersion
}

type TurnChangeStore struct {
	db *DB
}

func NewTurnChangeStore(db *DB) *TurnChangeStore {
	return &TurnChangeStore{db: db}
}

func (s *AgentTurnStore) ChangeReports() *TurnChangeStore {
	if s == nil || s.db == nil {
		return nil
	}
	return NewTurnChangeStore(s.db)
}

func (db *DB) ensureTurnChangeReportSchema() error {
	_, err := db.sql.Exec(`
		CREATE TABLE IF NOT EXISTS turn_change_reports (
			turn_id         TEXT PRIMARY KEY,
			recipe_version  INTEGER NOT NULL,
			added_count     INTEGER NOT NULL DEFAULT 0,
			deleted_count   INTEGER NOT NULL DEFAULT 0,
			modified_count  INTEGER NOT NULL DEFAULT 0,
			files_json      TEXT NOT NULL DEFAULT '[]',
			source          TEXT NOT NULL CHECK (
				source IN ('live','backfill','unavailable')
			),
			computed_at     TEXT NOT NULL
		);
	`)
	return err
}

func (s *TurnChangeStore) Get(turnID string) (TurnChangeReport, bool, error) {
	if s == nil {
		return TurnChangeReport{}, false, nil
	}
	report, err := scanTurnChangeReport(s.db.sql.QueryRow(`
		SELECT turn_id, recipe_version, added_count, deleted_count, modified_count,
		       files_json, source, computed_at
		FROM turn_change_reports WHERE turn_id = ?`, turnID))
	if err == sql.ErrNoRows {
		return TurnChangeReport{}, false, nil
	}
	if err != nil {
		return TurnChangeReport{}, false, err
	}
	return report, true, nil
}

func (s *TurnChangeStore) ListByTurnIDs(ids []string) (map[string]TurnChangeReport, error) {
	out := make(map[string]TurnChangeReport, len(ids))
	if s == nil || len(ids) == 0 {
		return out, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	rows, err := s.db.sql.Query(`
		SELECT turn_id, recipe_version, added_count, deleted_count, modified_count,
		       files_json, source, computed_at
		FROM turn_change_reports WHERE turn_id IN (`+strings.Join(placeholders, ",")+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		report, err := scanTurnChangeReport(rows)
		if err != nil {
			return nil, err
		}
		out[report.TurnID] = report
	}
	return out, rows.Err()
}

func (s *TurnChangeStore) Upsert(report TurnChangeReport) error {
	if s == nil {
		return fmt.Errorf("turn change store is unavailable")
	}
	if report.TurnID == "" {
		return fmt.Errorf("turn_id is required")
	}
	if report.RecipeVersion == 0 {
		report.RecipeVersion = TurnChangeRecipeVersion
	}
	if report.Source == "" {
		report.Source = TurnChangeBackfill
	}
	if report.ComputedAt.IsZero() {
		report.ComputedAt = time.Now().UTC()
	}
	if report.Files == nil {
		report.Files = []TurnChangeFile{}
	}
	filesJSON, err := json.Marshal(report.Files)
	if err != nil {
		return err
	}
	_, err = s.db.sql.Exec(`
		INSERT INTO turn_change_reports (
			turn_id, recipe_version, added_count, deleted_count, modified_count,
			files_json, source, computed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(turn_id) DO UPDATE SET
			recipe_version = excluded.recipe_version,
			added_count = excluded.added_count,
			deleted_count = excluded.deleted_count,
			modified_count = excluded.modified_count,
			files_json = excluded.files_json,
			source = excluded.source,
			computed_at = excluded.computed_at
	`, report.TurnID, report.RecipeVersion, report.AddedCount, report.DeletedCount,
		report.ModifiedCount, string(filesJSON), string(report.Source), timeToStr(report.ComputedAt))
	return err
}

// ResolveTurnID maps a history item turnId (canonical id, clientRequestId,
// runtimeRequestId, or promptMessageId) onto agent_turns.id for the given session.
func (s *TurnChangeStore) ResolveTurnID(sessionID, historyTurnID string) (string, bool, error) {
	if s == nil || sessionID == "" || historyTurnID == "" {
		return "", false, nil
	}
	var id string
	err := s.db.sql.QueryRow(`
		SELECT id FROM agent_turns
		WHERE session_id = ? AND (
			id = ? OR client_request_id = ? OR runtime_request_id = ? OR prompt_message_id = ?
		)
		LIMIT 1`, sessionID, historyTurnID, historyTurnID, historyTurnID, historyTurnID).Scan(&id)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return id, true, nil
}

func scanTurnChangeReport(row rowScanner) (TurnChangeReport, error) {
	var report TurnChangeReport
	var filesJSON, source, computedAt string
	if err := row.Scan(
		&report.TurnID, &report.RecipeVersion, &report.AddedCount,
		&report.DeletedCount, &report.ModifiedCount, &filesJSON, &source, &computedAt,
	); err != nil {
		return TurnChangeReport{}, err
	}
	report.Source = TurnChangeSource(source)
	report.ComputedAt = strToTime(computedAt)
	if filesJSON != "" && filesJSON != "null" {
		if err := json.Unmarshal([]byte(filesJSON), &report.Files); err != nil {
			return TurnChangeReport{}, err
		}
	}
	if report.Files == nil {
		report.Files = []TurnChangeFile{}
	}
	return report, nil
}

// HistoryChangeItem is the subset of a 1ACP history_response item used to
// compute 本轮资产变化. Unknown kinds are ignored except for collecting turnId.
type HistoryChangeItem struct {
	Kind       string          `json:"kind"`
	ToolName   string          `json:"toolName"`
	ToolKind   string          `json:"toolKind,omitempty"`
	Input      json.RawMessage `json:"input"`
	ToolCallID string          `json:"toolCallId"`
	TurnID     string          `json:"turnId"`
}

// AggregateTurnChanges groups file ops by history turnId. The same path keeps
// only the last op. Turns with a turnId but no file-touching tools map to an
// empty slice (caller writes source=unavailable). Items without turnId are dropped.
func AggregateTurnChanges(items []HistoryChangeItem) map[string][]TurnChangeFile {
	seen := map[string]map[string]TurnChangeFile{}
	order := map[string][]string{}
	for _, item := range items {
		if item.TurnID == "" {
			continue
		}
		if _, ok := seen[item.TurnID]; !ok {
			seen[item.TurnID] = map[string]TurnChangeFile{}
		}
		if item.Kind != "tool_use" {
			continue
		}
		for _, file := range filesFromTool(item) {
			if _, exists := seen[item.TurnID][file.Path]; !exists {
				order[item.TurnID] = append(order[item.TurnID], file.Path)
			}
			seen[item.TurnID][file.Path] = file
		}
	}
	out := make(map[string][]TurnChangeFile, len(seen))
	for turnID, byPath := range seen {
		files := make([]TurnChangeFile, 0, len(byPath))
		for _, path := range order[turnID] {
			files = append(files, byPath[path])
		}
		out[turnID] = files
	}
	return out
}

func CountTurnChangeOps(files []TurnChangeFile) (added, deleted, modified int) {
	for _, file := range files {
		switch file.Op {
		case TurnChangeAdded:
			added++
		case TurnChangeDeleted:
			deleted++
		case TurnChangeModified:
			modified++
		}
	}
	return added, deleted, modified
}

func filesFromTool(item HistoryChangeItem) []TurnChangeFile {
	kind := classifyToolKind(item.ToolName)
	if kind == "ignore" || kind == "" {
		kind = classifyToolKind(item.ToolKind)
	}
	input := parseToolInput(item.Input)
	switch kind {
	case "ignore", "":
		return nil
	case "delete":
		return stampFiles(pathsFromInput(input), TurnChangeDeleted, item)
	case "move":
		oldPath, newPath := movePaths(input)
		var files []TurnChangeFile
		if oldPath != "" {
			files = append(files, TurnChangeFile{Path: oldPath, Op: TurnChangeDeleted, Tool: item.ToolName, ToolCallID: item.ToolCallID})
		}
		if newPath != "" {
			files = append(files, TurnChangeFile{Path: newPath, Op: TurnChangeAdded, Tool: item.ToolName, ToolCallID: item.ToolCallID})
		}
		return files
	case "execute":
		return filesFromExecute(item, input)
	default: // edit / write / create / patch
		op := TurnChangeModified
		if looksLikeCreate(item.ToolName, input) {
			op = TurnChangeAdded
		}
		return stampFiles(pathsFromInput(input), op, item)
	}
}

func filesFromExecute(item HistoryChangeItem, input map[string]any) []TurnChangeFile {
	cmd := firstString(input, "command", "cmd", "script")
	if !looksLikeDeleteCommand(cmd) {
		return nil
	}
	return stampFiles(pathsFromDeleteCommand(cmd), TurnChangeDeleted, item)
}

var deleteCommandRe = regexp.MustCompile(`(?i)(?:^|[;&|\n])\s*(?:sudo\s+)?(?:rm|unlink|rmdir)\b`)

func looksLikeDeleteCommand(cmd string) bool {
	return strings.TrimSpace(cmd) != "" && deleteCommandRe.MatchString(cmd)
}

func pathsFromDeleteCommand(cmd string) []string {
	var paths []string
	seenDelete := false
	for _, raw := range strings.Fields(cmd) {
		tok := strings.Trim(raw, `"'`)
		low := strings.ToLower(tok)
		if low == "sudo" {
			continue
		}
		if low == "rm" || low == "unlink" || low == "rmdir" {
			seenDelete = true
			continue
		}
		if strings.ContainsAny(tok, ";&|") {
			seenDelete = false
			continue
		}
		if !seenDelete || tok == "--" || strings.HasPrefix(tok, "-") {
			continue
		}
		if looksLikePathToken(tok) {
			paths = append(paths, tok)
		}
	}
	return uniqueNonEmptyPaths(paths)
}

func looksLikePathToken(tok string) bool {
	if tok == "" || tok == "." || tok == ".." {
		return false
	}
	return strings.ContainsAny(tok, `/.\`)
}

func stampFiles(paths []string, op TurnChangeOp, item HistoryChangeItem) []TurnChangeFile {
	files := make([]TurnChangeFile, 0, len(paths))
	for _, path := range paths {
		files = append(files, TurnChangeFile{
			Path:       path,
			Op:         op,
			Tool:       item.ToolName,
			ToolCallID: item.ToolCallID,
		})
	}
	return files
}

var (
	toolKindEdit    = regexp.MustCompile(`(multiedit|str_replace|\bedit\b|\bwrite\b|create_file|apply_patch|notebook)`)
	toolKindDelete  = regexp.MustCompile(`(delete|remove|\brm\b|unlink)`)
	toolKindMove    = regexp.MustCompile(`(\bmove\b|rename|\bmv\b)`)
	toolKindRead    = regexp.MustCompile(`(read|\bcat\b|open_file|view)`)
	toolKindSearch  = regexp.MustCompile(`(grep|glob|search|find|ripgrep|\brg\b)`)
	toolKindFetch   = regexp.MustCompile(`(webfetch|websearch|\bfetch\b|curl|http)`)
	toolKindThink   = regexp.MustCompile(`(think)`)
	toolKindExecute = regexp.MustCompile(`(bash|shell|\brun\b|execute|command|terminal|exec)`)
	toolKindWrite   = regexp.MustCompile(`\bwrite\b`)
)

func classifyToolKind(toolName string) string {
	n := strings.ToLower(strings.TrimSpace(toolName))
	switch n {
	case "edit":
		return "edit"
	case "delete":
		return "delete"
	case "move":
		return "move"
	case "execute":
		return "execute"
	case "read", "search", "think", "fetch", "other":
		return "ignore"
	}
	switch {
	case toolKindEdit.MatchString(n):
		return "edit"
	case toolKindDelete.MatchString(n):
		return "delete"
	case toolKindMove.MatchString(n):
		return "move"
	case toolKindRead.MatchString(n), toolKindSearch.MatchString(n), toolKindFetch.MatchString(n), toolKindThink.MatchString(n):
		return "ignore"
	case toolKindExecute.MatchString(n):
		return "execute"
	default:
		return "ignore"
	}
}

func looksLikeCreate(toolName string, input map[string]any) bool {
	n := strings.ToLower(toolName)
	if strings.Contains(n, "create_file") || toolKindWrite.MatchString(n) {
		return true
	}
	if hasInputKey(input, "old_string", "oldString", "old_str", "edits") {
		return false
	}
	return hasInputKey(input, "contents", "content")
}

func parseToolInput(raw json.RawMessage) map[string]any {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var asMap map[string]any
	if err := json.Unmarshal(raw, &asMap); err == nil {
		return asMap
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil && asString != "" {
		if err := json.Unmarshal([]byte(asString), &asMap); err == nil {
			return asMap
		}
	}
	return nil
}

func pathsFromInput(input map[string]any) []string {
	if input == nil {
		return nil
	}
	var paths []string
	for _, key := range []string{"path", "file_path", "filePath", "file", "filename", "target_file", "targetFile"} {
		paths = append(paths, stringValues(input[key])...)
	}
	for _, key := range []string{"files", "paths"} {
		paths = append(paths, stringValues(input[key])...)
	}
	return uniqueNonEmptyPaths(paths)
}

func movePaths(input map[string]any) (oldPath, newPath string) {
	if input == nil {
		return "", ""
	}
	oldPath = firstString(input, "from", "old_path", "oldPath", "source", "src")
	newPath = firstString(input, "to", "new_path", "newPath", "destination", "dest", "dst")
	return strings.TrimSpace(oldPath), strings.TrimSpace(newPath)
}

func firstString(input map[string]any, keys ...string) string {
	for _, key := range keys {
		vals := stringValues(input[key])
		if len(vals) > 0 {
			return vals[0]
		}
	}
	return ""
}

func hasInputKey(input map[string]any, keys ...string) bool {
	if input == nil {
		return false
	}
	for _, key := range keys {
		if _, ok := input[key]; ok {
			return true
		}
	}
	return false
}

func stringValues(v any) []string {
	switch t := v.(type) {
	case string:
		return []string{t}
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				out = append(out, s)
			} else if m, ok := item.(map[string]any); ok {
				out = append(out, pathsFromInput(m)...)
			}
		}
		return out
	case map[string]any:
		return pathsFromInput(t)
	default:
		return nil
	}
}

func uniqueNonEmptyPaths(paths []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" || strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	return out
}
