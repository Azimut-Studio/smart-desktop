package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
)

func main() {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	fill := func(x, y, w, h int, c color.RGBA) {
		for j := y; j < y+h; j++ {
			for i := x; i < x+w; i++ {
				img.SetRGBA(i, j, c)
			}
		}
	}
	fill(1, 3, 30, 23, color.RGBA{32, 49, 75, 255})
	fill(3, 5, 26, 18, color.RGBA{55, 113, 172, 255})
	for _, p := range [][2]int{{5, 7}, {5, 14}, {13, 7}, {13, 14}} {
		fill(p[0], p[1], 4, 4, color.RGBA{225, 243, 255, 255})
	}
	fill(12, 26, 8, 3, color.RGBA{32, 49, 75, 255})
	fill(8, 29, 16, 2, color.RGBA{32, 49, 75, 255})
	fill(22, 16, 8, 13, color.RGBA{64, 182, 121, 255})
	fill(24, 18, 4, 3, color.RGBA{238, 255, 243, 255})
	fill(24, 24, 4, 3, color.RGBA{238, 255, 243, 255})
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, img); err != nil {
		panic(err)
	}
	var out bytes.Buffer
	for _, n := range []uint16{0, 1, 1} {
		if err := binary.Write(&out, binary.LittleEndian, n); err != nil {
			panic(err)
		}
	}
	out.Write([]byte{32, 32, 0, 0})
	for _, n := range []uint16{1, 32} {
		if err := binary.Write(&out, binary.LittleEndian, n); err != nil {
			panic(err)
		}
	}
	for _, n := range []uint32{uint32(pngData.Len()), 22} {
		if err := binary.Write(&out, binary.LittleEndian, n); err != nil {
			panic(err)
		}
	}
	out.Write(pngData.Bytes())
	if err := os.WriteFile("assets\\smart-desktop.ico", out.Bytes(), 0644); err != nil {
		panic(err)
	}
}
