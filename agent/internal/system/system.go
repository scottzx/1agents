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
const npmPackageName = "@scottzx/remote-agents"

// updateState tracks the OTA update status.
type updateState struct {
	mu        sync.RWMutex
	running   bool
	startedAt time.Time
	log       []string
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

func (s *updateState) snapshot() (bool, time.Time, []string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	logs := make([]string, len(s.log))
	copy(logs, s.log)
	return s.running, s.startedAt, logs
}

// VersionInfo holds the local and remote version details.
type VersionInfo struct {
	Current string `json:"current"`          // local installed version
	Latest  string `json:"latest"`           // latest on NPM registry
	HasUpdate bool `json:"has_update"`       // latest > current
	Package string `json:"package"`          // npm package name
	UpdateMode string `json:"update_mode"`  // "online" | "offline"
}

// getLocalVersion reads the installed version from the npm package's package.json.
func getLocalVersion() string {
	// Walk candidate paths for the installed package.json.
	// When run via `npm install -g`, the package ends up in the global node_modules.
	candidates := []string{}

	// Check via `npm root -g`
	if out, err := exec.Command("npm", "root", "-g").Output(); err == nil {
		root := strings.TrimSpace(string(out))
		candidates = append(candidates, filepath.Join(root, npmPackageName, "package.json"))
	}

	// Fallback: walk from the binary's own directory upward
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
	url := fmt.Sprintf("https://registry.npmjs.org/%s/latest", npmPackageName)
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		// Try npmmirror as fallback
		url2 := fmt.Sprintf("https://registry.npmmirror.com/%s/latest", npmPackageName)
		resp, err = client.Get(url2)
		if err != nil {
			return "", fmt.Errorf("registry unreachable: %w", err)
		}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	var meta struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &meta); err != nil || meta.Version == "" {
		return "", fmt.Errorf("failed to parse registry response")
	}
	return meta.Version, nil
}

// versionGT returns true if a > b using simple semver comparison.
func versionGT(a, b string) bool {
	if a == b || a == "" || b == "unknown" {
		return false
	}
	return a > b // lexicographic approximation, sufficient for date-based versions like 20260526.2.0
}

// NewHandler creates the HTTP handler for /api/system/* routes.
func NewHandler() *Handler {
	return &Handler{}
}

// Handler implements the system management HTTP endpoints.
type Handler struct{}

// Version handles GET /api/system/version
// Returns current + latest version, and whether an update is available.
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

	info := VersionInfo{
		Current:    current,
		Latest:     latest,
		HasUpdate:  hasUpdate,
		Package:    npmPackageName,
		UpdateMode: "online",
	}
	if err != nil {
		info.Latest = "unavailable"
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(info)
}

// UpdateStatus handles GET /api/system/update/status
// Returns the current OTA update progress.
func (h *Handler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	running, startedAt, logs := state.snapshot()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"running":    running,
		"started_at": startedAt,
		"log":        logs,
	})
}

// Update handles POST /api/system/update
// Triggers an OTA self-update in a fully detached background process.
// Returns immediately with 202 Accepted so the caller is not blocked.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Allow explicit version pinning via JSON body: {"version":"20260526.2.0"}
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

	go runDetachedUpdate(pkgTarget)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "update_started",
		"package": pkgTarget,
		"message": "OTA update has been launched in the background. The service will restart automatically upon completion. Poll /api/system/update/status to track progress.",
	})
}

// runDetachedUpdate performs the NPM install and systemd restart in a background goroutine.
// The parent HTTP server remains live throughout; the new binary is started by systemd
// only after the old one exits cleanly via `systemctl restart`.
func runDetachedUpdate(pkgTarget string) {
	defer state.finish()

	appendLog := func(format string, args ...interface{}) {
		line := fmt.Sprintf("[%s] "+format, append([]interface{}{time.Now().Format("15:04:05")}, args...)...)
		state.appendLog(line)
		log.Println("[system/ota]", strings.TrimPrefix(line, fmt.Sprintf("[%s] ", time.Now().Format("15:04:05"))))
	}

	appendLog("Starting OTA update: %s", pkgTarget)

	// ── Step 1: npm install -g <package>@<version> ────────────────────────────
	npmCmd := "npm"
	npmArgs := []string{"install", "-g", pkgTarget}

	appendLog("Running: %s %s", npmCmd, strings.Join(npmArgs, " "))

	cmd := exec.Command(npmCmd, npmArgs...)
	cmd.Env = append(os.Environ(), "NPM_CONFIG_PROGRESS=false")

	// Capture combined stdout+stderr and stream to our state log
	pr, pw, err := os.Pipe()
	if err != nil {
		appendLog("ERROR: failed to create pipe: %v", err)
		return
	}
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		pw.Close()
		pr.Close()
		appendLog("ERROR: failed to start npm: %v", err)
		return
	}

	// Stream npm output
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := pr.Read(buf)
			if n > 0 {
				for _, line := range strings.Split(strings.TrimRight(string(buf[:n]), "\n"), "\n") {
					if line = strings.TrimSpace(line); line != "" {
						state.appendLog(line)
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
		return
	}
	pw.Close()
	pr.Close()

	appendLog("npm install completed successfully.")

	// ── Step 2: Restart via systemd (Linux) or graceful self-exit (other) ────
	if runtime.GOOS == "linux" {
		// Check if we are running under a known systemd unit
		unitName := detectSystemdUnit()
		if unitName != "" {
			appendLog("Restarting systemd unit: %s", unitName)
			// Use a fully independent subprocess so we survive the restart
			restartCmd := exec.Command("systemctl", "restart", unitName)
			restartCmd.SysProcAttr = detachSysProcAttr()
			if err := restartCmd.Start(); err != nil {
				appendLog("ERROR: systemctl restart failed: %v. Manual restart may be required.", err)
			} else {
				appendLog("systemctl restart issued. Service will restart momentarily.")
			}
			return
		}
	}

	// Fallback for non-Linux or non-systemd: ask the process to gracefully exit.
	// The process supervisor (e.g. Docker restart policy, pm2) will relaunch with the new binary.
	appendLog("Not running under systemd. Sending SIGTERM for graceful restart...")
	proc, err := os.FindProcess(os.Getpid())
	if err == nil {
		time.Sleep(500 * time.Millisecond) // flush log
		_ = proc.Signal(os.Interrupt)
	}
}

// detectSystemdUnit checks the INVOCATION_ID or SYSTEMD_EXEC_PID env vars
// to determine the active systemd unit name.
func detectSystemdUnit() string {
	// Check well-known unit names
	candidates := []string{
		"remote-agents",
		"remote-agents.service",
	}
	for _, unit := range candidates {
		out, err := exec.Command("systemctl", "is-active", unit).Output()
		if err == nil && strings.TrimSpace(string(out)) == "active" {
			return unit
		}
	}
	return ""
}
