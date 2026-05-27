//go:build linux

package system

import "syscall"

// detachSysProcAttr returns a SysProcAttr that fully detaches the child process
// from the current process group, ensuring it outlives its parent on Linux.
func detachSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setsid: true, // Create a new session — child becomes process group leader
	}
}
