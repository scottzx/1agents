package ingest

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"

	"github.com/scottzx/1Agents/backend/internal/sources"
)

// governance_manifest_http.go is the 治理规则/脚本 hot-add surface — the governance
// twin of connectors_http.go (集成/治理解耦). A standalone governance DAG — steps that
// read any data.db table (built-in silver/gold + connector silver) and write entity
// tables — is added from the API without a restart: POST a manifest YAML plus any
// Python scripts it references, and it is validated, its scripts + file persisted to
// ~/.1agents/governance/, and its steps hot-registered into the running DAG.

// scriptRelRe restricts an uploaded script's relative path (kept under the governance
// dir; no absolute paths, no "..").
var scriptRelRe = regexp.MustCompile(`^[a-z0-9_-]+(/[a-z0-9_-]+)*\.(py|js|ts|sh)$`)

type addGovernanceReq struct {
	YAML    string            `json:"yaml"`
	Scripts map[string]string `json:"scripts"` // relative path → file content (for Python steps)
}

// AddGovernanceManifest validates a standalone governance manifest, writes any
// referenced scripts + the manifest file into the governance dir, and hot-registers
// its steps. Re-adding a manifest with the same step names replaces those steps
// (idempotent update), so a rule can be fixed and re-POSTed.
func (h *Handler) AddGovernanceManifest(req addGovernanceReq) (sources.GovernanceManifest, error) {
	gm, err := sources.ParseGovernanceManifest([]byte(req.YAML))
	if err != nil {
		return gm, err
	}
	if err := sources.ValidateGovernanceManifest(gm); err != nil {
		return gm, err
	}
	dir := sources.GovernanceDir()
	// Scripts first — a Python step must find its file before it can run.
	for rel, content := range req.Scripts {
		if !scriptRelRe.MatchString(rel) {
			return gm, fmt.Errorf("unsafe script path %q", rel)
		}
		dst := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return gm, err
		}
		if err := os.WriteFile(dst, []byte(content), 0o644); err != nil {
			return gm, err
		}
	}
	if err := sources.SaveGovernanceManifest(gm.Name, []byte(req.YAML)); err != nil {
		return gm, err
	}
	// Drop any prior versions of these steps so a re-add updates in place.
	names := make(map[string]bool, len(gm.Steps))
	for _, s := range gm.Steps {
		names[s.Name] = true
	}
	h.removeGovernStepsByName(names)
	h.RegisterGovernanceManifests([]sources.GovernanceManifest{gm})
	return gm, nil
}

// removeGovernStepsByName drops registered SQL + script steps whose name is in set,
// so a hot re-add replaces rather than duplicates them.
func (h *Handler) removeGovernStepsByName(set map[string]bool) {
	gold := h.manifestGold[:0]
	for _, s := range h.manifestGold {
		if !set[s.Name] {
			gold = append(gold, s)
		}
	}
	h.manifestGold = gold
	script := h.manifestScript[:0]
	for _, s := range h.manifestScript {
		if !set[s.Name] {
			script = append(script, s)
		}
	}
	h.manifestScript = script
}

// HandleGovernanceManifests serves /api/data/governance/manifests.
//
//	GET  → installed standalone governance manifests
//	POST → add one ({yaml, scripts}), hot-registered immediately (no restart)
func (h *Handler) HandleGovernanceManifests(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		gms, err := sources.LoadGovernanceManifests()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, gms)
	case http.MethodPost:
		var req addGovernanceReq
		if err := json.NewDecoder(io.LimitReader(r.Body, 512<<10)).Decode(&req); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		gm, err := h.AddGovernanceManifest(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"name": gm.Name, "steps": len(gm.Steps)})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
