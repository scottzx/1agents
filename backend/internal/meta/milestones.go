package meta

import (
	"database/sql"
	"fmt"
	"time"
)

// Milestone CRUD lives on TaskStore because milestones and tasks share both the
// project resolution and the per-workspace write lock (a rename has to touch
// both the milestones row and every task's denormalized milestone label inside
// one transaction). Identity is (project_id, name); see the Milestone doc.

// ErrMilestoneExists is returned when creating/renaming would collide with an
// existing milestone name in the same project.
var ErrMilestoneExists = fmt.Errorf("meta: milestone name already exists")

const milestoneCols = `id, name, description, target_date, position, predecessor_id, created_at, updated_at`

func scanMilestone(r rowScanner) (Milestone, error) {
	var m Milestone
	var targetDate sql.NullString
	var createdAt, updatedAt string
	if err := r.Scan(&m.ID, &m.Name, &m.Description, &targetDate, &m.Position,
		&m.PredecessorID, &createdAt, &updatedAt); err != nil {
		return Milestone{}, err
	}
	m.TargetDate = valToTimePtr(targetDate)
	m.CreatedAt = strToTime(createdAt)
	m.UpdatedAt = strToTime(updatedAt)
	return m, nil
}

// ListMilestones returns the project's milestones in roadmap order (position,
// then creation), each enriched with live executable-task counts (total / completed).
// Issue items and cancelled tasks excluded (both numerator and denominator).
func (s *TaskStore) ListMilestones(workspacePath string) ([]Milestone, error) {
	projectID, err := s.db.projectIDByPath(workspacePath)
	if err != nil {
		return nil, err
	}
	if projectID == "" {
		return []Milestone{}, nil
	}
	// Self-heal: a task may reference a milestone name that has no metadata row
	// yet when it was inserted outside the HTTP layer (legacy tasks.json import,
	// the seed-roadmap command). Backfill those so the roadmap is always
	// complete. Writes only when an orphan actually exists, so the common poll
	// path stays read-only.
	if err := s.healOrphanMilestones(workspacePath, projectID); err != nil {
		return nil, err
	}
	rows, err := s.db.sql.Query(`
		SELECT `+milestoneCols+`,
			(SELECT COUNT(1) FROM tasks t
			   WHERE t.project_id = m.project_id AND t.milestone = m.name AND (t.type = 'task' OR t.type = '') AND t.status != 'cancelled') AS total,
			(SELECT COUNT(1) FROM tasks t
			   WHERE t.project_id = m.project_id AND t.milestone = m.name
			     AND (t.type = 'task' OR t.type = '')
			     AND t.status = 'completed') AS completed
		FROM milestones m
		WHERE m.project_id = ?
		ORDER BY m.position, m.created_at`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Milestone{}
	for rows.Next() {
		var m Milestone
		var targetDate sql.NullString
		var createdAt, updatedAt string
		if err := rows.Scan(&m.ID, &m.Name, &m.Description, &targetDate, &m.Position,
			&m.PredecessorID, &createdAt, &updatedAt, &m.Total, &m.Completed); err != nil {
			return nil, err
		}
		m.ProjectID = projectID
		m.TargetDate = valToTimePtr(targetDate)
		m.CreatedAt = strToTime(createdAt)
		m.UpdatedAt = strToTime(updatedAt)
		out = append(out, m)
	}
	return out, rows.Err()
}

// healOrphanMilestones creates metadata rows for milestone names referenced by
// tasks but missing from the milestones table. It queries first and only takes
// the write lock (via EnsureMilestone) when there is something to backfill.
func (s *TaskStore) healOrphanMilestones(workspacePath, projectID string) error {
	rows, err := s.db.sql.Query(`
		SELECT DISTINCT t.milestone FROM tasks t
		WHERE t.project_id = ? AND t.milestone != ''
		  AND NOT EXISTS (
			SELECT 1 FROM milestones m
			WHERE m.project_id = t.project_id AND m.name = t.milestone)`, projectID)
	if err != nil {
		return err
	}
	var orphans []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		orphans = append(orphans, name)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, name := range orphans {
		if err := s.EnsureMilestone(workspacePath, name); err != nil {
			return err
		}
	}
	return nil
}

// getMilestoneTx loads a single milestone (without counts) inside a tx.
func getMilestoneTx(tx *sql.Tx, projectID, id string) (Milestone, error) {
	row := tx.QueryRow(`SELECT `+milestoneCols+`
		FROM milestones WHERE project_id = ? AND id = ?`, projectID, id)
	m, err := scanMilestone(row)
	if err == sql.ErrNoRows {
		return Milestone{}, ErrNotFound
	}
	return m, err
}

// CreateMilestone inserts a new milestone at the end of the project's roadmap.
// Returns ErrMilestoneExists on a duplicate name.
func (s *TaskStore) CreateMilestone(workspacePath, name, description string, targetDate *time.Time, predecessorID string) (Milestone, error) {
	m := s.wsMutex(workspacePath)
	m.Lock()
	defer m.Unlock()

	tx, err := s.db.sql.Begin()
	if err != nil {
		return Milestone{}, err
	}
	defer tx.Rollback()

	projectID, err := ensureProjectByPathTx(tx, workspacePath)
	if err != nil {
		return Milestone{}, err
	}

	var dup int
	if err := tx.QueryRow(`SELECT COUNT(1) FROM milestones WHERE project_id = ? AND name = ?`,
		projectID, name).Scan(&dup); err != nil {
		return Milestone{}, err
	}
	if dup > 0 {
		return Milestone{}, ErrMilestoneExists
	}

	var nextPos int
	if err := tx.QueryRow(`SELECT COALESCE(MAX(position), -1) + 1 FROM milestones WHERE project_id = ?`,
		projectID).Scan(&nextPos); err != nil {
		return Milestone{}, err
	}

	now := time.Now().UTC()
	ms := Milestone{
		ID:            newID(),
		ProjectID:     projectID,
		Name:          name,
		Description:   description,
		TargetDate:    targetDate,
		Position:      nextPos,
		PredecessorID: predecessorID,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if _, err := tx.Exec(`
		INSERT INTO milestones (id, project_id, name, description, target_date, position, predecessor_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ms.ID, projectID, ms.Name, ms.Description, timePtrToVal(ms.TargetDate),
		ms.Position, ms.PredecessorID, timeToStr(ms.CreatedAt), timeToStr(ms.UpdatedAt)); err != nil {
		return Milestone{}, err
	}
	return ms, tx.Commit()
}

// MilestonePatch carries the optional fields of an UpdateMilestone call; a nil
// pointer leaves that field untouched.
type MilestonePatch struct {
	Name          *string
	Description   *string
	TargetDate    **time.Time // double pointer: outer nil = untouched, *nil = clear
	PredecessorID *string     // pointer: nil = untouched, "" = clear (make root)
}

// UpdateMilestone applies a partial edit. Renaming cascades to every task's
// milestone label so the name stays a valid join key. Returns ErrMilestoneExists
// if the new name collides.
func (s *TaskStore) UpdateMilestone(workspacePath, id string, patch MilestonePatch) (Milestone, error) {
	m := s.wsMutex(workspacePath)
	m.Lock()
	defer m.Unlock()

	tx, err := s.db.sql.Begin()
	if err != nil {
		return Milestone{}, err
	}
	defer tx.Rollback()

	projectID, err := ensureProjectByPathTx(tx, workspacePath)
	if err != nil {
		return Milestone{}, err
	}
	cur, err := getMilestoneTx(tx, projectID, id)
	if err != nil {
		return Milestone{}, err
	}

	if patch.Name != nil && *patch.Name != cur.Name {
		var dup int
		if err := tx.QueryRow(`SELECT COUNT(1) FROM milestones WHERE project_id = ? AND name = ? AND id != ?`,
			projectID, *patch.Name, id).Scan(&dup); err != nil {
			return Milestone{}, err
		}
		if dup > 0 {
			return Milestone{}, ErrMilestoneExists
		}
		// Cascade the rename to the denormalized label on tasks.
		if _, err := tx.Exec(`UPDATE tasks SET milestone = ?, updated_at = ?
			WHERE project_id = ? AND milestone = ?`,
			*patch.Name, timeToStr(time.Now().UTC()), projectID, cur.Name); err != nil {
			return Milestone{}, err
		}
		cur.Name = *patch.Name
	}
	if patch.Description != nil {
		cur.Description = *patch.Description
	}
	if patch.TargetDate != nil {
		cur.TargetDate = *patch.TargetDate
	}
	if patch.PredecessorID != nil {
		// Guard the obvious cycle: a milestone can't be its own predecessor.
		if *patch.PredecessorID == id {
			cur.PredecessorID = ""
		} else {
			cur.PredecessorID = *patch.PredecessorID
		}
	}
	cur.UpdatedAt = time.Now().UTC()

	if _, err := tx.Exec(`UPDATE milestones
		SET name = ?, description = ?, target_date = ?, predecessor_id = ?, updated_at = ?
		WHERE id = ?`,
		cur.Name, cur.Description, timePtrToVal(cur.TargetDate), cur.PredecessorID,
		timeToStr(cur.UpdatedAt), id); err != nil {
		return Milestone{}, err
	}
	return cur, tx.Commit()
}

// ReorderMilestones sets position by the index of each id in orderedIDs. Ids
// not belonging to the project are ignored; omitted milestones keep their old
// position (so a partial list still works, though the UI always sends the full
// order).
func (s *TaskStore) ReorderMilestones(workspacePath string, orderedIDs []string) error {
	m := s.wsMutex(workspacePath)
	m.Lock()
	defer m.Unlock()

	tx, err := s.db.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	projectID, err := ensureProjectByPathTx(tx, workspacePath)
	if err != nil {
		return err
	}
	now := timeToStr(time.Now().UTC())
	for i, id := range orderedIDs {
		if _, err := tx.Exec(`UPDATE milestones SET position = ?, updated_at = ?
			WHERE project_id = ? AND id = ?`, i, now, projectID, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteMilestone removes a milestone and unassigns its tasks (their milestone
// label is cleared, so they fall back into 未分组 rather than being deleted).
func (s *TaskStore) DeleteMilestone(workspacePath, id string) error {
	m := s.wsMutex(workspacePath)
	m.Lock()
	defer m.Unlock()

	tx, err := s.db.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	projectID, err := ensureProjectByPathTx(tx, workspacePath)
	if err != nil {
		return err
	}
	cur, err := getMilestoneTx(tx, projectID, id)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE tasks SET milestone = '', updated_at = ?
		WHERE project_id = ? AND milestone = ?`,
		timeToStr(time.Now().UTC()), projectID, cur.Name); err != nil {
		return err
	}
	// Reparent any children onto the deleted node's predecessor so the tree
	// stays connected (splice out, rather than orphaning whole branches).
	if _, err := tx.Exec(`UPDATE milestones SET predecessor_id = ?, updated_at = ?
		WHERE project_id = ? AND predecessor_id = ?`,
		cur.PredecessorID, timeToStr(time.Now().UTC()), projectID, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM milestones WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// EnsureMilestone lazily creates a metadata row for a milestone name that a
// task references but that has no row yet (e.g. a task created with a brand-new
// milestone label). Idempotent: a no-op when the name already exists or is
// empty. Keeps the roadmap's milestone list complete without forcing callers to
// create the milestone first.
func (s *TaskStore) EnsureMilestone(workspacePath, name string) error {
	if name == "" {
		return nil
	}
	m := s.wsMutex(workspacePath)
	m.Lock()
	defer m.Unlock()

	tx, err := s.db.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	projectID, err := ensureProjectByPathTx(tx, workspacePath)
	if err != nil {
		return err
	}
	var exists int
	if err := tx.QueryRow(`SELECT COUNT(1) FROM milestones WHERE project_id = ? AND name = ?`,
		projectID, name).Scan(&exists); err != nil {
		return err
	}
	if exists > 0 {
		return tx.Commit()
	}
	var nextPos int
	if err := tx.QueryRow(`SELECT COALESCE(MAX(position), -1) + 1 FROM milestones WHERE project_id = ?`,
		projectID).Scan(&nextPos); err != nil {
		return err
	}
	now := timeToStr(time.Now().UTC())
	if _, err := tx.Exec(`
		INSERT INTO milestones (id, project_id, name, description, target_date, position, created_at, updated_at)
		VALUES (?, ?, ?, '', NULL, ?, ?, ?)`,
		newID(), projectID, name, nextPos, now, now); err != nil {
		return err
	}
	return tx.Commit()
}
