package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFetchModelsTriesCompatibleCandidatePaths(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer secret" || r.Header.Get("x-api-key") != "secret" {
			t.Errorf("missing auth headers")
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"model-b"},{"id":"model-a"},{"id":"model-a"}]}`))
	}))
	defer server.Close()
	models, err := FetchModelsFromEndpoint(server.URL+"/anthropic", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(models, ",") != "model-b,model-a" {
		t.Fatalf("models = %#v", models)
	}
}

func TestLoadMigratesLegacyProviderData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "providers.json")
	legacy := `{"schema_version":3,"active_provider_id":"relay","providers":[{"id":"relay","name":"Relay","protocol":"dual","base_url":"https://example.test/v1","api_key":"secret","model":"model-a","model_ids":["model-a","model-b"]}]}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	pd, err := NewStore(path).Load()
	if err != nil {
		t.Fatal(err)
	}
	if pd.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("schema version = %d", pd.SchemaVersion)
	}
	if len(pd.Providers[0].Endpoints) != 2 {
		t.Fatalf("endpoints = %#v", pd.Providers[0].Endpoints)
	}
	if len(pd.Models) != 2 {
		t.Fatalf("models = %#v", pd.Models)
	}
	if len(pd.Bindings) != 2 {
		t.Fatalf("bindings = %#v", pd.Bindings)
	}
	persisted, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(persisted), `"schema_version": 4`) {
		t.Fatalf("migration was not persisted: %s", persisted)
	}
	if _, err := os.Stat(path + ".v3.bak"); err != nil {
		t.Fatalf("schema v3 backup missing: %v", err)
	}
}

func TestLoadRetiresGeminiAndCanonicalizesImportedEndpoints(t *testing.T) {
	path := filepath.Join(t.TempDir(), "providers.json")
	data := `{"schema_version":2,"providers":[{"id":"relay","name":"Relay","apps":["gemini","openclaw","opencode"],"endpoints":[{"agent_id":"gemini","protocol":"gemini","base_url":"https://gemini.test"},{"agent_id":"openclaw","protocol":"anthropic-messages","base_url":"https://anthropic.test"},{"agent_id":"opencode","protocol":"openai","base_url":"https://openai.test"}]}],"bindings":[{"agent_id":"gemini","provider_id":"relay","updated_at":1},{"agent_id":"openclaw","provider_id":"relay","updated_at":1}]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	pd, err := NewStore(path).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(pd.Providers[0].Endpoints) != 2 || pd.Providers[0].Endpoints[0].Family != EndpointFamilyAnthropic || pd.Providers[0].Endpoints[1].Family != EndpointFamilyOpenAI {
		t.Fatalf("canonical endpoints = %#v", pd.Providers[0].Endpoints)
	}
	if strings.Join(pd.Providers[0].Apps, ",") != "openclaw,opencode" {
		t.Fatalf("source app types = %#v", pd.Providers[0].Apps)
	}
	if len(pd.Bindings) != 0 {
		t.Fatalf("retired bindings = %#v", pd.Bindings)
	}
}

func TestPlanCodexPreservesUnownedConfiguration(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	before := "sandbox_mode = \"workspace-write\"\nmodel_reasoning_effort = \"medium\"\n[mcp_servers.demo]\ncommand = \"demo\"\n"
	if err := os.WriteFile(path, []byte(before), 0o600); err != nil {
		t.Fatal(err)
	}
	authPath := filepath.Join(home, ".codex", "auth.json")
	if err := os.WriteFile(authPath, []byte(`{"tokens":{"access_token":"keep"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	p := Provider{ID: "relay", Name: "Relay", APIKey: "secret", Endpoints: []ProviderEndpoint{{AgentID: AgentCodex, Protocol: "openai_responses", BaseURL: "https://example.test/v1"}}}
	plan, err := PlanAgentBinding(home, p, AgentBinding{AgentID: AgentCodex, ProviderID: p.ID, ModelID: "model-a"})
	if err != nil {
		t.Fatal(err)
	}
	after := plan.Changes[0].After
	for _, want := range []string{"sandbox_mode = \"workspace-write\"", "model_reasoning_effort = \"medium\"", "[mcp_servers.demo]", "command = \"demo\"", "model = \"model-a\""} {
		if !strings.Contains(after, want) {
			t.Errorf("rendered config missing %q:\n%s", want, after)
		}
	}
	if len(plan.Changes) != 2 {
		t.Fatalf("changes = %#v", plan.Changes)
	}
	if !strings.Contains(plan.Changes[1].After, `"access_token": "keep"`) || !strings.Contains(plan.Changes[1].After, `"OPENAI_API_KEY": "secret"`) {
		t.Fatalf("auth was not merged: %s", plan.Changes[1].After)
	}
}

func TestApplyClaudeUsesSettingsJSONAndCreatesBackup(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"permissions":{"allow":["Read"]},"env":{"KEEP":"yes"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	p := Provider{ID: "relay", APIKey: "secret", Endpoints: []ProviderEndpoint{{AgentID: AgentClaude, Protocol: "anthropic", BaseURL: "https://example.test"}}}
	result, err := ApplyAgentBinding(home, p, AgentBinding{AgentID: AgentClaude, ProviderID: p.ID, ModelID: "model-a", ModelMapping: map[string]string{"haiku": "model-fast"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 || result.Files[0].BackupPath == "" {
		t.Fatal("expected backup path")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	if root["permissions"] == nil {
		t.Fatal("permissions were not preserved")
	}
	env := root["env"].(map[string]any)
	if env["KEEP"] != "yes" || env["ANTHROPIC_MODEL"] != "model-a" || env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != "model-fast" {
		t.Fatalf("env = %#v", env)
	}
}

func TestMergeDiscoveredModelsMarksMissingUnavailable(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "providers.json"))
	p, err := store.AddOrUpdate(Provider{ID: "relay", Name: "Relay", Protocol: "openai", BaseURL: "https://example.test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MergeDiscoveredModels(p.ID, []string{"old"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MergeDiscoveredModels(p.ID, []string{"new"}); err != nil {
		t.Fatal(err)
	}
	models, err := store.Models(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	state := map[string]bool{}
	for _, model := range models {
		state[model.ModelID] = model.Available
	}
	if state["old"] {
		t.Fatalf("old model should be unavailable: %#v", state)
	}
	if !state["new"] {
		t.Fatalf("new model should be available: %#v", state)
	}
}
