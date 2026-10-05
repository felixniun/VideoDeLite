package encoder

import (
	"context"
	"strings"
	"sync"
	"time"

	"videodelite/internal/proc"
)

// EncoderOption describes one selectable encoder for one codec.
// Unavailable entries stay visible with a reason (plan §32: "Disabled + Reason").
type EncoderOption struct {
	ID        string `json:"id"`        // auto | cpu | nvenc | qsv | amf
	Label     string `json:"label"`     // display label
	Vendor    string `json:"vendor"`    // NVIDIA / Intel / AMD / CPU / -
	Kind      string `json:"kind"`      // hw | sw
	Available bool   `json:"available"`
	Reason    string `json:"reason"`    // empty when available
}

// Detector probes real encoder availability by running tiny test encodes.
// An encoder listed by FFmpeg may still fail against an old driver, so the
// probe is the source of truth (plan §97: capability detection).
type Detector struct {
	FFmpegBin string

	mu       sync.Mutex
	cache    map[string]bool  // encoderArg -> usable
	reasons  map[string]string
	inflight map[string]*sync.WaitGroup
}

func NewDetector(ffmpegBin string) *Detector {
	return &Detector{
		FFmpegBin: ffmpegBin,
		cache:     map[string]bool{},
		reasons:   map[string]string{},
		inflight:  map[string]*sync.WaitGroup{},
	}
}

var hwFamilies = []struct {
	id     string
	label  string
	vendor string
	kind   string
}{
	{"nvenc", "NVIDIA NVENC", "NVIDIA", "hw"},
	{"qsv", "Intel QSV", "Intel", "hw"},
	{"amf", "AMD AMF/VCN", "AMD", "hw"},
}

// encoderArg maps (family, codec) to the FFmpeg encoder name.
func encoderArg(family, codec string) string {
	switch family {
	case "nvenc":
		if codec == "h265" {
			return "hevc_nvenc"
		}
		return "h264_nvenc"
	case "qsv":
		if codec == "h265" {
			return "hevc_qsv"
		}
		return "h264_qsv"
	case "amf":
		if codec == "h265" {
			return "hevc_amf"
		}
		return "h264_amf"
	case "cpu":
		if codec == "h265" {
			return "libx265"
		}
		return "libx264"
	}
	return ""
}

// probe runs a minimal encode through the given encoder to verify the driver
// path actually works end to end.
func (d *Detector) probe(encoderName string) (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	args := []string{
		"-hide_banner", "-nostdin",
		"-f", "lavfi", "-i", "color=c=black:s=320x240:r=30:d=0.3",
		"-frames:v", "6",
		"-c:v", encoderName,
		"-f", "null", "-",
	}
	cmd := proc.CommandContext(ctx, d.FFmpegBin, args...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return true, ""
	}
	if ctx.Err() == context.DeadlineExceeded {
		return false, "probe timed out"
	}
	return false, firstMeaningfulLine(string(out))
}

func firstMeaningfulLine(out string) string {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "[lavf") || strings.HasPrefix(line, "[NULL") {
			continue
		}
		if len(line) > 160 {
			line = line[:160] + "…"
		}
		return line
	}
	return "encoder not usable"
}

// availability returns the cached probe result, probing on first use.
func (d *Detector) availability(encoderName string) (bool, string) {
	d.mu.Lock()
	if ok, seen := d.cache[encoderName]; seen {
		reason := d.reasons[encoderName]
		d.mu.Unlock()
		return ok, reason
	}
	if wg, running := d.inflight[encoderName]; running {
		d.mu.Unlock()
		wg.Wait()
		d.mu.Lock()
		ok, reason := d.cache[encoderName], d.reasons[encoderName]
		d.mu.Unlock()
		return ok, reason
	}
	wg := &sync.WaitGroup{}
	wg.Add(1)
	d.inflight[encoderName] = wg
	d.mu.Unlock()

	ok, reason := d.probe(encoderName)

	d.mu.Lock()
	d.cache[encoderName] = ok
	d.reasons[encoderName] = reason
	delete(d.inflight, encoderName)
	d.mu.Unlock()
	wg.Done()
	return ok, reason
}

// OptionsFor returns the full option list for a codec, auto first.
// Probes run concurrently so the UI gets everything in one round trip.
func (d *Detector) OptionsFor(codec string) []EncoderOption {
	type result struct {
		opt    EncoderOption
		ok     bool
		reason string
	}
	families := append([]struct {
		id     string
		label  string
		vendor string
		kind   string
	}{{"cpu", "CPU 软件编码", "CPU", "sw"}}, hwFamilies...)

	results := make([]result, len(families))
	var wg sync.WaitGroup
	for i, fam := range families {
		wg.Add(1)
		go func(i int, fam struct {
			id     string
			label  string
			vendor string
			kind   string
		}) {
			defer wg.Done()
			arg := encoderArg(fam.id, codec)
			ok, reason := d.availability(arg)
			results[i] = result{EncoderOption{ID: fam.id, Label: fam.label, Vendor: fam.vendor, Kind: fam.kind, Available: ok}, ok, reason}
		}(i, fam)
	}
	wg.Wait()

	opts := []EncoderOption{{
		ID: "auto", Label: "Auto", Vendor: "-", Kind: "auto",
		Available: true,
	}}
	for _, r := range results {
		o := r.opt
		if !o.Available {
			o.Reason = r.reason
		}
		opts = append(opts, o)
	}
	return opts
}

// SelectedEncoder is what the selector hands to the command builder.
type SelectedEncoder struct {
	Family string // nvenc | qsv | amf | cpu
	Arg    string // FFmpeg encoder name
	Label  string
}

// Select picks the encoder for a job (plan §33: hardware first, then CPU;
// no absolute vendor priority — dynamic by actual availability).
func (d *Detector) Select(codec, prefer string, allowHardware bool) (*SelectedEncoder, error) {
	if prefer == "auto" || prefer == "" {
		if allowHardware {
			for _, fam := range hwFamilies {
				arg := encoderArg(fam.id, codec)
				if ok, _ := d.availability(arg); ok {
					return &SelectedEncoder{Family: fam.id, Arg: arg, Label: fam.label}, nil
				}
			}
		}
		arg := encoderArg("cpu", codec)
		return &SelectedEncoder{Family: "cpu", Arg: arg, Label: "CPU 软件编码"}, nil
	}
	famLabel, vendor := "CPU 软件编码", "CPU"
	if prefer != "cpu" {
		for _, f := range hwFamilies {
			if f.id == prefer {
				famLabel, vendor = f.label, f.vendor
			}
		}
	}
	arg := encoderArg(prefer, codec)
	ok, reason := d.availability(arg)
	if !ok {
		return nil, &EncoderUnavailableError{Family: prefer, Reason: reason}
	}
	_ = vendor
	return &SelectedEncoder{Family: prefer, Arg: arg, Label: famLabel}, nil
}

type EncoderUnavailableError struct {
	Family string
	Reason string
}

func (e *EncoderUnavailableError) Error() string {
	return "encoder unavailable: " + e.Family + " (" + e.Reason + ")"
}

// IsHardware reports whether the family is a hardware encoder.
func IsHardware(family string) bool {
	return family == "nvenc" || family == "qsv" || family == "amf"
}
