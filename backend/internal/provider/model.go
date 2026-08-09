package provider

import "encoding/json"

const CurrentSchemaVersion = 3

type AgentID string

const (
	AgentClaude   AgentID = "claude"
	AgentCodex    AgentID = "codex"
	AgentOpenClaw AgentID = "openclaw"
	AgentOpenCode AgentID = "opencode"
)

// ProviderEndpoint describes how one desktop agent reaches a provider. A
// provider may expose different protocols and URLs to different agents.
type ProviderEndpoint struct {
	AgentID        AgentID           `json:"agent_id"`
	Protocol       string            `json:"protocol"`
	BaseURL        string            `json:"base_url"`
	APIKey         string            `json:"api_key,omitempty"`
	HasAPIKey      bool              `json:"has_api_key,omitempty"`
	ModelsEndpoint string            `json:"models_endpoint,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	HasHeaders     bool              `json:"has_headers,omitempty"`
	HeaderNames    []string          `json:"header_names,omitempty"`
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
