package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// workspaceAgentFile resolves a team member file. Team selection is a runtime
// persona concern, not a HarnessKit extension identity: API mutation handlers
// use extension IDs and never call this helper.
func workspaceAgentFile(workspacePath, teamRef string) (string, error) {
	ref := strings.TrimSpace(teamRef)
	if strings.ContainsAny(ref, `/\`) {
		clean := filepath.Clean(filepath.FromSlash(ref))
		if clean == ".." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("invalid team member %q", teamRef)
		}
		candidate := filepath.Join(workspacePath, clean)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
		return "", fmt.Errorf("team member %q is not installed in this workspace", ref)
	}
	name := filepath.Base(ref)
	if name == "" || name == "." || name == ".." {
		return "", fmt.Errorf("invalid team member %q", teamRef)
	}
	candidates := []string{
		filepath.Join(workspacePath, ".claude", "agents", name),
		filepath.Join(workspacePath, ".agents", "agents", name),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("team member %q is not installed in this workspace", name)
}

func linkAgentsAgents(workspacePath string) error {
	agentsRoot := filepath.Join(workspacePath, ".agents")
	if err := os.MkdirAll(agentsRoot, 0o755); err != nil {
		return err
	}
	link := filepath.Join(agentsRoot, "agents")
	if _, err := os.Lstat(link); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Symlink(filepath.Join("..", ".claude", "agents"), link)
}
