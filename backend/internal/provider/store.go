package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Provider represents an LLM API provider entry.
type Provider struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Protocol         string   `json:"protocol"` // "anthropic", "openai", "gemini", "dual"
	BaseURL          string   `json:"base_url"`
	AnthropicBaseURL string   `json:"anthropic_base_url,omitempty"`
	OpenAIBaseURL    string   `json:"openai_base_url,omitempty"`
	APIKey           string   `json:"api_key"`
	Model            string   `json:"model"`
	ModelIDs         []string `json:"model_ids,omitempty"`
	HaikuModel       string   `json:"haiku_model,omitempty"`
	SonnetModel      string   `json:"sonnet_model,omitempty"`
	OpusModel        string   `json:"opus_model,omitempty"`
	Apps             []string `json:"apps,omitempty"` // e.g. ["claude", "codex", "gemini"]
	CreatedAt        int64    `json:"created_at"`
	UpdatedAt        int64    `json:"updated_at"`
}

// ProviderData is the root JSON structure saved at ~/.1agents/providers.json.
type ProviderData struct {
	ActiveProviderID string     `json:"active_provider_id"`
	Providers        []Provider `json:"providers"`
}

// AgentStatus represents the active runtime configuration of a specific local agent CLI.
type AgentStatus struct {
	Agent       string `json:"agent"` // "claude", "codex", "gemini"
	ProviderID  string `json:"provider_id,omitempty"`
	BaseURL     string `json:"base_url"`
	Model       string `json:"model"`
	HaikuModel  string `json:"haiku_model,omitempty"`
	SonnetModel string `json:"sonnet_model,omitempty"`
	OpusModel   string `json:"opus_model,omitempty"`
}

// AgentSyncRequest carries fine-tuning model overrides when binding an Agent to a Provider.
type AgentSyncRequest struct {
	Agent       string `json:"agent"` // "claude", "codex", "gemini"
	ProviderID  string `json:"provider_id"`
	Model       string `json:"model,omitempty"`
	HaikuModel  string `json:"haiku_model,omitempty"`
	SonnetModel string `json:"sonnet_model,omitempty"`
	OpusModel   string `json:"opus_model,omitempty"`
}

// Store manages thread-safe CRUD and persistence for ~/.1agents/providers.json.
type Store struct {
	mu       sync.RWMutex
	filePath string
}

// DefaultPath returns the default path for ~/.1agents/providers.json.
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".1agents", "providers.json")
}

// AgentsStdPath returns the ClawBox standard path ~/.agents/providers.json.
func AgentsStdPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".agents", "providers.json")
}

// NewStore creates a Store with specified path (or DefaultPath if empty).
func NewStore(filePath string) *Store {
	if filePath == "" {
		filePath = DefaultPath()
	}
	return &Store{filePath: filePath}
}

// Load reads and parses provider data from disk. If file does not exist,
// initializes an empty dataset with standard presets and persists it.
func (s *Store) Load() (*ProviderData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.loadUnlocked()
}

func (s *Store) loadUnlocked() (*ProviderData, error) {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			if stdData, stdErr := os.ReadFile(AgentsStdPath()); stdErr == nil {
				var pd ProviderData
				if jsonErr := json.Unmarshal(stdData, &pd); jsonErr == nil {
					_ = s.saveUnlocked(&pd)
					return &pd, nil
				}
			}

			defaultData := s.defaultPresets()
			if saveErr := s.saveUnlocked(defaultData); saveErr != nil {
				return nil, fmt.Errorf("init default providers: %w", saveErr)
			}
			return defaultData, nil
		}
		return nil, fmt.Errorf("read providers file: %w", err)
	}

	var pd ProviderData
	if err := json.Unmarshal(data, &pd); err != nil {
		return nil, fmt.Errorf("parse providers json: %w", err)
	}
	if pd.Providers == nil {
		pd.Providers = []Provider{}
	}
	return &pd, nil
}

// Save writes ProviderData atomically to ~/.1agents/providers.json and syncs to ~/.agents/providers.json.
func (s *Store) Save(pd *ProviderData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.saveUnlocked(pd)
}

func (s *Store) saveUnlocked(pd *ProviderData) error {
	if pd == nil {
		return errors.New("cannot save nil provider data")
	}

	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create dir %s: %w", dir, err)
	}

	bytes, err := json.MarshalIndent(pd, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal providers: %w", err)
	}

	tmpFile := s.filePath + ".tmp"
	if err := os.WriteFile(tmpFile, bytes, 0600); err != nil {
		return fmt.Errorf("write tmp file: %w", err)
	}

	if err := os.Rename(tmpFile, s.filePath); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("atomic rename providers file: %w", err)
	}

	agentsDir := filepath.Dir(AgentsStdPath())
	if err := os.MkdirAll(agentsDir, 0755); err == nil {
		_ = os.WriteFile(AgentsStdPath(), bytes, 0600)
	}

	return nil
}

// List returns all providers and the current active provider ID.
func (s *Store) List() ([]Provider, string, error) {
	pd, err := s.Load()
	if err != nil {
		return nil, "", err
	}
	return pd.Providers, pd.ActiveProviderID, nil
}

// Get returns a single provider by ID.
func (s *Store) Get(id string) (*Provider, error) {
	pd, err := s.Load()
	if err != nil {
		return nil, err
	}
	for _, p := range pd.Providers {
		if p.ID == id {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("provider with id %q not found", id)
}

// AddOrUpdate inserts or updates a provider.
func (s *Store) AddOrUpdate(p Provider) (*Provider, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pd, err := s.loadUnlocked()
	if err != nil {
		return nil, err
	}

	if p.ID == "" {
		p.ID = sanitizeID(p.Name)
	}
	if p.ID == "" {
		return nil, errors.New("provider ID or valid Name is required")
	}
	if p.Protocol == "" {
		p.Protocol = "openai"
	}

	now := time.Now().Unix()
	found := false
	for i, existing := range pd.Providers {
		if existing.ID == p.ID {
			p.CreatedAt = existing.CreatedAt
			p.UpdatedAt = now
			pd.Providers[i] = p
			found = true
			break
		}
	}

	if !found {
		p.CreatedAt = now
		p.UpdatedAt = now
		pd.Providers = append(pd.Providers, p)
	}

	if pd.ActiveProviderID == "" {
		pd.ActiveProviderID = p.ID
	}

	if err := s.saveUnlocked(pd); err != nil {
		return nil, err
	}

	return &p, nil
}

// Delete removes a provider by ID.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	pd, err := s.loadUnlocked()
	if err != nil {
		return err
	}

	index := -1
	for i, p := range pd.Providers {
		if p.ID == id {
			index = i
			break
		}
	}
	if index == -1 {
		return fmt.Errorf("provider with id %q not found", id)
	}

	pd.Providers = append(pd.Providers[:index], pd.Providers[index+1:]...)
	if pd.ActiveProviderID == id {
		if len(pd.Providers) > 0 {
			pd.ActiveProviderID = pd.Providers[0].ID
		} else {
			pd.ActiveProviderID = ""
		}
	}

	return s.saveUnlocked(pd)
}

// SetActive sets the active provider ID and syncs config files to local agents.
func (s *Store) SetActive(id string) (*Provider, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pd, err := s.loadUnlocked()
	if err != nil {
		return nil, err
	}

	var target *Provider
	for i, p := range pd.Providers {
		if p.ID == id {
			target = &pd.Providers[i]
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("provider with id %q not found", id)
	}

	pd.ActiveProviderID = id
	if err := s.saveUnlocked(pd); err != nil {
		return nil, err
	}

	if err := SyncActiveToAgentConfigs(target); err != nil {
		fmt.Printf("[provider] warning syncing active provider: %v\n", err)
	}

	return target, nil
}

// SyncAgentConfig applies a specific provider and optional model fine-tuning to a target agent CLI.
func (s *Store) SyncAgentConfig(req AgentSyncRequest) error {
	p, err := s.Get(req.ProviderID)
	if err != nil {
		return err
	}

	// Create a temporary Provider instance with model fine-tuning overrides
	effProvider := *p
	if req.Model != "" {
		effProvider.Model = req.Model
	}
	if req.HaikuModel != "" {
		effProvider.HaikuModel = req.HaikuModel
	}
	if req.SonnetModel != "" {
		effProvider.SonnetModel = req.SonnetModel
	}
	if req.OpusModel != "" {
		effProvider.OpusModel = req.OpusModel
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get user home: %w", err)
	}

	switch strings.ToLower(req.Agent) {
	case "claude", "claudecode":
		return syncClaudeConfig(home, &effProvider)
	case "codex":
		return syncCodexConfig(home, &effProvider)
	case "gemini":
		return syncGeminiConfig(home, &effProvider)
	default:
		return fmt.Errorf("unsupported agent %q", req.Agent)
	}
}

// FetchModelsFromEndpoint calls HTTP GET {base_url}/models and returns available model IDs.
func FetchModelsFromEndpoint(baseURL, apiKey string) ([]string, error) {
	baseURL = strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, errors.New("base_url is required")
	}

	url := baseURL + "/models"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned HTTP %d", resp.StatusCode)
	}

	var parsed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("parse json: %w", err)
	}

	models := make([]string, 0, len(parsed.Data))
	for _, m := range parsed.Data {
		if strings.TrimSpace(m.ID) != "" {
			models = append(models, m.ID)
		}
	}
	return models, nil
}

// SyncActiveToAgentConfigs writes active provider settings directly to local agent config files.
func SyncActiveToAgentConfigs(p *Provider) error {
	if p == nil {
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get user home: %w", err)
	}

	_ = syncClaudeConfig(home, p)
	_ = syncCodexConfig(home, p)
	_ = syncGeminiConfig(home, p)

	return nil
}

func syncClaudeConfig(home string, p *Provider) error {
	configPath := filepath.Join(home, ".claude.json")

	var root map[string]any
	data, err := os.ReadFile(configPath)
	if err == nil {
		_ = json.Unmarshal(data, &root)
	}
	if root == nil {
		root = make(map[string]any)
	}

	envMap, _ := root["env"].(map[string]any)
	if envMap == nil {
		envMap = make(map[string]any)
	}

	baseURL := p.AnthropicBaseURL
	if baseURL == "" {
		baseURL = p.BaseURL
	}
	if baseURL != "" {
		envMap["ANTHROPIC_BASE_URL"] = baseURL
	}
	if p.APIKey != "" {
		envMap["ANTHROPIC_AUTH_TOKEN"] = p.APIKey
	}

	mainModel := p.SonnetModel
	if mainModel == "" {
		mainModel = p.Model
	}
	if mainModel != "" {
		envMap["ANTHROPIC_MODEL"] = mainModel
	}

	haiku := p.HaikuModel
	if haiku == "" {
		haiku = mainModel
	}
	sonnet := p.SonnetModel
	if sonnet == "" {
		sonnet = mainModel
	}
	opus := p.OpusModel
	if opus == "" {
		opus = mainModel
	}
	if haiku != "" {
		envMap["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = haiku
	}
	if sonnet != "" {
		envMap["ANTHROPIC_DEFAULT_SONNET_MODEL"] = sonnet
	}
	if opus != "" {
		envMap["ANTHROPIC_DEFAULT_OPUS_MODEL"] = opus
	}

	root["env"] = envMap

	bytes, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, bytes, 0600)
}

func syncCodexConfig(home string, p *Provider) error {
	dir := filepath.Join(home, ".codex")
	_ = os.MkdirAll(dir, 0755)

	baseURL := p.OpenAIBaseURL
	if baseURL == "" {
		baseURL = p.BaseURL
	}

	modelID := p.Model
	if modelID == "" {
		modelID = p.SonnetModel
	}

	configPath := filepath.Join(dir, "config.toml")
	content := fmt.Sprintf("# Auto-generated by 1agents provider manager\nbase_url = %q\nmodel = %q\n", baseURL, modelID)
	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		return err
	}

	authPath := filepath.Join(dir, "auth.json")
	authData := map[string]string{
		"OPENAI_API_KEY": p.APIKey,
	}
	authBytes, _ := json.MarshalIndent(authData, "", "  ")
	return os.WriteFile(authPath, authBytes, 0600)
}

func syncGeminiConfig(home string, p *Provider) error {
	dir := filepath.Join(home, ".config", "gemini")
	_ = os.MkdirAll(dir, 0755)

	baseURL := p.OpenAIBaseURL
	if baseURL == "" {
		baseURL = p.BaseURL
	}

	modelID := p.Model

	configPath := filepath.Join(dir, "config.json")
	cfgData := map[string]string{
		"GOOGLE_GEMINI_BASE_URL": baseURL,
		"GEMINI_API_KEY":         p.APIKey,
		"GEMINI_MODEL":           modelID,
	}
	bytes, _ := json.MarshalIndent(cfgData, "", "  ")
	return os.WriteFile(configPath, bytes, 0600)
}

func (s *Store) defaultPresets() *ProviderData {
	now := time.Now().Unix()
	return &ProviderData{
		ActiveProviderID: "anthropic-official",
		Providers: []Provider{
			{
				ID:               "anthropic-official",
				Name:             "Anthropic (Official Direct)",
				Protocol:         "anthropic",
				BaseURL:          "https://api.anthropic.com",
				AnthropicBaseURL: "https://api.anthropic.com",
				APIKey:           "",
				Model:            "claude-3-7-sonnet-20250219",
				HaikuModel:       "claude-3-5-haiku-20241022",
				SonnetModel:      "claude-3-7-sonnet-20250219",
				OpusModel:        "claude-3-opus-20240229",
				CreatedAt:        now,
				UpdatedAt:        now,
			},
			{
				ID:            "deepseek-api",
				Name:          "DeepSeek API",
				Protocol:      "dual",
				BaseURL:       "https://api.deepseek.com/v1",
				OpenAIBaseURL: "https://api.deepseek.com/v1",
				AnthropicBaseURL: "https://api.deepseek.com/beta",
				APIKey:        "",
				Model:         "deepseek-chat",
				SonnetModel:   "deepseek-chat",
				HaikuModel:    "deepseek-chat",
				OpusModel:     "deepseek-reasoner",
				CreatedAt:     now,
				UpdatedAt:     now,
			},
			{
				ID:            "openrouter-api",
				Name:          "OpenRouter",
				Protocol:      "openai",
				BaseURL:       "https://openrouter.ai/api/v1",
				OpenAIBaseURL: "https://openrouter.ai/api/v1",
				APIKey:        "",
				Model:         "anthropic/claude-3.7-sonnet",
				CreatedAt:     now,
				UpdatedAt:     now,
			},
		},
	}
}

func sanitizeID(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "_", "-")
	return s
}
