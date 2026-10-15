//go:build !windows

package lab

import (
	"syscall"
)

func getFreeDiskSpace(path string) uint64 {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err == nil {
		return stat.Bavail * uint64(stat.Bsize)
	}
	return 0
}
