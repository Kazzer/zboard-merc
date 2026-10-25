//go:build windows

package platwin

import "testing"

func TestStrokes(t *testing.T) {
	hkl, _, _ := getKeyboardLayout.Call(0)
	got := Strokes("aA1!\né", hkl)
	want := []Stroke{
		{VK: 0x41, Scan: 0x1E},
		{VK: 0x41, Scan: 0x1E, Shift: true},
		{VK: 0x31, Scan: 0x02},
		{VK: 0x31, Scan: 0x02, Shift: true},
		{VK: 0x0D, Scan: 0x1C},
		{Unicode: 0xE9},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d strokes: %+v", len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("stroke %d: %+v, want %+v", i, got[i], want[i])
		}
	}
}
