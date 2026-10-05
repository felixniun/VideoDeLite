package task

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"videodelite/internal/encoder"
	"videodelite/internal/media"
)

// State is the frozen task state set (plan §36).
type State string

const (
	StateWaiting    State = "Waiting"
	StatePreparing  State = "Preparing"
	StateEncoding   State = "Encoding"
	StatePaused     State = "Paused"
	StateValidating State = "Validating"
	StateCompleted  State = "Completed"
	StateCanceled   State = "Canceled"
	StateFailed     State = "Failed"
)

func (s State) Terminal() bool {
	return s == StateCompleted || s == StateCanceled || s == StateFailed
}

// CreateRequest is the single enqueue payload for both modes.
type CreateRequest struct {
	InputPath string `json:"inputPath"`
	Mode      string `json:"mode"` // simple | professional

	// Simple mode inputs
	Codec     string `json:"codec"`     // h264 | h265
	Quality   string `json:"quality"`   // low | mid | high
	Container string `json:"container"` // mp4 | mkv
	OutputDir string `json:"outputDir"`
	Encoder   string `json:"encoder"` // auto | cpu | nvenc | qsv | amf

	// Professional mode config (plan §85 EncodingConfig)
	Config encoder.EncodingConfig `json:"config"`
}

func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("t%d", time.Now().UnixNano())
	}
	return "t" + hex.EncodeToString(b)
}

// Task is the internal mutable task record. Guarded by Manager.mu.
type Task struct {
	ID        string
	State     State
	Request   CreateRequest
	CreatedAt time.Time
	StartedAt time.Time
	EndedAt   time.Time

	InputInfo *media.MediaInfo
	InfoErr   string

	OutputPath string // final destination
	TempDir    string

	EncoderLabel string
	EncoderArg   string
	EncoderFamily string
	Config       encoder.EncodingConfig

	// live progress
	Percent    float64
	Speed      float64
	FPS        float64
	OutSize    int64
	BitrateKbps float64
	ETASec     float64

	Warnings    []string
	ErrCode     string
	ErrMessage  string
	EncodedOnce bool // set when FFmpeg actually started (plan §55: running task)

	run          interface{ Kill() error; Suspend() error; Resume() error }
	fallbackUsed bool
	pausePending bool
	cancelReq    bool
	dispatched   bool // handed to a worker goroutine (may still be Waiting)
	lastEmit     time.Time
}

// TaskView is the immutable snapshot sent to the frontend.
type TaskView struct {
	ID          string   `json:"id"`
	State       string   `json:"state"`
	InputPath   string   `json:"inputPath"`
	InputName   string   `json:"inputName"`
	InputSize   int64    `json:"inputSize"`
	OutputPath  string   `json:"outputPath"`
	Mode        string   `json:"mode"`
	Codec       string   `json:"codec"`
	Container   string   `json:"container"`
	Quality     string   `json:"quality"`
	Encoder     string   `json:"encoder"`
	EncoderArg  string   `json:"encoderArg"`
	BitrateMbps float64  `json:"bitrateMbps"`
	Percent     float64  `json:"percent"`
	Speed       float64  `json:"speed"`
	FPS         float64  `json:"fps"`
	OutSize     int64    `json:"outSize"`
	BitrateKbps float64  `json:"bitrateKbps"`
	ETASec      float64  `json:"etaSec"`
	DurationSec float64  `json:"durationSec"`
	Warnings    []string `json:"warnings"`
	ErrCode     string   `json:"errCode"`
	ErrMessage  string   `json:"errMessage"`
	CreatedAt   int64    `json:"createdAt"`
	StartedAt   int64    `json:"startedAt"`
	EndedAt     int64    `json:"endedAt"`
	EncodeTime  float64  `json:"encodeTimeSec"`
	Fallback    bool     `json:"fallbackUsed"`
}

func (t *Task) view() *TaskView {
	v := &TaskView{
		ID:          t.ID,
		State:       string(t.State),
		InputPath:   t.Request.InputPath,
		Mode:        t.Request.Mode,
		Codec:       t.Config.VideoCodec,
		Container:   t.Config.Container,
		Quality:     t.Request.Quality,
		Encoder:     t.EncoderLabel,
		EncoderArg:  t.EncoderArg,
		BitrateMbps: t.Config.BitrateMbps,
		Percent:     t.Percent,
		Speed:       t.Speed,
		FPS:         t.FPS,
		OutSize:     t.OutSize,
		BitrateKbps: t.BitrateKbps,
		ETASec:      t.ETASec,
		Warnings:    t.Warnings,
		ErrCode:     t.ErrCode,
		ErrMessage:  t.ErrMessage,
		CreatedAt:   t.CreatedAt.UnixMilli(),
		OutputPath:  t.OutputPath,
		Fallback:    t.fallbackUsed,
	}
	if t.StartedAt != (time.Time{}) {
		v.StartedAt = t.StartedAt.UnixMilli()
	}
	if t.EndedAt != (time.Time{}) {
		v.EndedAt = t.EndedAt.UnixMilli()
		v.EncodeTime = t.EndedAt.Sub(t.StartedAt).Seconds()
	}
	if t.InputInfo != nil {
		v.InputName = t.InputInfo.FileName
		v.InputSize = t.InputInfo.FileSize
		v.DurationSec = t.InputInfo.Duration
	} else {
		v.InputName = baseNameOf(t.Request.InputPath)
		if st, err := statFile(t.Request.InputPath); err == nil {
			v.InputSize = st
		}
	}
	if v.Warnings == nil {
		v.Warnings = []string{}
	}
	return v
}
