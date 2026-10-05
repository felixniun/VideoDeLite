// Package errs classifies failures into the frozen error taxonomy
// (plan §88). CPU fallback is only allowed for encoder-related classes
// (plan §34/§89).
package errs

import "strings"

const (
	InputFileError             = "InputFileError"
	OutputFileError            = "OutputFileError"
	EncoderUnavailable         = "EncoderUnavailable"
	EncoderInitializationFailed = "EncoderInitializationFailed"
	UnsupportedCodec           = "UnsupportedCodec"
	UnsupportedPixelFormat     = "UnsupportedPixelFormat"
	DriverError                = "DriverError"
	FFmpegArgumentError        = "FFmpegArgumentError"
	DiskFull                   = "DiskFull"
	PermissionDenied           = "PermissionDenied"
	ValidationFailed           = "ValidationFailed"
	Canceled                   = "Canceled"
	UnknownError               = "UnknownError"
)

// Info is a classified failure.
type Info struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// EncoderRelated reports whether this class may trigger the single automatic
// hardware → CPU fallback.
func (i Info) EncoderRelated() bool {
	switch i.Code {
	case EncoderUnavailable, EncoderInitializationFailed, DriverError, UnsupportedPixelFormat:
		return true
	}
	return false
}

// Classify inspects FFmpeg's stderr tail. Lines are checked from most
// specific to least specific.
func Classify(tail []string, fallbackMsg string) Info {
	text := ""
	for i := len(tail) - 1; i >= 0 && len(text) < 4000; i-- {
		text = tail[i] + "\n" + text
	}
	l := strings.ToLower(text)

	switch {
	case containsAny(l, "no space left", "disk full", "not enough space"):
		return Info{DiskFull, pickLine(tail, "space")}
	case containsAny(l, "permission denied", "access is denied", "access denied"):
		return Info{PermissionDenied, pickLine(tail, "denied")}
	case containsAny(l, "no such file or directory", "failed to open input", "cannot open", "invalid data found"):
		if strings.Contains(l, "output") || strings.Contains(l, "writing") {
			return Info{OutputFileError, pickLine(tail, "")}
		}
		return Info{InputFileError, pickLine(tail, "")}
	case containsAny(l, "error opening output", "unable to open output", "failed to open output", "cannot write"):
		return Info{OutputFileError, pickLine(tail, "output")}
	case containsAny(l,
		"cannot load nvcuda", "no capable devices found", "openencode session",
		"failed to create", "initializing encoder", "init_encoder", "encoder init",
		"no nvidia capable devices", "cuvid", "d3d11", "not support the required nvenc",
		"driver", "amf", "qsv"):
		return Info{EncoderInitializationFailed, pickLine(tail, "")}
	case containsAny(l, "unsupported pixel format", "incompatible pixel format", "pixel format"):
		return Info{UnsupportedPixelFormat, pickLine(tail, "pixel")}
	case containsAny(l, "unsupported codec", "unknown encoder", "codec not supported", "incompatible"):
		return Info{UnsupportedCodec, pickLine(tail, "codec")}
	case containsAny(l, "option not found", "invalid option", "unrecognized option", "error splitting the argument list", "invalid argument"):
		return Info{FFmpegArgumentError, pickLine(tail, "")}
	}
	msg := strings.TrimSpace(fallbackMsg)
	if msg == "" {
		msg = pickLine(tail, "")
	}
	if msg == "" {
		msg = "unknown encoding failure"
	}
	return Info{UnknownError, msg}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// pickLine returns the last stderr line containing hint, or the last line.
func pickLine(tail []string, hint string) string {
	if len(tail) == 0 {
		return ""
	}
	if hint != "" {
		for i := len(tail) - 1; i >= 0; i-- {
			if strings.Contains(strings.ToLower(tail[i]), hint) {
				return trim(tail[i])
			}
		}
	}
	for i := len(tail) - 1; i >= 0; i-- {
		if t := trim(tail[i]); t != "" {
			return t
		}
	}
	return ""
}

func trim(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[error]")
	s = strings.TrimSpace(s)
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}
