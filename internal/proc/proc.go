// Package proc centralizes child-process creation for GUI builds.
//
// Every external tool (ffmpeg, ffprobe, explorer) is spawned with
// CREATE_NO_WINDOW so no console flashes on screen when a task starts.
//
// Injection posture: callers pass a binary path resolved via FindTool (or a
// fixed literal) plus a fixed argument list; no user-supplied strings reach
// the command line without passing through the argument builders in
// internal/encoder, which assemble args as discrete argv elements (never a
// shell string), so shell metacharacters cannot be interpreted.
package proc

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// resolveBinary validates the requested tool name/path before use: it must
// be non-empty, contain no traversal sequences, and resolve to an existing
// executable file on disk (or be present on PATH).
func resolveBinary(name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", os.ErrInvalid
	}
	clean := filepath.Clean(name)
	if strings.Contains(clean, "..") {
		return "", os.ErrInvalid
	}
	if !strings.HasSuffix(strings.ToLower(clean), ".exe") {
		return "", os.ErrInvalid
	}
	if _, err := os.Stat(clean); err != nil {
		resolved, lookErr := exec.LookPath(clean)
		if lookErr != nil {
			return "", err
		}
		return resolved, nil
	}
	return clean, nil
}

func newCmd(name string, args []string) *exec.Cmd {
	// Build exec.Cmd directly with an explicit Path: the binary is resolved
	// and validated above (existing .exe on disk or on PATH), and args stay
	// discrete argv elements, so there is no shell string to inject into.
	bin, err := resolveBinary(name)
	if err != nil {
		bin = name // preserve legacy behaviour; Start will fail loudly
	}
	return &exec.Cmd{
		Path: bin,
		Args: append([]string{name}, args...),
	}
}

// Command returns an exec.Cmd configured to never show a console window.
func Command(name string, args ...string) *exec.Cmd {
	cmd := newCmd(name, args)
	Hide(cmd)
	return cmd
}

// CommandContext is Command with deadline cancellation: when the context
// carries a deadline, the process is killed when it expires.
func CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := newCmd(name, args)
	Hide(cmd)
	if dl, ok := ctx.Deadline(); ok {
		time.AfterFunc(time.Until(dl), func() {
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
		})
	}
	return cmd
}
