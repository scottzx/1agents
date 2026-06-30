package ccconnect

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/chenhg5/cc-connect/config"
)

// migrate.go implements the #277 Phase 4 one-shot migrator: it folds legacy
// `X__<agentA>` / `X__<agentB>` projects that share the same work_dir into a
// single de-suffixed project `X` with one channel per original project. Each
// original project's agent type becomes the corresponding channel's agent
// binding (Phase 1's [projects.platforms.agent]); platforms/providers/other
// fields are preserved. New-shape configs (no `__` suffix, no same-path
// collisions) pass through untouched, so the migrator is idempotent.

// stripAgentSuffix removes a trailing `__<agent>` suffix from a project name,
// returning the de-suffixed base. Falls back to filepath.Base(workDir) when the
// strip leaves an empty string. Mirrors the suffix handling already used in the
// runner's path-sync code.
func stripAgentSuffix(name, workDir string) string {
	base := name
	if idx := strings.LastIndex(name, "__"); idx >= 0 {
		base = strings.Trim(name[:idx], "_")
	}
	if base == "" {
		if workDir != "" {
			base = filepath.Base(workDir)
		} else {
			base = name
		}
	}
	return base
}

// hasAgentSuffix reports whether a project name carries a non-empty
// `__<agent>` suffix (e.g. "X__claudecode").
func hasAgentSuffix(name string) bool {
	idx := strings.LastIndex(name, "__")
	return idx >= 0 && strings.TrimSpace(name[idx+2:]) != ""
}

// MigrateLegacyAgentSuffixProjects folds legacy suffixed projects sharing a
// work_dir into single multi-channel projects (#277 Phase 4). It returns the
// migrated project slice and a changed flag; changed is false when the input is
// already in the new shape, making the call idempotent.
//
// Rules:
//   - Projects are grouped by normalized work_dir. Projects without a work_dir
//     (placeholders, platform-only configs) are passed through verbatim.
//   - Within a group, the first project (config order) provides the merged
//     project's identity (de-suffixed name) and default agent. Subsequent
//     projects contribute their platforms as channels bound to their own agent.
//   - A platform inherits the merged default agent (no [platforms.agent]) when
//     its source project's agent type equals the default; otherwise it gets an
//     explicit channel-level agent binding carrying that agent's work_dir.
//   - A source project with no platforms gets a synthesized bridge channel so
//     its agent remains reachable after the fold.
//   - Migration runs only when at least one group has >1 project OR any name
//     carries a `__<agent>` suffix; otherwise the input is returned unchanged.
func MigrateLegacyAgentSuffixProjects(projects []config.ProjectConfig) ([]config.ProjectConfig, bool) {
	// Decide up front whether any work is needed. This keeps the no-op path a
	// pure read and guarantees idempotency on a second run.
	needsMigration := false
	pathCounts := make(map[string]int)
	for _, p := range projects {
		wd, _ := p.Agent.Options["work_dir"].(string)
		if wd == "" {
			continue
		}
		np := normalizePath(wd)
		pathCounts[np]++
		if pathCounts[np] > 1 || hasAgentSuffix(p.Name) {
			needsMigration = true
		}
	}
	if !needsMigration {
		return projects, false
	}

	// Group work_dir projects by normalized path, preserving first-seen order so
	// the output is deterministic.
	type group struct {
		projs []config.ProjectConfig
	}
	var groups []*group
	byPath := make(map[string]*group)
	var passthrough []config.ProjectConfig // path-less placeholders / platform-only

	for _, p := range projects {
		wd, _ := p.Agent.Options["work_dir"].(string)
		if wd == "" {
			passthrough = append(passthrough, p)
			continue
		}
		np := normalizePath(wd)
		g, ok := byPath[np]
		if !ok {
			g = &group{}
			byPath[np] = g
			groups = append(groups, g)
		}
		g.projs = append(g.projs, p)
	}

	merged := make([]config.ProjectConfig, 0, len(groups))
	for _, g := range groups {
		merged = append(merged, foldGroup(g.projs))
	}

	// Path-backed merged projects first (stable order), then the path-less
	// passthroughs collected above.
	return append(merged, passthrough...), true
}

// foldGroup collapses one same-work_dir group into a single project. With a
// single member it merely de-suffixes the name (and keeps its channels). With
// multiple members it concatenates their channels, binding each member's agent
// to the channels it contributed.
func foldGroup(projs []config.ProjectConfig) config.ProjectConfig {
	first := projs[0]
	wd, _ := first.Agent.Options["work_dir"].(string)

	out := first
	out.Name = stripAgentSuffix(first.Name, wd)
	// The default agent for the folded project is the first member's agent.
	defaultType := first.Agent.Type

	platforms := make([]config.PlatformConfig, 0)
	for _, p := range projs {
		channels := p.Platforms
		if len(channels) == 0 {
			// A source project with no platforms still owns an agent; give it a
			// bridge channel so the binding survives the fold.
			channels = []config.PlatformConfig{{Type: "bridge"}}
		}
		for _, pc := range channels {
			platforms = append(platforms, bindChannelAgent(pc, p.Agent, defaultType, wd))
		}
	}
	out.Platforms = platforms
	return out
}

// bindChannelAgent returns a copy of pc with a channel-level agent binding for
// srcAgent, unless pc already carries its own binding or srcAgent matches the
// folded project's default type (in which case it inherits the default).
func bindChannelAgent(pc config.PlatformConfig, srcAgent config.AgentConfig, defaultType, workDir string) config.PlatformConfig {
	if pc.Agent != nil {
		// The channel already has an explicit binding; leave it untouched.
		return pc
	}
	if strings.EqualFold(srcAgent.Type, defaultType) {
		// Inherits the folded project default — no per-channel binding needed.
		return pc
	}
	bound := srcAgent
	// Ensure the channel agent runs in the project work_dir even if the source
	// project carried it only on the project-level agent options.
	if _, ok := bound.Options["work_dir"]; !ok && workDir != "" {
		opts := make(map[string]any, len(bound.Options)+1)
		for k, v := range bound.Options {
			opts[k] = v
		}
		opts["work_dir"] = workDir
		bound.Options = opts
	}
	pc.Agent = &bound
	return pc
}

// MigrateConfigFile loads the cc-connect config at path, folds legacy suffixed
// projects, and writes the result back only when something changed. It returns
// whether the file was modified. Used as a one-shot pass on the startup sync
// path (and directly callable / testable). Safe to call repeatedly: a no-op on
// already-migrated configs.
func MigrateConfigFile(path string) (bool, error) {
	cfg := &config.Config{}
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return false, fmt.Errorf("decode config for migration: %w", err)
	}
	migrated, changed := MigrateLegacyAgentSuffixProjects(cfg.Projects)
	if !changed {
		return false, nil
	}
	cfg.Projects = migrated
	if err := saveConfig(cfg, path); err != nil {
		return false, fmt.Errorf("save migrated config: %w", err)
	}
	return true, nil
}
