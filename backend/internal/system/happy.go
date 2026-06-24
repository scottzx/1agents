package system

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// happyHome returns the ~/.happy directory path.
func happyHome() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".happy"
	}
	return filepath.Join(home, ".happy")
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
}

// HappyStatus handles GET /api/system/happy/status.
// Reads ~/.happy/daemon.state.json and ~/.happy/access.key and returns the
// combined status. Only called from localhost so returning raw credentials is safe.
func (h *Handler) HappyStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp := HappyStatusResponse{}

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
		}
		if json.Unmarshal(data, &settings) == nil {
			resp.ServerURL = settings.ServerURL
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

	bin, err := exec.LookPath("happy")
	if err != nil {
		jsonError(w, "happy binary not found in PATH: "+err.Error(), http.StatusNotFound)
		return
	}

	cmd := exec.Command(bin, "daemon", "start")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // detach from Go's process group
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
