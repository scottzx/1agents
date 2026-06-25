package system

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
)

// relay credentials persistence (issue #109).
//
// The relay *client account* master key ({token, secretB64}) used to live only
// in the browser localStorage, so it was lost on device change or when storage
// was cleared. This persists it on the 1agents host so logging into the local
// 1agents auto-loads the credentials, independent of browser storage.
//
// Note: this is the *client-side account* key (created in-browser via
// createAccount). It is orthogonal to the *machine/daemon* credentials surfaced
// by /api/system/happy/status (read from ~/.happy/, written by `happy auth
// login`). We deliberately do not add a parallel relay_config table — a single
// JSON file under ~/.1agents/ matches the existing file-based credential pattern
// in happy.go and avoids plumbing meta.DB into this stateless handler.

// RelayCredentials is the JSON shape persisted and exchanged with the frontend.
type RelayCredentials struct {
	RelayURL  string `json:"relayUrl,omitempty"`
	Token     string `json:"token"`
	SecretB64 string `json:"secretB64"`
	CreatedAt int64  `json:"createdAt,omitempty"`
}

// oneAgentsHome resolves ~/.1agents (honoring ONEAGENTS_HOME, same as meta.DB).
func oneAgentsHome() string {
	if val := os.Getenv("ONEAGENTS_HOME"); val != "" {
		return filepath.Join(val, ".1agents")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".1agents"
	}
	return filepath.Join(home, ".1agents")
}

func relayCredsPath() string {
	return filepath.Join(oneAgentsHome(), "relay-creds.json")
}

// RelayCredentialsHandler handles GET/POST/DELETE /api/relay/credentials.
// Only reachable from the local 1agents server, so returning raw credentials is
// safe (same trust model as /api/system/happy/status).
func (h *Handler) RelayCredentialsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.getRelayCredentials(w, r)
	case http.MethodPost:
		h.postRelayCredentials(w, r)
	case http.MethodDelete:
		h.deleteRelayCredentials(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// getRelayCredentials returns the stored credentials, or null when none exist.
func (h *Handler) getRelayCredentials(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	data, err := os.ReadFile(relayCredsPath())
	if err != nil {
		// No file (or unreadable) → treat as "no credentials".
		_, _ = w.Write([]byte("null"))
		return
	}
	var creds RelayCredentials
	if json.Unmarshal(data, &creds) != nil || creds.Token == "" || creds.SecretB64 == "" {
		_, _ = w.Write([]byte("null"))
		return
	}
	_ = json.NewEncoder(w).Encode(creds)
}

// postRelayCredentials saves (or replaces) the credentials.
func (h *Handler) postRelayCredentials(w http.ResponseWriter, r *http.Request) {
	var creds RelayCredentials
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		jsonError(w, "invalid JSON body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if creds.Token == "" || creds.SecretB64 == "" {
		jsonError(w, "token and secretB64 are required", http.StatusBadRequest)
		return
	}
	if err := os.MkdirAll(oneAgentsHome(), 0o755); err != nil {
		jsonError(w, "ensure home dir: "+err.Error(), http.StatusInternalServerError)
		return
	}
	data, err := json.Marshal(creds)
	if err != nil {
		jsonError(w, "marshal: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// 0600: the file holds a secret master key.
	if err := os.WriteFile(relayCredsPath(), data, 0o600); err != nil {
		jsonError(w, "write credentials: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// deleteRelayCredentials removes the stored credentials (unbind account).
func (h *Handler) deleteRelayCredentials(w http.ResponseWriter, _ *http.Request) {
	if err := os.Remove(relayCredsPath()); err != nil && !os.IsNotExist(err) {
		jsonError(w, "delete credentials: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write([]byte(`{"ok":true}`))
}
