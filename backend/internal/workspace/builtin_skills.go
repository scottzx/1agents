package workspace

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// builtinSkillsFS embeds the skill packages that ship with 1agents itself and
// are seeded into the skill-manager shared store on startup, so they behave like
// any installed skill (materialized into projects via syncSkillsToWorkspace) but
// require no marketplace/import step. Today that's just "pm" — the project-manager
// skill that drives the `1agents project-items` CLI.
//
//go:embed builtinskills
var builtinSkillsFS embed.FS

// builtinSkills lists the embedded packages to seed. dir is the embed subdir AND
// the shared-store package dir. id is a stable skill id (so the same builtin keeps
// one identity across installs). Bump version when the embedded content changes
// in a way you want existing untouched copies to pick up.
var builtinSkills = []struct {
	dir     string
	id      string
	version int
}{
	{dir: "pm", id: "skl_builtinpm00001", version: 1},
}

// EnsureBuiltinSkills seeds the embedded builtin skill packages into the
// skill-manager shared store (~/.1agents/skill-manager/shared/<dir>) and registers
// each in the store manifest so it shows up as a first-class library skill. It is
// idempotent and create-only: a package that already exists on disk is left
// untouched (never clobbers a user edit); a missing manifest entry is added.
//
// Call this at daemon startup, before EnsureDefaultWorkspace / any project create
// that auto-attaches these skills, and ideally before the skill-manager service is
// contended (the manifest write only happens the first time an entry is absent).
func EnsureBuiltinSkills() {
	root := sharedSkillsRoot()
	if err := os.MkdirAll(root, 0o755); err != nil {
		log.Printf("[workspace] builtin skills: ensure shared root: %v", err)
		return
	}
	for _, bs := range builtinSkills {
		dest := filepath.Join(root, bs.dir)
		if _, err := os.Stat(filepath.Join(dest, "SKILL.md")); err != nil {
			// Not present (or unreadable) — seed the embedded package.
			if err := writeEmbeddedTree("builtinskills/"+bs.dir, dest); err != nil {
				log.Printf("[workspace] builtin skill %q: seed failed: %v", bs.dir, err)
				continue
			}
			// The skill invokes the `project-items` CLI, which is a subcommand of
			// THIS daemon binary. It's not on PATH, so bake the resolved absolute
			// path into the seeded copy (done before fingerprinting so the manifest
			// revision matches the on-disk content).
			rewriteCLIBinaryPath(dest)
			writeSkillMeta(dest, bs.id, bs.version)
			log.Printf("[workspace] seeded builtin skill %q into %s", bs.dir, dest)
		}
		// Register in the store manifest if absent (so it lists in the 1skills UI).
		if err := ensureManifestEntry(bs.dir, bs.id, bs.version); err != nil {
			log.Printf("[workspace] builtin skill %q: manifest register: %v", bs.dir, err)
		}
	}
}

// writeEmbeddedTree copies an embed.FS subtree (embedRoot) onto disk at destRoot.
func writeEmbeddedTree(embedRoot, destRoot string) error {
	return fs.WalkDir(builtinSkillsFS, embedRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(embedRoot, p)
		if err != nil {
			return err
		}
		target := filepath.Join(destRoot, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := builtinSkillsFS.ReadFile(p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// rewriteCLIBinaryPath replaces the `1agents project-items` command literal in
// the seeded skill's markdown with `<abs-daemon-binary> project-items`, so the
// agent can invoke the CLI even though `1agents` isn't on PATH. The daemon binary
// carries the `project-items` subcommand, so its own path is the CLI. No-op if the
// executable can't be resolved (the skill's `1agents project-items` fallback text
// still applies).
func rewriteCLIBinaryPath(dest string) {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		return
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	replacement := shellQuote(exe) + " project-items"
	for _, name := range []string{"SKILL.md", filepath.Join("references", "cli.md")} {
		p := filepath.Join(dest, name)
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		out := strings.ReplaceAll(string(data), "1agents project-items", replacement)
		if out != string(data) {
			_ = os.WriteFile(p, []byte(out), 0o644)
		}
	}
}

// shellQuote wraps s in single quotes for safe use as one shell token when it
// contains characters a shell would otherwise interpret (spaces, etc.).
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	safe := true
	for _, r := range s {
		if !(r == '/' || r == '.' || r == '-' || r == '_' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r > 127) {
			safe = false
			break
		}
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// writeSkillMeta writes the stable-id sidecar (#379) the skill-manager expects.
// The fingerprint deliberately excludes this file, so its contents don't count as
// skill content.
func writeSkillMeta(dest, id string, version int) {
	meta := map[string]any{
		"id":          id,
		"baseVersion": version,
		"createdAt":   time.Now().UTC().Format(time.RFC3339),
	}
	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return
	}
	if err := os.WriteFile(filepath.Join(dest, ".skillmeta.json"), b, 0o644); err != nil {
		log.Printf("[workspace] write .skillmeta.json for %s: %v", dest, err)
	}
}

// ensureManifestEntry adds a store-manifest entry for packageDir if one isn't
// already present. Matches the skill-manager's SkillStoreManifest JSON shape and
// computes the same content fingerprint (see fingerprint_package in the python
// skill_manager). No-ops (no write) when the entry already exists, so after the
// first seed there is no contention with the running skill-manager service.
func ensureManifestEntry(packageDir, id string, version int) error {
	manifestPath := filepath.Join(get1AgentsHome(), ".1agents", "skill-manager", "manifest.json")
	type entry struct {
		PackageDir    string `json:"packageDir"`
		DeclaredName  string `json:"declaredName"`
		SourceKind    string `json:"sourceKind"`
		SourceLocator string `json:"sourceLocator"`
		Revision      string `json:"revision"`
		Version       int    `json:"version"`
		ID            string `json:"id"`
		IsPrimary     bool   `json:"isPrimary"`
	}
	type manifest struct {
		Entries []entry `json:"entries"`
	}
	var m manifest
	if data, err := os.ReadFile(manifestPath); err == nil {
		_ = json.Unmarshal(data, &m)
	}
	for _, e := range m.Entries {
		if e.PackageDir == packageDir {
			return nil // already registered
		}
	}
	rev, err := fingerprintPackage(filepath.Join(sharedSkillsRoot(), packageDir))
	if err != nil {
		return err
	}
	m.Entries = append(m.Entries, entry{
		PackageDir:    packageDir,
		DeclaredName:  packageDir,
		SourceKind:    "centralized",
		SourceLocator: "centralized:" + packageDir,
		Revision:      rev,
		Version:       version,
		ID:            id,
		IsPrimary:     true,
	})
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(manifestPath, b, 0o644)
}

// fingerprintPackage replicates skill_manager's fingerprint_package: sha256 over
// every file under root (recursive, sorted by relative posix path), excluding the
// .skillmeta.json sidecar and .DS_Store; each contributes relpath + NUL + bytes +
// NUL. Must stay byte-compatible with the python side so the manifest revision
// matches what the service would compute.
func fingerprintPackage(root string) (string, error) {
	var rels []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := filepath.Base(p)
		if name == ".DS_Store" || name == ".skillmeta.json" {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rels = append(rels, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(rels)
	h := sha256.New()
	for _, rel := range rels {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return "", err
		}
		h.Write([]byte(rel))
		h.Write([]byte{0})
		h.Write(data)
		h.Write([]byte{0})
	}
	if !containsString(rels, "SKILL.md") {
		return "", fmt.Errorf("missing SKILL.md in %s", root)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func containsString(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
