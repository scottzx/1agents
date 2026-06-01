// Package system provides system-level management APIs:
// version info, latest version check, and OTA self-update via NPM.
package system

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// npmPackageName is the canonical NPM package for this agent.
const npmPackageName = "@scottzx/1agents"

// ── Update state tracker ──────────────────────────────────────────────────────

type updateState struct {
	mu          sync.RWMutex
	running     bool
	startedAt   time.Time
	restartMode string // "systemd" | "exec" | "manual"
	log         []string
}

var state = &updateState{}

func (s *updateState) start() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return false
	}
	s.running = true
	s.startedAt = time.Now()
	s.log = nil
	s.restartMode = ""
	return true
}

func (s *updateState) finish() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = false
}

func (s *updateState) appendLog(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.log = append(s.log, line)
	if len(s.log) > 200 {
		s.log = s.log[len(s.log)-200:]
	}
}

func (s *updateState) setRestartMode(mode string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restartMode = mode
}

func (s *updateState) snapshot() (bool, time.Time, string, []string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	logs := make([]string, len(s.log))
	copy(logs, s.log)
	return s.running, s.startedAt, s.restartMode, logs
}

// ── Version info ──────────────────────────────────────────────────────────────

// VersionInfo holds the local and remote version details.
type VersionInfo struct {
	Current     string `json:"current"`      // local installed version
	Latest      string `json:"latest"`       // latest on NPM registry
	HasUpdate   bool   `json:"has_update"`   // latest > current
	Package     string `json:"package"`      // npm package name
	RestartMode string `json:"restart_mode"` // how OTA will restart: "systemd" | "exec" | "manual"
}

// getLocalVersion reads the installed version from the npm package's package.json.
func getLocalVersion() string {
	candidates := []string{}

	if out, err := exec.Command("npm", "root", "-g").Output(); err == nil {
		root := strings.TrimSpace(string(out))
		candidates = append(candidates, filepath.Join(root, npmPackageName, "package.json"))
	}

	exe, _ := os.Executable()
	if exe != "" {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "..", "package.json"),
			filepath.Join(dir, "package.json"),
		)
	}

	for _, p := range candidates {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var pkg struct {
			Version string `json:"version"`
		}
		if json.Unmarshal(data, &pkg) == nil && pkg.Version != "" {
			return pkg.Version
		}
	}
	return "unknown"
}

// getLatestVersion queries the NPM registry for the latest published version.
func getLatestVersion() (string, error) {
	client := &http.Client{Timeout: 8 * time.Second}

	urls := []string{
		fmt.Sprintf("https://registry.npmjs.org/%s/latest", npmPackageName),
		fmt.Sprintf("https://registry.npmmirror.com/%s/latest", npmPackageName),
	}

	for _, url := range urls {
		resp, err := client.Get(url)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		resp.Body.Close()
		var meta struct {
			Version string `json:"version"`
		}
		if err := json.Unmarshal(body, &meta); err == nil && meta.Version != "" {
			return meta.Version, nil
		}
	}
	return "", fmt.Errorf("all registries unreachable")
}

// versionGT returns true if a > b (lexicographic, sufficient for date-based versions).
func versionGT(a, b string) bool {
	return a != "" && b != "" && b != "unknown" && a > b
}

// ── Restart mode detection ────────────────────────────────────────────────────

// restartMode describes how the service will be restarted after OTA update.
type restartMode int

const (
	restartSystemd restartMode = iota // Linux: systemctl restart <unit>
	restartExec                       // Unix: syscall.Exec — in-place binary replacement
	restartManual                     // Windows or unknown: user must restart manually
)

func detectRestartMode() (restartMode, string) {
	// 1. Linux + systemd
	if runtime.GOOS == "linux" {
		if unit := detectSystemdUnit(); unit != "" {
			return restartSystemd, unit
		}
	}

	// 2. Unix (macOS, Linux without systemd, *BSD): exec restart
	if canExecRestart() {
		return restartExec, ""
	}

	// 3. Other (Windows): manual
	return restartManual, ""
}

func restartModeName(m restartMode) string {
	switch m {
	case restartSystemd:
		return "systemd"
	case restartExec:
		return "exec"
	default:
		return "manual"
	}
}

// detectSystemdUnit checks for a running 1agents systemd unit.
func detectSystemdUnit() string {
	for _, unit := range []string{"1agents", "1agents.service"} {
		out, err := exec.Command("systemctl", "is-active", unit).Output()
		if err == nil && strings.TrimSpace(string(out)) == "active" {
			return unit
		}
	}
	return ""
}

// ── HTTP Handlers ─────────────────────────────────────────────────────────────

// NewHandler creates the HTTP handler for /api/system/* routes.
func NewHandler() *Handler {
	return &Handler{}
}

// Handler implements the system management HTTP endpoints.
type Handler struct{}

// Version handles GET /api/system/version
func (h *Handler) Version(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	current := getLocalVersion()
	latest, err := getLatestVersion()
	hasUpdate := false
	if err == nil {
		hasUpdate = versionGT(latest, current)
	}

	mode, _ := detectRestartMode()

	info := VersionInfo{
		Current:     current,
		Package:     npmPackageName,
		HasUpdate:   hasUpdate,
		RestartMode: restartModeName(mode),
	}
	if err != nil {
		info.Latest = "unavailable"
	} else {
		info.Latest = latest
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(info)
}

// UpdateStatus handles GET /api/system/update/status
func (h *Handler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	running, startedAt, mode, logs := state.snapshot()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"running":      running,
		"started_at":   startedAt,
		"restart_mode": mode,
		"log":          logs,
	})
}

// Update handles POST /api/system/update
// Body (optional): {"version":"20260526.2.0"}
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Version string `json:"version"`
	}
	if r.ContentLength > 0 {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}

	if !state.start() {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "An update is already in progress. Check /api/system/update/status.",
		})
		return
	}

	pkgTarget := npmPackageName + "@latest"
	if body.Version != "" {
		pkgTarget = npmPackageName + "@" + body.Version
	}

	mode, extra := detectRestartMode()
	state.setRestartMode(restartModeName(mode))

	go runUpdate(pkgTarget, mode, extra)

	msg := "OTA update launched. Service will restart automatically when done."
	if mode == restartManual {
		msg = "OTA update launched. npm install will complete, but you must restart the service manually (no system supervisor detected)."
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"status":       "update_started",
		"package":      pkgTarget,
		"restart_mode": restartModeName(mode),
		"message":      msg,
	})
}

// ── OTA update background worker ─────────────────────────────────────────────

func runUpdate(pkgTarget string, mode restartMode, extra string) {
	defer state.finish()

	ts := func() string { return time.Now().Format("15:04:05") }
	appendLog := func(format string, args ...interface{}) {
		line := fmt.Sprintf("[%s] "+format, append([]interface{}{ts()}, args...)...)
		state.appendLog(line)
		log.Printf("[system/ota] %s", fmt.Sprintf(format, args...))
	}

	appendLog("=== OTA update started ===")
	appendLog("Package: %s", pkgTarget)
	appendLog("Restart strategy: %s", restartModeName(mode))

	// ── Step 1: npm install -g ────────────────────────────────────────────────
	appendLog("Running: npm install -g %s", pkgTarget)

	cmd := exec.Command("npm", "install", "-g", pkgTarget)
	cmd.Env = append(os.Environ(), "NPM_CONFIG_PROGRESS=false")

	pr, pw, err := os.Pipe()
	if err != nil {
		appendLog("ERROR: pipe: %v", err)
		return
	}
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		pw.Close()
		pr.Close()
		appendLog("ERROR: npm start: %v", err)
		appendLog("HINT: Make sure 'npm' is in your PATH. Current PATH: %s", os.Getenv("PATH"))
		return
	}

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := pr.Read(buf)
			if n > 0 {
				for _, line := range strings.Split(strings.TrimRight(string(buf[:n]), "\n"), "\n") {
					if l := strings.TrimSpace(line); l != "" {
						state.appendLog(l)
					}
				}
			}
			if err != nil {
				break
			}
		}
	}()

	if err := cmd.Wait(); err != nil {
		pw.Close()
		pr.Close()
		appendLog("ERROR: npm install failed: %v", err)
		if os.Getuid() != 0 {
			appendLog("HINT: If permission denied, your npm global directory may be owned by root.")
			appendLog("      Try: sudo chown -R $(whoami) $(npm root -g)/..")
		}
		return
	}
	pw.Close()
	pr.Close()
	appendLog("npm install completed successfully.")

	// ── Step 2: Restart ───────────────────────────────────────────────────────
	switch mode {
	case restartSystemd:
		appendLog("Restarting via systemd: %s", extra)
		restartCmd := exec.Command("systemctl", "restart", extra)
		restartCmd.SysProcAttr = detachSysProcAttr()
		if err := restartCmd.Start(); err != nil {
			appendLog("ERROR: systemctl restart failed: %v", err)
			appendLog("You may need to restart manually: systemctl restart %s", extra)
		} else {
			appendLog("systemctl restart issued. Service will be back in ~5 seconds.")
		}

	case restartExec:
		// Find the new binary path from npm global bin
		newBin := findNpmGlobalBin("1agents")
		appendLog("In-place restart (exec): replacing process with %s", newBin)
		appendLog("Connection will drop briefly — the service restarts with the same arguments.")

		// Small delay so this log line reaches the client before the process is replaced
		time.Sleep(800 * time.Millisecond)

		if err := execRestart(newBin); err != nil {
			// execRestart only returns on failure
			appendLog("ERROR: exec restart failed: %v", err)
			appendLog("Please restart the service manually.")
		}

	case restartManual:
		appendLog("=== Update complete ===")
		appendLog("No system supervisor detected (not systemd, not Unix exec).")
		appendLog("Please restart the service manually to apply the update.")
	}
}

// findNpmGlobalBin returns the path to a binary installed in the npm global bin directory.
func findNpmGlobalBin(name string) string {
	// Try `npm bin -g` first
	if out, err := exec.Command("npm", "bin", "-g").Output(); err == nil {
		binDir := strings.TrimSpace(string(out))
		candidate := filepath.Join(binDir, name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	// Fallback: use PATH resolution
	if path, err := exec.LookPath(name); err == nil {
		return path
	}
	return name
}
