package workspace

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// sharedAgentsRoot is the skill-manager shared store for agents (the source of
// truth for installed agents). Agents are single Markdown files <name>.md kept
// flat under ~/.1agents/skill-manager/agents (mirrors sharedSkillsRoot, which
// points at .../shared for skill packages).
func sharedAgentsRoot() string {
	return filepath.Join(get1AgentsHome(), ".1agents", "skill-manager", "agents")
}

// normalizeAgentRef reduces an agent reference to its shared-store file name
// (<name>.md). The frontend picker may send a plain file name ("foo.md") or a
// scoped ref ("shared:foo.md" / "centralized:foo.md"); either way we key off the
// file name — matching the Python side, which does
// Path(agent_ref.rsplit(":", 1)[-1]).name and looks up store.root/<name>.md.
// The .md extension is preserved. filepath.Base guards against path traversal.
func normalizeAgentRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if i := strings.LastIndex(ref, ":"); i >= 0 {
		ref = ref[i+1:]
	}
	return filepath.Base(ref)
}

// syncAgentsToWorkspace materializes the given agents (by shared-store file name
// <name>.md) into a workspace as the "weak copy" of #360: the shared store stays
// the single source of truth, and each workspace gets a decoupled instance.
//
// Layout — one physical copy, one whole-directory symlink (same idea as skills):
//   - real copies → <ws>/.claude/agents/<name>.md  (Claude Code reads these natively)
//   - dir symlink → <ws>/.agents/agents → ../.claude/agents
//     (the generic agent convention; one link covers every current AND future
//     agent, so there's no per-agent upkeep)
//
// Missing source agents are logged and skipped; an already-present real copy is
// left untouched (idempotent). Returns the file names actually synced.
func syncAgentsToWorkspace(workspacePath string, refs []string) ([]string, error) {
	if workspacePath == "" || len(refs) == 0 {
		return nil, nil
	}
	root := sharedAgentsRoot()
	storeRoot := filepath.Join(workspacePath, ".claude", "agents")
	if err := os.MkdirAll(storeRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create agents dir: %w", err)
	}
	synced := make([]string, 0, len(refs))
	for _, ref := range refs {
		name := normalizeAgentRef(ref)
		if name == "" || name == "." || name == ".." {
			continue
		}
		src := filepath.Join(root, name)
		si, err := os.Stat(src)
		if err != nil || si.IsDir() {
			log.Printf("[workspace] agent %q not found in shared store (%s); skipped", name, root)
			continue
		}
		store := filepath.Join(storeRoot, name)
		if _, err := os.Stat(store); err != nil {
			if err := copyFile(src, store, si.Mode().Perm()); err != nil {
				log.Printf("[workspace] copy agent %q: %v", name, err)
				continue
			}
		}
		synced = append(synced, name)
		log.Printf("[workspace] copied agent %q into %s", name, store)
	}
	// One whole-directory symlink instead of per-agent links: any agent in
	// .claude/agents (now or later) is reachable via .agents/agents for free.
	if err := linkAgentsAgents(workspacePath); err != nil {
		log.Printf("[workspace] link .agents/agents: %v", err)
	}
	return synced, nil
}

// workspaceAgentFile resolves a workspace's own copy of an agent file
// (<ws>/.claude/agents/<name>.md) from an agent ref, validating it is a real
// file. The file name is derived via normalizeAgentRef, which strips any scope
// prefix and guards against path traversal.
func workspaceAgentFile(workspacePath, agentRef string) (string, error) {
	name := normalizeAgentRef(agentRef)
	if name == "" || name == "." || name == ".." {
		return "", fmt.Errorf("invalid agent ref %q", agentRef)
	}
	file := filepath.Join(workspacePath, ".claude", "agents", name)
	if info, err := os.Stat(file); err != nil || info.IsDir() {
		return "", fmt.Errorf("no agent file at %s", file)
	}
	return file, nil
}

// pushAgentToShared forwards a workspace's edited agent copy back to the 1skills
// (母体) shared store via its push-from-path endpoint. The store fingerprints the
// source and only rewrites (and bumps the manifest revision) when the content
// actually differs — Go never touches the store or manifest directly, keeping
// drift detection correct. Returns whether the store baseline changed.
func pushAgentToShared(skillsAddr, agentRef, sourcePath string) (changed, created bool, err error) {
	if skillsAddr == "" {
		skillsAddr = defaultSkillsAddr
	}
	target := &url.URL{
		Scheme: "http",
		Host:   skillsAddr,
		Path:   "/api/agents/" + agentRef + "/push-from-path",
	}
	payload, _ := json.Marshal(map[string]string{"sourcePath": sourcePath})
	resp, err := http.Post(target.String(), "application/json", bytes.NewReader(payload))
	if err != nil {
		return false, false, fmt.Errorf("reach skill manager: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &e)
		if e.Error == "" {
			e.Error = strings.TrimSpace(string(body))
		}
		return false, false, fmt.Errorf("skill manager: %s", e.Error)
	}
	var out struct {
		Changed bool `json:"changed"`
		Created bool `json:"created"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return false, false, fmt.Errorf("decode skill manager response: %w", err)
	}
	return out.Changed, out.Created, nil
}

// WorkspaceAgentStatus describes one agent materialized in a workspace's
// .claude/agents: its parsed frontmatter (for the card) plus its relationship to
// the shared store (母体) as one of three states.
type WorkspaceAgentStatus struct {
	AgentRef    string `json:"agentRef"`    // "shared:<name>.md" — the store ref
	File        string `json:"file"`        // <name>.md file name
	Name        string `json:"name"`        // declared name (or file stem)
	Description string `json:"description"` // description from frontmatter
	// State: "synced" (in store, identical), "modified" (in store, drifted →
	// push overwrites), or "local" (not in store → push creates/ingests).
	State string `json:"state"`
}

// listWorkspaceAgents enumerates the <name>.md files under <ws>/.claude/agents
// and asks the 1skills store for each one's status (in-store + drift +
// frontmatter). An agent whose status check fails is reported as "local" rather
// than dropped, so the detail page still lists it.
func listWorkspaceAgents(workspacePath, skillsAddr string) ([]WorkspaceAgentStatus, error) {
	root := filepath.Join(workspacePath, ".claude", "agents")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []WorkspaceAgentStatus{}, nil
		}
		return nil, err
	}
	out := make([]WorkspaceAgentStatus, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue // agents are single <name>.md files
		}
		file := filepath.Join(root, e.Name())
		ref := "shared:" + e.Name()
		stem := strings.TrimSuffix(e.Name(), ".md")
		st, err := agentStatusAgainstShared(skillsAddr, ref, file)
		if err != nil {
			log.Printf("[workspace] status agent %q: %v", e.Name(), err)
			out = append(out, WorkspaceAgentStatus{AgentRef: ref, File: e.Name(), Name: stem, State: "local"})
			continue
		}
		name := st.Name
		if name == "" {
			name = stem
		}
		out = append(out, WorkspaceAgentStatus{
			AgentRef:    ref,
			File:        e.Name(),
			Name:        name,
			Description: st.Description,
			State:       skillState(st),
		})
	}
	return out, nil
}

// agentStatusAgainstShared asks the 1skills store (母体) for a workspace copy's
// status (in-store, drift, parsed frontmatter). Read-only counterpart to
// pushAgentToShared.
func agentStatusAgainstShared(skillsAddr, agentRef, sourcePath string) (sharedSkillStatus, error) {
	if skillsAddr == "" {
		skillsAddr = defaultSkillsAddr
	}
	target := &url.URL{
		Scheme: "http",
		Host:   skillsAddr,
		Path:   "/api/agents/" + agentRef + "/status-from-path",
	}
	payload, _ := json.Marshal(map[string]string{"sourcePath": sourcePath})
	resp, err := http.Post(target.String(), "application/json", bytes.NewReader(payload))
	if err != nil {
		return sharedSkillStatus{}, fmt.Errorf("reach skill manager: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return sharedSkillStatus{}, fmt.Errorf("skill manager: %s", strings.TrimSpace(string(body)))
	}
	var out sharedSkillStatus
	if err := json.Unmarshal(body, &out); err != nil {
		return sharedSkillStatus{}, fmt.Errorf("decode status response: %w", err)
	}
	return out, nil
}

// linkAgentsAgents points <ws>/.agents/agents at <ws>/.claude/agents with a
// single relative directory symlink. Idempotent; a stale entry is removed and
// replaced. (Parallel to linkAgentsSkills's <ws>/.agents/skills.)
func linkAgentsAgents(workspacePath string) error {
	agentsDir := filepath.Join(workspacePath, ".agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		return err
	}
	link := filepath.Join(agentsDir, "agents")
	return ensureSymlink(filepath.Join("..", ".claude", "agents"), link)
}
