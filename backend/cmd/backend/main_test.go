package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPrependUserAgentBinDirs(t *testing.T) {
	home := filepath.Join(string(filepath.Separator), "Users", "test")
	localBin := filepath.Join(home, ".local", "bin")
	grokBin := filepath.Join(home, ".grok", "bin")
	base := strings.Join([]string{"/usr/bin", localBin, "/bin"}, string(filepath.ListSeparator))

	got := prependUserAgentBinDirs(base, home)
	want := strings.Join([]string{localBin, grokBin, "/usr/bin", "/bin"}, string(filepath.ListSeparator))
	if got != want {
		t.Fatalf("PATH = %q, want %q", got, want)
	}
}

func TestEnsureUserAgentBinDirsMakesClaudeDiscoverable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("user-local bin fallback is for Unix-like hosts")
	}

	home := t.TempDir()
	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	claudePath := filepath.Join(binDir, "claude")
	if err := os.WriteFile(claudePath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())
	ensureUserAgentBinDirs()

	got, err := exec.LookPath("claude")
	if err != nil {
		t.Fatalf("LookPath(claude): %v", err)
	}
	if got != claudePath {
		t.Fatalf("Claude path = %q, want %q", got, claudePath)
	}
}
