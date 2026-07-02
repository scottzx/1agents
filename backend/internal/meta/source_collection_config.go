package meta

import (
	"database/sql"
	"time"
)

// SourceCollectionConfig is the per-(source, kind) crawl policy for the data-
// source ingestion base: whether the kind is crawled at all, how far back the
// first crawl reaches, how often incremental crawls run (fed to the work-order
// scheduler's interval Recurrence), and the page size. It governs FETCH only;
// governance (bronze→gold) is a separate step.
type SourceCollectionConfig struct {
	Source              string `json:"source"`
	Kind                string `json:"kind"`
	Enabled             bool   `json:"enabled"`
	InitialLookbackDays int    `json:"initialLookbackDays"`
	IncrementalMinutes  int    `json:"incrementalMinutes"`
	PageSize            int    `json:"pageSize"`
	UpdatedAt           string `json:"updatedAt"`
}

// defaults for a never-configured collection.
const (
	defaultLookbackDays = 7
	defaultIncrMinutes  = 60
	defaultPageSize     = 50
)

// SourceCollectionStore persists per-collection crawl config.
type SourceCollectionStore struct{ db *DB }

// NewSourceCollectionStore returns a store over db.
func NewSourceCollectionStore(db *DB) *SourceCollectionStore { return &SourceCollectionStore{db: db} }

// ensureSourceCollectionConfig creates the table if absent. Run unconditionally
// at Open (not version-gated) so it survives the meta schema-version collisions
// noted in migrateSchema. Idempotent.
func (db *DB) ensureSourceCollectionConfig() error {
	_, err := db.sql.Exec(`CREATE TABLE IF NOT EXISTS source_collection_config (
        source                TEXT    NOT NULL,
        kind                  TEXT    NOT NULL,
        enabled               INTEGER NOT NULL DEFAULT 0,
        initial_lookback_days INTEGER NOT NULL DEFAULT 7,
        incremental_minutes   INTEGER NOT NULL DEFAULT 60,
        page_size             INTEGER NOT NULL DEFAULT 50,
        updated_at            TEXT    NOT NULL DEFAULT '',
        PRIMARY KEY (source, kind)
    )`)
	return err
}

// Get returns the stored config for (source, kind). ok=false (with a default-
// valued struct) when the collection was never configured.
func (s *SourceCollectionStore) Get(source, kind string) (SourceCollectionConfig, bool, error) {
	c := SourceCollectionConfig{
		Source: source, Kind: kind,
		InitialLookbackDays: defaultLookbackDays,
		IncrementalMinutes:  defaultIncrMinutes,
		PageSize:            defaultPageSize,
	}
	var enabled int
	err := s.db.sql.QueryRow(`SELECT enabled, initial_lookback_days, incremental_minutes, page_size, updated_at
        FROM source_collection_config WHERE source = ? AND kind = ?`, source, kind).
		Scan(&enabled, &c.InitialLookbackDays, &c.IncrementalMinutes, &c.PageSize, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return c, false, nil
	}
	if err != nil {
		return c, false, err
	}
	c.Enabled = enabled != 0
	return c, true, nil
}

// List returns every stored config for a source (unstored kinds are the caller's
// job to fill from the catalog with defaults).
func (s *SourceCollectionStore) List(source string) ([]SourceCollectionConfig, error) {
	rows, err := s.db.sql.Query(`SELECT kind, enabled, initial_lookback_days, incremental_minutes, page_size, updated_at
        FROM source_collection_config WHERE source = ? ORDER BY kind`, source)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SourceCollectionConfig{}
	for rows.Next() {
		c := SourceCollectionConfig{Source: source}
		var enabled int
		if err := rows.Scan(&c.Kind, &enabled, &c.InitialLookbackDays, &c.IncrementalMinutes, &c.PageSize, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.Enabled = enabled != 0
		out = append(out, c)
	}
	return out, rows.Err()
}

// ListEnabled returns only the enabled collections for a source — the set the
// puller crawls.
func (s *SourceCollectionStore) ListEnabled(source string) ([]SourceCollectionConfig, error) {
	all, err := s.List(source)
	if err != nil {
		return nil, err
	}
	out := make([]SourceCollectionConfig, 0, len(all))
	for _, c := range all {
		if c.Enabled {
			out = append(out, c)
		}
	}
	return out, nil
}

// Upsert writes a collection's config, clamping obviously-bad values to safe
// minimums so a malformed request can't wedge the scheduler.
func (s *SourceCollectionStore) Upsert(c SourceCollectionConfig) error {
	if c.IncrementalMinutes < 1 {
		c.IncrementalMinutes = defaultIncrMinutes
	}
	if c.PageSize < 1 {
		c.PageSize = defaultPageSize
	}
	if c.InitialLookbackDays < 0 {
		c.InitialLookbackDays = 0
	}
	enabled := 0
	if c.Enabled {
		enabled = 1
	}
	_, err := s.db.sql.Exec(`INSERT INTO source_collection_config
        (source, kind, enabled, initial_lookback_days, incremental_minutes, page_size, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(source, kind) DO UPDATE SET
            enabled               = excluded.enabled,
            initial_lookback_days = excluded.initial_lookback_days,
            incremental_minutes   = excluded.incremental_minutes,
            page_size             = excluded.page_size,
            updated_at            = excluded.updated_at`,
		c.Source, c.Kind, enabled, c.InitialLookbackDays, c.IncrementalMinutes, c.PageSize,
		timeToStr(time.Now().UTC()))
	return err
}
