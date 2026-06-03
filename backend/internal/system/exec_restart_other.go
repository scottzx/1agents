//go:build !unix

package system

import "fmt"

// execRestart is not supported on non-Unix platforms (e.g. Windows).
func execRestart(_ string) error {
	return fmt.Errorf("exec restart is not supported on this platform")
}

func canExecRestart() bool { return false }
