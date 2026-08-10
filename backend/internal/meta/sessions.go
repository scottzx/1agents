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
	cc_session_id, acp_session_id, session_key, permission_mode, role,
	created_at, last_event_at, archived_at, user_named, cwd, profile_id, profile_revision`

func scanSession(r rowScanner) (ChatSessionRecord, error) {
	var rec ChatSessionRecord
	var createdAt, lastEventAt, archivedAt string
	var userNamed int
	if err := r.Scan(&rec.ID, &rec.WorkspaceID, &rec.TaskID, &rec.Name, &rec.AgentType,
		&rec.CcProject, &rec.CcSessionID, &rec.AcpSessionID, &rec.SessionKey,
		&rec.PermissionMode, &rec.Role, &createdAt, &lastEventAt, &archivedAt, &userNamed,
		&rec.Cwd, &rec.ProfileID, &rec.ProfileRevision); err != nil {
		return ChatSessionRecord{}, err
	}
	rec.CreatedAt = strToTime(createdAt)
	rec.LastEventAt = strToTime(lastEventAt)
	rec.ArchivedAt = strToTime(archivedAt)
	rec.UserNamed = userNamed != 0
	return rec, nil
}

// ListByWorkspace returns chat sessions belonging to a workspace, sorted
// newest-first by last assistant-text activity (last_event_at), falling
// back to created_at when last_event_at is empty. Archived sessions are
// excluded unless includeArchived is set (the 会话 archive view passes true).
func (s *SessionStore) ListByWorkspace(workspaceID string, includeArchived bool) ([]ChatSessionRecord, error) {
	query := `SELECT ` + sessionCols + ` FROM sessions WHERE project_id = ?`
	if !includeArchived {
		query += ` AND archived_at = ''`
	}
	// last_event_at is bumped when the assistant emits a text block (see
	// acpx_client text_delta intercept). Empty last_event_at (legacy rows)
	// falls back to created_at so brand-new / never-replied sessions still
	// sort sanely among themselves.
	query += ` ORDER BY CASE WHEN last_event_at = '' THEN created_at ELSE last_event_at END DESC`
	rows, err := s.db.sql.Query(query, workspaceID)
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
	// Seed last_event_at so a freshly created session sorts to the top of
	// the sidebar (newest-first by assistant activity) before any reply.
	if rec.LastEventAt.IsZero() {
		rec.LastEventAt = rec.CreatedAt
	}
	res, err := s.db.sql.Exec(`
		INSERT INTO sessions (id, project_id, task_id, name, agent_type, cc_project,
			cc_session_id, acp_session_id, session_key, permission_mode, role,
			created_at, last_event_at, archived_at, user_named, cwd, profile_id, profile_revision)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO NOTHING`,
		rec.ID, rec.WorkspaceID, rec.TaskID, rec.Name, rec.AgentType, rec.CcProject,
		rec.CcSessionID, rec.AcpSessionID, rec.SessionKey, rec.PermissionMode, rec.Role,
		timeToStr(rec.CreatedAt), timeToStr(rec.LastEventAt), timeToStr(rec.ArchivedAt),
		boolToInt(rec.UserNamed), rec.Cwd, rec.ProfileID, rec.ProfileRevision)
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

// SetArchived soft-deletes (archived=true) or restores (archived=false) a
// session by stamping/clearing archived_at. Returns ErrNotFound if no match.
func (s *SessionStore) SetArchived(id string, archived bool) error {
	var at time.Time
	if archived {
		at = time.Now().UTC()
	}
	return s.execOne(`UPDATE sessions SET archived_at = ? WHERE id = ?`, timeToStr(at), id)
}

// Touch updates the LastEventAt timestamp on a record.
func (s *SessionStore) Touch(id string) error {
	return s.execOne(`UPDATE sessions SET last_event_at = ? WHERE id = ?`,
		timeToStr(time.Now().UTC()), id)
}

// UpdateName updates the name/title of the session with the given id. The
// user_named flag is set to 1 so subsequent list/get calls do not overwrite
// the user's chosen title with an AI-resolved default (#94).
func (s *SessionStore) UpdateName(id, name string) error {
	return s.execOne(`UPDATE sessions SET name = ?, user_named = 1 WHERE id = ?`, name, id)
}

// ClearUserNamed resets the user_named flag for a session, allowing the next
// list/get to overwrite the title with an AI-resolved default. Not currently
// exposed via HTTP (no reset-to-AI-title endpoint yet); reserved for future
// use.
func (s *SessionStore) ClearUserNamed(id string) error {
	return s.execOne(`UPDATE sessions SET user_named = 0 WHERE id = ?`, id)
}

// UpdateTask sets the task soft-link on a session record.
func (s *SessionStore) UpdateTask(id, taskID string) error {
	return s.execOne(`UPDATE sessions SET task_id = ? WHERE id = ?`, taskID, id)
}

// UpdatePermissionMode persists the per-session permission policy. The
// bridge-server reads this on ensure_session (and on subsequent
// set_permission_mode actions from the client) to gate the permission
// prompt callback. Mode must be one of "approve-reads", "approve-all",
// "deny-all", "auto"; the caller is expected to validate.
func (s *SessionStore) UpdatePermissionMode(id, mode string) error {
	return s.execOne(`UPDATE sessions SET permission_mode = ? WHERE id = ?`, mode, id)
}

// UpdateProfile records the concrete profile revision currently attached to a
// chat session. It never stores provider credentials.
func (s *SessionStore) UpdateProfile(id, profileID string, revision int) error {
	return s.execOne(`UPDATE sessions SET profile_id = ?, profile_revision = ? WHERE id = ?`,
		profileID, revision, id)
}

// UpdateACP persists the agent-managed session id for a chat record. Used
// when the bridge-server reports back the agent's session uuid via
// session_ready, so that subsequent opens can resume the same session
// (and find its native storage, e.g. Claude Code's <uuid>.jsonl or
// Grok's ~/.grok/sessions/.../summary.json).
// It also tries to resolve a descriptive session title from Claude's
// sessions index, then Grok's generated_title, when the session still has
// a default or empty name.
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
	if !rec.UserNamed && isDefaultSessionName(rec.Name) {
		if title, err := ResolveClaudeSessionName(acpSessionID); err == nil && title != "" {
			newName = title
		} else if title, err := ResolveGrokSessionTitle("", acpSessionID); err == nil && title != "" {
			// Walk-only: UpdateACP has no workspace path; Grok layout is
			// ~/.grok/sessions/<encoded-cwd>/<id>/summary.json.
			newName = title
		}
	}
	if newAcp == rec.AcpSessionID && newName == rec.Name {
		return nil
	}
	return s.execOne(`UPDATE sessions SET acp_session_id = ?, name = ? WHERE id = ?`,
		newAcp, newName, id)
}

// DeleteACP clears the acp_session_id and sets exec_status = 'closed' for the session with the given ID.
func (s *SessionStore) DeleteACP(id string) error {
	return s.execOne(`UPDATE sessions SET acp_session_id = '', exec_status = 'closed' WHERE id = ?`, id)
}

// UpdateAuthStatus is a stub method since auth status is only kept in memory on backend.
func (s *SessionStore) UpdateAuthStatus(id string, status string) error {
	return nil
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
