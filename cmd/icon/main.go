// Generates the app icon locally using only the Go standard library.
package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

func icon(n int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			var sums [4]int
			for sy := 0; sy < 4; sy++ {
				for sx := 0; sx < 4; sx++ {
					px := (float64(x) + (float64(sx)+.5)/4) / float64(n)
					py := (float64(y) + (float64(sy)+.5)/4) / float64(n)
					c := color.NRGBA{}
					dx := math.Max(math.Abs(px-.5)-.29, 0)
					dy := math.Max(math.Abs(py-.5)-.29, 0)
					if dx*dx+dy*dy < .16*.16 {
						c = color.NRGBA{25, 32, 47, 255}
					}
					if math.Hypot(px-.44, py-.45) < .265 && math.Hypot(px-.57, py-.33) > .225 {
						c = color.NRGBA{137, 178, 255, 255}
					}
					// Download arrow, deliberately broad enough to remain legible at 16px.
					if (px > .635 && px < .715 && py > .39 && py < .65) || (py >= .59 && py < .755 && math.Abs(px-.675) < (.755-py)*.85) {
						c = color.NRGBA{217, 232, 255, 255}
					}
					sums[0] += int(c.R)
					sums[1] += int(c.G)
					sums[2] += int(c.B)
					sums[3] += int(c.A)
				}
			}
			img.SetNRGBA(x, y, color.NRGBA{uint8(sums[0] / 16), uint8(sums[1] / 16), uint8(sums[2] / 16), uint8(sums[3] / 16)})
		}
	}
	return img
}
func main() {
	sizes := []int{16, 24, 32, 48, 64, 128, 256}
	var blobs [][]byte
	for _, n := range sizes {
		var b bytes.Buffer
		if e := png.Encode(&b, icon(n)); e != nil {
			panic(e)
		}
		blobs = append(blobs, b.Bytes())
	}
	if e := os.WriteFile("build/appicon.png", blobs[len(blobs)-1], 0644); e != nil {
		panic(e)
	}
	var ico bytes.Buffer
	binary.Write(&ico, binary.LittleEndian, uint16(0))
	binary.Write(&ico, binary.LittleEndian, uint16(1))
	binary.Write(&ico, binary.LittleEndian, uint16(len(sizes)))
	offset := 6 + 16*len(sizes)
	for i, n := range sizes {
		ico.Write([]byte{byte(n % 256), byte(n % 256), 0, 0})
		binary.Write(&ico, binary.LittleEndian, uint16(1))
		binary.Write(&ico, binary.LittleEndian, uint16(32))
		binary.Write(&ico, binary.LittleEndian, uint32(len(blobs[i])))
		binary.Write(&ico, binary.LittleEndian, uint32(offset))
		offset += len(blobs[i])
	}
	for _, b := range blobs {
		ico.Write(b)
	}
	if e := os.WriteFile("build/windows/icon.ico", ico.Bytes(), 0644); e != nil {
		panic(e)
	}
	if e := os.WriteFile("frontend/public/appicon.png", blobs[len(blobs)-1], 0644); e != nil {
		panic(e)
	}
}
