//go:build windows

package main

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// openFolder asks the Windows shell to open a directory in the user's normal
// Explorer session. Launching explorer.exe as a hidden child can exit without
// creating a visible window, even when process creation succeeds.
func openFolder(folder string) error {
	verb, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	path, err := windows.UTF16PtrFromString(folder)
	if err != nil {
		return err
	}
	if err := windows.ShellExecute(0, verb, path, nil, nil, windows.SW_SHOWNORMAL); err != nil {
		return fmt.Errorf("无法打开文件夹 %q: %w", folder, err)
	}
	return nil
}
