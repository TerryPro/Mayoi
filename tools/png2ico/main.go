// Command png2ico converts a square PNG into a multi-size Windows .ico
// with PNG payloads, the same sizes mygo build uses.
package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
)

var sizes = []int{16, 24, 32, 48, 64, 128, 256}

func main() {
	if len(os.Args) != 3 {
		log.Fatal("usage: png2ico input.png output.ico")
	}
	src, err := os.ReadFile(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(src))
	if err != nil {
		log.Fatal(err)
	}
	images := make([][]byte, len(sizes))
	for i, s := range sizes {
		var p bytes.Buffer
		if err := png.Encode(&p, resize(img, s)); err != nil {
			log.Fatal(err)
		}
		images[i] = p.Bytes()
	}
	var out bytes.Buffer
	_ = binary.Write(&out, binary.LittleEndian, [3]uint16{0, 1, uint16(len(sizes))})
	offset := 6 + 16*len(sizes)
	for i, s := range sizes {
		dim := byte(s)
		if s >= 256 {
			dim = 0
		}
		out.Write([]byte{dim, dim, 0, 0})
		_ = binary.Write(&out, binary.LittleEndian, [2]uint16{1, 32})
		_ = binary.Write(&out, binary.LittleEndian, [2]uint32{uint32(len(images[i])), uint32(offset)})
		offset += len(images[i])
	}
	for _, p := range images {
		out.Write(p)
	}
	if err := os.WriteFile(os.Args[2], out.Bytes(), 0o644); err != nil {
		log.Fatal(err)
	}
}

func resize(img image.Image, size int) *image.NRGBA {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	if w == 0 || h == 0 {
		return dst
	}
	sx, sy := float64(w)/float64(size), float64(h)/float64(size)
	at := func(x, y int) (float64, float64, float64, float64) {
		c := color.NRGBAModel.Convert(img.At(b.Min.X+min(max(x, 0), w-1), b.Min.Y+min(max(y, 0), h-1))).(color.NRGBA)
		a := float64(c.A)
		return float64(c.R) * a, float64(c.G) * a, float64(c.B) * a, a
	}
	for y := range size {
		for x := range size {
			var r, g, bl, a, n float64
			if sx >= 1 {
				x0, x1 := int(float64(x)*sx), max(int(float64(x+1)*sx), int(float64(x)*sx)+1)
				y0, y1 := int(float64(y)*sy), max(int(float64(y+1)*sy), int(float64(y)*sy)+1)
				for yy := y0; yy < y1; yy++ {
					for xx := x0; xx < x1; xx++ {
						cr, cg, cb, ca := at(xx, yy)
						r, g, bl, a, n = r+cr, g+cg, bl+cb, a+ca, n+1
					}
				}
			} else {
				fx, fy := (float64(x)+0.5)*sx-0.5, (float64(y)+0.5)*sy-0.5
				x0, y0 := int(fx), int(fy)
				dx, dy := fx-float64(x0), fy-float64(y0)
				for _, s := range [4]struct {
					x, y int
					wt   float64
				}{{x0, y0, (1 - dx) * (1 - dy)}, {x0 + 1, y0, dx * (1 - dy)}, {x0, y0 + 1, (1 - dx) * dy}, {x0 + 1, y0 + 1, dx * dy}} {
					cr, cg, cb, ca := at(s.x, s.y)
					r, g, bl, a, n = r+cr*s.wt, g+cg*s.wt, bl+cb*s.wt, a+ca*s.wt, n+s.wt
				}
			}
			if a == 0 {
				continue
			}
			dst.SetNRGBA(x, y, color.NRGBA{R: uint8(r/a + 0.5), G: uint8(g/a + 0.5), B: uint8(bl/a + 0.5), A: uint8(a/n + 0.5)})
		}
	}
	return dst
}
