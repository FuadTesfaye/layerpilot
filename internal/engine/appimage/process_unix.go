//go:build !windows

package appimage

import (
	"errors"
	"syscall"
)

func isProcessDead(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err != nil && (errors.Is(err, syscall.ESRCH) || err == syscall.ESRCH)
}
