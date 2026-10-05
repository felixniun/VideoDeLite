// qamatrix runs the Phase 8 media-preservation QA matrix (plan §135).
//
// It generates real test sources (HDR10 10-bit / multi-audio / subtitles /
// chapters / rotation), compresses each through the REAL product pipeline
// (media.Analyzer → encoder.BuildSimpleConfig → encoder.BuildCommand →
// ffmpegx runner), then compares source vs output with FFprobe.
//
// Output: docs/QA-PRESERVATION.md — results come from real runs only.
//
//	go run ./tools/qamatrix
package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"
	"path/filepath"
	"strings"
	"time"

	"videodelite/internal/encoder"
	"videodelite/internal/ffmpegx"
	"videodelite/internal/media"
)

var (
	ffmpeg   string
	ffprobe  string
	workDir  string
	analyzer *media.Analyzer
)

type result struct {
	name   string
	checks []check
}

type check struct {
	name   string
	pass   bool
	detail string
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintln(os.Stderr, "fatal:", r)
			fmt.Fprintln(os.Stderr, string(debug.Stack()))
			os.Exit(1)
		}
	}()
	var err error
	ffmpeg, err = exec.LookPath("ffmpeg")
	must(err)
	ffprobe, err = exec.LookPath("ffprobe")
	must(err)
	workDir, err = os.MkdirTemp("", "qamatrix-*")
	must(err)
	defer os.RemoveAll(workDir)
	analyzer = media.NewAnalyzer(ffprobe)

	fmt.Println("== VideoDelite Phase 8 media-preservation QA ==")
	start := time.Now()

	var results []result
	results = append(results, caseHDR10())
	results = append(results, caseTenBitSDR())
	results = append(results, caseMultiAudio())
	results = append(results, caseSubtitlesChapters())
	results = append(results, caseRotation())

	out := render(results, time.Since(start))
	must(os.MkdirAll("docs", 0o755))
	must(os.WriteFile(filepath.Join("docs", "QA-PRESERVATION.md"), []byte(out), 0o644))
	fmt.Printf("\nwritten: docs/QA-PRESERVATION.md (%.0fs)\n", time.Since(start).Seconds())
}

// encode runs the real product pipeline on src and returns the output path.
func encode(src string, cfg encoder.EncodingConfig) string {
	mi, err := analyzer.Analyze(src)
	must(err)
	sel, err := encoder.NewDetector(ffmpeg).Select(cfg.VideoCodec, "cpu", true)
	must(err)
	out := filepath.Join(workDir, "out_"+strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))+".mp4")
	build, err := encoder.BuildCommand(src, out, mi, cfg, sel)
	must(err)
	run, err := ffmpegx.Start(ffmpeg, build.Args, nil, func(line string) { fmt.Println("  ffmpeg:", line) })
	must(err)
	if werr := run.Wait(); werr != nil {
		fmt.Println("  FFmpeg args:", strings.Join(build.Args, " "))
		fmt.Println("  stderr tail:", strings.Join(run.ErrTail(), " | "))
		must(werr)
	}
	return out
}

// compare probes source and output and records the named preservation check.
func compare(r *result, name string, src, out *media.MediaInfo, ok bool, detail string) {
	if !ok && detail == "" {
		detail = "not preserved"
	}
	r.checks = append(r.checks, check{name: name, pass: ok, detail: detail})
}

func caseHDR10() result {
	r := result{name: "HDR10 + 10-bit（H.265）"}
	src := filepath.Join(workDir, "hdr10_source.mkv")
	must(exec.Command("ffmpeg", "-y", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=640x360:rate=30:duration=3",
		"-vf", "format=yuv420p10le",
		"-c:v", "libx265", "-preset", "ultrafast",
		"-x265-params", "log-level=error:colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc",
		src).Run())

	out := encode(src, encoder.BuildSimpleConfig(mustAnalyze(src), "h265", "mid", "mp4"))
	smi, omi := mustAnalyze(src), mustAnalyze(out)

	compare(&r, "10-bit 保留", smi, omi, omi.Video.BitDepth >= 10,
		fmt.Sprintf("%d-bit", omi.Video.BitDepth))
	compare(&r, "HDR10 色彩标记", smi, omi, omi.Video.HDR == "HDR10",
		fmt.Sprintf("trc=%s", omi.Video.ColorTransfer))
	compare(&r, "BT.2020 色域", smi, omi, omi.Video.ColorPrimaries == "bt2020",
		omi.Video.ColorPrimaries)
	return r
}

func caseTenBitSDR() result {
	r := result{name: "10-bit SDR（H.265）"}
	src := filepath.Join(workDir, "sdr10_source.mkv")
	must(exec.Command("ffmpeg", "-y", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=640x360:rate=30:duration=3",
		"-vf", "format=yuv420p10le",
		"-c:v", "libx265", "-preset", "ultrafast", "-x265-params", "log-level=error",
		src).Run())

	out := encode(src, encoder.BuildSimpleConfig(mustAnalyze(src), "h265", "mid", "mp4"))
	omi := mustAnalyze(out)
	compare(&r, "10-bit 保留", nil, omi, omi.Video.BitDepth >= 10,
		fmt.Sprintf("%d-bit", omi.Video.BitDepth))
	return r
}

func caseMultiAudio() result {
	r := result{name: "多音轨 + 语言标记"}
	src := filepath.Join(workDir, "multiaudio_source.mkv")
	must(exec.Command("ffmpeg", "-y", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=640x360:rate=30:duration=3",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=3",
		"-f", "lavfi", "-i", "sine=frequency=880:duration=3",
		"-map", "0:v", "-map", "1:a", "-map", "2:a",
		"-c:v", "libx264", "-preset", "ultrafast",
		"-c:a", "aac",
		"-metadata:s:a:0", "language=chi", "-metadata:s:a:0", "title=中文",
		"-metadata:s:a:1", "language=eng", "-metadata:s:a:1", "title=English",
		src).Run())

	out := encode(src, encoder.BuildSimpleConfig(mustAnalyze(src), "h264", "mid", "mp4"))
	smi, omi := mustAnalyze(src), mustAnalyze(out)

	compare(&r, "音轨数量", smi, omi, len(omi.Audio) == len(smi.Audio),
		fmt.Sprintf("%d → %d", len(smi.Audio), len(omi.Audio)))
	langsKept := 0
	for _, sa := range smi.Audio {
		for _, oa := range omi.Audio {
			if oa.Language == sa.Language {
				langsKept++
				break
			}
		}
	}
	compare(&r, "语言标记保留", smi, omi, langsKept == len(smi.Audio),
		fmt.Sprintf("%d/%d", langsKept, len(smi.Audio)))
	return r
}

func caseSubtitlesChapters() result {
	r := result{name: "字幕 + 章节（MKV 源）"}
	src := filepath.Join(workDir, "subs_source.mkv")
	srt := filepath.Join(workDir, "subs.srt")
	must(os.WriteFile(srt, []byte("1\n00:00:00,000 --> 00:00:02,000\nHello VideoDelite\n\n2\n00:00:02,000 --> 00:00:03,000\n第二行字幕\n"), 0o644))
	must(exec.Command("ffmpeg", "-y", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=640x360:rate=30:duration=3",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=3",
		"-f", "srt", "-i", srt,
		"-map", "0:v", "-map", "1:a", "-map", "2:s",
		"-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac", "-c:s", "srt",
		"-metadata:s:s:0", "language=chi",
		src).Run())
	// chapters via metadata file
	chmeta := filepath.Join(workDir, "chapters.txt")
	must(os.WriteFile(chmeta, []byte(";FFMETADATA1\n[CHAPTER]\nTIMEBASE=1/1000\nSTART=0\nEND=2000\ntitle=第一章\n[CHAPTER]\nTIMEBASE=1/1000\nSTART=2000\nEND=3000\ntitle=第二章\n"), 0o644))
	src2 := filepath.Join(workDir, "subs_chapters_source.mkv")
	must(exec.Command("ffmpeg", "-y", "-hide_banner", "-loglevel", "error",
		"-i", src, "-i", chmeta, "-map_metadata", "1", "-codec", "copy", src2).Run())

	out := encode(src2, encoder.BuildSimpleConfig(mustAnalyze(src2), "h264", "mid", "mkv"))
	smi, omi := mustAnalyze(src2), mustAnalyze(out)

	compare(&r, "字幕轨道（MKV）", smi, omi, len(omi.Subtitles) == len(smi.Subtitles),
		fmt.Sprintf("%d → %d", len(smi.Subtitles), len(omi.Subtitles)))
	subLang := ""
	for _, s := range omi.Subtitles {
		subLang = s.Language
	}
	compare(&r, "字幕语言标记", smi, omi, len(omi.Subtitles) > 0 && smi.Subtitles[0].Language == subLang,
		subLang)
	compare(&r, "章节保留", smi, omi, omi.Chapters == smi.Chapters,
		fmt.Sprintf("%d → %d", smi.Chapters, omi.Chapters))
	return r
}

func caseRotation() result {
	r := result{name: "旋转元数据（手机竖拍）"}
	src := filepath.Join(workDir, "rot_source.mp4")
	must(exec.Command("ffmpeg", "-y", "-hide_banner", "-loglevel", "error",
		// -display_rotation is an INPUT option: it must precede -i.
		"-display_rotation", "90",
		"-f", "lavfi", "-i", "testsrc2=size=640x360:rate=30:duration=3",
		"-c:v", "libx264", "-preset", "ultrafast",
		src).Run())

	out := encode(src, encoder.BuildSimpleConfig(mustAnalyze(src), "h264", "mid", "mp4"))
	smi, omi := mustAnalyze(src), mustAnalyze(out)
	compare(&r, "旋转元数据 90°", smi, omi, omi.Rotation == smi.Rotation,
		fmt.Sprintf("%d° → %d°", smi.Rotation, omi.Rotation))
	return r
}

func mustAnalyze(p string) *media.MediaInfo {
	mi, err := analyzer.Analyze(p)
	must(err)
	return mi
}

func render(results []result, elapsed time.Duration) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# VideoDelite Phase 8 媒体保留 QA 矩阵\n\n")
	fmt.Fprintf(&b, "> 由 `go run ./tools/qamatrix` 于 %s 实测生成。\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&b, "> 所有用例走产品真实管线（Analyzer → BuildSimpleConfig → BuildCommand → ffmpegx）。\n\n")
	pass, total := 0, 0
	for _, r := range results {
		fmt.Fprintf(&b, "## %s\n\n| 检查项 | 结果 | 详情 |\n|---|---|---|\n", r.name)
		for _, c := range r.checks {
			total++
			mark := "❌ FAIL"
			if c.pass {
				mark = "✅ PASS"
				pass++
			}
			fmt.Fprintf(&b, "| %s | %s | %s |\n", c.name, mark, c.detail)
		}
		fmt.Fprintf(&b, "\n")
	}
	fmt.Fprintf(&b, "**汇总： %d/%d 项通过（%.0f 秒实测）**\n", pass, total, elapsed.Seconds())
	return b.String()
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
