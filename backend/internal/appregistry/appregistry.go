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
	"errors"
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
//
// Product Shell fields (C0, design §8 / D7) are all optional: a mount point
// without them behaves exactly as before — visible in every enabled shell,
// legacy slot/order, no permission gate.
type MountPoint struct {
	Type  string `json:"type"`            // "project-tab" | "l1-page" | "lens"
	ID    string `json:"id"`              // stable slug, unique within the app
	Label string `json:"label"`           // display text (may be i18n key)
	View  string `json:"view"`            // frontend component name
	Icon  string `json:"icon,omitempty"`  // lucide icon slug (l1-page only)
	Scope string `json:"scope,omitempty"` // "project" | "" (lens only)

	// Shells lists the Product Shell ids this mount contributes to
	// (e.g. ["presales"]). Empty means every enabled shell (legacy
	// behavior).
	Shells []string `json:"shells,omitempty"`
	// Slot is the placement zone within a shell (e.g. "nav", "home").
	// Empty falls back to a slot derived from Type.
	Slot string `json:"slot,omitempty"`
	// Order sorts mounts within the same slot (ascending; 0 default).
	Order int `json:"order,omitempty"`
	// Permission is the permission key required to see this mount.
	// Empty means visible to everyone. When the active permission
	// resolver denies it, the mount is dropped from navigation and
	// composition entirely.
	Permission string `json:"permission,omitempty"`
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

	// Capability Contract fields (C0, design §6.2/§6.3). All optional:
	// manifests without them behave exactly as before C0.
	Provides    []string      `json:"provides,omitempty"`    // capability refs "id@major" this app provides
	Requires    []Requirement `json:"requires,omitempty"`    // capabilities this app needs
	Permissions []string      `json:"permissions,omitempty"` // declared permissions (C0: declaration only)
}

var (
	mu             sync.RWMutex
	registry       = map[string]*AppManifest{}
	order          []string              // insertion order for deterministic List()
	providersByRef = map[string]string{} // capability ref ("id@major") → providing app id
)

// Register declares an application at compile time. Call from init().
// Panics on duplicate ID, duplicate capability provider, or malformed
// provides/requires references to catch copy-paste errors early.
func Register(m AppManifest) {
	mu.Lock()
	defer mu.Unlock()
	if _, dup := registry[m.ID]; dup {
		panic(fmt.Sprintf("appregistry: duplicate app id %q", m.ID))
	}
	for _, p := range m.Provides {
		ref, err := ParseCapabilityRef(p)
		if err != nil {
			panic(fmt.Sprintf("appregistry: app %q: %v", m.ID, err))
		}
		if owner, dup := providersByRef[ref.String()]; dup {
			panic(fmt.Sprintf("appregistry: duplicate provider for capability %s: %q and %q",
				ref, owner, m.ID))
		}
		providersByRef[ref.String()] = m.ID
	}
	for _, req := range m.Requires {
		if req.Capability == "" || req.Major < 0 {
			panic(fmt.Sprintf("appregistry: app %q: malformed requirement %+v", m.ID, req))
		}
	}
	cp := m
	registry[m.ID] = &cp
	order = append(order, m.ID)
}

// List returns all registered manifests in registration order, with the
// effective enabled state from capability validation (persisted intent merged
// with manifest defaults). Mount points gated by unmet optional requirements
// are omitted.
func List() []AppManifest {
	diags := Validate(currentPersistedStates())
	diagByID := make(map[string]AppDiagnostic, len(diags))
	for _, d := range diags {
		diagByID[d.AppID] = d
	}

	mu.RLock()
	defer mu.RUnlock()
	out := make([]AppManifest, 0, len(order))
	for _, id := range order {
		m := *registry[id]
		m.Enabled = diagByID[id].Enabled
		m.MountPoints = filterMountPoints(m.MountPoints, diagByID[id].DroppedMountPoints)
		out = append(out, m)
	}
	return out
}

// Get returns a manifest by id, with the effective enabled state from
// capability validation. ok=false when the id is not registered.
func Get(id string) (AppManifest, bool) {
	diags := Validate(currentPersistedStates())
	var diag AppDiagnostic
	found := false
	for _, d := range diags {
		if d.AppID == id {
			diag = d
			found = true
			break
		}
	}
	if !found {
		return AppManifest{}, false
	}
	mu.RLock()
	cp := *registry[id]
	mu.RUnlock()
	cp.Enabled = diag.Enabled
	cp.MountPoints = filterMountPoints(cp.MountPoints, diag.DroppedMountPoints)
	return cp, true
}

// SetEnabled persists the enabled state for app id and updates the in-memory
// default. Returns an error when the app is not registered, and a
// *RejectedError when capability validation rejects the change:
//
//   - enabling is rejected when the app's required capabilities are unmet
//     (missing, incompatible major, provider disabled) or part of a cycle;
//   - disabling is rejected while effectively-enabled apps still require
//     capabilities this app provides.
func SetEnabled(id string, enabled bool) error {
	mu.RLock()
	_, ok := registry[id]
	mu.RUnlock()
	if !ok {
		return fmt.Errorf("appregistry: unknown app %q", id)
	}
	if err := validateStateChange(id, enabled); err != nil {
		return err
	}

	mu.Lock()
	registry[id].Enabled = enabled
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

// filterMountPoints removes mount points whose ids are in dropped.
func filterMountPoints(mps []MountPoint, dropped []string) []MountPoint {
	if len(dropped) == 0 {
		return mps
	}
	drop := make(map[string]bool, len(dropped))
	for _, id := range dropped {
		drop[id] = true
	}
	out := make([]MountPoint, 0, len(mps))
	for _, mp := range mps {
		if !drop[mp.ID] {
			out = append(out, mp)
		}
	}
	return out
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

// ── Permission gating (design §10: C0 uses the same authorization facts) ──

// PermissionResolver decides whether userID holds the given permission key.
// nil resolver (the C0 default) allows everything, so manifests without
// permissions and deployments without RBAC behave exactly as before.
type PermissionResolver func(userID, permission string) bool

var (
	permMu       sync.RWMutex
	permResolver PermissionResolver
)

// SetPermissionResolver installs the process-wide permission check used for
// mount-point visibility. Passing nil restores allow-all.
func SetPermissionResolver(r PermissionResolver) {
	permMu.Lock()
	permResolver = r
	permMu.Unlock()
}

// permissionAllowed reports whether userID may see a mount gated by
// permission. Empty permission is always allowed; with no resolver
// registered everything is allowed.
func permissionAllowed(userID, permission string) bool {
	if permission == "" {
		return true
	}
	permMu.RLock()
	r := permResolver
	permMu.RUnlock()
	if r == nil {
		return true
	}
	return r(userID, permission)
}

// filterMountsByPermission drops mounts whose permission the user does not
// hold, so pages the user may not see never enter navigation (design §8:
// shells compose only visible contributions).
func filterMountsByPermission(userID string, mps []MountPoint) []MountPoint {
	out := make([]MountPoint, 0, len(mps))
	for _, mp := range mps {
		if permissionAllowed(userID, mp.Permission) {
			out = append(out, mp)
		}
	}
	return out
}

// ── HTTP handlers ─────────────────────────────────────────────────────────

// DefaultUserID is the C0 single-user identity: the backend has no
// per-request user model yet (installation access token only), so shell and
// permission APIs default to the local owner.
const DefaultUserID = "owner"

// requestUserID returns the caller identity for shell/permission scoping:
// the ?user= query value when present, DefaultUserID otherwise.
func requestUserID(r *http.Request) string {
	if u := r.URL.Query().Get("user"); u != "" {
		return u
	}
	return DefaultUserID
}

// HandleList handles GET /api/apps → {apps: [...]}. Mount points the
// requesting user lacks permission for are omitted (?user=, default owner).
func HandleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user := requestUserID(r)
	apps := List()
	for i := range apps {
		apps[i].MountPoints = filterMountsByPermission(user, apps[i].MountPoints)
	}
	writeJSON(w, map[string]any{"apps": apps})
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
		writeSetEnabledError(w, err)
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
		writeSetEnabledError(w, err)
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

// writeSetEnabledError maps a SetEnabled error to an HTTP response: 409 for
// capability-validation rejections (the message carries the reasons), 500
// otherwise.
func writeSetEnabledError(w http.ResponseWriter, err error) {
	var rejected *RejectedError
	if errors.As(err, &rejected) {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}
