package meta

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"
)

// PersonalProjectID is the fixed id of the legacy "个人任务" reserved bucket, a
// pseudo-project that once backed the removed personal-tasks feature. It is kept
// only so any pre-existing bucket row stays excluded from the workspace registry
// (sidebar) and PMO dispatch targets — nothing creates it anymore.
const PersonalProjectID = "__personal__"

// EnsureProject upserts a project row keyed by id (= workspace id). Name and
// path are refreshed on every call so renames in the workspace registry
// propagate.
func (db *DB) EnsureProject(id, name, workspacePath string) error {
	now := timeToStr(time.Now().UTC())
	_, err := db.sql.Exec(`
		INSERT INTO projects (id, name, workspace_path, status, created_at, updated_at)
		VALUES (?, ?, ?, 'active', ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			workspace_path = excluded.workspace_path,
			updated_at = excluded.updated_at`,
		id, name, workspacePath, now, now)
	return err
}

// upsertWorkspaceProject inserts or refreshes a project row with the full
// workspace registry fields, writing position explicitly. status is set to
// 'active' on insert and left untouched on update (archiving must not be undone
// by a later refresh — mirrors EnsureProject). Used by the migration (which pins
// position to the legacy json order) and EnsureWorkspaceProject.
func (db *DB) upsertWorkspaceProject(p Project, position int) error {
	now := timeToStr(time.Now().UTC())
	builtin := 0
	if p.Builtin {
		builtin = 1
	}
	agents, _ := json.Marshal(p.AvailableAgents)
	kind := NormalizeProjectKind(p.Kind)
	_, err := db.sql.Exec(`
		INSERT INTO projects (id, name, workspace_path, status,
			terminal_dir, chat_channel, default_agent, builtin, position,
			available_agents, kind, avatar, created_at, updated_at)
		VALUES (?, ?, ?, 'active', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			workspace_path = excluded.workspace_path,
			terminal_dir = excluded.terminal_dir,
			chat_channel = excluded.chat_channel,
			default_agent = excluded.default_agent,
			builtin = excluded.builtin,
			position = excluded.position,
			available_agents = excluded.available_agents,
			kind = excluded.kind,
			avatar = excluded.avatar,
			updated_at = excluded.updated_at`,
		p.ID, p.Name, p.WorkspacePath,
		p.TerminalDir, p.ChatChannel, p.DefaultAgent, builtin, position,
		string(agents), kind, p.Avatar, now, now)
	return err
}

// EnsureWorkspaceProject upserts a workspace-backed project (the unified
// registry row the sidebar lists). New rows are appended (position = MAX+1);
// existing rows keep their current position. Write path for the workspace
// Create/Update handlers and 立项 (Incubate).
func (db *DB) EnsureWorkspaceProject(p Project) error {
	var position int
	err := db.sql.QueryRow(`SELECT position FROM projects WHERE id = ?`, p.ID).Scan(&position)
	if err == sql.ErrNoRows {
		if e := db.sql.QueryRow(
			`SELECT COALESCE(MAX(position), 0) + 1 FROM projects WHERE id != ?`,
			PersonalProjectID).Scan(&position); e != nil {
			return e
		}
	} else if err != nil {
		return err
	}
	return db.upsertWorkspaceProject(p, position)
}

// ListWorkspaceProjects returns every workspace-backed project (any status,
// excluding the reserved personal bucket) in sidebar order. Faithful replacement
// for reading workspaces_dir.json; callers needing only the active subset (the
// sidebar) filter by status themselves.
func (db *DB) ListWorkspaceProjects() ([]Project, error) {
	return db.queryProjects(
		`SELECT `+projectColumns+` FROM projects WHERE id != ?
		 ORDER BY position ASC, created_at ASC`, PersonalProjectID)
}

// PruneInvalidProjects removes project rows whose id is empty or whitespace.
// Such rows were minted before the CCProjectSlug/import fix, when a non-ASCII
// workspace name (e.g. the built-in "对话") slugged to "_" and sanitized to an
// empty id. An empty-id workspace makes the frontend request
// /api/agent/sessions?workspace_id= → 400, so self-heal on startup. Returns the
// number of rows removed.
func (db *DB) PruneInvalidProjects() (int64, error) {
	res, err := db.sql.Exec(`DELETE FROM projects WHERE TRIM(id) = ''`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// DeleteProject removes a project row by id. Tasks keyed by the gone project_id
// are left as-is (orphaned), matching pre-unification behavior where a deleted
// workspace dropped out of the registry but its meta rows remained.
func (db *DB) DeleteProject(id string) error {
	res, err := db.sql.Exec(`DELETE FROM projects WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return affectedOrNotFound(res)
}

// ReorderProjects rewrites the position column to match the given id order. The
// single-connection pool serializes this, so no explicit transaction is needed.
func (db *DB) ReorderProjects(ids []string) error {
	now := timeToStr(time.Now().UTC())
	for i, id := range ids {
		if _, err := db.sql.Exec(
			`UPDATE projects SET position = ?, updated_at = ? WHERE id = ?`,
			i, now, id); err != nil {
			return err
		}
	}
	return nil
}

// ensureProjectByPath returns the project id for a workspace path, creating
// a stub row (id = random, name = basename) when none exists. Used by the
// task store, which is keyed by path.
func (db *DB) ensureProjectByPath(workspacePath string) (string, error) {
	id, err := db.projectIDByPath(workspacePath)
	if err != nil {
		return "", err
	}
	if id != "" {
		return id, nil
	}
	id = newID()
	if err := db.EnsureProject(id, filepath.Base(workspacePath), workspacePath); err != nil {
		return "", err
	}
	return id, nil
}

func (db *DB) projectIDByPath(workspacePath string) (string, error) {
	var id string
	err := db.sql.QueryRow(
		`SELECT id FROM projects WHERE workspace_path = ? LIMIT 1`, workspacePath).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return id, nil
}

// projectColumns is the canonical SELECT column list, kept in sync with
// scanProject's Scan order.
const projectColumns = `id, name, workspace_path, status,
	archive_reason, archive_note, archived_at,
	terminal_dir, chat_channel, default_agent, builtin, position,
	COALESCE(available_agents, '[]'),
	COALESCE(kind, 'project'), COALESCE(avatar, ''),
	created_at, updated_at`

// GetProject returns a project by id.
func (db *DB) GetProject(id string) (Project, bool, error) {
	row := db.sql.QueryRow(
		`SELECT `+projectColumns+`
		 FROM projects WHERE id = ?`, id)
	p, err := scanProject(row)
	if err == sql.ErrNoRows {
		return Project{}, false, nil
	}
	if err != nil {
		return Project{}, false, err
	}
	return p, true, nil
}

// GetProjectByName returns a project by its display name. Names are not
// guaranteed unique (they default to the workspace directory basename), so on
// collision the most recently updated row wins; callers needing an exact match
// should resolve by id via GetProject. Returns ok=false when no project carries
// that name.
func (db *DB) GetProjectByName(name string) (Project, bool, error) {
	row := db.sql.QueryRow(
		`SELECT `+projectColumns+`
		 FROM projects WHERE name = ? ORDER BY updated_at DESC LIMIT 1`, name)
	p, err := scanProject(row)
	if err == sql.ErrNoRows {
		return Project{}, false, nil
	}
	if err != nil {
		return Project{}, false, err
	}
	return p, true, nil
}

// ListProjects returns all projects, most recently updated first.
func (db *DB) ListProjects() ([]Project, error) {
	return db.queryProjects(`SELECT ` + projectColumns + ` FROM projects ORDER BY updated_at DESC`)
}

// ListProjectsByStatus returns projects with the given status, most recently
// updated first. Used to split the active board from the archive view (#141).
func (db *DB) ListProjectsByStatus(status ProjectStatus) ([]Project, error) {
	return db.queryProjects(
		`SELECT `+projectColumns+` FROM projects WHERE status = ? ORDER BY updated_at DESC`,
		string(status))
}

func (db *DB) queryProjects(query string, args ...any) ([]Project, error) {
	rows, err := db.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Project{}
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ArchiveProject moves a project out of the active view (#141). status must be
// ProjectStatusArchived (阶段性完成归档) or ProjectStatusKilled (竞品出现砍掉);
// reason records why and note is an optional free-text rationale. Data is kept
// — only the status/reason/timestamp change. Returns ErrNotFound for an
// unknown id.
func (db *DB) ArchiveProject(id string, status ProjectStatus, reason ArchiveReason, note string) error {
	if status != ProjectStatusArchived && status != ProjectStatusKilled {
		return fmt.Errorf("meta: invalid archive status %q", status)
	}
	now := time.Now().UTC()
	res, err := db.sql.Exec(`
		UPDATE projects SET status = ?, archive_reason = ?, archive_note = ?,
			archived_at = ?, updated_at = ?
		WHERE id = ?`,
		string(status), string(reason), note, timeToStr(now), timeToStr(now), id)
	if err != nil {
		return err
	}
	return affectedOrNotFound(res)
}

// SetProjectStatus is the neutral status setter used by system-owned workspaces
// (ProjectStatusSystem). Unlike ArchiveProject it takes no ArchiveReason and
// leaves archive_note/archived_at alone — a "system" workspace isn't archived,
// it's platform-owned. Returns ErrNotFound for an unknown id.
func (db *DB) SetProjectStatus(id string, status ProjectStatus) error {
	res, err := db.sql.Exec(
		`UPDATE projects SET status = ?, updated_at = ? WHERE id = ?`,
		string(status), timeToStr(time.Now().UTC()), id)
	if err != nil {
		return err
	}
	return affectedOrNotFound(res)
}

// ReopenProject returns an archived/killed project to active, clearing the
// archive reason/note/timestamp (#141). Returns ErrNotFound for an unknown id.
func (db *DB) ReopenProject(id string) error {
	now := timeToStr(time.Now().UTC())
	res, err := db.sql.Exec(`
		UPDATE projects SET status = 'active', archive_reason = '', archive_note = '',
			archived_at = NULL, updated_at = ?
		WHERE id = ?`, now, id)
	if err != nil {
		return err
	}
	return affectedOrNotFound(res)
}

func affectedOrNotFound(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

type rowScanner interface{ Scan(dest ...any) error }

func scanProject(r rowScanner) (Project, error) {
	var p Project
	var status, reason, note string
	var archivedAt sql.NullString
	var builtin int
	var availableAgents string
	var createdAt, updatedAt string
	if err := r.Scan(&p.ID, &p.Name, &p.WorkspacePath, &status,
		&reason, &note, &archivedAt,
		&p.TerminalDir, &p.ChatChannel, &p.DefaultAgent, &builtin, &p.Position,
		&availableAgents,
		&p.Kind, &p.Avatar,
		&createdAt, &updatedAt); err != nil {
		return Project{}, err
	}
	p.Status = ProjectStatus(status)
	p.ArchiveReason = ArchiveReason(reason)
	p.ArchiveNote = note
	p.ArchivedAt = valToTimePtr(archivedAt)
	p.Builtin = builtin != 0
	p.CreatedAt = strToTime(createdAt)
	p.UpdatedAt = strToTime(updatedAt)
	if availableAgents != "" && availableAgents != "[]" {
		_ = json.Unmarshal([]byte(availableAgents), &p.AvailableAgents)
	}
	// Surface only the two canonical kinds on read paths (post-#189).
	p.Kind = NormalizeProjectKind(p.Kind)
	return p, nil
}
