package govern

// venv.go gives a Python script step its own dependencies (issue #406). By default
// a step runs under bare python3 with only stdlib — fine for the messy-JSON cases
// it was designed for, but a step that needs a third-party package (pandas, a
// vendor SDK) had no way to get it. When a step declares `requirements:`, the
// framework materializes a per-step virtualenv under ~/.1agents/venvs/<step>/ and
// pip-installs them once (a requirements-hash marker skips re-install), then runs
// the script with that venv's interpreter. Opt-in: no requirements ⇒ bare python3,
// no venv, no network.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// venvSetupTimeout bounds venv creation + pip install (network-bound).
const venvSetupTimeout = 5 * time.Minute

func governHome() string {
	if v := os.Getenv("ONEAGENTS_HOME"); v != "" {
		return v
	}
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return "."
}

// ensureVenv creates (once) a per-step virtualenv with the given pip requirements
// and returns its python interpreter path. Empty reqs ⇒ ("", nil): the caller uses
// the default interpreter. Idempotent — a requirements-hash marker skips a rebuild.
func ensureVenv(name string, reqs []string) (string, error) {
	if len(reqs) == 0 {
		return "", nil
	}
	if !identRe.MatchString(name) {
		return "", fmt.Errorf("govern: unsafe venv name %q", name)
	}
	base := filepath.Join(governHome(), ".1agents", "venvs", name)
	py := filepath.Join(base, "bin", "python")
	marker := filepath.Join(base, ".requirements")
	want := strings.Join(reqs, "\n")
	if b, err := os.ReadFile(marker); err == nil && string(b) == want {
		return py, nil // already provisioned with these exact requirements
	}

	ctx, cancel := context.WithTimeout(context.Background(), venvSetupTimeout)
	defer cancel()
	if _, err := os.Stat(py); err != nil {
		if out, err := exec.CommandContext(ctx, "python3", "-m", "venv", base).CombinedOutput(); err != nil {
			return "", fmt.Errorf("govern: create venv %s: %w: %s", name, err, strings.TrimSpace(string(out)))
		}
	}
	args := append([]string{"-m", "pip", "install", "--disable-pip-version-check", "-q"}, reqs...)
	if out, err := exec.CommandContext(ctx, py, args...).CombinedOutput(); err != nil {
		return "", fmt.Errorf("govern: pip install for %s: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	if err := os.WriteFile(marker, []byte(want), 0o644); err != nil {
		return "", err
	}
	return py, nil
}
