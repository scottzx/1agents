package harnesskitmigration

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

type migrationLock struct {
	path string
}

type lockRecord struct {
	PID       int       `json:"pid"`
	Hostname  string    `json:"hostname"`
	StartedAt time.Time `json:"startedAt"`
}

func DefaultConfig() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, err
	}
	oneAgentsHome := os.Getenv("ONEAGENTS_HOME")
	if oneAgentsHome == "" {
		oneAgentsHome = home
	}
	base := filepath.Join(oneAgentsHome, ".1agents")
	return Config{
		Home:              home,
		OneAgentsHome:     oneAgentsHome,
		LegacyDir:         filepath.Join(base, "skill-manager"),
		HarnessKitDataDir: filepath.Join(base, "harnesskit"),
		BackupRoot:        filepath.Join(base, "migrations"),
		Now:               time.Now,
	}, nil
}

func normalizeConfig(cfg Config) Config {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.HKRunner == nil {
		cfg.HKRunner = runHarnessKitCLI
	}
	return cfg
}

func validateConfigPaths(cfg Config) error {
	paths := []struct {
		name string
		path string
	}{
		{name: "home", path: cfg.Home},
		{name: "oneagents home", path: cfg.OneAgentsHome},
		{name: "legacy directory", path: cfg.LegacyDir},
		{name: "HarnessKit data directory", path: cfg.HarnessKitDataDir},
		{name: "backup root", path: cfg.BackupRoot},
	}
	for _, candidate := range paths {
		if candidate.path == "" || !filepath.IsAbs(candidate.path) {
			return fmt.Errorf("%s must be an absolute path", candidate.name)
		}
	}
	for _, pair := range [][2]string{
		{cfg.LegacyDir, cfg.HarnessKitDataDir},
		{cfg.LegacyDir, cfg.BackupRoot},
		{cfg.HarnessKitDataDir, cfg.BackupRoot},
	} {
		left := filepath.Clean(pair[0])
		right := filepath.Clean(pair[1])
		if pathWithin(left, right) || pathWithin(right, left) {
			return fmt.Errorf("migration data paths must not overlap: %s and %s", left, right)
		}
	}
	return nil
}

func acquireLock(cfg Config) (*migrationLock, error) {
	migrationDir := filepath.Join(cfg.HarnessKitDataDir, "migrations")
	if err := os.MkdirAll(migrationDir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(migrationDir, lockFileName)
	hostname, _ := os.Hostname()
	record := lockRecord{PID: os.Getpid(), Hostname: hostname, StartedAt: cfg.Now().UTC()}
	payload, _ := json.Marshal(record)
	for attempt := 0; attempt < 2; attempt++ {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			if _, err := file.Write(append(payload, '\n')); err != nil {
				_ = file.Close()
				_ = os.Remove(path)
				return nil, err
			}
			if err := file.Sync(); err != nil {
				_ = file.Close()
				_ = os.Remove(path)
				return nil, err
			}
			if err := file.Close(); err != nil {
				_ = os.Remove(path)
				return nil, err
			}
			return &migrationLock{path: path}, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
		var existing lockRecord
		if readErr := readJSON(path, &existing); readErr != nil {
			return nil, fmt.Errorf("migration lock exists and cannot be validated: %w", readErr)
		}
		if existing.Hostname != hostname {
			return nil, fmt.Errorf("migration lock belongs to host %q (pid %d)", existing.Hostname, existing.PID)
		}
		if processAlive(existing.PID) {
			return nil, fmt.Errorf("migration already running in pid %d", existing.PID)
		}
		if removeErr := os.Remove(path); removeErr != nil {
			return nil, fmt.Errorf("remove stale migration lock: %w", removeErr)
		}
	}
	return nil, errors.New("unable to acquire migration lock")
}

func (lock *migrationLock) release() {
	if lock != nil {
		_ = os.Remove(lock.path)
	}
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}

func ensureServicesStopped(cfg Config, inspectLegacy bool) error {
	statePaths := []string{filepath.Join(cfg.OneAgentsHome, ".1agents", "daemon.json")}
	if inspectLegacy {
		statePaths = append(statePaths, filepath.Join(cfg.LegacyDir, "runtime.json"))
	}
	for _, statePath := range statePaths {
		payload, err := os.ReadFile(statePath)
		if err != nil {
			continue
		}
		var state map[string]any
		if json.Unmarshal(payload, &state) != nil {
			continue
		}
		address := ""
		if value, ok := state["base_url"].(string); ok {
			address = strings.TrimPrefix(strings.TrimPrefix(value, "http://"), "https://")
		}
		if value, ok := state["listen_addr"].(string); ok {
			address = value
		}
		if address == "" {
			continue
		}
		if strings.HasPrefix(address, ":") {
			address = "127.0.0.1" + address
		}
		if strings.HasPrefix(address, "0.0.0.0:") {
			address = "127.0.0.1:" + strings.TrimPrefix(address, "0.0.0.0:")
		}
		if slash := strings.IndexByte(address, '/'); slash >= 0 {
			address = address[:slash]
		}
		connection, err := net.DialTimeout("tcp", address, 200*time.Millisecond)
		if err == nil {
			_ = connection.Close()
			return fmt.Errorf("service described by %s is still listening at %s; stop it before migration", statePath, address)
		}
	}
	return nil
}

func runHarnessKitCLI(ctx context.Context, cfg Config) error {
	binary, err := resolveHarnessKitBinary(cfg)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, binary, "--data-dir", cfg.HarnessKitDataDir, "status")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("HarnessKit scan/status failed: %w", err)
	}
	return nil
}

func resolveHarnessKitBinary(cfg Config) (string, error) {
	name := "hk"
	if runtime.GOOS == "windows" {
		name = "hk.exe"
	}
	if cfg.HarnessKitBinary != "" {
		if info, err := os.Stat(cfg.HarnessKitBinary); err == nil && info.Mode().IsRegular() {
			return cfg.HarnessKitBinary, nil
		}
		return "", fmt.Errorf("configured hk binary is unavailable")
	}
	if env := os.Getenv("ONEAGENTS_HARNESSKIT_BIN"); env != "" {
		if info, err := os.Stat(env); err == nil && info.Mode().IsRegular() {
			return env, nil
		}
		return "", fmt.Errorf("ONEAGENTS_HARNESSKIT_BIN is unavailable")
	}
	if self, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(self), name)
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return candidate, nil
		}
	}
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("hk binary not found; pass --hk-bin")
}

func runCLI(args []string, stdout, stderr io.Writer) int {
	cfg, err := DefaultConfig()
	if err != nil {
		fmt.Fprintln(stderr, "harnesskit migration:", err)
		return 1
	}
	flags := flag.NewFlagSet("1agents migrate harnesskit", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var planMode, applyMode, cleanStart bool
	var rollbackID string
	flags.BoolVar(&planMode, "plan", false, "read-only migration inventory")
	flags.BoolVar(&applyMode, "apply", false, "backup and apply the migration plan")
	flags.BoolVar(&cleanStart, "clean-start", false, "initialize HarnessKit without reading legacy data")
	flags.StringVar(&rollbackID, "data-rollback", "", "restore rewritten paths from a backup ID")
	flags.StringVar(&cfg.Home, "home", cfg.Home, "Agent home directory used for native extension paths")
	flags.StringVar(&cfg.OneAgentsHome, "oneagents-home", cfg.OneAgentsHome, "parent directory containing .1agents")
	flags.StringVar(&cfg.LegacyDir, "legacy-dir", cfg.LegacyDir, "legacy Skills-manager data directory")
	flags.StringVar(&cfg.HarnessKitDataDir, "harnesskit-data-dir", cfg.HarnessKitDataDir, "HarnessKit data directory")
	flags.StringVar(&cfg.BackupRoot, "backup-root", cfg.BackupRoot, "migration backup directory")
	flags.StringVar(&cfg.HarnessKitBinary, "hk-bin", "", "path to the hk executable")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "unexpected arguments: %s\n", strings.Join(flags.Args(), " "))
		return 2
	}
	modeCount := 0
	for _, selected := range []bool{planMode, applyMode, cleanStart, rollbackID != ""} {
		if selected {
			modeCount++
		}
	}
	if modeCount != 1 {
		fmt.Fprintln(stderr, "choose exactly one of --plan, --apply, --clean-start, or --data-rollback <backup-id>")
		return 2
	}
	cfg = normalizeConfig(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	var output any
	switch {
	case planMode:
		output, err = BuildPlan(cfg)
	case applyMode:
		output, err = Apply(ctx, cfg)
	case cleanStart:
		output, err = CleanStart(ctx, cfg)
	default:
		output, err = DataRollback(ctx, cfg, rollbackID)
	}
	if err != nil {
		fmt.Fprintln(stderr, "harnesskit migration:", err)
		return 1
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(output); err != nil {
		fmt.Fprintln(stderr, "harnesskit migration:", err)
		return 1
	}
	return 0
}
