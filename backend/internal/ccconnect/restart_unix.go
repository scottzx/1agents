//go:build !windows

package ccconnect

import (
	"os"
	"syscall"
)

func restartProcess(execPath string) error {
	return syscall.Exec(execPath, os.Args, os.Environ())
}
