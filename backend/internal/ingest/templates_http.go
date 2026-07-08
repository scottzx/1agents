package ingest

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/scottzx/1Agents/backend/examples"
	"github.com/scottzx/1Agents/backend/internal/sources"
)

// templates_http.go is the 从模板一键安装 surface: the built-in connector + governance
// examples are go:embed-ed into the binary (package examples), so a user can install
// a working 训记 REST connector or the 跨源联系人并集 governance DAG with one click —
// no YAML to author. Installing copies the manifest + any referenced scripts into
// ~/.1agents/{connectors,governance}/ and hot-registers it, exactly as a hand-dropped
// file is loaded at startup (参考 connectors 热加载).

// templateInfo describes one installable template.
type templateInfo struct {
	ID          string `json:"id"`   // "connectors/<base>" | "governance/<base>"
	Kind        string `json:"kind"` // connector | governance
	Vendor      string `json:"vendor,omitempty"`
	Label       string `json:"label"`
	Collections int    `json:"collections,omitempty"`
	Steps       int    `json:"steps,omitempty"`
	Installed   bool   `json:"installed"`
}

var templateBaseRe = regexp.MustCompile(`^[a-z0-9_-]{1,60}$`)

func isYAMLName(n string) bool {
	return strings.HasSuffix(n, ".yaml") || strings.HasSuffix(n, ".yml")
}

// listTemplates enumerates the embedded connector + governance templates, marking
// which are already installed (connector: vendor registered; governance: file present).
func listTemplates() ([]templateInfo, error) {
	var out []templateInfo
	if entries, err := fs.ReadDir(examples.FS, "connectors"); err == nil {
		for _, e := range entries {
			if e.IsDir() || !isYAMLName(e.Name()) {
				continue
			}
			b, err := examples.FS.ReadFile("connectors/" + e.Name())
			if err != nil {
				return nil, err
			}
			m, err := sources.ParseManifest(b)
			if err != nil {
				continue
			}
			base := strings.TrimSuffix(strings.TrimSuffix(e.Name(), ".yaml"), ".yml")
			label := m.Label
			if label == "" {
				label = m.Vendor
			}
			out = append(out, templateInfo{
				ID: "connectors/" + base, Kind: "connector", Vendor: m.Vendor, Label: label,
				Collections: len(m.Collections), Installed: sources.VendorFor(m.Vendor) != nil,
			})
		}
	}
	if entries, err := fs.ReadDir(examples.FS, "governance"); err == nil {
		for _, e := range entries {
			if e.IsDir() || !isYAMLName(e.Name()) {
				continue
			}
			b, err := examples.FS.ReadFile("governance/" + e.Name())
			if err != nil {
				return nil, err
			}
			gm, err := sources.ParseGovernanceManifest(b)
			if err != nil || gm.Name == "" {
				continue
			}
			base := strings.TrimSuffix(strings.TrimSuffix(e.Name(), ".yaml"), ".yml")
			_, statErr := os.Stat(filepath.Join(sources.GovernanceDir(), e.Name()))
			out = append(out, templateInfo{
				ID: "governance/" + base, Kind: "governance", Label: gm.Name,
				Steps: len(gm.Steps), Installed: statErr == nil,
			})
		}
	}
	return out, nil
}

// copyEmbeddedScripts copies each step's referenced script from the embedded tree
// (<subdir>/<script>) into destDir/<script>, so the interpreter finds it at run
// time. Absolute script paths are left untouched.
func copyEmbeddedScripts(steps []sources.ManifestStep, subdir, destDir string) error {
	for _, s := range steps {
		if s.Script == "" || filepath.IsAbs(s.Script) {
			continue
		}
		b, err := examples.FS.ReadFile(subdir + "/" + s.Script)
		if err != nil {
			return fmt.Errorf("template script %s: %w", s.Script, err)
		}
		dst := filepath.Join(destDir, s.Script)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// installTemplate copies a template (manifest + scripts) into the live dir and
// hot-registers it. Returns the installed template's descriptor.
func (h *Handler) installTemplate(id string) (templateInfo, error) {
	var zero templateInfo
	parts := strings.SplitN(id, "/", 2)
	if len(parts) != 2 || !templateBaseRe.MatchString(parts[1]) {
		return zero, fmt.Errorf("bad template id %q", id)
	}
	kind, base := parts[0], parts[1]
	switch kind {
	case "connectors":
		b, err := examples.FS.ReadFile("connectors/" + base + ".yaml")
		if err != nil {
			return zero, fmt.Errorf("no such template %q", id)
		}
		m, err := sources.ParseManifest(b)
		if err != nil {
			return zero, err
		}
		// Scripts must exist before the connector's governance steps run.
		if err := copyEmbeddedScripts(m.Governance, "connectors", sources.ConnectorsDir()); err != nil {
			return zero, err
		}
		if _, err := h.AddConnector(b); err != nil { // persists manifest + hot-registers
			return zero, err
		}
		return templateInfo{ID: id, Kind: "connector", Vendor: m.Vendor, Label: orVendor(m.Label, m.Vendor), Collections: len(m.Collections), Installed: true}, nil
	case "governance":
		b, err := examples.FS.ReadFile("governance/" + base + ".yaml")
		if err != nil {
			return zero, fmt.Errorf("no such template %q", id)
		}
		gm, err := sources.ParseGovernanceManifest(b)
		if err != nil {
			return zero, err
		}
		if err := copyEmbeddedScripts(gm.Steps, "governance", sources.GovernanceDir()); err != nil {
			return zero, err
		}
		if err := os.MkdirAll(sources.GovernanceDir(), 0o755); err != nil {
			return zero, err
		}
		if err := os.WriteFile(filepath.Join(sources.GovernanceDir(), base+".yaml"), b, 0o644); err != nil {
			return zero, err
		}
		h.RegisterGovernanceManifests([]sources.GovernanceManifest{gm})
		return templateInfo{ID: id, Kind: "governance", Label: gm.Name, Steps: len(gm.Steps), Installed: true}, nil
	default:
		return zero, fmt.Errorf("unknown template kind %q", kind)
	}
}

func orVendor(label, vendor string) string {
	if label == "" {
		return vendor
	}
	return label
}

// HandleTemplates serves /api/sources/templates.
//
//	GET  → the embedded connector + governance templates (with installed flag)
//	POST → install one ({id}), hot-registered immediately
func (h *Handler) HandleTemplates(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list, err := listTemplates()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, list)
	case http.MethodPost:
		var req struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&req); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		if req.ID == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}
		info, err := h.installTemplate(req.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, info)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
