package workspace

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/harnesskit"
	"github.com/scottzx/1Agents/backend/internal/meta"
)

type fakeExtensionClient struct {
	ensuredPath string
	filter      harnesskit.ListExtensionsFilter
	rows        []harnesskit.Extension
}

func (f *fakeExtensionClient) EnsureProject(_ context.Context, path string) error {
	f.ensuredPath = path
	return nil
}
func (f *fakeExtensionClient) ScanAndSync(context.Context) error { return nil }
func (f *fakeExtensionClient) ListExtensions(_ context.Context, filter harnesskit.ListExtensionsFilter) ([]harnesskit.Extension, error) {
	f.filter = filter
	return f.rows, nil
}
func (f *fakeExtensionClient) InstallToAgent(context.Context, string, string, string, string) (string, error) {
	return "", nil
}
func (f *fakeExtensionClient) DeleteExtension(context.Context, string) error { return nil }
func (f *fakeExtensionClient) UpdateExtension(context.Context, string) error { return nil }

func TestWorkspaceSkillsUsesHarnessKitProjectScopeAndExtensionIDs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ONEAGENTS_HOME", home)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ONEAGENTS_AGENT_EXTENSION_MAP", filepath.Join(cwd, "..", "..", "..", "config", "agent-extension-map.json"))
	projectPath := filepath.Join(home, "demo")
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := meta.OpenDefault()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.EnsureWorkspaceProject(meta.Project{
		ID: "demo", Name: "Demo", WorkspacePath: projectPath, DefaultAgent: "claudecode",
	}); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(projectPath, ".claude", "skills", "review")
	fake := &fakeExtensionClient{rows: []harnesskit.Extension{{
		ID: "project-skill", Kind: "skill", Name: "Review", Description: "Review changes",
		Agents: []string{"claude"}, Enabled: true, SourcePath: &sourcePath,
		Scope: harnesskit.ExtensionScope{Type: "project", Name: "Demo", Path: projectPath},
	}}}
	handler := NewHandler()
	handler.extensions = fake

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/workspace/skills?id=demo", nil)
	handler.WorkspaceSkills(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if fake.ensuredPath != projectPath || fake.filter.ScopeType != "project" ||
		fake.filter.ScopePath != projectPath || fake.filter.Kind != "skill" {
		t.Fatalf("ensure/filter mismatch: path=%q filter=%+v", fake.ensuredPath, fake.filter)
	}
	var body struct {
		Skills []WorkspaceExtensionStatus `json:"skills"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Skills) != 1 || body.Skills[0].ExtensionID != "project-skill" ||
		body.Skills[0].Path != ".claude/skills/review" {
		t.Fatalf("skills = %+v", body.Skills)
	}
}

func TestWorkspaceTeamSucceedsWhenDefaultAgentHasNoDeploymentMapping(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ONEAGENTS_HOME", home)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ONEAGENTS_AGENT_EXTENSION_MAP", filepath.Join(cwd, "..", "..", "..", "config", "agent-extension-map.json"))
	projectPath := filepath.Join(home, "acp-demo")
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := meta.OpenDefault()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.EnsureWorkspaceProject(meta.Project{
		ID: "acp-demo", Name: "ACP Demo", WorkspacePath: projectPath, DefaultAgent: "acp",
	}); err != nil {
		t.Fatal(err)
	}
	fake := &fakeExtensionClient{rows: []harnesskit.Extension{}}
	handler := NewHandler()
	handler.extensions = fake

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/workspace/team?id=acp-demo", nil)
	handler.WorkspaceTeam(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s, want 200 OK", rec.Code, rec.Body.String())
	}
}
