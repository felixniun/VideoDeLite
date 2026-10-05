//go:build windows

package ffmpegx

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modkernel32     = windows.NewLazySystemDLL("kernel32.dll")
	procSuspendThread = modkernel32.NewProc("SuspendThread")
	procResumeThread  = modkernel32.NewProc("ResumeThread")
)

// Suspend pauses the FFmpeg process by suspending its threads.
// Pause/Resume is process-level control, not a checkpoint (plan §37).
func (r *Run) Suspend() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.suspended {
		return nil
	}
	if err := setThreads(r.cmd.Process.Pid, true); err != nil {
		return err
	}
	r.suspended = true
	return nil
}

// Resume un-pauses the FFmpeg process.
func (r *Run) Resume() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.suspended {
		return nil
	}
	if err := setThreads(r.cmd.Process.Pid, false); err != nil {
		return err
	}
	r.suspended = false
	return nil
}

func setThreads(pid int, suspend bool) error {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return fmt.Errorf("thread snapshot: %w", err)
	}
	defer windows.CloseHandle(snap)

	var te windows.ThreadEntry32
	te.Size = uint32(unsafe.Sizeof(te))
	err = windows.Thread32First(snap, &te)
	if err != nil {
		return fmt.Errorf("thread enum: %w", err)
	}
	target := uint32(pid)
	count := 0
	for {
		if te.OwnerProcessID == target {
			h, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, te.ThreadID)
			if err == nil {
				if suspend {
					procSuspendThread.Call(uintptr(h))
				} else {
					procResumeThread.Call(uintptr(h))
				}
				windows.CloseHandle(h)
				count++
			}
		}
		if err := windows.Thread32Next(snap, &te); err != nil {
			break
		}
	}
	if count == 0 {
		return fmt.Errorf("no threads found for pid %d", pid)
	}
	return nil
}

var _ = syscall.InvalidHandle
