package templateregistry_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/appregistry"
	"github.com/scottzx/1Agents/backend/internal/templateregistry"
)

// setupEnv creates a temp ONEAGENTS_HOME and returns a cleanup func.
func setupEnv(t *testing.T) (string, func()) {
	t.Helper()
	tmp, err := os.MkdirTemp("", "templateregistry-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ONEAGENTS_HOME", tmp)
	return tmp, func() { os.RemoveAll(tmp) }
}

func registerTestApp(t *testing.T, id string) {
	t.Helper()
	// Register silently if already registered (test process reuse).
	defer func() { recover() }()
	appregistry.Register(appregistry.AppManifest{
		ID:      id,
		Name:    id,
		Version: "0.1.0",
		Enabled: true,
	})
}

func TestRegisterAndList(t *testing.T) {
	_, cleanup := setupEnv(t)
	defer cleanup()

	registerTestApp(t, "podcast")

	templateregistry.Register(templateregistry.ProjectTemplate{
		ID:    "podcast.episode",
		Name:  "Podcast Episode",
		AppID: "podcast",
		Subdirs: []string{"录音", "剪辑"},
		PresetConfig: templateregistry.ProjectConfig{
			Instructions: "Record and edit podcast episodes.",
		},
	})

	templates := templateregistry.List()
	var found bool
	for _, tmpl := range templates {
		if tmpl.ID == "podcast.episode" {
			found = true
			if tmpl.AppID != "podcast" {
				t.Errorf("expected appId 'podcast', got %q", tmpl.AppID)
			}
		}
	}
	if !found {
		t.Error("registered template not found in List()")
	}
}

func TestGetTemplate(t *testing.T) {
	_, ok := templateregistry.Get("podcast.episode")
	if !ok {
		t.Error("Get: expected podcast.episode to be registered")
	}

	_, ok = templateregistry.Get("nonexistent.template")
	if ok {
		t.Error("Get: expected false for nonexistent template")
	}
}

func TestScaffoldCreatesDirs(t *testing.T) {
	home, cleanup := setupEnv(t)
	defer cleanup()

	// Create a workspace dir for the test project.
	wsPath := filepath.Join(home, "test-workspace")
	if err := os.MkdirAll(wsPath, 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := templateregistry.Scaffold("podcast.episode", "proj-001", wsPath)
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}

	if len(result.ArtifactDirs) != 2 {
		t.Errorf("expected 2 artifact dirs, got %d", len(result.ArtifactDirs))
	}

	for _, dir := range result.ArtifactDirs {
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("artifact dir not created: %s: %v", dir, err)
		}
	}
}

func TestScaffoldUnknownTemplate(t *testing.T) {
	_, cleanup := setupEnv(t)
	defer cleanup()

	_, err := templateregistry.Scaffold("nonexistent.xyz", "proj-002", "/tmp")
	if err == nil {
		t.Error("expected error for unknown template")
	}
}

func TestProjectConfigRoundTrip(t *testing.T) {
	_, cleanup := setupEnv(t)
	defer cleanup()

	cfg := templateregistry.ProjectConfig{
		Instructions: "be concise",
		Connectors:   []string{"mcp-git", "mcp-fs"},
		Experts:      []string{"editor"},
		Skills:       []string{"silenceDetect"},
		Automation:   []string{"autoPublish"},
	}

	if err := templateregistry.SaveProjectConfig("proj-cfg-test", cfg); err != nil {
		t.Fatalf("SaveProjectConfig: %v", err)
	}

	loaded, err := templateregistry.LoadProjectConfig("proj-cfg-test")
	if err != nil {
		t.Fatalf("LoadProjectConfig: %v", err)
	}

	if loaded.Instructions != cfg.Instructions {
		t.Errorf("instructions mismatch: got %q want %q", loaded.Instructions, cfg.Instructions)
	}
	if len(loaded.Connectors) != 2 {
		t.Errorf("connectors: got %v", loaded.Connectors)
	}
	if len(loaded.Experts) != 1 {
		t.Errorf("experts: got %v", loaded.Experts)
	}
}

func TestProjectConfigMissingReturnsZero(t *testing.T) {
	_, cleanup := setupEnv(t)
	defer cleanup()

	cfg, err := templateregistry.LoadProjectConfig("does-not-exist")
	if err != nil {
		t.Fatalf("LoadProjectConfig for missing id should not error: %v", err)
	}
	if cfg.Instructions != "" {
		t.Error("expected empty instructions for missing config")
	}
}

func TestProjectConfigUpsert(t *testing.T) {
	_, cleanup := setupEnv(t)
	defer cleanup()

	if err := templateregistry.SaveProjectConfig("upsert-proj", templateregistry.ProjectConfig{
		Instructions: "first",
	}); err != nil {
		t.Fatal(err)
	}
	if err := templateregistry.SaveProjectConfig("upsert-proj", templateregistry.ProjectConfig{
		Instructions: "second",
	}); err != nil {
		t.Fatal(err)
	}
	loaded, _ := templateregistry.LoadProjectConfig("upsert-proj")
	if loaded.Instructions != "second" {
		t.Errorf("expected 'second', got %q", loaded.Instructions)
	}
}
