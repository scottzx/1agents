package agent

import (
	"embed"
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
//	builtin (embedded) → user (~/.1agents/agents) → project (<ws>/.1agents/agents)
//
// Same-named templates in a higher layer override lower ones. Per-file parse
// errors are logged and skipped — one bad user template never breaks the
// builtin roles. Templates load even when their engine isn't installed; they
// are flagged via Available/Unavailable for the caller to act on.
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
			reg.add(raw, "builtin", "roles/"+e.Name())
		}
	}

	// Layer 2 + 3: user-level then project-level dirs.
	if dir := userRolesDir(); dir != "" {
		reg.loadDir(dir, "user")
	}
	if workspacePath != "" {
		reg.loadDir(filepath.Join(workspacePath, ".1agents", "agents"), "project")
	}

	return reg
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
		r.add(raw, source, path)
	}
}

// add parses one template and inserts it keyed by name, overriding any existing
// entry (later layers win). Parse failures are logged and skipped.
func (r *RoleRegistry) add(raw []byte, source, path string) {
	tpl, err := parseRoleMarkdown(raw)
	if err != nil {
		log.Printf("[agent] role %s: %v (skipped)", path, err)
		return
	}
	tpl.Source = source
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

// userRolesDir is the user-level role template directory (~/.1agents/agents),
// honoring ONEAGENTS_HOME first, mirroring internal/workspace's config dir.
func userRolesDir() string {
	if val := os.Getenv("ONEAGENTS_HOME"); val != "" {
		return filepath.Join(val, ".1agents", "agents")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".1agents", "agents")
}
