//go:build unix

package system

import (
	"os"
	"os/exec"
	"syscall"
)

// execRestart replaces the current process image with the newly installed binary.
// This is the cross-platform restart mechanism for non-systemd environments (macOS, etc.).
// It preserves the same arguments so the new process starts with identical configuration.
func execRestart(newBinaryPath string) error {
	// Resolve the final absolute path (follow symlinks created by npm)
	resolved, err := exec.LookPath(newBinaryPath)
	if err != nil {
		resolved = newBinaryPath
	}
	// syscall.Exec replaces the current process image in-place.
	// The PID stays the same; the new binary starts with identical args and environment.
	return syscall.Exec(resolved, os.Args, os.Environ())
}

// canExecRestart reports whether in-place exec restart is supported on this platform.
func canExecRestart() bool { return true }
