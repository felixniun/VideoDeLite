// Package validation runs the mandatory post-encode FFprobe check.
//
// Frozen rule (plan §52/§54): FFmpeg exit code 0 is NOT success. A task is
// Completed only if the output file exists, is non-empty, and probes back
// with the expected container/video/audio properties.
package validation

import (
	"fmt"
	"math"
	"os"

	"videodelite/internal/encoder"
	"videodelite/internal/media"
)

type Check struct {
	Name     string `json:"name"`
	Pass     bool   `json:"pass"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	Warning  bool   `json:"warning"` // advisory only, does not fail the task
}

type Report struct {
	Pass     bool     `json:"pass"`
	Checks   []Check  `json:"checks"`
	Warnings []string `json:"warnings"`
}

func expectedCodecFamily(encoderArg, fallbackCodec string) string {
	switch {
	case contains(encoderArg, "h264"):
		return "h264"
	case contains(encoderArg, "hevc"), contains(encoderArg, "x265"):
		return "hevc"
	case contains(encoderArg, "av1"):
		return "av1"
	}
	if fallbackCodec == "h265" {
		return "hevc"
	}
	return "h264"
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// Validate probes outputPath and compares it against the source media and
// the configuration that was supposed to be applied.
func Validate(analyzer *media.Analyzer, outputPath string, src *media.MediaInfo, cfg encoder.EncodingConfig, encoderArg string) (*Report, error) {
	rep := &Report{Pass: false}

	// 1. file exists and is non-empty
	st, err := os.Stat(outputPath)
	if err != nil || st.Size() == 0 {
		rep.Checks = append(rep.Checks, Check{Name: "file", Pass: false,
			Expected: "existing non-empty output", Actual: fmt.Sprintf("stat error: %v", err)})
		return rep, nil
	}

	out, err := analyzer.Analyze(outputPath)
	if err != nil {
		rep.Checks = append(rep.Checks, Check{Name: "probe", Pass: false,
			Expected: "ffprobe parses the output", Actual: err.Error()})
		return rep, nil
	}

	addCheckWarn := func(name, expected, actual string) {
		rep.Checks = append(rep.Checks, Check{Name: name, Pass: true, Warning: true, Expected: expected, Actual: actual})
	}

	// 2. container
	wantFormat := "mp4"
	if cfg.Container == "mkv" {
		wantFormat = "matroska"
	}
	okFormat := false
	if wantFormat == "mp4" {
		okFormat = containsStr(out.Format, "mov") || containsStr(out.Format, "mp4")
	} else {
		okFormat = containsStr(out.Format, "matroska")
	}
	rep.Checks = append(rep.Checks, Check{Name: "container", Pass: okFormat,
		Expected: wantFormat, Actual: out.Format})

	// 3. video codec family
	wantCodec := expectedCodecFamily(encoderArg, cfg.VideoCodec)
	rep.Checks = append(rep.Checks, Check{Name: "videoCodec", Pass: out.Video.Codec == wantCodec,
		Expected: wantCodec, Actual: out.Video.Codec})

	// 4. resolution (rotation-aware)
	sw, sh := src.DisplaySize()
	rep.Checks = append(rep.Checks, Check{Name: "resolution", Pass: out.Video.Width == sw && out.Video.Height == sh,
		Expected: fmt.Sprintf("%dx%d", sw, sh), Actual: fmt.Sprintf("%dx%d", out.Video.Width, out.Video.Height)})

	// 5. FPS (allow small drift, never enforce table FPS)
	fpsOK := math.Abs(out.Video.FPS-src.Video.FPS) <= 1.5
	rep.Checks = append(rep.Checks, Check{Name: "fps", Pass: fpsOK,
		Expected: fmt.Sprintf("%.3f", src.Video.FPS), Actual: fmt.Sprintf("%.3f", out.Video.FPS)})

	// 6. duration (±2% or ±2s)
	durDelta := math.Abs(out.Duration - src.Duration)
	durOK := durDelta <= math.Max(2.0, src.Duration*0.02)
	rep.Checks = append(rep.Checks, Check{Name: "duration", Pass: durOK,
		Expected: fmt.Sprintf("%.2fs", src.Duration), Actual: fmt.Sprintf("%.2fs", out.Duration)})

	// 7. audio presence and codec
	if src.HasAudio() {
		if len(out.Audio) == 0 {
			rep.Checks = append(rep.Checks, Check{Name: "audio", Pass: false,
				Expected: "audio track(s), aac", Actual: "none"})
		} else {
			rep.Checks = append(rep.Checks, Check{Name: "audio", Pass: out.Audio[0].Codec == "aac",
				Expected: "aac", Actual: out.Audio[0].Codec})
			if len(out.Audio) < len(src.Audio) {
				addCheckWarn("audioTracks", fmt.Sprintf("%d", len(src.Audio)), fmt.Sprintf("%d", len(out.Audio)))
				rep.Warnings = append(rep.Warnings, fmt.Sprintf("音轨数量 %d → %d", len(src.Audio), len(out.Audio)))
			}
		}
	}

	// 8. HDR / color signaling — advisory (plan §48: check honestly, warn if lost)
	if src.Video.HDR != "" && cfg.VideoCodec == "h265" {
		if out.Video.HDR == src.Video.HDR {
			addCheckWarn("hdr", src.Video.HDR, out.Video.HDR)
		} else {
			addCheckWarn("hdr", src.Video.HDR, orDash(out.Video.HDR))
			rep.Warnings = append(rep.Warnings, "HDR 色彩标记未完整保留（源 "+src.Video.HDR+"）")
		}
	}

	// 9. bit depth — advisory for hevc
	if src.Video.BitDepth >= 10 && cfg.VideoCodec == "h265" {
		if out.Video.BitDepth >= 10 {
			addCheckWarn("bitDepth", "10-bit", fmt.Sprintf("%d-bit", out.Video.BitDepth))
		} else {
			addCheckWarn("bitDepth", "10-bit", "8-bit")
			rep.Warnings = append(rep.Warnings, "10-bit 未保留（输出为 8-bit）")
		}
	}

	// 10. subtitles / chapters — advisory
	if cfg.PreserveSubtitles && len(src.Subtitles) > 0 && len(out.Subtitles) < len(src.Subtitles) {
		addCheckWarn("subtitles", fmt.Sprintf("%d", len(src.Subtitles)), fmt.Sprintf("%d", len(out.Subtitles)))
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("字幕轨道 %d → %d", len(src.Subtitles), len(out.Subtitles)))
	}
	if cfg.PreserveChapters && src.Chapters > 0 && out.Chapters < src.Chapters {
		addCheckWarn("chapters", fmt.Sprintf("%d", src.Chapters), fmt.Sprintf("%d", out.Chapters))
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("章节 %d → %d", src.Chapters, out.Chapters))
	}

	// Aggregate: hard checks must all pass.
	rep.Pass = true
	for _, c := range rep.Checks {
		if !c.Pass && !c.Warning {
			rep.Pass = false
			break
		}
	}
	return rep, nil
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
