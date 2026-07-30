package config

import (
	"path/filepath"
	"testing"
)

func TestDefaultHarnessKitConfiguration(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ONEAGENTS_HOME", home)

	cfg := Default()
	if cfg.HarnessKitMode != "supervised" {
		t.Fatalf("HarnessKitMode = %q", cfg.HarnessKitMode)
	}
	if cfg.HarnessKitHost != "127.0.0.1" {
		t.Fatalf("HarnessKitHost = %q", cfg.HarnessKitHost)
	}
	if cfg.HarnessKitPort != 0 {
		t.Fatalf("HarnessKitPort = %d, want dynamic port 0", cfg.HarnessKitPort)
	}
	wantDir := filepath.Join(home, ".1agents", "harnesskit")
	if cfg.HarnessKitDataDir != wantDir {
		t.Fatalf("HarnessKitDataDir = %q, want %q", cfg.HarnessKitDataDir, wantDir)
	}
}
