package meta

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// WorkspaceRef is the minimal slice of the workspace registry (id, display
// name, absolute path). Aliased by internal/agent and used by the scheduler's
// workspace-enumeration callback.
type WorkspaceRef struct {
	ID   string
	Name string
	Path string
}

// legacyWorkspace mirrors a workspaces_dir.json entry. Declared locally so the
// importer stays independent of the workspace package (which imports meta).
type legacyWorkspace struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Path         string `json:"path"`
	Status       string `json:"status"`
	TerminalDir  string `json:"terminalDir"`
	ChatChannel  string `json:"chatChannel"`
	DefaultAgent string `json:"defaultAgent"`
	Builtin      bool   `json:"builtin"`
}

// ImportLegacyWorkspaces performs the one-time unification of
// ~/.1agents/workspaces_dir.json into the projects table — making projects the
// single source of truth for the sidebar/workspace API. Each entry becomes a
// workspace-backed project row with position = its json array index (preserving
// sidebar order). The json is renamed to *.migrated afterward, so it never
// re-imports and survives as a manual fallback. No-op once the file is gone.
//
// Existing rows (e.g. created by a prior boot) are upserted: their archived
// status is preserved (upsertWorkspaceProject leaves status on conflict), while
// the absorbed workspace fields and position are refreshed from the json.
func (db *DB) ImportLegacyWorkspaces() error {
	legacy := filepath.Join(get1AgentsHome(), ".1agents", "workspaces_dir.json")
	data, err := os.ReadFile(legacy)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var cfg struct {
		Workspaces []legacyWorkspace `json:"workspaces"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("meta: parse legacy %s: %w", legacy, err)
	}
	for i, ws := range cfg.Workspaces {
		if ws.ID == "" {
			continue
		}
		if err := db.upsertWorkspaceProject(Project{
			ID:            ws.ID,
			Name:          ws.Name,
			WorkspacePath: ws.Path,
			TerminalDir:   ws.TerminalDir,
			ChatChannel:   ws.ChatChannel,
			DefaultAgent:  ws.DefaultAgent,
			Builtin:       ws.Builtin,
		}, i); err != nil {
			return fmt.Errorf("meta: import workspace %s: %w", ws.ID, err)
		}
	}
	if err := os.Rename(legacy, legacy+".migrated"); err != nil {
		return err
	}
	log.Printf("[meta] imported %d workspaces from %s into projects", len(cfg.Workspaces), legacy)
	return nil
}

// MigrateLegacy performs the one-time import of the remaining legacy JSON stores:
//
//  1. ~/.1agents/agent-sessions.json   → sessions table
//  2. <workspace>/.1agents/tasks.json  → tasks/replies/deps tables
//
// Workspace registry import now lives in ImportLegacyWorkspaces, which must run
// first so the per-workspace task paths are readable from the projects table.
// Imported files are renamed to *.migrated (kept as a fallback). Idempotent:
// rerunning is a no-op once the files are renamed.
func (db *DB) MigrateLegacy() error {
	if err := db.importLegacySessions(); err != nil {
		return err
	}

	projects, err := db.ListWorkspaceProjects()
	if err != nil {
		return err
	}
	store := NewTaskStore(db)
	for _, p := range projects {
		if err := store.maybeImportLegacy(p.WorkspacePath); err != nil {
			log.Printf("[meta] tasks import for %s failed: %v", p.WorkspacePath, err)
		}
	}
	return nil
}

// importLegacySessions imports ~/.1agents/agent-sessions.json once.
func (db *DB) importLegacySessions() error {
	legacy := filepath.Join(get1AgentsHome(), ".1agents", "agent-sessions.json")
	data, err := os.ReadFile(legacy)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var cfg struct {
		Sessions []ChatSessionRecord `json:"sessions"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("meta: parse legacy %s: %w", legacy, err)
	}

	for _, rec := range cfg.Sessions {
		if rec.ID == "" {
			continue
		}
		if _, err := db.sql.Exec(`
			INSERT OR IGNORE INTO sessions (id, project_id, task_id, name, agent_type,
				cc_project, cc_session_id, acp_session_id, session_key,
				permission_mode, created_at, last_event_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			rec.ID, rec.WorkspaceID, rec.TaskID, rec.Name, rec.AgentType,
			rec.CcProject, rec.CcSessionID, rec.AcpSessionID, rec.SessionKey,
			rec.PermissionMode, timeToStr(rec.CreatedAt), timeToStr(rec.LastEventAt)); err != nil {
			return err
		}
	}
	if err := os.Rename(legacy, legacy+".migrated"); err != nil {
		return err
	}
	log.Printf("[meta] imported %d legacy chat sessions from %s", len(cfg.Sessions), legacy)
	return nil
}
