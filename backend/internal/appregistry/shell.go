// Product Shell Registry (design: docs/architecture/enterprise-foundation-v1.0.0.md §8, D7).
//
// A Product Shell is a UX composition layer — navigation, home, terminology
// and default views — NOT a data layer. The three built-in shells (personal
// workbench, presales & delivery, commerce operations) share the same
// Workspace, Task, Session, permission and audit facts; nothing is copied
// per shell, and there is exactly one frontend build.
//
// C0 scope: compile-time shell registration, tenant-scoped enable state and
// default shell (Product Profile), per-user shell preference, and
// shell/slot/order-aware composition of app mount points. Full RBAC,
// multi-tenant management and shell-specific data stores are explicitly out
// of scope.
package appregistry

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

// Built-in Product Shell ids (design §8). Registration order defines the
// fallback order for default-shell resolution.
const (
	ShellPersonal = "personal" // 个人工作台
	ShellPresales = "presales" // 售前与交付
	ShellCommerce = "commerce" // 电商运营
)

// DefaultTenantID is the C0 tenant scope. Multi-tenancy is deferred
// (design §10), but all profile state is already keyed by tenant so the
// single-tenant → multi-tenant upgrade is a data migration only.
const DefaultTenantID = "default"

// ProductShellManifest declares one product shell at compile time.
type ProductShellManifest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
	// Enabled is the manifest default; the effective state merges the
	// persisted tenant intent (shell_state), mirroring AppManifest.
	Enabled bool `json:"enabled"`
}

// ComposedMount is one app mount point placed in a shell by ComposeShell.
type ComposedMount struct {
	AppID string `json:"appId"`
	// Slot is the resolved placement zone: the mount's declared slot, or
	// its type for legacy mounts (so existing rendering zones are kept).
	Slot  string     `json:"slot"`
	Mount MountPoint `json:"mount"`
}

var (
	shellMu       sync.RWMutex
	shellRegistry = map[string]*ProductShellManifest{}
	shellOrder    []string // registration order for deterministic fallbacks
)

// RegisterShell declares a product shell at compile time. Call from init().
// Panics on duplicate or malformed ids to catch copy-paste errors early.
func RegisterShell(m ProductShellManifest) {
	if m.ID == "" {
		panic("appregistry: shell manifest with empty id")
	}
	shellMu.Lock()
	defer shellMu.Unlock()
	if _, dup := shellRegistry[m.ID]; dup {
		panic(fmt.Sprintf("appregistry: duplicate shell id %q", m.ID))
	}
	cp := m
	shellRegistry[m.ID] = &cp
	shellOrder = append(shellOrder, m.ID)
}

func init() {
	// The three v1.0.0 product shells (design §8). All start enabled;
	// tenants disable what they do not use. personal is registered first so
	// it is the fallback default shell.
	RegisterShell(ProductShellManifest{
		ID:          ShellPersonal,
		Name:        "个人工作台",
		Version:     "1.0.0",
		Description: "跨领域管理自己的工作、会话、文件、Agent、待审批和异常",
		Icon:        "home",
		Enabled:     true,
	})
	RegisterShell(ProductShellManifest{
		ID:          ShellPresales,
		Name:        "售前与交付",
		Version:     "1.0.0",
		Description: "从线索证据推进到方案、建设和验收",
		Icon:        "briefcase",
		Enabled:     true,
	})
	RegisterShell(ProductShellManifest{
		ID:          ShellCommerce,
		Name:        "电商运营",
		Version:     "1.0.0",
		Description: "推进商品上新内容生产和发布",
		Icon:        "shopping-bag",
		Enabled:     true,
	})
}

// ListShells returns all registered shells in registration order with the
// effective enabled state for the tenant (persisted intent merged with the
// manifest default).
func ListShells(tenantID string) []ProductShellManifest {
	states := loadShellStates(tenantID)
	shellMu.RLock()
	defer shellMu.RUnlock()
	out := make([]ProductShellManifest, 0, len(shellOrder))
	for _, id := range shellOrder {
		m := *shellRegistry[id]
		if v, ok := states[id]; ok {
			m.Enabled = v
		}
		out = append(out, m)
	}
	return out
}

// GetShell returns one shell manifest with its effective enabled state for
// the tenant. ok=false when the id is not registered.
func GetShell(tenantID, id string) (ProductShellManifest, bool) {
	shellMu.RLock()
	m, ok := shellRegistry[id]
	shellMu.RUnlock()
	if !ok {
		return ProductShellManifest{}, false
	}
	cp := *m
	if v, present := loadShellStates(tenantID)[id]; present {
		cp.Enabled = v
	}
	return cp, true
}

// SetShellEnabled persists the tenant's enable/disable intent for a shell.
//
// Disabling a shell NEVER deletes data: it only flips the persisted flag.
// The product profile (default shell, user preferences) and every app's
// domain data are untouched, and re-enabling restores the previous state.
func SetShellEnabled(tenantID, id string, enabled bool) error {
	shellMu.RLock()
	_, ok := shellRegistry[id]
	shellMu.RUnlock()
	if !ok {
		return fmt.Errorf("appregistry: unknown shell %q", id)
	}
	db, err := meta.OpenDefault()
	if err != nil {
		return fmt.Errorf("appregistry: open db: %w", err)
	}
	if err := ensureShellTables(db.SQL()); err != nil {
		return fmt.Errorf("appregistry: ensure shell tables: %w", err)
	}
	_, err = db.SQL().Exec(
		`INSERT INTO shell_state (tenant_id, shell_id, enabled) VALUES (?, ?, ?)
		 ON CONFLICT(tenant_id, shell_id) DO UPDATE SET enabled=excluded.enabled`,
		tenantID, id, boolToInt(enabled),
	)
	return err
}

// SetDefaultShell persists the tenant's default shell in the product
// profile. The shell must be registered and currently enabled for the tenant
// (a disabled shell cannot be the product entry point).
func SetDefaultShell(tenantID, shellID string) error {
	m, ok := GetShell(tenantID, shellID)
	if !ok {
		return fmt.Errorf("appregistry: unknown shell %q", shellID)
	}
	if !m.Enabled {
		return fmt.Errorf("appregistry: shell %q is disabled; enable it before making it the default", shellID)
	}
	db, err := meta.OpenDefault()
	if err != nil {
		return fmt.Errorf("appregistry: open db: %w", err)
	}
	if err := ensureShellTables(db.SQL()); err != nil {
		return fmt.Errorf("appregistry: ensure shell tables: %w", err)
	}
	_, err = db.SQL().Exec(
		`INSERT INTO product_profile (tenant_id, default_shell) VALUES (?, ?)
		 ON CONFLICT(tenant_id) DO UPDATE SET default_shell=excluded.default_shell`,
		tenantID, shellID,
	)
	return err
}

// DefaultShell returns the tenant's persisted default shell ("" when unset).
func DefaultShell(tenantID string) string {
	db, err := meta.OpenDefault()
	if err != nil {
		return ""
	}
	if err := ensureShellTables(db.SQL()); err != nil {
		return ""
	}
	var shell string
	err = db.SQL().QueryRow(
		`SELECT default_shell FROM product_profile WHERE tenant_id = ?`, tenantID,
	).Scan(&shell)
	if err != nil {
		return ""
	}
	return shell
}

// SetUserPreferredShell persists a user's preferred shell. The shell must be
// registered; pass "" to clear the preference. The preference may point at a
// shell the tenant currently disabled — it is kept (disabling deletes no
// data) and simply ignored by EffectiveDefaultShell until the tenant
// re-enables the shell ("user preferences override the tenant default only
// within the allowed range").
func SetUserPreferredShell(tenantID, userID, shellID string) error {
	if shellID != "" {
		shellMu.RLock()
		_, ok := shellRegistry[shellID]
		shellMu.RUnlock()
		if !ok {
			return fmt.Errorf("appregistry: unknown shell %q", shellID)
		}
	}
	db, err := meta.OpenDefault()
	if err != nil {
		return fmt.Errorf("appregistry: open db: %w", err)
	}
	if err := ensureShellTables(db.SQL()); err != nil {
		return fmt.Errorf("appregistry: ensure shell tables: %w", err)
	}
	_, err = db.SQL().Exec(
		`INSERT INTO shell_user_preference (tenant_id, user_id, preferred_shell) VALUES (?, ?, ?)
		 ON CONFLICT(tenant_id, user_id) DO UPDATE SET preferred_shell=excluded.preferred_shell`,
		tenantID, userID, shellID,
	)
	return err
}

// UserPreferredShell returns the user's persisted shell preference
// ("" when unset).
func UserPreferredShell(tenantID, userID string) string {
	db, err := meta.OpenDefault()
	if err != nil {
		return ""
	}
	if err := ensureShellTables(db.SQL()); err != nil {
		return ""
	}
	var shell string
	err = db.SQL().QueryRow(
		`SELECT preferred_shell FROM shell_user_preference WHERE tenant_id = ? AND user_id = ?`,
		tenantID, userID,
	).Scan(&shell)
	if err != nil {
		return ""
	}
	return shell
}

// EffectiveDefaultShell resolves the shell the user lands on:
//
//  1. the user's preferred shell — when set, registered AND enabled for the
//     tenant (the "allowed range");
//  2. the tenant's default shell — when enabled;
//  3. the first enabled shell in registration order;
//  4. "" when no shell is enabled.
func EffectiveDefaultShell(tenantID, userID string) string {
	shells := ListShells(tenantID)
	enabled := make(map[string]bool, len(shells))
	for _, s := range shells {
		enabled[s.ID] = s.Enabled
	}
	if pref := UserPreferredShell(tenantID, userID); pref != "" && enabled[pref] {
		return pref
	}
	if def := DefaultShell(tenantID); def != "" && enabled[def] {
		return def
	}
	for _, s := range shells {
		if s.Enabled {
			return s.ID
		}
	}
	return ""
}

// ComposeShell returns the app mount points visible in shellID for userID:
// effectively-enabled apps only, shell targeting applied (mounts without a
// shells list appear in every shell — legacy behavior), permission-denied
// mounts dropped, sorted by slot then order (stable within equal keys).
//
// Errors: unknown shell, or a shell the tenant disabled (composition of a
// disabled shell is refused, but nothing is deleted).
func ComposeShell(tenantID, userID, shellID string) ([]ComposedMount, error) {
	shell, ok := GetShell(tenantID, shellID)
	if !ok {
		return nil, fmt.Errorf("appregistry: unknown shell %q", shellID)
	}
	if !shell.Enabled {
		return nil, fmt.Errorf("appregistry: shell %q is disabled", shellID)
	}

	var out []ComposedMount
	for _, app := range List() { // List() already merges capability validation
		if !app.Enabled {
			continue
		}
		for _, mp := range app.MountPoints {
			if !mountTargetsShell(mp, shellID) {
				continue
			}
			if !permissionAllowed(userID, mp.Permission) {
				continue
			}
			out = append(out, ComposedMount{
				AppID: app.ID,
				Slot:  resolvedSlot(mp),
				Mount: mp,
			})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Slot != out[j].Slot {
			return out[i].Slot < out[j].Slot
		}
		return out[i].Mount.Order < out[j].Mount.Order
	})
	return out, nil
}

// mountTargetsShell reports whether mp contributes to shellID. An empty
// shells list targets every enabled shell (legacy mount points keep their
// existing rendering).
func mountTargetsShell(mp MountPoint, shellID string) bool {
	if len(mp.Shells) == 0 {
		return true
	}
	for _, s := range mp.Shells {
		if s == shellID {
			return true
		}
	}
	return false
}

// resolvedSlot returns the mount's declared slot, falling back to its type
// so legacy mounts stay in the placement zones the frontend already renders.
func resolvedSlot(mp MountPoint) string {
	if mp.Slot != "" {
		return mp.Slot
	}
	return mp.Type
}

// ── HTTP handlers ─────────────────────────────────────────────────────────

// HandleShellList handles GET /api/shells →
// {shells:[...], tenant, defaultShell, userPreference, effectiveDefault}.
func HandleShellList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tenant := requestTenant(r)
	user := requestUserID(r)
	writeJSON(w, map[string]any{
		"shells":           ListShells(tenant),
		"tenant":           tenant,
		"defaultShell":     DefaultShell(tenant),
		"userPreference":   UserPreferredShell(tenant, user),
		"effectiveDefault": EffectiveDefaultShell(tenant, user),
	})
}

// HandleShellEnable handles POST /api/shells/{id}/enable.
func HandleShellEnable(w http.ResponseWriter, r *http.Request) {
	shellEnableHandler(w, r, "/enable", true)
}

// HandleShellDisable handles POST /api/shells/{id}/disable.
func HandleShellDisable(w http.ResponseWriter, r *http.Request) {
	shellEnableHandler(w, r, "/disable", false)
}

func shellEnableHandler(w http.ResponseWriter, r *http.Request, suffix string, enabled bool) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := extractShellID(r.URL.Path, suffix)
	if id == "" {
		http.Error(w, "missing shell id", http.StatusBadRequest)
		return
	}
	if err := SetShellEnabled(requestTenant(r), id, enabled); err != nil {
		log.Printf("[appregistry] shell %s %s: %v", id, strings.TrimPrefix(suffix, "/"), err)
		if strings.Contains(err.Error(), "unknown shell") {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "id": id, "enabled": enabled})
}

// HandleShellDefault handles PUT /api/shells/default {"shell": "...", "tenant": "..."}
// (tenant optional; defaults to DefaultTenantID).
func HandleShellDefault(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Shell  string `json:"shell"`
		Tenant string `json:"tenant"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	tenant := body.Tenant
	if tenant == "" {
		tenant = requestTenant(r)
	}
	if err := SetDefaultShell(tenant, body.Shell); err != nil {
		log.Printf("[appregistry] set default shell: %v", err)
		if strings.Contains(err.Error(), "unknown shell") {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "tenant": tenant, "defaultShell": body.Shell})
}

// HandleShellPreference handles PUT /api/shells/preference
// {"shell": "...", "user": "..."} — "" clears the preference.
func HandleShellPreference(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Shell  string `json:"shell"`
		User   string `json:"user"`
		Tenant string `json:"tenant"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	tenant := body.Tenant
	if tenant == "" {
		tenant = requestTenant(r)
	}
	user := body.User
	if user == "" {
		user = requestUserID(r)
	}
	if err := SetUserPreferredShell(tenant, user, body.Shell); err != nil {
		log.Printf("[appregistry] set shell preference: %v", err)
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]any{
		"ok": true, "tenant": tenant, "user": user, "preferredShell": body.Shell,
		"effectiveDefault": EffectiveDefaultShell(tenant, user),
	})
}

// HandleShellComposition handles GET /api/shells/composition?shell=<id> →
// {shell, mounts:[{appId, slot, mount}]}.
func HandleShellComposition(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	shellID := r.URL.Query().Get("shell")
	if shellID == "" {
		http.Error(w, "missing shell query parameter", http.StatusBadRequest)
		return
	}
	tenant := requestTenant(r)
	mounts, err := ComposeShell(tenant, requestUserID(r), shellID)
	if err != nil {
		if strings.Contains(err.Error(), "unknown shell") {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if mounts == nil {
		mounts = []ComposedMount{}
	}
	writeJSON(w, map[string]any{"shell": shellID, "tenant": tenant, "mounts": mounts})
}

// ── internals ─────────────────────────────────────────────────────────────

func ensureShellTables(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS shell_state (
		tenant_id TEXT NOT NULL,
		shell_id  TEXT NOT NULL,
		enabled   INTEGER NOT NULL DEFAULT 1,
		PRIMARY KEY (tenant_id, shell_id)
	)`)
	if err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS product_profile (
		tenant_id     TEXT PRIMARY KEY,
		default_shell TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS shell_user_preference (
		tenant_id       TEXT NOT NULL,
		user_id         TEXT NOT NULL,
		preferred_shell TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (tenant_id, user_id)
	)`)
	return err
}

func loadShellStates(tenantID string) map[string]bool {
	states := map[string]bool{}
	db, err := meta.OpenDefault()
	if err != nil {
		return states
	}
	if err := ensureShellTables(db.SQL()); err != nil {
		return states
	}
	rows, err := db.SQL().Query(
		`SELECT shell_id, enabled FROM shell_state WHERE tenant_id = ?`, tenantID,
	)
	if err != nil {
		return states
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var en int
		if err := rows.Scan(&id, &en); err == nil {
			states[id] = en != 0
		}
	}
	return states
}

// requestTenant returns the tenant scope: ?tenant= when present,
// DefaultTenantID otherwise (C0 is single-tenant).
func requestTenant(r *http.Request) string {
	if t := r.URL.Query().Get("tenant"); t != "" {
		return t
	}
	return DefaultTenantID
}

// extractShellID strips prefix and suffix from /api/shells/{id}/enable etc.
func extractShellID(path, suffix string) string {
	path = strings.TrimPrefix(path, "/api/shells/")
	path = strings.TrimSuffix(path, suffix)
	return strings.Trim(path, "/")
}
