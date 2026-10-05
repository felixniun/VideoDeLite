// Package logging implements the local-only log pipeline.
//
// Frozen rules (plan §41–§44, §94–§95):
//   - logs never leave this machine (no auto upload)
//   - 7 day retention, auto cleanup
//   - 500 MB total cap, oldest deleted first
//   - 20 MB per-task cap: after the cap only key errors are kept,
//     high-frequency FFmpeg output is dropped
package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	retentionDays  = 7
	totalCapBytes  = 500 * 1024 * 1024
	taskCapBytes   = 20 * 1024 * 1024
	dayFormat      = "20060102"
	fileTimeFormat = "2006-01-02 15:04:05"
)

type Logger struct {
	mu  sync.Mutex
	dir string
	day string
	app *os.File
}

// New opens (or creates) today's application log and performs retention cleanup.
func New(dir string) *Logger {
	l := &Logger{dir: dir}
	l.openDay()
	Cleanup(dir)
	go l.cleanupLoop()
	return l
}

func (l *Logger) openDay() {
	day := time.Now().Format(dayFormat)
	if l.app != nil && l.day == day {
		return
	}
	if l.app != nil {
		l.app.Close()
	}
	f, err := os.OpenFile(filepath.Join(l.dir, "app-"+day+".log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		f = nil // never crash the app because of logging
	}
	l.day = day
	l.app = f
}

func (l *Logger) write(level, msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.openDay()
	if l.app == nil {
		return
	}
	fmt.Fprintf(l.app, "%s [%s] %s\n", time.Now().Format(fileTimeFormat), level, msg)
}

func (l *Logger) Info(msg string)  { l.write("INFO", msg) }
func (l *Logger) Warn(msg string)  { l.write("WARN", msg) }
func (l *Logger) Error(msg string) { l.write("ERROR", msg) }

// Dir returns the log directory (used for per-task log files).
func (l *Logger) Dir() string { return l.dir }

func (l *Logger) cleanupLoop() {
	t := time.NewTicker(time.Hour)
	for range t.C {
		Cleanup(l.dir)
	}
}

// TaskLog is a capped writer for one encoding task.
type TaskLog struct {
	mu       sync.Mutex
	f        *os.File
	written  int64
	capped   bool
}

// OpenTaskLog creates logs/task-<id>.log for the given task.
func OpenTaskLog(dir, taskID string) *TaskLog {
	f, err := os.OpenFile(filepath.Join(dir, "task-"+taskID+".log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil
	}
	return &TaskLog{f: f}
}

// WriteLine enforces the 20 MB cap: once exceeded, only lines that look like
// key errors keep being recorded (plan §43).
func (t *TaskLog) WriteLine(line string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.f == nil {
		return
	}
	line = strings.TrimRight(line, "\r\n")
	if t.capped && !isKeyError(line) {
		return
	}
	n, _ := fmt.Fprintf(t.f, "%s\n", line)
	t.written += int64(n)
	if t.written > taskCapBytes {
		if !t.capped {
			fmt.Fprintf(t.f, "== task log capped at 20MB: only errors are recorded from here ==\n")
			t.capped = true
		}
	}
}

func (t *TaskLog) Close() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.f != nil {
		t.f.Close()
		t.f = nil
	}
}

func isKeyError(line string) bool {
	l := strings.ToLower(line)
	for _, kw := range []string{"error", "fail", "fatal", "abort", "invalid", "denied", "cannot", "unable"} {
		if strings.Contains(l, kw) {
			return true
		}
	}
	return false
}

// Cleanup deletes logs older than 7 days and enforces the 500 MB total cap.
func Cleanup(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type fileInfo struct {
		path    string
		modTime time.Time
		size    int64
	}
	var files []fileInfo
	var total int64
	now := time.Now()
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		fi := fileInfo{path: filepath.Join(dir, e.Name()), modTime: info.ModTime(), size: info.Size()}
		total += fi.size
		files = append(files, fi)
	}
	// 7-day retention: delete outright.
	var kept []fileInfo
	for _, f := range files {
		if now.Sub(f.modTime) > retentionDays*24*time.Hour {
			os.Remove(f.path)
			total -= f.size
			continue
		}
		kept = append(kept, f)
	}
	// 500 MB cap: delete oldest first.
	if total > totalCapBytes {
		sort.Slice(kept, func(i, j int) bool { return kept[i].modTime.Before(kept[j].modTime) })
		for _, f := range kept {
			if total <= totalCapBytes {
				break
			}
			os.Remove(f.path)
			total -= f.size
		}
	}
}

// Usage reports current log disk usage for the settings page.
type Usage struct {
	TotalBytes int64 `json:"totalBytes"`
	FileCount  int   `json:"fileCount"`
}

func UsageOf(dir string) Usage {
	var u Usage
	entries, err := os.ReadDir(dir)
	if err != nil {
		return u
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		u.TotalBytes += info.Size()
		u.FileCount++
	}
	return u
}
