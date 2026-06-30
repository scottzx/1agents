package system

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/scottzx/1Agents/backend/internal/feishu"
	"github.com/scottzx/1Agents/backend/internal/meta"
)

// reset.go implements POST /api/system/reset — the settings "重置数据" action.
//
// It clears all local App data (tasks/projects/sessions/inbox/digest in meta.db,
// synced chats in sync.db, and the on-disk knowledge base + scratch files) while
// deliberately PRESERVING the relay pairing identity:
//   - ~/.happy/ (E2EE access.key + daemon state) is never touched.
//   - ~/.1agents/relay-creds.json (client account master key, #109) is kept.
//   - ~/.1agents/daemon.json is kept.
//
// DB tables are cleared in place (DELETE FROM, schema/user_version intact) rather
// than deleting the .db files, so the long-lived server keeps using the same
// connections. The workspace-backed projects in ~/.cc-connect/config.toml are
// also purged, otherwise the boot-time workspace↔cc-connect sync reflows the
// wiped projects back. After clearing, the default workspace is re-seeded via the
// injected reseedDefault hook so the App never boots into a broken empty state.

// ResetSummary is the JSON returned to the frontend describing what was wiped
// and what was deliberately preserved.
type ResetSummary struct {
	OK             bool     `json:"ok"`
	ClearedTables  []string `json:"clearedTables"`
	ClearedSync    bool     `json:"clearedSync"`
	RemovedPaths   []string `json:"removedPaths"`
	PurgedProjects int      `json:"purgedCcProjects"`
	Preserved      []string `json:"preserved"`
	DefaultSeeded  bool     `json:"defaultWorkspaceReseeded"`
}

// ResetHandler builds the POST /api/system/reset handler. reseedDefault re-seeds
// the built-in default workspace after the wipe (wired to
// workspace.Handler.EnsureDefaultWorkspace in server.go). purgeCCProjects strips
// the workspace-backed projects from ~/.cc-connect/config.toml so the boot-time
// sync can't reflow the wiped projects back (wired to
// ccconnect.PurgeWorkspaceProjects). Either hook may be nil in tests.
func (h *Handler) ResetHandler(reseedDefault func() error, purgeCCProjects func() (int, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		summary, err := runReset(reseedDefault, purgeCCProjects)
		if err != nil {
			jsonError(w, "reset failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, summary)
	}
}

// runReset performs the wipe and returns a summary. Split out from the handler
// so tests can drive it directly with ONEAGENTS_HOME pointed at a temp dir.
func runReset(reseedDefault func() error, purgeCCProjects func() (int, error)) (ResetSummary, error) {
	summary := ResetSummary{
		Preserved: []string{"~/.happy", "relay-creds.json", "daemon.json"},
	}

	// ── meta.db: clear all data tables in place ──────────────────────────────
	db, err := meta.OpenDefault()
	if err != nil {
		return summary, err
	}
	cleared, err := db.ClearAllData()
	if err != nil {
		return summary, err
	}
	summary.ClearedTables = cleared

	// ── sync.db (Feishu digest): clear its tables in place ───────────────────
	if fs, err := feishu.OpenDefault(); err == nil {
		if err := fs.ClearAllData(); err != nil {
			return summary, err
		}
		summary.ClearedSync = true
	}

	// ── file storage ─────────────────────────────────────────────────────────
	home := oneAgentsHome() // ~/.1agents (honors ONEAGENTS_HOME)

	// Directories: empty their contents but keep the directory itself.
	for _, dir := range []string{
		filepath.Join(home, "knowledge"),           // kwiki raw/wiki/output
		filepath.Join(home, "acpx-state"),          // ACP engine state
		filepath.Join(home, "projects", "default"), // default workspace contents
	} {
		if emptyDir(dir) {
			summary.RemovedPaths = append(summary.RemovedPaths, dir)
		}
	}

	// Files: remove outright.
	for _, f := range []string{
		filepath.Join(home, "devices.json"),
		filepath.Join(home, "session_names.json"),
	} {
		if err := os.Remove(f); err == nil {
			summary.RemovedPaths = append(summary.RemovedPaths, f)
		} else if !os.IsNotExist(err) {
			return summary, err
		}
	}

	// ── purge workspace-backed cc-connect projects ──────────────────────────
	// Without this, the boot-time workspace↔cc-connect sync re-imports every
	// config.toml [[projects]] entry with a work_dir back into the registry,
	// undoing the meta.db wipe. Provider/model/platform config is preserved.
	if purgeCCProjects != nil {
		n, err := purgeCCProjects()
		if err != nil {
			return summary, err
		}
		summary.PurgedProjects = n
	}

	// ── re-seed default workspace so the App is never broken-empty ───────────
	if reseedDefault != nil {
		if err := reseedDefault(); err != nil {
			return summary, err
		}
		summary.DefaultSeeded = true
	}

	summary.OK = true
	return summary, nil
}

// emptyDir removes everything inside dir (but not dir itself). Returns true if
// dir existed and was emptied. A missing dir is a no-op returning false.
func emptyDir(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false // missing or unreadable — nothing to clear
	}
	for _, e := range entries {
		_ = os.RemoveAll(filepath.Join(dir, e.Name()))
	}
	return true
}
