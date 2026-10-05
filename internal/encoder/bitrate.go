package encoder

import "math"

// Simple Mode bitrate policy, frozen by plan §17–§21:
//
//   - H.264 table below (Mbps), buckets = resolution class × FPS class
//   - H.265 = H.264 × 0.75, rounded to 0.5 Mbps granularity
//   - below 720p: 720p baseline scaled by pixel ratio, floor 1 Mbps
//   - above 4K:   4K baseline scaled by pixel ratio, capped (validated in P0)
//   - non-standard FPS keeps source FPS, nearest bucket for bitrate

type qualityIndex int

const (
	qualityLow qualityIndex = 0
	qualityMid qualityIndex = 1
	qualityHigh qualityIndex = 2
)

// h264Table[class][quality] in Mbps. class = resolution|fpsBucket.
var h264Table = map[string][3]float64{
	"720|24-30":  {1.8, 3, 4.5},
	"720|48-60":  {2.5, 4, 6},
	"1080|24-30": {3.5, 5, 8},
	"1080|48-60": {5, 7, 10},
	"1440|24-30": {6, 10, 16},
	"1440|48-60": {8, 14, 20},
	"2160|24-30": {10, 16, 25},
	"2160|48-60": {14, 20, 30},
}

const (
	// maxBitrateCap bounds the >4K proportional scaling (plan §20:
	// "同时设置合理上限，防止产生异常巨大的 bitrate").
	maxBitrateCapMbps = 120
	// minBitrateMbps floors the sub-720p proportional scaling (plan §21).
	minBitrateMbps = 1.0
)

// ResolutionClass classifies by the short side so portrait phone video
// (1080×1920) also lands in the 1080p bucket.
func ResolutionClass(width, height int) (class int) {
	short := min(width, height)
	switch {
	case short >= 2160:
		return 2160
	case short >= 1440:
		return 1440
	case short >= 1080:
		return 1080
	case short >= 720:
		return 720
	default:
		return short // sub-720p: caller applies pixel-ratio scaling
	}
}

// FPSBucket picks the nearest frozen FPS tier (plan §19/§22). Source FPS is
// never modified; this only selects the bitrate baseline.
func FPSBucket(fps float64) string {
	if fps > 40 { // nearest of 24–30 / 48–60
		return "48-60"
	}
	return "24-30"
}

func qualityOf(q string) qualityIndex {
	switch q {
	case "low":
		return qualityLow
	case "high":
		return qualityHigh
	default:
		return qualityMid
	}
}

// SimpleBitrateMbps computes the Simple Mode target bitrate in Mbps.
func SimpleBitrateMbps(width, height int, fps float64, codec, quality string) float64 {
	q := qualityOf(quality)
	class := ResolutionClass(width, height)
	bucket := FPSBucket(fps)
	k := bucketKey(class, bucket)

	var mbps float64
	switch {
	case class >= 2160 && class != 2160:
		// above 4K: 4K baseline × pixel ratio, capped
		base := h264Table[bucketKey(2160, bucket)][q]
		mbps = base * float64(width*height) / float64(2160*2160)
		mbps = math.Min(mbps, maxBitrateCapMbps)
	case class < 720:
		// below 720p: 720p baseline × pixel ratio, floor 1 Mbps
		base := h264Table[bucketKey(720, bucket)][q]
		pixels := float64(width * height)
		basePixels := float64(1280 * 720)
		mbps = base * (pixels / basePixels)
		mbps = math.Max(mbps, minBitrateMbps)
	default:
		mbps = h264Table[k][q]
	}

	if codec == "h265" || codec == "hevc" {
		// plan §18: H.265 ≈ H.264 × 0.75, 0.5 Mbps granularity
		mbps = math.Round(mbps*0.75*2) / 2
	}
	// Keep at least 0.5 Mbps so nothing degenerates to zero.
	return math.Max(mbps, 0.5)
}

func bucketKey(class int, bucket string) string {
	switch class {
	case 720, 1080, 1440, 2160:
		return itoa(class) + "|" + bucket
	default:
		// Unknown class falls back to the closest standard row.
		if class > 2160 {
			return "2160|" + bucket
		}
		return "720|" + bucket
	}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [8]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
