package keys

import (
	"fmt"
	"strings"

	"github.com/Kazzer/zboard-merc/internal/hidrd"
)

const (
	pageKeyboard = 0x0007
	pageConsumer = 0x000C
)

type Key struct {
	Name       string
	Usage      hidrd.Usage
	Linux      string
	Scan       uint16
	Extended   bool
	VK         uint8
	ScanShared bool
}

func kbd(name string, usage uint16, linux string, scan uint16, ext bool, vk uint8) Key {
	return Key{Name: name, Usage: hidrd.Usage(uint32(pageKeyboard)<<16 | uint32(usage)), Linux: linux, Scan: scan, Extended: ext, VK: vk}
}

func shared(k Key) Key {
	k.ScanShared = true
	return k
}

func cons(name string, usage uint16, linux string, vk uint8) Key {
	return Key{Name: name, Usage: hidrd.Usage(uint32(pageConsumer)<<16 | uint32(usage)), Linux: linux, VK: vk}
}

var All = []Key{
	kbd("a", 0x04, "KEY_A", 0x1E, false, 0x41), kbd("b", 0x05, "KEY_B", 0x30, false, 0x42), kbd("c", 0x06, "KEY_C", 0x2E, false, 0x43),
	kbd("d", 0x07, "KEY_D", 0x20, false, 0x44), kbd("e", 0x08, "KEY_E", 0x12, false, 0x45), kbd("f", 0x09, "KEY_F", 0x21, false, 0x46),
	kbd("g", 0x0A, "KEY_G", 0x22, false, 0x47), kbd("h", 0x0B, "KEY_H", 0x23, false, 0x48), kbd("i", 0x0C, "KEY_I", 0x17, false, 0x49),
	kbd("j", 0x0D, "KEY_J", 0x24, false, 0x4A), kbd("k", 0x0E, "KEY_K", 0x25, false, 0x4B), kbd("l", 0x0F, "KEY_L", 0x26, false, 0x4C),
	kbd("m", 0x10, "KEY_M", 0x32, false, 0x4D), kbd("n", 0x11, "KEY_N", 0x31, false, 0x4E), kbd("o", 0x12, "KEY_O", 0x18, false, 0x4F),
	kbd("p", 0x13, "KEY_P", 0x19, false, 0x50), kbd("q", 0x14, "KEY_Q", 0x10, false, 0x51), kbd("r", 0x15, "KEY_R", 0x13, false, 0x52),
	kbd("s", 0x16, "KEY_S", 0x1F, false, 0x53), kbd("t", 0x17, "KEY_T", 0x14, false, 0x54), kbd("u", 0x18, "KEY_U", 0x16, false, 0x55),
	kbd("v", 0x19, "KEY_V", 0x2F, false, 0x56), kbd("w", 0x1A, "KEY_W", 0x11, false, 0x57), kbd("x", 0x1B, "KEY_X", 0x2D, false, 0x58),
	kbd("y", 0x1C, "KEY_Y", 0x15, false, 0x59), kbd("z", 0x1D, "KEY_Z", 0x2C, false, 0x5A),
	kbd("1", 0x1E, "KEY_1", 0x02, false, 0x31), kbd("2", 0x1F, "KEY_2", 0x03, false, 0x32), kbd("3", 0x20, "KEY_3", 0x04, false, 0x33),
	kbd("4", 0x21, "KEY_4", 0x05, false, 0x34), kbd("5", 0x22, "KEY_5", 0x06, false, 0x35), kbd("6", 0x23, "KEY_6", 0x07, false, 0x36),
	kbd("7", 0x24, "KEY_7", 0x08, false, 0x37), kbd("8", 0x25, "KEY_8", 0x09, false, 0x38), kbd("9", 0x26, "KEY_9", 0x0A, false, 0x39),
	kbd("0", 0x27, "KEY_0", 0x0B, false, 0x30),
	kbd("enter", 0x28, "KEY_ENTER", 0x1C, false, 0x0D), kbd("escape", 0x29, "KEY_ESC", 0x01, false, 0x1B),
	kbd("backspace", 0x2A, "KEY_BACKSPACE", 0x0E, false, 0x08), kbd("tab", 0x2B, "KEY_TAB", 0x0F, false, 0x09),
	kbd("space", 0x2C, "KEY_SPACE", 0x39, false, 0x20), kbd("minus", 0x2D, "KEY_MINUS", 0x0C, false, 0xBD),
	kbd("equal", 0x2E, "KEY_EQUAL", 0x0D, false, 0xBB), kbd("left-bracket", 0x2F, "KEY_LEFTBRACE", 0x1A, false, 0xDB),
	kbd("right-bracket", 0x30, "KEY_RIGHTBRACE", 0x1B, false, 0xDD), kbd("backslash", 0x31, "KEY_BACKSLASH", 0x2B, false, 0xDC),
	kbd("semicolon", 0x33, "KEY_SEMICOLON", 0x27, false, 0xBA), kbd("apostrophe", 0x34, "KEY_APOSTROPHE", 0x28, false, 0xDE),
	kbd("grave", 0x35, "KEY_GRAVE", 0x29, false, 0xC0), kbd("comma", 0x36, "KEY_COMMA", 0x33, false, 0xBC),
	kbd("period", 0x37, "KEY_DOT", 0x34, false, 0xBE), kbd("slash", 0x38, "KEY_SLASH", 0x35, false, 0xBF),
	kbd("caps-lock", 0x39, "KEY_CAPSLOCK", 0x3A, false, 0x14),
	kbd("f1", 0x3A, "KEY_F1", 0x3B, false, 0x70), kbd("f2", 0x3B, "KEY_F2", 0x3C, false, 0x71), kbd("f3", 0x3C, "KEY_F3", 0x3D, false, 0x72),
	kbd("f4", 0x3D, "KEY_F4", 0x3E, false, 0x73), kbd("f5", 0x3E, "KEY_F5", 0x3F, false, 0x74), kbd("f6", 0x3F, "KEY_F6", 0x40, false, 0x75),
	kbd("f7", 0x40, "KEY_F7", 0x41, false, 0x76), kbd("f8", 0x41, "KEY_F8", 0x42, false, 0x77), kbd("f9", 0x42, "KEY_F9", 0x43, false, 0x78),
	kbd("f10", 0x43, "KEY_F10", 0x44, false, 0x79), kbd("f11", 0x44, "KEY_F11", 0x57, false, 0x7A), kbd("f12", 0x45, "KEY_F12", 0x58, false, 0x7B),
	kbd("print-screen", 0x46, "KEY_SYSRQ", 0x37, true, 0x2C), kbd("scroll-lock", 0x47, "KEY_SCROLLLOCK", 0x46, false, 0x91),
	shared(kbd("pause", 0x48, "KEY_PAUSE", 0x45, false, 0x13)),
	kbd("insert", 0x49, "KEY_INSERT", 0x52, true, 0x2D), kbd("home", 0x4A, "KEY_HOME", 0x47, true, 0x24),
	kbd("page-up", 0x4B, "KEY_PAGEUP", 0x49, true, 0x21), kbd("delete", 0x4C, "KEY_DELETE", 0x53, true, 0x2E),
	kbd("end", 0x4D, "KEY_END", 0x4F, true, 0x23), kbd("page-down", 0x4E, "KEY_PAGEDOWN", 0x51, true, 0x22),
	kbd("right", 0x4F, "KEY_RIGHT", 0x4D, true, 0x27), kbd("left", 0x50, "KEY_LEFT", 0x4B, true, 0x25),
	kbd("down", 0x51, "KEY_DOWN", 0x50, true, 0x28), kbd("up", 0x52, "KEY_UP", 0x48, true, 0x26),
	shared(kbd("num-lock", 0x53, "KEY_NUMLOCK", 0x45, true, 0x90)),
	kbd("kp-slash", 0x54, "KEY_KPSLASH", 0x35, true, 0x6F), kbd("kp-asterisk", 0x55, "KEY_KPASTERISK", 0x37, false, 0x6A),
	kbd("kp-minus", 0x56, "KEY_KPMINUS", 0x4A, false, 0x6D), kbd("kp-plus", 0x57, "KEY_KPPLUS", 0x4E, false, 0x6B),
	kbd("kp-enter", 0x58, "KEY_KPENTER", 0x1C, true, 0x0D),
	kbd("kp-1", 0x59, "KEY_KP1", 0x4F, false, 0x61), kbd("kp-2", 0x5A, "KEY_KP2", 0x50, false, 0x62), kbd("kp-3", 0x5B, "KEY_KP3", 0x51, false, 0x63),
	kbd("kp-4", 0x5C, "KEY_KP4", 0x4B, false, 0x64), kbd("kp-5", 0x5D, "KEY_KP5", 0x4C, false, 0x65), kbd("kp-6", 0x5E, "KEY_KP6", 0x4D, false, 0x66),
	kbd("kp-7", 0x5F, "KEY_KP7", 0x47, false, 0x67), kbd("kp-8", 0x60, "KEY_KP8", 0x48, false, 0x68), kbd("kp-9", 0x61, "KEY_KP9", 0x49, false, 0x69),
	kbd("kp-0", 0x62, "KEY_KP0", 0x52, false, 0x60), kbd("kp-period", 0x63, "KEY_KPDOT", 0x53, false, 0x6E),
	kbd("102nd", 0x64, "KEY_102ND", 0x56, false, 0xE2), kbd("menu", 0x65, "KEY_COMPOSE", 0x5D, true, 0x5D),
	kbd("kp-equal", 0x67, "KEY_KPEQUAL", 0x59, false, 0x0C),
	kbd("f13", 0x68, "KEY_F13", 0x64, false, 0x7C), kbd("f14", 0x69, "KEY_F14", 0x65, false, 0x7D), kbd("f15", 0x6A, "KEY_F15", 0x66, false, 0x7E),
	kbd("f16", 0x6B, "KEY_F16", 0x67, false, 0x7F), kbd("f17", 0x6C, "KEY_F17", 0x68, false, 0x80), kbd("f18", 0x6D, "KEY_F18", 0x69, false, 0x81),
	kbd("f19", 0x6E, "KEY_F19", 0x6A, false, 0x82), kbd("f20", 0x6F, "KEY_F20", 0x6B, false, 0x83), kbd("f21", 0x70, "KEY_F21", 0x6C, false, 0x84),
	kbd("f22", 0x71, "KEY_F22", 0x6D, false, 0x85), kbd("f23", 0x72, "KEY_F23", 0x6E, false, 0x86), kbd("f24", 0x73, "KEY_F24", 0x76, false, 0x87),
	kbd("kp-comma", 0x85, "KEY_KPCOMMA", 0x7E, false, 0xC2),
	kbd("intl1", 0x87, "KEY_RO", 0x73, false, 0xC1), kbd("intl2", 0x88, "KEY_KATAKANAHIRAGANA", 0x70, false, 0x15),
	kbd("intl3", 0x89, "KEY_YEN", 0x7D, false, 0xDC), kbd("intl4", 0x8A, "KEY_HENKAN", 0x79, false, 0x1C),
	kbd("intl5", 0x8B, "KEY_MUHENKAN", 0x7B, false, 0x1D), kbd("intl6", 0x8C, "KEY_KPJPCOMMA", 0x5C, false, 0),
	kbd("lang3", 0x92, "KEY_KATAKANA", 0x78, false, 0), kbd("lang4", 0x93, "KEY_HIRAGANA", 0x77, false, 0),
	kbd("execute", 0x74, "KEY_OPEN", 0, false, 0), kbd("help", 0x75, "KEY_HELP", 0, false, 0), kbd("props", 0x76, "KEY_PROPS", 0, false, 0),
	kbd("select", 0x77, "KEY_FRONT", 0, false, 0), kbd("stop", 0x78, "KEY_STOP", 0, false, 0), kbd("again", 0x79, "KEY_AGAIN", 0, false, 0),
	kbd("undo", 0x7A, "KEY_UNDO", 0, false, 0), kbd("cut", 0x7B, "KEY_CUT", 0, false, 0), kbd("copy", 0x7C, "KEY_COPY", 0, false, 0),
	kbd("paste", 0x7D, "KEY_PASTE", 0, false, 0), kbd("find", 0x7E, "KEY_FIND", 0, false, 0),
	kbd("left-ctrl", 0xE0, "KEY_LEFTCTRL", 0x1D, false, 0xA2), kbd("left-shift", 0xE1, "KEY_LEFTSHIFT", 0x2A, false, 0xA0),
	kbd("left-alt", 0xE2, "KEY_LEFTALT", 0x38, false, 0xA4), kbd("left-win", 0xE3, "KEY_LEFTMETA", 0x5B, true, 0x5B),
	kbd("right-ctrl", 0xE4, "KEY_RIGHTCTRL", 0x1D, true, 0xA3), kbd("right-shift", 0xE5, "KEY_RIGHTSHIFT", 0x36, false, 0xA1),
	kbd("right-alt", 0xE6, "KEY_RIGHTALT", 0x38, true, 0xA5), kbd("right-win", 0xE7, "KEY_RIGHTMETA", 0x5C, true, 0x5C),
	cons("next-track", 0xB5, "KEY_NEXTSONG", 0xB0), cons("previous-track", 0xB6, "KEY_PREVIOUSSONG", 0xB1),
	cons("stop-media", 0xB7, "KEY_STOPCD", 0xB2), cons("play-pause", 0xCD, "KEY_PLAYPAUSE", 0xB3),
	cons("mute", 0xE2, "KEY_MUTE", 0xAD), cons("volume-up", 0xE9, "KEY_VOLUMEUP", 0xAF), cons("volume-down", 0xEA, "KEY_VOLUMEDOWN", 0xAE),
	cons("media-select", 0x183, "KEY_CONFIG", 0xB5), cons("mail", 0x18A, "KEY_MAIL", 0xB4), cons("calculator", 0x192, "KEY_CALC", 0xB7),
	cons("my-computer", 0x194, "KEY_FILE", 0xB6), cons("browser-search", 0x221, "KEY_SEARCH", 0xAA),
	cons("browser-home", 0x223, "KEY_HOMEPAGE", 0xAC), cons("browser-back", 0x224, "KEY_BACK", 0xA6),
	cons("browser-forward", 0x225, "KEY_FORWARD", 0xA7), cons("browser-stop", 0x226, "KEY_STOP", 0xA9),
	cons("browser-refresh", 0x227, "KEY_REFRESH", 0xA8), cons("browser-favorites", 0x22A, "KEY_BOOKMARKS", 0xAB),
}

var (
	byName  = map[string]Key{}
	byUsage = map[hidrd.Usage]Key{}
	byLinux = map[string]Key{}
	byScan  = map[uint32]Key{}
	byVK    = map[uint8]Key{}
)

func init() {
	for _, k := range All {
		byName[k.Name] = k
		byUsage[k.Usage] = k
		if k.Linux != "" {
			if _, dup := byLinux[k.Linux]; !dup {
				byLinux[k.Linux] = k
			}
		}
		if k.Scan != 0 {
			byScan[scanKey(k.Scan, k.Extended)] = k
		}
		if k.VK != 0 && k.Scan == 0 {
			byVK[k.VK] = k
		}
	}
}

func scanKey(scan uint16, ext bool) uint32 {
	if ext {
		return uint32(scan) | 1<<16
	}
	return uint32(scan)
}

func ByName(name string) (Key, bool) {
	k, ok := byName[strings.ToLower(name)]
	return k, ok
}

func ByUsage(u hidrd.Usage) (Key, bool) {
	k, ok := byUsage[u]
	return k, ok
}

func ByLinuxCode(code uint16) (Key, bool) {
	for name, c := range LinuxCodes {
		if c == code {
			if k, ok := byLinux[name]; ok {
				return k, true
			}
		}
	}
	return Key{}, false
}

func ByScan(scan uint16, ext bool, vk uint8) (Key, bool) {
	if scan == 0 {
		k, ok := byVK[vk]
		return k, ok
	}
	if ext && (scan == 0x2A || scan == 0x36) {
		ext = false
	}
	k, ok := byScan[scanKey(scan, ext)]
	return k, ok
}

func (k Key) LinuxCode() (uint16, error) {
	c, ok := LinuxCodes[k.Linux]
	if !ok {
		return 0, fmt.Errorf("keys: %q has no Linux code", k.Name)
	}
	return c, nil
}

func (k Key) WindowsVisible() bool { return k.Scan != 0 || k.VK != 0 }
