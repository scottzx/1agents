package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/scottzx/1Agents/backend/internal/provider"
)

var defaultProviderStore = provider.NewStore("")

var persistAgentBinding = func(binding provider.AgentBinding) error {
	return defaultProviderStore.SetBinding(binding)
}

func jsonError(msg string) string {
	bytes, _ := json.Marshal(map[string]string{"error": msg})
	return string(bytes)
}

// handleProviders handles GET (list), POST (add/update), DELETE (remove) for /api/providers.
func handleProviders(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		providers, activeID, err := defaultProviderStore.List()
		if err != nil {
			http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"providers":          provider.RedactedProviders(providers),
			"active_provider_id": activeID,
			"bindings":           func() []provider.AgentBinding { bindings, _ := defaultProviderStore.ListBindings(); return bindings }(),
		})

	case http.MethodPost:
		var p provider.Provider
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, jsonError("invalid provider json"), http.StatusBadRequest)
			return
		}
		saved, err := defaultProviderStore.AddOrUpdate(p)
		if err != nil {
			http.Error(w, jsonError(err.Error()), http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(provider.RedactedProvider(*saved))

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, jsonError("missing provider id parameter"), http.StatusBadRequest)
			return
		}
		if err := defaultProviderStore.Delete(id); err != nil {
			http.Error(w, jsonError(err.Error()), http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"success": true})

	default:
		http.Error(w, jsonError("method not allowed"), http.StatusMethodNotAllowed)
	}
}

func handleProviderModels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		http.Error(w, jsonError("method not allowed"), http.StatusMethodNotAllowed)
		return
	}
	models, err := defaultProviderStore.Models(r.URL.Query().Get("provider_id"))
	if err != nil {
		http.Error(w, jsonError(err.Error()), http.StatusBadRequest)
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"models": models})
}

// handleProviderSwitch preserves the legacy provider selection marker. It no
// longer mutates desktop agent files; callers should apply an AgentBinding.
func handleProviderSwitch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, jsonError("method not allowed"), http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.ID) == "" {
		http.Error(w, jsonError("invalid provider id"), http.StatusBadRequest)
		return
	}

	active, err := defaultProviderStore.SetActive(req.ID)
	if err != nil {
		http.Error(w, jsonError(err.Error()), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"success":            true,
		"active_provider_id": active.ID,
		"provider":           provider.RedactedProvider(*active),
	})
}

// handleFetchModels handles POST /api/providers/fetch-models to dynamically query available model IDs.
func handleFetchModels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		http.Error(w, jsonError("method not allowed"), http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		BaseURL string `json:"base_url"`
		APIKey  string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, jsonError("invalid json payload"), http.StatusBadRequest)
		return
	}

	models, err := provider.FetchModelsFromEndpoint(req.BaseURL, req.APIKey)
	if err != nil {
		http.Error(w, jsonError(err.Error()), http.StatusBadRequest)
		return
	}

	json.NewEncoder(w).Encode(map[string]any{
		"models": models,
	})
}

func handleDiscoverProviderModels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		http.Error(w, jsonError("method not allowed"), http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ProviderID string `json:"provider_id"`
		AgentID    string `json:"agent_id"`
		BaseURL    string `json:"base_url"`
		APIKey     string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ProviderID == "" {
		http.Error(w, jsonError("provider_id is required"), http.StatusBadRequest)
		return
	}
	p, err := defaultProviderStore.Get(req.ProviderID)
	if err != nil {
		http.Error(w, jsonError(err.Error()), http.StatusNotFound)
		return
	}
	endpoint := provider.EndpointForAgent(*p, provider.AgentID(req.AgentID))
	if req.BaseURL != "" {
		endpoint.BaseURL = req.BaseURL
	} else {
		req.BaseURL = endpoint.BaseURL
	}
	if req.APIKey == "" {
		if endpoint.APIKey != "" {
			req.APIKey = endpoint.APIKey
		} else {
			req.APIKey = provider.CredentialForEndpoint(*p, req.BaseURL)
		}
	}
	ids, err := provider.FetchModelsForEndpoint(endpoint, req.APIKey)
	if err != nil {
		http.Error(w, jsonError(err.Error()), http.StatusBadRequest)
		return
	}
	models, err := defaultProviderStore.MergeDiscoveredModels(req.ProviderID, ids)
	if err != nil {
		http.Error(w, jsonError(err.Error()), http.StatusBadRequest)
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"models": models})
}

func handleAgentRuntime(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		http.Error(w, jsonError("method not allowed"), http.StatusMethodNotAllowed)
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
		return
	}
	statuses := make([]provider.AgentRuntimeStatus, 0, 2)
	for _, id := range []provider.AgentID{provider.AgentClaude, provider.AgentCodex} {
		status, readErr := provider.ReadAgentRuntime(home, id)
		if readErr != nil {
			status.Warnings = append(status.Warnings, readErr.Error())
		}
		statuses = append(statuses, status)
	}
	json.NewEncoder(w).Encode(map[string]any{"agents": statuses})
}

func handleAgentOptions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		http.Error(w, jsonError("method not allowed"), http.StatusMethodNotAllowed)
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"agents": provider.AgentOptionSchemas()})
}

func handleAgentBinding(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		http.Error(w, jsonError("method not allowed"), http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Binding provider.AgentBinding `json:"binding"`
		Apply   bool                  `json:"apply"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Binding.AgentID == "" || req.Binding.ProviderID == "" {
		http.Error(w, jsonError("binding.agent_id and binding.provider_id are required"), http.StatusBadRequest)
		return
	}
	p, err := defaultProviderStore.Get(req.Binding.ProviderID)
	if err != nil {
		http.Error(w, jsonError(err.Error()), http.StatusNotFound)
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
		return
	}
	plan, err := provider.PlanAgentBinding(home, *p, req.Binding)
	if err != nil {
		http.Error(w, jsonError(err.Error()), http.StatusBadRequest)
		return
	}
	if !req.Apply {
		json.NewEncoder(w).Encode(map[string]any{"plan": plan})
		return
	}
	result, err := provider.ApplyAgentBinding(home, *p, req.Binding)
	if err != nil {
		http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
		return
	}
	if err := persistAgentBinding(req.Binding); err != nil {
		if rollbackErr := provider.RollbackApply(result); rollbackErr != nil {
			err = fmt.Errorf("save binding: %v; rollback config: %v", err, rollbackErr)
		}
		http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"plan": plan, "result": result})
}

// handleAgentSwitch handles POST /api/agents/switch to bind a provider & model fine-tuning to a specific Agent CLI.
func handleAgentSwitch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		http.Error(w, jsonError("method not allowed"), http.StatusMethodNotAllowed)
		return
	}

	var req provider.AgentSyncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Agent == "" || req.ProviderID == "" {
		http.Error(w, jsonError("agent and provider_id are required"), http.StatusBadRequest)
		return
	}

	if err := defaultProviderStore.SyncAgentConfig(req); err != nil {
		http.Error(w, jsonError(err.Error()), http.StatusBadRequest)
		return
	}

	json.NewEncoder(w).Encode(map[string]any{
		"success":     true,
		"agent":       req.Agent,
		"provider_id": req.ProviderID,
	})
}
