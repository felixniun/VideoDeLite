//go:build !windows

package ffmpegx

import "os/exec"

// assignJob is Windows-only; other platforms rely on process trees.
func assignJob(cmd *exec.Cmd) (uintptr, error) { return 0, nil }
