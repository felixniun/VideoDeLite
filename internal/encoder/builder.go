package encoder

import (
	"fmt"
	"math"
	"strings"

	"videodelite/internal/media"
)

// EncodingConfig is the single source of truth handed from the UI to Go
// (plan §84–§85: the frontend never assembles FFmpeg commands).
type EncodingConfig struct {
	Container  string `json:"container"`  // mp4 | mkv
	VideoCodec string `json:"videoCodec"` // h264 | h265
	Encoder    string `json:"encoder"`    // auto | cpu | nvenc | qsv | amf
	RateControl string `json:"rateControl"` // simple | cbr | vbr | cq
	BitrateMbps float64 `json:"bitrateMbps"`
	MaxBitrateMbps float64 `json:"maxBitrateMbps"`
	Quality int `json:"quality"` // 0–51, encoder-mapped (plan §31)
	AudioCodec string `json:"audioCodec"` // aac
	AudioBitrateKbps int `json:"audioBitrateKbps"`
	AudioRateControl string `json:"audioRateControl"` // cbr | vbr
	AudioQuality int `json:"audioQuality"` // 1–5 (plan §26)
	Preset string `json:"preset"`
	PreserveMetadata bool `json:"preserveMetadata"`
	PreserveSubtitles bool `json:"preserveSubtitles"`
	PreserveChapters bool `json:"preserveChapters"`
	PreserveAudioTracks bool `json:"preserveAudioTracks"`
}

// BuildSimpleConfig derives the frozen Simple Mode configuration.
func BuildSimpleConfig(mi *media.MediaInfo, codec, quality, container string) EncodingConfig {
	return EncodingConfig{
		Container:  container,
		VideoCodec: codec,
		Encoder:    "auto",
		RateControl: "simple",
		BitrateMbps: SimpleBitrateMbps(mi.Video.Width, mi.Video.Height, mi.Video.FPS, codec, quality),
		AudioCodec: "aac",
		AudioBitrateKbps: 128, // plan §23
		AudioRateControl: "cbr",
		PreserveMetadata: true,
		PreserveSubtitles: true,
		PreserveChapters: true,
		PreserveAudioTracks: true,
	}
}

// BuildResult carries everything needed to spawn FFmpeg.
type BuildResult struct {
	Args     []string
	Warnings []string
	Selected *SelectedEncoder
	// Effective bitrate after fallbacks, for history/UI.
	EffectiveBitrateMbps float64
}

// mp4TextSubs are the subtitle codecs FFmpeg can convert to mov_text.
var mp4TextSubs = map[string]bool{
	"subrip": true, "srt": true, "ass": true, "ssa": true,
	"webvtt": true, "mov_text": true, "text": true,
}

// BuildCommand assembles the full FFmpeg argument list.
func BuildCommand(inputPath, outputPath string, mi *media.MediaInfo, cfg EncodingConfig, sel *SelectedEncoder) (*BuildResult, error) {
	var args []string
	var warnings []string

	args = append(args, "-hide_banner", "-nostdin", "-y")
	args = append(args, "-i", inputPath)

	// ---- stream mapping ----
	args = append(args, "-map", "0:v:0")
	if cfg.PreserveAudioTracks && len(mi.Audio) > 0 {
		args = append(args, "-map", "0:a?")
	} else if len(mi.Audio) > 0 {
		args = append(args, "-map", "0:a:0")
	}
	subOK := true
	if cfg.PreserveSubtitles && len(mi.Subtitles) > 0 {
		if cfg.Container == "mp4" {
			// Only text subs can become mov_text; image subs (PGS etc.)
			// are incompatible: warn, never silently drop (plan §49).
			compatible := 0
			for _, s := range mi.Subtitles {
				if mp4TextSubs[s.Codec] {
					compatible++
				} else {
					warnings = append(warnings,
						fmt.Sprintf("字幕轨道 #%d (%s) 与 MP4 不兼容，输出将不包含该轨道", s.Index, s.Codec))
				}
			}
			if compatible > 0 {
				args = append(args, "-map", "0:s?")
			}
		} else {
			args = append(args, "-map", "0:s?")
		}
		subOK = strings.Contains(strings.Join(args, " "), "0:s")
	}
	if cfg.PreserveMetadata {
		args = append(args, "-map_metadata", "0")
		if cfg.Container == "mkv" && mi.Attachments > 0 {
			args = append(args, "-map", "0:t?")
		} else if mi.Attachments > 0 && cfg.Container == "mp4" {
			warnings = append(warnings, "MP4 容器不保留附件/封面元数据")
		}
	}

	// ---- video encoder ----
	args = append(args, "-c:v", sel.Arg)
	args = append(args, videoRateControl(sel.Family, cfg)...)

	// pixel format / bit depth (plan §48: 10-bit must survive honestly)
	tenBit := mi.Video != nil && mi.Video.BitDepth >= 10
	if cfg.VideoCodec == "h265" && tenBit {
		switch sel.Family {
		case "cpu":
			args = append(args, "-pix_fmt", "yuv420p10le")
		default:
			args = append(args, "-pix_fmt", "p010le")
		}
	} else {
		if tenBit && cfg.VideoCodec == "h264" {
			warnings = append(warnings, "H.264 输出为 8-bit，源 10-bit 将被转换")
		}
		args = append(args, "-pix_fmt", "yuv420p")
	}

	// HDR / color signaling (best effort; validation verifies the result)
	if mi.Video != nil && cfg.VideoCodec == "h265" {
		hasColor := mi.Video.ColorPrimaries != "" || mi.Video.ColorTransfer != "" || mi.Video.ColorSpace != ""
		if hasColor {
			if mi.Video.ColorPrimaries != "" {
				args = append(args, "-color_primaries", mi.Video.ColorPrimaries)
			}
			if mi.Video.ColorTransfer != "" {
				args = append(args, "-color_trc", mi.Video.ColorTransfer)
			}
			if mi.Video.ColorSpace != "" {
				args = append(args, "-colorspace", mi.Video.ColorSpace)
			}
			// libx265 ignores bare -color_* output options for its VUI;
			// the flags must go through x265-params or HDR markers are lost
			// (found by the Phase 8 QA matrix, plan §48).
			if sel.Family == "cpu" {
				var p []string
				if mi.Video.ColorPrimaries != "" {
					p = append(p, "colorprim="+mi.Video.ColorPrimaries)
				}
				if mi.Video.ColorTransfer != "" {
					p = append(p, "transfer="+mi.Video.ColorTransfer)
				}
				if mi.Video.ColorSpace != "" {
					p = append(p, "colormatrix="+mi.Video.ColorSpace)
				}
				if len(p) > 0 {
					args = append(args, "-x265-params", strings.Join(p, ":"))
				}
			}
			if mi.Video.HDR != "" && sel.Family != "cpu" {
				warnings = append(warnings, "HDR 动态元数据在硬件编码下可能不完整，输出已验证基础色彩标记")
			}
		}
	}

	// ---- audio ----
	if len(mi.Audio) > 0 {
		args = append(args, "-c:a", "aac")
		if cfg.AudioRateControl == "vbr" {
			args = append(args, "-q:a", aacVBRQuality(cfg.AudioQuality))
		} else {
			args = append(args, "-b:a", fmt.Sprintf("%dk", clamp(cfg.AudioBitrateKbps, 64, 512)))
		}
	}

	// ---- subtitles codec ----
	if subOK {
		if cfg.Container == "mp4" {
			args = append(args, "-c:s", "mov_text")
		} else {
			args = append(args, "-c:s", "copy")
		}
	}

	if cfg.Container == "mp4" {
		args = append(args, "-movflags", "+faststart")
		args = append(args, "-f", "mp4")
	} else {
		args = append(args, "-f", "matroska")
	}
	args = append(args, "-max_muxing_queue_size", "1024")
	args = append(args, outputPath)

	return &BuildResult{
		Args:                 args,
		Warnings:             warnings,
		Selected:             sel,
		EffectiveBitrateMbps: cfg.BitrateMbps,
	}, nil
}

// videoRateControl maps the unified UI model onto encoder-native parameters
// (plan §30: backend owns the encoder-specific mapping).
func videoRateControl(family string, cfg EncodingConfig) []string {
	mbps := cfg.BitrateMbps
	max := cfg.MaxBitrateMbps
	rc := cfg.RateControl
	if rc == "" {
		rc = "simple"
	}
	if max > 0 && max < mbps {
		max = mbps // plan §29: never let max < average
	}
	kbps := func(v float64) string {
		return fmt.Sprintf("%dk", int(math.Round(v*1000)))
	}
	var args []string

	addPreset := func() {
		p := presetFor(family, cfg.Preset)
		if p != "" {
			args = append(args, "-preset", p)
		} else if family == "nvenc" {
			args = append(args, "-preset", "p5")
		}
	}

	switch family {
	case "nvenc":
		addPreset()
		switch rc {
		case "cq":
			args = append(args, "-rc", "vbr", "-cq", fmt.Sprint(cfg.Quality), "-b:v", "0")
		case "cbr":
			args = append(args, "-rc", "cbr", "-b:v", kbps(mbps), "-maxrate", kbps(mbps), "-bufsize", kbps(2*mbps))
		default: // simple + vbr
			if max <= 0 {
				max = 1.5 * mbps
			}
			args = append(args, "-rc", "vbr", "-b:v", kbps(mbps), "-maxrate", kbps(max), "-bufsize", kbps(3*mbps))
		}
	case "qsv":
		addPreset()
		switch rc {
		case "cq":
			args = append(args, "-global_quality", fmt.Sprint(cfg.Quality), "-look_ahead", "0")
		case "cbr":
			args = append(args, "-b:v", kbps(mbps), "-maxrate", kbps(mbps), "-bufsize", kbps(2*mbps))
		default:
			if max <= 0 {
				max = 1.5 * mbps
			}
			args = append(args, "-b:v", kbps(mbps), "-maxrate", kbps(max), "-bufsize", kbps(3*mbps))
		}
	case "amf":
		addPreset()
		switch rc {
		case "cq":
			args = append(args, "-rc", "cqp", "-qp_i", fmt.Sprint(cfg.Quality), "-qp_p", fmt.Sprint(cfg.Quality))
		case "cbr":
			args = append(args, "-rc", "cbr", "-b:v", kbps(mbps), "-maxrate", kbps(mbps))
		default:
			if max <= 0 {
				max = 1.5 * mbps
			}
			args = append(args, "-rc", "vbr_peak", "-b:v", kbps(mbps), "-maxrate", kbps(max))
		}
	default: // cpu
		addPreset()
		switch rc {
		case "cq":
			if cfg.VideoCodec == "h265" {
				args = append(args, "-crf", fmt.Sprint(cfg.Quality))
			} else {
				args = append(args, "-crf", fmt.Sprint(cfg.Quality))
			}
		case "cbr":
			args = append(args, "-b:v", kbps(mbps), "-maxrate", kbps(mbps), "-bufsize", kbps(3*mbps))
		default:
			if max <= 0 {
				max = 1.5 * mbps
			}
			args = append(args, "-b:v", kbps(mbps), "-maxrate", kbps(max), "-bufsize", kbps(3*mbps))
		}
	}
	return args
}

// presetFor maps the generic preset names onto encoder-native presets
// (plan §64: names are not forced identical across encoders).
func presetFor(family, preset string) string {
	if preset == "" || preset == "default" {
		return ""
	}
	switch family {
	case "nvenc":
		switch preset {
		case "fast":
			return "p3"
		case "medium":
			return "p5"
		case "slow":
			return "p7"
		}
	case "qsv":
		return map[string]string{"fast": "fast", "medium": "medium", "slow": "slow"}[preset]
	case "amf":
		return map[string]string{"fast": "speed", "medium": "balanced", "slow": "quality"}[preset]
	default:
		return map[string]string{"fast": "veryfast", "medium": "medium", "slow": "slow"}[preset]
	}
	return ""
}

// aacVBRQuality maps UI Quality 1–5 onto the native AAC encoder scale.
func aacVBRQuality(q int) string {
	switch {
	case q <= 1:
		return "0.4"
	case q == 2:
		return "0.6"
	case q == 3:
		return "0.8"
	case q == 4:
		return "1.0"
	default:
		return "1.2"
	}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ProgressArgs are appended by the runner (kept here so tests can build full
// commands). The runner owns the pipes.
func ProgressArgs() []string {
	return []string{"-progress", "pipe:1", "-nostats", "-loglevel", "warning"}
}

// AppendProgress attaches the progress reporting flags to args.
func AppendProgress(args []string) []string {
	return append(args, ProgressArgs()...)
}
