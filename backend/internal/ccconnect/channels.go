package ccconnect

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/chenhg5/cc-connect/config"
	"github.com/chenhg5/cc-connect/core"
)

// channels.go implements the #277 Phase 2 channel↔agent binding API. A
// cc-connect project owns one work_dir (= one 1agents workspace) and one or more
// channels ([[projects.platforms]]). Each channel can bind its own agent type
// via [projects.platforms.agent]; when unset it inherits [projects.agent].
//
// Channels are addressed by their index in the project's Platforms slice — a
// stable selector within a project's config that survives across reads (the
// list is rewritten whole on save, preserving order). The project default agent
// is reported alongside so the frontend can show "inherited" bindings.

// ChannelBinding is one channel's current agent binding within a project.
type ChannelBinding struct {
	Index     int    `json:"index"`             // position in [[projects.platforms]]
	Type      string `json:"type"`              // platform type (bridge, feishu, telegram, …)
	Agent     string `json:"agent"`             // effective agent type for this channel
	Inherited bool   `json:"inherited"`         // true when Agent comes from the project default
	WorkDir   string `json:"workDir,omitempty"` // channel agent work_dir override, if any
}

// ProjectChannels is the read model for one project's channel bindings.
type ProjectChannels struct {
	Project      string           `json:"project"`
	WorkDir      string           `json:"workDir"`
	DefaultAgent string           `json:"defaultAgent"` // [projects.agent].type
	Channels     []ChannelBinding `json:"channels"`
}

// configFilePath returns the active cc-connect config path. Tests set
// config.ConfigPath to a temp file; runtime uses ~/.cc-connect/config.toml.
func configFilePath() string {
	if config.ConfigPath != "" {
		return config.ConfigPath
	}
	return ccConfigPath()
}

// loadConfigForChannels decodes the cc-connect config without validation, so the
// API works even when the live config is mid-edit or platform-less.
func loadConfigForChannels(path string) (*config.Config, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("cc-connect config not found")
	}
	cfg := &config.Config{}
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	return cfg, nil
}

// GetProjectChannels returns the channel↔agent bindings for one project.
func GetProjectChannels(projectName string) (*ProjectChannels, error) {
	cfg, err := loadConfigForChannels(configFilePath())
	if err != nil {
		return nil, err
	}
	for i := range cfg.Projects {
		p := cfg.Projects[i]
		if p.Name != projectName {
			continue
		}
		workDir, _ := p.Agent.Options["work_dir"].(string)
		out := &ProjectChannels{
			Project:      p.Name,
			WorkDir:      workDir,
			DefaultAgent: p.Agent.Type,
			Channels:     make([]ChannelBinding, 0, len(p.Platforms)),
		}
		for idx, pc := range p.Platforms {
			eff := config.ResolvePlatformAgent(p, pc)
			chWorkDir, _ := eff.Options["work_dir"].(string)
			out.Channels = append(out.Channels, ChannelBinding{
				Index:     idx,
				Type:      pc.Type,
				Agent:     eff.Type,
				Inherited: pc.Agent == nil,
				WorkDir:   chWorkDir,
			})
		}
		return out, nil
	}
	return nil, fmt.Errorf("project %q not found", projectName)
}

// SetChannelAgentBinding writes the agent override for a single channel back to
// the cc-connect config. An empty agentType clears the override so the channel
// re-inherits the project default. It does not validate the agent type against
// the installed CLI catalog — the engine logs and skips a bad channel agent on
// next boot (the same failure mode as a bad project agent).
func SetChannelAgentBinding(projectName string, channelIndex int, agentType string) error {
	path := configFilePath()
	cfg, err := loadConfigForChannels(path)
	if err != nil {
		return err
	}

	projIdx := -1
	for i := range cfg.Projects {
		if cfg.Projects[i].Name == projectName {
			projIdx = i
			break
		}
	}
	if projIdx < 0 {
		return fmt.Errorf("project %q not found", projectName)
	}
	proj := &cfg.Projects[projIdx]
	if channelIndex < 0 || channelIndex >= len(proj.Platforms) {
		return fmt.Errorf("channel index %d out of range (project has %d channels)", channelIndex, len(proj.Platforms))
	}

	pc := &proj.Platforms[channelIndex]
	agentType = strings.TrimSpace(agentType)
	if agentType == "" || strings.EqualFold(agentType, proj.Agent.Type) {
		// Clearing the override (or setting it to the project default) → drop the
		// per-channel agent so the channel inherits the project agent.
		pc.Agent = nil
	} else {
		// Bind a new agent type. Carry over the project's work_dir so the channel
		// agent runs in the same workspace directory as the project default.
		opts := map[string]any{}
		if wd, ok := proj.Agent.Options["work_dir"].(string); ok && wd != "" {
			opts["work_dir"] = wd
		}
		pc.Agent = &config.AgentConfig{Type: agentType, Options: opts}
	}

	if err := saveConfig(cfg, path); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	// Hot-reload the engines so the new binding takes effect without a manual
	// restart (runEngine re-reads the config and re-wires channel agents).
	select {
	case core.RestartCh <- core.RestartRequest{}:
	default:
		// A restart is already pending; the reload will pick up this write too.
	}
	return nil
}

// ChannelsHandler serves the #277 channel↔agent binding API:
//
//	GET  /api/cc-connect/channels?project=<name>   → ProjectChannels
//	POST /api/cc-connect/channels  {project,index,agent}  → set/clear binding
func ChannelsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		project := strings.TrimSpace(r.URL.Query().Get("project"))
		if project == "" {
			writeJSONError(w, http.StatusBadRequest, "project query parameter is required")
			return
		}
		pc, err := GetProjectChannels(project)
		if err != nil {
			writeJSONError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, pc)

	case http.MethodPost:
		var body struct {
			Project string `json:"project"`
			Index   *int   `json:"index"`
			Agent   string `json:"agent"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if strings.TrimSpace(body.Project) == "" || body.Index == nil {
			writeJSONError(w, http.StatusBadRequest, "project and index are required")
			return
		}
		if err := SetChannelAgentBinding(body.Project, *body.Index, body.Agent); err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "out of range") {
				writeJSONError(w, http.StatusNotFound, err.Error())
			} else {
				writeJSONError(w, http.StatusInternalServerError, err.Error())
			}
			return
		}
		// Return the refreshed binding set so the frontend can render the result
		// of the change directly.
		if pc, err := GetProjectChannels(body.Project); err == nil {
			writeJSON(w, http.StatusOK, pc)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
