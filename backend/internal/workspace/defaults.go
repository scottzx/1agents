package workspace

import (
	_ "embed"
	"log"
	"os"
	"path/filepath"
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

// ensureProjectGuideFiles makes sure the workspace directory contains the agent
// guidance files (CLAUDE.md / AGENTS.md). Missing files are created with the
// default behavioral-guidelines template; existing files are left untouched.
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
}
