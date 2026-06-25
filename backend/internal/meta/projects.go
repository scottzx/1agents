package meta

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"time"
)

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
	archive_reason, archive_note, archived_at, created_at, updated_at`

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
	var createdAt, updatedAt string
	if err := r.Scan(&p.ID, &p.Name, &p.WorkspacePath, &status,
		&reason, &note, &archivedAt, &createdAt, &updatedAt); err != nil {
		return Project{}, err
	}
	p.Status = ProjectStatus(status)
	p.ArchiveReason = ArchiveReason(reason)
	p.ArchiveNote = note
	p.ArchivedAt = valToTimePtr(archivedAt)
	p.CreatedAt = strToTime(createdAt)
	p.UpdatedAt = strToTime(updatedAt)
	return p, nil
}
