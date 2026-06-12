package meta

import (
	"database/sql"
	"strings"
	"time"
)

// SessionStore is the SQLite-backed replacement for the legacy
// agent-sessions.json store. Method set mirrors the old agent.Store so the
// HTTP handlers swap over without changes.
type SessionStore struct {
	db *DB
}

// NewSessionStore returns a SessionStore over db.
func NewSessionStore(db *DB) *SessionStore {
	return &SessionStore{db: db}
}

const sessionCols = `id, project_id, task_id, name, agent_type, cc_project,
	cc_session_id, acp_session_id, session_key, permission_mode,
	created_at, last_event_at`

func scanSession(r rowScanner) (ChatSessionRecord, error) {
	var rec ChatSessionRecord
	var createdAt, lastEventAt string
	if err := r.Scan(&rec.ID, &rec.WorkspaceID, &rec.TaskID, &rec.Name, &rec.AgentType,
		&rec.CcProject, &rec.CcSessionID, &rec.AcpSessionID, &rec.SessionKey,
		&rec.PermissionMode, &createdAt, &lastEventAt); err != nil {
		return ChatSessionRecord{}, err
	}
	rec.CreatedAt = strToTime(createdAt)
	rec.LastEventAt = strToTime(lastEventAt)
	return rec, nil
}

// ListByWorkspace returns all chat sessions belonging to a workspace,
// sorted newest-first by CreatedAt.
func (s *SessionStore) ListByWorkspace(workspaceID string) ([]ChatSessionRecord, error) {
	rows, err := s.db.sql.Query(
		`SELECT `+sessionCols+` FROM sessions
		 WHERE project_id = ? ORDER BY created_at DESC`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ChatSessionRecord{}
	for rows.Next() {
		rec, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// Get returns a single record by id, or (zero, false) if not found.
func (s *SessionStore) Get(id string) (ChatSessionRecord, bool, error) {
	row := s.db.sql.QueryRow(`SELECT `+sessionCols+` FROM sessions WHERE id = ?`, id)
	rec, err := scanSession(row)
	if err == sql.ErrNoRows {
		return ChatSessionRecord{}, false, nil
	}
	if err != nil {
		return ChatSessionRecord{}, false, err
	}
	return rec, true, nil
}

// Add inserts a new record. Returns ErrDuplicate if id already exists.
func (s *SessionStore) Add(rec ChatSessionRecord) error {
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now().UTC()
	}
	res, err := s.db.sql.Exec(`
		INSERT INTO sessions (id, project_id, task_id, name, agent_type, cc_project,
			cc_session_id, acp_session_id, session_key, permission_mode,
			created_at, last_event_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO NOTHING`,
		rec.ID, rec.WorkspaceID, rec.TaskID, rec.Name, rec.AgentType, rec.CcProject,
		rec.CcSessionID, rec.AcpSessionID, rec.SessionKey, rec.PermissionMode,
		timeToStr(rec.CreatedAt), timeToStr(rec.LastEventAt))
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrDuplicate
	}
	return nil
}

// Delete removes the record with the given id. Returns ErrNotFound if no match.
func (s *SessionStore) Delete(id string) error {
	return s.execOne(`DELETE FROM sessions WHERE id = ?`, id)
}

// Touch updates the LastEventAt timestamp on a record.
func (s *SessionStore) Touch(id string) error {
	return s.execOne(`UPDATE sessions SET last_event_at = ? WHERE id = ?`,
		timeToStr(time.Now().UTC()), id)
}

// UpdateName updates the name/title of the session with the given id.
func (s *SessionStore) UpdateName(id, name string) error {
	return s.execOne(`UPDATE sessions SET name = ? WHERE id = ?`, name, id)
}

// UpdateTask sets the task soft-link on a session record.
func (s *SessionStore) UpdateTask(id, taskID string) error {
	return s.execOne(`UPDATE sessions SET task_id = ? WHERE id = ?`, taskID, id)
}

// UpdatePermissionMode persists the per-session permission policy. The
// bridge-server reads this on ensure_session (and on subsequent
// set_permission_mode actions from the client) to gate the permission
// prompt callback. Mode must be one of "approve-reads", "approve-all",
// "deny-all"; the caller is expected to validate.
func (s *SessionStore) UpdatePermissionMode(id, mode string) error {
	return s.execOne(`UPDATE sessions SET permission_mode = ? WHERE id = ?`, mode, id)
}

// UpdateACP persists the agent-managed session id for a chat record. Used
// when the bridge-server reports back the agent's session uuid via
// session_ready, so that subsequent opens can resume the same session
// (and find its native storage, e.g. Claude Code's <uuid>.jsonl).
// It also tries to resolve a descriptive session title from Claude's
// sessions index if the session currently has a default or empty name.
func (s *SessionStore) UpdateACP(id, acpSessionID string) error {
	if acpSessionID == "" {
		return nil
	}
	rec, ok, err := s.Get(id)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}

	newAcp := rec.AcpSessionID
	if newAcp == "" {
		newAcp = acpSessionID
	}
	newName := rec.Name
	if isDefaultSessionName(rec.Name) {
		if title, err := ResolveClaudeSessionName(acpSessionID); err == nil && title != "" {
			newName = title
		}
	}
	if newAcp == rec.AcpSessionID && newName == rec.Name {
		return nil
	}
	return s.execOne(`UPDATE sessions SET acp_session_id = ?, name = ? WHERE id = ?`,
		newAcp, newName, id)
}

func isDefaultSessionName(name string) bool {
	return name == "" || name == "聊天会话" || name == "新建会话" ||
		strings.HasPrefix(name, "Chat") || strings.HasSuffix(name, "会话")
}

// execOne runs a statement that must affect exactly one row, mapping zero
// rows to ErrNotFound.
func (s *SessionStore) execOne(query string, args ...any) error {
	res, err := s.db.sql.Exec(query, args...)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
