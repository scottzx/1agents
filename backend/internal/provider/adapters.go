package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

func endpointFor(p Provider, agentID AgentID) ProviderEndpoint {
	for _, endpoint := range p.Endpoints {
		if endpoint.AgentID == agentID {
			return endpoint
		}
	}
	baseURL, protocol := p.BaseURL, p.Protocol
	if agentID == AgentClaude && p.AnthropicBaseURL != "" {
		baseURL = p.AnthropicBaseURL
	}
	if agentID != AgentClaude && p.OpenAIBaseURL != "" {
		baseURL = p.OpenAIBaseURL
	}
	return ProviderEndpoint{AgentID: agentID, Protocol: protocol, BaseURL: baseURL}
}

// EndpointForAgent returns the agent-specific endpoint, falling back to the
// provider's legacy flat fields for migrated configurations.
func EndpointForAgent(p Provider, agentID AgentID) ProviderEndpoint {
	return endpointFor(p, agentID)
}

func credentialFor(p Provider, endpoint ProviderEndpoint) string {
	if endpoint.APIKey != "" {
		return endpoint.APIKey
	}
	return p.APIKey
}

func effectiveModel(binding AgentBinding, role string) string {
	if value := binding.ModelMapping[role]; value != "" {
		return value
	}
	return binding.ModelID
}

func ApplyAgentBinding(home string, p Provider, binding AgentBinding) (ApplyResult, error) {
	plan, err := PlanAgentBinding(home, p, binding)
	if err != nil {
		return ApplyResult{AgentID: binding.AgentID}, err
	}
	if len(plan.Changes) == 0 {
		return ApplyResult{AgentID: binding.AgentID, Success: true}, nil
	}
	result := ApplyResult{AgentID: binding.AgentID}
	for _, change := range plan.Changes {
		backup, err := atomicWriteWithBackup(change.Path, []byte(change.After))
		if err != nil {
			_ = RollbackApply(result)
			return result, err
		}
		result.Files = append(result.Files, AppliedFile{ConfigPath: change.Path, BackupPath: backup})
	}
	result.Success = true
	return result, nil
}

func RollbackApply(result ApplyResult) error {
	for i := len(result.Files) - 1; i >= 0; i-- {
		file := result.Files[i]
		if file.BackupPath == "" {
			if err := os.Remove(file.ConfigPath); err != nil && !os.IsNotExist(err) {
				return err
			}
			continue
		}
		data, err := os.ReadFile(file.BackupPath)
		if err != nil {
			return err
		}
		if _, err := atomicWriteWithBackup(file.ConfigPath, data); err != nil {
			return err
		}
	}
	return nil
}

func PlanAgentBinding(home string, p Provider, binding AgentBinding) (ChangePlan, error) {
	var path string
	var after []byte
	var err error
	switch binding.AgentID {
	case AgentClaude:
		path = filepath.Join(home, ".claude", "settings.json")
		after, err = renderClaude(path, p, binding)
	case AgentCodex:
		path = filepath.Join(home, ".codex", "config.toml")
		after, err = renderCodex(path, p, binding)
	default:
		err = fmt.Errorf("unsupported agent %q", binding.AgentID)
	}
	if err != nil {
		return ChangePlan{AgentID: binding.AgentID}, err
	}
	before, readErr := os.ReadFile(path)
	if readErr != nil && !os.IsNotExist(readErr) {
		return ChangePlan{AgentID: binding.AgentID}, readErr
	}
	plan := ChangePlan{AgentID: binding.AgentID}
	if !bytes.Equal(before, after) {
		plan.Changes = append(plan.Changes, ConfigChange{Path: path, Before: string(before), After: string(after)})
	}
	if binding.AgentID == AgentCodex && credentialFor(p, endpointFor(p, AgentCodex)) != "" {
		authPath := filepath.Join(home, ".codex", "auth.json")
		authAfter, authErr := renderCodexAuth(authPath, credentialFor(p, endpointFor(p, AgentCodex)))
		if authErr != nil {
			return plan, authErr
		}
		authBefore, readErr := os.ReadFile(authPath)
		if readErr != nil && !os.IsNotExist(readErr) {
			return plan, readErr
		}
		if !bytes.Equal(authBefore, authAfter) {
			plan.Changes = append(plan.Changes, ConfigChange{Path: authPath, Before: string(authBefore), After: string(authAfter)})
		}
	}
	return plan, nil
}

func readJSONObject(path string) (map[string]any, error) {
	root := map[string]any{}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return root, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return root, nil
}

func renderClaude(path string, p Provider, binding AgentBinding) ([]byte, error) {
	root, err := readJSONObject(path)
	if err != nil {
		return nil, err
	}
	env, _ := root["env"].(map[string]any)
	if env == nil {
		env = map[string]any{}
	}
	endpoint := endpointFor(p, AgentClaude)
	setOrDelete(env, "ANTHROPIC_BASE_URL", endpoint.BaseURL)
	setOrDelete(env, "ANTHROPIC_AUTH_TOKEN", credentialFor(p, endpoint))
	setOrDelete(env, "ANTHROPIC_MODEL", binding.ModelID)
	setOrDelete(env, "ANTHROPIC_DEFAULT_HAIKU_MODEL", effectiveModel(binding, "haiku"))
	setOrDelete(env, "ANTHROPIC_DEFAULT_SONNET_MODEL", effectiveModel(binding, "sonnet"))
	setOrDelete(env, "ANTHROPIC_DEFAULT_OPUS_MODEL", effectiveModel(binding, "opus"))
	if optionBool(binding.Options, "thinking_enabled") {
		budget := optionInt(binding.Options, "thinking_budget_tokens", 10000)
		env["MAX_THINKING_TOKENS"] = fmt.Sprintf("%d", budget)
	} else {
		delete(env, "MAX_THINKING_TOKENS")
	}
	root["env"] = env
	return json.MarshalIndent(root, "", "  ")
}

func optionBool(options map[string]json.RawMessage, key string) bool {
	var value bool
	return json.Unmarshal(options[key], &value) == nil && value
}

func optionInt(options map[string]json.RawMessage, key string, fallback int) int {
	var value int
	if json.Unmarshal(options[key], &value) == nil && value > 0 {
		return value
	}
	return fallback
}

func renderCodex(path string, p Provider, binding AgentBinding) ([]byte, error) {
	root := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		if _, err := toml.Decode(string(data), &root); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	endpoint := endpointFor(p, AgentCodex)
	providerID := "1agents_" + sanitizeID(p.ID)
	root["model"] = binding.ModelID
	root["model_provider"] = providerID
	providers, _ := root["model_providers"].(map[string]any)
	if providers == nil {
		providers = map[string]any{}
	}
	entry, _ := providers[providerID].(map[string]any)
	if entry == nil {
		entry = map[string]any{}
	}
	entry["name"] = p.Name
	entry["base_url"] = endpoint.BaseURL
	entry["env_key"] = "OPENAI_API_KEY"
	if strings.Contains(endpoint.Protocol, "chat") {
		entry["wire_api"] = "chat"
	} else {
		entry["wire_api"] = "responses"
	}
	providers[providerID] = entry
	root["model_providers"] = providers
	if raw := binding.Options["reasoning_effort"]; len(raw) > 0 {
		var effort string
		if json.Unmarshal(raw, &effort) == nil && effort != "" {
			root["model_reasoning_effort"] = effort
		}
	}
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(root); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func renderCodexAuth(path, apiKey string) ([]byte, error) {
	root, err := readJSONObject(path)
	if err != nil {
		return nil, err
	}
	root["OPENAI_API_KEY"] = apiKey
	return json.MarshalIndent(root, "", "  ")
}

func setOrDelete(values map[string]any, key, value string) {
	if value == "" {
		delete(values, key)
	} else {
		values[key] = value
	}
}

func atomicWriteWithBackup(path string, data []byte) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	backup := ""
	if existing, err := os.ReadFile(path); err == nil {
		backup = fmt.Sprintf("%s.1agents-backup-%d", path, time.Now().UnixNano())
		if err := os.WriteFile(backup, existing, 0o600); err != nil {
			return "", fmt.Errorf("backup %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".1agents-config-*")
	if err != nil {
		return backup, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return backup, err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return backup, err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return backup, err
	}
	if err := tmp.Close(); err != nil {
		return backup, err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return backup, err
	}
	return backup, nil
}

func ReadAgentRuntime(home string, agentID AgentID) (AgentRuntimeStatus, error) {
	status := AgentRuntimeStatus{AgentID: agentID, Options: map[string]any{}}
	switch agentID {
	case AgentClaude:
		status.ConfigPath = filepath.Join(home, ".claude", "settings.json")
		root, err := readJSONObject(status.ConfigPath)
		if err != nil {
			return status, err
		}
		env, _ := root["env"].(map[string]any)
		status.BaseURL, _ = env["ANTHROPIC_BASE_URL"].(string)
		status.ModelID, _ = env["ANTHROPIC_MODEL"].(string)
	case AgentCodex:
		status.ConfigPath = filepath.Join(home, ".codex", "config.toml")
		root := map[string]any{}
		if data, err := os.ReadFile(status.ConfigPath); err == nil {
			if _, err := toml.Decode(string(data), &root); err != nil {
				return status, err
			}
		}
		status.ModelID, _ = root["model"].(string)
		status.Options["model_provider"] = root["model_provider"]
	default:
		return status, fmt.Errorf("unsupported agent %q", agentID)
	}
	_, err := os.Stat(status.ConfigPath)
	status.Installed = err == nil
	return status, nil
}

func sortModels(models []ProviderModel) {
	sort.Slice(models, func(i, j int) bool { return models[i].ModelID < models[j].ModelID })
}
