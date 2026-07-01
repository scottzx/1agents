package meta

import (
	"net/http"
	"strconv"
	"strings"
)

// SearchHandler serves the global "对话历史" quick search over meta.db:
//
//	GET /api/search?q=xxx&limit=30 → {tasks:[...], sessions:[...]}
//
// It searches the two work surfaces the sidebar can jump to — tasks (title /
// description / summary / #number) and chat sessions (name) — joining the
// projects table so each hit carries a display-ready project name. Chat message
// content is NOT here (it's owned by 1acp), so only the structured metadata is
// searchable. The tables are small and local, so a LIKE scan with LIMIT is
// well within budget; no FTS index is warranted at this scale.
func SearchHandler(db *DB) http.HandlerFunc {
	type taskHit struct {
		ID          string `json:"id"`
		ProjectID   string `json:"project_id"`
		ProjectName string `json:"project_name"`
		Number      int    `json:"number"`
		Title       string `json:"title"`
		Status      string `json:"status"`
		Type        string `json:"type"`
	}
	type sessionHit struct {
		ID          string `json:"id"`
		ProjectID   string `json:"project_id"`
		ProjectName string `json:"project_name"`
		TaskID      string `json:"task_id,omitempty"`
		Name        string `json:"name"`
		AgentType   string `json:"agent_type"`
	}
	type result struct {
		Tasks    []taskHit    `json:"tasks"`
		Sessions []sessionHit `json:"sessions"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		out := result{Tasks: []taskHit{}, Sessions: []sessionHit{}}

		q := strings.TrimSpace(r.URL.Query().Get("q"))
		if q == "" {
			writeJSON(w, out)
			return
		}

		limit := 30
		if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 {
			limit = v
		}
		if limit > 100 {
			limit = 100
		}

		like := "%" + escapeLike(q) + "%"
		sqlDB := db.SQL()

		// ── Tasks ─────────────────────────────────────────────────────────
		// Match title / description / summary; also match a bare #number so
		// "#42" or "42" jumps straight to the task.
		numMatch := strings.TrimPrefix(q, "#")
		taskRows, err := sqlDB.Query(`
			SELECT t.id, t.project_id, COALESCE(p.name, ''), t.number, t.title, t.status, t.type
			FROM tasks t
			LEFT JOIN projects p ON p.id = t.project_id
			WHERE t.title LIKE ? ESCAPE '\'
			   OR t.description LIKE ? ESCAPE '\'
			   OR t.summary LIKE ? ESCAPE '\'
			   OR CAST(t.number AS TEXT) = ?
			ORDER BY t.updated_at DESC
			LIMIT ?`, like, like, like, numMatch, limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer taskRows.Close()
		for taskRows.Next() {
			var h taskHit
			if err := taskRows.Scan(&h.ID, &h.ProjectID, &h.ProjectName, &h.Number, &h.Title, &h.Status, &h.Type); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			out.Tasks = append(out.Tasks, h)
		}
		if err := taskRows.Err(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// ── Sessions (会话) ────────────────────────────────────────────────
		// Active sessions only (archived ones stay out of the jump list).
		sessRows, err := sqlDB.Query(`
			SELECT s.id, s.project_id, COALESCE(p.name, ''), s.task_id, s.name, s.agent_type
			FROM sessions s
			LEFT JOIN projects p ON p.id = s.project_id
			WHERE s.archived_at = '' AND s.name LIKE ? ESCAPE '\'
			ORDER BY s.last_event_at DESC, s.created_at DESC
			LIMIT ?`, like, limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer sessRows.Close()
		for sessRows.Next() {
			var h sessionHit
			if err := sessRows.Scan(&h.ID, &h.ProjectID, &h.ProjectName, &h.TaskID, &h.Name, &h.AgentType); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			out.Sessions = append(out.Sessions, h)
		}
		if err := sessRows.Err(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSON(w, out)
	}
}

// escapeLike escapes the LIKE metacharacters (%, _, and the \ escape itself)
// so a user query like "50%" or "a_b" matches literally instead of as a
// wildcard. Paired with `ESCAPE '\'` in the query.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}
