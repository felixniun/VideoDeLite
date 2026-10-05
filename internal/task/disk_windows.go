//go:build windows

package task

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

// diskFree reports available bytes on the volume containing dir.
func diskFree(dir string) (uint64, bool) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return 0, false
	}
	// Root of the volume, e.g. "D:\"
	root := filepath.VolumeName(abs) + string(filepath.Separator)
	var free, total, avail uint64
	p16, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return 0, false
	}
	if err := windows.GetDiskFreeSpaceEx(p16, &free, &total, &avail); err != nil {
		return 0, false
	}
	return free, true
}
