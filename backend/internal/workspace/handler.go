package workspace

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/chenhg5/cc-connect/config"
	"github.com/chenhg5/cc-connect/core"
	"github.com/scottzx/1Agents/backend/internal/meta"
)

func get1AgentsHome() string {
	if val := os.Getenv("ONEAGENTS_HOME"); val != "" {
		return val
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return home
}

// Workspace represents a single workspace entry.
type Workspace struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Path         string `json:"path"`
	Status       string `json:"status"`
	TerminalDir  string `json:"terminalDir,omitempty"`
	ChatChannel  string `json:"chatChannel,omitempty"`
	DefaultAgent string `json:"defaultAgent,omitempty"`
	Builtin      bool   `json:"builtin,omitempty"`
	// AvailableAgents is the allowlist of agent type slugs that may run in
	// this workspace (§325). Empty means unrestricted.
	AvailableAgents []string `json:"availableAgents,omitempty"`
	// Kind: "assistant" | "project" — see meta.Project.Kind.
	Kind string `json:"kind,omitempty"`
	// Avatar: image URL under /avatars/ (preset or upload); empty means unset.
	Avatar string `json:"avatar,omitempty"`
}

// WorkspacesConfig is the top-level workspace registry shape. Its persistence
// has moved from workspaces_dir.json into meta.db's projects table (a project IS
// a workspace); the struct is kept for API/JSON compatibility with the many
// callers of LoadWorkspacesConfig.
type WorkspacesConfig struct {
	Workspaces []Workspace `json:"workspaces"`
}

type Handler struct {
	tmuxSession string
	skillsAddr  string // 1skills FastAPI addr for skill push-back; empty → defaultSkillsAddr
}

func NewHandler(tmuxSession ...string) *Handler {
	session := ""
	if len(tmuxSession) > 0 {
		session = tmuxSession[0]
	}
	return &Handler{tmuxSession: session}
}

// SetSkillsAddr pins the 1skills service address used by PushSkill (defaults to
// defaultSkillsAddr when unset). Called once at server startup with the resolved
// config so the push-back forwards to the same instance the supervisor launched.
func (h *Handler) SetSkillsAddr(addr string) {
	if addr != "" {
		h.skillsAddr = addr
	}
}

// projectToWorkspace maps a meta project row to the workspace registry shape.
func projectToWorkspace(p meta.Project) Workspace {
	return Workspace{
		ID:              p.ID,
		Name:            p.Name,
		Path:            p.WorkspacePath,
		Status:          string(p.Status),
		TerminalDir:     p.TerminalDir,
		ChatChannel:     p.ChatChannel,
		DefaultAgent:    p.DefaultAgent,
		Builtin:         p.Builtin,
		AvailableAgents: p.AvailableAgents,
		Kind:            p.Kind,
		Avatar:          p.Avatar,
	}
}

// workspaceToProject maps a workspace into a meta project (write side).
func workspaceToProject(ws Workspace) meta.Project {
	return meta.Project{
		ID:              ws.ID,
		Name:            ws.Name,
		WorkspacePath:   ws.Path,
		TerminalDir:     ws.TerminalDir,
		ChatChannel:     ws.ChatChannel,
		DefaultAgent:    ws.DefaultAgent,
		Builtin:         ws.Builtin,
		AvailableAgents: ws.AvailableAgents,
		Kind:            ws.Kind,
		Avatar:          ws.Avatar,
	}
}

// loadConfig returns every workspace-backed project (any status, excluding the
// reserved personal bucket) in sidebar order — the faithful replacement for
// reading workspaces_dir.json. Status filtering (e.g. hiding archived projects
// from the sidebar) is applied by callers such as the List handler.
func (h *Handler) loadConfig() (*WorkspacesConfig, error) {
	db, err := meta.OpenDefault()
	if err != nil {
		return nil, err
	}
	projects, err := db.ListWorkspaceProjects()
	if err != nil {
		return nil, err
	}
	wss := make([]Workspace, 0, len(projects))
	for _, p := range projects {
		wss = append(wss, projectToWorkspace(p))
	}
	return &WorkspacesConfig{Workspaces: wss}, nil
}

// LoadWorkspacesConfig loads the workspace registry from meta.db. Returns all
// non-personal projects regardless of status (matching the legacy json), so
// id→path resolution still works for archived projects.
func (h *Handler) LoadWorkspacesConfig() (*WorkspacesConfig, error) {
	return h.loadConfig()
}

// SaveWorkspacesConfig reconciles the projects table to the given registry:
// upserts each workspace, deletes non-personal projects no longer present, and
// rewrites positions to match the given order. Kept for API compatibility; the
// CRUD handlers below write the table directly.
func (h *Handler) SaveWorkspacesConfig(cfg *WorkspacesConfig) error {
	db, err := meta.OpenDefault()
	if err != nil {
		return err
	}
	keep := make(map[string]bool, len(cfg.Workspaces))
	ids := make([]string, 0, len(cfg.Workspaces))
	for _, ws := range cfg.Workspaces {
		if err := db.EnsureWorkspaceProject(workspaceToProject(ws)); err != nil {
			return err
		}
		keep[ws.ID] = true
		ids = append(ids, ws.ID)
	}
	existing, err := db.ListWorkspaceProjects()
	if err != nil {
		return err
	}
	for _, p := range existing {
		if !keep[p.ID] {
			if err := db.DeleteProject(p.ID); err != nil {
				return err
			}
		}
	}
	return db.ReorderProjects(ids)
}

// defaultAssistantAvatar is the built-in default assistant's avatar — the robot
// from the embedded preset set (workspace/avatar.go).
const defaultAssistantAvatar = "/avatars/presets/preset-8.png"

// EnsureDefaultWorkspace creates the built-in default workspace if it does not
// already exist. Called once at server startup so new installs skip onboarding.
func (h *Handler) EnsureDefaultWorkspace() error {
	db, err := meta.OpenDefault()
	if err != nil {
		return err
	}
	// Self-heal legacy empty-id project rows (from the "_" slug bug) so the
	// sidebar never surfaces a workspace with id="" — which the frontend would
	// turn into GET /api/agent/sessions?workspace_id= → 400.
	if n, err := db.PruneInvalidProjects(); err != nil {
		log.Printf("[workspace] prune invalid projects: %v", err)
	} else if n > 0 {
		log.Printf("[workspace] pruned %d invalid (empty-id) project row(s)", n)
	}
	existing, ok, err := db.GetProject("default")
	if err != nil {
		return err
	}
	if ok {
		// Migrate the legacy default row into the assistant family. Older installs
		// created it with name "对话" and no kind; the sidebar now shows this row as
		// the always-first assistant, so it needs kind='assistant' and a friendlier
		// display name. Users can rename it later.
		needs := existing.Kind != "assistant" || existing.Name == "对话" || existing.Name == "" ||
			existing.Avatar == ""
		if needs {
			existing.Kind = "assistant"
			if existing.Name == "对话" || existing.Name == "" {
				existing.Name = "助理"
			}
			if existing.Avatar == "" {
				existing.Avatar = defaultAssistantAvatar
			}
			if err := db.EnsureWorkspaceProject(existing); err != nil {
				log.Printf("[workspace] migrate default row: %v", err)
			} else {
				log.Printf("[workspace] migrated default row → kind=assistant, name=%q", existing.Name)
			}
		}
		return nil
	}
	homeDir := get1AgentsHome()
	defaultPath := filepath.Join(homeDir, ".1agents", "projects", "default")
	if err := os.MkdirAll(defaultPath, 0o755); err != nil {
		return fmt.Errorf("create default workspace dir: %w", err)
	}
	ensureProjectGuideFiles(defaultPath)
	if err := db.EnsureWorkspaceProject(meta.Project{
		ID:            "default",
		Name:          "助理",
		WorkspacePath: defaultPath,
		DefaultAgent:  "claudecode",
		Builtin:       true,
		Kind:          "assistant",
		Avatar:        defaultAssistantAvatar,
	}); err != nil {
		return err
	}
	log.Printf("[workspace] created built-in default workspace at %s", defaultPath)
	return nil
}

// isNameTaken returns the existing workspace whose display name matches (case-
// insensitive, whitespace-trimmed), excluding the given id (so an update keeps
// its own name). Empty match returns ok=false. Names must be globally unique
// across assistants + projects — the user's ask.
func (h *Handler) isNameTaken(name, excludeID string) (Workspace, bool, error) {
	target := strings.ToLower(strings.TrimSpace(name))
	if target == "" {
		return Workspace{}, false, nil
	}
	cfg, err := h.loadConfig()
	if err != nil {
		return Workspace{}, false, err
	}
	for _, w := range cfg.Workspaces {
		if w.ID == excludeID {
			continue
		}
		if strings.ToLower(strings.TrimSpace(w.Name)) == target {
			return w, true, nil
		}
	}
	return Workspace{}, false, nil
}

// List handles GET /api/workspace/list
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cfg, err := h.loadConfig()
	if err != nil {
		log.Printf("[workspace] load error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Sidebar shows only active workspaces — archived/killed projects (#141) drop
	// out of the registry view while staying queryable by id for the archive board.
	active := make([]Workspace, 0, len(cfg.Workspaces))
	for _, ws := range cfg.Workspaces {
		if ws.Status == "" || ws.Status == string(meta.ProjectStatusActive) {
			active = append(active, ws)
		}
	}
	writeJSON(w, active)
}

// RegisterIncubatedProject wires the cc-connect bridge + guide files for a
// project promoted via 立项 (the project row is already persisted by
// meta.Incubate as a workspace). Passed as the onIncubated hook to
// meta.PersonalTaskItemHandler so promotion gets a working bridge without meta
// importing the workspace package.
func (h *Handler) RegisterIncubatedProject(p meta.Project) {
	h.registerWorkspaceProject(projectToWorkspace(p))
}

// registerWorkspaceProject performs the side-effects of bringing a workspace
// online: agent guidance files + dynamic CC-Connect bridge registration + hot
// restart. Shared by Create and the 立项 (Incubate) hook so a promoted project
// gets a working bridge exactly like a hand-created workspace. Persisting the
// project row is the caller's job; this only does the non-storage side-effects.
func (h *Handler) registerWorkspaceProject(ws Workspace) {
	// Ensure the project has agent guidance files (CLAUDE.md / AGENTS.md).
	ensureProjectGuideFiles(ws.Path)

	// Dynamically register this workspace as a CC-Connect project. The project
	// name must match what the panel route addresses (ccconnect.CCProjectName):
	// the raw name only when it is already slug-safe ascii; otherwise the
	// workspace id (badge / hex — always ascii). Registering the raw "办公2"
	// here while the panel asked for its slug "_2" is exactly the mismatch that
	// 404'd every /api/v1/projects/... call for Chinese-named assistants.
	projName := ccSafeProjectName(ws)
	if config.ConfigPath != "" {
		if err := config.AddPlatformToProject(projName, config.PlatformConfig{
			Type: "bridge",
		}, ws.Path, "claudecode"); err != nil {
			log.Printf("[workspace] ccconnect add project error: %v", err)
		} else {
			log.Printf("[workspace] Dynamically registered CC-Connect project %s at path %s", projName, ws.Path)

			// Trigger cc-connect to hot restart itself and reload the configuration!
			select {
			case core.RestartCh <- core.RestartRequest{}:
				log.Println("[workspace] Successfully requested CC-Connect process hot restart for configuration reload")
			default:
				log.Println("[workspace] CC-Connect hot restart already pending")
			}

			// Wait for the hot restart to finish to avoid 502 Bad Gateway race condition on immediate redirect
			time.Sleep(1000 * time.Millisecond)
		}
	}
}

// Create handles POST /api/workspace/create
// Optional body field templateId: when set, Scaffold is called after the
// project row is persisted (creates artifact subdirs + domain tables + seeds
// project_config). The workspace is created even when Scaffold fails — the
// error is logged and returned in a non-fatal "scaffoldError" response field.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Workspace
		TemplateID string `json:"templateId"`
		// Skills is an optional list of shared-store skill package dirs to weak-copy
		// into <ws>/.claude/skills on creation (#360). Used by the create-assistant
		// flow's skill picker; empty for a plain workspace.
		Skills []string `json:"skills"`
		// Agents is an optional list of shared-store agent file names (<name>.md) to
		// weak-copy into <ws>/.claude/agents on creation. Parallel to Skills; empty
		// for a plain workspace.
		Agents []string `json:"agents"`
		// Soul is an optional curated persona preset ref (see presets/souls). When
		// set, its markdown is seeded into <ws>/SOUL.md as the assistant's system
		// prompt. Empty = 空人设 (no persona file, no injection).
		Soul string `json:"soul"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ws := body.Workspace
	// Backfill default agent if missing — matches the existing
	// "claudecode" default in handler.go:175 + the agent.DefaultAgentType
	// constant in internal/agent/types.go.
	if ws.DefaultAgent == "" {
		ws.DefaultAgent = "claudecode"
	}
	// Default kind: 'project' unless the caller (assistant flow) says otherwise.
	if ws.Kind == "" {
		ws.Kind = "project"
	}
	// Name uniqueness across assistants + projects.
	if taken, ok, err := h.isNameTaken(ws.Name, ws.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	} else if ok {
		writeJSONStatus(w, http.StatusConflict, map[string]interface{}{
			"error":    "name_taken",
			"message":  fmt.Sprintf("名称已被%s使用,请改一个", nameKindLabel(taken.Kind)),
			"conflict": taken,
		})
		return
	}
	db, err := meta.OpenDefault()
	if err != nil {
		log.Printf("[workspace] db open error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if ws.ID == "temp" || ws.Path == "temp" {
		homeDir := get1AgentsHome()
		tempDir := filepath.Join(homeDir, "temp")
		_ = os.MkdirAll(tempDir, 0755)
		ws.Path = tempDir
	}
	// Assistant creation: the client picked no directory, so the backend mints an
	// identity badge (YYYYMMDD-NNNN — the assistant's "工牌") and owns a fresh
	// folder under ~/.1agents/projects/<badge>. The directory itself is created by
	// registerWorkspaceProject → ensureProjectGuideFiles below.
	if ws.Path == "" && ws.ID != "temp" {
		badge := newAssistantBadge()
		if ws.ID == "" {
			ws.ID = badge
		}
		ws.Path = filepath.Join(get1AgentsHome(), ".1agents", "projects", badge)
	}
	if ws.ID == "" {
		ws.ID = meta.NewID()
	}
	// Check for duplicate ID
	if _, ok, err := db.GetProject(ws.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	} else if ok {
		http.Error(w, "workspace with this ID already exists", http.StatusConflict)
		return
	}
	// Check for duplicate PATH (case-insensitive on macOS/Windows): the same
	// directory must map to a single workspace, so /Users/x/Coze and …/coze
	// don't become two projects. Return the existing one instead of creating a dup.
	if ws.Path != "" {
		if existing, err := db.ListWorkspaceProjects(); err == nil {
			target := normalizeWorkspacePath(ws.Path)
			for _, p := range existing {
				if normalizeWorkspacePath(p.WorkspacePath) == target {
					log.Printf("[workspace] create: path %s already registered as %q; returning existing", ws.Path, p.ID)
					writeJSON(w, map[string]interface{}{"ok": true, "workspace": projectToWorkspace(p), "existing": true})
					return
				}
			}
		}
	}
	if err := db.EnsureWorkspaceProject(workspaceToProject(ws)); err != nil {
		log.Printf("[workspace] save error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.registerWorkspaceProject(ws)

	// Optional template scaffold (#329).
	resp := map[string]interface{}{"ok": true, "workspace": ws}
	if body.TemplateID != "" {
		if _, sErr := scaffoldProject(body.TemplateID, ws.ID, ws.Path); sErr != nil {
			log.Printf("[workspace] scaffold template %s for project %s: %v", body.TemplateID, ws.ID, sErr)
			resp["scaffoldError"] = sErr.Error()
		} else {
			resp["scaffolded"] = true
		}
	}

	// Optional skill weak-copy into <ws>/.claude/skills (#360) — the assistant
	// create flow's skill picker. Non-fatal: a copy failure is surfaced but the
	// workspace still exists.
	if len(body.Skills) > 0 {
		if copied, sErr := syncSkillsToWorkspace(ws.Path, body.Skills); sErr != nil {
			log.Printf("[workspace] sync skills for project %s: %v", ws.ID, sErr)
			resp["skillsError"] = sErr.Error()
		} else {
			resp["skills"] = copied
		}
	}

	// Optional agent weak-copy into <ws>/.claude/agents — parallel to skills.
	// Non-fatal: a copy failure is surfaced but the workspace still exists.
	if len(body.Agents) > 0 {
		if copied, aErr := syncAgentsToWorkspace(ws.Path, body.Agents); aErr != nil {
			log.Printf("[workspace] sync agents for project %s: %v", ws.ID, aErr)
			resp["agentsError"] = aErr.Error()
		} else {
			resp["agents"] = copied
		}
	}

	// Optional persona seed into <ws>/SOUL.md — the assistant create flow's 人设
	// picker. Non-fatal: a seed failure is surfaced but the workspace still exists.
	if body.Soul != "" {
		if sErr := seedSoulToWorkspace(ws.Path, body.Soul); sErr != nil {
			log.Printf("[workspace] seed soul %q for project %s: %v", body.Soul, ws.ID, sErr)
			resp["soulError"] = sErr.Error()
		} else {
			resp["soul"] = body.Soul
		}
	}
	writeJSON(w, resp)
}

// ListSouls handles GET /api/assistant/souls?lang=zh: the curated persona presets
// for the create picker. Content is included for in-modal preview.
func (h *Handler) ListSouls(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	souls, err := listCuratedSouls(r.URL.Query().Get("lang"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"souls": souls})
}

// resolveWorkspacePath returns the on-disk path for a workspace id, or "" (with
// ok=false) when unknown. Shared by the soul GET/POST handlers.
func (h *Handler) resolveWorkspacePath(id string) (string, bool) {
	cfg, err := h.loadConfig()
	if err != nil {
		return "", false
	}
	for _, ws := range cfg.Workspaces {
		if ws.ID == id {
			return ws.Path, true
		}
	}
	return "", false
}

// WorkspaceSoul handles the assistant persona file:
//
//	GET  /api/workspace/soul?id=<wsId>       → {content}
//	POST /api/workspace/soul {id, content}   → writes <ws>/SOUL.md (empty clears it)
func (h *Handler) WorkspaceSoul(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		wsPath, ok := h.resolveWorkspacePath(r.URL.Query().Get("id"))
		if !ok {
			http.Error(w, "workspace not found", http.StatusNotFound)
			return
		}
		content, err := ReadWorkspaceSoul(wsPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"content": content})
	case http.MethodPost:
		var body struct {
			ID      string `json:"id"`
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		wsPath, ok := h.resolveWorkspacePath(body.ID)
		if !ok {
			http.Error(w, "workspace not found", http.StatusNotFound)
			return
		}
		if err := writeWorkspaceSoul(wsPath, body.Content); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// WorkspaceSkills handles GET /api/workspace/skills?id=<wsId>: lists the skills
// materialized in the workspace's .claude/skills, each flagged with whether the
// local copy has drifted from the 母体 baseline (so the detail page can light up
// a "推送到母体" affordance only where there's something to push).
func (h *Handler) WorkspaceSkills(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	cfg, err := h.loadConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var wsPath string
	for _, ws := range cfg.Workspaces {
		if ws.ID == id {
			wsPath = ws.Path
			break
		}
	}
	if wsPath == "" {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	skills, err := listWorkspaceSkills(wsPath, h.skillsAddr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"skills": skills})
}

// PushSkill handles POST /api/workspace/push-skill {id, skillRef}: the reverse of
// the create-time weak-copy. It resolves the workspace's own edited copy
// (<ws>/.claude/skills/<dir>) and pushes it back to the 1skills shared store
// (母体) as the new baseline. The store no-ops when the copy is unchanged, so the
// response's `changed` tells the caller whether the baseline actually moved.
func (h *Handler) PushSkill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		ID       string `json:"id"`
		SkillRef string `json:"skillRef"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.ID == "" || body.SkillRef == "" {
		http.Error(w, "id and skillRef are required", http.StatusBadRequest)
		return
	}
	cfg, err := h.loadConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var wsPath string
	for _, ws := range cfg.Workspaces {
		if ws.ID == body.ID {
			wsPath = ws.Path
			break
		}
	}
	if wsPath == "" {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	src, err := workspaceSkillDir(wsPath, body.SkillRef)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	changed, created, version, err := pushSkillToShared(h.skillsAddr, body.SkillRef, src)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "changed": changed, "created": created, "version": version})
}

// WorkspaceAgents handles GET /api/workspace/agents?id=<wsId>: lists the agents
// materialized in the workspace's .claude/agents (single <name>.md files), each
// flagged with whether the local copy has drifted from the 母体 baseline (so the
// detail page can light up a "推送到母体" affordance only where there's something
// to push). Mirror of WorkspaceSkills.
func (h *Handler) WorkspaceAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	cfg, err := h.loadConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var wsPath string
	for _, ws := range cfg.Workspaces {
		if ws.ID == id {
			wsPath = ws.Path
			break
		}
	}
	if wsPath == "" {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	agents, err := listWorkspaceAgents(wsPath, h.skillsAddr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"agents": agents})
}

// PushAgent handles POST /api/workspace/push-agent {id, agentRef}: the reverse of
// the create-time weak-copy. It resolves the workspace's own edited copy
// (<ws>/.claude/agents/<name>.md) and pushes it back to the 1skills shared store
// (母体) as the new baseline. The store no-ops when the copy is unchanged, so the
// response's `changed` tells the caller whether the baseline actually moved.
// Mirror of PushSkill.
func (h *Handler) PushAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		ID       string `json:"id"`
		AgentRef string `json:"agentRef"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.ID == "" || body.AgentRef == "" {
		http.Error(w, "id and agentRef are required", http.StatusBadRequest)
		return
	}
	cfg, err := h.loadConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var wsPath string
	for _, ws := range cfg.Workspaces {
		if ws.ID == body.ID {
			wsPath = ws.Path
			break
		}
	}
	if wsPath == "" {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	src, err := workspaceAgentFile(wsPath, body.AgentRef)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	changed, created, err := pushAgentToShared(h.skillsAddr, body.AgentRef, src)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "changed": changed, "created": created})
}

// Update handles POST /api/workspace/update
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var ws Workspace
	if err := json.NewDecoder(r.Body).Decode(&ws); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	db, err := meta.OpenDefault()
	if err != nil {
		log.Printf("[workspace] db open error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	existing, ok, err := db.GetProject(ws.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	// Built-in workspace is allowed rename/avatar edits (default assistant), but
	// its kind and path are pinned to the stored values — clients cannot demote
	// the built-in default via update.
	if existing.Builtin {
		ws.Kind = existing.Kind
		ws.Path = existing.WorkspacePath
	}
	// builtin can never be set via update — pin it to the stored value.
	ws.Builtin = existing.Builtin
	// Name uniqueness across assistants + projects (excluding self).
	if taken, ok, err := h.isNameTaken(ws.Name, ws.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	} else if ok {
		writeJSONStatus(w, http.StatusConflict, map[string]interface{}{
			"error":    "name_taken",
			"message":  fmt.Sprintf("名称已被%s使用,请改一个", nameKindLabel(taken.Kind)),
			"conflict": taken,
		})
		return
	}
	// EnsureWorkspaceProject preserves the row's status and position on conflict.
	if err := db.EnsureWorkspaceProject(workspaceToProject(ws)); err != nil {
		log.Printf("[workspace] save error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "workspace": ws})
}

// Delete handles DELETE /api/workspace/delete
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id query parameter", http.StatusBadRequest)
		return
	}
	if id == "default" {
		http.Error(w, "cannot delete built-in workspace", http.StatusForbidden)
		return
	}
	db, err := meta.OpenDefault()
	if err != nil {
		log.Printf("[workspace] db open error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wsToDelete, ok, err := db.GetProject(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	if wsToDelete.Builtin {
		http.Error(w, "cannot delete built-in workspace", http.StatusForbidden)
		return
	}
	if err := db.DeleteProject(id); err != nil {
		log.Printf("[workspace] delete error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Dynamically remove this workspace's CC-Connect project(s). Since #277 the
	// project is keyed by work_dir PATH, not name — names have drifted (plain
	// slug, legacy __<agent> suffix, or "_" for non-ASCII), so a name-based
	// removal silently misses and the startup import loop resurrects the
	// workspace on the next restart. Match by path so deletion actually sticks.
	if config.ConfigPath != "" {
		removed := removeCCProjectsByPath(wsToDelete.WorkspacePath)
		if removed == 0 {
			// Fallback for projects recorded without a work_dir: try the legacy
			// name-based removal so pre-#277 configs still clean up.
			nameOrID := wsToDelete.Name
			if nameOrID == "" {
				nameOrID = wsToDelete.ID
			}
			projName := getCCProjectName(nameOrID, "claudecode")
			if err := config.RemoveProject(projName); err != nil {
				log.Printf("[workspace] ccconnect remove project %q: %v", projName, err)
			} else {
				removed = 1
			}
		}
		if removed > 0 {
			log.Printf("[workspace] removed %d CC-Connect project(s) for workspace %s (%s)", removed, id, wsToDelete.WorkspacePath)

			// Trigger cc-connect to hot restart itself and reload the configuration!
			select {
			case core.RestartCh <- core.RestartRequest{}:
				log.Println("[workspace] Successfully requested CC-Connect process hot restart for configuration reload")
			default:
				log.Println("[workspace] CC-Connect hot restart already pending")
			}

			// Wait for the hot restart to finish to avoid 502 Bad Gateway race condition on immediate redirect
			time.Sleep(1000 * time.Millisecond)
		}
	}

	// Clean up tmux windows associated with this workspace
	if h.tmuxSession != "" {
		if exec.Command("tmux", "has-session", "-t", h.tmuxSession).Run() == nil {
			cmd := exec.Command("tmux", "list-windows", "-t", h.tmuxSession, "-F", "#{window_index}|#{window_name}")
			if out, err := cmd.Output(); err == nil {
				lines := strings.Split(strings.TrimSpace(string(out)), "\n")
				var windowsToKill []int
				var totalWindows int
				for _, line := range lines {
					if line == "" {
						continue
					}
					totalWindows++
					parts := strings.SplitN(line, "|", 2)
					if len(parts) != 2 {
						continue
					}
					idx, err1 := strconv.Atoi(parts[0])
					name := parts[1]
					if err1 != nil {
						continue
					}

					// Parse workspace ID from name: "{workspaceId}_{n}" or "{workspaceId}"
					wsID := name
					if lastUnderscore := strings.LastIndex(name, "_"); lastUnderscore > 0 {
						wsID = name[:lastUnderscore]
					}

					if wsID == id {
						windowsToKill = append(windowsToKill, idx)
					}
				}

				if len(windowsToKill) > 0 {
					// If we are about to kill all windows, create a placeholder "p" first to keep session alive
					if len(windowsToKill) >= totalWindows {
						_ = exec.Command("tmux", "new-window", "-t", h.tmuxSession, "-n", "p").Run()
					}

					// Kill target windows
					for _, idx := range windowsToKill {
						log.Printf("[workspace] Killing tmux window %d for deleted workspace %s", idx, id)
						_ = exec.Command("tmux", "kill-window", "-t", fmt.Sprintf("%s:%d", h.tmuxSession, idx)).Run()
					}
				}
			}
		}
	}

	writeJSON(w, map[string]interface{}{"ok": true})
}

// Reorder handles POST /api/workspace/reorder
func (h *Handler) Reorder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	db, err := meta.OpenDefault()
	if err != nil {
		log.Printf("[workspace] db open error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cfg, err := h.loadConfig()
	if err != nil {
		log.Printf("[workspace] load error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Build the full ordered id list: requested ids first (only those that
	// exist), then any remaining workspaces in their current order — mirroring
	// the previous "append unspecified at the end" semantics.
	exists := make(map[string]bool, len(cfg.Workspaces))
	for _, ws := range cfg.Workspaces {
		exists[ws.ID] = true
	}
	ordered := make([]string, 0, len(cfg.Workspaces))
	seen := make(map[string]bool, len(req.IDs))
	for _, id := range req.IDs {
		if exists[id] && !seen[id] {
			ordered = append(ordered, id)
			seen[id] = true
		}
	}
	for _, ws := range cfg.Workspaces {
		if !seen[ws.ID] {
			ordered = append(ordered, ws.ID)
		}
	}

	if err := db.ReorderProjects(ordered); err != nil {
		log.Printf("[workspace] reorder error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{"ok": true})
}

// PickDirectory handles POST /api/workspace/pick-directory.
// It opens a native OS folder picker dialog and returns the selected path.
func (h *Handler) PickDirectory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path, err := pickDirectory()
	if err != nil {
		if isUserCancel(err) {
			writeJSON(w, map[string]string{"path": ""})
			return
		}
		log.Printf("[workspace] pick-directory error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"path": path})
}

func pickDirectory() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		return pickDirectoryDarwin()
	case "linux":
		return pickDirectoryLinux()
	case "windows":
		return pickDirectoryWindows()
	default:
		return "", fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func pickDirectoryWindows() (string, error) {
	script := `Add-Type -AssemblyName System.Windows.Forms; $f = New-Object System.Windows.Forms.FolderBrowserDialog; $f.Description = "选择工作空间目录"; if ($f.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { $f.SelectedPath }`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func pickDirectoryDarwin() (string, error) {
	script := `try
		POSIX path of (choose folder with prompt "选择工作空间目录")
	end try`
	cmd := exec.Command("osascript", "-e", script)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func pickDirectoryLinux() (string, error) {
	cmd := exec.Command("zenity", "--file-selection", "--directory", "--title=选择工作空间目录")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func isUserCancel(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "User canceled") ||
		strings.Contains(s, "canceled") ||
		strings.Contains(s, "exit status 1")
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[workspace] json encode error: %v", err)
	}
}

// writeJSONStatus writes an explicit HTTP status code with a JSON body. Used by
// Create/Update to return structured 409 name-conflict payloads the frontend
// can distinguish from generic errors.
func writeJSONStatus(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[workspace] json encode error: %v", err)
	}
}

// nameKindLabel returns a Chinese label ("助理" / "项目") for a conflicting
// workspace's kind — used in the 409 message shown to the user.
func nameKindLabel(kind string) string {
	if kind == "assistant" {
		return "助理"
	}
	return "项目"
}

// ListDirectories handles GET /api/workspace/list-directories
func (h *Handler) ListDirectories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	pathParam := r.URL.Query().Get("path")
	pathParam = expandTilde(pathParam)
	var targetPath string

	if pathParam == "" || pathParam == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Printf("[workspace] os.UserHomeDir failed: %v", err)
			// Try manual environment lookups as a fallback for user home directory
			if h := os.Getenv("HOME"); h != "" {
				home = h
			} else if u := os.Getenv("USER"); u != "" {
				candidate := "/home/" + u
				if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
					home = candidate
				}
			}

			// If still empty or failed to find a valid directory, fall back to system root
			if home == "" {
				if runtime.GOOS == "windows" {
					drive := os.Getenv("SystemDrive")
					if drive != "" {
						home = drive + "\\"
					} else {
						home = "C:\\"
					}
				} else {
					home = "/"
				}
				log.Printf("[workspace] Falling back to system root directory: %s", home)
			}
		}
		targetPath = home
	} else {
		abs, err := filepath.Abs(pathParam)
		if err != nil {
			http.Error(w, "invalid path: "+err.Error(), http.StatusBadRequest)
			return
		}
		targetPath = abs
	}

	entries, err := os.ReadDir(targetPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type DirEntry struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}

	directories := []DirEntry{}
	for _, e := range entries {
		if e.IsDir() {
			name := e.Name()
			if name == "." || name == ".." {
				continue
			}
			directories = append(directories, DirEntry{
				Name: name,
				Path: filepath.Join(targetPath, name),
			})
		}
	}

	parentPath := filepath.Dir(targetPath)
	if parentPath == targetPath {
		parentPath = ""
	}

	writeJSON(w, map[string]any{
		"currentPath": targetPath,
		"parentPath":  parentPath,
		"directories": directories,
	})
}

// expandTilde expands a ~ prefix to the user's home directory.
func expandTilde(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~"+string(os.PathSeparator)) {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// removeCCProjectsByPath removes every cc-connect project whose agent work_dir
// resolves to the same filesystem path as wsPath (#277 path identity). Returns
// the number removed. Project names are unreliable delete keys (slug drift,
// legacy suffixes, "_"); matching by path is what makes a workspace deletion
// survive a backend restart — otherwise the import loop re-adds it.
func removeCCProjectsByPath(wsPath string) int {
	if wsPath == "" {
		return 0
	}
	cfg, err := config.Load(config.ConfigPath)
	if err != nil {
		log.Printf("[workspace] load cc-connect config for path-remove: %v", err)
		return 0
	}
	target := normalizeWorkspacePath(wsPath)
	var names []string
	for _, p := range cfg.Projects {
		wd, _ := p.Agent.Options["work_dir"].(string)
		if wd != "" && normalizeWorkspacePath(wd) == target {
			names = append(names, p.Name)
		}
	}
	removed := 0
	for _, n := range names {
		if err := config.RemoveProject(n); err != nil {
			log.Printf("[workspace] ccconnect remove project %q: %v", n, err)
			continue
		}
		removed++
	}
	return removed
}

// normalizeWorkspacePath canonicalizes a path for identity comparison (absolute
// + cleaned, case-folded on case-insensitive filesystems), mirroring
// ccconnect.normalizePath so create/delete match the same key the reconciler
// uses — /Users/x/Coze and …/coze are the same dir on macOS/Windows.
func normalizeWorkspacePath(p string) string {
	out := filepath.Clean(p)
	if abs, err := filepath.Abs(p); err == nil {
		out = filepath.Clean(abs)
	}
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		out = strings.ToLower(out)
	}
	return out
}

// ccSafeProjectName mirrors ccconnect.CCProjectName without importing the
// package (ccconnect already imports workspace): use the workspace name only
// when every rune is cc-connect-safe ([a-zA-Z0-9_-], ≤32 chars); otherwise use
// the workspace id, which is always ascii (assistant badge / hex / "default").
// Keeping register + panel + reconciler on one naming rule is what makes the
// embed panel's /api/v1/projects/<name> lookups hit the registered project.
func ccSafeProjectName(ws Workspace) string {
	name := ws.Name
	if name == "" || len(name) > 32 {
		return ws.ID
	}
	for _, r := range name {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '_' || r == '-'
		if !ok {
			return ws.ID
		}
	}
	return name
}

func getCCProjectName(workspaceName string, agentType string) string {
	var sb strings.Builder
	inInvalidSeq := false
	for _, r := range workspaceName {
		isValid := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
		if isValid {
			sb.WriteRune(r)
			inInvalidSeq = false
		} else {
			if !inInvalidSeq {
				sb.WriteRune('_')
				inInvalidSeq = true
			}
		}
	}
	slug := sb.String()
	if len(slug) > 32 {
		slug = slug[:32]
	}
	if slug == "" {
		slug = "ws"
	}
	return fmt.Sprintf("%s__%s", slug, agentType)
}

// CreateDirectory handles POST /api/workspace/create-directory
func (h *Handler) CreateDirectory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ParentPath string `json:"parentPath"`
		Name       string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		http.Error(w, "directory name cannot be empty", http.StatusBadRequest)
		return
	}
	if strings.Contains(req.Name, "/") || strings.Contains(req.Name, "\\") || req.Name == ".." || req.Name == "." {
		http.Error(w, "invalid directory name", http.StatusBadRequest)
		return
	}

	parent := expandTilde(req.ParentPath)
	if parent == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			http.Error(w, "unable to determine home directory", http.StatusInternalServerError)
			return
		}
		parent = home
	}
	parentAbs, err := filepath.Abs(parent)
	if err != nil {
		http.Error(w, "invalid parent path: "+err.Error(), http.StatusBadRequest)
		return
	}

	targetPath := filepath.Join(parentAbs, req.Name)
	targetPath = filepath.Clean(targetPath)
	parentAbs = filepath.Clean(parentAbs)

	// Verify path safety (prevent directory traversal)
	if !strings.HasPrefix(targetPath, parentAbs) {
		http.Error(w, "path traversal detected", http.StatusBadRequest)
		return
	}

	if err := os.MkdirAll(targetPath, 0755); err != nil {
		http.Error(w, "failed to create directory: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{
		"ok":   true,
		"path": targetPath,
	})
}
