package config

import (
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Config holds all runtime configuration for the 1agent daemon.
type Config struct {
	// ListenAddr is the address the Go gateway listens on externally.
	// Example: ":8080"
	ListenAddr string

	// TtydAddr is the address ttyd listens on locally (127.0.0.1 only).
	// Example: "127.0.0.1:7681"
	TtydAddr string

	// TtydBinaryPath is the path to the ttyd executable.
	// Example: "./ttyd"
	TtydBinaryPath string

	// TtydArgs are extra arguments passed to ttyd after the port/bind flags.
	// Example: []string{"bash"}
	TtydArgs []string

	// TmuxSession is the tmux session name used for terminal persistence.
	TmuxSession string

	// WorkDir is the root directory exposed by the file system API.
	// The API will refuse to serve files outside this directory.
	WorkDir string

	// StaticDir is the directory containing the compiled frontend assets.
	// Example: "./frontend/dist"
	StaticDir string

	// RestartDelay is how long the supervisor waits before restarting ttyd
	// after an unexpected exit.
	RestartDelay time.Duration

	// MaxRestarts is the maximum number of consecutive restarts allowed.
	// Once exceeded, the supervisor gives up to prevent infinite loops.
	MaxRestarts int

	// EnableTunnel determines if the public tunnel is automatically started on boot.
	EnableTunnel bool

	// TunnelToken stores the active session authentication token in memory.
	TunnelToken string

	// HarnessKitMode controls how the extension daemon is reached. "supervised"
	// starts the bundled hk binary; "external" connects to a development daemon.
	HarnessKitMode string

	// HarnessKitHost is fixed to loopback in supervised mode.
	HarnessKitHost string

	// HarnessKitPort is the daemon port. Zero asks the supervisor to allocate an
	// available loopback port for every child start.
	HarnessKitPort int

	// HarnessKitBinaryPath is an explicit path to the hk executable. When empty,
	// the supervisor discovers an adjacent, packaged, or development binary.
	HarnessKitBinaryPath string

	// HarnessKitDataDir is the root for HarnessKit's SQLite database and state.
	HarnessKitDataDir string

	// HarnessKitToken authenticates an external development daemon. Supervised
	// mode ignores this value and generates a fresh in-memory token per start.
	HarnessKitToken string

	// CoffeeAddr is the loopback address for the Alipay coffee payment service.
	CoffeeAddr string

	// CoffeeNodeBinaryPath is the Node.js executable used by the payment service.
	CoffeeNodeBinaryPath string

	// CoffeeSourceDir is the @1agents/alipay-coffee package root.
	// When empty, the supervisor discovers the module automatically.
	CoffeeSourceDir string

	// OTA configures the over-the-air self-update behaviour.
	// When Enabled is false the /api/system/update endpoint still accepts
	// requests but reports "disabled".
	OTA OTAConfig
}

// OTAConfig holds settings for the GitHub-Releases-based self-updater.
type OTAConfig struct {
	// Enabled controls whether the self-update machinery is active.
	// False in desktop mode (Tauri manages updates) and in Docker
	// (user manages updates via docker pull).
	Enabled bool
}

// Default returns a Config populated with safe default values.
func Default() *Config {
	home := os.Getenv("ONEAGENTS_HOME")
	if home == "" {
		if resolved, err := os.UserHomeDir(); err == nil {
			home = resolved
		} else {
			home = "."
		}
	}

	cfg := &Config{
		ListenAddr:           ":38080",
		TtydAddr:             "127.0.0.1:37681",
		TtydBinaryPath:       "./ttyd",
		HarnessKitMode:       "supervised",
		HarnessKitHost:       "127.0.0.1",
		HarnessKitPort:       0,
		HarnessKitDataDir:    filepath.Join(home, ".1agents", "harnesskit"),
		CoffeeAddr:           "127.0.0.1:38087",
		CoffeeNodeBinaryPath: "node",
		WorkDir:              "~",
		StaticDir:            "./frontend/dist",
		RestartDelay:         3 * time.Second,
		MaxRestarts:          5,
	}

	if runtime.GOOS == "windows" {
		cfg.TtydArgs = []string{"powershell.exe"}
		cfg.TmuxSession = ""
	} else {
		cfg.TtydArgs = []string{"tmux", "new-session", "-A", "-s", "1agents"}
		cfg.TmuxSession = "1agents"
	}

	return cfg
}
