package harnesskitmigration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type bindingRoot struct {
	kind  string
	agent string
	path  string
	ext   string
}

func BuildPlan(cfg Config) (Plan, error) {
	cfg = normalizeConfig(cfg)
	if err := validateConfigPaths(cfg); err != nil {
		return Plan{}, err
	}
	plan := Plan{
		Version:           planVersion,
		MigrationID:       migrationID,
		LegacyDir:         cfg.LegacyDir,
		HarnessKitDataDir: cfg.HarnessKitDataDir,
		Counts:            map[string]int{},
	}
	legacyInfo, err := os.Stat(cfg.LegacyDir)
	if err != nil {
		if os.IsNotExist(err) {
			return plan, nil
		}
		return Plan{}, fmt.Errorf("inspect legacy data: %w", err)
	}
	if !legacyInfo.IsDir() {
		return Plan{}, fmt.Errorf("legacy path is not a directory: %s", cfg.LegacyDir)
	}
	plan.SourceExists = true
	legacyRoot, err := filepath.EvalSymlinks(cfg.LegacyDir)
	if err != nil {
		return Plan{}, fmt.Errorf("resolve legacy data: %w", err)
	}
	legacyRoot, _ = filepath.Abs(legacyRoot)
	plan.SourceFingerprint, err = fingerprintTree(legacyRoot)
	if err != nil {
		return Plan{}, fmt.Errorf("fingerprint legacy data: %w", err)
	}

	linkedSources := map[string]bool{}
	for _, root := range knownBindingRoots(cfg) {
		items, conflicts, sources, scanErr := scanBindingRoot(root, legacyRoot)
		if scanErr != nil {
			plan.Conflicts = append(plan.Conflicts, Conflict{
				Path: root.path, Kind: root.kind, Reason: scanErr.Error(),
			})
			continue
		}
		plan.Items = append(plan.Items, items...)
		plan.Conflicts = append(plan.Conflicts, conflicts...)
		for _, source := range sources {
			linkedSources[source] = true
		}
	}

	orphanItems, orphanConflicts, metadataLosses, metadata, err := inspectLegacyMetadata(cfg, legacyRoot, linkedSources)
	if err != nil {
		plan.Conflicts = append(plan.Conflicts, Conflict{
			Path: legacyRoot, Kind: "legacy-metadata", Reason: err.Error(),
		})
	}
	plan.Items = append(plan.Items, orphanItems...)
	plan.Conflicts = append(plan.Conflicts, orphanConflicts...)
	plan.LegacyMetadata = metadata
	plan.Losses = append(plan.Losses, metadataLosses...)

	for _, configPath := range knownMCPConfigPaths(cfg) {
		info, statErr := os.Lstat(configPath)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				continue
			}
			plan.Conflicts = append(plan.Conflicts, Conflict{
				Path: configPath, Kind: "mcp", Reason: statErr.Error(),
			})
			continue
		}
		item, conflict, source := inspectBinding(configPath, "mcp", "native-config", legacyRoot, info)
		if conflict != nil {
			plan.Conflicts = append(plan.Conflicts, *conflict)
		} else {
			plan.Items = append(plan.Items, item)
			if source != "" {
				linkedSources[source] = true
			}
		}
	}

	sort.Slice(plan.Items, func(i, j int) bool {
		if plan.Items[i].Path == plan.Items[j].Path {
			return plan.Items[i].Kind < plan.Items[j].Kind
		}
		return plan.Items[i].Path < plan.Items[j].Path
	})
	sort.Slice(plan.Conflicts, func(i, j int) bool { return plan.Conflicts[i].Path < plan.Conflicts[j].Path })
	sort.Slice(plan.Losses, func(i, j int) bool {
		if plan.Losses[i].Kind == plan.Losses[j].Kind {
			return plan.Losses[i].Path < plan.Losses[j].Path
		}
		return plan.Losses[i].Kind < plan.Losses[j].Kind
	})
	for _, item := range plan.Items {
		plan.Counts[item.Kind+":"+item.Action]++
	}
	plan.Counts["conflicts"] = len(plan.Conflicts)
	plan.Counts["preserved_not_imported"] = len(plan.Losses)
	return plan, nil
}

func knownBindingRoots(cfg Config) []bindingRoot {
	configHome := filepath.Join(cfg.Home, ".config")
	return []bindingRoot{
		{kind: "skill", agent: "codex-compatible", path: filepath.Join(cfg.Home, ".agents", "skills")},
		{kind: "skill", agent: "codex-legacy", path: filepath.Join(cfg.Home, ".codex", "skills")},
		{kind: "skill", agent: "claude", path: filepath.Join(cfg.Home, ".claude", "skills")},
		{kind: "skill", agent: "cursor", path: filepath.Join(cfg.Home, ".cursor", "skills")},
		{kind: "skill", agent: "opencode", path: filepath.Join(configHome, "opencode", "skills")},
		{kind: "skill", agent: "openclaw", path: filepath.Join(cfg.Home, ".openclaw", "skills")},
		{kind: "skill", agent: "grok", path: filepath.Join(cfg.Home, ".grok", "skills")},
		{kind: "subagent", agent: "claude", path: filepath.Join(cfg.Home, ".claude", "agents"), ext: ".md"},
		{kind: "subagent", agent: "grok", path: filepath.Join(cfg.Home, ".grok", "agents"), ext: ".md"},
		{kind: "command", agent: "codex", path: filepath.Join(cfg.Home, ".codex", "prompts"), ext: ".md"},
		{kind: "command", agent: "claude", path: filepath.Join(cfg.Home, ".claude", "commands"), ext: ".md"},
		{kind: "command", agent: "cursor", path: filepath.Join(cfg.Home, ".cursor", "commands"), ext: ".md"},
		{kind: "command", agent: "opencode", path: filepath.Join(configHome, "opencode", "commands"), ext: ".md"},
		{kind: "command", agent: "grok", path: filepath.Join(cfg.Home, ".grok", "commands"), ext: ".md"},
	}
}

func knownMCPConfigPaths(cfg Config) []string {
	return []string{
		filepath.Join(cfg.Home, ".codex", "config.toml"),
		filepath.Join(cfg.Home, ".claude.json"),
		filepath.Join(cfg.Home, ".cursor", "mcp.json"),
		filepath.Join(cfg.Home, ".opencode", "opencode.jsonc"),
		filepath.Join(cfg.Home, ".config", "opencode", "opencode.json"),
		filepath.Join(cfg.Home, ".openclaw", "openclaw.json"),
		filepath.Join(cfg.Home, ".grok", "config.toml"),
	}
}

func scanBindingRoot(root bindingRoot, legacyRoot string) ([]Item, []Conflict, []string, error) {
	entries, err := os.ReadDir(root.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil, nil
		}
		return nil, nil, nil, err
	}
	var items []Item
	var conflicts []Conflict
	var sources []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if root.ext != "" && filepath.Ext(entry.Name()) != root.ext {
			continue
		}
		path := filepath.Join(root.path, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			conflicts = append(conflicts, Conflict{Path: path, Kind: root.kind, Reason: err.Error()})
			continue
		}
		if root.kind == "skill" && info.Mode()&os.ModeSymlink == 0 && !info.IsDir() {
			conflicts = append(conflicts, Conflict{
				Path: path, Kind: root.kind, Reason: "skill binding is not a directory",
			})
			continue
		}
		item, conflict, source := inspectBinding(path, root.kind, root.agent, legacyRoot, info)
		if conflict != nil {
			conflicts = append(conflicts, *conflict)
			continue
		}
		items = append(items, item)
		if source != "" {
			sources = append(sources, source)
		}
	}
	return items, conflicts, sources, nil
}

func inspectBinding(path, kind, agent, legacyRoot string, info os.FileInfo) (Item, *Conflict, string) {
	item := Item{Kind: kind, Path: path, Agent: agent}
	if info.Mode()&os.ModeSymlink == 0 {
		fingerprint, err := fingerprintTree(path)
		if err != nil {
			return Item{}, &Conflict{Path: path, Kind: kind, Reason: err.Error()}, ""
		}
		item.Action = "preserve-native"
		item.Fingerprint = fingerprint
		item.Reason = "already stored in the Agent-native location"
		return item, nil, ""
	}

	linkTarget, err := os.Readlink(path)
	if err != nil {
		return Item{}, &Conflict{Path: path, Kind: kind, Reason: err.Error()}, ""
	}
	resolved, fingerprint, err := fingerprintResolved(path)
	if err != nil {
		return Item{}, &Conflict{Path: path, Kind: kind, Reason: "broken or cyclic symlink: " + err.Error()}, ""
	}
	item.LinkTarget = linkTarget
	item.SourcePath = resolved
	item.Fingerprint = fingerprint
	if !pathWithin(resolved, legacyRoot) {
		item.Action = "preserve-external-symlink"
		item.Reason = "symlink target is outside the legacy Skills-manager data directory"
		return item, nil, ""
	}
	item.Action = "materialize"
	item.Reason = "replace the managed symlink with an equivalent native copy"
	return item, nil, resolved
}

func inspectLegacyMetadata(cfg Config, legacyRoot string, linkedSources map[string]bool) ([]Item, []Conflict, []Loss, LegacyMetadata, error) {
	var items []Item
	var conflicts []Conflict
	var losses []Loss
	var metadata LegacyMetadata
	var firstErr error

	skillManifestPath := filepath.Join(legacyRoot, "manifest.json")
	metadata.SkillManifestEntries, firstErr = countManifestEntries(skillManifestPath)
	if _, err := os.Stat(skillManifestPath); err == nil {
		losses = append(losses, Loss{
			Kind: "skill-source-metadata", Path: skillManifestPath,
			Disposition: "preserved-not-imported",
			Reason:      "version, fork lineage, primary branch and historical source fields have no direct HarnessKit equivalent",
		})
	}
	agentManifestPath := filepath.Join(legacyRoot, "agents-manifest.json")
	count, err := countManifestEntries(agentManifestPath)
	metadata.AgentManifestEntries = count
	if firstErr == nil {
		firstErr = err
	}
	if _, err := os.Stat(agentManifestPath); err == nil {
		losses = append(losses, Loss{
			Kind: "subagent-source-metadata", Path: agentManifestPath,
			Disposition: "preserved-not-imported",
			Reason:      "legacy source locator and revision metadata is retained in backup",
		})
	}

	for _, sourceRoot := range []struct {
		kind       string
		path       string
		targetRoot string
		agent      string
		ext        string
	}{
		{
			kind: "skill", path: filepath.Join(legacyRoot, "shared"),
			targetRoot: filepath.Join(cfg.Home, ".agents", "skills"), agent: "codex-compatible",
		},
		{
			kind: "subagent", path: filepath.Join(legacyRoot, "agents"),
			targetRoot: filepath.Join(cfg.Home, ".claude", "agents"), agent: "claude", ext: ".md",
		},
	} {
		entries, _ := os.ReadDir(sourceRoot.path)
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			if sourceRoot.ext != "" && filepath.Ext(entry.Name()) != sourceRoot.ext {
				continue
			}
			source := filepath.Join(sourceRoot.path, entry.Name())
			resolved, resolveErr := filepath.EvalSymlinks(source)
			if resolveErr != nil {
				conflicts = append(conflicts, Conflict{
					Path: source, Kind: sourceRoot.kind, Reason: "cannot resolve central-store item: " + resolveErr.Error(),
				})
				continue
			}
			resolved, _ = filepath.Abs(resolved)
			if linkedSources[resolved] {
				continue
			}
			sourceInfo, statErr := os.Stat(resolved)
			if statErr != nil || (sourceRoot.kind == "skill" && !sourceInfo.IsDir()) ||
				(sourceRoot.kind == "subagent" && !sourceInfo.Mode().IsRegular()) {
				conflicts = append(conflicts, Conflict{
					Path: source, Kind: sourceRoot.kind, Reason: "central-store item has an unsupported file type",
				})
				continue
			}
			if sourceRoot.kind == "skill" {
				if info, skillErr := os.Stat(filepath.Join(resolved, "SKILL.md")); skillErr != nil || !info.Mode().IsRegular() {
					conflicts = append(conflicts, Conflict{
						Path: source, Kind: sourceRoot.kind, Reason: "central-store skill has no regular SKILL.md",
					})
					continue
				}
			}
			sourceFingerprint, fingerprintErr := fingerprintTree(resolved)
			if fingerprintErr != nil {
				conflicts = append(conflicts, Conflict{
					Path: source, Kind: sourceRoot.kind, Reason: fingerprintErr.Error(),
				})
				continue
			}
			target := filepath.Join(sourceRoot.targetRoot, entry.Name())
			targetInfo, targetErr := os.Lstat(target)
			if targetErr == nil {
				targetFingerprint, fingerprintErr := bindingFingerprint(target, targetInfo)
				if fingerprintErr != nil {
					conflicts = append(conflicts, Conflict{
						Path: target, Kind: sourceRoot.kind, Reason: fingerprintErr.Error(),
					})
					continue
				}
				if targetFingerprint != sourceFingerprint {
					conflicts = append(conflicts, Conflict{
						Path: target, Kind: sourceRoot.kind,
						Reason: "preferred native import target exists with different content",
					})
				}
				continue
			}
			if !os.IsNotExist(targetErr) {
				conflicts = append(conflicts, Conflict{
					Path: target, Kind: sourceRoot.kind, Reason: targetErr.Error(),
				})
				continue
			}
			items = append(items, Item{
				Kind: sourceRoot.kind, Action: "copy-orphan", Path: target,
				SourcePath: resolved, Fingerprint: sourceFingerprint, Agent: sourceRoot.agent,
				Reason: "import central-store item that has no active Agent-native binding",
			})
		}
	}

	commandDir := filepath.Join(legacyRoot, "slash-commands", "commands")
	commandEntries, _ := filepath.Glob(filepath.Join(commandDir, "*.toml"))
	metadata.SlashCommands = len(commandEntries)
	for _, path := range commandEntries {
		losses = append(losses, Loss{
			Kind: "command-canonical-record", Path: path, Name: strings.TrimSuffix(filepath.Base(path), ".toml"),
			Disposition: "preserved-not-imported",
			Reason:      "native rendered command files remain active; the canonical TOML record is archived",
		})
	}
	metadata.SlashSyncRecords = countSlashSyncRecords(filepath.Join(legacyRoot, "slash-commands", "sync-state.json"))

	mcpPath := filepath.Join(legacyRoot, "mcp", "manifest.json")
	mcpNames, mcpErr := readMCPNames(mcpPath)
	if firstErr == nil {
		firstErr = mcpErr
	}
	metadata.MCPServers = len(mcpNames)
	metadata.MCPServerNames = mcpNames
	if len(mcpNames) > 0 {
		losses = append(losses, Loss{
			Kind: "mcp-canonical-record", Path: mcpPath,
			Disposition: "preserved-not-imported",
			Reason:      "native Agent configurations remain active; secret-bearing canonical records stay only in the restricted backup",
		})
	}

	historyRoot := filepath.Join(legacyRoot, "history")
	metadata.HistoryPackages = countDirectories(historyRoot)
	if metadata.HistoryPackages > 0 {
		losses = append(losses, Loss{
			Kind: "skill-history", Path: historyRoot,
			Disposition: "preserved-not-imported",
			Reason:      "historical versions are archived and are not active HarnessKit extensions",
		})
	}
	pendingPath := filepath.Join(legacyRoot, "pending-conflicts", "pending_conflicts.json")
	metadata.PendingConflicts, _ = countManifestEntries(pendingPath)
	if metadata.PendingConflicts > 0 {
		losses = append(losses, Loss{
			Kind: "pending-conflict", Path: pendingPath,
			Disposition: "preserved-not-imported",
			Reason:      "unresolved mother-store push conflicts cannot be represented by the in-place model",
		})
	}
	return items, conflicts, losses, metadata, firstErr
}

func countManifestEntries(path string) (int, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	var document map[string]any
	if err := json.Unmarshal(payload, &document); err != nil {
		return 0, fmt.Errorf("%s: %w", path, err)
	}
	for _, key := range []string{"entries", "servers"} {
		if entries, ok := document[key].([]any); ok {
			return len(entries), nil
		}
	}
	return 0, nil
}

func countSlashSyncRecords(path string) int {
	var payload map[string]any
	if readJSON(path, &payload) != nil {
		return 0
	}
	commands, _ := payload["commands"].(map[string]any)
	total := 0
	for _, raw := range commands {
		if records, ok := raw.(map[string]any); ok {
			total += len(records)
		}
	}
	return total
}

func readMCPNames(path string) ([]string, error) {
	var payload map[string]any
	if err := readJSON(path, &payload); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	servers, _ := payload["servers"].([]any)
	var names []string
	for _, raw := range servers {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if name, ok := entry["name"].(string); ok && name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

func countDirectories(path string) int {
	entries, err := os.ReadDir(path)
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			count++
		}
	}
	return count
}
