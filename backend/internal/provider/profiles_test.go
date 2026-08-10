package provider

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSchemaV4SeedsAndResolvesDeepSeekBuildProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "providers.json")
	data := `{"schema_version":3,"providers":[{"id":"deepseek-api","name":"DeepSeek API","api_key":"top-secret","model":"deepseek-v4-flash","endpoints":[{"agent_id":"codex","protocol":"openai_responses","base_url":"https://api.deepseek.test/v1"}]}],"models":[{"provider_id":"deepseek-api","model_id":"deepseek-v4-flash","source":"manual","available":true}]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	store := NewStore(path)
	pd, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if pd.SchemaVersion != 4 {
		t.Fatalf("schema version = %d", pd.SchemaVersion)
	}
	backup, err := os.ReadFile(path + ".v3.bak")
	if err != nil || string(backup) != data {
		t.Fatalf("v3 backup = %q, %v", backup, err)
	}
	if len(pd.Providers[0].Endpoints) != 1 || pd.Providers[0].Endpoints[0].Family != EndpointFamilyOpenAI || pd.Providers[0].Endpoints[0].AgentID != "" {
		t.Fatalf("migrated endpoint = %#v", pd.Providers[0].Endpoints)
	}
	profile := profileByID(pd.Profiles, DeepSeekBuildProfileID)
	if profile == nil || profile.Status != ProfileStatusActive || profile.ProviderID != LegacyDeepSeekProviderID {
		t.Fatalf("seeded profile = %#v", profile)
	}
	launch, err := store.ResolveProfile(DeepSeekBuildProfileID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(launch.Argv, " ") != "grok agent --model deepseek-v4-flash stdio" {
		t.Fatalf("argv = %#v", launch.Argv)
	}
	if launch.Credentials["xai.api_key"] != "top-secret" {
		t.Fatalf("credential was not resolved")
	}
	encoded, err := json.Marshal(launch)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "top-secret") {
		t.Fatalf("launch JSON leaked credential: %s", encoded)
	}
}

func TestProfileCRUDRevisionArchiveAndRestore(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "providers.json"))
	provider, err := store.AddOrUpdate(Provider{
		ID:       "kimi",
		Name:     "Kimi",
		APIKey:   "secret",
		Model:    "kimi-k2",
		ModelIDs: []string{"kimi-k2"},
		Endpoints: []ProviderEndpoint{{
			Family:   EndpointFamilyOpenAI,
			Protocol: "openai_chat",
			BaseURL:  "https://kimi.test/v1",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := store.AddProfile(AgentProfile{Name: "Kimi Build", RuntimeID: GrokBuildRuntimeID, ProviderID: provider.ID, ModelID: "kimi-k2"})
	if err != nil {
		t.Fatal(err)
	}
	if profile.Revision != 1 || profile.ID != "kimi-build" {
		t.Fatalf("created profile = %#v", profile)
	}
	if _, err := store.AddOrUpdate(Provider{
		ID:       "kimi",
		Name:     "Kimi",
		APIKey:   "secret",
		Model:    "kimi-k2-thinking",
		ModelIDs: []string{"kimi-k2", "kimi-k2-thinking"},
		Endpoints: []ProviderEndpoint{{
			Family:   EndpointFamilyOpenAI,
			Protocol: "openai_chat",
			BaseURL:  "https://kimi.test/v1",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	profile.ModelID = "kimi-k2-thinking"
	updated, err := store.UpdateProfile(profile.ID, *profile)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != 2 {
		t.Fatalf("revision = %d", updated.Revision)
	}
	archived, err := store.SetProfileStatus(profile.ID, ProfileStatusArchived)
	if err != nil {
		t.Fatal(err)
	}
	if archived.Status != ProfileStatusArchived || archived.Revision != 3 {
		t.Fatalf("archived profile = %#v", archived)
	}
	if _, err := store.ResolveProfile(profile.ID); err == nil || !strings.Contains(err.Error(), "archived") {
		t.Fatalf("resolve archived profile err = %v", err)
	}
	restored, err := store.SetProfileStatus(profile.ID, ProfileStatusActive)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Status != ProfileStatusActive || restored.Revision != 4 {
		t.Fatalf("restored profile = %#v", restored)
	}
}

func TestReferencedProviderIsArchivedWithoutCascadingProfile(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "providers.json"))
	_, err := store.AddOrUpdate(Provider{ID: "referenced", Name: "Referenced", APIKey: "secret", Model: "model-a", Endpoints: []ProviderEndpoint{{Family: EndpointFamilyOpenAI, BaseURL: "https://example.test/v1"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddProfile(AgentProfile{ID: "referenced-build", Name: "Referenced Build", RuntimeID: GrokBuildRuntimeID, ProviderID: "referenced", ModelID: "model-a"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete("referenced"); err != nil {
		t.Fatal(err)
	}
	saved, err := store.Get("referenced")
	if err != nil || saved.Status != ProfileStatusArchived {
		t.Fatalf("referenced provider = %#v, %v", saved, err)
	}
	profile, err := store.GetProfile("referenced-build")
	if err != nil || profile.ProviderID != "referenced" {
		t.Fatalf("referencing profile was removed: %#v, %v", profile, err)
	}
}

func TestResolveProfileRejectsUnavailableModelAndMissingCredential(t *testing.T) {
	path := filepath.Join(t.TempDir(), "providers.json")
	pd := ProviderData{
		SchemaVersion: CurrentSchemaVersion,
		Providers:     []Provider{{ID: "relay", Name: "Relay", Status: ProfileStatusActive, Endpoints: []ProviderEndpoint{{Family: EndpointFamilyOpenAI, BaseURL: "https://relay.test/v1"}}}},
		Models:        []ProviderModel{{ProviderID: "relay", ModelID: "gone", Source: "remote", Available: false}},
		Profiles:      []AgentProfile{{ID: "relay-build", Name: "Relay Build", RuntimeID: GrokBuildRuntimeID, ProviderID: "relay", ModelID: "gone", Revision: 1, Status: ProfileStatusActive}},
	}
	store := NewStore(path)
	if err := store.Save(&pd); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveProfile("relay-build"); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("unavailable model err = %v", err)
	}
	pd.Models[0].Available = true
	if err := store.Save(&pd); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveProfile("relay-build"); err == nil || !strings.Contains(err.Error(), "no credential") {
		t.Fatalf("missing credential err = %v", err)
	}
}

func TestDeepSeekSeedDoesNotActivateUnavailableLegacyModel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "providers.json")
	pd := ProviderData{
		SchemaVersion: CurrentSchemaVersion,
		Providers: []Provider{{
			ID: LegacyDeepSeekProviderID, Name: "DeepSeek", APIKey: "secret", Model: DeepSeekDefaultModelID,
			Endpoints: []ProviderEndpoint{{Family: EndpointFamilyOpenAI, BaseURL: "https://deepseek.test/v1"}},
		}},
		Models: []ProviderModel{{
			ProviderID: LegacyDeepSeekProviderID, ModelID: DeepSeekDefaultModelID, Available: false,
		}},
	}
	store := NewStore(path)
	if err := store.Save(&pd); err != nil {
		t.Fatal(err)
	}
	profile, err := store.GetProfile(DeepSeekBuildProfileID)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Status != ProfileStatusDisabled || profile.ModelID != "" {
		t.Fatalf("unavailable legacy model activated system profile: %#v", profile)
	}
}
