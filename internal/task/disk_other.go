//go:build !windows

package task

func diskFree(dir string) (uint64, bool) { return 0, false }
