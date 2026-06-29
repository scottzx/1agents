package system

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

// defaultRelayURL is the 1agents production relay ("中转") base. It overrides
// happy-cli's upstream default (api.cluster-fluster.com) — that submodule is a
// clean upstream blueprint we steer via HAPPY_SERVER_URL, not by patching it.
const defaultRelayURL = "https://agents.dreammate.work"

// happyHome returns the ~/.happy directory path.
func happyHome() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".happy"
	}
	return filepath.Join(home, ".happy")
}

// resolveHappy locates the happy CLI and returns the command name plus any
// leading args needed to invoke it. Resolution order:
//  1. `happy` on PATH (global install / `npm link`)
//  2. the bundled happy-cli's Node entrypoint, run via node — the layout that
//     every release channel ships (build-happy-bundle.sh puts happy-cli/ next
//     to the daemon binary)
//  3. the happy-cli submodule's Node entrypoint — the in-repo dev layout where
//     the daemon runs as build/1agents
//
// ok is false when happy cannot be located by any strategy.
func resolveHappy() (name string, lead []string, ok bool) {
	if p, err := exec.LookPath("happy"); err == nil {
		return p, nil, true
	}
	exe, err := os.Executable()
	if err != nil {
		return "", nil, false
	}
	binDir := filepath.Dir(exe)

	// happy.mjs is a thin launcher that re-execs node on dist/index.mjs, so we
	// run it through an explicit node (covers Tauri, where node is bundled and
	// not on PATH). Bundled layout first, then the in-repo submodule.
	for _, mjs := range []string{
		filepath.Join(binDir, "happy-cli", "bin", "happy.mjs"),                  // release bundle
		filepath.Join(binDir, "..", "modules", "happy-cli", "bin", "happy.mjs"), // repo dev
	} {
		if fileExists(mjs) {
			if node, ok := resolveNode(binDir); ok {
				return node, []string{mjs}, true
			}
		}
	}
	return "", nil, false
}

// resolveNode finds a node runtime: PATH first, then the runtime bundled by the
// Tauri desktop build (resources/runtime/node/bin/node, sibling of the bin/ dir).
func resolveNode(binDir string) (string, bool) {
	if p, err := exec.LookPath("node"); err == nil {
		return p, true
	}
	bundled := filepath.Join(binDir, "..", "runtime", "node", "bin", "node")
	if isExecutableFile(bundled) {
		return bundled, true
	}
	return "", false
}

// happyAdapterEntry returns the path to the 1agents RPC adapter entrypoint that
// happy loads via HAPPY_RPC_ADAPTER_ENTRY, or "" if it is not bundled. Mirrors
// the resolution layouts in resolveHappy (release bundle, then repo dev).
func happyAdapterEntry() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	binDir := filepath.Dir(exe)
	for _, p := range []string{
		filepath.Join(binDir, "adapter", "rpc", "index.mjs"),       // release bundle
		filepath.Join(binDir, "..", "adapter", "rpc", "index.mjs"), // repo dev
	} {
		if fileExists(p) {
			return p
		}
	}
	return ""
}

// happySettingsServerURL returns the serverUrl pinned in ~/.happy/settings.json,
// or "" if absent. Used to honor a paired relay before falling back to the
// 1agents default (matching happy-cli's env → settings.json → default order).
func happySettingsServerURL() string {
	data, err := os.ReadFile(filepath.Join(happyHome(), "settings.json"))
	if err != nil {
		return ""
	}
	var s struct {
		ServerURL string `json:"serverUrl"`
	}
	if json.Unmarshal(data, &s) != nil {
		return ""
	}
	return s.ServerURL
}

// isExecutableFile reports whether path is a regular file with an executable bit.
func isExecutableFile(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular() && fi.Mode().Perm()&0o111 != 0
}

// fileExists reports whether path exists and is a regular file.
func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular()
}

// HappyStatusResponse is the JSON shape returned by GET /api/system/happy/status.
type HappyStatusResponse struct {
	Running   bool   `json:"running"`
	Pid       int    `json:"pid,omitempty"`
	StartedAt string `json:"startedAt,omitempty"`
	// relay / credentials fields (empty when not logged in)
	ServerURL  string `json:"serverUrl,omitempty"`
	Token      string `json:"token,omitempty"`
	MachineKey string `json:"machineKey,omitempty"` // base64 AES key for E2EE
	PublicKey  string `json:"publicKey,omitempty"`  // base64 NaCl public key
	MachineID  string `json:"machineId,omitempty"`  // relay machine entity id (RPC address)
	Hostname   string `json:"hostname,omitempty"`   // mDNS / local hostname, seeds the client device name
}

// deviceHostname returns a human-friendly local device name to seed the client's
// device label. On macOS this is the mDNS LocalHostName (e.g. "MacBook-Air-8");
// elsewhere it falls back to the short OS hostname. The client may override it
// with its own alias.
func deviceHostname() string {
	if runtime.GOOS == "darwin" {
		if out, err := exec.Command("scutil", "--get", "LocalHostName").Output(); err == nil {
			if name := strings.TrimSpace(string(out)); name != "" {
				return name
			}
		}
	}
	if h, err := os.Hostname(); err == nil {
		if i := strings.IndexByte(h, '.'); i > 0 {
			return h[:i]
		}
		return h
	}
	return ""
}

// HappyStatus handles GET /api/system/happy/status.
// Reads ~/.happy/daemon.state.json and ~/.happy/access.key and returns the
// combined status. Only called from localhost so returning raw credentials is safe.
func (h *Handler) HappyStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp := HappyStatusResponse{Hostname: deviceHostname()}

	// ── daemon state ──────────────────────────────────────────────────────────
	stateFile := filepath.Join(happyHome(), "daemon.state.json")
	if data, err := os.ReadFile(stateFile); err == nil {
		var state struct {
			Pid       int    `json:"pid"`
			StartTime string `json:"startTime"`
		}
		if json.Unmarshal(data, &state) == nil && state.Pid > 0 {
			// check process is alive
			proc, err := os.FindProcess(state.Pid)
			if err == nil && proc.Signal(syscall.Signal(0)) == nil {
				resp.Running = true
				resp.Pid = state.Pid
				resp.StartedAt = state.StartTime
			}
		}
	}

	// ── credentials (access.key) ──────────────────────────────────────────────
	keyFile := filepath.Join(happyHome(), "access.key")
	if data, err := os.ReadFile(keyFile); err == nil {
		var creds struct {
			Token      string `json:"token"`
			Encryption *struct {
				PublicKey  string `json:"publicKey"`
				MachineKey string `json:"machineKey"`
			} `json:"encryption"`
		}
		if json.Unmarshal(data, &creds) == nil {
			resp.Token = creds.Token
			if creds.Encryption != nil {
				resp.MachineKey = creds.Encryption.MachineKey
				resp.PublicKey = creds.Encryption.PublicKey
			}
		}
	}

	// ── settings (serverUrl) ─────────────────────────────────────────────────
	settingsFile := filepath.Join(happyHome(), "settings.json")
	if data, err := os.ReadFile(settingsFile); err == nil {
		var settings struct {
			ServerURL string `json:"serverUrl"`
			MachineID string `json:"machineId"`
		}
		if json.Unmarshal(data, &settings) == nil {
			resp.ServerURL = settings.ServerURL
			resp.MachineID = settings.MachineID
		}
	}

	// Resolve serverUrl the same way the happy CLI does (configuration.ts):
	// HAPPY_SERVER_URL env → settings.json → built-in default. Without this the
	// client QR bundle may lack the relay address and be unconnectable.
	if resp.ServerURL == "" {
		if env := os.Getenv("HAPPY_SERVER_URL"); env != "" {
			resp.ServerURL = env
		} else {
			resp.ServerURL = defaultRelayURL
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(resp)
}

// HappyDaemonStart handles POST /api/system/happy/daemon/start.
// Finds the `happy` binary and launches `happy daemon start` (which itself
// spawns a detached start-sync child and exits immediately).
func (h *Handler) HappyDaemonStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name, lead, ok := resolveHappy()
	if !ok {
		jsonError(w, "happy CLI not found — build it with `make happy` or install it on PATH", http.StatusServiceUnavailable)
		return
	}

	args := append(append([]string{}, lead...), "daemon", "start")
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // detach from Go's process group
	// Wire happy to the 1agents RPC glue when it ships alongside, unless the
	// environment already pins it. happy reads HAPPY_RPC_ADAPTER_ENTRY itself.
	cmd.Env = os.Environ()
	if _, set := os.LookupEnv("HAPPY_RPC_ADAPTER_ENTRY"); !set {
		if adapter := happyAdapterEntry(); adapter != "" {
			cmd.Env = append(cmd.Env, "HAPPY_RPC_ADAPTER_ENTRY="+adapter)
		}
	}
	// Pin the relay so the daemon connects to the 1agents relay, not happy-cli's
	// upstream default. Precedence matches happy-cli (env → settings.json →
	// default): only inject when neither an env nor a paired settings.json set it.
	if _, set := os.LookupEnv("HAPPY_SERVER_URL"); !set && happySettingsServerURL() == "" {
		cmd.Env = append(cmd.Env, "HAPPY_SERVER_URL="+defaultRelayURL)
	}
	if err := cmd.Start(); err != nil {
		jsonError(w, "failed to start daemon: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// We don't Wait() — the child detaches itself and exits the parent quickly.
	go func() { _ = cmd.Wait() }()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	fmt.Fprintf(w, `{"ok":true}`)
}

// HappyDaemonStop handles POST /api/system/happy/daemon/stop.
// Reads the PID from ~/.happy/daemon.state.json and sends SIGTERM.
func (h *Handler) HappyDaemonStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stateFile := filepath.Join(happyHome(), "daemon.state.json")
	data, err := os.ReadFile(stateFile)
	if err != nil {
		jsonError(w, "daemon state file not found — is daemon running?", http.StatusNotFound)
		return
	}

	var state struct {
		Pid int `json:"pid"`
	}
	if err := json.Unmarshal(data, &state); err != nil || state.Pid <= 0 {
		jsonError(w, "invalid daemon state file", http.StatusInternalServerError)
		return
	}

	proc, err := os.FindProcess(state.Pid)
	if err != nil {
		jsonError(w, "process not found: "+err.Error(), http.StatusNotFound)
		return
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		// Already dead?
		if strings.Contains(err.Error(), "process already finished") ||
			strings.Contains(err.Error(), "no such process") {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			fmt.Fprintf(w, `{"ok":true,"note":"process was already stopped"}`)
			return
		}
		jsonError(w, "failed to stop daemon: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	fmt.Fprintf(w, `{"ok":true,"pid":%s}`, strconv.Itoa(state.Pid))
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	fmt.Fprintf(w, `{"error":%q}`, msg)
}
