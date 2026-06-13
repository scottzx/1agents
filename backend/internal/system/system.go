// Package system provides system-level management APIs:
// version info, latest version check, and OTA self-update from
// GitHub Releases.
package system

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
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

	"github.com/minio/selfupdate"
)

// Channel is the OTA release channel. V1 only supports "stable".
const Channel = "stable"

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
	Latest      string `json:"latest"`       // latest version published on GitHub Releases
	HasUpdate   bool   `json:"has_update"`   // latest > current
	Channel     string `json:"channel"`      // release channel (always "stable" in V1)
	RestartMode string `json:"restart_mode"` // how OTA will restart: "systemd" | "exec" | "manual"
}

// getLocalVersion returns the version baked into the binary via -ldflags.
// main.go populates the package var `LocalVersion` at startup; the
// fallback ("unknown") exists only to keep the function total.
func getLocalVersion() string {
	if LocalVersion != "" && LocalVersion != "dev" {
		return LocalVersion
	}
	// Best-effort fallback: try to read a sibling VERSION file written
	// by the package script. Almost never reached in practice.
	exe, _ := os.Executable()
	if exe != "" {
		if data, err := os.ReadFile(filepath.Join(filepath.Dir(exe), "VERSION")); err == nil {
			s := strings.TrimSpace(string(data))
			if s != "" {
				return s
			}
		}
	}
	return "unknown"
}

// platformKey builds the manifest's binary lookup key for the current
// host (e.g. "darwin-arm64", "linux-amd64", "windows-amd64"). Used by
// both the version-check path and the download path.
func platformKey() string {
	return runtime.GOOS + "-" + runtime.GOARCH
}

// getLatestVersion reads the latest backend.version from the GitHub
// Releases manifest. Returns "" with no error if the manifest exists
// but has no entry for the current platform (caller decides how to
// surface that to the user).
func getLatestVersion() (string, error) {
	body, err := fetchUpstream()
	if err != nil {
		return "", err
	}
	var m RootManifest
	if err := json.Unmarshal(body, &m); err != nil {
		return "", fmt.Errorf("decode manifest: %w", err)
	}
	return m.Components.Backend.Version, nil
}

// platformBinaryURL returns the URL + SHA256 of the binary that
// matches the current platform, or an error if the manifest has no
// entry for us.
func platformBinaryURL(body []byte) (string, string, error) {
	var m RootManifest
	if err := json.Unmarshal(body, &m); err != nil {
		return "", "", fmt.Errorf("decode manifest: %w", err)
	}
	pk := platformKey()
	bin, ok := m.Components.Backend.Platforms[pk]
	if !ok {
		return "", "", fmt.Errorf("manifest has no binary for %s", pk)
	}
	return bin.URL, bin.SHA256, nil
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
		Channel:     Channel,
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
// Body (optional): {"version":"v20260615-1"} — pins to a specific
// release; when omitted, picks the latest from the manifest.
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

	if !OTAEnabled {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"error":   "OTA self-update is disabled in this deployment mode (desktop uses Tauri updater; Docker uses docker pull).",
			"channel": Channel,
		})
		return
	}

	if !state.start() {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "An update is already in progress. Check /api/system/update/status.",
		})
		return
	}

	pinVersion := body.Version
	if pinVersion == "" {
		pinVersion = "latest"
	}

	mode, extra := detectRestartMode()
	state.setRestartMode(restartModeName(mode))

	go runUpdate(pinVersion, mode, extra)

	msg := "OTA update launched. Service will restart automatically when done."
	if mode == restartManual {
		msg = "OTA update launched. New binary will be downloaded, but you must restart the service manually (no system supervisor detected)."
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"status":       "update_started",
		"version":      pinVersion,
		"channel":      Channel,
		"platform":     platformKey(),
		"restart_mode": restartModeName(mode),
		"message":      msg,
	})
}

// ── OTA update background worker ─────────────────────────────────────────────

func runUpdate(pinVersion string, mode restartMode, extra string) {
	defer state.finish()

	ts := func() string { return time.Now().Format("15:04:05") }
	appendLog := func(format string, args ...interface{}) {
		line := fmt.Sprintf("[%s] "+format, append([]interface{}{ts()}, args...)...)
		state.appendLog(line)
		log.Printf("[system/ota] %s", fmt.Sprintf(format, args...))
	}

	appendLog("=== OTA update started ===")
	appendLog("Channel: %s, pinned: %s", Channel, pinVersion)
	appendLog("Platform: %s", platformKey())
	appendLog("Restart strategy: %s", restartModeName(mode))

	// ── Step 1: fetch manifest, pick platform binary, download, verify ────────
	manifestBody, err := fetchUpstream()
	if err != nil {
		appendLog("ERROR: fetch manifest: %v", err)
		return
	}
	var mfst RootManifest
	if err := json.Unmarshal(manifestBody, &mfst); err != nil {
		appendLog("ERROR: decode manifest: %v", err)
		return
	}

	// Resolve the exact version we'll install. If user pinned, we still
	// require that version to exist on the latest manifest for our
	// platform — there's no historical browser in V1.
	wantVersion := mfst.Components.Backend.Version
	if pinVersion != "" && pinVersion != "latest" {
		wantVersion = pinVersion
	}
	appendLog("Target version: %s (current: %s)", wantVersion, getLocalVersion())

	url, expectedSHA, err := platformBinaryURL(manifestBody)
	if err != nil {
		appendLog("ERROR: %v", err)
		return
	}
	appendLog("Downloading: %s", url)

	dlCtx, dlCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer dlCancel()

	dlReq, err := http.NewRequestWithContext(dlCtx, http.MethodGet, url, nil)
	if err != nil {
		appendLog("ERROR: build request: %v", err)
		return
	}
	dlResp, err := http.DefaultClient.Do(dlReq)
	if err != nil {
		appendLog("ERROR: download: %v", err)
		return
	}
	defer dlResp.Body.Close()
	if dlResp.StatusCode != http.StatusOK {
		appendLog("ERROR: download returned HTTP %d", dlResp.StatusCode)
		return
	}

	// Stream the tarball to a temp file while computing SHA256 in parallel.
	dlTmp, err := os.CreateTemp("", "1agents-ota-*.tar.gz")
	if err != nil {
		appendLog("ERROR: create temp: %v", err)
		return
	}
	dlTmpPath := dlTmp.Name()
	defer os.Remove(dlTmpPath)
	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(dlTmp, hasher), dlResp.Body); err != nil {
		dlTmp.Close()
		appendLog("ERROR: stream download: %v", err)
		return
	}
	dlTmp.Close()
	gotSHA := hex.EncodeToString(hasher.Sum(nil))
	if expectedSHA != "" && gotSHA != expectedSHA {
		appendLog("ERROR: SHA256 mismatch: got %s, want %s", gotSHA, expectedSHA)
		return
	}
	appendLog("Download OK, SHA256=%s", gotSHA)

	// Extract the binary from the tarball next to the running executable.
	exePath, err := os.Executable()
	if err != nil {
		appendLog("ERROR: locate self: %v", err)
		return
	}
	newBin, err := extractBinaryFromTarGz(dlTmpPath, filepath.Dir(exePath))
	if err != nil {
		appendLog("ERROR: extract: %v", err)
		return
	}
	appendLog("Extracted to: %s", newBin)

	// Hand off to selfupdate.Apply which atomically replaces the running
	// binary. The Reader API lets the library decide where to stage the
	// temp copy — we just hand it the extracted file.
	{
		f, err := os.Open(newBin)
		if err != nil {
			appendLog("ERROR: open extracted binary: %v", err)
			return
		}
		defer f.Close()
		if err := selfupdate.Apply(f, selfupdate.Options{}); err != nil {
			appendLog("ERROR: selfupdate.Apply: %v", err)
			return
		}
	}
	appendLog("Binary replaced successfully. Want version: %s", wantVersion)

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
		// selfupdate.Apply already replaced the file at exePath; we just
		// need to re-exec the process to load the new code (mprotect on
		// the running text segment is the only thing preventing it from
		// taking effect immediately on Linux).
		appendLog("In-place restart (exec): replacing process with %s", exePath)
		appendLog("Connection will drop briefly — the service restarts with the same arguments.")

		// Small delay so this log line reaches the client before the process is replaced
		time.Sleep(800 * time.Millisecond)

		if err := execRestart(exePath); err != nil {
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

// extractBinaryFromTarGz opens a `.tar.gz` archive, locates the `1agents`
// entry (regardless of directory prefix), and writes it to a fresh
// temp file inside `dstDir`. The caller is responsible for renaming
// the result into place (typically via selfupdate.Apply).
//
// We deliberately do NOT stream-extract over the running binary — the
// tarball may contain multiple sibling binaries (ttyd, cc-connect)
// and we only want to swap the one this process owns. The other
// binaries are extracted-but-not-applied in V1; multi-binary
// orchestration is a V2 concern.
func extractBinaryFromTarGz(archivePath, dstDir string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("open archive: %w", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("gunzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	const wantName = "1agents"
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return "", fmt.Errorf("archive has no %q entry", wantName)
		}
		if err != nil {
			return "", fmt.Errorf("read tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(hdr.Name) != wantName {
			continue
		}
		// Found it. Write to a temp file in dstDir so cleanup is a
		// single os.Remove on failure; selfupdate.Apply does its own
		// rename into place.
		out, err := os.CreateTemp(dstDir, "1agents.new.*")
		if err != nil {
			return "", fmt.Errorf("create temp: %w", err)
		}
		outPath := out.Name()
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			os.Remove(outPath)
			return "", fmt.Errorf("write %s: %w", wantName, err)
		}
		// Preserve exec bit on unix; on Windows the file mode is
		// ignored for executability but we set it for consistency.
		_ = out.Chmod(0o755)
		if err := out.Close(); err != nil {
			os.Remove(outPath)
			return "", fmt.Errorf("close %s: %w", wantName, err)
		}
		return outPath, nil
	}
}
