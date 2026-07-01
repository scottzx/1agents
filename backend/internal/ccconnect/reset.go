package ccconnect

import (
	"log"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/chenhg5/cc-connect/config"
)

// reset.go supports the "重置本地数据" feature (settings → 重置数据). Clearing
// meta.db alone is not enough: on the next boot the workspace↔cc-connect sync in
// configureAndRun reflows every cc-connect [[projects]] entry that carries a
// work_dir back into the workspace registry (see runner.go — the "Automatically
// imported workspace … from CC-Connect project config" path). Its only guard is
// "import nothing when the workspace list is empty", which the re-seeded default
// workspace defeats. So a reset must also strip the workspace-backed projects
// from ~/.cc-connect/config.toml, or the wiped projects bounce right back.

// ccConfigDir returns 1agents' OWN cc-connect config directory,
// ~/.1agents/im_channels. It is deliberately NOT ~/.cc-connect so a globally
// installed cc-connect (which owns ~/.cc-connect) is never touched — 1agents
// runs its embedded cc-connect against a private config to avoid two-way
// interference. Honors ONEAGENTS_HOME for tests/sandboxes.
func ccConfigDir() string {
	if val := os.Getenv("ONEAGENTS_HOME"); val != "" {
		return filepath.Join(val, ".1agents", "im_channels")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".1agents", "im_channels")
}

// ccConfigPath returns 1agents' private cc-connect config file,
// ~/.1agents/im_channels/config.toml.
func ccConfigPath() string {
	return filepath.Join(ccConfigDir(), "config.toml")
}

// legacyCCConfigPath returns the old shared location ~/.cc-connect/config.toml
// that 1agents used before decoupling. Kept only for the one-time migration in
// Start(); nothing else should read it.
func legacyCCConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".cc-connect", "config.toml")
}

// PurgeWorkspaceProjects removes the workspace-backed [[projects]] entries from
// the cc-connect config — exactly the set the boot-time sync would re-import: a
// project whose agent options carry a non-empty work_dir. Provider/model/agent
// and platform/relay config (global [[providers]], [management], [bridge], and
// projects without a work_dir) are preserved verbatim by round-tripping the
// whole config.Config through the TOML codec.
//
// Returns the number of project entries removed. A missing config file is a
// no-op (returns 0). Wired into the reset endpoint as a hook from server.go.
func PurgeWorkspaceProjects() (int, error) {
	path := ccConfigPath()
	if config.ConfigPath != "" {
		path = config.ConfigPath
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return 0, nil
	}

	var cfg config.Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return 0, err
	}

	kept := cfg.Projects[:0]
	removed := 0
	for _, p := range cfg.Projects {
		if workDir, _ := p.Agent.Options["work_dir"].(string); workDir != "" {
			removed++
			continue // workspace-backed project — drop it
		}
		kept = append(kept, p)
	}
	if removed == 0 {
		return 0, nil
	}
	cfg.Projects = kept

	if err := saveConfig(&cfg, path); err != nil {
		return 0, err
	}
	log.Printf("[ccconnect] reset: purged %d workspace-backed projects from %s", removed, path)
	return removed, nil
}
