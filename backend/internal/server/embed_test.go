package server

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestEmbedBundleCandidates_prefersStaticEmbed(t *testing.T) {
	static := filepath.Join(string(filepath.Separator), "opt", "web", "dist")
	got := embedBundleCandidates(static, "skills-embed.js", []string{
		"modules/1skills/dist-embed/skills-embed.js",
	})
	if len(got) < 2 {
		t.Fatalf("expected at least static + monorepo candidates, got %v", got)
	}
	if got[0] != filepath.Join(static, "embed", "skills-embed.js") {
		t.Fatalf("first candidate should be StaticDir/embed/file, got %q", got[0])
	}
	if got[1] != filepath.Join(static, "skills-embed.js") {
		t.Fatalf("second candidate should be StaticDir/file, got %q", got[1])
	}
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "modules/1skills/dist-embed/skills-embed.js") {
		t.Fatalf("missing monorepo fallback: %v", got)
	}
}

func TestEmbedBundleCandidates_emptyStaticStillHasMonorepo(t *testing.T) {
	got := embedBundleCandidates("", "cc-connect-embed.js", []string{
		"modules/cc-connect/web/dist-embed/cc-connect-embed.js",
	})
	found := false
	for _, p := range got {
		if p == "modules/cc-connect/web/dist-embed/cc-connect-embed.js" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected monorepo path in %v", got)
	}
}
