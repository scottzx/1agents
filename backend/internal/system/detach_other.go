//go:build !linux

package system

import "syscall"

// detachSysProcAttr returns a SysProcAttr for non-Linux platforms.
// On macOS/Windows, simply return an empty struct (no Setsid support).
func detachSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}
