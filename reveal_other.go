//go:build !windows

package main

import "os/exec"

func openFolder(folder string) error {
	return exec.Command("xdg-open", folder).Start()
}
