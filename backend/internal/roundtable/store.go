package roundtable

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

// Store persists rooms, seats, and turns in meta.db (design §5.3: 刷新可恢复).
type Store struct {
	db *meta.DB
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
			summary_r2 TEXT NOT NULL DEFAULT '',
			summary_r3 TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
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
	return nil
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
			(id, title, state, brief_json, summary_r2, summary_r3, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		room.ID, room.Title, string(room.State), nullIfEmpty(briefJSON),
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
		`SELECT id, title, state, brief_json, summary_r2, summary_r3, created_at, updated_at
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
		`SELECT id, title, state, brief_json, summary_r2, summary_r3, created_at, updated_at
		 FROM agents_roundtable_rooms WHERE id = ?`, id)
	var room Room
	var state, briefJSON, createdAt, updatedAt string
	var briefNS sql.NullString
	err := row.Scan(
		&room.ID, &room.Title, &state, &briefNS,
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
	return &room, nil
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
