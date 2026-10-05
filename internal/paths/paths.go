// Package paths resolves the VideoDelite data directory.
//
// Data location policy: prefer the INSTALL directory (exe_dir/Data) so all
// app files live together (user request 2026-10-04); fall back to
// %LOCALAPPDATA%/VideoDelite when the install dir is not writable
// (e.g. Program Files without the installer's ACL grant).
//
// NOTE: upgrading from pre-r3 builds? Old history lives in
// %LOCALAPPDATA%\VideoDelite\videodelite.db — copy it into Data\ manually
// (automatic migration was deferred).
//
// All paths are built with filepath.Join from a validated root plus fixed
// file-name constants defined in this file; no user input participates.
package paths

import (
	"os"
	"path/filepath"
)

type Paths struct {
	Root   string // Data root (db/logs/temp/cache live here)
	Config string
	Logs   string
	Temp   string
	Cache  string
	DB     string
}

// Ensure creates and returns the data layout.
func Ensure() (*Paths, error) {
	root := dataRoot()
	p := &Paths{
		Root:   root,
		Config: filepath.Join(root, "Config"),
		Logs:   filepath.Join(root, "Logs"),
		Temp:   filepath.Join(root, "Temp"),
		Cache:  filepath.Join(root, "Cache"),
		DB:     filepath.Join(root, "videodelite.db"),
	}
	for _, dir := range []string{p.Root, p.Config, p.Logs, p.Temp, p.Cache} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	return p, nil
}

// dataRoot prefers the install dir (exe_dir/Data) when writable, and falls
// back to %LOCALAPPDATA%\VideoDelite otherwise.
func dataRoot() string {
	if exe, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(exe), "Data")
		if writable(cand) {
			return cand
		}
	}
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = os.Getenv("APPDATA")
	}
	return filepath.Join(base, "VideoDelite")
}

// writable verifies real write access by creating and removing a probe file.
func writable(dir string) bool {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false
	}
	probe := filepath.Join(dir, ".vd_write_test")
	f, err := os.Create(probe)
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(probe)
	return true
}

// DefaultOutputDir returns the first-run default output folder
// %USERPROFILE%\Videos\Compressed.
func DefaultOutputDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = os.Getenv("USERPROFILE")
	}
	return filepath.Join(home, "Videos", "Compressed")
}
