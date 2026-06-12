package meta

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// WorkspaceRef is the minimal slice of the workspace registry the migration
// needs (id, display name, absolute path).
type WorkspaceRef struct {
	ID   string
	Name string
	Path string
}

// MigrateLegacy performs the one-time import from the legacy JSON stores:
//
//  1. every registered workspace becomes a projects row (id = workspace id)
//  2. ~/.1agents/agent-sessions.json   → sessions table
//  3. <workspace>/.1agents/tasks.json  → tasks/replies/deps tables
//
// Imported files are renamed to *.migrated (kept as a fallback, never
// deleted). Idempotent: rerunning is a no-op once the files are renamed,
// and inserts use OR IGNORE so existing rows are never clobbered.
func (db *DB) MigrateLegacy(workspaces []WorkspaceRef) error {
	for _, ws := range workspaces {
		if err := db.EnsureProject(ws.ID, ws.Name, ws.Path); err != nil {
			return fmt.Errorf("meta: ensure project %s: %w", ws.ID, err)
		}
	}

	if err := db.importLegacySessions(); err != nil {
		return err
	}

	store := NewTaskStore(db)
	for _, ws := range workspaces {
		if err := store.maybeImportLegacy(ws.Path); err != nil {
			log.Printf("[meta] tasks import for %s failed: %v", ws.Path, err)
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
