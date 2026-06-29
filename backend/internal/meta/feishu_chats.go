package meta

import (
	"database/sql"
	"time"
)

// TrackedChat is one Feishu group the user has chosen to keep synced. AutoSync
// gates whether the periodic loop includes it; LastSyncedAt (epoch ms) is bumped
// after each sync and drives the per-chat cadence. ChatName/Avatar are cached
// from the chat list so the Messages tab can show a human name (Feishu often
// omits group names in message payloads).
type TrackedChat struct {
	ChatID       string    `json:"chatId"`
	ChatName     string    `json:"chatName"`
	Avatar       string    `json:"avatar"`
	External     bool      `json:"external"`
	AutoSync     bool      `json:"autoSync"`
	LastSyncedAt int64     `json:"lastSyncedAt"`
	CreatedAt    time.Time `json:"createdAt"`
}

// SyncConfig is the global auto-sync toggle + interval (minutes) governing the
// periodic loop. Single row (id = 1).
type SyncConfig struct {
	Enabled         bool `json:"enabled"`
	IntervalMinutes int  `json:"intervalMinutes"`
}

// FeishuChatStore manages tracked chats + the global sync config (meta.db v17).
type FeishuChatStore struct {
	db *DB
}

// NewFeishuChatStore returns a FeishuChatStore over db.
func NewFeishuChatStore(db *DB) *FeishuChatStore { return &FeishuChatStore{db: db} }

const trackedChatCols = `chat_id, chat_name, avatar, external, auto_sync, last_synced_at, created_at`

func scanTrackedChat(r rowScanner) (TrackedChat, error) {
	var t TrackedChat
	var external, autoSync int
	var createdAt string
	if err := r.Scan(&t.ChatID, &t.ChatName, &t.Avatar, &external, &autoSync, &t.LastSyncedAt, &createdAt); err != nil {
		return TrackedChat{}, err
	}
	t.External = external != 0
	t.AutoSync = autoSync != 0
	t.CreatedAt = strToTime(createdAt)
	return t, nil
}

// ListTrackedChats returns tracked chats ordered by created_at. When autoOnly is
// set, only those with auto_sync = 1 are returned (the periodic-loop subset).
func (s *FeishuChatStore) ListTrackedChats(autoOnly bool) ([]TrackedChat, error) {
	q := `SELECT ` + trackedChatCols + ` FROM feishu_tracked_chats`
	if autoOnly {
		q += ` WHERE auto_sync = 1`
	}
	q += ` ORDER BY created_at`
	rows, err := s.db.sql.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TrackedChat{}
	for rows.Next() {
		t, err := scanTrackedChat(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetTrackedChat returns a tracked chat by id; ok is false when absent.
func (s *FeishuChatStore) GetTrackedChat(chatID string) (TrackedChat, bool, error) {
	row := s.db.sql.QueryRow(`SELECT `+trackedChatCols+` FROM feishu_tracked_chats WHERE chat_id = ?`, chatID)
	t, err := scanTrackedChat(row)
	if err == sql.ErrNoRows {
		return TrackedChat{}, false, nil
	}
	if err != nil {
		return TrackedChat{}, false, err
	}
	return t, true, nil
}

// UpsertTrackedChat inserts a tracked chat or refreshes its name/avatar/external/
// auto_sync, preserving created_at and last_synced_at on update (fetch-or-keep).
func (s *FeishuChatStore) UpsertTrackedChat(t TrackedChat) error {
	existing, ok, err := s.GetTrackedChat(t.ChatID)
	if err != nil {
		return err
	}
	createdAt := time.Now().UTC()
	lastSynced := t.LastSyncedAt
	if ok {
		createdAt = existing.CreatedAt
		lastSynced = existing.LastSyncedAt
	}
	_, err = s.db.sql.Exec(`INSERT OR REPLACE INTO feishu_tracked_chats
        (`+trackedChatCols+`)
        VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t.ChatID, t.ChatName, t.Avatar, boolToInt(t.External), boolToInt(t.AutoSync),
		lastSynced, timeToStr(createdAt))
	return err
}

// DeleteTrackedChat removes a chat from the tracked set (untrack).
func (s *FeishuChatStore) DeleteTrackedChat(chatID string) error {
	_, err := s.db.sql.Exec(`DELETE FROM feishu_tracked_chats WHERE chat_id = ?`, chatID)
	return err
}

// SetTrackedAutoSync toggles a tracked chat's auto-sync flag. Returns ErrNotFound
// when the chat is not tracked.
func (s *FeishuChatStore) SetTrackedAutoSync(chatID string, on bool) error {
	res, err := s.db.sql.Exec(`UPDATE feishu_tracked_chats SET auto_sync = ? WHERE chat_id = ?`,
		boolToInt(on), chatID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateTrackedSynced bumps last_synced_at (epoch ms) and refreshes the cached
// name after a sync. No-op (no error) if the chat is not tracked.
func (s *FeishuChatStore) UpdateTrackedSynced(chatID, chatName string, ts int64) error {
	if chatName != "" {
		_, err := s.db.sql.Exec(`UPDATE feishu_tracked_chats SET last_synced_at = ?, chat_name = ? WHERE chat_id = ?`,
			ts, chatName, chatID)
		return err
	}
	_, err := s.db.sql.Exec(`UPDATE feishu_tracked_chats SET last_synced_at = ? WHERE chat_id = ?`, ts, chatID)
	return err
}

// GetSyncConfig returns the global sync config, inserting the default
// {enabled: true, interval: 180} row when absent.
func (s *FeishuChatStore) GetSyncConfig() (SyncConfig, error) {
	var enabled, interval int
	err := s.db.sql.QueryRow(`SELECT enabled, interval_minutes FROM feishu_sync_config WHERE id = 1`).
		Scan(&enabled, &interval)
	if err == sql.ErrNoRows {
		if _, err := s.db.sql.Exec(`INSERT INTO feishu_sync_config (id, enabled, interval_minutes) VALUES (1, 1, 180)`); err != nil {
			return SyncConfig{}, err
		}
		return SyncConfig{Enabled: true, IntervalMinutes: 180}, nil
	}
	if err != nil {
		return SyncConfig{}, err
	}
	return SyncConfig{Enabled: enabled != 0, IntervalMinutes: interval}, nil
}

// SetSyncConfig persists the global toggle + interval (upsert on the single row).
func (s *FeishuChatStore) SetSyncConfig(enabled bool, intervalMinutes int) error {
	if intervalMinutes <= 0 {
		intervalMinutes = 180
	}
	_, err := s.db.sql.Exec(`INSERT INTO feishu_sync_config (id, enabled, interval_minutes)
        VALUES (1, ?, ?)
        ON CONFLICT(id) DO UPDATE SET enabled = excluded.enabled, interval_minutes = excluded.interval_minutes`,
		boolToInt(enabled), intervalMinutes)
	return err
}

// TrackedNamesBySession returns a session_id → chat_name map for tracked chats
// with a non-empty name. Used to overlay human names onto SessionSummaries.
func (s *FeishuChatStore) TrackedNamesBySession() (map[string]string, error) {
	rows, err := s.db.sql.Query(`SELECT chat_id, chat_name FROM feishu_tracked_chats WHERE chat_name != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	names := map[string]string{}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		names[id] = name
	}
	return names, rows.Err()
}
