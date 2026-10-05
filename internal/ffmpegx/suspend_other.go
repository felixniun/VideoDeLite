//go:build !windows

package ffmpegx

import "errors"

// Suspend is Windows-only in V1 (first platform is Windows, plan §4).
func (r *Run) Suspend() error { return errors.New("pause is not supported on this platform") }

func (r *Run) Resume() error { return errors.New("resume is not supported on this platform") }
