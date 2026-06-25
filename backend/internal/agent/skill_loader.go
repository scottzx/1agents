package agent

import (
	"embed"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sync"
)

//go:embed skills/*/SKILL.md
var builtinSkillsFS embed.FS

// SkillRegistry holds skills merged by name across the builtin, user, and
// project layers (later layers override earlier ones by the skill's name field).
// It is the skill-side twin of RoleRegistry; roles bind skills by name.
type SkillRegistry struct {
	mu     sync.RWMutex
	skills map[string]*Skill
}

// Resolve returns the skill for a name.
func (r *SkillRegistry) Resolve(name string) (*Skill, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	sk, ok := r.skills[name]
	return sk, ok
}

// Names returns the resolved skill names (unordered). Used by tests/diagnostics.
func (r *SkillRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.skills))
	for n := range r.skills {
		out = append(out, n)
	}
	return out
}

// LoadSkills builds a skill registry from three layers, lowest precedence first:
//
//	builtin (embedded) → user (~/.1agents/skills) → project (<ws>/.1agents/skills)
//
// Each skill is a directory containing a SKILL.md (Claude Code / superpowers
// convention), so existing skill folders import as-is. Same-named skills in a
// higher layer override lower ones. Per-file parse errors are logged and skipped.
func LoadSkills(workspacePath string) *SkillRegistry {
	reg := &SkillRegistry{skills: make(map[string]*Skill)}

	// Layer 1: builtin (embedded). Walk skills/<name>/SKILL.md.
	_ = fs.WalkDir(builtinSkillsFS, "skills", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "SKILL.md" {
			return nil
		}
		raw, rerr := builtinSkillsFS.ReadFile(path)
		if rerr != nil {
			log.Printf("[agent] builtin skill %s: read failed: %v", path, rerr)
			return nil
		}
		reg.add(raw, "builtin", path)
		return nil
	})

	// Layer 2 + 3: user-level then project-level dirs.
	if dir := userSkillsDir(); dir != "" {
		reg.loadDir(dir, "user")
	}
	if workspacePath != "" {
		reg.loadDir(filepath.Join(workspacePath, ".1agents", "skills"), "project")
	}

	return reg
}

// loadDir parses every <name>/SKILL.md under dir (one level deep). A missing dir
// is a no-op; per-skill errors are logged and skipped.
func (r *SkillRegistry) loadDir(dir, source string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return // missing dir is fine
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name(), "SKILL.md")
		raw, err := os.ReadFile(path)
		if err != nil {
			continue // dir without a SKILL.md is not a skill
		}
		r.add(raw, source, path)
	}
}

// add parses one skill and inserts it keyed by name, overriding any existing
// entry (later layers win). Parse failures are logged and skipped.
func (r *SkillRegistry) add(raw []byte, source, path string) {
	sk, err := parseSkillMarkdown(raw)
	if err != nil {
		log.Printf("[agent] skill %s: %v (skipped)", path, err)
		return
	}
	sk.Source = source
	r.mu.Lock()
	r.skills[sk.Name] = sk
	r.mu.Unlock()
}

// userSkillsDir is the user-level skill directory (~/.1agents/skills), honoring
// ONEAGENTS_HOME first, mirroring userRolesDir.
func userSkillsDir() string {
	if val := os.Getenv("ONEAGENTS_HOME"); val != "" {
		return filepath.Join(val, ".1agents", "skills")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".1agents", "skills")
}
