// channels.go owns the privacy/consent + crawl-rule layer for the 渠道 page. Each
// data-source sub-module (iCloud 通讯录, iMessage, 飞书群消息) must be explicitly
// authorized by the user before any sync runs, and carries a small set of
// deterministic crawl rules (frequency + scope) consumed by the Go syncers — never
// by an AI. These endpoints back the redesigned 渠道 tab.
package contacts

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

// Channel sub-module IDs — the consent + crawl-rule unit, stored in channel_modules.
const (
	ModICloudContacts = "icloud.contacts"
	ModAppleIMessage  = "apple.imessage"
	ModFeishuGroups   = "feishu.groups"
)

var knownModules = map[string]bool{
	ModICloudContacts: true,
	ModAppleIMessage:  true,
	ModFeishuGroups:   true,
}

// errNotConsented gates a sync behind explicit per-module authorization.
var errNotConsented = errors.New("channel module not authorized — grant consent first")

// requireConsent returns errNotConsented unless the module has been authorized.
func (h *Handler) requireConsent(moduleID string) error {
	if h.cms == nil {
		return nil
	}
	m, err := h.cms.Get(moduleID)
	if err != nil {
		return err
	}
	if !m.Consented {
		return errNotConsented
	}
	return nil
}

// moduleRules returns a module's stored crawl rules (empty map if none/unset).
func (h *Handler) moduleRules(moduleID string) map[string]any {
	if h.cms == nil {
		return map[string]any{}
	}
	m, _ := h.cms.Get(moduleID)
	if m.Rules == nil {
		return map[string]any{}
	}
	return m.Rules
}

// ruleInt reads a numeric rule (JSON numbers decode as float64); ok is false when
// absent or not a number.
func ruleInt(rules map[string]any, key string) (int, bool) {
	if v, present := rules[key]; present {
		if f, isNum := v.(float64); isNum {
			return int(f), true
		}
	}
	return 0, false
}

// ruleBool reads a boolean rule, defaulting to def when absent.
func ruleBool(rules map[string]any, key string, def bool) bool {
	if v, present := rules[key]; present {
		if b, isBool := v.(bool); isBool {
			return b
		}
	}
	return def
}

// HandleChannelModules: GET /api/channels/modules → consent + crawl rules for
// every known sub-module (the page's config state; live connection status comes
// from the per-channel endpoints).
func (h *Handler) HandleChannelModules(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ids := []string{ModICloudContacts, ModAppleIMessage, ModFeishuGroups}
	out := make([]meta.ChannelModule, 0, len(ids))
	for _, id := range ids {
		m, err := h.cms.Get(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		out = append(out, m)
	}
	writeJSON(w, http.StatusOK, out)
}

// HandleChannelModuleItem routes per-module actions:
//
//	POST   /api/channels/modules/{id}/consent  — record explicit authorization
//	DELETE /api/channels/modules/{id}/consent  — revoke
//	PUT    /api/channels/modules/{id}/rules     — set crawl rules {autoSync, intervalMinutes, rules}
func (h *Handler) HandleChannelModuleItem(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/channels/modules/")
	id, action, ok := strings.Cut(rest, "/")
	if !ok || !knownModules[id] {
		badRequest(w, "unknown channel module")
		return
	}
	switch {
	case action == "consent" && r.Method == http.MethodPost:
		if err := h.cms.SetConsent(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case action == "consent" && r.Method == http.MethodDelete:
		if err := h.cms.RevokeConsent(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case action == "rules" && r.Method == http.MethodPut:
		var body struct {
			AutoSync        bool           `json:"autoSync"`
			IntervalMinutes int            `json:"intervalMinutes"`
			Rules           map[string]any `json:"rules"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			badRequest(w, "invalid body")
			return
		}
		if err := h.cms.SetRules(id, body.AutoSync, body.IntervalMinutes, body.Rules); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		m, _ := h.cms.Get(id)
		writeJSON(w, http.StatusOK, m)
	default:
		badRequest(w, "unknown action")
	}
}
