package media

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"videodelite/internal/proc"
)

// Analyzer runs FFprobe against local files. It is fully offline: no network,
// no account dependency (plan §29: the local pipeline never crosses HTTP).
type Analyzer struct {
	FFprobeBin string
	Timeout    time.Duration
}

func NewAnalyzer(ffprobeBin string) *Analyzer {
	return &Analyzer{FFprobeBin: ffprobeBin, Timeout: 60 * time.Second}
}

// Analyze probes a single file.
func (a *Analyzer) Analyze(path string) (*MediaInfo, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("input file not accessible: %w", err)
	}
	if st.IsDir() {
		return nil, fmt.Errorf("input is a directory: %s", path)
	}
	if st.Size() == 0 {
		return nil, fmt.Errorf("input file is empty: %s", path)
	}

	ctx, cancel := context.WithTimeout(context.Background(), a.Timeout)
	defer cancel()

	args := []string{
		"-v", "error",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		"-show_chapters",
		path,
	}
	cmd := proc.CommandContext(ctx, a.FFprobeBin, args...)
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("ffprobe timed out on %s", path)
		}
		return nil, fmt.Errorf("ffprobe failed on %s: %w", path, err)
	}
	var raw ffprobeOutput
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("ffprobe returned unparsable JSON for %s: %w", path, err)
	}
	mi := fromFFprobe(path, st.Size(), &raw)
	if mi.Video == nil {
		return nil, fmt.Errorf("no video stream found in %s", path)
	}
	return mi, nil
}

// VideoExtensions lists the container extensions accepted on import.
var VideoExtensions = map[string]bool{
	".mp4": true, ".mkv": true, ".mov": true, ".avi": true, ".webm": true,
	".flv": true, ".ts": true, ".m2ts": true, ".mts": true, ".wmv": true,
	".m4v": true, ".mpg": true, ".mpeg": true, ".vob": true, ".3gp": true,
	".ogv": true, ".rm": true, ".rmvb": true, ".asf": true, ".divx": true,
}

// IsVideoFile reports whether the path looks like an importable video.
func IsVideoFile(p string) bool {
	return VideoExtensions[strings.ToLower(filepath.Ext(p))]
}

// ExpandPaths takes dropped files/folders and returns the video files inside,
// keeping a stable, sorted order (plan §11: files, folders, recursive scan).
func ExpandPaths(inputs []string) ([]string, error) {
	var files []string
	seen := map[string]bool{}
	add := func(p string) {
		abs, err := filepath.Abs(p)
		if err != nil {
			abs = p
		}
		if !seen[abs] {
			seen[abs] = true
			files = append(files, abs)
		}
	}
	for _, in := range inputs {
		if in == "" {
			continue
		}
		st, err := os.Stat(in)
		if err != nil {
			continue // silently skip unreachable entries on drag & drop
		}
		if !st.IsDir() {
			if IsVideoFile(in) {
				add(in)
			}
			continue
		}
		err = filepath.WalkDir(in, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return nil // best-effort scan
			}
			if d.IsDir() {
				return nil
			}
			if IsVideoFile(p) {
				add(p)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(files)
	return files, nil
}
