// Package media defines the local media analysis model produced by FFprobe.
// All data stays on this machine (boundary spec §18: FFprobe data is local).
package media

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type VideoStream struct {
	Index          int     `json:"index"`
	Codec          string  `json:"codec"`
	Profile        string  `json:"profile"`
	Width          int     `json:"width"`
	Height         int     `json:"height"`
	FPS            float64 `json:"fps"`
	Bitrate        int64   `json:"bitrate"`
	PixelFormat    string  `json:"pixelFormat"`
	BitDepth       int     `json:"bitDepth"`
	ColorSpace     string  `json:"colorSpace"`
	ColorTransfer  string  `json:"colorTransfer"`
	ColorPrimaries string  `json:"colorPrimaries"`
	HDR            string  `json:"hdr"` // "" | HDR10 | HLG
}

type AudioStream struct {
	Index      int    `json:"index"`
	Codec      string `json:"codec"`
	Profile    string `json:"profile"`
	Channels   int    `json:"channels"`
	SampleRate int    `json:"sampleRate"`
	Bitrate    int64  `json:"bitrate"`
	Language   string `json:"language"`
	Title      string `json:"title"`
	Default    bool   `json:"default"`
}

type SubtitleStream struct {
	Index    int    `json:"index"`
	Codec    string `json:"codec"`
	Language string `json:"language"`
	Title    string `json:"title"`
	Default  bool   `json:"default"`
}

type MediaInfo struct {
	FilePath       string            `json:"filePath"`
	FileName       string            `json:"fileName"`
	FileSize       int64             `json:"fileSize"`
	Format         string            `json:"format"`
	Duration       float64           `json:"duration"`
	OverallBitrate int64             `json:"overallBitrate"`
	Video          *VideoStream      `json:"video"`
	Audio          []AudioStream     `json:"audio"`
	Subtitles      []SubtitleStream  `json:"subtitles"`
	Chapters       int               `json:"chapters"`
	Attachments    int               `json:"attachments"`
	Rotation       int               `json:"rotation"`
	Metadata       map[string]string `json:"metadata"`
}

// DisplaySize returns the presentation dimensions after rotation metadata
// is applied (plan §47: rotation must be honored in validation).
func (m *MediaInfo) DisplaySize() (int, int) {
	if m.Video == nil {
		return 0, 0
	}
	w, h := m.Video.Width, m.Video.Height
	if m.Rotation == 90 || m.Rotation == 270 {
		w, h = h, w
	}
	return w, h
}

// HasAudio reports whether any audio track exists.
func (m *MediaInfo) HasAudio() bool { return len(m.Audio) > 0 }

// ---- FFprobe JSON structures ----

type ffprobeOutput struct {
	Format struct {
		Filename   string            `json:"filename"`
		FormatName string            `json:"format_name"`
		Duration   string            `json:"duration"`
		BitRate    string            `json:"bit_rate"`
		Size       string            `json:"size"`
		Tags       map[string]string `json:"tags"`
	} `json:"format"`
	Streams []struct {
		Index    int             `json:"index"`
		CodecName string         `json:"codec_name"`
		CodecType string         `json:"codec_type"`
		Profile  string          `json:"profile"`
		Width    int             `json:"width"`
		Height   int             `json:"height"`
		PixFmt   string          `json:"pix_fmt"`
		RFrameRate string        `json:"r_frame_rate"`
		AvgFrameRate string      `json:"avg_frame_rate"`
		BitRate  string          `json:"bit_rate"`
		Duration string          `json:"duration"`
		Channels int             `json:"channels"`
		SampleRate string        `json:"sample_rate"`
		ColorSpace string        `json:"color_space"`
		ColorTransfer string     `json:"color_transfer"`
		ColorPrimaries string    `json:"color_primaries"`
		Tags     map[string]string `json:"tags"`
		SideDataList []struct {
			SideDataType string `json:"side_data_type"`
			// ffprobe emits rotation as a number on some builds/streams and
			// as a string on others; any + flexible parse handles both.
			Rotation any `json:"rotation"`
		} `json:"side_data_list"`
		Disposition map[string]int `json:"disposition"`
	} `json:"streams"`
	Chapters []struct {
		ID int `json:"id"`
	} `json:"chapters"`
}

// sideDataString normalizes ffprobe side-data values that may arrive as
// strings, numbers, or json.Number depending on the ffprobe build/stream.
func sideDataString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case json.Number:
		return t.String()
	}
	return ""
}

func parseFPS(v string) float64 {	parts := strings.Split(v, "/")
	if len(parts) != 2 {
		f, _ := strconv.ParseFloat(v, 64)
		return f
	}
	num, err1 := strconv.ParseFloat(parts[0], 64)
	den, err2 := strconv.ParseFloat(parts[1], 64)
	if err1 != nil || err2 != nil || den == 0 {
		return 0
	}
	return num / den
}

func parseBitrate(v string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	return n
}

func bitDepthOf(pixFmt string) int {
	// yuv420p10le / yuv420p10be / p010le ...
	if strings.HasSuffix(pixFmt, "10le") || strings.HasSuffix(pixFmt, "10be") ||
		strings.HasPrefix(pixFmt, "p010") || strings.HasPrefix(pixFmt, "yuv420p16") {
		return 10
	}
	if strings.HasSuffix(pixFmt, "12le") || strings.HasSuffix(pixFmt, "12be") {
		return 12
	}
	return 8
}

func hdrOf(transfer string) string {
	switch transfer {
	case "smpte2084":
		return "HDR10"
	case "arib-std-b67":
		return "HLG"
	}
	return ""
}

// fromFFprobe converts the raw JSON output into the public MediaInfo model.
func fromFFprobe(path string, fileSize int64, raw *ffprobeOutput) *MediaInfo {
	mi := &MediaInfo{
		FilePath:  path,
		FileName:  baseName(path),
		FileSize:  fileSize,
		Format:    raw.Format.FormatName,
		Metadata:  raw.Format.Tags,
		Chapters:  len(raw.Chapters),
	}
	if d, err := strconv.ParseFloat(raw.Format.Duration, 64); err == nil {
		mi.Duration = d
	}
	mi.OverallBitrate = parseBitrate(raw.Format.BitRate)

	for i := range raw.Streams {
		s := &raw.Streams[i]
		switch s.CodecType {
		case "video":
			if mi.Video == nil {
				v := &VideoStream{
					Index:          s.Index,
					Codec:          s.CodecName,
					Profile:        s.Profile,
					Width:          s.Width,
					Height:         s.Height,
					PixelFormat:    s.PixFmt,
					BitDepth:       bitDepthOf(s.PixFmt),
					ColorSpace:     s.ColorSpace,
					ColorTransfer:  s.ColorTransfer,
					ColorPrimaries: s.ColorPrimaries,
					HDR:            hdrOf(s.ColorTransfer),
					Bitrate:        parseBitrate(s.BitRate),
				}
				fps := parseFPS(s.AvgFrameRate)
				if fps == 0 {
					fps = parseFPS(s.RFrameRate)
				}
				v.FPS = fps
				for _, sd := range s.SideDataList {
					if sd.SideDataType == "Display Matrix" && sideDataString(sd.Rotation) != "" {
						if r, err := strconv.ParseFloat(sideDataString(sd.Rotation), 64); err == nil {
							v2 := int(r) % 360
							if v2 < 0 {
								v2 += 360
							}
							mi.Rotation = v2
						}
					}
				}
				mi.Video = v
			} else if s.CodecName == "mjpeg" || s.CodecName == "png" {
				mi.Attachments++ // cover art
			}
		case "audio":
			a := AudioStream{
				Index:      s.Index,
				Codec:      s.CodecName,
				Profile:    s.Profile,
				Channels:   s.Channels,
				Language:   s.Tags["language"],
				Title:      s.Tags["title"],
				Bitrate:    parseBitrate(s.BitRate),
			}
			if sr, err := strconv.Atoi(strings.TrimSpace(s.SampleRate)); err == nil {
				a.SampleRate = sr
			}
			if s.Disposition != nil && s.Disposition["default"] == 1 {
				a.Default = true
			}
			mi.Audio = append(mi.Audio, a)
		case "subtitle":
			sub := SubtitleStream{
				Index:    s.Index,
				Codec:    s.CodecName,
				Language: s.Tags["language"],
				Title:    s.Tags["title"],
			}
			if s.Disposition != nil && s.Disposition["default"] == 1 {
				sub.Default = true
			}
			mi.Subtitles = append(mi.Subtitles, sub)
		case "attachment":
			mi.Attachments++
		}
	}
	return mi
}

func baseName(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		p = p[i+1:]
	}
	return p
}

// HumanDuration renders seconds as H:MM:SS for UI display.
func HumanDuration(sec float64) string {
	if sec < 0 {
		sec = 0
	}
	h := int(sec) / 3600
	m := (int(sec) % 3600) / 60
	s := int(sec) % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}
