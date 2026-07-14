package supervisor

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	anthropicAuthToken = "ANTHROPIC_AUTH_TOKEN"
	anthropicBaseURL   = "ANTHROPIC_BASE_URL"
)

type claudeSettings struct {
	Env map[string]string `json:"env"`
}

// claudeEnvironment reads the two Claude provider variables that are relevant
// to ttyd sessions. Missing settings are intentionally ignored so the normal
// process environment remains the fallback.
func claudeEnvironment() (map[string]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}

	data, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("read Claude settings: %w", err)
	}

	var settings claudeSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("parse Claude settings: %w", err)
	}

	env := make(map[string]string, 2)
	for _, name := range []string{anthropicAuthToken, anthropicBaseURL} {
		if value, ok := settings.Env[name]; ok && value != "" {
			env[name] = value
		}
	}
	return env, nil
}

func mergeEnvironment(base []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return base
	}

	merged := make(map[string]string, len(base)+len(overrides))
	for _, entry := range base {
		for i := 0; i < len(entry); i++ {
			if entry[i] == '=' {
				merged[entry[:i]] = entry[i+1:]
				break
			}
		}
	}
	for name, value := range overrides {
		merged[name] = value
	}

	out := make([]string, 0, len(merged))
	for name, value := range merged {
		out = append(out, name+"="+value)
	}
	return out
}

// refreshTmuxEnvironment makes newly created windows use the same settings as
// the ttyd process. Existing shell processes cannot have their environment
// changed and must be recreated by the user.
func refreshTmuxEnvironment(session string, env map[string]string) {
	if len(env) == 0 {
		return
	}
	for name, value := range env {
		// Update the server-wide value first so a session created by ttyd after
		// this call inherits the refreshed setting.
		if err := exec.Command("tmux", "set-environment", "-g", name, value).Run(); err != nil {
			log.Printf("[supervisor] unable to refresh global tmux environment for %s: %v", name, err)
		}
		if session != "" {
			if err := exec.Command("tmux", "set-environment", "-t", session, name, value).Run(); err != nil {
				// The session may not exist yet; the global value covers the next
				// session created by ttyd.
				log.Printf("[supervisor] unable to refresh tmux environment for %s: %v", name, err)
			}
		}
	}
}

// RefreshClaudeTmuxEnvironment refreshes a session after the backend has
// ensured that it exists. This is separate from startProcess because ttyd is
// started before the HTTP server creates the persistent tmux session.
func RefreshClaudeTmuxEnvironment(session string) {
	env, err := claudeEnvironment()
	if err != nil {
		log.Printf("[supervisor] Claude settings unavailable for tmux refresh: %v", err)
		return
	}
	refreshTmuxEnvironment(session, env)
}
