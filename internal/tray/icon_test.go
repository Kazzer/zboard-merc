package tray

import (
	"bytes"
	"image/png"
	"testing"
)

func TestIcons(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range [][3]bool{{false, false, true}, {true, false, true}, {false, true, true}, {true, true, true}, {false, false, false}} {
		p := PNG(c[0], c[1], c[2])
		img, err := png.Decode(bytes.NewReader(p))
		if err != nil {
			t.Fatalf("%v: %v", c, err)
		}
		if img.Bounds().Dx() != size || img.Bounds().Dy() != size {
			t.Errorf("%v: size %v", c, img.Bounds())
		}
		if seen[string(p)] {
			t.Errorf("%v: same image as another LED state", c)
		}
		seen[string(p)] = true
		ico := ICO(c[0], c[1], c[2])
		if len(ico) != 22+len(p) || ico[2] != 1 || ico[4] != 1 || ico[6] != size || !bytes.Equal(ico[22:], p) {
			t.Errorf("%v: ico header wrong", c)
		}
	}
}
