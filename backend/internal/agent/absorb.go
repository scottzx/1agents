package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// 吸收管线 (#188 / RFC §5)。把上游开源项目 (superpowers / gstack) 的 SKILL.md
// 「转化」成我们的格式并落进后端 embed 仓 (skills/ 与 roles/)，复用 #187 的
// parseSkillMarkdown / parseRoleMarkdown 解析能力，只补 provenance frontmatter
// (source / license / upstream_sha) 并写一份 .absorbed.json 做增量同步。
//
// 双轨 (RFC §5)：
//   - 轨道 A：modules/superpowers、modules/gstack 作为 submodule 只读参照源，
//     钉 SHA，不参与构建、不在此修改。
//   - 轨道 B：转化成品落 backend/internal/agent/{skills,roles}/，随二进制 embed。
//
// 增量同步：.absorbed.json 按目标名记 {source, upstream_sha, content_hash}。
// 重复运行时 content_hash 未变的项直接跳过 (= kwiki .ingested.json 思路)。

// AbsorbKind is whether a manifest entry becomes a skill or a role.
type AbsorbKind string

const (
	AbsorbSkill AbsorbKind = "skill"
	AbsorbRole  AbsorbKind = "role"
)

// AbsorbEntry is one curated item to absorb from an upstream submodule. The set
// is curated (not "absorb everything") on purpose: gstack's command SKILL.md
// files are auto-generated and code-backed (shell preambles referencing
// ~/.claude/skills/.../bin), which we must not vendor (RFC §10). The manifest is
// the allowlist of what is format-clean enough to transform.
type AbsorbEntry struct {
	// Source is the upstream project id, also the provenance label ("superpowers"
	// | "gstack"). It selects which submodule dir the SrcPath is relative to.
	Source string
	// SrcPath is the SKILL.md path relative to the submodule root.
	SrcPath string
	// Kind decides the sink: AbsorbSkill → skills/<name>/SKILL.md, AbsorbRole →
	// roles/<name>.md.
	Kind AbsorbKind
	// Name overrides the absorbed item's name (and the on-disk file/dir name).
	// Empty keeps the upstream frontmatter name.
	Name string
}

// AbsorbConfig points the absorber at the upstream submodules and the sinks.
type AbsorbConfig struct {
	// ModulesDir is the repo's modules/ dir holding the upstream submodules.
	ModulesDir string
	// SkillsDir is the backend embed skills dir (sink for AbsorbSkill).
	SkillsDir string
	// RolesDir is the backend embed roles dir (sink for AbsorbRole).
	RolesDir string
	// LedgerPath is the .absorbed.json ledger path.
	LedgerPath string
}

// absorbRecord is one ledger entry: enough to detect upstream change (SHA) and
// to skip re-writes when our transformed output is unchanged (content hash).
type absorbRecord struct {
	Source      string `json:"source"`
	UpstreamSHA string `json:"upstream_sha"`
	ContentHash string `json:"content_hash"`
	Kind        string `json:"kind"`
}

// AbsorbResult reports, per absorbed name, what happened (written | skipped).
type AbsorbResult struct {
	Name    string
	Kind    AbsorbKind
	Source  string
	Action  string // "written" | "skipped"
	DstPath string
}

// licenseBySource maps an upstream id to its SPDX license. Both projects are MIT
// (verified from their LICENSE files); recorded per-file for attribution.
var licenseBySource = map[string]string{
	"superpowers": "MIT",
	"gstack":      "MIT",
}

// DefaultAbsorbManifest is the curated first batch (RFC §5 拆分规则):
//   - superpowers ≈ 格式同构 → 过程方法技能整批进 skills。
//   - gstack → 拆：方法→技能、角色身份→角色仓。仅取 format-clean 的少量，
//     code-backed 的 (/browse 等) 不在内 (重映射到自有能力，不 vendor 代码)。
func DefaultAbsorbManifest() []AbsorbEntry {
	var m []AbsorbEntry
	for _, name := range []string{
		"brainstorming",
		"writing-plans",
		"executing-plans",
		"test-driven-development",
		"systematic-debugging",
		"verification-before-completion",
		"requesting-code-review",
		"receiving-code-review",
	} {
		m = append(m, AbsorbEntry{
			Source:  "superpowers",
			SrcPath: filepath.Join("skills", name, "SKILL.md"),
			Kind:    AbsorbSkill,
		})
	}
	// gstack: one role (cso → "员工"角色仓) + one methodology skill.
	m = append(m,
		AbsorbEntry{Source: "gstack", SrcPath: "design-shotgun/SKILL.md", Kind: AbsorbSkill},
		AbsorbEntry{Source: "gstack", SrcPath: "cso/SKILL.md", Kind: AbsorbRole},
	)
	return m
}

// Absorb runs the manifest against cfg: for each entry it reads the upstream
// SKILL.md (read-only reference), transforms it into our format with provenance
// frontmatter, and writes it to the sink — but only when the transformed content
// changed since the last run (per the .absorbed.json ledger). The ledger is
// rewritten at the end. Returns one result per entry.
func Absorb(cfg AbsorbConfig, manifest []AbsorbEntry) ([]AbsorbResult, error) {
	ledger, err := loadLedger(cfg.LedgerPath)
	if err != nil {
		return nil, err
	}
	upstreamSHAs := map[string]string{}

	var results []AbsorbResult
	for _, e := range manifest {
		subRoot := filepath.Join(cfg.ModulesDir, e.Source)
		src := filepath.Join(subRoot, e.SrcPath)
		raw, err := os.ReadFile(src)
		if err != nil {
			return nil, fmt.Errorf("read upstream %s: %w", src, err)
		}

		sha, ok := upstreamSHAs[e.Source]
		if !ok {
			sha = submoduleSHA(subRoot)
			upstreamSHAs[e.Source] = sha
		}
		license := licenseBySource[e.Source]

		name, out, err := transform(e, raw, license, sha)
		if err != nil {
			return nil, fmt.Errorf("transform %s: %w", src, err)
		}

		hash := hashBytes(out)
		dst := sinkPath(cfg, e.Kind, name)

		// Skip when the transformed output is byte-identical to last run AND the
		// file is still on disk. Re-emit otherwise (upstream changed or missing).
		if rec, found := ledger[name]; found && rec.ContentHash == hash && fileExists(dst) {
			results = append(results, AbsorbResult{Name: name, Kind: e.Kind, Source: e.Source, Action: "skipped", DstPath: dst})
			continue
		}

		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
		}
		if err := os.WriteFile(dst, out, 0o644); err != nil {
			return nil, fmt.Errorf("write %s: %w", dst, err)
		}
		ledger[name] = absorbRecord{Source: e.Source, UpstreamSHA: sha, ContentHash: hash, Kind: string(e.Kind)}
		results = append(results, AbsorbResult{Name: name, Kind: e.Kind, Source: e.Source, Action: "written", DstPath: dst})
	}

	if err := saveLedger(cfg.LedgerPath, ledger); err != nil {
		return nil, err
	}
	return results, nil
}

// transform parses an upstream SKILL.md (reusing the #187 parsers) and re-emits
// it in our format with provenance frontmatter. It returns the resolved name and
// the rendered bytes. A skill becomes name/description + provenance + body; a
// role is parsed as a role template and rendered with provenance appended.
func transform(e AbsorbEntry, raw []byte, license, sha string) (string, []byte, error) {
	prov := provenance{source: e.Source, license: license, upstreamSHA: sha}
	switch e.Kind {
	case AbsorbSkill:
		sk, err := parseSkillMarkdown(raw)
		if err != nil {
			return "", nil, err
		}
		name := sk.Name
		if e.Name != "" {
			name = e.Name
		}
		return name, renderAbsorbedSkill(name, sk.Description, prov, sk.Body), nil
	case AbsorbRole:
		tpl, err := parseImportableRole(raw)
		if err != nil {
			return "", nil, err
		}
		if tpl.Engine == "" {
			tpl.Engine = "claude-code"
		}
		name := tpl.Name
		if e.Name != "" {
			name = e.Name
			tpl.Name = e.Name
		}
		return name, renderAbsorbedRole(tpl, prov), nil
	default:
		return "", nil, fmt.Errorf("unknown kind %q", e.Kind)
	}
}

// provenance is the source/license/SHA triple written into absorbed frontmatter.
type provenance struct {
	source      string
	license     string
	upstreamSHA string
}

// writeFrontmatter appends the provenance lines to a frontmatter builder.
func (p provenance) write(b *strings.Builder) {
	if p.source != "" {
		fmt.Fprintf(b, "source: %s\n", p.source)
	}
	if p.license != "" {
		fmt.Fprintf(b, "license: %s\n", p.license)
	}
	if p.upstreamSHA != "" {
		fmt.Fprintf(b, "upstream_sha: %s\n", p.upstreamSHA)
	}
}

// renderAbsorbedSkill emits a SKILL.md in our format: name + description +
// provenance frontmatter, then the original markdown body.
func renderAbsorbedSkill(name, description string, prov provenance, body string) []byte {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "name: %s\n", name)
	if description != "" {
		fmt.Fprintf(&b, "description: %s\n", yamlScalar(description))
	}
	prov.write(&b)
	b.WriteString("---\n")
	if body != "" {
		b.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			b.WriteByte('\n')
		}
	}
	return []byte(b.String())
}

// renderAbsorbedRole emits a role template in our format: the modeled role fields
// (reusing renderRoleMarkdown's field set is avoided to keep provenance inline),
// then provenance, then the prompt body.
func renderAbsorbedRole(tpl *RoleTemplate, prov provenance) []byte {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "name: %s\n", tpl.Name)
	if tpl.Description != "" {
		fmt.Fprintf(&b, "description: %s\n", yamlScalar(tpl.Description))
	}
	if tpl.Engine != "" {
		fmt.Fprintf(&b, "engine: %s\n", tpl.Engine)
	}
	if tpl.Model != "" {
		fmt.Fprintf(&b, "model: %s\n", tpl.Model)
	}
	if tpl.PermissionMode != "" {
		fmt.Fprintf(&b, "permission_mode: %s\n", tpl.PermissionMode)
	}
	if tpl.EffortLevel != "" {
		fmt.Fprintf(&b, "effort_level: %s\n", tpl.EffortLevel)
	}
	if len(tpl.Tools.Allow) > 0 || len(tpl.Tools.Deny) > 0 {
		b.WriteString("tools:\n")
		if len(tpl.Tools.Allow) > 0 {
			fmt.Fprintf(&b, "  allow: [%s]\n", strings.Join(tpl.Tools.Allow, ", "))
		}
		if len(tpl.Tools.Deny) > 0 {
			fmt.Fprintf(&b, "  deny: [%s]\n", strings.Join(tpl.Tools.Deny, ", "))
		}
	}
	if len(tpl.Skills) > 0 {
		fmt.Fprintf(&b, "skills: [%s]\n", strings.Join(tpl.Skills, ", "))
	}
	if len(tpl.McpServers) > 0 {
		fmt.Fprintf(&b, "mcp_servers: [%s]\n", strings.Join(tpl.McpServers, ", "))
	}
	prov.write(&b)
	b.WriteString("---\n")
	if tpl.Prompt != "" {
		b.WriteString(tpl.Prompt)
		if !strings.HasSuffix(tpl.Prompt, "\n") {
			b.WriteByte('\n')
		}
	}
	return []byte(b.String())
}

// yamlScalar quotes a frontmatter scalar when it contains YAML-significant
// characters (colon, leading quote) so the re-emitted file parses back cleanly.
func yamlScalar(s string) string {
	if strings.ContainsAny(s, ":#") || strings.HasPrefix(s, "\"") || strings.HasPrefix(s, "'") {
		return "\"" + strings.ReplaceAll(s, "\"", "\\\"") + "\""
	}
	return s
}

// sinkPath is the on-disk destination for an absorbed item.
func sinkPath(cfg AbsorbConfig, kind AbsorbKind, name string) string {
	if kind == AbsorbRole {
		return filepath.Join(cfg.RolesDir, name+".md")
	}
	return filepath.Join(cfg.SkillsDir, name, "SKILL.md")
}

// submoduleSHA returns the checked-out commit of the submodule at root, or ""
// when it can't be resolved (the SHA is provenance metadata, not load-bearing).
func submoduleSHA(root string) string {
	cmd := exec.Command("git", "-C", root, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// loadLedger reads .absorbed.json; a missing ledger is an empty one.
func loadLedger(path string) (map[string]absorbRecord, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]absorbRecord{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read ledger %s: %w", path, err)
	}
	var m map[string]absorbRecord
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parse ledger %s: %w", path, err)
	}
	if m == nil {
		m = map[string]absorbRecord{}
	}
	return m, nil
}

// saveLedger writes .absorbed.json deterministically (sorted keys via the
// encoder's map ordering) with a trailing newline.
func saveLedger(path string, ledger map[string]absorbRecord) error {
	// json.Marshal sorts map keys, giving a stable diff across runs.
	raw, err := json.MarshalIndent(ledger, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write ledger %s: %w", path, err)
	}
	return nil
}

// AbsorbedNames returns the ledger's absorbed names sorted, for diagnostics.
func AbsorbedNames(path string) ([]string, error) {
	ledger, err := loadLedger(path)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(ledger))
	for n := range ledger {
		names = append(names, n)
	}
	sort.Strings(names)
	return names, nil
}
