package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	GrokBuildRuntimeID       = "grok-build"
	DeepSeekBuildProfileID   = "deepseek-build"
	DeepSeekDefaultModelID   = "deepseek-v4-flash"
	LegacyDeepSeekProviderID = "deepseek-api"
)

func normalizeProfiles(pd *ProviderData, now int64) {
	for i := range pd.Profiles {
		profile := &pd.Profiles[i]
		profile.ID = sanitizeID(profile.ID)
		if profile.Name == "" {
			profile.Name = profile.ID
		}
		if profile.Revision < 1 {
			profile.Revision = 1
		}
		if profile.Status == "" {
			profile.Status = ProfileStatusActive
		}
		if profile.CreatedAt == 0 {
			profile.CreatedAt = now
		}
		if profile.UpdatedAt == 0 {
			profile.UpdatedAt = profile.CreatedAt
		}
	}
	if profileByID(pd.Profiles, DeepSeekBuildProfileID) == nil {
		pd.Profiles = append(pd.Profiles, seedDeepSeekProfile(pd, now))
	}
	if pd.DefaultProfileID == "" {
		if profile := profileByID(pd.Profiles, DeepSeekBuildProfileID); profile != nil && profile.Status == ProfileStatusActive {
			pd.DefaultProfileID = profile.ID
		}
	}
}

func seedDeepSeekProfile(pd *ProviderData, now int64) AgentProfile {
	profile := AgentProfile{
		ID:        DeepSeekBuildProfileID,
		Name:      "DeepSeek Build",
		RuntimeID: GrokBuildRuntimeID,
		ModelID:   DeepSeekDefaultModelID,
		Revision:  1,
		Status:    ProfileStatusDisabled,
		System:    true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	candidates := make([]*Provider, 0, 1)
	if exact := providerByID(pd.Providers, LegacyDeepSeekProviderID); exact != nil && exact.Status != ProfileStatusArchived && providerSupportsFamily(*exact, EndpointFamilyOpenAI) {
		candidates = append(candidates, exact)
	} else {
		for i := range pd.Providers {
			candidate := &pd.Providers[i]
			name := strings.ToLower(candidate.ID + " " + candidate.Name)
			if candidate.Status != ProfileStatusArchived && strings.Contains(name, "deepseek") && providerSupportsFamily(*candidate, EndpointFamilyOpenAI) {
				candidates = append(candidates, candidate)
			}
		}
	}
	if len(candidates) != 1 {
		return profile
	}
	profile.ProviderID = candidates[0].ID
	if !providerHasModel(*pd, profile.ProviderID, profile.ModelID) {
		models := uniqueAvailableModels(*pd, *candidates[0])
		if len(models) == 1 {
			profile.ModelID = models[0]
		} else {
			profile.ModelID = ""
		}
	}
	if profile.ModelID != "" {
		profile.Status = ProfileStatusActive
	}
	return profile
}

func uniqueAvailableModels(pd ProviderData, p Provider) []string {
	seen := map[string]bool{}
	out := []string{}
	appendModel := func(id string) {
		id = strings.TrimSpace(id)
		if id != "" && !seen[id] {
			for _, model := range pd.Models {
				if model.ProviderID == p.ID && model.ModelID == id && !model.Available {
					return
				}
			}
			seen[id] = true
			out = append(out, id)
		}
	}
	appendModel(p.Model)
	for _, id := range p.ModelIDs {
		appendModel(id)
	}
	for _, model := range pd.Models {
		if model.ProviderID == p.ID && model.Available {
			appendModel(model.ModelID)
		}
	}
	return out
}

func profileByID(profiles []AgentProfile, id string) *AgentProfile {
	for i := range profiles {
		if profiles[i].ID == id {
			return &profiles[i]
		}
	}
	return nil
}

func providerSupportsFamily(p Provider, family EndpointFamily) bool {
	return endpointForFamily(p, family) != nil
}

func endpointForFamily(p Provider, family EndpointFamily) *ProviderEndpoint {
	for i := range p.Endpoints {
		if p.Endpoints[i].Family == family {
			return &p.Endpoints[i]
		}
	}
	return nil
}

func providerHasModel(pd ProviderData, providerID, modelID string) bool {
	for _, model := range pd.Models {
		if model.ProviderID == providerID && model.ModelID == modelID {
			return model.Available
		}
	}
	provider := providerByID(pd.Providers, providerID)
	if provider == nil {
		return false
	}
	if provider.Model == modelID {
		return true
	}
	for _, candidate := range provider.ModelIDs {
		if candidate == modelID {
			return true
		}
	}
	return false
}

func validateProfile(pd *ProviderData, profile AgentProfile) error {
	if profile.ID == "" || profile.Name == "" {
		return errors.New("profile id and name are required")
	}
	if profile.RuntimeID != GrokBuildRuntimeID {
		return fmt.Errorf("runtime %q is not supported by provider profiles", profile.RuntimeID)
	}
	if profile.Status != ProfileStatusActive && profile.Status != ProfileStatusDisabled && profile.Status != ProfileStatusArchived {
		return fmt.Errorf("invalid profile status %q", profile.Status)
	}
	if profile.Status != ProfileStatusActive {
		return nil
	}
	provider := providerByID(pd.Providers, profile.ProviderID)
	if provider == nil {
		return fmt.Errorf("provider with id %q not found", profile.ProviderID)
	}
	if provider.Status == ProfileStatusArchived {
		return fmt.Errorf("provider %q is archived", profile.ProviderID)
	}
	if endpointForFamily(*provider, EndpointFamilyOpenAI) == nil {
		return fmt.Errorf("provider %q has no openai endpoint", profile.ProviderID)
	}
	if strings.TrimSpace(profile.ModelID) == "" {
		return errors.New("profile model_id is required")
	}
	for _, model := range pd.Models {
		if model.ProviderID == profile.ProviderID && model.ModelID == profile.ModelID && !model.Available {
			return fmt.Errorf("model %q is unavailable", profile.ModelID)
		}
	}
	if !providerHasModel(*pd, profile.ProviderID, profile.ModelID) {
		return fmt.Errorf("model %q is not registered for provider %q", profile.ModelID, profile.ProviderID)
	}
	return nil
}

func (s *Store) ListProfiles(includeArchived bool) ([]AgentProfile, error) {
	pd, err := s.Load()
	if err != nil {
		return nil, err
	}
	out := make([]AgentProfile, 0, len(pd.Profiles))
	for _, profile := range pd.Profiles {
		if !includeArchived && profile.Status == ProfileStatusArchived {
			continue
		}
		out = append(out, profile)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s *Store) GetProfile(id string) (*AgentProfile, error) {
	pd, err := s.Load()
	if err != nil {
		return nil, err
	}
	profile := profileByID(pd.Profiles, sanitizeID(id))
	if profile == nil {
		return nil, fmt.Errorf("profile with id %q not found", id)
	}
	copy := *profile
	return &copy, nil
}

func (s *Store) AddProfile(profile AgentProfile) (*AgentProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pd, err := s.loadUnlocked()
	if err != nil {
		return nil, err
	}
	profile.ID = sanitizeID(profile.ID)
	if profile.ID == "" {
		profile.ID = sanitizeID(profile.Name)
	}
	if profileByID(pd.Profiles, profile.ID) != nil {
		return nil, fmt.Errorf("profile with id %q already exists", profile.ID)
	}
	if profile.Status == "" {
		profile.Status = ProfileStatusActive
	}
	if err := validateProfile(pd, profile); err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	profile.Revision = 1
	profile.CreatedAt = now
	profile.UpdatedAt = now
	pd.Profiles = append(pd.Profiles, profile)
	if err := s.saveUnlocked(pd); err != nil {
		return nil, err
	}
	return &profile, nil
}

func (s *Store) UpdateProfile(id string, next AgentProfile) (*AgentProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pd, err := s.loadUnlocked()
	if err != nil {
		return nil, err
	}
	current := profileByID(pd.Profiles, sanitizeID(id))
	if current == nil {
		return nil, fmt.Errorf("profile with id %q not found", id)
	}
	next.ID = current.ID
	next.System = current.System
	next.CreatedAt = current.CreatedAt
	next.Revision = current.Revision + 1
	next.UpdatedAt = time.Now().Unix()
	if next.Status == "" {
		next.Status = current.Status
	}
	if err := validateProfile(pd, next); err != nil {
		return nil, err
	}
	*current = next
	if err := s.saveUnlocked(pd); err != nil {
		return nil, err
	}
	return &next, nil
}

func (s *Store) SetProfileStatus(id, status string) (*AgentProfile, error) {
	profile, err := s.GetProfile(id)
	if err != nil {
		return nil, err
	}
	profile.Status = status
	return s.UpdateProfile(id, *profile)
}

func (s *Store) ResolveProfile(id string) (*ProfileLaunchSpec, error) {
	pd, err := s.Load()
	if err != nil {
		return nil, err
	}
	profile := profileByID(pd.Profiles, sanitizeID(id))
	if profile == nil {
		return nil, fmt.Errorf("profile with id %q not found", id)
	}
	if err := validateProfile(pd, *profile); err != nil {
		return nil, err
	}
	if profile.Status != ProfileStatusActive {
		return nil, fmt.Errorf("profile %q is %s", profile.ID, profile.Status)
	}
	provider := providerByID(pd.Providers, profile.ProviderID)
	endpoint := endpointForFamily(*provider, EndpointFamilyOpenAI)
	if parsed, parseErr := url.Parse(endpoint.BaseURL); parseErr != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("provider %q has invalid openai base URL", provider.ID)
	}
	credential := CredentialForEndpoint(*provider, endpoint.BaseURL)
	if credential == "" {
		return nil, fmt.Errorf("provider %q has no credential", provider.ID)
	}
	modelsEndpoint := endpoint.ModelsEndpoint
	if modelsEndpoint == "" {
		modelsEndpoint = strings.TrimSuffix(endpoint.BaseURL, "/") + "/models"
	}
	if parsed, parseErr := url.Parse(modelsEndpoint); parseErr != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("provider %q has invalid models endpoint", provider.ID)
	}
	options := cloneRawOptions(profile.Options)
	snapshot := ResolvedProfileSnapshot{
		ProfileID:       profile.ID,
		ProfileName:     profile.Name,
		ProfileRevision: profile.Revision,
		RuntimeID:       profile.RuntimeID,
		ProviderID:      provider.ID,
		ProviderName:    provider.Name,
		ModelID:         profile.ModelID,
		EndpointFamily:  EndpointFamilyOpenAI,
		Protocol:        endpoint.Protocol,
		BaseURL:         endpoint.BaseURL,
		ModelsEndpoint:  modelsEndpoint,
		Options:         options,
		ResolvedAt:      time.Now().Unix(),
	}
	return &ProfileLaunchSpec{
		Snapshot: snapshot,
		Argv:     []string{"grok", "agent", "--model", profile.ModelID, "stdio"},
		Model:    profile.ModelID,
		Env: map[string]string{
			"GROK_XAI_API_BASE_URL": endpoint.BaseURL,
			"GROK_MODELS_BASE_URL":  endpoint.BaseURL,
			"GROK_MODELS_LIST_URL":  modelsEndpoint,
			"GROK_DEFAULT_MODEL":    profile.ModelID,
		},
		TransientEnv: map[string]string{"XAI_API_KEY": credential},
		Credentials:  map[string]string{"xai.api_key": credential},
	}, nil
}

// DefaultProfileID returns the system-level assignment default. It is a
// reference only; callers still validate the referenced profile at job create
// time and again when a run starts.
func (s *Store) DefaultProfileID() (string, error) {
	pd, err := s.Load()
	if err != nil {
		return "", err
	}
	return pd.DefaultProfileID, nil
}

func cloneRawOptions(values map[string]json.RawMessage) map[string]json.RawMessage {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]json.RawMessage, len(values))
	for key, value := range values {
		out[key] = append(json.RawMessage(nil), value...)
	}
	return out
}
