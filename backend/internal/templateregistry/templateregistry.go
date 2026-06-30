// Package templateregistry is the project template mechanism. A template
// binds an app to a scaffolded workspace: it declares which subdirectories to
// create, which domain DDLs to run, and what project_config to seed.
//
// Wave 3 apps register templates from their init(). The project-creation
// endpoint in the workspace handler calls Scaffold when a templateId is
// supplied.
//
// Design constraints:
//   - MUST NOT import app code. Apps import this package.
//   - Registration is done at compile time via init(); no runtime discovery.
//   - Scaffold is the single entry point for creating a project from a template.
package templateregistry

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/scottzx/1Agents/backend/internal/appregistry"
	"github.com/scottzx/1Agents/backend/internal/domainstore"
	"github.com/scottzx/1Agents/backend/internal/meta"
)

// ProjectTemplate defines a project scaffold.
type ProjectTemplate struct {
	// ID is the stable template identifier, e.g. "media.content_project".
	ID string `json:"id"`
	// Name is the display name shown in the creation dialog.
	Name string `json:"name"`
	// AppID is the owning app's manifest id. The app must be registered.
	AppID string `json:"appId"`
	// Subdirs is the list of artifact subdirectories to create under
	// <workspace>/.artifacts/<AppID>/. E.g. ["素材库", "剪辑", "发布"].
	Subdirs []string `json:"subdirs"`
	// DomainDDL is the list of CREATE TABLE IF NOT EXISTS statements to run.
	// Each must be prefixed with AppID + "_".
	DomainDDL []string `json:"domainDdl"`
	// PresetConfig is the initial ProjectConfig seeded into the new project.
	PresetConfig ProjectConfig `json:"presetConfig"`
}

// ProjectConfig is the per-project agent context (§327). Stored in the
// project_config table. Instructions is the system prompt; Connectors is
// the MCP list; Experts, Skills, Automation are future extension points.
type ProjectConfig struct {
	Instructions string   `json:"instructions"`
	Connectors   []string `json:"connectors"`
	Experts      []string `json:"experts"`
	Skills       []string `json:"skills"`
	Automation   []string `json:"automation"`
}

var (
	mu       sync.RWMutex
	registry = map[string]*ProjectTemplate{}
	order    []string
)

// Register declares a project template. Call from app init(). Panics on
// duplicate ID.
func Register(t ProjectTemplate) {
	mu.Lock()
	defer mu.Unlock()
	if _, dup := registry[t.ID]; dup {
		panic(fmt.Sprintf("templateregistry: duplicate template id %q", t.ID))
	}
	cp := t
	registry[t.ID] = &cp
	order = append(order, t.ID)
}

// List returns all registered templates in registration order.
func List() []ProjectTemplate {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]ProjectTemplate, 0, len(order))
	for _, id := range order {
		out = append(out, *registry[id])
	}
	return out
}

// Get returns a template by id. ok=false when not found.
func Get(id string) (ProjectTemplate, bool) {
	mu.RLock()
	defer mu.RUnlock()
	t, ok := registry[id]
	if !ok {
		return ProjectTemplate{}, false
	}
	return *t, true
}

// ScaffoldResult is returned by Scaffold.
type ScaffoldResult struct {
	// ArtifactDirs lists the directories created under the workspace.
	ArtifactDirs []string
}

// Scaffold applies a template to a newly created workspace:
//  1. Ensures the app's domain tables exist in meta.db.
//  2. Creates the subdirectory skeleton under <workspacePath>/.artifacts/<appID>/.
//  3. Seeds project_config with the template's preset.
//
// workspacePath must already exist as a directory. projectID is the projects
// table id for the new project.
func Scaffold(templateID, projectID, workspacePath string) (ScaffoldResult, error) {
	t, ok := Get(templateID)
	if !ok {
		return ScaffoldResult{}, fmt.Errorf("templateregistry: unknown template %q", templateID)
	}

	// Verify the owning app is registered (not necessarily enabled; scaffolding
	// a disabled app's template is a dev-time operation).
	if _, appOK := appregistry.Get(t.AppID); !appOK {
		return ScaffoldResult{}, fmt.Errorf("templateregistry: app %q (owner of template %q) is not registered", t.AppID, templateID)
	}

	// 1. Ensure domain tables.
	if len(t.DomainDDL) > 0 {
		if err := appregistry.EnsureDomainTables(t.AppID, t.DomainDDL); err != nil {
			return ScaffoldResult{}, fmt.Errorf("templateregistry: ensure domain tables: %w", err)
		}
	}

	// 2. Create artifact subdirectories.
	var created []string
	for _, sub := range t.Subdirs {
		dir, err := domainstore.ArtifactDir(workspacePath, t.AppID, sub)
		if err != nil {
			return ScaffoldResult{}, fmt.Errorf("templateregistry: create subdir %q: %w", sub, err)
		}
		created = append(created, dir)
	}

	// 3. Seed project_config.
	if err := SaveProjectConfig(projectID, t.PresetConfig); err != nil {
		return ScaffoldResult{}, fmt.Errorf("templateregistry: seed project config: %w", err)
	}

	return ScaffoldResult{ArtifactDirs: created}, nil
}

// ── Project config (§327) ─────────────────────────────────────────────────

// ensureProjectConfigTable creates the project_config table idempotently.
func ensureProjectConfigTable(db *meta.DB) error {
	_, err := db.SQL().Exec(`CREATE TABLE IF NOT EXISTS project_config (
		project_id   TEXT PRIMARY KEY,
		instructions TEXT NOT NULL DEFAULT '',
		connectors   TEXT NOT NULL DEFAULT '[]',
		experts      TEXT NOT NULL DEFAULT '[]',
		skills       TEXT NOT NULL DEFAULT '[]',
		automation   TEXT NOT NULL DEFAULT '[]',
		updated_at   TEXT NOT NULL DEFAULT ''
	)`)
	return err
}

// LoadProjectConfig reads a project's agent context config. Returns a zero
// value (all-empty) config when no row exists yet, which is a valid state
// (the project just hasn't been configured).
func LoadProjectConfig(projectID string) (ProjectConfig, error) {
	db, err := meta.OpenDefault()
	if err != nil {
		return ProjectConfig{}, fmt.Errorf("project config: open db: %w", err)
	}
	if err := ensureProjectConfigTable(db); err != nil {
		return ProjectConfig{}, fmt.Errorf("project config: ensure table: %w", err)
	}
	row := db.SQL().QueryRow(
		`SELECT instructions, connectors, experts, skills, automation
		 FROM project_config WHERE project_id = ?`, projectID)
	var (
		instructions string
		connectors   string
		experts      string
		skills       string
		automation   string
	)
	err = row.Scan(&instructions, &connectors, &experts, &skills, &automation)
	if err != nil {
		// No row → return zero config (not an error).
		return ProjectConfig{}, nil
	}
	cfg := ProjectConfig{Instructions: instructions}
	_ = json.Unmarshal([]byte(connectors), &cfg.Connectors)
	_ = json.Unmarshal([]byte(experts), &cfg.Experts)
	_ = json.Unmarshal([]byte(skills), &cfg.Skills)
	_ = json.Unmarshal([]byte(automation), &cfg.Automation)
	return cfg, nil
}

// SaveProjectConfig persists a project's agent context config. Upserts the row.
func SaveProjectConfig(projectID string, cfg ProjectConfig) error {
	db, err := meta.OpenDefault()
	if err != nil {
		return fmt.Errorf("project config: open db: %w", err)
	}
	if err := ensureProjectConfigTable(db); err != nil {
		return fmt.Errorf("project config: ensure table: %w", err)
	}
	connectors, _ := json.Marshal(cfg.Connectors)
	experts, _ := json.Marshal(cfg.Experts)
	skills, _ := json.Marshal(cfg.Skills)
	automation, _ := json.Marshal(cfg.Automation)

	_, err = db.SQL().Exec(
		`INSERT INTO project_config
			(project_id, instructions, connectors, experts, skills, automation, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, datetime('now'))
		 ON CONFLICT(project_id) DO UPDATE SET
			instructions = excluded.instructions,
			connectors   = excluded.connectors,
			experts      = excluded.experts,
			skills       = excluded.skills,
			automation   = excluded.automation,
			updated_at   = excluded.updated_at`,
		projectID,
		cfg.Instructions,
		string(connectors),
		string(experts),
		string(skills),
		string(automation),
	)
	return err
}

// ── HTTP handlers ──────────────────────────────────────────────────────────

// HandleList handles GET /api/templates → {templates: [...]}
func HandleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{"templates": List()})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[templateregistry] json encode: %v", err)
	}
}
