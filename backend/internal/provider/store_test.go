package provider

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestProviderStore_CRUD(t *testing.T) {
	tempDir := t.TempDir()
	testPath := filepath.Join(tempDir, "providers.json")

	store := NewStore(testPath)

	// 1. Initial Load should create default presets
	pd, err := store.Load()
	if err != nil {
		t.Fatalf("Failed to initial load: %v", err)
	}
	if len(pd.Providers) == 0 {
		t.Errorf("Expected default providers, got empty list")
	}
	if pd.ActiveProviderID == "" {
		t.Errorf("Expected active_provider_id to be set")
	}

	// Verify file was written to disk
	if _, err := os.Stat(testPath); os.IsNotExist(err) {
		t.Fatalf("Expected file %s to be created", testPath)
	}

	// 2. Add a new Provider
	newP := Provider{
		Name:     "Test DeepSeek",
		Protocol: "openai",
		BaseURL:  "https://api.deepseek.com/v1",
		APIKey:   "sk-testkey123",
		Model:    "deepseek-coder",
		Apps:     []string{"claude", "codex"},
	}

	added, err := store.AddOrUpdate(newP)
	if err != nil {
		t.Fatalf("Failed to add provider: %v", err)
	}
	if added.ID != "test-deepseek" {
		t.Errorf("Expected ID 'test-deepseek', got %q", added.ID)
	}

	// 3. Get Provider
	got, err := store.Get("test-deepseek")
	if err != nil {
		t.Fatalf("Failed to get provider: %v", err)
	}
	if got.APIKey != "sk-testkey123" {
		t.Errorf("Expected APIKey 'sk-testkey123', got %q", got.APIKey)
	}

	// 4. Set Active
	active, err := store.SetActive("test-deepseek")
	if err != nil {
		t.Fatalf("Failed to set active provider: %v", err)
	}
	if active.ID != "test-deepseek" {
		t.Errorf("Expected active ID 'test-deepseek', got %q", active.ID)
	}

	// Verify persistence from file
	raw, err := os.ReadFile(testPath)
	if err != nil {
		t.Fatalf("Failed to read raw file: %v", err)
	}
	var filePD ProviderData
	if err := json.Unmarshal(raw, &filePD); err != nil {
		t.Fatalf("Failed to parse persisted JSON: %v", err)
	}
	if filePD.ActiveProviderID != "test-deepseek" {
		t.Errorf("Persisted active ID mismatch: got %q, want 'test-deepseek'", filePD.ActiveProviderID)
	}

	// 5. Update existing Provider
	added.Model = "deepseek-reasoner"
	updated, err := store.AddOrUpdate(*added)
	if err != nil {
		t.Fatalf("Failed to update provider: %v", err)
	}
	if updated.Model != "deepseek-reasoner" {
		t.Errorf("Expected model 'deepseek-reasoner', got %q", updated.Model)
	}

	// 6. Delete Provider
	if err := store.Delete("test-deepseek"); err != nil {
		t.Fatalf("Failed to delete provider: %v", err)
	}
	_, err = store.Get("test-deepseek")
	if err == nil {
		t.Errorf("Expected error after deleting provider, got nil")
	}
}

func TestDefaultStoreOnlyWritesOneAgentsDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	store := NewStore("")
	if _, err := store.Load(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(home, ".1agents", "providers.json"))
	if err != nil {
		t.Fatalf("expected canonical provider file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("provider file permission = %o, want 600", info.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "providers.json")); !os.IsNotExist(err) {
		t.Fatalf("unexpected ~/.agents/providers.json: %v", err)
	}
}
