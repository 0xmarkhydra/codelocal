//go:build windows

package osutil

import "os"

func ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	_, err := os.FindProcess(pid)
	return err == nil
}
