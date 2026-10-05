// Package ffmpegx runs FFmpeg as a child process with live progress parsing
// and process-level pause/resume/cancel.
//
// Architectural red line (plan §15, authorization spec): the runner has zero
// dependencies on account/license/network. It only talks to the local OS.
package ffmpegx

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"videodelite/internal/proc"
)

// Progress is the normalized task progress parsed from `-progress pipe:1`
// (plan §91: frame / fps / bitrate / total_size / out_time / speed).
type Progress struct {
	Frame       int64   `json:"frame"`
	FPS         float64 `json:"fps"`
	BitrateKbps float64 `json:"bitrateKbps"`
	TotalSize   int64   `json:"totalSize"`
	OutTimeSec  float64 `json:"outTimeSec"`
	Speed       float64 `json:"speed"`
}

type Run struct {
	cmd        *exec.Cmd
	onProgress func(Progress)
	stderrSink func(string)

	mu        sync.Mutex
	suspended bool
	killed    bool
	errTail   []string
	job       uintptr // windows job object handle (0 elsewhere)

	doneCh chan error
}

// FindTool resolves an FFmpeg-family binary: explicit override, bundled
// next to the executable (installer layout), then PATH.
func FindTool(name string, override string) (string, error) {
	if override != "" {
		if _, err := os.Stat(override); err == nil {
			return override, nil
		}
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		for _, c := range []string{
			filepath.Join(dir, "bin", name+".exe"),
			filepath.Join(dir, name+".exe"),
		} {
			if _, err := os.Stat(c); err == nil {
				return c, nil
			}
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	if p, err := exec.LookPath(name + ".exe"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("%s not found (bundle it or add it to PATH)", name)
}

// Start launches FFmpeg. onProgress fires once per progress block,
// stderrSink once per stderr line.
func Start(bin string, args []string, onProgress func(Progress), stderrSink func(string)) (*Run, error) {
	full := append(append([]string{}, args...), "-progress", "pipe:1", "-nostats", "-loglevel", "warning")
	cmd := proc.Command(bin, full...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	r := &Run{
		cmd:        cmd,
		onProgress: onProgress,
		stderrSink: stderrSink,
		doneCh:     make(chan error, 1),
	}
	// Crash safety: tie the child's lifetime to ours (plan §116). The job
	// handle is intentionally never closed — Windows kills the child when
	// this process exits, which is exactly the desired behavior.
	if job, err := assignJob(cmd); err == nil {
		r.mu.Lock()
		r.job = uintptr(job)
		r.mu.Unlock()
	}
	go r.pumpStdout(stdout)
	go r.pumpStderr(stderr)
	go func() {
		r.doneCh <- cmd.Wait()
	}()
	return r, nil
}

func (r *Run) pumpStdout(rc io.ReadCloser) {
	sc := bufio.NewScanner(rc)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	var p Progress
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		eq := strings.Index(line, "=")
		if eq < 0 {
			continue
		}
		key, val := line[:eq], strings.TrimSpace(line[eq+1:])
		switch key {
		case "frame":
			p.Frame, _ = strconv.ParseInt(val, 10, 64)
		case "fps":
			p.FPS, _ = strconv.ParseFloat(val, 64)
		case "bitrate":
			p.BitrateKbps = parseBitrateKbps(val)
		case "total_size":
			p.TotalSize, _ = strconv.ParseInt(val, 10, 64)
		case "out_time_us":
			if us, err := strconv.ParseInt(val, 10, 64); err == nil {
				p.OutTimeSec = float64(us) / 1e6
			}
		case "out_time_ms":
			// Historic quirk: ffmpeg once wrote microseconds here. Only
			// trust it when out_time_us was absent.
			if p.OutTimeSec == 0 {
				if ms, err := strconv.ParseInt(val, 10, 64); err == nil {
					p.OutTimeSec = float64(ms) / 1e6
				}
			}
		case "out_time":
			if p.OutTimeSec == 0 {
				p.OutTimeSec = parseTimecode(val)
			}
		case "speed":
			p.Speed = parseSpeed(val)
		case "progress":
			if r.onProgress != nil {
				r.onProgress(p)
			}
			if val == "end" {
				return
			}
			p = Progress{}
		}
	}
}

func (r *Run) pumpStderr(rc io.ReadCloser) {
	sc := bufio.NewScanner(rc)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		r.mu.Lock()
		r.errTail = append(r.errTail, line)
		if len(r.errTail) > 60 {
			r.errTail = r.errTail[len(r.errTail)-60:]
		}
		sink := r.stderrSink
		r.mu.Unlock()
		if sink != nil {
			sink(line)
		}
	}
}

// Wait blocks until FFmpeg exits and returns its error (nil on exit 0).
func (r *Run) Wait() error {
	return <-r.doneCh
}

// ErrTail returns the tail of stderr for classification.
func (r *Run) ErrTail() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.errTail))
	copy(out, r.errTail)
	return out
}

// Kill terminates the process (user cancel / task shutdown).
func (r *Run) Kill() error {
	r.mu.Lock()
	r.killed = true
	r.mu.Unlock()
	if r.cmd.Process != nil {
		return r.cmd.Process.Kill()
	}
	return nil
}

func (r *Run) Killed() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.killed
}

func parseBitrateKbps(v string) float64 {
	v = strings.TrimSpace(v)
	mult := 1.0
	switch {
	case strings.HasSuffix(v, "Mbit/s"):
		mult = 1000
		v = strings.TrimSuffix(v, "Mbit/s")
	case strings.HasSuffix(v, "kbits/s"):
		mult = 1
		v = strings.TrimSuffix(v, "kbits/s")
	}
	f, _ := strconv.ParseFloat(strings.TrimSpace(v), 64)
	return f * mult
}

func parseSpeed(v string) float64 {
	v = strings.TrimSuffix(strings.TrimSpace(v), "x")
	f, _ := strconv.ParseFloat(v, 64)
	return f
}

// parseTimecode handles HH:MM:SS.microseconds.
func parseTimecode(v string) float64 {
	parts := strings.Split(v, ":")
	if len(parts) != 3 {
		return 0
	}
	h, _ := strconv.ParseFloat(parts[0], 64)
	m, _ := strconv.ParseFloat(parts[1], 64)
	s, _ := strconv.ParseFloat(parts[2], 64)
	return h*3600 + m*60 + s
}
