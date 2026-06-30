package workspace

// Project config HTTP handlers (§327) and template scaffold shim (§329).
//
// The storage layer lives in templateregistry.LoadProjectConfig / SaveProjectConfig
// so that templateregistry.Scaffold can seed config without a circular import.
// These handlers expose it as a REST endpoint on the workspace HTTP surface.
//
// Routes registered in server.go:
//   GET  /api/project/config?id={projectID}
//   PUT  /api/project/config

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/scottzx/1Agents/backend/internal/templateregistry"
)

// scaffoldProject delegates to templateregistry.Scaffold. Called by the Create
// handler when a templateId is supplied.
func scaffoldProject(templateID, projectID, workspacePath string) (templateregistry.ScaffoldResult, error) {
	return templateregistry.Scaffold(templateID, projectID, workspacePath)
}

// HandleProjectConfigGet handles GET /api/project/config?id=<projectID>
func HandleProjectConfigGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id query parameter", http.StatusBadRequest)
		return
	}
	cfg, err := templateregistry.LoadProjectConfig(id)
	if err != nil {
		log.Printf("[workspace] load project config %s: %v", id, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, cfg)
}

// HandleProjectConfigPut handles PUT /api/project/config
// Body: {"id": "...", "instructions": "...", "connectors": [...], ...}
func HandleProjectConfigPut(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		ID     string                        `json:"id"`
		Config templateregistry.ProjectConfig `json:"config"`
	}
	// Accept either wrapped {"id":..., "config":{...}} or flat {"id":..., "instructions":...}
	// For simplicity, accept flat format where config fields are at top level.
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if idRaw, ok := raw["id"]; ok {
		_ = json.Unmarshal(idRaw, &body.ID)
	}
	// Decode config fields — accept both flat and wrapped shape.
	if cfgRaw, ok := raw["config"]; ok {
		_ = json.Unmarshal(cfgRaw, &body.Config)
	} else {
		// Flat shape: config fields at top level.
		if v, ok := raw["instructions"]; ok {
			_ = json.Unmarshal(v, &body.Config.Instructions)
		}
		if v, ok := raw["connectors"]; ok {
			_ = json.Unmarshal(v, &body.Config.Connectors)
		}
		if v, ok := raw["experts"]; ok {
			_ = json.Unmarshal(v, &body.Config.Experts)
		}
		if v, ok := raw["skills"]; ok {
			_ = json.Unmarshal(v, &body.Config.Skills)
		}
		if v, ok := raw["automation"]; ok {
			_ = json.Unmarshal(v, &body.Config.Automation)
		}
	}
	if body.ID == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	if err := templateregistry.SaveProjectConfig(body.ID, body.Config); err != nil {
		log.Printf("[workspace] save project config %s: %v", body.ID, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}
