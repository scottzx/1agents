package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/scottzx/1Agents/backend/internal/provider"
)

var defaultProviderStore = provider.NewStore("")

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
			"providers":          providers,
			"active_provider_id": activeID,
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
		json.NewEncoder(w).Encode(saved)

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

// handleProviderSwitch handles POST /api/providers/switch to activate a provider across all default agents.
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
		"provider":           active,
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
