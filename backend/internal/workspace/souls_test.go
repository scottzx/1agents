package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListCuratedSoulsLocalized(t *testing.T) {
	zh, err := listCuratedSouls("zh")
	if err != nil {
		t.Fatalf("list zh: %v", err)
	}
	if len(zh) == 0 {
		t.Fatal("expected curated souls, got none")
	}
	en, err := listCuratedSouls("en")
	if err != nil {
		t.Fatalf("list en: %v", err)
	}
	if len(en) != len(zh) {
		t.Fatalf("zh (%d) and en (%d) should have the same count", len(zh), len(en))
	}
	// technical-writer is a stable curated entry — check localization + content.
	var tw *SoulPreset
	for i := range zh {
		if zh[i].Ref == "technical-writer" {
			tw = &zh[i]
		}
	}
	if tw == nil {
		t.Fatal("technical-writer missing from curated set")
	}
	if tw.Title != "技术写作者" {
		t.Errorf("zh title = %q", tw.Title)
	}
	if tw.Content == "" || !strings.Contains(tw.Content, "SOUL.md") {
		t.Errorf("expected embedded SOUL.md content, got %.40q", tw.Content)
	}
	for i := range en {
		if en[i].Ref == "technical-writer" && en[i].Title != "Technical Writer" {
			t.Errorf("en title = %q", en[i].Title)
		}
	}
}

func TestSeedAndReadWorkspaceSoul(t *testing.T) {
	ws := t.TempDir()

	// Blank ref is a no-op — no SOUL.md, no persona.
	if err := seedSoulToWorkspace(ws, ""); err != nil {
		t.Fatalf("blank seed: %v", err)
	}
	if content, _ := ReadWorkspaceSoul(ws); content != "" {
		t.Errorf("blank seed should leave no SOUL.md, got %.40q", content)
	}

	// A known preset seeds SOUL.md with its markdown.
	if err := seedSoulToWorkspace(ws, "zen-master"); err != nil {
		t.Fatalf("seed zen-master: %v", err)
	}
	content, err := ReadWorkspaceSoul(ws)
	if err != nil {
		t.Fatalf("read soul: %v", err)
	}
	if !strings.Contains(content, "Zen Master") {
		t.Errorf("SOUL.md should carry the zen-master persona, got %.60q", content)
	}
	if _, err := os.Stat(filepath.Join(ws, "SOUL.md")); err != nil {
		t.Errorf("SOUL.md should exist on disk: %v", err)
	}

	// Unknown ref errors.
	if err := seedSoulToWorkspace(ws, "no-such-soul"); err == nil {
		t.Error("expected error for unknown soul ref")
	}

	// Overwrite via write, then clear with empty content removes the file.
	if err := writeWorkspaceSoul(ws, "custom persona"); err != nil {
		t.Fatalf("write soul: %v", err)
	}
	if content, _ := ReadWorkspaceSoul(ws); content != "custom persona" {
		t.Errorf("write round-trip = %q", content)
	}
	if err := writeWorkspaceSoul(ws, "   "); err != nil {
		t.Fatalf("clear soul: %v", err)
	}
	if _, err := os.Stat(filepath.Join(ws, "SOUL.md")); !os.IsNotExist(err) {
		t.Errorf("clearing should remove SOUL.md, stat err = %v", err)
	}
}
