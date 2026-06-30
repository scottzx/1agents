// Package radio is the AI Radio application (#344-347) — the THIRD installable
// app, used as the extensibility acceptance gate: implementing it touches ZERO
// existing tables and ZERO task-main-flow code. Everything here is purely
// additive — a namespaced domain schema (radio_*), function handlers, completion
// writeback, and frontend views — all driven through the North Task API.
//
// 一个产物两种角色: an episode is both a domain object and the binding seam
// (business_ref = "radio:episode:<id>") for the 3-stage pipeline.
package radio

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

// AppID is the application namespace. This single value is the prefix for the
// business_ref ("radio:episode:7"), the domain tables ("radio_episode"), the
// task types ("radio.tts_synthesize") and the RegisterApp namespace.
const AppID = "radio"

// Episode statuses — the 3-stage pipeline lifecycle.
const (
	StatusDraft        = "draft"        // created, pipeline not started
	StatusSummarizing  = "summarizing"  // stage 1: agent summary running
	StatusTranscribing = "transcribing" // stage 2: agent transcript running
	StatusSynthesizing = "synthesizing" // stage 3: function TTS running
	StatusReady        = "ready"        // audio_path set, playable
)

// Episode is the radio_episode domain row. Audio bytes live on the FILE face
// (domainstore.ArtifactDir) — this row only carries audio_path.
type Episode struct {
	ID         int64     `json:"id"`
	Title      string    `json:"title"`
	SourceURL  string    `json:"sourceUrl"`
	Status     string    `json:"status"`
	Summary    string    `json:"summary"`
	Transcript string    `json:"transcript"`
	AudioPath  string    `json:"audioPath"` // relative path under the workspace
	Duration   int       `json:"duration"`  // seconds
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// schemaDDL is the idempotent domain DDL. All tables prefixed "radio_". Never
// bumps the global meta.schemaVersion — owned via appregistry.EnsureDomainTables.
var schemaDDL = []string{
	`CREATE TABLE IF NOT EXISTS radio_episode (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		title         TEXT    NOT NULL DEFAULT '',
		source_url    TEXT    NOT NULL DEFAULT '',
		status        TEXT    NOT NULL DEFAULT 'draft',
		summary       TEXT    NOT NULL DEFAULT '',
		transcript    TEXT    NOT NULL DEFAULT '',
		audio_path    TEXT    NOT NULL DEFAULT '',
		duration      INTEGER NOT NULL DEFAULT 0,
		workspace     TEXT    NOT NULL DEFAULT '',
		created_at    TEXT    NOT NULL DEFAULT '',
		updated_at    TEXT    NOT NULL DEFAULT ''
	)`,
}

// Store is the radio domain data-access layer over meta.db. It owns ONLY the
// radio_* tables; it never touches kernel tables.
type Store struct {
	db *meta.DB
}

// NewStore returns a Store over db.
func NewStore(db *meta.DB) *Store { return &Store{db: db} }

// EnsureTables creates the radio_* domain tables idempotently. Safe to call on
// every startup; does not bump the global schemaVersion (contract R4).
func (s *Store) EnsureTables() error {
	for _, ddl := range schemaDDL {
		if _, err := s.db.SQL().Exec(ddl); err != nil {
			return fmt.Errorf("radio: ensure tables: %w", err)
		}
	}
	return nil
}

// CreateEpisode inserts a draft episode and returns it with its assigned id.
func (s *Store) CreateEpisode(workspace, title, sourceURL string) (Episode, error) {
	now := time.Now().UTC()
	res, err := s.db.SQL().Exec(
		`INSERT INTO radio_episode (title, source_url, status, workspace, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		title, sourceURL, StatusDraft, workspace, now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if err != nil {
		return Episode{}, fmt.Errorf("radio: create episode: %w", err)
	}
	id, _ := res.LastInsertId()
	return Episode{
		ID: id, Title: title, SourceURL: sourceURL, Status: StatusDraft,
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

// GetEpisode returns one episode by id. ok=false when not found.
func (s *Store) GetEpisode(id int64) (Episode, bool, error) {
	row := s.db.SQL().QueryRow(
		`SELECT id, title, source_url, status, summary, transcript, audio_path, duration, created_at, updated_at
		 FROM radio_episode WHERE id = ?`, id)
	ep, err := scanEpisode(row)
	if err == sql.ErrNoRows {
		return Episode{}, false, nil
	}
	if err != nil {
		return Episode{}, false, err
	}
	return ep, true, nil
}

// GetWorkspace returns the workspace path stored for an episode (needed to
// resolve its artifact directory). ok=false when not found.
func (s *Store) GetWorkspace(id int64) (string, bool, error) {
	var ws string
	err := s.db.SQL().QueryRow(`SELECT workspace FROM radio_episode WHERE id = ?`, id).Scan(&ws)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return ws, true, nil
}

// ListEpisodes returns all episodes, newest first.
func (s *Store) ListEpisodes() ([]Episode, error) {
	rows, err := s.db.SQL().Query(
		`SELECT id, title, source_url, status, summary, transcript, audio_path, duration, created_at, updated_at
		 FROM radio_episode ORDER BY id DESC`)
	if err != nil {
		return nil, fmt.Errorf("radio: list episodes: %w", err)
	}
	defer rows.Close()
	var out []Episode
	for rows.Next() {
		ep, err := scanEpisode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ep)
	}
	return out, rows.Err()
}

// SetStatus updates only the status column.
func (s *Store) SetStatus(id int64, status string) error {
	_, err := s.db.SQL().Exec(
		`UPDATE radio_episode SET status = ?, updated_at = ? WHERE id = ?`,
		status, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

// SetSummary persists the stage-1 summary and advances status.
func (s *Store) SetSummary(id int64, summary string) error {
	_, err := s.db.SQL().Exec(
		`UPDATE radio_episode SET summary = ?, updated_at = ? WHERE id = ?`,
		summary, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

// SetTranscript persists the stage-2 transcript.
func (s *Store) SetTranscript(id int64, transcript string) error {
	_, err := s.db.SQL().Exec(
		`UPDATE radio_episode SET transcript = ?, updated_at = ? WHERE id = ?`,
		transcript, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

// SetAudio persists the stage-3 audio path + duration and marks the episode ready.
func (s *Store) SetAudio(id int64, audioPath string, duration int) error {
	_, err := s.db.SQL().Exec(
		`UPDATE radio_episode SET audio_path = ?, duration = ?, status = ?, updated_at = ? WHERE id = ?`,
		audioPath, duration, StatusReady, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

// scanner is satisfied by *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanEpisode(sc scanner) (Episode, error) {
	var ep Episode
	var createdAt, updatedAt string
	if err := sc.Scan(&ep.ID, &ep.Title, &ep.SourceURL, &ep.Status, &ep.Summary,
		&ep.Transcript, &ep.AudioPath, &ep.Duration, &createdAt, &updatedAt); err != nil {
		return Episode{}, err
	}
	ep.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	ep.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return ep, nil
}
