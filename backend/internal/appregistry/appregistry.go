// Package appregistry is the compile-time application manifest registry.
// Each app calls Register from its init() function; the HTTP layer exposes
// the manifest list and enable/disable toggle (persisted via a small
// app_state table in meta.db).
//
// Design principle: this package MUST NOT import app code. Apps import it.
package appregistry

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/scottzx/1Agents/backend/internal/domainstore"
	"github.com/scottzx/1Agents/backend/internal/meta"
)

// MountPoint declares where an app's UI surface is attached.
//
// type ∈ { "project-tab", "l1-page", "lens" }.
type MountPoint struct {
	Type  string `json:"type"`            // "project-tab" | "l1-page" | "lens"
	ID    string `json:"id"`              // stable slug, unique within the app
	Label string `json:"label"`           // display text (may be i18n key)
	View  string `json:"view"`            // frontend component name
	Icon  string `json:"icon,omitempty"`  // lucide icon slug (l1-page only)
	Scope string `json:"scope,omitempty"` // "project" | "" (lens only)
}

// AppManifest is the canonical shape shared with the frontend and docs agents.
// Field names here are authoritative.
type AppManifest struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Version      string       `json:"version"`
	Enabled      bool         `json:"enabled"`
	MountPoints  []MountPoint `json:"mountPoints"`
	TaskTypes    []string     `json:"taskTypes"`
	DomainTables []string     `json:"domainTables"`
}

var (
	mu       sync.RWMutex
	registry = map[string]*AppManifest{}
	order    []string // insertion order for deterministic List()
)

// Register declares an application at compile time. Call from init().
// Panics on duplicate ID to catch copy-paste errors early.
func Register(m AppManifest) {
	mu.Lock()
	defer mu.Unlock()
	if _, dup := registry[m.ID]; dup {
		panic(fmt.Sprintf("appregistry: duplicate app id %q", m.ID))
	}
	cp := m
	registry[m.ID] = &cp
	order = append(order, m.ID)
}

// List returns all registered manifests in registration order, with current
// enabled state merged from the database.
func List() []AppManifest {
	db, err := meta.OpenDefault()
	states := map[string]bool{}
	if err == nil {
		states = loadStates(db)
	}

	mu.RLock()
	defer mu.RUnlock()
	out := make([]AppManifest, 0, len(order))
	for _, id := range order {
		m := *registry[id]
		if en, ok := states[id]; ok {
			m.Enabled = en
		}
		out = append(out, m)
	}
	return out
}

// Get returns a manifest by id, with enabled state merged from the database.
// ok=false when the id is not registered.
func Get(id string) (AppManifest, bool) {
	mu.RLock()
	m, ok := registry[id]
	mu.RUnlock()
	if !ok {
		return AppManifest{}, false
	}
	cp := *m
	db, err := meta.OpenDefault()
	if err == nil {
		states := loadStates(db)
		if en, found := states[id]; found {
			cp.Enabled = en
		}
	}
	return cp, true
}

// SetEnabled persists the enabled state for app id and updates the in-memory
// default. Returns an error when the app is not registered.
func SetEnabled(id string, enabled bool) error {
	mu.Lock()
	m, ok := registry[id]
	if !ok {
		mu.Unlock()
		return fmt.Errorf("appregistry: unknown app %q", id)
	}
	m.Enabled = enabled
	mu.Unlock()

	db, err := meta.OpenDefault()
	if err != nil {
		return fmt.Errorf("appregistry: open db: %w", err)
	}
	if err := ensureAppStateTable(db.SQL()); err != nil {
		return fmt.Errorf("appregistry: ensure app_state table: %w", err)
	}
	_, err = db.SQL().Exec(
		`INSERT INTO app_state (app_id, enabled) VALUES (?, ?)
		 ON CONFLICT(app_id) DO UPDATE SET enabled=excluded.enabled`,
		id, boolToInt(enabled),
	)
	return err
}

// EnsureDomainTables creates domain tables for appID using the provided DDL
// statements. Each DDL must use "CREATE TABLE IF NOT EXISTS" and the table
// name must be prefixed with appID + "_". Idempotent — safe to call on every
// startup. Does NOT touch the global schemaVersion counter.
//
// Delegates to domainstore.EnsureTables for the actual execution.
//
// Example call from an app's init():
//
//	appregistry.EnsureDomainTables("media", []string{
//	    `CREATE TABLE IF NOT EXISTS media_content_project (...)`,
//	    `CREATE TABLE IF NOT EXISTS media_material (...)`,
//	})
func EnsureDomainTables(appID string, ddls []string) error {
	db, err := meta.OpenDefault()
	if err != nil {
		return fmt.Errorf("appregistry: open db for %s domain tables: %w", appID, err)
	}
	return domainstore.EnsureTables(db.SQL(), appID, ddls)
}

// ── HTTP handlers ─────────────────────────────────────────────────────────

// HandleList handles GET /api/apps → {apps: [...]}
func HandleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{"apps": List()})
}

// HandleEnable handles POST /api/apps/{id}/enable
func HandleEnable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := extractAppID(r.URL.Path, "/enable")
	if id == "" {
		http.Error(w, "missing app id", http.StatusBadRequest)
		return
	}
	if err := SetEnabled(id, true); err != nil {
		log.Printf("[appregistry] enable %s: %v", id, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "id": id, "enabled": true})
}

// HandleDisable handles POST /api/apps/{id}/disable
func HandleDisable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := extractAppID(r.URL.Path, "/disable")
	if id == "" {
		http.Error(w, "missing app id", http.StatusBadRequest)
		return
	}
	if err := SetEnabled(id, false); err != nil {
		log.Printf("[appregistry] disable %s: %v", id, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "id": id, "enabled": false})
}

// ── internals ─────────────────────────────────────────────────────────────

func ensureAppStateTable(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS app_state (
		app_id  TEXT PRIMARY KEY,
		enabled INTEGER NOT NULL DEFAULT 1
	)`)
	return err
}

func loadStates(db *meta.DB) map[string]bool {
	if err := ensureAppStateTable(db.SQL()); err != nil {
		return map[string]bool{}
	}
	rows, err := db.SQL().Query(`SELECT app_id, enabled FROM app_state`)
	if err != nil {
		return map[string]bool{}
	}
	defer rows.Close()
	states := map[string]bool{}
	for rows.Next() {
		var id string
		var en int
		if err := rows.Scan(&id, &en); err == nil {
			states[id] = en != 0
		}
	}
	return states
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// extractAppID strips prefix and suffix from a path like /api/apps/{id}/enable.
// suffix is "/enable" or "/disable".
func extractAppID(path, suffix string) string {
	// e.g. /api/apps/media/enable
	path = strings.TrimPrefix(path, "/api/apps/")
	path = strings.TrimSuffix(path, suffix)
	return strings.Trim(path, "/")
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[appregistry] json encode: %v", err)
	}
}
