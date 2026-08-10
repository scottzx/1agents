package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/provider"
)

func useTestProviderStore(t *testing.T) *provider.Store {
	t.Helper()
	previous := defaultProviderStore
	store := provider.NewStore(filepath.Join(t.TempDir(), "providers.json"))
	defaultProviderStore = store
	t.Cleanup(func() { defaultProviderStore = previous })
	return store
}

func addBindingTestProvider(t *testing.T, store *provider.Store) {
	t.Helper()
	_, err := store.AddOrUpdate(provider.Provider{
		ID:     "binding-provider",
		Name:   "Binding Provider",
		APIKey: "test-secret",
		Endpoints: []provider.ProviderEndpoint{
			{AgentID: provider.AgentClaude, Protocol: "anthropic", BaseURL: "https://example.test/anthropic"},
			{AgentID: provider.AgentCodex, Protocol: "openai_responses", BaseURL: "https://example.test/v1"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestProvidersAPIAlwaysRedactsCredentials(t *testing.T) {
	store := useTestProviderStore(t)
	_, err := store.AddOrUpdate(provider.Provider{
		ID:     "secret-provider",
		Name:   "Secret Provider",
		APIKey: "global-secret",
		Endpoints: []provider.ProviderEndpoint{
			{AgentID: provider.AgentCodex, Protocol: "openai_responses", BaseURL: "https://example.test/v1", APIKey: "endpoint-secret", Headers: map[string]string{"Authorization": "custom-secret", "X-Tenant": "tenant-secret"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	handleProviders(recorder, httptest.NewRequest(http.MethodGet, "/api/providers", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET status = %d: %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if strings.Contains(body, "global-secret") || strings.Contains(body, "endpoint-secret") || strings.Contains(body, "custom-secret") || strings.Contains(body, "tenant-secret") {
		t.Fatalf("provider response leaked a credential: %s", body)
	}
	if !strings.Contains(body, `"has_api_key":true`) || !strings.Contains(body, `"header_names":["Authorization","X-Tenant"]`) {
		t.Fatalf("provider response did not report stored credential presence: %s", body)
	}

	update := []byte(`{"id":"secret-provider","name":"Renamed","endpoints":[{"agent_id":"codex","protocol":"openai_responses","base_url":"https://example.test/v1","headers":{"Authorization":"","X-Tenant":""}}]}`)
	recorder = httptest.NewRecorder()
	handleProviders(recorder, httptest.NewRequest(http.MethodPost, "/api/providers", bytes.NewReader(update)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("POST status = %d: %s", recorder.Code, recorder.Body.String())
	}
	saved, err := store.Get("secret-provider")
	if err != nil {
		t.Fatal(err)
	}
	if saved.APIKey != "global-secret" || saved.Endpoints[0].APIKey != "endpoint-secret" {
		t.Fatalf("blank credential update erased secrets: %#v", saved)
	}
	if saved.Endpoints[0].Headers["Authorization"] != "custom-secret" || saved.Endpoints[0].Headers["X-Tenant"] != "tenant-secret" {
		t.Fatalf("blank header update erased secrets: %#v", saved.Endpoints[0].Headers)
	}
}

func TestDiscoverModelsUsesSavedEndpointCredentialAndURL(t *testing.T) {
	store := useTestProviderStore(t)
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/catalog" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer endpoint-secret" {
			http.Error(w, "missing credential", http.StatusUnauthorized)
			return
		}
		if r.Header.Get("X-Tenant") != "tenant-a" {
			http.Error(w, "missing custom header", http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"id": "model-a"}}})
	}))
	defer modelServer.Close()

	_, err := store.AddOrUpdate(provider.Provider{
		ID:   "discover-provider",
		Name: "Discover Provider",
		Endpoints: []provider.ProviderEndpoint{
			{AgentID: provider.AgentCodex, Protocol: "openai_responses", BaseURL: modelServer.URL, ModelsEndpoint: modelServer.URL + "/catalog", APIKey: "endpoint-secret", Headers: map[string]string{"X-Tenant": "tenant-a"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"provider_id":"discover-provider","agent_id":"codex"}`)
	handleDiscoverProviderModels(recorder, httptest.NewRequest(http.MethodPost, "/api/providers/discover-models", body))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"model_id":"model-a"`) {
		t.Fatalf("unexpected model response: %s", recorder.Body.String())
	}
}

func TestAgentOptionsAPI(t *testing.T) {
	recorder := httptest.NewRecorder()
	handleAgentOptions(recorder, httptest.NewRequest(http.MethodGet, "/api/agents/options", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, option := range []string{"thinking_enabled", "thinking_budget_tokens", "reasoning_effort"} {
		if !strings.Contains(body, option) {
			t.Fatalf("missing %s in option schema: %s", option, body)
		}
	}
	if strings.Contains(body, "gemini") {
		t.Fatalf("retired gemini schema is still exposed: %s", body)
	}
}

func TestAgentProfilesCRUDArchiveAndRestore(t *testing.T) {
	store := useTestProviderStore(t)
	_, err := store.AddOrUpdate(provider.Provider{
		ID:       "profile-provider",
		Name:     "Profile Provider",
		APIKey:   "profile-secret",
		Model:    "model-a",
		ModelIDs: []string{"model-a", "model-b"},
		Endpoints: []provider.ProviderEndpoint{{
			Family:   provider.EndpointFamilyOpenAI,
			Protocol: "openai_chat",
			BaseURL:  "https://example.test/v1",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	create := `{"id":"profile-build","name":"Profile Build","runtime_id":"grok-build","provider_id":"profile-provider","model_id":"model-a"}`
	recorder := httptest.NewRecorder()
	handleAgentProfiles(recorder, httptest.NewRequest(http.MethodPost, "/api/agent-profiles", strings.NewReader(create)))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"revision":1`) {
		t.Fatalf("create status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "profile-secret") {
		t.Fatalf("profile response leaked secret: %s", recorder.Body.String())
	}

	update := `{"name":"Profile Build","runtime_id":"grok-build","provider_id":"profile-provider","model_id":"model-b","status":"active"}`
	recorder = httptest.NewRecorder()
	handleAgentProfileItem(recorder, httptest.NewRequest(http.MethodPut, "/api/agent-profiles/profile-build", strings.NewReader(update)))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"revision":2`) {
		t.Fatalf("update status = %d: %s", recorder.Code, recorder.Body.String())
	}

	for _, action := range []string{"archive", "restore"} {
		recorder = httptest.NewRecorder()
		handleAgentProfileItem(recorder, httptest.NewRequest(http.MethodPost, "/api/agent-profiles/profile-build/"+action, nil))
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"status":"`+map[string]string{"archive": "archived", "restore": "active"}[action]+`"`) {
			t.Fatalf("%s status = %d: %s", action, recorder.Code, recorder.Body.String())
		}
	}

	recorder = httptest.NewRecorder()
	handleAgentProfiles(recorder, httptest.NewRequest(http.MethodGet, "/api/agent-profiles", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"runtimes"`) || !strings.Contains(recorder.Body.String(), `"profile-build"`) {
		t.Fatalf("list status = %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestAgentRuntimeReadsDesktopConfigFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	files := map[string]string{
		filepath.Join(home, ".claude", "settings.json"): `{"env":{"ANTHROPIC_BASE_URL":"https://claude.test","ANTHROPIC_MODEL":"claude-model"}}`,
		filepath.Join(home, ".codex", "config.toml"):    "model = \"codex-model\"\n",
	}
	for path, contents := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	recorder := httptest.NewRecorder()
	handleAgentRuntime(recorder, httptest.NewRequest(http.MethodGet, "/api/agents/runtime", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	for _, value := range []string{"claude-model", "codex-model", "https://claude.test"} {
		if !strings.Contains(recorder.Body.String(), value) {
			t.Fatalf("runtime response missing %q: %s", value, recorder.Body.String())
		}
	}
	if strings.Contains(recorder.Body.String(), "gemini") {
		t.Fatalf("retired gemini runtime is still exposed: %s", recorder.Body.String())
	}
}

func TestAgentBindingPreviewThenApply(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	store := useTestProviderStore(t)
	addBindingTestProvider(t, store)
	body := `{"binding":{"agent_id":"claude","provider_id":"binding-provider","model_id":"model-a","options":{"thinking_enabled":true,"thinking_budget_tokens":12000}},"apply":false}`
	recorder := httptest.NewRecorder()
	handleAgentBinding(recorder, httptest.NewRequest(http.MethodPost, "/api/agents/binding", strings.NewReader(body)))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"plan"`) {
		t.Fatalf("preview status = %d: %s", recorder.Code, recorder.Body.String())
	}
	configPath := filepath.Join(home, ".claude", "settings.json")
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("preview wrote config: %v", err)
	}

	body = strings.Replace(body, `"apply":false`, `"apply":true`, 1)
	recorder = httptest.NewRecorder()
	handleAgentBinding(recorder, httptest.NewRequest(http.MethodPost, "/api/agents/binding", strings.NewReader(body)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("apply status = %d: %s", recorder.Code, recorder.Body.String())
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"ANTHROPIC_MODEL": "model-a"`) || !strings.Contains(string(data), `"MAX_THINKING_TOKENS": "12000"`) {
		t.Fatalf("unexpected applied config: %s", data)
	}
	bindings, err := store.ListBindings()
	if err != nil || len(bindings) != 1 || bindings[0].ModelID != "model-a" {
		t.Fatalf("binding not persisted: %#v, %v", bindings, err)
	}
}

func TestAgentBindingRollsBackWhenPersistenceFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	store := useTestProviderStore(t)
	addBindingTestProvider(t, store)
	configPath := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatal(err)
	}
	before := []byte(`{"permissions":{"allow":["Read"]},"env":{"KEEP":"yes"}}`)
	if err := os.WriteFile(configPath, before, 0o600); err != nil {
		t.Fatal(err)
	}
	previousPersist := persistAgentBinding
	persistAgentBinding = func(provider.AgentBinding) error { return errors.New("forced persistence failure") }
	t.Cleanup(func() { persistAgentBinding = previousPersist })

	body := `{"binding":{"agent_id":"claude","provider_id":"binding-provider","model_id":"model-a"},"apply":true}`
	recorder := httptest.NewRecorder()
	handleAgentBinding(recorder, httptest.NewRequest(http.MethodPost, "/api/agents/binding", strings.NewReader(body)))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("rollback did not restore original file:\nwant %s\n got %s", before, after)
	}
}
