package roundtable

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

// Store persists rooms, seats, and turns in meta.db (design §5.3: 刷新可恢复).
type Store struct {
	db *meta.DB
}

// ErrBriefVersionConflict is returned when an optimistic Brief write was
// based on a version that is no longer current.
var ErrBriefVersionConflict = errors.New("roundtable: stale brief version")

// BriefVersionConflictError carries the versions needed by clients to reload.
type BriefVersionConflictError struct {
	Expected int
	Current  int
}

func (e *BriefVersionConflictError) Error() string {
	return fmt.Sprintf("%s: expected=%d current=%d", ErrBriefVersionConflict, e.Expected, e.Current)
}

func (e *BriefVersionConflictError) Unwrap() error {
	return ErrBriefVersionConflict
}

// NewStore opens (or reuses) the default meta.db and ensures domain tables.
func NewStore() (*Store, error) {
	db, err := meta.OpenDefault()
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.ensureSchema(); err != nil {
		return nil, err
	}
	return s, nil
}

// NewStoreWithDB is for tests that inject an isolated meta.DB.
func NewStoreWithDB(db *meta.DB) (*Store, error) {
	if db == nil {
		return nil, fmt.Errorf("roundtable: nil db")
	}
	s := &Store{db: db}
	if err := s.ensureSchema(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) ensureSchema() error {
	sqlDB := s.db.SQL()
	ddls := []string{
		`CREATE TABLE IF NOT EXISTS agents_roundtable_rooms (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL DEFAULT '',
			state TEXT NOT NULL,
			brief_json TEXT,
			current_brief_version INTEGER NOT NULL DEFAULT 0,
			confirmed_brief_version INTEGER NOT NULL DEFAULT 0,
			r2_brief_version INTEGER NOT NULL DEFAULT 0,
			summary_r2 TEXT NOT NULL DEFAULT '',
			summary_r3 TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS agents_roundtable_brief_versions (
			room_id TEXT NOT NULL,
			version INTEGER NOT NULL,
			status TEXT NOT NULL,
			content_json TEXT NOT NULL,
			proposed_by TEXT NOT NULL,
			source_turn_id TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			confirmed_at TEXT,
			PRIMARY KEY(room_id, version)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_agents_roundtable_brief_versions_room
			ON agents_roundtable_brief_versions(room_id, version DESC)`,
		`CREATE TABLE IF NOT EXISTS agents_roundtable_seats (
			id TEXT PRIMARY KEY,
			room_id TEXT NOT NULL,
			role TEXT NOT NULL,
			agent_type TEXT NOT NULL,
			workspace_id TEXT NOT NULL,
			session_id TEXT NOT NULL DEFAULT '',
			acp_session_id TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'ready',
			created_at TEXT NOT NULL,
			UNIQUE(room_id, role)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_agents_roundtable_seats_room
			ON agents_roundtable_seats(room_id)`,
		`CREATE TABLE IF NOT EXISTS agents_roundtable_turns (
			id TEXT PRIMARY KEY,
			room_id TEXT NOT NULL,
			round INTEGER NOT NULL,
			seat_id TEXT NOT NULL DEFAULT '',
			kind TEXT NOT NULL,
			content_text TEXT NOT NULL DEFAULT '',
			process_ref TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_agents_roundtable_turns_room
			ON agents_roundtable_turns(room_id, created_at)`,
	}
	for _, ddl := range ddls {
		if _, err := sqlDB.Exec(ddl); err != nil {
			return fmt.Errorf("roundtable schema: %w", err)
		}
	}
	// Slice-2 migration: older DBs may lack session_id on seats.
	if !columnExists(sqlDB, "agents_roundtable_seats", "session_id") {
		if _, err := sqlDB.Exec(
			`ALTER TABLE agents_roundtable_seats ADD COLUMN session_id TEXT NOT NULL DEFAULT ''`,
		); err != nil {
			return fmt.Errorf("roundtable migrate session_id: %w", err)
		}
	}
	for _, col := range []string{"current_brief_version", "confirmed_brief_version", "r2_brief_version"} {
		if columnExists(sqlDB, "agents_roundtable_rooms", col) {
			continue
		}
		if _, err := sqlDB.Exec(fmt.Sprintf(
			`ALTER TABLE agents_roundtable_rooms ADD COLUMN %s INTEGER NOT NULL DEFAULT 0`, col,
		)); err != nil {
			return fmt.Errorf("roundtable migrate %s: %w", col, err)
		}
	}
	return s.migrateLegacyBriefs()
}

// migrateLegacyBriefs promotes old rooms.brief_json values to a confirmed v1
// snapshot. brief_json remains as a compatibility projection for old readers.
func (s *Store) migrateLegacyBriefs() error {
	sqlDB := s.db.SQL()
	tx, err := sqlDB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.Query(
		`SELECT id, brief_json, created_at, updated_at
		 FROM agents_roundtable_rooms
		 WHERE current_brief_version = 0
		   AND brief_json IS NOT NULL
		   AND trim(brief_json) <> ''`)
	if err != nil {
		return fmt.Errorf("roundtable query legacy briefs: %w", err)
	}
	type legacyBrief struct {
		roomID, contentJSON, createdAt, updatedAt string
	}
	var legacy []legacyBrief
	for rows.Next() {
		var item legacyBrief
		if err := rows.Scan(&item.roomID, &item.contentJSON, &item.createdAt, &item.updatedAt); err != nil {
			_ = rows.Close()
			return err
		}
		var content Brief
		if err := json.Unmarshal([]byte(item.contentJSON), &content); err != nil {
			_ = rows.Close()
			return fmt.Errorf("roundtable parse legacy brief room=%s: %w", item.roomID, err)
		}
		legacy = append(legacy, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range legacy {
		_, err := tx.Exec(
			`INSERT OR IGNORE INTO agents_roundtable_brief_versions
				(room_id, version, status, content_json, proposed_by, source_turn_id,
				 created_at, updated_at, confirmed_at)
			 VALUES (?, 1, ?, ?, ?, '', ?, ?, ?)`,
			item.roomID, string(BriefStatusConfirmed), item.contentJSON,
			string(BriefProposerUser), item.createdAt, item.updatedAt, item.updatedAt,
		)
		if err != nil {
			return fmt.Errorf("roundtable migrate legacy brief room=%s: %w", item.roomID, err)
		}
		if _, err := tx.Exec(
			`UPDATE agents_roundtable_rooms
			 SET current_brief_version = 1, confirmed_brief_version = 1
			 WHERE id = ? AND current_brief_version = 0`,
			item.roomID,
		); err != nil {
			return fmt.Errorf("roundtable point legacy brief room=%s: %w", item.roomID, err)
		}
	}
	return tx.Commit()
}

func columnExists(db *sql.DB, table, col string) bool {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false
		}
		if name == col {
			return true
		}
	}
	return false
}

// InsertRoom writes a new room and its seats in one transaction.
func (s *Store) InsertRoom(room *Room) error {
	if room == nil || room.ID == "" {
		return fmt.Errorf("roundtable: room id required")
	}
	sqlDB := s.db.SQL()
	tx, err := sqlDB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	briefJSON := ""
	if room.Brief != nil {
		b, err := json.Marshal(room.Brief)
		if err != nil {
			return err
		}
		briefJSON = string(b)
	}
	_, err = tx.Exec(
		`INSERT INTO agents_roundtable_rooms
			(id, title, state, brief_json, current_brief_version,
			 confirmed_brief_version, r2_brief_version,
			 summary_r2, summary_r3, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		room.ID, room.Title, string(room.State), nullIfEmpty(briefJSON),
		room.CurrentBriefVersion, room.ConfirmedBriefVersion, room.R2BriefVersion,
		room.SummaryR2, room.SummaryR3,
		timeToStr(room.CreatedAt), timeToStr(room.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("insert room: %w", err)
	}
	for _, seat := range room.Seats {
		if err := insertSeatTx(tx, &seat); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func insertSeatTx(tx *sql.Tx, seat *Seat) error {
	_, err := tx.Exec(
		`INSERT INTO agents_roundtable_seats
			(id, room_id, role, agent_type, workspace_id, session_id, acp_session_id, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		seat.ID, seat.RoomID, string(seat.Role), seat.AgentType, seat.WorkspaceID,
		seat.SessionID, seat.AcpSessionID, string(seat.Status), timeToStr(seat.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("insert seat %s: %w", seat.Role, err)
	}
	return nil
}

// ListRooms returns rooms newest-first (updated_at DESC). No seats/turns.
// limit ≤ 0 defaults to 100; hard cap 500.
func (s *Store) ListRooms(limit int) ([]Room, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := s.db.SQL().Query(
		`SELECT id, title, state, brief_json, current_brief_version,
		        confirmed_brief_version, r2_brief_version,
		        summary_r2, summary_r3, created_at, updated_at
		 FROM agents_roundtable_rooms
		 ORDER BY updated_at DESC
		 LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Room
	for rows.Next() {
		var room Room
		var state, createdAt, updatedAt string
		var briefNS sql.NullString
		if err := rows.Scan(
			&room.ID, &room.Title, &state, &briefNS,
			&room.CurrentBriefVersion, &room.ConfirmedBriefVersion, &room.R2BriefVersion,
			&room.SummaryR2, &room.SummaryR3, &createdAt, &updatedAt,
		); err != nil {
			return nil, err
		}
		room.State = RoomState(state)
		room.CreatedAt = strToTime(createdAt)
		room.UpdatedAt = strToTime(updatedAt)
		if briefNS.Valid && strings.TrimSpace(briefNS.String) != "" {
			var b Brief
			if err := json.Unmarshal([]byte(briefNS.String), &b); err != nil {
				return nil, fmt.Errorf("parse brief: %w", err)
			}
			room.Brief = &b
		}
		if err := s.hydrateRoomBriefVersions(&room); err != nil {
			return nil, err
		}
		out = append(out, room)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []Room{}
	}
	return out, nil
}

// GetRoom loads a room by id. Seats/turns are not included; use ListSeats/ListTurns.
func (s *Store) GetRoom(id string) (*Room, error) {
	row := s.db.SQL().QueryRow(
		`SELECT id, title, state, brief_json, current_brief_version,
		        confirmed_brief_version, r2_brief_version,
		        summary_r2, summary_r3, created_at, updated_at
		 FROM agents_roundtable_rooms WHERE id = ?`, id)
	var room Room
	var state, briefJSON, createdAt, updatedAt string
	var briefNS sql.NullString
	err := row.Scan(
		&room.ID, &room.Title, &state, &briefNS,
		&room.CurrentBriefVersion, &room.ConfirmedBriefVersion, &room.R2BriefVersion,
		&room.SummaryR2, &room.SummaryR3, &createdAt, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, meta.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	room.State = RoomState(state)
	room.CreatedAt = strToTime(createdAt)
	room.UpdatedAt = strToTime(updatedAt)
	if briefNS.Valid {
		briefJSON = briefNS.String
	}
	if strings.TrimSpace(briefJSON) != "" {
		var b Brief
		if err := json.Unmarshal([]byte(briefJSON), &b); err != nil {
			return nil, fmt.Errorf("parse brief: %w", err)
		}
		room.Brief = &b
	}
	if err := s.hydrateRoomBriefVersions(&room); err != nil {
		return nil, err
	}
	return &room, nil
}

func (s *Store) hydrateRoomBriefVersions(room *Room) error {
	if room == nil {
		return nil
	}
	var err error
	if room.CurrentBriefVersion > 0 {
		room.CurrentBrief, err = s.GetBriefVersion(room.ID, room.CurrentBriefVersion)
		if err != nil {
			return fmt.Errorf("load current brief v%d: %w", room.CurrentBriefVersion, err)
		}
		// The legacy field follows current content so existing clients keep
		// seeing edits/proposals without becoming an R2 source of truth.
		content := room.CurrentBrief.Content
		room.Brief = &content
	}
	if room.ConfirmedBriefVersion > 0 {
		room.ConfirmedBrief, err = s.GetBriefVersion(room.ID, room.ConfirmedBriefVersion)
		if err != nil {
			return fmt.Errorf("load confirmed brief v%d: %w", room.ConfirmedBriefVersion, err)
		}
	}
	if room.R2BriefVersion > 0 {
		room.R2Brief, err = s.GetBriefVersion(room.ID, room.R2BriefVersion)
		if err != nil {
			return fmt.Errorf("load r2 brief v%d: %w", room.R2BriefVersion, err)
		}
	}
	return nil
}

// GetBriefVersion loads one immutable Brief content snapshot.
func (s *Store) GetBriefVersion(roomID string, version int) (*BriefVersion, error) {
	row := s.db.SQL().QueryRow(
		`SELECT room_id, version, status, content_json, proposed_by,
		        source_turn_id, created_at, updated_at, confirmed_at
		 FROM agents_roundtable_brief_versions
		 WHERE room_id = ? AND version = ?`,
		roomID, version,
	)
	return scanBriefVersion(row)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanBriefVersion(row rowScanner) (*BriefVersion, error) {
	var version BriefVersion
	var status, proposedBy, contentJSON, createdAt, updatedAt string
	var confirmedAt sql.NullString
	if err := row.Scan(
		&version.RoomID, &version.Version, &status, &contentJSON, &proposedBy,
		&version.SourceTurnID, &createdAt, &updatedAt, &confirmedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, meta.ErrNotFound
		}
		return nil, err
	}
	version.Status = BriefStatus(status)
	version.ProposedBy = BriefProposer(proposedBy)
	version.CreatedAt = strToTime(createdAt)
	version.UpdatedAt = strToTime(updatedAt)
	if err := json.Unmarshal([]byte(contentJSON), &version.Content); err != nil {
		return nil, fmt.Errorf("parse brief version content: %w", err)
	}
	if confirmedAt.Valid && strings.TrimSpace(confirmedAt.String) != "" {
		t := strToTime(confirmedAt.String)
		version.ConfirmedAt = &t
	}
	return &version, nil
}

// ListBriefVersions returns all versions in ascending order.
func (s *Store) ListBriefVersions(roomID string) ([]BriefVersion, error) {
	rows, err := s.db.SQL().Query(
		`SELECT room_id, version, status, content_json, proposed_by,
		        source_turn_id, created_at, updated_at, confirmed_at
		 FROM agents_roundtable_brief_versions
		 WHERE room_id = ?
		 ORDER BY version ASC`,
		roomID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BriefVersion{}
	for rows.Next() {
		version, err := scanBriefVersion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *version)
	}
	return out, rows.Err()
}

// CreateBriefVersion atomically appends a draft/proposal when expectedVersion
// is still the room's current version. Content is immutable after insertion.
func (s *Store) CreateBriefVersion(
	roomID string,
	expectedVersion int,
	status BriefStatus,
	content Brief,
	proposedBy BriefProposer,
	sourceTurnID string,
) (*BriefVersion, error) {
	if status != BriefStatusDraft && status != BriefStatusProposed {
		return nil, fmt.Errorf("roundtable: new brief status must be draft|proposed")
	}
	if proposedBy != BriefProposerUser && proposedBy != BriefProposerReferee {
		return nil, fmt.Errorf("roundtable: proposed_by must be user|referee")
	}
	contentJSON, err := json.Marshal(content)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	sqlDB := s.db.SQL()
	tx, err := sqlDB.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	newVersion := expectedVersion + 1
	res, err := tx.Exec(
		`UPDATE agents_roundtable_rooms
		 SET brief_json = ?, current_brief_version = ?, state = ?, updated_at = ?
		 WHERE id = ?
		   AND current_brief_version = ?
		   AND state NOT IN (?, ?)
		   AND NOT (
		     state = ?
		     AND r2_brief_version > 0
		     AND r2_brief_version = confirmed_brief_version
		   )`,
		string(contentJSON), newVersion, string(StateDraftingBrief), timeToStr(now),
		roomID, expectedVersion,
		string(StateSummarizingR2), string(StateSummarizingR3), string(StateWaitingR2),
	)
	if err != nil {
		return nil, err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		var currentVersion, confirmedVersion, r2Version int
		var state string
		err := tx.QueryRow(
			`SELECT state, current_brief_version, confirmed_brief_version, r2_brief_version
			 FROM agents_roundtable_rooms WHERE id = ?`,
			roomID,
		).Scan(&state, &currentVersion, &confirmedVersion, &r2Version)
		if err == sql.ErrNoRows {
			return nil, meta.ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		if currentVersion != expectedVersion {
			return nil, &BriefVersionConflictError{Expected: expectedVersion, Current: currentVersion}
		}
		if RoomState(state) == StateWaitingR2 && r2Version > 0 && r2Version == confirmedVersion {
			return nil, fmt.Errorf("roundtable: brief cannot change while r2 is running")
		}
		return nil, fmt.Errorf("roundtable: brief cannot change while a round is running (state=%s)", state)
	}
	if expectedVersion > 0 {
		if _, err := tx.Exec(
			`UPDATE agents_roundtable_brief_versions
			 SET status = ?, updated_at = ?
			 WHERE room_id = ? AND version = ? AND status IN (?, ?)`,
			string(BriefStatusSuperseded), timeToStr(now), roomID, expectedVersion,
			string(BriefStatusDraft), string(BriefStatusProposed),
		); err != nil {
			return nil, err
		}
	}
	if _, err := tx.Exec(
		`INSERT INTO agents_roundtable_brief_versions
			(room_id, version, status, content_json, proposed_by, source_turn_id,
			 created_at, updated_at, confirmed_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
		roomID, newVersion, string(status), string(contentJSON), string(proposedBy),
		strings.TrimSpace(sourceTurnID), timeToStr(now), timeToStr(now),
	); err != nil {
		return nil, fmt.Errorf("insert brief version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetBriefVersion(roomID, newVersion)
}

// ConfirmBriefVersion atomically confirms the current version. The separate
// operation is intentionally user-facing; agent proposal paths never call it.
func (s *Store) ConfirmBriefVersion(roomID string, version, expectedVersion int) (*BriefVersion, error) {
	target, err := s.GetBriefVersion(roomID, version)
	if err != nil {
		return nil, err
	}
	if target.Status == BriefStatusConfirmed {
		room, roomErr := s.GetRoom(roomID)
		if roomErr != nil {
			return nil, roomErr
		}
		if room.CurrentBriefVersion != expectedVersion {
			return nil, &BriefVersionConflictError{
				Expected: expectedVersion,
				Current:  room.CurrentBriefVersion,
			}
		}
		if room.ConfirmedBriefVersion == version {
			return target, nil
		}
	}
	if target.Status != BriefStatusDraft && target.Status != BriefStatusProposed {
		return nil, fmt.Errorf(
			"roundtable: brief v%d cannot be confirmed from status=%s",
			version,
			target.Status,
		)
	}
	contentJSON, err := json.Marshal(target.Content)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	tx, err := s.db.SQL().Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.Exec(
		`UPDATE agents_roundtable_rooms
		 SET brief_json = ?, confirmed_brief_version = ?, state = ?, updated_at = ?
		 WHERE id = ?
		   AND current_brief_version = ?
		   AND current_brief_version = ?
		   AND state = ?`,
		string(contentJSON), version, string(StateWaitingR2), timeToStr(now),
		roomID, expectedVersion, version, string(StateDraftingBrief),
	)
	if err != nil {
		return nil, err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		var currentVersion int
		var state string
		err := tx.QueryRow(
			`SELECT state, current_brief_version
			 FROM agents_roundtable_rooms WHERE id = ?`,
			roomID,
		).Scan(&state, &currentVersion)
		if err == sql.ErrNoRows {
			return nil, meta.ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		if currentVersion != expectedVersion || version != currentVersion {
			return nil, &BriefVersionConflictError{Expected: expectedVersion, Current: currentVersion}
		}
		return nil, fmt.Errorf("roundtable: confirm brief only in drafting_brief (state=%s)", state)
	}
	if _, err := tx.Exec(
		`UPDATE agents_roundtable_brief_versions
		 SET status = ?, updated_at = ?
		 WHERE room_id = ? AND version <> ? AND status = ?`,
		string(BriefStatusSuperseded), timeToStr(now), roomID, version,
		string(BriefStatusConfirmed),
	); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(
		`UPDATE agents_roundtable_brief_versions
		 SET status = ?, updated_at = ?, confirmed_at = ?
		 WHERE room_id = ? AND version = ?`,
		string(BriefStatusConfirmed), timeToStr(now), timeToStr(now), roomID, version,
	); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetBriefVersion(roomID, version)
}

// CaptureConfirmedBriefForR2 pins the confirmed version before any R2 prompt
// is built. Later current/confirmed pointer changes cannot affect this snapshot.
func (s *Store) CaptureConfirmedBriefForR2(roomID string) (*BriefVersion, error) {
	tx, err := s.db.SQL().Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(
		`UPDATE agents_roundtable_rooms
		 SET r2_brief_version = confirmed_brief_version, updated_at = ?
		 WHERE id = ?
		   AND state = ?
		   AND confirmed_brief_version > 0
		   AND r2_brief_version <> confirmed_brief_version`,
		timeToStr(time.Now().UTC()), roomID, string(StateWaitingR2),
	)
	if err != nil {
		return nil, err
	}
	affected, _ := res.RowsAffected()
	var state string
	var confirmedVersion, r2Version int
	err = tx.QueryRow(
		`SELECT state, confirmed_brief_version, r2_brief_version
		 FROM agents_roundtable_rooms WHERE id = ?`,
		roomID,
	).Scan(&state, &confirmedVersion, &r2Version)
	if err == sql.ErrNoRows {
		return nil, meta.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		if RoomState(state) != StateWaitingR2 {
			return nil, fmt.Errorf("roundtable: r2 only allowed in waiting_r2 (state=%s)", state)
		}
		if confirmedVersion <= 0 {
			return nil, fmt.Errorf("roundtable: confirmed brief required before r2")
		}
		return nil, fmt.Errorf("roundtable: r2 already started for brief v%d", r2Version)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetBriefVersion(roomID, confirmedVersion)
}

// ListSeats returns seats for a room in roster order (role order of DefaultRoster).
func (s *Store) ListSeats(roomID string) ([]Seat, error) {
	rows, err := s.db.SQL().Query(
		`SELECT id, room_id, role, agent_type, workspace_id, session_id, acp_session_id, status, created_at
		 FROM agents_roundtable_seats WHERE room_id = ?`, roomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byRole := map[Role]Seat{}
	for rows.Next() {
		var seat Seat
		var role, status, createdAt string
		if err := rows.Scan(
			&seat.ID, &seat.RoomID, &role, &seat.AgentType, &seat.WorkspaceID,
			&seat.SessionID, &seat.AcpSessionID, &status, &createdAt,
		); err != nil {
			return nil, err
		}
		seat.Role = Role(role)
		seat.Status = SeatStatus(status)
		seat.CreatedAt = strToTime(createdAt)
		byRole[seat.Role] = seat
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Seat, 0, len(DefaultRoster))
	for _, r := range DefaultRoster {
		if seat, ok := byRole[r]; ok {
			out = append(out, seat)
			delete(byRole, r)
		}
	}
	// Any unexpected roles last.
	for _, seat := range byRole {
		out = append(out, seat)
	}
	return out, nil
}

// FindSeatByWorkspaceID returns the seat bound to a disposable workspace id.
func (s *Store) FindSeatByWorkspaceID(workspaceID string) (*Seat, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return nil, fmt.Errorf("roundtable: workspace_id required")
	}
	row := s.db.SQL().QueryRow(
		`SELECT id, room_id, role, agent_type, workspace_id, session_id, acp_session_id, status, created_at
		 FROM agents_roundtable_seats WHERE workspace_id = ? LIMIT 1`, workspaceID)
	var seat Seat
	var role, status, createdAt string
	err := row.Scan(
		&seat.ID, &seat.RoomID, &role, &seat.AgentType, &seat.WorkspaceID,
		&seat.SessionID, &seat.AcpSessionID, &status, &createdAt,
	)
	if err == sql.ErrNoRows {
		return nil, meta.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	seat.Role = Role(role)
	seat.Status = SeatStatus(status)
	seat.CreatedAt = strToTime(createdAt)
	return &seat, nil
}

// FindRoomIDByWorkspacePath resolves a seat cwd (or workspace path) to room_id
// via meta.projects → seats.workspace_id. Used when CLI has no --room/env/sidecar.
func (s *Store) FindRoomIDByWorkspacePath(absPath string) (string, error) {
	absPath = filepath.Clean(strings.TrimSpace(absPath))
	if absPath == "" || absPath == "." {
		return "", fmt.Errorf("roundtable: empty workspace path")
	}
	projects, err := s.db.ListProjects()
	if err != nil {
		return "", err
	}
	// Match exact cwd, then parent (agent may chdir into a subdir of the seat).
	matchPath := func(path string) (string, bool) {
		for _, p := range projects {
			if filepath.Clean(p.WorkspacePath) != path {
				continue
			}
			seat, err := s.FindSeatByWorkspaceID(p.ID)
			if err == meta.ErrNotFound {
				continue
			}
			if err != nil {
				return "", false
			}
			if seat.RoomID != "" {
				return seat.RoomID, true
			}
		}
		return "", false
	}
	if id, ok := matchPath(absPath); ok {
		return id, nil
	}
	parent := filepath.Dir(absPath)
	if parent != absPath {
		if id, ok := matchPath(parent); ok {
			return id, nil
		}
	}
	return "", meta.ErrNotFound
}

// GetSeat returns one seat by id.
func (s *Store) GetSeat(seatID string) (*Seat, error) {
	row := s.db.SQL().QueryRow(
		`SELECT id, room_id, role, agent_type, workspace_id, session_id, acp_session_id, status, created_at
		 FROM agents_roundtable_seats WHERE id = ?`, seatID)
	var seat Seat
	var role, status, createdAt string
	err := row.Scan(
		&seat.ID, &seat.RoomID, &role, &seat.AgentType, &seat.WorkspaceID,
		&seat.SessionID, &seat.AcpSessionID, &status, &createdAt,
	)
	if err == sql.ErrNoRows {
		return nil, meta.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	seat.Role = Role(role)
	seat.Status = SeatStatus(status)
	seat.CreatedAt = strToTime(createdAt)
	return &seat, nil
}

// UpdateSeatSession persists session_id / acp_session_id / status for a seat.
func (s *Store) UpdateSeatSession(seat *Seat) error {
	if seat == nil || seat.ID == "" {
		return fmt.Errorf("roundtable: seat id required")
	}
	res, err := s.db.SQL().Exec(
		`UPDATE agents_roundtable_seats
		 SET session_id = ?, acp_session_id = ?, status = ?
		 WHERE id = ?`,
		seat.SessionID, seat.AcpSessionID, string(seat.Status), seat.ID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return meta.ErrNotFound
	}
	return nil
}

// UpdateRoomState persists room.State and UpdatedAt after a transition.
func (s *Store) UpdateRoomState(room *Room) error {
	if room == nil || room.ID == "" {
		return fmt.Errorf("roundtable: room id required")
	}
	room.UpdatedAt = time.Now().UTC()
	res, err := s.db.SQL().Exec(
		`UPDATE agents_roundtable_rooms SET state = ?, updated_at = ? WHERE id = ?`,
		string(room.State), timeToStr(room.UpdatedAt), room.ID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return meta.ErrNotFound
	}
	return nil
}

// UpdateRoomBriefAndState persists brief_json + state (R1 exit: confirm Brief → waiting_r2).
func (s *Store) UpdateRoomBriefAndState(room *Room) error {
	if room == nil || room.ID == "" {
		return fmt.Errorf("roundtable: room id required")
	}
	briefJSON := ""
	if room.Brief != nil {
		b, err := json.Marshal(room.Brief)
		if err != nil {
			return err
		}
		briefJSON = string(b)
	}
	room.UpdatedAt = time.Now().UTC()
	res, err := s.db.SQL().Exec(
		`UPDATE agents_roundtable_rooms SET brief_json = ?, state = ?, updated_at = ? WHERE id = ?`,
		nullIfEmpty(briefJSON), string(room.State), timeToStr(room.UpdatedAt), room.ID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return meta.ErrNotFound
	}
	return nil
}

// UpdateRoomSummaryR2AndState persists summary_r2 + state (R2 exit → waiting_r3).
func (s *Store) UpdateRoomSummaryR2AndState(room *Room) error {
	if room == nil || room.ID == "" {
		return fmt.Errorf("roundtable: room id required")
	}
	room.UpdatedAt = time.Now().UTC()
	res, err := s.db.SQL().Exec(
		`UPDATE agents_roundtable_rooms SET summary_r2 = ?, state = ?, updated_at = ? WHERE id = ?`,
		room.SummaryR2, string(room.State), timeToStr(room.UpdatedAt), room.ID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return meta.ErrNotFound
	}
	return nil
}

// UpdateRoomSummaryR3AndState persists summary_r3 + state (R3 exit → done).
func (s *Store) UpdateRoomSummaryR3AndState(room *Room) error {
	if room == nil || room.ID == "" {
		return fmt.Errorf("roundtable: room id required")
	}
	room.UpdatedAt = time.Now().UTC()
	res, err := s.db.SQL().Exec(
		`UPDATE agents_roundtable_rooms SET summary_r3 = ?, state = ?, updated_at = ? WHERE id = ?`,
		room.SummaryR3, string(room.State), timeToStr(room.UpdatedAt), room.ID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return meta.ErrNotFound
	}
	return nil
}

// InsertTurn appends a timeline turn.
func (s *Store) InsertTurn(t *Turn) error {
	if t == nil || t.ID == "" {
		return fmt.Errorf("roundtable: turn id required")
	}
	_, err := s.db.SQL().Exec(
		`INSERT INTO agents_roundtable_turns
			(id, room_id, round, seat_id, kind, content_text, process_ref, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.RoomID, t.Round, t.SeatID, t.Kind, t.ContentText, t.ProcessRef,
		timeToStr(t.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("insert turn: %w", err)
	}
	return nil
}

// ListTurns returns turns for a room in creation order (main timeline).
func (s *Store) ListTurns(roomID string) ([]Turn, error) {
	rows, err := s.db.SQL().Query(
		`SELECT id, room_id, round, seat_id, kind, content_text, process_ref, created_at
		 FROM agents_roundtable_turns WHERE room_id = ?
		 ORDER BY created_at ASC, id ASC`, roomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Turn
	for rows.Next() {
		var t Turn
		var createdAt string
		if err := rows.Scan(
			&t.ID, &t.RoomID, &t.Round, &t.SeatID, &t.Kind,
			&t.ContentText, &t.ProcessRef, &createdAt,
		); err != nil {
			return nil, err
		}
		t.CreatedAt = strToTime(createdAt)
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []Turn{}
	}
	return out, nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func timeToStr(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func strToTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t2, err2 := time.Parse(time.RFC3339, s)
		if err2 != nil {
			return time.Time{}
		}
		return t2
	}
	return t
}
