package meta

import (
	"database/sql"
	"encoding/json"
	"time"
)

// ChannelModule is one data-source sub-module's consent + crawl-rule state — e.g.
// icloud.contacts, apple.imessage, feishu.groups. Consent (an explicit, recorded
// user authorization) is required before any sync runs; Rules are the deterministic
// syncer's input parameters (frequency + scope), never interpreted by an AI.
type ChannelModule struct {
	ID              string         `json:"id"`
	Consented       bool           `json:"consented"`
	ConsentedAt     string         `json:"consentedAt"`
	AutoSync        bool           `json:"autoSync"`
	IntervalMinutes int            `json:"intervalMinutes"`
	Rules           map[string]any `json:"rules"`
}

// ChannelModuleStore persists per-sub-module consent + crawl rules.
type ChannelModuleStore struct{ db *DB }

// NewChannelModuleStore returns a store over db.
func NewChannelModuleStore(db *DB) *ChannelModuleStore { return &ChannelModuleStore{db: db} }

// ensureChannelModules creates the table if absent. Run unconditionally at Open
// (not version-gated) so it survives the meta schema-version collisions noted in
// migrateSchema. Idempotent.
func (db *DB) ensureChannelModules() error {
	_, err := db.sql.Exec(`CREATE TABLE IF NOT EXISTS channel_modules (
        module_id        TEXT    NOT NULL PRIMARY KEY,
        consented_at     TEXT    NOT NULL DEFAULT '',
        auto_sync        INTEGER NOT NULL DEFAULT 0,
        interval_minutes INTEGER NOT NULL DEFAULT 0,
        rules            TEXT    NOT NULL DEFAULT '{}',
        updated_at       TEXT    NOT NULL DEFAULT ''
    )`)
	return err
}

// Get returns a module's state. A never-configured module reads back un-consented
// with empty rules (not an error).
func (s *ChannelModuleStore) Get(id string) (ChannelModule, error) {
	m := ChannelModule{ID: id, Rules: map[string]any{}}
	var consentedAt, rules string
	var autoSync, interval int
	err := s.db.sql.QueryRow(`SELECT consented_at, auto_sync, interval_minutes, rules
        FROM channel_modules WHERE module_id = ?`, id).Scan(&consentedAt, &autoSync, &interval, &rules)
	if err == sql.ErrNoRows {
		return m, nil
	}
	if err != nil {
		return m, err
	}
	m.ConsentedAt = consentedAt
	m.Consented = consentedAt != ""
	m.AutoSync = autoSync != 0
	m.IntervalMinutes = interval
	if rules != "" {
		_ = json.Unmarshal([]byte(rules), &m.Rules)
	}
	return m, nil
}

// SetConsent records explicit user authorization (stamps consented_at = now),
// creating the row if needed.
func (s *ChannelModuleStore) SetConsent(id string) error {
	now := timeToStr(time.Now().UTC())
	_, err := s.db.sql.Exec(`INSERT INTO channel_modules (module_id, consented_at, updated_at)
        VALUES (?, ?, ?)
        ON CONFLICT(module_id) DO UPDATE SET consented_at = excluded.consented_at, updated_at = excluded.updated_at`,
		id, now, now)
	return err
}

// RevokeConsent clears a module's authorization (sync is gated again).
func (s *ChannelModuleStore) RevokeConsent(id string) error {
	now := timeToStr(time.Now().UTC())
	_, err := s.db.sql.Exec(`INSERT INTO channel_modules (module_id, consented_at, updated_at)
        VALUES (?, '', ?)
        ON CONFLICT(module_id) DO UPDATE SET consented_at = '', updated_at = excluded.updated_at`,
		id, now)
	return err
}

// SetRules upserts a module's crawl rules (frequency + scope), preserving consent.
func (s *ChannelModuleStore) SetRules(id string, autoSync bool, intervalMinutes int, rules map[string]any) error {
	if rules == nil {
		rules = map[string]any{}
	}
	blob, err := json.Marshal(rules)
	if err != nil {
		return err
	}
	now := timeToStr(time.Now().UTC())
	a := 0
	if autoSync {
		a = 1
	}
	_, err = s.db.sql.Exec(`INSERT INTO channel_modules (module_id, auto_sync, interval_minutes, rules, updated_at)
        VALUES (?, ?, ?, ?, ?)
        ON CONFLICT(module_id) DO UPDATE SET
            auto_sync = excluded.auto_sync,
            interval_minutes = excluded.interval_minutes,
            rules = excluded.rules,
            updated_at = excluded.updated_at`,
		id, a, intervalMinutes, string(blob), now)
	return err
}
