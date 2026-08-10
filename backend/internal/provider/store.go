package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Provider represents an LLM API provider entry.
type Provider struct {
	ID               string             `json:"id"`
	Name             string             `json:"name"`
	Protocol         string             `json:"protocol"` // "anthropic", "openai", "dual"
	BaseURL          string             `json:"base_url"`
	AnthropicBaseURL string             `json:"anthropic_base_url,omitempty"`
	OpenAIBaseURL    string             `json:"openai_base_url,omitempty"`
	APIKey           string             `json:"api_key"`
	HasAPIKey        bool               `json:"has_api_key,omitempty"`
	Model            string             `json:"model"`
	ModelIDs         []string           `json:"model_ids,omitempty"`
	HaikuModel       string             `json:"haiku_model,omitempty"`
	SonnetModel      string             `json:"sonnet_model,omitempty"`
	OpusModel        string             `json:"opus_model,omitempty"`
	Apps             []string           `json:"apps,omitempty"` // cc-switch source types
	Endpoints        []ProviderEndpoint `json:"endpoints,omitempty"`
	Status           string             `json:"status,omitempty"`
	CreatedAt        int64              `json:"created_at"`
	UpdatedAt        int64              `json:"updated_at"`
}

// ProviderData is the root JSON structure saved at ~/.1agents/providers.json.
type ProviderData struct {
	SchemaVersion    int              `json:"schema_version,omitempty"`
	ActiveProviderID string           `json:"active_provider_id"`
	Providers        []Provider       `json:"providers"`
	Models           []ProviderModel  `json:"models,omitempty"`
	Bindings         []AgentBinding   `json:"bindings,omitempty"`
	Profiles         []AgentProfile   `json:"profiles,omitempty"`
	Migrations       map[string]int64 `json:"migrations,omitempty"`
}

// AgentStatus represents the active runtime configuration of a specific local agent CLI.
type AgentStatus struct {
	Agent       string `json:"agent"` // "claude", "codex"
	ProviderID  string `json:"provider_id,omitempty"`
	BaseURL     string `json:"base_url"`
	Model       string `json:"model"`
	HaikuModel  string `json:"haiku_model,omitempty"`
	SonnetModel string `json:"sonnet_model,omitempty"`
	OpusModel   string `json:"opus_model,omitempty"`
}

// AgentSyncRequest carries fine-tuning model overrides when binding an Agent to a Provider.
type AgentSyncRequest struct {
	Agent       string `json:"agent"` // "claude", "codex"
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
			defaultData := s.defaultPresets()
			if _, importErr := s.importLegacyCCSwitch(defaultData); importErr != nil {
				return nil, importErr
			}
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
	previousVersion := pd.SchemaVersion
	if previousVersion == 3 {
		if err := s.writeV3Backup(data); err != nil {
			return nil, err
		}
	}
	s.normalize(&pd)
	imported, err := s.importLegacyCCSwitch(&pd)
	if err != nil {
		return nil, err
	}
	if previousVersion < CurrentSchemaVersion || imported {
		if err := s.saveUnlocked(&pd); err != nil {
			return nil, fmt.Errorf("persist provider migration: %w", err)
		}
	}
	return &pd, nil
}

// normalize upgrades the legacy flat format in memory. Legacy fields remain
// populated so older frontend builds continue to work during the migration.
func (s *Store) normalize(pd *ProviderData) {
	previousVersion := pd.SchemaVersion
	now := time.Now().Unix()
	for i := range pd.Providers {
		p := &pd.Providers[i]
		p.Endpoints = canonicalEndpoints(p.Endpoints)
		if p.Status == "" {
			p.Status = ProfileStatusActive
		}
		p.Apps = withoutStrings(p.Apps, "gemini")
		if len(p.Endpoints) == 0 {
			if p.AnthropicBaseURL != "" || p.Protocol == "anthropic" || p.Protocol == "dual" {
				baseURL := p.AnthropicBaseURL
				if baseURL == "" {
					baseURL = p.BaseURL
				}
				p.Endpoints = append(p.Endpoints, ProviderEndpoint{Family: EndpointFamilyAnthropic, Protocol: "anthropic", BaseURL: baseURL})
			}
			if p.OpenAIBaseURL != "" || p.Protocol == "openai" || p.Protocol == "dual" {
				baseURL := p.OpenAIBaseURL
				if baseURL == "" {
					baseURL = p.BaseURL
				}
				p.Endpoints = append(p.Endpoints, ProviderEndpoint{Family: EndpointFamilyOpenAI, Protocol: "openai_responses", BaseURL: baseURL})
			}
		}
		if previousVersion < CurrentSchemaVersion {
			seen := map[string]bool{}
			for _, modelID := range append(append([]string{}, p.ModelIDs...), p.Model) {
				modelID = strings.TrimSpace(modelID)
				if modelID == "" || seen[modelID] {
					continue
				}
				seen[modelID] = true
				upsertModel(&pd.Models, ProviderModel{ProviderID: p.ID, ModelID: modelID, Source: "legacy", Available: true, DiscoveredAt: now, LastSeenAt: now})
			}
		}
	}
	bindings := pd.Bindings[:0]
	for _, binding := range pd.Bindings {
		if binding.AgentID == AgentClaude || binding.AgentID == AgentCodex {
			bindings = append(bindings, binding)
		}
	}
	pd.Bindings = bindings
	if previousVersion < CurrentSchemaVersion && len(pd.Bindings) == 0 && pd.ActiveProviderID != "" {
		if p := providerByID(pd.Providers, pd.ActiveProviderID); p != nil {
			for _, endpoint := range p.Endpoints {
				agentID := agentIDForFamily(endpoint.Family)
				if agentID == "" {
					continue
				}
				mapping := map[string]string{}
				if agentID == AgentClaude {
					mapping = map[string]string{"haiku": p.HaikuModel, "sonnet": p.SonnetModel, "opus": p.OpusModel}
				}
				pd.Bindings = append(pd.Bindings, AgentBinding{AgentID: agentID, ProviderID: p.ID, ModelID: p.Model, ModelMapping: compactMapping(mapping), UpdatedAt: now})
			}
		}
	}
	normalizeProfiles(pd, now)
	pd.SchemaVersion = CurrentSchemaVersion
}

func canonicalEndpoints(endpoints []ProviderEndpoint) []ProviderEndpoint {
	out := make([]ProviderEndpoint, 0, 2)
	for _, endpoint := range endpoints {
		family := endpoint.Family
		if family == "" {
			family = familyForLegacyEndpoint(endpoint)
		}
		if family != EndpointFamilyOpenAI && family != EndpointFamilyAnthropic {
			continue
		}
		endpoint.Family = family
		endpoint.AgentID = ""
		upsertEndpoint(&out, endpoint)
	}
	return out
}

func familyForLegacyEndpoint(endpoint ProviderEndpoint) EndpointFamily {
	switch endpoint.AgentID {
	case AgentClaude:
		return EndpointFamilyAnthropic
	case AgentCodex:
		return EndpointFamilyOpenAI
	case AgentOpenClaw, AgentOpenCode:
		if strings.Contains(strings.ToLower(endpoint.Protocol), "anthropic") {
			return EndpointFamilyAnthropic
		}
		return EndpointFamilyOpenAI
	}
	if strings.Contains(strings.ToLower(endpoint.Protocol), "anthropic") {
		return EndpointFamilyAnthropic
	}
	if strings.Contains(strings.ToLower(endpoint.Protocol), "openai") {
		return EndpointFamilyOpenAI
	}
	return ""
}

func agentIDForFamily(family EndpointFamily) AgentID {
	switch family {
	case EndpointFamilyAnthropic:
		return AgentClaude
	case EndpointFamilyOpenAI:
		return AgentCodex
	default:
		return ""
	}
}

func hasEndpoint(endpoints []ProviderEndpoint, family EndpointFamily) bool {
	for _, endpoint := range endpoints {
		if endpoint.Family == family {
			return true
		}
	}
	return false
}

func (s *Store) writeV3Backup(data []byte) error {
	backupPath := s.filePath + ".v3.bak"
	file, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("create provider schema v3 backup: %w", err)
	}
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("write provider schema v3 backup: %w", err)
	}
	return nil
}

func withoutStrings(values []string, removed string) []string {
	out := values[:0]
	for _, value := range values {
		if !strings.EqualFold(value, removed) {
			out = append(out, value)
		}
	}
	return out
}

func providerByID(providers []Provider, id string) *Provider {
	for i := range providers {
		if providers[i].ID == id {
			return &providers[i]
		}
	}
	return nil
}

func RedactedProvider(p Provider) Provider {
	p.HasAPIKey = p.APIKey != ""
	p.APIKey = ""
	for i := range p.Endpoints {
		p.Endpoints[i].HasAPIKey = p.Endpoints[i].APIKey != ""
		p.Endpoints[i].APIKey = ""
		p.Endpoints[i].HasHeaders = len(p.Endpoints[i].Headers) > 0
		p.Endpoints[i].HeaderNames = make([]string, 0, len(p.Endpoints[i].Headers))
		for name := range p.Endpoints[i].Headers {
			p.Endpoints[i].HeaderNames = append(p.Endpoints[i].HeaderNames, name)
		}
		sort.Strings(p.Endpoints[i].HeaderNames)
		p.Endpoints[i].Headers = nil
	}
	return p
}

func RedactedProviders(providers []Provider) []Provider {
	out := make([]Provider, len(providers))
	for i, p := range providers {
		out[i] = RedactedProvider(p)
	}
	return out
}

func CredentialForEndpoint(p Provider, baseURL string) string {
	for _, endpoint := range p.Endpoints {
		if strings.TrimSuffix(endpoint.BaseURL, "/") == strings.TrimSuffix(baseURL, "/") && endpoint.APIKey != "" {
			return endpoint.APIKey
		}
	}
	return p.APIKey
}

func compactMapping(values map[string]string) map[string]string {
	for key, value := range values {
		if strings.TrimSpace(value) == "" {
			delete(values, key)
		}
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

// Save writes ProviderData atomically to ~/.1agents/providers.json.
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

	s.normalize(pd)
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
	if p.Status == "" {
		p.Status = ProfileStatusActive
	}
	p.Endpoints = canonicalEndpoints(p.Endpoints)
	if len(p.Endpoints) == 0 {
		p.Endpoints = legacyEndpoints(p)
	}

	now := time.Now().Unix()
	found := false
	for i, existing := range pd.Providers {
		if existing.ID == p.ID {
			if p.APIKey == "" {
				p.APIKey = existing.APIKey
			}
			for endpointIndex := range p.Endpoints {
				for _, existingEndpoint := range existing.Endpoints {
					if existingEndpoint.Family == p.Endpoints[endpointIndex].Family {
						if p.Endpoints[endpointIndex].APIKey == "" {
							p.Endpoints[endpointIndex].APIKey = existingEndpoint.APIKey
						}
						if p.Endpoints[endpointIndex].Headers == nil {
							p.Endpoints[endpointIndex].Headers = existingEndpoint.Headers
						} else {
							for name, value := range p.Endpoints[endpointIndex].Headers {
								if value == "" {
									p.Endpoints[endpointIndex].Headers[name] = existingEndpoint.Headers[name]
								}
							}
						}
						break
					}
				}
			}
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
	for _, modelID := range append(append([]string{}, p.ModelIDs...), p.Model) {
		modelID = strings.TrimSpace(modelID)
		if modelID == "" {
			continue
		}
		exists := false
		for i := range pd.Models {
			if pd.Models[i].ProviderID == p.ID && pd.Models[i].ModelID == modelID {
				pd.Models[i].Available = true
				exists = true
				break
			}
		}
		if !exists {
			pd.Models = append(pd.Models, ProviderModel{ProviderID: p.ID, ModelID: modelID, Source: "manual", Available: true, DiscoveredAt: now, LastSeenAt: now})
		}
	}

	if pd.ActiveProviderID == "" {
		pd.ActiveProviderID = p.ID
	}

	if err := s.saveUnlocked(pd); err != nil {
		return nil, err
	}

	return &p, nil
}

func legacyEndpoints(p Provider) []ProviderEndpoint {
	var endpoints []ProviderEndpoint
	if p.AnthropicBaseURL != "" || p.Protocol == "anthropic" || p.Protocol == "dual" {
		baseURL := p.AnthropicBaseURL
		if baseURL == "" {
			baseURL = p.BaseURL
		}
		endpoints = append(endpoints, ProviderEndpoint{Family: EndpointFamilyAnthropic, Protocol: "anthropic", BaseURL: baseURL})
	}
	if p.OpenAIBaseURL != "" || p.Protocol == "openai" || p.Protocol == "dual" {
		baseURL := p.OpenAIBaseURL
		if baseURL == "" {
			baseURL = p.BaseURL
		}
		endpoints = append(endpoints, ProviderEndpoint{Family: EndpointFamilyOpenAI, Protocol: "openai_responses", BaseURL: baseURL})
	}
	return endpoints
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
	for _, profile := range pd.Profiles {
		if profile.ProviderID == id {
			pd.Providers[index].Status = ProfileStatusArchived
			pd.Providers[index].UpdatedAt = time.Now().Unix()
			return s.saveUnlocked(pd)
		}
	}

	pd.Providers = append(pd.Providers[:index], pd.Providers[index+1:]...)
	models := pd.Models[:0]
	for _, model := range pd.Models {
		if model.ProviderID != id {
			models = append(models, model)
		}
	}
	pd.Models = models
	bindings := pd.Bindings[:0]
	for _, binding := range pd.Bindings {
		if binding.ProviderID != id {
			bindings = append(bindings, binding)
		}
	}
	pd.Bindings = bindings
	if pd.ActiveProviderID == id {
		if len(pd.Providers) > 0 {
			pd.ActiveProviderID = pd.Providers[0].ID
		} else {
			pd.ActiveProviderID = ""
		}
	}

	return s.saveUnlocked(pd)
}

// SetActive preserves the legacy selection marker without mutating desktop
// agent files. Agent configuration is now changed only through AgentBinding.
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

	agentID := AgentID(strings.ToLower(req.Agent))
	switch agentID {
	case "claude", "claudecode":
		agentID = AgentClaude
	case "codex":
	default:
		return fmt.Errorf("unsupported agent %q", req.Agent)
	}
	binding := AgentBinding{AgentID: agentID, ProviderID: p.ID, ModelID: effProvider.Model, UpdatedAt: time.Now().Unix()}
	if agentID == AgentClaude {
		binding.ModelMapping = compactMapping(map[string]string{"haiku": effProvider.HaikuModel, "sonnet": effProvider.SonnetModel, "opus": effProvider.OpusModel})
	}
	if err := s.SetBinding(binding); err != nil {
		return err
	}
	_, err = ApplyAgentBinding(home, *p, binding)
	return err
}

func (s *Store) ListBindings() ([]AgentBinding, error) {
	pd, err := s.Load()
	if err != nil {
		return nil, err
	}
	return pd.Bindings, nil
}

func (s *Store) SetBinding(binding AgentBinding) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	pd, err := s.loadUnlocked()
	if err != nil {
		return err
	}
	if providerByID(pd.Providers, binding.ProviderID) == nil {
		return fmt.Errorf("provider with id %q not found", binding.ProviderID)
	}
	binding.UpdatedAt = time.Now().Unix()
	for i := range pd.Bindings {
		if pd.Bindings[i].AgentID == binding.AgentID {
			pd.Bindings[i] = binding
			return s.saveUnlocked(pd)
		}
	}
	pd.Bindings = append(pd.Bindings, binding)
	return s.saveUnlocked(pd)
}

// FetchModelsFromEndpoint calls HTTP GET {base_url}/models and returns available model IDs.
func FetchModelsFromEndpoint(baseURL, apiKey string) ([]string, error) {
	baseURL = strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, errors.New("base_url is required")
	}

	return fetchModelsFromURLs(modelCandidateURLs(baseURL), apiKey, nil)
}

// FetchModelsForEndpoint honors an explicit models_endpoint when configured.
func FetchModelsForEndpoint(endpoint ProviderEndpoint, apiKey string) ([]string, error) {
	if modelsURL := strings.TrimSpace(endpoint.ModelsEndpoint); modelsURL != "" {
		return fetchModelsFromURLs([]string{modelsURL}, apiKey, endpoint.Headers)
	}
	baseURL := strings.TrimSuffix(strings.TrimSpace(endpoint.BaseURL), "/")
	if baseURL == "" {
		return nil, errors.New("base_url is required")
	}
	return fetchModelsFromURLs(modelCandidateURLs(baseURL), apiKey, endpoint.Headers)
}

func fetchModelsFromURLs(urls []string, apiKey string, headers map[string]string) ([]string, error) {
	client := &http.Client{Timeout: 8 * time.Second}
	var attempts []string
	for _, url := range urls {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
			req.Header.Set("x-api-key", apiKey)
		}
		for name, value := range headers {
			if strings.TrimSpace(name) != "" {
				req.Header.Set(name, value)
			}
		}
		resp, err := client.Do(req)
		if err != nil {
			attempts = append(attempts, url+": "+err.Error())
			continue
		}
		var parsed struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
			Models []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"models"`
		}
		decodeErr := json.NewDecoder(resp.Body).Decode(&parsed)
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			attempts = append(attempts, fmt.Sprintf("%s: HTTP %d", url, resp.StatusCode))
			continue
		}
		if decodeErr != nil {
			attempts = append(attempts, url+": "+decodeErr.Error())
			continue
		}
		seen := map[string]bool{}
		models := make([]string, 0, len(parsed.Data)+len(parsed.Models))
		for _, m := range parsed.Data {
			id := strings.TrimSpace(m.ID)
			if id != "" && !seen[id] {
				seen[id] = true
				models = append(models, id)
			}
		}
		for _, m := range parsed.Models {
			id := strings.TrimSpace(m.ID)
			if id == "" {
				id = strings.TrimPrefix(strings.TrimSpace(m.Name), "models/")
			}
			if id != "" && !seen[id] {
				seen[id] = true
				models = append(models, id)
			}
		}
		if len(models) > 0 {
			return models, nil
		}
		attempts = append(attempts, url+": empty model list")
	}
	return nil, fmt.Errorf("model discovery failed: %s", strings.Join(attempts, "; "))
}

func modelCandidateURLs(baseURL string) []string {
	base := strings.TrimSuffix(baseURL, "/")
	withoutCompat := base
	for _, suffix := range []string{"/anthropic", "/openai", "/v1"} {
		if strings.HasSuffix(strings.ToLower(withoutCompat), suffix) {
			withoutCompat = strings.TrimSuffix(withoutCompat, withoutCompat[len(withoutCompat)-len(suffix):])
			break
		}
	}
	candidates := []string{base + "/models"}
	if !strings.HasSuffix(strings.ToLower(base), "/v1") {
		candidates = append(candidates, base+"/v1/models")
	}
	candidates = append(candidates, withoutCompat+"/v1/models", withoutCompat+"/models")
	seen := map[string]bool{}
	out := candidates[:0]
	for _, candidate := range candidates {
		if !seen[candidate] {
			seen[candidate] = true
			out = append(out, candidate)
		}
	}
	return out
}

func (s *Store) MergeDiscoveredModels(providerID string, modelIDs []string) ([]ProviderModel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pd, err := s.loadUnlocked()
	if err != nil {
		return nil, err
	}
	if providerByID(pd.Providers, providerID) == nil {
		return nil, fmt.Errorf("provider with id %q not found", providerID)
	}
	now := time.Now().Unix()
	seen := map[string]bool{}
	for _, id := range modelIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			seen[id] = true
		}
	}
	for i := range pd.Models {
		model := &pd.Models[i]
		if model.ProviderID != providerID || model.Source == "manual" {
			continue
		}
		model.Available = seen[model.ModelID]
		if model.Available {
			model.LastSeenAt = now
			delete(seen, model.ModelID)
		}
	}
	for id := range seen {
		pd.Models = append(pd.Models, ProviderModel{ProviderID: providerID, ModelID: id, Source: "remote", Available: true, DiscoveredAt: now, LastSeenAt: now})
	}
	if err := s.saveUnlocked(pd); err != nil {
		return nil, err
	}
	out := make([]ProviderModel, 0)
	for _, model := range pd.Models {
		if model.ProviderID == providerID {
			out = append(out, model)
		}
	}
	sortModels(out)
	return out, nil
}

func (s *Store) Models(providerID string) ([]ProviderModel, error) {
	pd, err := s.Load()
	if err != nil {
		return nil, err
	}
	out := make([]ProviderModel, 0)
	for _, model := range pd.Models {
		if providerID == "" || model.ProviderID == providerID {
			out = append(out, model)
		}
	}
	sortModels(out)
	return out, nil
}

func (s *Store) defaultPresets() *ProviderData {
	now := time.Now().Unix()
	return &ProviderData{
		SchemaVersion:    CurrentSchemaVersion,
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
				Status:           ProfileStatusActive,
				Endpoints:        []ProviderEndpoint{{Family: EndpointFamilyAnthropic, Protocol: "anthropic", BaseURL: "https://api.anthropic.com"}},
			},
			{
				ID:               "deepseek-api",
				Name:             "DeepSeek API",
				Protocol:         "dual",
				BaseURL:          "https://api.deepseek.com/v1",
				OpenAIBaseURL:    "https://api.deepseek.com/v1",
				AnthropicBaseURL: "https://api.deepseek.com/beta",
				APIKey:           "",
				Model:            "deepseek-chat",
				SonnetModel:      "deepseek-chat",
				HaikuModel:       "deepseek-chat",
				OpusModel:        "deepseek-reasoner",
				CreatedAt:        now,
				UpdatedAt:        now,
				Status:           ProfileStatusActive,
				Endpoints: []ProviderEndpoint{
					{Family: EndpointFamilyAnthropic, Protocol: "anthropic", BaseURL: "https://api.deepseek.com/beta"},
					{Family: EndpointFamilyOpenAI, Protocol: "openai_responses", BaseURL: "https://api.deepseek.com/v1"},
				},
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
				Status:        ProfileStatusActive,
				Endpoints:     []ProviderEndpoint{{Family: EndpointFamilyOpenAI, Protocol: "openai_responses", BaseURL: "https://openrouter.ai/api/v1"}},
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
