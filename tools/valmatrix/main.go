// valmatrix is the Phase 0 technical validation harness (plan §98/§132).
//
// It runs REAL encodes against the local FFmpeg and writes
// docs/TECHNICAL-VALIDATION.md with actual results. Nothing is pre-filled.
//
//	go run ./tools/valmatrix
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var ffmpeg, ffprobe, workDir string

type cell struct {
	status string // PASS / FAIL / N/A
	note   string
}

func main() {
	var err error
	ffmpeg, err = exec.LookPath("ffmpeg")
	must(err)
	ffprobe, err = exec.LookPath("ffprobe")
	must(err)
	workDir, err = os.MkdirTemp("", "valmatrix-*")
	must(err)
	defer os.RemoveAll(workDir)

	fmt.Println("== VideoDelite Phase 0 technical validation ==")
	fmt.Println("ffmpeg:", ffmpeg)
	fmt.Printf("started: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))

	families := []struct {
		key, label string
		h264, h265 string
	}{
		{"cpu", "CPU (libx264/libx265)", "libx264", "libx265"},
		{"nvenc", "NVIDIA NVENC", "h264_nvenc", "hevc_nvenc"},
		{"qsv", "Intel QSV", "h264_qsv", "hevc_qsv"},
		{"amf", "AMD AMF", "h264_amf", "hevc_amf"},
	}

	// matrix[feature][family] = cell
	matrix := map[string]map[string]cell{}
	record := func(feature, family string, c cell) {
		if matrix[feature] == nil {
			matrix[feature] = map[string]cell{}
		}
		matrix[feature][family] = c
	}

	for _, f := range families {
		for _, codec := range []string{"h264", "h265"} {
			enc := f.h264
			if codec == "h265" {
				enc = f.h265
			}
			// SDR 1080p MP4 + MKV, with AAC audio
			mp4Res := encodeTest(enc, codec, "mp4", false, false)
			mkvRes := encodeTest(enc, codec, "mkv", false, false)
			res := combine(mp4Res, mkvRes)
			record(strings.ToUpper(codec)+" MP4+MKV", f.key, res)
			// 10-bit HEVC
			if codec == "h265" {
				record("10-bit HEVC", f.key, encodeTest(enc, codec, "mp4", true, false))
				// HDR10 signaling
				record("HDR10 signaling", f.key, encodeTest(enc, codec, "mp4", true, true))
			}
		}
	}
	// AAC itself (CPU encode path with audio)
	record("AAC 128k", "cpu", audioTest())
	// containers/audio/subs for the remaining families inherit the matrix row
	for _, f := range families[1:] {
		record("AAC 128k", f.key, audioTest())
	}

	out := render(families, matrix)
	os.MkdirAll("docs", 0o755)
	must(os.WriteFile(filepath.Join("docs", "TECHNICAL-VALIDATION.md"), []byte(out), 0o644))
	fmt.Println("\nwritten: docs/TECHNICAL-VALIDATION.md")
}

func combine(a, b cell) cell {
	if a.status == "PASS" && b.status == "PASS" {
		return cell{"PASS", ""}
	}
	if a.status == "FAIL" && b.status == "FAIL" {
		return cell{"FAIL", firstNonEmpty(a.note, b.note)}
	}
	return cell{"PARTIAL", firstNonEmpty(a.note, b.note)}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// encodeTest generates a 4s lavfi source and encodes it.
func encodeTest(enc, codec, container string, tenBit, hdr bool) cell {
	label := fmt.Sprintf("%s %s %s 10bit=%v hdr=%v", enc, codec, container, tenBit, hdr)
	out := filepath.Join(workDir, fmt.Sprintf("%s_%s_%s_%v_%v.%s",
		enc, codec, container, tenBit, hdr, container))

	args := []string{"-hide_banner", "-nostdin", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=1920x1080:rate=30:duration=4",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=4"}

	if tenBit {
		args = append(args, "-vf", "format=yuv420p10le")
	}
	args = append(args, "-c:v", enc)
	// Bitrate-style defaults like Simple Mode uses.
	args = append(args, "-b:v", "6000k", "-maxrate", "9000k", "-bufsize", "18000k")
	if tenBit {
		switch {
		case strings.Contains(enc, "nvenc"), strings.Contains(enc, "qsv"), strings.Contains(enc, "amf"):
			args = append(args, "-pix_fmt", "p010le")
		default:
			args = append(args, "-pix_fmt", "yuv420p10le")
		}
	}
	if hdr {
		args = append(args,
			"-color_primaries", "bt2020",
			"-color_trc", "smpte2084",
			"-colorspace", "bt2020nc")
	}
	args = append(args, "-c:a", "aac", "-b:a", "128k")
	if container == "mp4" {
		args = append(args, "-movflags", "+faststart", "-f", "mp4")
	} else {
		args = append(args, "-f", "matroska")
	}
	args = append(args, out)

	start := time.Now()
	cmd := exec.Command("ffmpeg", args...)
	out689, err := cmd.CombinedOutput()
	elapsed := time.Since(start).Round(time.Millisecond)
	if err != nil {
		fmt.Printf("  FAIL  %-52s (%v) %s\n", label, elapsed, firstLine(out689))
		return cell{"FAIL", firstLine(out689)}
	}

	// Verify with ffprobe.
	type probe struct {
		Streams []struct {
			CodecName   string `json:"codec_name"`
			CodecType   string `json:"codec_type"`
			ColorTrc    string `json:"color_transfer"`
			PixFmt      string `json:"pix_fmt"`
		} `json:"streams"`
	}
	p := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", out)
	pj, err := p.Output()
	if err != nil {
		fmt.Printf("  FAIL  %-52s probe error %v\n", label, err)
		return cell{"FAIL", "ffprobe failed: " + err.Error()}
	}
	var pr probe
	must(json.Unmarshal(pj, &pr))
	wantVideo := "h264"
	if codec == "h265" {
		wantVideo = "hevc"
	}
	gotVideo, gotAudio := "", ""
	hdrOK := true
	depthOK := true
	for _, s := range pr.Streams {
		if s.CodecType == "video" {
			gotVideo = s.CodecName
			if hdr && s.ColorTrc != "smpte2084" {
				hdrOK = false
			}
			if tenBit && !strings.Contains(s.PixFmt, "10") && s.PixFmt != "p010le" {
				depthOK = false
			}
		}
		if s.CodecType == "audio" {
			gotAudio = s.CodecName
		}
	}
	switch {
	case gotVideo != wantVideo:
		fmt.Printf("  FAIL  %-52s (%v) video=%s want=%s\n", label, elapsed, gotVideo, wantVideo)
		return cell{"FAIL", "encoded as " + gotVideo}
	case gotAudio != "aac":
		fmt.Printf("  FAIL  %-52s (%v) audio=%s\n", label, elapsed, gotAudio)
		return cell{"FAIL", "audio=" + gotAudio}
	case hdr && !hdrOK:
		fmt.Printf("  PARTIAL %-51s (%v) HDR flags lost\n", label, elapsed)
		return cell{"PARTIAL", "HDR 信号未写入输出"}
	case tenBit && !depthOK:
		fmt.Printf("  PARTIAL %-51s (%v) output 8-bit\n", label, elapsed)
		return cell{"PARTIAL", "10-bit 未保留"}
	}
	fmt.Printf("  PASS  %-52s (%v)\n", label, elapsed)
	return cell{"PASS", ""}
}

// audioTest checks AAC at the frozen Simple bitrate.
func audioTest() cell {
	out := filepath.Join(workDir, "aac.m4a")
	args := []string{"-hide_banner", "-nostdin", "-y",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=4",
		"-c:a", "aac", "-b:a", "128k", out}
	if out689, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		return cell{"FAIL", firstLine(out689)}
	}
	pj, err := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", out).Output()
	if err != nil {
		return cell{"FAIL", err.Error()}
	}
	if !strings.Contains(string(pj), `"codec_name": "aac"`) && !strings.Contains(string(pj), `"codec_name":"aac"`) {
		return cell{"FAIL", "not aac"}
	}
	return cell{"PASS", ""}
}

func firstLine(b []byte) string {
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if l != "" && !strings.HasPrefix(l, "[lavf") && !strings.HasPrefix(l, "[null") && !strings.HasPrefix(l, "[out#") && !strings.HasPrefix(l, "[vost#") && !strings.HasPrefix(l, "[ast#") && !strings.HasPrefix(l, "[auto") {
			if len(l) > 130 {
				l = l[:130] + "…"
			}
			return l
		}
	}
	return "unknown error"
}

func render(families []struct {
	key, label string
	h264, h265 string
}, matrix map[string]map[string]cell) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# VideoDelite Phase 0 技术验证矩阵\n\n")
	fmt.Fprintf(&b, "> 本文档由 `go run ./tools/valmatrix` 于 %s 实测生成，结果来自真实编码运行。\n\n",
		time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&b, "- FFmpeg: `%s`\n- FFprobe: `%s`\n- 测试源: lavfi testsrc2 1920x1080@30 4s + sine 音频\n\n",
		ffmpeg, ffprobe)

	featureOrder := []string{"H264 MP4+MKV", "H265 MP4+MKV", "10-bit HEVC", "HDR10 signaling", "AAC 128k"}
	fmt.Fprintf(&b, "| Feature | CPU | NVENC | QSV | AMF |\n|---|---|---|---|---|\n")
	for _, feat := range featureOrder {
		fmt.Fprintf(&b, "| %s ", feat)
		for _, f := range families {
			c := matrix[feat][f.key]
			fmt.Fprintf(&b, "| %s ", cellText(c))
		}
		fmt.Fprintf(&b, "|\n")
	}
	fmt.Fprintf(&b, "\n## 说明\n\n")
	fmt.Fprintf(&b, "- PASS = 编码成功且 FFprobe 验证通过；PARTIAL = 编码成功但部分特性丢失（已注明）；FAIL = 编码失败（真实错误信息）。\n")
	fmt.Fprintf(&b, "- 本机无该厂商 GPU 时 FAIL/错误信息属预期结果（计划书 §97：Detect + Disable + Reason）。\n")
	fmt.Fprintf(&b, "- HDR10 signaling 仅验证 smpte2084 基础标记写入；动态元数据（SMPTE ST 2086）保留为 V1 已知限制，输出验证阶段会给出警告。\n")
	return b.String()
}

func cellText(c cell) string {
	switch c.status {
	case "PASS":
		return "PASS"
	case "PARTIAL":
		return "PARTIAL — " + c.note
	case "FAIL":
		if c.note == "" {
			return "FAIL"
		}
		return "FAIL — " + c.note
	}
	return "N/A"
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}
