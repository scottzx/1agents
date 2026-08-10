package provider

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportCCSwitchPreservesPerAgentEndpointsAndBindings(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "cc-switch.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE providers (id TEXT, app_type TEXT, name TEXT, settings_config TEXT, is_current BOOLEAN)`)
	if err != nil {
		t.Fatal(err)
	}
	rows := []struct{ app, settings string }{
		{"claude", `{"env":{"ANTHROPIC_BASE_URL":"https://relay.test/anthropic","ANTHROPIC_AUTH_TOKEN":"claude-secret","ANTHROPIC_MODEL":"model-main","ANTHROPIC_DEFAULT_HAIKU_MODEL":"model-fast"}}`},
		{"codex", `{"auth":{"OPENAI_API_KEY":"codex-secret"},"config":"base_url = \"https://relay.test/v1\"\nmodel = \"model-code\"\nwire_api = \"responses\"\n"}`},
		{"gemini", `{"env":{"GOOGLE_GEMINI_BASE_URL":"https://relay.test/gemini","GEMINI_API_KEY":"gemini-secret","GEMINI_MODEL":"model-gemini"}}`},
		{"openclaw", `{"baseUrl":"https://relay.test/openclaw","apiKey":"openclaw-secret","api":"anthropic-messages","models":[{"id":"model-openclaw-a"},{"id":"model-openclaw-b"}]}`},
		{"opencode", `{"npm":"@ai-sdk/openai-compatible","name":"Relay","options":{"baseURL":"https://relay.test/opencode","apiKey":"opencode-secret"},"models":{"model-opencode":{"name":"OpenCode Model"}}}`},
		{"future-agent", `{"baseUrl":"https://relay.test/future"}`},
	}
	for _, row := range rows {
		if _, err := db.Exec(`INSERT INTO providers VALUES ('relay', ?, 'Relay', ?, 1)`, row.app, row.settings); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store := NewStore(filepath.Join(t.TempDir(), "providers.json"))
	if err := store.ImportCCSwitch(dbPath); err != nil {
		t.Fatal(err)
	}
	p, err := store.Get("relay")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Endpoints) != 2 {
		t.Fatalf("endpoints = %#v", p.Endpoints)
	}
	credentials := map[EndpointFamily]string{}
	for _, endpoint := range p.Endpoints {
		credentials[endpoint.Family] = endpoint.APIKey
	}
	if credentials[EndpointFamilyAnthropic] != "claude-secret" || credentials[EndpointFamilyOpenAI] != "codex-secret" {
		t.Fatalf("credentials = %#v", credentials)
	}
	if strings.Join(p.Apps, ",") != "claude,codex,openclaw,opencode" {
		t.Fatalf("source app types = %#v", p.Apps)
	}
	bindings, err := store.ListBindings()
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 2 {
		t.Fatalf("bindings = %#v", bindings)
	}
	for _, binding := range bindings {
		if binding.AgentID == AgentClaude && binding.ModelMapping["haiku"] != "model-fast" {
			t.Fatalf("claude mapping = %#v", binding.ModelMapping)
		}
	}
	models, err := store.Models("relay")
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 6 {
		t.Fatalf("models = %#v", models)
	}
}

func TestImportCCSwitchFromConfiguredDatabase(t *testing.T) {
	dbPath := os.Getenv("CC_SWITCH_TEST_DB")
	if dbPath == "" {
		t.Skip("CC_SWITCH_TEST_DB is not set")
	}
	store := NewStore(filepath.Join(t.TempDir(), "providers.json"))
	if err := store.ImportCCSwitch(dbPath); err != nil {
		t.Fatal(err)
	}
	providers, _, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) == 0 {
		t.Fatal("real cc-switch database imported no providers")
	}
	seen := map[string]bool{}
	for _, item := range providers {
		for _, appType := range item.Apps {
			seen[appType] = true
		}
	}
	for _, appType := range []string{"claude", "codex", "openclaw", "opencode"} {
		if !seen[appType] {
			t.Fatalf("real cc-switch database did not import %s", appType)
		}
	}
	if seen["gemini"] {
		t.Fatal("real cc-switch database imported retired gemini providers")
	}
}
