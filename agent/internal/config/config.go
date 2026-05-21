package config

import "time"

// Config holds all runtime configuration for the remote-agent daemon.
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
	// Example: "./html/dist"
	StaticDir string

	// RestartDelay is how long the supervisor waits before restarting ttyd
	// after an unexpected exit.
	RestartDelay time.Duration

	// MaxRestarts is the maximum number of consecutive restarts allowed.
	// Once exceeded, the supervisor gives up to prevent infinite loops.
	MaxRestarts int
}

// Default returns a Config populated with safe default values.
func Default() *Config {
	return &Config{
		ListenAddr:     ":8080",
		TtydAddr:       "127.0.0.1:7681",
		TtydBinaryPath: "./ttyd",
		TtydArgs:       []string{"tmux", "new-session", "-A", "-s", "remote-agents"},
			TmuxSession:    "remote-agents",
		WorkDir:        ".",
		StaticDir:      "./html/dist",
		RestartDelay:   3 * time.Second,
		MaxRestarts:    5,
	}
}
