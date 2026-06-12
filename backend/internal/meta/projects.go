package meta

import (
	"database/sql"
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

// GetProject returns a project by id.
func (db *DB) GetProject(id string) (Project, bool, error) {
	row := db.sql.QueryRow(
		`SELECT id, name, workspace_path, status, created_at, updated_at
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

// ListProjects returns all projects, most recently updated first.
func (db *DB) ListProjects() ([]Project, error) {
	rows, err := db.sql.Query(
		`SELECT id, name, workspace_path, status, created_at, updated_at
		 FROM projects ORDER BY updated_at DESC`)
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

type rowScanner interface{ Scan(dest ...any) error }

func scanProject(r rowScanner) (Project, error) {
	var p Project
	var createdAt, updatedAt string
	if err := r.Scan(&p.ID, &p.Name, &p.WorkspacePath, &p.Status, &createdAt, &updatedAt); err != nil {
		return Project{}, err
	}
	p.CreatedAt = strToTime(createdAt)
	p.UpdatedAt = strToTime(updatedAt)
	return p, nil
}
