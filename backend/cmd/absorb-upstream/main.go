// Command absorb-upstream runs the open-source absorption pipeline (#188 / RFC
// §5): it transforms a curated set of SKILL.md files from the read-only upstream
// submodules (modules/superpowers, modules/gstack) into our format with
// provenance frontmatter, writes them into the backend embed dirs
// (internal/agent/{skills,roles}), and records each in
// internal/agent/.absorbed.json for incremental re-runs.
//
// Run from the backend dir:  go run ./cmd/absorb-upstream
// Re-running is cheap: items whose transformed output is unchanged are skipped.
//
// It never writes into the submodules — they are read-only reference (轨道 A).
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/scottzx/1Agents/backend/internal/agent"
)

func main() {
	repoRoot := flag.String("repo", "", "repo root (default: parent of backend/)")
	flag.Parse()

	root, err := resolveRepoRoot(*repoRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, "absorb-upstream:", err)
		os.Exit(1)
	}

	agentDir := filepath.Join(root, "backend", "internal", "agent")
	cfg := agent.AbsorbConfig{
		ModulesDir: filepath.Join(root, "modules"),
		SkillsDir:  filepath.Join(agentDir, "skills"),
		RolesDir:   filepath.Join(agentDir, "roles"),
		LedgerPath: filepath.Join(agentDir, ".absorbed.json"),
	}

	results, err := agent.Absorb(cfg, agent.DefaultAbsorbManifest())
	if err != nil {
		fmt.Fprintln(os.Stderr, "absorb-upstream:", err)
		os.Exit(1)
	}

	var written, skipped int
	for _, r := range results {
		fmt.Printf("%-8s %-5s %-30s %s\n", r.Action, r.Kind, r.Name, r.Source)
		if r.Action == "written" {
			written++
		} else {
			skipped++
		}
	}
	fmt.Printf("\n%d written, %d skipped (ledger: %s)\n", written, skipped, cfg.LedgerPath)
}

// resolveRepoRoot returns the explicit -repo flag, or infers the root by walking
// up from cwd until a dir containing both backend/ and modules/ is found.
func resolveRepoRoot(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if isDir(filepath.Join(dir, "backend")) && isDir(filepath.Join(dir, "modules")) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("cannot find repo root (no backend/ + modules/) above %s; pass -repo", dir)
		}
		dir = parent
	}
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}
