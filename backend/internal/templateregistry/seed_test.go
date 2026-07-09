package templateregistry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/appregistry"
)

// TestScaffoldSeedsProjectLocalConfig verifies Scaffold writes the template's
// preset into <workspace>/.1agents/project_config.json in the frontend's shape.
func TestScaffoldSeedsProjectLocalConfig(t *testing.T) {
	appregistry.Register(appregistry.AppManifest{ID: "seedtest_app", Name: "SeedTest", Enabled: true})
	Register(ProjectTemplate{
		ID:           "seedtest_app.tpl",
		Name:         "Seed Test",
		AppID:        "seedtest_app",
		PresetConfig: ProjectConfig{Instructions: "种子指令", Connectors: []string{"feishu"}},
	})

	ws := t.TempDir()
	if _, err := Scaffold("seedtest_app.tpl", "proj1", ws); err != nil {
		t.Fatalf("Scaffold: %v", err)
	}

	b, err := os.ReadFile(filepath.Join(ws, ".1agents", "project_config.json"))
	if err != nil {
		t.Fatalf("read seeded config: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["instructions"] != "种子指令" {
		t.Errorf("instructions = %v, want 种子指令", m["instructions"])
	}
	conns, _ := m["connectors"].([]any)
	if len(conns) != 1 || conns[0] != "feishu" {
		t.Errorf("connectors = %v, want [feishu]", m["connectors"])
	}
	// frontend-shape defaults for the incompatible fields
	if experts, ok := m["experts"].([]any); !ok || len(experts) != 0 {
		t.Errorf("experts = %v, want []", m["experts"])
	}
	if m["automation"] != "" {
		t.Errorf("automation = %v, want empty string", m["automation"])
	}
}
