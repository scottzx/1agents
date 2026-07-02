// Package sources is the neutral owner of the multi-source ingestion "bronze"
// layer in sync.db: one opaque table (source_records) holding every source's
// raw records verbatim, and a generic cursor registry (sync_cursors) that
// replaces the Feishu-only epoch-ms watermark. Pulling writes only bronze;
// governance (internal/govern) reads bronze → writes the curated meta.db gold.
// This decouples "fetch" from "shape", so re-shaping never re-fetches (which
// matters because iCloud throttles repeated full pulls).
//
// The physical file is sync.db, currently owned by package feishu (a misnomer
// for a general ingestion store). We borrow its already-configured *sql.DB
// handle rather than opening a third DB file; the feishu→sources rename is a
// later cleanup (Epic #359 Phase 5).
package sources

import (
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/scottzx/1Agents/backend/internal/feishu"
)

// RawRecord is one source record as a puller reports it. UID is the source's
// stable per-record id (for DAV sources, the resource href); ETag is the
// per-record change token used to detect "this row actually changed".
type RawRecord struct {
	Kind        string // contact|event|todo|message|...
	Collection  string // address book / calendar / chat id the record lives in
	UID         string // stable id within (source, kind, collection)
	ETag        string // per-record change token; "" means "always treat as changed"
	ContentType string // text/vcard|text/calendar|application/json
	Payload     string // verbatim source bytes
	Deleted     bool   // a tombstone reported by an incremental sync
}

// Cursor is an opaque per-collection sync position (a DAV sync-token, a Graph
// delta link, an epoch-ms timestamp, ...). Kind names the flavor so a puller
// can interpret Value.
type Cursor struct {
	Kind  string // sync_token|delta_link|timestamp|page_token|ctag|govern
	Value string
}

// StoredRecord is a bronze row read back for governance / the data-source viewer.
type StoredRecord struct {
	Kind        string
	Collection  string
	UID         string
	ETag        string
	ContentType string
	Payload     string
	Deleted     bool
	FetchedAt   int64 // epoch ms
}

// Store is the bronze/cursor layer over sync.db.
type Store struct {
	sql *sql.DB
}

// NewStore wraps an existing sync.db handle and ensures the bronze schema.
func NewStore(db *sql.DB) (*Store, error) {
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("sources: apply schema: %w", err)
	}
	return &Store{sql: db}, nil
}

// OpenDefault borrows the default sync.db handle (the same cached connection the
// feishu/digest modules use) and ensures the bronze schema.
func OpenDefault() (*Store, error) {
	fs, err := feishu.OpenDefault()
	if err != nil {
		return nil, err
	}
	return NewStore(fs.SQL())
}

// schema is idempotent (CREATE IF NOT EXISTS); both tables are generic and
// additive, so no version-gated migration is needed (mirrors the feishu store).
const schema = `
CREATE TABLE IF NOT EXISTS source_records (
    source       TEXT    NOT NULL,
    account_id   TEXT    NOT NULL DEFAULT 'default',
    kind         TEXT    NOT NULL,
    collection   TEXT    NOT NULL DEFAULT '',
    uid          TEXT    NOT NULL,
    etag         TEXT    NOT NULL DEFAULT '',
    content_type TEXT    NOT NULL DEFAULT '',
    payload      TEXT    NOT NULL DEFAULT '',
    deleted      INTEGER NOT NULL DEFAULT 0,
    fetched_at   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (source, account_id, kind, collection, uid)
);
CREATE INDEX IF NOT EXISTS idx_source_records_gov
    ON source_records(source, kind, fetched_at);

CREATE TABLE IF NOT EXISTS sync_cursors (
    source       TEXT    NOT NULL,
    account_id   TEXT    NOT NULL DEFAULT 'default',
    kind         TEXT    NOT NULL DEFAULT '',
    collection   TEXT    NOT NULL DEFAULT '',
    cursor_kind  TEXT    NOT NULL DEFAULT '',
    cursor_value TEXT    NOT NULL DEFAULT '',
    gate_value   TEXT    NOT NULL DEFAULT '',
    updated_at   TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (source, account_id, kind, collection)
);
`

// CommitPage atomically upserts one page of records and advances the collection
// cursor. A record whose etag is unchanged is left untouched (its fetched_at
// stays put, so governance won't reprocess it); a changed etag, a new uid, or a
// tombstone bumps fetched_at. Returns how many rows were actually written.
// The collection's version gate (CTag) is preserved — use SaveGate for that.
func (st *Store) CommitPage(source, accountID string, recs []RawRecord, next Cursor) (changed int, err error) {
	tx, err := st.sql.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	now := time.Now().UnixMilli()
	stmt, err := tx.Prepare(`INSERT INTO source_records
        (source, account_id, kind, collection, uid, etag, content_type, payload, deleted, fetched_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(source, account_id, kind, collection, uid) DO UPDATE SET
            etag         = excluded.etag,
            content_type = excluded.content_type,
            payload      = excluded.payload,
            deleted      = excluded.deleted,
            fetched_at   = excluded.fetched_at
        WHERE source_records.etag != excluded.etag
           OR source_records.deleted != excluded.deleted`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	for _, r := range recs {
		del := 0
		if r.Deleted {
			del = 1
		}
		res, err := stmt.Exec(source, accountID, r.Kind, r.Collection, r.UID,
			r.ETag, r.ContentType, r.Payload, del, now)
		if err != nil {
			return 0, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			changed++
		}
	}
	// A page is always single-collection (the driver pulls one collection at a
	// time), so the first record identifies the (kind, collection) the cursor
	// belongs to. An empty page carries no target here; the driver advances that
	// cursor via SaveCollectionCursor instead.
	if next.Kind != "" && len(recs) > 0 {
		if err := upsertCursorKC(tx, source, accountID, recs[0].Kind, recs[0].Collection, next); err != nil {
			return 0, err
		}
	}
	return changed, tx.Commit()
}

// SaveCollectionCursor advances the cursor for an explicit (kind, collection),
// independent of a record page — used for empty incremental pages (nothing
// changed but the sync-token still moved) and for the governance cursor.
func (st *Store) SaveCollectionCursor(source, accountID, kind, collection string, cur Cursor) error {
	tx, err := st.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := upsertCursorKC(tx, source, accountID, kind, collection, cur); err != nil {
		return err
	}
	return tx.Commit()
}

// upsertCursorKC upserts the (cursor_kind, cursor_value) for an explicit
// (source, account, kind, collection), preserving gate_value on conflict.
func upsertCursorKC(tx *sql.Tx, source, accountID, kind, collection string, cur Cursor) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := tx.Exec(`INSERT INTO sync_cursors
        (source, account_id, kind, collection, cursor_kind, cursor_value, gate_value, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, '', ?)
        ON CONFLICT(source, account_id, kind, collection) DO UPDATE SET
            cursor_kind  = excluded.cursor_kind,
            cursor_value = excluded.cursor_value,
            updated_at   = excluded.updated_at`,
		source, accountID, kind, collection, cur.Kind, cur.Value, now)
	return err
}

// LoadCursor returns the stored cursor + version gate for a (kind, collection).
// ok is false when nothing has been synced yet.
func (st *Store) LoadCursor(source, accountID, kind, collection string) (cur Cursor, gate string, ok bool, err error) {
	var ck, cv string
	e := st.sql.QueryRow(`SELECT cursor_kind, cursor_value, gate_value FROM sync_cursors
        WHERE source = ? AND account_id = ? AND kind = ? AND collection = ?`,
		source, accountID, kind, collection).Scan(&ck, &cv, &gate)
	if e == sql.ErrNoRows {
		return Cursor{}, "", false, nil
	}
	if e != nil {
		return Cursor{}, "", false, e
	}
	return Cursor{Kind: ck, Value: cv}, gate, true, nil
}

// SaveGate records a collection's version gate (CardDAV CTag), so an unchanged
// collection can be skipped entirely on the next sync (zero network).
func (st *Store) SaveGate(source, accountID, kind, collection, gate string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := st.sql.Exec(`INSERT INTO sync_cursors
        (source, account_id, kind, collection, cursor_kind, cursor_value, gate_value, updated_at)
        VALUES (?, ?, ?, ?, '', '', ?, ?)
        ON CONFLICT(source, account_id, kind, collection) DO UPDATE SET
            gate_value = excluded.gate_value,
            updated_at = excluded.updated_at`,
		source, accountID, kind, collection, gate, now)
	return err
}

// RecordsSince returns every non-tombstone bronze record for (source, kind)
// with fetched_at strictly greater than since, plus the max fetched_at seen
// (the value to persist as the next governance cursor). Governance uses this to
// process only records changed since it last ran; a full re-govern is always
// safe (pass since=0) because the gold upsert is idempotent.
func (st *Store) RecordsSince(source, kind string, since int64) (recs []StoredRecord, maxFetched int64, err error) {
	rows, err := st.sql.Query(`SELECT kind, collection, uid, etag, content_type, payload, deleted, fetched_at
        FROM source_records
        WHERE source = ? AND kind = ? AND fetched_at > ?
        ORDER BY fetched_at`, source, kind, since)
	if err != nil {
		return nil, since, err
	}
	defer rows.Close()
	maxFetched = since
	for rows.Next() {
		var r StoredRecord
		var del int
		if err := rows.Scan(&r.Kind, &r.Collection, &r.UID, &r.ETag, &r.ContentType, &r.Payload, &del, &r.FetchedAt); err != nil {
			return nil, since, err
		}
		r.Deleted = del != 0
		if r.FetchedAt > maxFetched {
			maxFetched = r.FetchedAt
		}
		recs = append(recs, r)
	}
	return recs, maxFetched, rows.Err()
}

// SourceSummary is one (source, account, kind) rollup for the data-source
// overview: how many live records, across how many collections, and when last
// fetched. AccountID lets the 源为中心 UI show each account's own totals (谷歌+A
// vs 谷歌+B) instead of merging a vendor's accounts.
type SourceSummary struct {
	Source        string `json:"source"`
	AccountID     string `json:"accountId"`
	Kind          string `json:"kind"`
	Count         int    `json:"count"`         // non-deleted records
	Collections   int    `json:"collections"`   // distinct collections
	LastFetchedAt int64  `json:"lastFetchedAt"` // epoch ms, 0 if empty
}

// Summaries rolls up source_records by (source, account_id, kind) for the
// overview cards.
func (st *Store) Summaries() ([]SourceSummary, error) {
	rows, err := st.sql.Query(`SELECT source, account_id, kind,
        SUM(CASE WHEN deleted = 0 THEN 1 ELSE 0 END) AS cnt,
        COUNT(DISTINCT collection) AS colls,
        MAX(fetched_at) AS last
        FROM source_records
        GROUP BY source, account_id, kind
        ORDER BY source, account_id, kind`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SourceSummary{}
	for rows.Next() {
		var s SourceSummary
		if err := rows.Scan(&s.Source, &s.AccountID, &s.Kind, &s.Count, &s.Collections, &s.LastFetchedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ListRecords returns a (source, kind)'s bronze records, most-recently-fetched
// first, capped at limit (<=0 → a default cap). account scopes to one account_id
// when non-empty (源为中心 per-account browse); "" spans all of the source's
// accounts. Tombstones are included so the viewer can show deletions.
func (st *Store) ListRecords(source, account, kind string, limit int) ([]StoredRecord, error) {
	if limit <= 0 {
		limit = 1000
	}
	q := `SELECT kind, collection, uid, etag, content_type, payload, deleted, fetched_at
        FROM source_records
        WHERE source = ? AND kind = ?`
	args := []any{source, kind}
	if account != "" {
		q += ` AND account_id = ?`
		args = append(args, account)
	}
	q += ` ORDER BY fetched_at DESC, uid LIMIT ?`
	args = append(args, limit)
	rows, err := st.sql.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StoredRecord{}
	for rows.Next() {
		var r StoredRecord
		var del int
		if err := rows.Scan(&r.Kind, &r.Collection, &r.UID, &r.ETag, &r.ContentType, &r.Payload, &del, &r.FetchedAt); err != nil {
			return nil, err
		}
		r.Deleted = del != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

// ReassignAccount re-keys a source's bronze rows and cursors from oldID to newID.
// Used once at seed time to migrate pre-account-model "default" data onto the
// account registry id so per-account views (Summaries/ListRecords) line up.
// Idempotent in effect (a no-op when no oldID rows remain).
func (st *Store) ReassignAccount(source, oldID, newID string) error {
	if oldID == newID {
		return nil
	}
	tx, err := st.sql.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`UPDATE OR REPLACE source_records SET account_id = ? WHERE source = ? AND account_id = ?`,
		newID, source, oldID); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE OR REPLACE sync_cursors SET account_id = ? WHERE source = ? AND account_id = ?`,
		newID, source, oldID); err != nil {
		return err
	}
	return tx.Commit()
}

// GovernCursor reads the last-governed high-water mark (epoch ms) for a
// (source, kind). Stored as a cursor row with kind=<kind>, collection=governColl.
func (st *Store) GovernCursor(source, kind string) (int64, error) {
	cur, _, ok, err := st.LoadCursor(source, "default", kind, governColl)
	if err != nil || !ok || cur.Value == "" {
		return 0, err
	}
	v, err := strconv.ParseInt(cur.Value, 10, 64)
	if err != nil {
		return 0, nil // corrupt cursor → re-govern from scratch (safe, idempotent)
	}
	return v, nil
}

// SaveGovernCursor persists the governance high-water mark for (source, kind).
func (st *Store) SaveGovernCursor(source, kind string, fetchedAt int64) error {
	return st.SaveCollectionCursor(source, "default", kind, governColl,
		Cursor{Kind: "govern", Value: strconv.FormatInt(fetchedAt, 10)})
}

// governColl is the reserved collection slot for a per-(source,kind) governance
// cursor. Pull cursors always live under a non-empty resource collection, so
// this sentinel never collides with them.
const governColl = "\x00govern"
