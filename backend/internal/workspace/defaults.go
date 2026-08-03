package workspace

import (
	_ "embed"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// claudeGuideTemplate is the default content written to a project's CLAUDE.md
// when it is missing.
//
//go:embed templates/project_guide.md
var claudeGuideTemplate string

// agentsGuideTemplate is the default content written to a project's AGENTS.md
// when it is missing.
//
//go:embed templates/agents_guide.md
var agentsGuideTemplate string

// guideFiles maps each agent guidance file ensured for every project workspace
// to its default template content.
var guideFiles = map[string]string{
	"CLAUDE.md": claudeGuideTemplate,
	"AGENTS.md": agentsGuideTemplate,
}

// scaffoldDirs are the agent infrastructure directories created alongside the
// guidance files for every new project workspace: .claude holds the
// Claude-native layout (agents/, skills/, settings), .agents the 1agents team
// layer (team.json).
var scaffoldDirs = []string{".claude", ".agents"}

// ensureProjectGuideFiles makes sure the workspace directory contains the agent
// guidance files (CLAUDE.md / AGENTS.md) and scaffold dirs (.claude / .agents).
// Missing files are created with the default behavioral-guidelines template;
// existing files and directories are left untouched.
func ensureProjectGuideFiles(dir string) {
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("[workspace] ensure guide dir %s: %v", dir, err)
		return
	}
	for name, content := range guideFiles {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			continue // already exists, leave untouched
		} else if !os.IsNotExist(err) {
			log.Printf("[workspace] stat %s: %v", path, err)
			continue
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			log.Printf("[workspace] write %s: %v", path, err)
			continue
		}
		log.Printf("[workspace] created %s", path)
	}
	for _, name := range scaffoldDirs {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			continue // already exists, leave untouched
		} else if !os.IsNotExist(err) {
			log.Printf("[workspace] stat %s: %v", path, err)
			continue
		}
		if err := os.MkdirAll(path, 0o755); err != nil {
			log.Printf("[workspace] mkdir %s: %v", path, err)
			continue
		}
		// git ignores empty directories; a .gitkeep keeps the fresh dir in the
		// initial commit.
		keep := filepath.Join(path, ".gitkeep")
		if err := os.WriteFile(keep, nil, 0o644); err != nil {
			log.Printf("[workspace] write %s: %v", keep, err)
		}
		log.Printf("[workspace] created %s", path)
	}
}

// gitInitProject initializes a local git repository in a freshly created
// project directory and records its initial state as an "init" commit.
// Best-effort by design: a missing git binary, an existing repository, or any
// failure only logs — project creation itself must never fail because of this.
func gitInitProject(dir string) {
	if dir == "" {
		return
	}
	gitBin, err := exec.LookPath("git")
	if err != nil {
		log.Printf("[workspace] git not installed, skip repository init for %s", dir)
		return
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return // already a repository — never re-init or auto-commit into it
	}
	run := func(args ...string) error {
		out, err := exec.Command(gitBin, append([]string{"-C", dir}, args...)...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	if err := run("init"); err != nil {
		log.Printf("[workspace] %v", err)
		return
	}
	// A repository without committer identity (no global git config) cannot
	// commit; seed a local one only for the missing keys.
	for key, fallback := range map[string]string{
		"user.name":  "1agents",
		"user.email": "1agents@localhost",
	} {
		if err := run("config", key); err != nil {
			if cerr := run("config", key, fallback); cerr != nil {
				log.Printf("[workspace] %v", cerr)
			}
		}
	}
	if err := run("add", "-A"); err != nil {
		log.Printf("[workspace] %v", err)
		return
	}
	// commit.gpgsign off: this is a daemon-side automatic commit and must not
	// hang on a GPG pinentry prompt.
	if err := run("-c", "commit.gpgsign=false", "commit", "-m", "init"); err != nil {
		log.Printf("[workspace] %v", err)
		return
	}
	log.Printf("[workspace] initialized git repository with 'init' commit at %s", dir)
}
