//go:build windows

package appimage

func isProcessDead(pid int) bool {
	return false
}
