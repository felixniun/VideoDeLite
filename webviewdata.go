package main

import (
	"os"
	"path/filepath"
)

// webviewDataPath keeps the WebView2 cache beside the rest of the app data:
// <DataRoot>/WebView2 (install dir when writable, else %LOCALAPPDATA%).
func webviewDataPath() string {
	base := dataRootForWebview()
	return filepath.Join(base, "WebView2")
}

// dataRootForWebview mirrors internal/paths pickDataRoot without importing
// the package (main must stay dependency-light for wails bindings).
func dataRootForWebview() string {
	if exe, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(exe), "Data")
		if mkdirWritable(cand) {
			return cand
		}
	}
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = os.Getenv("APPDATA")
	}
	return filepath.Join(base, "VideoDelite")
}

func mkdirWritable(dir string) bool {
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
