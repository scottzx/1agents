package workspace

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// newAssistantBadge mints an assistant's identity badge (工牌): today's date plus
// a 4-digit sequence that increments per day, e.g. 20260702-0001. The sequence is
// derived by scanning existing badge folders under ~/.1agents/projects for
// today's prefix, so restarts and gaps never reuse a number within the same day.
func newAssistantBadge() string {
	date := time.Now().Format("20060102")
	prefix := date + "-"
	root := filepath.Join(get1AgentsHome(), ".1agents", "projects")
	max := 0
	if entries, err := os.ReadDir(root); err == nil {
		for _, e := range entries {
			if !e.IsDir() || !strings.HasPrefix(e.Name(), prefix) {
				continue
			}
			if v, err := strconv.Atoi(e.Name()[len(prefix):]); err == nil && v > max {
				max = v
			}
		}
	}
	return fmt.Sprintf("%s-%04d", date, max+1)
}

// sharedSkillsRoot is the skill-manager shared store (the source of truth for
// installed skills). It lives under ~/.1agents to match the XDG overrides the
// skills supervisor pins for the python service — see supervisor/skills.go
// (skillsDataEnv → XDG_DATA_HOME=~/.1agents → data_dir=~/.1agents/skill-manager,
// skills_store_root=data_dir/"shared").
func sharedSkillsRoot() string {
	return filepath.Join(get1AgentsHome(), ".1agents", "skill-manager", "shared")
}

// normalizeSkillRef reduces a skill reference to its shared-store package dir
// name. The frontend picker may send a plain dir name ("foo") or a scoped ref
// ("shared:foo" / "centralized:foo"); either way we key off the package dir.
// filepath.Base guards against path traversal in the incoming ref.
func normalizeSkillRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if i := strings.LastIndex(ref, ":"); i >= 0 {
		ref = ref[i+1:]
	}
	return filepath.Base(ref)
}

// syncSkillsToWorkspace materializes the given skills (by shared-store package
// dir name) into a workspace as the "weak copy" of #360: the shared store stays
// the single source of truth, and each workspace gets a decoupled instance.
//
// Layout — one physical copy, one whole-directory symlink (same idea as
// CLAUDE.md/AGENTS.md):
//   - real copies → <ws>/.claude/skills/<name>  (Claude Code reads these natively)
//   - dir symlink → <ws>/.agents/skills → ../.claude/skills
//     (the generic agent convention; one link covers every current AND future
//     skill, so there's no per-skill upkeep)
//
// A relative symlink keeps the workspace portable if its directory moves.
// Symlinks inside a package are followed and copied as real files, so the
// .claude copy never dangles back into the shared store.
//
// Missing source skills are logged and skipped; an already-present real copy is
// left untouched (idempotent). Returns the package dirs actually synced.
func syncSkillsToWorkspace(workspacePath string, refs []string) ([]string, error) {
	if workspacePath == "" || len(refs) == 0 {
		return nil, nil
	}
	root := sharedSkillsRoot()
	storeRoot := filepath.Join(workspacePath, ".claude", "skills")
	if err := os.MkdirAll(storeRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create skills dir: %w", err)
	}
	synced := make([]string, 0, len(refs))
	for _, ref := range refs {
		name := normalizeSkillRef(ref)
		if name == "" || name == "." || name == ".." {
			continue
		}
		src := filepath.Join(root, name)
		if info, err := os.Stat(src); err != nil || !info.IsDir() {
			log.Printf("[workspace] skill %q not found in shared store (%s); skipped", name, root)
			continue
		}
		store := filepath.Join(storeRoot, name)
		if _, err := os.Stat(store); err != nil {
			if err := copyDir(src, store); err != nil {
				log.Printf("[workspace] copy skill %q: %v", name, err)
				continue
			}
		}
		synced = append(synced, name)
		log.Printf("[workspace] copied skill %q into %s", name, store)
	}
	// One whole-directory symlink instead of per-skill links: any skill in
	// .claude/skills (now or later) is reachable via .agents/skills for free.
	if err := linkAgentsSkills(workspacePath); err != nil {
		log.Printf("[workspace] link .agents/skills: %v", err)
	}
	return synced, nil
}

// linkAgentsSkills points <ws>/.agents/skills at <ws>/.claude/skills with a
// single relative directory symlink. Idempotent; a stale entry (old per-skill
// dir or wrong link) is removed and replaced.
func linkAgentsSkills(workspacePath string) error {
	agentsDir := filepath.Join(workspacePath, ".agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		return err
	}
	link := filepath.Join(agentsDir, "skills")
	return ensureSymlink(filepath.Join("..", ".claude", "skills"), link)
}

// ensureSymlink makes link a symlink pointing at target. A correct symlink is
// left as-is (idempotent); anything else already there (a stale symlink or an
// older real-copy directory) is removed and replaced.
func ensureSymlink(target, link string) error {
	if fi, err := os.Lstat(link); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			if cur, _ := os.Readlink(link); cur == target {
				return nil
			}
		}
		if err := os.RemoveAll(link); err != nil {
			return err
		}
	}
	return os.Symlink(target, link)
}

// copyDir recursively copies src to dest. Entries are resolved with Stat (not
// lstat), so symlinked files/dirs inside a package are materialized as real
// copies rather than links.
func copyDir(src, dest string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dest, info.Mode().Perm()); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dest, e.Name())
		si, err := os.Stat(s)
		if err != nil {
			log.Printf("[workspace] copySkill: stat %s: %v (skipped)", s, err)
			continue
		}
		if si.IsDir() {
			if err := copyDir(s, d); err != nil {
				return err
			}
			continue
		}
		if err := copyFile(s, d, si.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}

// copyFile copies a single regular file's contents from src to dest.
func copyFile(src, dest string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
