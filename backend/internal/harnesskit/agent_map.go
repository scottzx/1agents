package harnesskit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const agentMapEnv = "ONEAGENTS_AGENT_EXTENSION_MAP"

type AgentMapping struct {
	OneAgents  string  `json:"oneAgents"`
	HarnessKit *string `json:"harnessKit"`
	Deployment bool    `json:"deployment"`
	Reason     string  `json:"reason,omitempty"`
}

type agentMapFile struct {
	Version  int            `json:"version"`
	Mappings []AgentMapping `json:"mappings"`
}

// ResolveAgentMapping consumes config/agent-extension-map.json as the single
// source of truth. Unsupported agents return their product-facing reason.
func ResolveAgentMapping(oneAgentsName string) (string, error) {
	raw, source, err := readAgentMap()
	if err != nil {
		return "", err
	}
	var manifest agentMapFile
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return "", fmt.Errorf("parse agent extension map %s: %w", source, err)
	}
	name := strings.TrimSpace(oneAgentsName)
	for _, mapping := range manifest.Mappings {
		if mapping.OneAgents != name {
			continue
		}
		if mapping.Deployment && mapping.HarnessKit != nil && strings.TrimSpace(*mapping.HarnessKit) != "" {
			return strings.TrimSpace(*mapping.HarnessKit), nil
		}
		reason := strings.TrimSpace(mapping.Reason)
		if reason == "" {
			reason = "This Agent has no verified HarnessKit project extension contract."
		}
		return "", fmt.Errorf("%s: %s", name, reason)
	}
	return "", fmt.Errorf("%s: Agent is not present in config/agent-extension-map.json", name)
}

func readAgentMap() ([]byte, string, error) {
	candidates := make([]string, 0, 6)
	if explicit := strings.TrimSpace(os.Getenv(agentMapEnv)); explicit != "" {
		candidates = append(candidates, explicit)
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(cwd, "config", "agent-extension-map.json"),
			filepath.Join(cwd, "..", "config", "agent-extension-map.json"),
		)
	}
	if executable, err := os.Executable(); err == nil {
		dir := filepath.Dir(executable)
		candidates = append(candidates,
			filepath.Join(dir, "config", "agent-extension-map.json"),
			filepath.Join(dir, "..", "config", "agent-extension-map.json"),
			filepath.Join(dir, "..", "Resources", "config", "agent-extension-map.json"),
		)
	}
	for _, candidate := range candidates {
		raw, err := os.ReadFile(candidate)
		if err == nil {
			return raw, candidate, nil
		}
	}
	return nil, "", fmt.Errorf("agent extension map not found; set %s or package config/agent-extension-map.json", agentMapEnv)
}
