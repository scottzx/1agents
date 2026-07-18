//go:build unix

package system

import "syscall"

func happyDaemonSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}
