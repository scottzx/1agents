package agent

import "encoding/json"

// injectSessionAttribution adds the host-owned Session identity to each
// project-items MCP subprocess. Existing server fields and unrelated MCP
// entries are preserved.
func injectSessionAttribution(raw json.RawMessage, sessionID, sessionToken string) json.RawMessage {
	if len(raw) == 0 || sessionID == "" || sessionToken == "" {
		return raw
	}
	var servers []map[string]any
	if err := json.Unmarshal(raw, &servers); err != nil {
		return raw
	}
	changed := false
	for _, server := range servers {
		name, _ := server["name"].(string)
		if name != "project_items" && name != "project-items" && name != "mcp-tasks" {
			continue
		}
		env, _ := server["env"].([]any)
		env = append(env,
			map[string]any{"name": "ONEAGENTS_SESSION_ID", "value": sessionID},
			map[string]any{"name": "ONEAGENTS_SESSION_TOKEN", "value": sessionToken},
		)
		server["env"] = env
		changed = true
	}
	if !changed {
		return raw
	}
	updated, err := json.Marshal(servers)
	if err != nil {
		return raw
	}
	return updated
}
