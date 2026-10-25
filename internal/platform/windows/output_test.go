//go:build windows

package platwin

import (
	"testing"

	"github.com/Kazzer/zboard-merc/internal/keys"
)

func TestInputFor(t *testing.T) {
	for _, c := range []struct {
		name  string
		down  bool
		vk    uint16
		scan  uint16
		flags uint32
	}{
		{"a", true, 0, 0x1E, keyeventfScanCode},
		{"a", false, 0, 0x1E, keyeventfScanCode | keyeventfKeyUp},
		{"left", true, 0, 0x4B, keyeventfScanCode | keyeventfExtendedKey},
		{"kp-8", true, 0, 0x48, keyeventfScanCode},
		{"num-lock", true, 0x90, 0x45, keyeventfExtendedKey},
		{"num-lock", false, 0x90, 0x45, keyeventfExtendedKey | keyeventfKeyUp},
		{"pause", true, 0x13, 0x45, 0},
		{"browser-home", true, 0xAC, 0, 0},
	} {
		k, ok := keys.ByName(c.name)
		if !ok {
			t.Fatalf("no key %q", c.name)
		}
		in, err := inputFor(k, c.down)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if in.VK != c.vk || in.Scan != c.scan || in.Flags != c.flags || in.ExtraInfo != InjectTag {
			t.Errorf("%s down=%v: vk %#x scan %#x flags %#x, want vk %#x scan %#x flags %#x", c.name, c.down, in.VK, in.Scan, in.Flags, c.vk, c.scan, c.flags)
		}
	}
	if _, err := inputFor(keys.Key{Name: "ghost"}, true); err == nil {
		t.Error("a key without a Windows form was accepted")
	}
}
