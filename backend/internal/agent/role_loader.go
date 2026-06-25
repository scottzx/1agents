package agent

import (
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

//go:embed roles/*.md
var builtinRolesFS embed.FS

// RoleRegistry holds the merged set of role templates resolved by name across
// the builtin, user, and project layers (later layers override earlier ones by
// the template's name: field).
type RoleRegistry struct {
	mu    sync.RWMutex
	roles map[string]*RoleTemplate
}

// Resolve returns the template for a role name. It returns the template even
// when !Available so the caller can decide whether to fall back.
func (r *RoleRegistry) Resolve(name string) (*RoleTemplate, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tpl, ok := r.roles[name]
	return tpl, ok
}

// LoadRoles builds a registry from three layers, lowest precedence first:
//
//	builtin (embedded) → user (~/.1agents/roles) → project (<ws>/.1agents/roles)
//
// Same-named templates in a higher layer override lower ones. Per-file parse
// errors are logged and skipped — one bad user template never breaks the
// builtin roles. Templates load even when their engine isn't installed; they
// are flagged via Available/Unavailable for the caller to act on. Each role's
// declared skills are resolved against the skill registry of the same workspace;
// names that don't resolve are recorded in MissingSkills (graceful degradation —
// the role still loads).
func LoadRoles(workspacePath string) *RoleRegistry {
	reg := &RoleRegistry{roles: make(map[string]*RoleTemplate)}

	// Layer 1: builtin (embedded).
	entries, err := builtinRolesFS.ReadDir("roles")
	if err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			raw, err := builtinRolesFS.ReadFile("roles/" + e.Name())
			if err != nil {
				log.Printf("[agent] builtin role %s: read failed: %v", e.Name(), err)
				continue
			}
			reg.add(raw, "builtin", "", "roles/"+e.Name())
		}
	}

	// Layer 2 + 3: user-level then project-level dirs. The canonical dir is
	// ".1agents/roles" (RFC §4); ".1agents/agents" is read too as the legacy
	// name from the first cut, lower precedence than the canonical one.
	if dir := userRolesLegacyDir(); dir != "" {
		reg.loadDir(dir, "user")
	}
	if dir := userRolesDir(); dir != "" {
		reg.loadDir(dir, "user")
	}
	if workspacePath != "" {
		reg.loadDir(filepath.Join(workspacePath, ".1agents", "agents"), "project")
		reg.loadDir(filepath.Join(workspacePath, ".1agents", "roles"), "project")
	}

	// Bind declared skills against the skill registry; flag any that are missing.
	reg.bindSkills(LoadSkills(workspacePath))

	return reg
}

// bindSkills records, per role, which of its declared skills don't resolve in
// the skill registry. Missing skills don't fail the role — they degrade it (the
// caller/UI surfaces MissingSkills as "不可用").
func (r *RoleRegistry) bindSkills(skills *SkillRegistry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, tpl := range r.roles {
		var missing []string
		for _, name := range tpl.Skills {
			if _, ok := skills.Resolve(name); !ok {
				missing = append(missing, name)
			}
		}
		tpl.MissingSkills = missing
	}
}

// loadDir parses every *.md role template under dir (non-recursive). A missing
// dir is a no-op; per-file errors are logged and skipped.
func (r *RoleRegistry) loadDir(dir, source string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return // missing dir is fine
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			log.Printf("[agent] role %s: read failed: %v", path, err)
			continue
		}
		r.add(raw, source, path, path)
	}
}

// add parses one template and inserts it keyed by name, overriding any existing
// entry (later layers win). diskPath is the on-disk path ("" for builtin), kept
// for fork/restore; logPath is only for log lines. Parse failures are logged and
// skipped.
func (r *RoleRegistry) add(raw []byte, source, diskPath, logPath string) {
	tpl, err := parseRoleMarkdown(raw)
	if err != nil {
		log.Printf("[agent] role %s: %v (skipped)", logPath, err)
		return
	}
	tpl.Source = source
	tpl.Path = diskPath
	markRoleAvailability(tpl)
	r.mu.Lock()
	r.roles[tpl.Name] = tpl
	r.mu.Unlock()
}

// markRoleAvailability sets Available/Unavailable by checking the template's
// engine against the shared agent catalog probe (reuses the cached exec.LookPath
// scan — does not re-probe). Skills are treated as opaque and never block.
func markRoleAvailability(tpl *RoleTemplate) {
	at := engineToAgentType(tpl.Engine)
	for _, st := range DefaultCatalog().Snapshot() {
		if st.Type == at {
			if st.ChatReady {
				tpl.Available = true
			} else {
				tpl.Available = false
				tpl.Unavailable = "engine " + tpl.Engine + " not installed"
			}
			return
		}
	}
	tpl.Available = false
	tpl.Unavailable = "unknown engine " + tpl.Engine
}

// oneAgentsHome returns the base dir that holds .1agents, honoring
// ONEAGENTS_HOME first and falling back to the user's home dir. Returns "" when
// neither is available.
func oneAgentsHome() string {
	if val := os.Getenv("ONEAGENTS_HOME"); val != "" {
		return val
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

// userRolesDir is the canonical user-level role template directory
// (~/.1agents/roles), per RFC §4. Forks are written here.
func userRolesDir() string {
	base := oneAgentsHome()
	if base == "" {
		return ""
	}
	return filepath.Join(base, ".1agents", "roles")
}

// userRolesLegacyDir is the original user-level dir (~/.1agents/agents) from the
// first cut, still read for backward compatibility (lower precedence than
// userRolesDir).
func userRolesLegacyDir() string {
	base := oneAgentsHome()
	if base == "" {
		return ""
	}
	return filepath.Join(base, ".1agents", "agents")
}

// ForkRole copies a role template to the user level so it can be edited without
// touching the builtin (which is embedded and can't be deleted). It writes the
// template's current raw markdown to ~/.1agents/roles/<name>.md and returns the
// new path. Editing the fork shadows the builtin by name; deleting the fork file
// restores the builtin (RestoreRole). It is an error to fork a role that is
// already user-level (no builtin to preserve) or whose raw content can't be read.
func ForkRole(name string, workspacePath string) (string, error) {
	reg := LoadRoles(workspacePath)
	tpl, ok := reg.Resolve(name)
	if !ok {
		return "", fmt.Errorf("role %q not found", name)
	}

	raw, err := rawRoleMarkdown(tpl)
	if err != nil {
		return "", err
	}

	dir := userRolesDir()
	if dir == "" {
		return "", fmt.Errorf("cannot resolve user roles dir")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dir, err)
	}
	dst := filepath.Join(dir, name+".md")
	if err := os.WriteFile(dst, raw, 0o644); err != nil {
		return "", fmt.Errorf("write fork %s: %w", dst, err)
	}
	return dst, nil
}

// RestoreRole deletes the user-level fork of a role, so the next LoadRoles falls
// back to the builtin (or to a project-level entry). It is a no-op when no fork
// exists. It refuses to delete anything outside the user roles dir.
func RestoreRole(name string) error {
	dir := userRolesDir()
	if dir == "" {
		return fmt.Errorf("cannot resolve user roles dir")
	}
	path := filepath.Join(dir, name+".md")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove fork %s: %w", path, err)
	}
	return nil
}

// rawRoleMarkdown returns the on-disk/embedded raw bytes for a role template,
// used by ForkRole. Builtin templates read from the embedded FS; disk templates
// read from tpl.Path.
func rawRoleMarkdown(tpl *RoleTemplate) ([]byte, error) {
	if tpl.Source == "builtin" {
		return builtinRolesFS.ReadFile("roles/" + tpl.Name + ".md")
	}
	if tpl.Path == "" {
		return nil, fmt.Errorf("role %q has no source path", tpl.Name)
	}
	return os.ReadFile(tpl.Path)
}
