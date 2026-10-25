package tray

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
)

const size = 32

var (
	body   = color.NRGBA{0x2b, 0x2b, 0x2e, 0xff}
	absent = color.NRGBA{0xb0, 0x3a, 0x2b, 0xff}
	keyCap = color.NRGBA{0xb8, 0xb8, 0xbc, 0xff}
	ledOff = color.NRGBA{0x5a, 0x5a, 0x5e, 0xff}
	green  = color.NRGBA{0x3f, 0xb9, 0x50, 0xff}
	amber  = color.NRGBA{0xe3, 0xa0, 0x08, 0xff}
)

func PNG(enabled, action, attached bool) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	rect(img, 1, 10, 31, 29, pick(attached, body, absent))
	for row := 0; row < 3; row++ {
		for col := 0; col < 6; col++ {
			x, y := 4+col*4+row, 13+row*5
			if row == 2 && col >= 1 && col <= 4 {
				if col == 1 {
					rect(img, x, y, x+15, y+3, keyCap)
				}
				continue
			}
			rect(img, x, y, x+3, y+3, keyCap)
		}
	}
	disc(img, 11, 5, 3.2, pick(enabled, green, ledOff))
	disc(img, 21, 5, 3.2, pick(action, amber, ledOff))
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

func ICO(enabled, action, attached bool) []byte {
	p := PNG(enabled, action, attached)
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, []uint16{0, 1, 1})
	buf.Write([]byte{size, size, 0, 0})
	binary.Write(&buf, binary.LittleEndian, []uint16{1, 32})
	binary.Write(&buf, binary.LittleEndian, uint32(len(p)))
	binary.Write(&buf, binary.LittleEndian, uint32(6+16))
	buf.Write(p)
	return buf.Bytes()
}

func pick(on bool, yes, no color.NRGBA) color.NRGBA {
	if on {
		return yes
	}
	return no
}

func rect(img *image.NRGBA, x0, y0, x1, y1 int, c color.NRGBA) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
}

func disc(img *image.NRGBA, cx, cy int, r float64, c color.NRGBA) {
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := float64(x-cx)+0.5, float64(y-cy)+0.5
			if dx*dx+dy*dy <= r*r {
				img.SetNRGBA(x, y, c)
			}
		}
	}
}
