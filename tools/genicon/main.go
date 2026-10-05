// genicon renders build/appicon.png and build/windows/icon.ico for VideoDelite.
//
//	go run ./tools/genicon
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

func main() {
	const size = 512
	img := render(size)

	os.MkdirAll("build/windows", 0o755)
	must(os.WriteFile(filepath.Join("build", "appicon.png"), encodePNG(img, size), 0o644))

	// 256px PNG-in-ICO entry (Vista+).
	small := render(256)
	pngBytes := encodePNG(small, 256)
	ico := buildICO(pngBytes)
	must(os.WriteFile(filepath.Join("build", "windows", "icon.ico"), ico, 0o644))
	fmt.Println("wrote build/appicon.png and build/windows/icon.ico")
}

func encodePNG(img *image.RGBA, size int) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func buildICO(pngBytes []byte) []byte {
	var head, entry, data bytes.Buffer
	binary.Write(&head, binary.LittleEndian, uint16(0)) // reserved
	binary.Write(&head, binary.LittleEndian, uint16(1)) // type icon
	binary.Write(&head, binary.LittleEndian, uint16(1)) // count
	entry.WriteByte(0)                                  // 256 → 0
	entry.WriteByte(0)
	entry.WriteByte(0) // colors
	entry.WriteByte(0) // reserved
	binary.Write(&entry, binary.LittleEndian, uint16(1)) // planes
	binary.Write(&entry, binary.LittleEndian, uint16(32)) // bpp
	binary.Write(&entry, binary.LittleEndian, uint32(len(pngBytes)))
	binary.Write(&entry, binary.LittleEndian, uint32(22)) // offset after 6+16 bytes
	data.Write(head.Bytes())
	data.Write(entry.Bytes())
	data.Write(pngBytes)
	return data.Bytes()
}

// render draws the VideoDelite mark: rounded square with a blue→violet
// gradient (Apple system-blue family) and a white play triangle.
func render(size int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	radius := float64(size) * 0.225
	ss := 3 // supersampling
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			r, g, b, a := 0.0, 0.0, 0.0, 0.0
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					px := float64(x) + (float64(sx)+0.5)/float64(ss)
					py := float64(y) + (float64(sy)+0.5)/float64(ss)
					cr, cg, cb, ca := shade(px, py, size, radius)
					r += cr
					g += cg
					b += cb
					a += ca
				}
			}
			n := float64(ss * ss)
			img.Set(x, y, color.RGBA64{
				R: uint16(r / n * 257), G: uint16(g / n * 257),
				B: uint16(b / n * 257), A: uint16(a / n * 257),
			})
		}
	}
	return img
}

func shade(x, y float64, size int, radius float64) (r, g, b, a float64) {
	s := float64(size)
	// rounded-rect signed distance
	cx, cy := s/2, s/2
	hw, hh := s/2-2, s/2-2 // inset 2px so AA edge isn't clipped
	qx := math.Abs(x-cx) - (hw - radius)
	qy := math.Abs(y-cy) - (hh - radius)
	dist := math.Min(math.Max(qx, qy), 0) + math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) - radius
	if dist > 0 {
		return 0, 0, 0, 0
	}
	// vertical gradient #0A84FF → #5E5CE6
	t := y / s
	r = lerp(0x0A, 0x5E, t)
	g = lerp(0x84, 0x5C, t)
	b = lerp(0xFF, 0xE6, t)
	// play triangle (slightly right of center)
	if inTriangle(x, y, s*0.40, s*0.30, s*0.40, s*0.70, s*0.70, s*0.50) {
		r, g, b = 255, 255, 255
	}
	return r, g, b, 255
}

func inTriangle(x, y, x1, y1, x2, y2, x3, y3 float64) bool {
	d1 := sign(x, y, x1, y1, x2, y2)
	d2 := sign(x, y, x2, y2, x3, y3)
	d3 := sign(x, y, x3, y3, x1, y1)
	hasNeg := d1 < 0 || d2 < 0 || d3 < 0
	hasPos := d1 > 0 || d2 > 0 || d3 > 0
	return !(hasNeg && hasPos)
}

func sign(x1, y1, x2, y2, x3, y3 float64) float64 {
	return (x1-x3)*(y2-y3) - (x2-x3)*(y1-y3)
}

func lerp(a, b float64, t float64) float64 {
	return a + (b-a)*t
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
