package provider

import (
	"encoding/json"
	"os/exec"
)

const CurrentSchemaVersion = 4

type EndpointFamily string

const (
	EndpointFamilyOpenAI    EndpointFamily = "openai"
	EndpointFamilyAnthropic EndpointFamily = "anthropic"
)

type AgentID string

const (
	AgentClaude   AgentID = "claude"
	AgentCodex    AgentID = "codex"
	AgentOpenClaw AgentID = "openclaw"
	AgentOpenCode AgentID = "opencode"
)

// ProviderEndpoint describes one protocol-family endpoint exposed by a
// provider. AgentID is retained only so schema v3 files can be migrated.
type ProviderEndpoint struct {
	Family         EndpointFamily    `json:"family,omitempty"`
	AgentID        AgentID           `json:"agent_id,omitempty"`
	Protocol       string            `json:"protocol"`
	BaseURL        string            `json:"base_url"`
	APIKey         string            `json:"api_key,omitempty"`
	HasAPIKey      bool              `json:"has_api_key,omitempty"`
	ModelsEndpoint string            `json:"models_endpoint,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	HasHeaders     bool              `json:"has_headers,omitempty"`
	HeaderNames    []string          `json:"header_names,omitempty"`
}

const (
	ProfileStatusActive   = "active"
	ProfileStatusDisabled = "disabled"
	ProfileStatusArchived = "archived"
)

// AgentProfile is a stable, schedulable combination of runtime, provider,
// model and public runtime options. Secrets remain provider-owned.
type AgentProfile struct {
	ID         string                     `json:"id"`
	Name       string                     `json:"name"`
	RuntimeID  string                     `json:"runtime_id"`
	ProviderID string                     `json:"provider_id,omitempty"`
	ModelID    string                     `json:"model_id,omitempty"`
	Options    map[string]json.RawMessage `json:"options,omitempty"`
	Revision   int                        `json:"revision"`
	Status     string                     `json:"status"`
	System     bool                       `json:"system,omitempty"`
	CreatedAt  int64                      `json:"created_at"`
	UpdatedAt  int64                      `json:"updated_at"`
}

type RuntimeDefinition struct {
	ID                        string                  `json:"id"`
	Label                     string                  `json:"label"`
	SupportedEndpointFamilies []EndpointFamily        `json:"supported_endpoint_families"`
	OptionSchema              []AgentOptionDefinition `json:"option_schema,omitempty"`
	Installed                 bool                    `json:"installed"`
	UnavailableReason         string                  `json:"unavailable_reason,omitempty"`
}

func RuntimeDefinitions() []RuntimeDefinition {
	_, err := exec.LookPath("grok")
	runtime := RuntimeDefinition{
		ID:                        "grok-build",
		Label:                     "Grok Build",
		SupportedEndpointFamilies: []EndpointFamily{EndpointFamilyOpenAI},
		Installed:                 err == nil,
	}
	if err != nil {
		runtime.UnavailableReason = "grok runtime is not installed"
	}
	return []RuntimeDefinition{runtime}
}

// ResolvedProfileSnapshot is safe to persist with tasks, turns and sessions.
// It deliberately contains no API keys or custom header values.
type ResolvedProfileSnapshot struct {
	ProfileID       string                     `json:"profile_id"`
	ProfileName     string                     `json:"profile_name"`
	ProfileRevision int                        `json:"profile_revision"`
	RuntimeID       string                     `json:"runtime_id"`
	ProviderID      string                     `json:"provider_id"`
	ProviderName    string                     `json:"provider_name"`
	ModelID         string                     `json:"model_id"`
	EndpointFamily  EndpointFamily             `json:"endpoint_family"`
	Protocol        string                     `json:"protocol"`
	BaseURL         string                     `json:"base_url"`
	ModelsEndpoint  string                     `json:"models_endpoint,omitempty"`
	Options         map[string]json.RawMessage `json:"options,omitempty"`
	ResolvedAt      int64                      `json:"resolved_at"`
}

// ProfileLaunchSpec is produced by a code-owned runtime adapter. Credentials
// are transient and intentionally excluded from JSON serialization.
type ProfileLaunchSpec struct {
	Snapshot     ResolvedProfileSnapshot `json:"snapshot"`
	Argv         []string                `json:"argv"`
	Model        string                  `json:"model"`
	Env          map[string]string       `json:"env,omitempty"`
	TransientEnv map[string]string       `json:"-"`
	Credentials  map[string]string       `json:"-"`
}

type ModelCapabilities struct {
	Reasoning bool `json:"reasoning,omitempty"`
	Vision    bool `json:"vision,omitempty"`
}

// ProviderModel is a catalog entry. Missing remote models are marked
// unavailable rather than deleted so saved bindings remain explainable.
type ProviderModel struct {
	ProviderID   string            `json:"provider_id"`
	ModelID      string            `json:"model_id"`
	DisplayName  string            `json:"display_name,omitempty"`
	Source       string            `json:"source"` // preset, remote, manual
	Capabilities ModelCapabilities `json:"capabilities,omitempty"`
	Available    bool              `json:"available"`
	DiscoveredAt int64             `json:"discovered_at,omitempty"`
	LastSeenAt   int64             `json:"last_seen_at,omitempty"`
}

// AgentBinding is the desired provider configuration for one desktop agent.
// Options are agent-owned and interpreted only by that agent's adapter.
type AgentBinding struct {
	AgentID      AgentID                    `json:"agent_id"`
	ProviderID   string                     `json:"provider_id"`
	ModelID      string                     `json:"model_id,omitempty"`
	ModelMapping map[string]string          `json:"model_mapping,omitempty"`
	Options      map[string]json.RawMessage `json:"options,omitempty"`
	UpdatedAt    int64                      `json:"updated_at"`
}

type AgentRuntimeStatus struct {
	AgentID       AgentID        `json:"agent_id"`
	Installed     bool           `json:"installed"`
	ConfigPath    string         `json:"config_path"`
	ProviderMatch string         `json:"provider_match,omitempty"`
	BaseURL       string         `json:"base_url,omitempty"`
	ModelID       string         `json:"model_id,omitempty"`
	Options       map[string]any `json:"options,omitempty"`
	Warnings      []string       `json:"warnings,omitempty"`
}

// AgentOptionDefinition describes a user-selectable adapter option exposed by
// the API. Unknown options remain ignored by adapters for forward compatibility.
type AgentOptionDefinition struct {
	Key       string   `json:"key"`
	Type      string   `json:"type"`
	Label     string   `json:"label"`
	Default   any      `json:"default,omitempty"`
	Choices   []string `json:"choices,omitempty"`
	Minimum   int      `json:"minimum,omitempty"`
	DependsOn string   `json:"depends_on,omitempty"`
}

type AgentOptionSchema struct {
	AgentID AgentID                 `json:"agent_id"`
	Options []AgentOptionDefinition `json:"options"`
}

func AgentOptionSchemas() []AgentOptionSchema {
	return []AgentOptionSchema{
		{
			AgentID: AgentClaude,
			Options: []AgentOptionDefinition{
				{Key: "thinking_enabled", Type: "boolean", Label: "Thinking mode", Default: false},
				{Key: "thinking_budget_tokens", Type: "integer", Label: "Thinking token budget", Default: 10000, Minimum: 1, DependsOn: "thinking_enabled"},
			},
		},
		{
			AgentID: AgentCodex,
			Options: []AgentOptionDefinition{
				{Key: "reasoning_effort", Type: "select", Label: "Reasoning effort", Default: "medium", Choices: []string{"low", "medium", "high", "xhigh"}},
			},
		},
	}
}

type ConfigChange struct {
	Path   string `json:"path"`
	Before string `json:"before,omitempty"`
	After  string `json:"after"`
}

type ChangePlan struct {
	AgentID AgentID        `json:"agent_id"`
	Changes []ConfigChange `json:"changes"`
}

type ApplyResult struct {
	AgentID AgentID       `json:"agent_id"`
	Files   []AppliedFile `json:"files,omitempty"`
	Success bool          `json:"success"`
}

type AppliedFile struct {
	ConfigPath string `json:"config_path"`
	BackupPath string `json:"backup_path,omitempty"`
}
