package media

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/scottzx/1Agents/backend/internal/appregistry"
	"github.com/scottzx/1Agents/backend/internal/meta"
)

// AppID is the media app's namespace. It doubles as the domain-table prefix,
// the business_ref prefix, the taskTypes prefix and the RegisterApp namespace.
const AppID = "media"

// Domain DDLs — all tables prefixed "media_", all CREATE TABLE IF NOT EXISTS.
// Owned by this app; never bumps the global meta schemaVersion.
var domainDDLs = []string{
	`CREATE TABLE IF NOT EXISTS media_content_project (
		id            TEXT PRIMARY KEY,
		project_id    TEXT NOT NULL DEFAULT '',
		workspace     TEXT NOT NULL DEFAULT '',
		title         TEXT NOT NULL,
		status        TEXT NOT NULL DEFAULT 'topic',
		created_at    TEXT NOT NULL,
		updated_at    TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS media_material (
		id            TEXT PRIMARY KEY,
		project_id    TEXT NOT NULL,
		kind          TEXT NOT NULL DEFAULT 'video',
		file_path     TEXT NOT NULL DEFAULT '',
		duration      REAL NOT NULL DEFAULT 0,
		stage         TEXT NOT NULL DEFAULT 'raw',
		metadata      TEXT NOT NULL DEFAULT '{}',
		created_at    TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS media_segment (
		id            TEXT PRIMARY KEY,
		material_id   TEXT NOT NULL,
		start_sec     REAL NOT NULL DEFAULT 0,
		end_sec       REAL NOT NULL DEFAULT 0,
		label         TEXT NOT NULL DEFAULT '',
		decision      TEXT NOT NULL DEFAULT 'undecided',
		ordinal       INTEGER NOT NULL DEFAULT 0,
		created_at    TEXT NOT NULL
	)`,
}

// EnsureTables creates the media domain tables idempotently.
func EnsureTables() error {
	return appregistry.EnsureDomainTables(AppID, domainDDLs)
}

// ── domain types ────────────────────────────────────────────────────────────

// ContentProject is a 自媒体 content project aggregate (#335). status follows the
// pipeline: topic → material → edit → package → publish → review.
type ContentProject struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"` // the kernel projects-table id (= workspace project)
	Workspace string `json:"workspace"` // absolute workspace path
	Title     string `json:"title"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// Material is one素材 row (#336). Bytes live on the file face under the project
// workspace; only the path + metadata are stored here.
type Material struct {
	ID        string            `json:"id"`
	ProjectID string            `json:"projectId"`
	Kind      string            `json:"kind"` // video | audio | image
	FilePath  string            `json:"filePath"`
	Duration  float64           `json:"duration"`
	Stage     string            `json:"stage"` // raw | silence_detected | trimmed | edited | approved
	Metadata  map[string]string `json:"metadata"`
	CreatedAt string            `json:"createdAt"`
}

// Segment is a 段落 of a material with a keep/drop decision (#337/#338).
type Segment struct {
	ID         string  `json:"id"`
	MaterialID string  `json:"materialId"`
	Start      float64 `json:"start"`
	End        float64 `json:"end"`
	Label      string  `json:"label"`
	Decision   string  `json:"decision"` // undecided | keep | drop
	Ordinal    int     `json:"ordinal"`
	CreatedAt  string  `json:"createdAt"`
}

// ── ContentProject CRUD ─────────────────────────────────────────────────────

func db() (*sql.DB, error) {
	d, err := meta.OpenDefault()
	if err != nil {
		return nil, fmt.Errorf("media: open db: %w", err)
	}
	return d.SQL(), nil
}

func nowStr() string { return time.Now().UTC().Format(time.RFC3339) }

// CreateProject inserts a content project row and returns it.
func CreateProject(projectID, workspace, title string) (ContentProject, error) {
	sqlDB, err := db()
	if err != nil {
		return ContentProject{}, err
	}
	cp := ContentProject{
		ID:        meta.NewID(),
		ProjectID: projectID,
		Workspace: workspace,
		Title:     title,
		Status:    "topic",
		CreatedAt: nowStr(),
		UpdatedAt: nowStr(),
	}
	_, err = sqlDB.Exec(
		`INSERT INTO media_content_project (id, project_id, workspace, title, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		cp.ID, cp.ProjectID, cp.Workspace, cp.Title, cp.Status, cp.CreatedAt, cp.UpdatedAt)
	if err != nil {
		return ContentProject{}, fmt.Errorf("media: insert content project: %w", err)
	}
	return cp, nil
}

// GetProject returns a content project by id.
func GetProject(id string) (ContentProject, bool, error) {
	sqlDB, err := db()
	if err != nil {
		return ContentProject{}, false, err
	}
	row := sqlDB.QueryRow(
		`SELECT id, project_id, workspace, title, status, created_at, updated_at
		 FROM media_content_project WHERE id = ?`, id)
	var cp ContentProject
	err = row.Scan(&cp.ID, &cp.ProjectID, &cp.Workspace, &cp.Title, &cp.Status, &cp.CreatedAt, &cp.UpdatedAt)
	if err == sql.ErrNoRows {
		return ContentProject{}, false, nil
	}
	if err != nil {
		return ContentProject{}, false, err
	}
	return cp, true, nil
}

// ListProjects returns all content projects, optionally filtered by workspace.
func ListProjects(workspace string) ([]ContentProject, error) {
	sqlDB, err := db()
	if err != nil {
		return nil, err
	}
	var rows *sql.Rows
	if workspace != "" {
		rows, err = sqlDB.Query(
			`SELECT id, project_id, workspace, title, status, created_at, updated_at
			 FROM media_content_project WHERE workspace = ? ORDER BY created_at DESC`, workspace)
	} else {
		rows, err = sqlDB.Query(
			`SELECT id, project_id, workspace, title, status, created_at, updated_at
			 FROM media_content_project ORDER BY created_at DESC`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ContentProject{}
	for rows.Next() {
		var cp ContentProject
		if err := rows.Scan(&cp.ID, &cp.ProjectID, &cp.Workspace, &cp.Title, &cp.Status, &cp.CreatedAt, &cp.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, cp)
	}
	return out, rows.Err()
}

// SetProjectStatus advances a content project's pipeline stage.
func SetProjectStatus(id, status string) error {
	sqlDB, err := db()
	if err != nil {
		return err
	}
	_, err = sqlDB.Exec(
		`UPDATE media_content_project SET status = ?, updated_at = ? WHERE id = ?`,
		status, nowStr(), id)
	return err
}

// ── Material CRUD ───────────────────────────────────────────────────────────

// AddMaterial inserts a material row (the bytes are already landed on disk; this
// stores only path + metadata).
func AddMaterial(m Material) (Material, error) {
	sqlDB, err := db()
	if err != nil {
		return Material{}, err
	}
	if m.ID == "" {
		m.ID = meta.NewID()
	}
	if m.Stage == "" {
		m.Stage = "raw"
	}
	if m.Kind == "" {
		m.Kind = "video"
	}
	if m.Metadata == nil {
		m.Metadata = map[string]string{}
	}
	m.CreatedAt = nowStr()
	metaJSON, _ := json.Marshal(m.Metadata)
	_, err = sqlDB.Exec(
		`INSERT INTO media_material (id, project_id, kind, file_path, duration, stage, metadata, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.ProjectID, m.Kind, m.FilePath, m.Duration, m.Stage, string(metaJSON), m.CreatedAt)
	if err != nil {
		return Material{}, fmt.Errorf("media: insert material: %w", err)
	}
	return m, nil
}

// ListMaterials returns all materials for a content project.
func ListMaterials(projectID string) ([]Material, error) {
	sqlDB, err := db()
	if err != nil {
		return nil, err
	}
	rows, err := sqlDB.Query(
		`SELECT id, project_id, kind, file_path, duration, stage, metadata, created_at
		 FROM media_material WHERE project_id = ? ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Material{}
	for rows.Next() {
		var m Material
		var metaJSON string
		if err := rows.Scan(&m.ID, &m.ProjectID, &m.Kind, &m.FilePath, &m.Duration, &m.Stage, &metaJSON, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.Metadata = map[string]string{}
		_ = json.Unmarshal([]byte(metaJSON), &m.Metadata)
		out = append(out, m)
	}
	return out, rows.Err()
}

// GetMaterial returns one material by id.
func GetMaterial(id string) (Material, bool, error) {
	sqlDB, err := db()
	if err != nil {
		return Material{}, false, err
	}
	row := sqlDB.QueryRow(
		`SELECT id, project_id, kind, file_path, duration, stage, metadata, created_at
		 FROM media_material WHERE id = ?`, id)
	var m Material
	var metaJSON string
	err = row.Scan(&m.ID, &m.ProjectID, &m.Kind, &m.FilePath, &m.Duration, &m.Stage, &metaJSON, &m.CreatedAt)
	if err == sql.ErrNoRows {
		return Material{}, false, nil
	}
	if err != nil {
		return Material{}, false, err
	}
	m.Metadata = map[string]string{}
	_ = json.Unmarshal([]byte(metaJSON), &m.Metadata)
	return m, true, nil
}

// SetMaterialStage updates a material's pipeline stage.
func SetMaterialStage(id, stage string) error {
	sqlDB, err := db()
	if err != nil {
		return err
	}
	_, err = sqlDB.Exec(`UPDATE media_material SET stage = ? WHERE id = ?`, stage, id)
	return err
}

// ── Segment CRUD ────────────────────────────────────────────────────────────

// ReplaceSegments deletes existing segments for a material and inserts the given
// set (used when a function/agent stage produces fresh segment candidates).
func ReplaceSegments(materialID string, segs []Segment) error {
	sqlDB, err := db()
	if err != nil {
		return err
	}
	tx, err := sqlDB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM media_segment WHERE material_id = ?`, materialID); err != nil {
		return err
	}
	for i, s := range segs {
		id := s.ID
		if id == "" {
			id = meta.NewID()
		}
		decision := s.Decision
		if decision == "" {
			decision = "undecided"
		}
		if _, err := tx.Exec(
			`INSERT INTO media_segment (id, material_id, start_sec, end_sec, label, decision, ordinal, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			id, materialID, s.Start, s.End, s.Label, decision, i, nowStr()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListSegments returns the segments of a material ordered by ordinal.
func ListSegments(materialID string) ([]Segment, error) {
	sqlDB, err := db()
	if err != nil {
		return nil, err
	}
	rows, err := sqlDB.Query(
		`SELECT id, material_id, start_sec, end_sec, label, decision, ordinal, created_at
		 FROM media_segment WHERE material_id = ? ORDER BY ordinal ASC`, materialID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Segment{}
	for rows.Next() {
		var s Segment
		if err := rows.Scan(&s.ID, &s.MaterialID, &s.Start, &s.End, &s.Label, &s.Decision, &s.Ordinal, &s.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SetSegmentDecision records a keep/drop decision for one segment.
func SetSegmentDecision(id, decision string) error {
	sqlDB, err := db()
	if err != nil {
		return err
	}
	_, err = sqlDB.Exec(`UPDATE media_segment SET decision = ? WHERE id = ?`, decision, id)
	return err
}
