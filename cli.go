package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"videodelite/internal/encoder"
	"videodelite/internal/task"
)

// cliDispatch handles headless operation for development and CI:
//
//	VideoDelite.exe --cli analyze <file...>
//	VideoDelite.exe --cli encode <file> [--codec h264|h265] [--quality low|mid|high]
//	                [--container mp4|mkv] [--encoder auto|cpu|nvenc|qsv|amf] [--out DIR]
//
// The CLI drives exactly the same manager the GUI uses, so a passing CLI run
// proves the real pipeline (plan §133 demo chain).
func cliDispatch() bool {
	args := os.Args
	if len(args) < 2 || args[1] != "--cli" {
		return false
	}
	rest := args[2:]
	if len(rest) == 0 {
		fmt.Fprintln(os.Stderr, "usage: --cli analyze <file...> | --cli encode <file> [flags]")
		os.Exit(2)
	}
	app, err := NewApp()
	if err != nil {
		fmt.Fprintln(os.Stderr, "startup failed:", err)
		os.Exit(1)
	}
	switch rest[0] {
	case "analyze":
		if len(rest) < 2 {
			fmt.Fprintln(os.Stderr, "analyze needs at least one file")
			os.Exit(2)
		}
		infos, err := app.AnalyzeFiles(rest[1:])
		if err != nil {
			fmt.Fprintln(os.Stderr, "analyze failed:", err)
			os.Exit(1)
		}
		for _, mi := range infos {
			fmt.Printf("%s\n  container=%s duration=%.2fs size=%d\n", mi.FileName, mi.Format, mi.Duration, mi.FileSize)
			if mi.Video != nil {
				fmt.Printf("  video=%s %dx%d@%.3f pix=%s depth=%d hdr=%s rot=%d\n",
					mi.Video.Codec, mi.Video.Width, mi.Video.Height, mi.Video.FPS,
					mi.Video.PixelFormat, mi.Video.BitDepth, mi.Video.HDR, mi.Rotation)
			}
			for _, a := range mi.Audio {
				fmt.Printf("  audio#%d=%s ch=%d lang=%s\n", a.Index, a.Codec, a.Channels, a.Language)
			}
			for _, s := range mi.Subtitles {
				fmt.Printf("  sub#%d=%s lang=%s\n", s.Index, s.Codec, s.Language)
			}
			fmt.Printf("  chapters=%d\n", mi.Chapters)
		}
	case "encode":
		codec, quality, container, enc, out := "h264", "mid", "mp4", "auto", ""
		mode, rc, br, maxbr, cq := "simple", "", 0.0, 0.0, 23
		file := ""
		for i := 1; i < len(rest); i++ {
			switch rest[i] {
			case "--codec":
				i++; codec = pick(rest, i)
			case "--quality":
				i++; quality = pick(rest, i)
			case "--container":
				i++; container = pick(rest, i)
			case "--encoder":
				i++; enc = pick(rest, i)
			case "--out":
				i++; out = pick(rest, i)
			case "--mode":
				i++; mode = pick(rest, i)
			case "--rc":
				i++; rc = pick(rest, i)
			case "--bitrate":
				i++; fmt.Sscanf(pick(rest, i), "%f", &br)
			case "--maxbitrate":
				i++; fmt.Sscanf(pick(rest, i), "%f", &maxbr)
			case "--cq":
				i++; fmt.Sscanf(pick(rest, i), "%d", &cq)
			default:
				if file == "" {
					file = rest[i]
				}
			}
		}
		if file == "" {
			fmt.Fprintln(os.Stderr, "encode needs a file")
			os.Exit(2)
		}
		var ids []string
		var err error
		if mode == "professional" {
			cfg := encoder.EncodingConfig{
				Container: container, VideoCodec: codec, Encoder: enc,
				AudioCodec: "aac", AudioBitrateKbps: 128, AudioRateControl: "cbr",
				PreserveMetadata: true, PreserveSubtitles: true,
				PreserveChapters: true, PreserveAudioTracks: true,
			}
			switch rc {
			case "cbr":
				cfg.RateControl, cfg.BitrateMbps = "cbr", br
			case "vbr":
				cfg.RateControl, cfg.BitrateMbps, cfg.MaxBitrateMbps = "vbr", br, maxbr
			case "cq":
				cfg.RateControl, cfg.Quality = "cq", cq
			default:
				cfg.RateControl, cfg.BitrateMbps = "simple", br
			}
			id, e := app.CreateProfessionalTask(task.CreateRequest{
				InputPath: file, Mode: "professional", OutputDir: out,
				Codec: codec, Quality: quality, Container: container, Encoder: enc,
				Config: cfg,
			})
			ids, err = []string{id}, e
		} else {
			ids, err = app.CreateSimpleTasks([]string{file}, codec, quality, container, out, enc)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "enqueue failed:", err)
			os.Exit(1)
		}
		fmt.Printf("queued %s mode=%s codec=%s rc=%s encoder=%s\n", file, mode, codec, rc, enc)
		exit := watchCLI(app, ids[0])
		os.Exit(exit)
	default:
		fmt.Fprintln(os.Stderr, "unknown cli command:", rest[0])
		os.Exit(2)
	}
	return true
}

func pick(rest []string, i int) string {
	if i < len(rest) {
		return rest[i]
	}
	return ""
}

func watchCLI(app *App, id string) int {
	last := ""
	for {
		time.Sleep(400 * time.Millisecond)
		var view *task.TaskView
		for _, t := range app.GetTasks() {
			if t.ID == id {
				view = t
				break
			}
		}
		if view == nil {
			fmt.Println("task disappeared")
			return 1
		}
		line := fmt.Sprintf("\r%-10s %5.1f%% speed=%.2fx out=%dMB", view.State, view.Percent*100, view.Speed, view.OutSize/1024/1024)
		if line != last {
			fmt.Print(line)
			last = line
		}
		if view.State == "Completed" {
			fmt.Printf("\ncompleted: %s (%d -> %d bytes)\n", view.OutputPath, view.InputSize, view.OutSize)
			for _, w := range view.Warnings {
				fmt.Println("warning:", w)
			}
			return 0
		}
		if view.State == "Failed" || view.State == "Canceled" {
			fmt.Printf("\n%s: %s %s\n", strings.ToLower(view.State), view.ErrCode, view.ErrMessage)
			return 1
		}
	}
}
