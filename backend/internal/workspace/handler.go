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
	"github.com/scottzx/1Agents/backend/internal/harnesskit"
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
	// Kind: "workforce" | "project" — see meta.Project.Kind / #189.
	// Legacy clients may still send "assistant"; create/update normalizes to workforce.
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
	extensions  extensionClient
}

func NewHandler(tmuxSession ...string) *Handler {
	session := ""
	if len(tmuxSession) > 0 {
		session = tmuxSession[0]
	}
	return &Handler{tmuxSession: session}
}

// SetHarnessKitRuntime wires the private, authenticated extension runtime into
// the workspace facade. The browser never receives the endpoint or token.
func (h *Handler) SetHarnessKitRuntime(runtime harnesskit.Runtime) {
	if runtime != nil {
		h.extensions = harnesskit.NewClient(runtime)
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
		// Migrate the legacy default row into the workforce (UI「助理」) family.
		// Older installs created it with name "对话" and no kind; the sidebar shows
		// this row as the always-first 助理, so it needs kind=workforce and a
		// friendlier display name. Users can rename it later. (#189)
		needs := existing.Kind != meta.KindWorkforce || existing.Name == "对话" || existing.Name == "" ||
			existing.Avatar == ""
		if needs {
			existing.Kind = meta.KindWorkforce
			if existing.Name == "对话" || existing.Name == "" {
				existing.Name = "助理"
			}
			if existing.Avatar == "" {
				existing.Avatar = defaultAssistantAvatar
			}
			if err := db.EnsureWorkspaceProject(existing); err != nil {
				log.Printf("[workspace] migrate default row: %v", err)
			} else {
				log.Printf("[workspace] migrated default row → kind=workforce, name=%q", existing.Name)
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
		Kind:          meta.KindWorkforce,
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
	// ?status=archived returns the archive board instead (the overview's 已归档
	// cards); any other status filters to that exact status.
	wantStatus := r.URL.Query().Get("status")
	out := make([]Workspace, 0, len(cfg.Workspaces))
	for _, ws := range cfg.Workspaces {
		isActive := ws.Status == "" || ws.Status == string(meta.ProjectStatusActive)
		switch {
		case wantStatus == "":
			if isActive {
				out = append(out, ws)
			}
		case wantStatus == string(meta.ProjectStatusActive):
			if isActive {
				out = append(out, ws)
			}
		default:
			if ws.Status == wantStatus {
				out = append(out, ws)
			}
		}
	}
	writeJSON(w, out)
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
		// Skills and Agents are HarnessKit global extension IDs selected by the
		// assistant-create flow. They are deployed into this project scope.
		Skills []string `json:"skills"`
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
	// Default kind: project unless the caller (create-assistant flow) says otherwise.
	// Normalize legacy kind=assistant → workforce so write path only persists
	// workforce|project (#189).
	ws.Kind = meta.NormalizeProjectKind(ws.Kind)
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

	// Optional HarnessKit project deployment. Workspace creation remains
	// successful if an extension is unavailable; the response makes the partial
	// failure explicit so the UI does not imply a hidden deployment.
	if len(body.Skills) > 0 {
		installed := make([]string, 0, len(body.Skills))
		for _, extensionID := range body.Skills {
			targetID, _, installErr := h.installExtension(r.Context(), ws, "skill", extensionID)
			if installErr != nil {
				log.Printf("[workspace] install HarnessKit skill %s for project %s: %v", extensionID, ws.ID, installErr)
				resp["skillsError"] = installErr.Error()
				break
			}
			installed = append(installed, targetID)
		}
		resp["skills"] = installed
	}

	if len(body.Agents) > 0 {
		installed := make([]string, 0, len(body.Agents))
		for _, extensionID := range body.Agents {
			targetID, status, installErr := h.installExtension(r.Context(), ws, "subagent", extensionID)
			if installErr != nil {
				log.Printf("[workspace] install HarnessKit subagent %s for project %s: %v", extensionID, ws.ID, installErr)
				resp["agentsError"] = installErr.Error()
				break
			}
			installed = append(installed, targetID)
			if status.File != "" {
				if team, teamErr := ReadTeam(ws.Path); teamErr == nil {
					team.Members = appendUnique(team.Members, status.File)
					_ = WriteTeam(ws.Path, team)
				}
			}
		}
		resp["agents"] = installed
	}

	// Optional persona seed — the assistant create flow's 人设 picker. The chosen
	// soul becomes the workspace's primary team agent (<ws>/.claude/agents/<ref>.md
	// + .agents/team.json). Empty = 空人设 (no agent, no team, no injection).
	// Non-fatal: a seed failure is surfaced but the workspace still exists.
	if body.Soul != "" {
		if file, sErr := seedSoulAsPrimaryAgent(ws.Path, body.Soul); sErr != nil {
			log.Printf("[workspace] seed soul %q for project %s: %v", body.Soul, ws.ID, sErr)
			resp["soulError"] = sErr.Error()
		} else {
			resp["soul"] = body.Soul
			resp["primaryAgent"] = file
		}
	}
	// A seeded persona is a native subagent file, so refresh HarnessKit after
	// all create-time filesystem writes. Registration conflict is idempotent.
	if client, cErr := h.requireExtensionClient(); cErr == nil {
		if scanErr := client.EnsureProject(r.Context(), ws.Path); scanErr != nil {
			log.Printf("[workspace] HarnessKit project scan for %s: %v", ws.ID, scanErr)
			resp["extensionsError"] = scanErr.Error()
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

// WorkspaceTeam handles the project's agent-team manifest (<ws>/.agents/team.json):
//
//	GET  /api/workspace/team?id=<wsId>      → {primary, members:[WorkspaceAgentStatus]}
//	POST /api/workspace/team {id, primary}  → set the primary agent (drives the
//	                                           default conversation). primary="" clears it.
//
// members is the full .claude/agents roster (with drift status) so the picker can
// list every expert; primary names which one drives the default main session.
func (h *Handler) WorkspaceTeam(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		ws, err := h.extensionWorkspace(r.URL.Query().Get("id"))
		if err != nil {
			http.Error(w, "workspace not found", http.StatusNotFound)
			return
		}
		team, err := ReadTeam(ws.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		members, err := h.listProjectExtensions(r.Context(), ws, "subagent")
		if err != nil {
			writeExtensionError(w, err)
			return
		}
		writeJSON(w, map[string]any{"primary": team.Primary, "members": members})
	case http.MethodPost:
		var body struct {
			ID      string `json:"id"`
			Primary string `json:"primary"`
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
		primary := strings.TrimSpace(body.Primary)
		// A non-empty primary must name a real agent file so the picker can't set
		// a dangling primary that resolves to no persona.
		if primary != "" {
			if _, err := workspaceAgentFile(wsPath, primary); err != nil {
				http.Error(w, "unknown agent: "+primary, http.StatusBadRequest)
				return
			}
		}
		team, err := ReadTeam(wsPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		team.Primary = primary
		if err := WriteTeam(wsPath, team); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "primary": primary})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// WorkspaceAvailableAgents lists global HarnessKit subagents that can be
// deployed into this project scope.
func (h *Handler) WorkspaceAvailableAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ws, err := h.extensionWorkspace(r.URL.Query().Get("id"))
	if err != nil {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	list, err := h.listAvailableExtensions(r.Context(), ws, "subagent")
	if err != nil {
		writeExtensionError(w, err)
		return
	}
	writeJSON(w, map[string]any{"agents": list})
}

// AddAgent deploys a global HarnessKit subagent into the project's native Agent
// configuration and registers its resulting file in team.json.
func (h *Handler) AddAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, extensionID, err := decodeExtensionRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ws, err := h.extensionWorkspace(id)
	if err != nil {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	targetID, status, err := h.installExtension(r.Context(), ws, "subagent", extensionID)
	if err != nil {
		writeExtensionError(w, err)
		return
	}
	if status.File != "" {
		if team, teamErr := ReadTeam(ws.Path); teamErr == nil {
			team.Members = appendUnique(team.Members, status.File)
			_ = WriteTeam(ws.Path, team)
		}
	}
	writeJSON(w, map[string]any{"ok": true, "extensionId": targetID})
}

// RemoveAgent deletes a project-scoped HarnessKit subagent and drops its native
// file from team.json, clearing the primary when necessary.
func (h *Handler) RemoveAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, extensionID, err := decodeExtensionRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ws, err := h.extensionWorkspace(id)
	if err != nil {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	client, ext, err := h.findProjectExtension(r.Context(), ws, "subagent", extensionID)
	if err != nil {
		writeExtensionError(w, err)
		return
	}
	status := extensionStatus(ws.Path, ext)
	if err := client.DeleteExtension(r.Context(), extensionID); err != nil {
		writeExtensionError(w, err)
		return
	}
	file := status.File
	if team, tErr := ReadTeam(ws.Path); tErr == nil {
		next := team.Members[:0]
		for _, m := range team.Members {
			if m != file {
				next = append(next, m)
			}
		}
		team.Members = next
		if team.Primary == file {
			team.Primary = ""
		}
		_ = WriteTeam(ws.Path, team)
	}
	writeJSON(w, map[string]any{"ok": true, "extensionId": extensionID})
}

// WorkspaceSkills lists project-scoped HarnessKit skills after ensuring the
// project is registered and its native files have been scanned.
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
	ws, err := h.extensionWorkspace(id)
	if err != nil {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	skills, err := h.listProjectExtensions(r.Context(), ws, "skill")
	if err != nil {
		writeExtensionError(w, err)
		return
	}
	writeJSON(w, map[string]any{"skills": skills})
}

// PushSkill retains its route name for API continuity, but its HarnessKit
// product semantic is "reindex project-native files". It never publishes a
// shared version or creates a branch.
func (h *Handler) PushSkill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, extensionID, err := decodeExtensionRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ws, err := h.extensionWorkspace(id)
	if err != nil {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	client, ext, err := h.findProjectExtension(r.Context(), ws, "skill", extensionID)
	if err != nil {
		writeExtensionError(w, err)
		return
	}
	if r.URL.Query().Get("preview") == "1" {
		writeJSON(w, map[string]any{
			"mode":        "in-place",
			"extensionId": ext.ID,
			"name":        ext.Name,
			"message":     "Reindex this project extension after local edits. HarnessKit does not publish it to a shared mother branch.",
		})
		return
	}
	if err := client.ScanAndSync(r.Context()); err != nil {
		writeExtensionError(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "status": "indexed", "extensionId": extensionID})
}

// PullSkill updates a project extension from its verified HarnessKit source.
// Extensions without update metadata receive an explicit 422 response.
func (h *Handler) PullSkill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, extensionID, err := decodeExtensionRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ws, err := h.extensionWorkspace(id)
	if err != nil {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	client, ext, err := h.findProjectExtension(r.Context(), ws, "skill", extensionID)
	if err != nil {
		writeExtensionError(w, err)
		return
	}
	status := extensionStatus(ws.Path, ext)
	if !status.CanUpdate {
		writeJSONStatus(w, http.StatusUnprocessableEntity, map[string]any{
			"error":   "source_update_unavailable",
			"message": status.UpdateReason,
		})
		return
	}
	if err := client.UpdateExtension(r.Context(), extensionID); err != nil {
		writeExtensionError(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "status": "updated", "extensionId": extensionID})
}

// workspacePathByID resolves a workspace id to its filesystem path from config.
func (h *Handler) workspacePathByID(id string) (string, error) {
	cfg, err := h.loadConfig()
	if err != nil {
		return "", err
	}
	for _, ws := range cfg.Workspaces {
		if ws.ID == id {
			return ws.Path, nil
		}
	}
	return "", fmt.Errorf("workspace not found")
}

// AvailableSkills lists global HarnessKit skills and whether each name is
// already deployed into this project scope.
func (h *Handler) AvailableSkills(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	ws, err := h.extensionWorkspace(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	list, err := h.listAvailableExtensions(r.Context(), ws, "skill")
	if err != nil {
		writeExtensionError(w, err)
		return
	}
	writeJSON(w, map[string]any{"skills": list})
}

// AddSkill deploys a global HarnessKit skill into the project's native Agent
// configuration and returns the project-scoped extension ID.
func (h *Handler) AddSkill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, extensionID, err := decodeExtensionRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ws, err := h.extensionWorkspace(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	targetID, _, err := h.installExtension(r.Context(), ws, "skill", extensionID)
	if err != nil {
		writeExtensionError(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "extensionId": targetID})
}

// RemoveSkill deletes exactly one verified project-scoped HarnessKit skill.
func (h *Handler) RemoveSkill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, extensionID, err := decodeExtensionRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ws, err := h.extensionWorkspace(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	client, _, err := h.findProjectExtension(r.Context(), ws, "skill", extensionID)
	if err != nil {
		writeExtensionError(w, err)
		return
	}
	if err := client.DeleteExtension(r.Context(), extensionID); err != nil {
		writeExtensionError(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "extensionId": extensionID})
}

// WorkspaceAgents lists project-scoped HarnessKit subagents. Team selection
// still uses their native file names, while mutations use stable extension IDs.
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
	ws, err := h.extensionWorkspace(id)
	if err != nil {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	agents, err := h.listProjectExtensions(r.Context(), ws, "subagent")
	if err != nil {
		writeExtensionError(w, err)
		return
	}
	writeJSON(w, map[string]any{"agents": agents})
}

// PushAgent retains its route name for API continuity; it reindexes native
// project subagent files and does not publish shared history.
func (h *Handler) PushAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, extensionID, err := decodeExtensionRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ws, err := h.extensionWorkspace(id)
	if err != nil {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	client, _, err := h.findProjectExtension(r.Context(), ws, "subagent", extensionID)
	if err != nil {
		writeExtensionError(w, err)
		return
	}
	if err := client.ScanAndSync(r.Context()); err != nil {
		writeExtensionError(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "status": "indexed", "extensionId": extensionID})
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
	// Built-in workspace is allowed rename/avatar edits (default 助理), but
	// its kind and path are pinned to the stored values — clients cannot demote
	// the built-in default via update.
	if existing.Builtin {
		ws.Kind = existing.Kind
		ws.Path = existing.WorkspacePath
	} else {
		// Write path only persists workforce|project (#189).
		ws.Kind = meta.NormalizeProjectKind(ws.Kind)
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

// nameKindLabel returns a Chinese label for a conflicting workspace's kind —
// used in the 409 message shown to the user.
func nameKindLabel(kind string) string {
	switch meta.NormalizeProjectKind(kind) {
	case meta.KindWorkforce:
		return "助理"
	case meta.KindTmp:
		return "临时对话"
	case meta.KindApp:
		return "应用"
	default:
		return "项目"
	}
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
