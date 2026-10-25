//go:build windows

package platwin

import (
	"unicode/utf16"
	"unsafe"
)

var (
	vkKeyScanEx       = user32.NewProc("VkKeyScanExW")
	mapVirtualKeyEx   = user32.NewProc("MapVirtualKeyExW")
	getKeyboardLayout = user32.NewProc("GetKeyboardLayout")
)

const (
	keyeventfUnicode = 0x0004
	vkShift, vkCtrl  = 0x10, 0x11
	vkAlt            = 0x12
)

type Stroke struct {
	VK, Scan         uint16
	Extended         bool
	Shift, Ctrl, Alt bool
	Unicode          uint16
}

func ForegroundLayout() uintptr {
	hwnd, _, _ := getForeground.Call()
	tid, _, _ := getWindowTID.Call(hwnd, 0)
	hkl, _, _ := getKeyboardLayout.Call(tid)
	return hkl
}

func Strokes(text string, hkl uintptr) []Stroke {
	var out []Stroke
	for _, r := range text {
		if r == '\n' || r == '\r' {
			r = '\r'
		}
		if r < 0x10000 {
			if v, _, _ := vkKeyScanEx.Call(uintptr(r), hkl); int16(v) != -1 {
				vk, mods := uint16(v)&0xFF, uint16(v)>>8
				sc, _, _ := mapVirtualKeyEx.Call(uintptr(vk), 4 /*MAPVK_VK_TO_VSC_EX*/, hkl)
				out = append(out, Stroke{VK: vk, Scan: uint16(sc) & 0xFF, Extended: uint16(sc)>>8 == 0xE0,
					Shift: mods&1 != 0, Ctrl: mods&2 != 0, Alt: mods&4 != 0})
				continue
			}
		}
		for _, u := range utf16.Encode([]rune{r}) {
			out = append(out, Stroke{Unicode: u})
		}
	}
	return out
}

func TypeText(text string) error {
	for _, s := range Strokes(text, ForegroundLayout()) {
		if err := sendStroke(s); err != nil {
			return err
		}
	}
	return nil
}

func sendStroke(s Stroke) error {
	if s.VK == 0 {
		for _, up := range []bool{false, true} {
			in := keybdInput{Type: 1, Scan: s.Unicode, Flags: keyeventfUnicode, ExtraInfo: InjectTag}
			if up {
				in.Flags |= keyeventfKeyUp
			}
			if err := send(in); err != nil {
				return err
			}
		}
		return nil
	}
	var mods []uint16
	for vk, on := range map[uint16]bool{vkShift: s.Shift, vkCtrl: s.Ctrl, vkAlt: s.Alt} {
		if on {
			mods = append(mods, vk)
		}
	}
	key := keybdInput{Type: 1, VK: s.VK, Scan: s.Scan, ExtraInfo: InjectTag}
	if s.Extended {
		key.Flags |= keyeventfExtendedKey
	}
	for _, vk := range mods {
		if err := send(keybdInput{Type: 1, VK: vk, ExtraInfo: InjectTag}); err != nil {
			return err
		}
	}
	if err := send(key); err != nil {
		return err
	}
	key.Flags |= keyeventfKeyUp
	if err := send(key); err != nil {
		return err
	}
	for i := len(mods) - 1; i >= 0; i-- {
		if err := send(keybdInput{Type: 1, VK: mods[i], Flags: keyeventfKeyUp, ExtraInfo: InjectTag}); err != nil {
			return err
		}
	}
	return nil
}

func send(in keybdInput) error {
	if n, _, e := sendInput.Call(1, uintptr(unsafe.Pointer(&in)), unsafe.Sizeof(in)); n != 1 {
		return e
	}
	return nil
}
