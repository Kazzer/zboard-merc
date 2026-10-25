package merc

import (
	"strings"

	"github.com/Kazzer/zboard-merc/internal/hidrd"
)

const (
	VendorID  = 0x1038
	ProductID = 0x0510
)

type Table uint8

const (
	Legend  Table = 1
	StandIn Table = 2
)

const (
	FeatureReportID = 2
	LEDEnabled      = 0x01
	LEDAction       = 0x02
	cmdMatrixTable  = 0x0B
)

func LEDReport(enabled, action bool) []byte {
	var mask byte
	if enabled {
		mask |= LEDEnabled
	}
	if action {
		mask |= LEDAction
	}
	return []byte{FeatureReportID, mask, 0}
}

func TableReport(t Table) []byte {
	return []byte{FeatureReportID, cmdMatrixTable, byte(t - 1)}
}

const (
	pageKeyboard = 0x0007
	pageConsumer = 0x000C
)

func kbd(u uint16) hidrd.Usage  { return hidrd.Usage(uint32(pageKeyboard)<<16 | uint32(u)) }
func cons(u uint16) hidrd.Usage { return hidrd.Usage(uint32(pageConsumer)<<16 | uint32(u)) }

type Area uint8

const (
	Main Area = iota
	Pad
	Numpad
	Top
)

type Key struct {
	Area    Area
	Legend  string
	Legends hidrd.Usage
	StandIn hidrd.Usage
}

var Keys = []Key{
	{Pad, "1", kbd(0x1E), kbd(0x59)}, {Pad, "2", kbd(0x1F), kbd(0x5A)}, {Pad, "3", kbd(0x20), kbd(0x5B)},
	{Pad, "4", kbd(0x21), kbd(0x5C)}, {Pad, "5", kbd(0x22), kbd(0x5D)}, {Pad, "6", kbd(0x23), kbd(0x5E)},
	{Pad, "7", kbd(0x24), kbd(0x5F)}, {Pad, "8", kbd(0x25), kbd(0x60)}, {Pad, "9", kbd(0x26), kbd(0x61)},
	{Pad, "0", kbd(0x27), kbd(0x62)}, {Pad, "-", kbd(0x2D), kbd(0x57)},
	{Pad, "LOAD", kbd(0x42), kbd(0x76)}, {Pad, "PRT SCR", kbd(0x46), kbd(0x46)}, {Pad, "SAVE", kbd(0x3E), kbd(0x79)},
	{Pad, "Caps", kbd(0x39), kbd(0x77)}, {Pad, "Tab", kbd(0x2B), cons(0x192)}, {Pad, "Alt", kbd(0xE2), cons(0x18A)},
	{Pad, "Shift", kbd(0xE1), kbd(0x54)}, {Pad, "Ctrl", kbd(0xE0), kbd(0x63)},
	{Pad, "Q", kbd(0x14), cons(0x227)}, {Pad, "W", kbd(0x1A), cons(0x223)}, {Pad, "E", kbd(0x08), cons(0x221)},
	{Pad, "A", kbd(0x04), cons(0x224)}, {Pad, "S", kbd(0x16), cons(0x226)}, {Pad, "D", kbd(0x07), cons(0x225)},
	{Pad, "R", kbd(0x15), cons(0x22A)}, {Pad, "T", kbd(0x17), kbd(0x6B)}, {Pad, "F", kbd(0x09), kbd(0x67)},
	{Pad, "G", kbd(0x0A), kbd(0x6C)}, {Pad, "V", kbd(0x19), kbd(0x6D)}, {Pad, "B", kbd(0x05), kbd(0x6E)},
	{Pad, "P", kbd(0x13), kbd(0x74)}, {Pad, "C", kbd(0x06), kbd(0x6F)}, {Pad, "Space", kbd(0x2C), kbd(0x55)},
	{Numpad, "/", kbd(0x54), kbd(0x78)}, {Numpad, "*", kbd(0x55), cons(0x194)}, {Numpad, "-", kbd(0x56), kbd(0x56)},
	{Numpad, "7", kbd(0x5F), kbd(0x7E)}, {Numpad, "8", kbd(0x60), kbd(0x4A)}, {Numpad, "9", kbd(0x61), kbd(0x4B)},
	{Numpad, "+", kbd(0x57), kbd(0x7D)}, {Numpad, "4", kbd(0x5C), kbd(0x49)}, {Numpad, "5", kbd(0x5D), kbd(0x4D)},
	{Numpad, "6", kbd(0x5E), kbd(0x4E)}, {Numpad, "1", kbd(0x59), kbd(0x4C)}, {Numpad, "2", kbd(0x5A), kbd(0x7B)},
	{Numpad, "3", kbd(0x5B), kbd(0x7C)}, {Numpad, "0", kbd(0x62), kbd(0x75)}, {Numpad, ".", kbd(0x63), kbd(0x7A)},
	{Numpad, "Enter", kbd(0x58), kbd(0x58)},
	{Top, "I", kbd(0x68), kbd(0x68)}, {Top, "II", kbd(0x69), kbd(0x69)}, {Top, "III", kbd(0x6A), kbd(0x6A)},
	{Top, "Restore", kbd(0x47), kbd(0x47)}, {Top, "Maximize", kbd(0x48), kbd(0x48)},
	{Top, "Z", cons(0x183), cons(0x183)},
	{Top, "Backlight intensity", kbd(0x72), kbd(0x72)}, {Top, "Backlight colour", kbd(0x71), kbd(0x71)},
	{Top, "Mute", cons(0xE2), cons(0xE2)}, {Top, "Volume down", cons(0xEA), cons(0xEA)}, {Top, "Volume up", cons(0xE9), cons(0xE9)},
	{Top, "Play/Pause", cons(0xCD), cons(0xCD)}, {Top, "Stop", cons(0xB7), cons(0xB7)},
	{Top, "Previous track", cons(0xB6), cons(0xB6)}, {Top, "Next track", cons(0xB5), cons(0xB5)},
}

type Shortcut struct {
	Legend   string
	Name     string
	Modifier hidrd.Usage
	Key      hidrd.Usage
}

var Shortcuts = []Shortcut{
	{"/", "Close", kbd(0xE2), kbd(0x3D)}, {"*", "My Computer", kbd(0xE3), kbd(0x08)}, {"-", "Show Desktop", kbd(0xE3), kbd(0x07)},
	{"7", "Find", kbd(0xE0), kbd(0x09)}, {"8", "Home", 0, kbd(0x4A)}, {"9", "Page Up", 0, kbd(0x4B)},
	{"+", "Paste", kbd(0xE0), kbd(0x19)}, {"4", "Insert", 0, kbd(0x49)}, {"5", "End", 0, kbd(0x4D)},
	{"6", "Page Down", 0, kbd(0x4E)}, {"1", "Delete", 0, kbd(0x4C)}, {"2", "Cut", kbd(0xE0), kbd(0x1B)},
	{"3", "Copy", kbd(0xE0), kbd(0x06)}, {"0", "Mark Left", kbd(0xE1), kbd(0x50)}, {".", "Mark Right", kbd(0xE5), kbd(0x4F)},
}

type Sequence struct {
	Legend string
	Chords [][]hidrd.Usage
}

var Sequences = []Sequence{
	{"Restore", [][]hidrd.Usage{{kbd(0xE2), kbd(0x2C)}, {kbd(0x28)}}},
	{"Maximize", [][]hidrd.Usage{{kbd(0xE2), kbd(0x2C)}, {kbd(0x52)}, {kbd(0x52)}, {kbd(0x28)}}},
}

const pageNumLockOff = 0xFF5A

func NumLockOff(u hidrd.Usage) (hidrd.Usage, bool) {
	for _, k := range Keys {
		if k.StandIn != u {
			continue
		}
		for _, q := range Sequences {
			if q.Legend == k.Legend {
				return hidrd.Usage(uint32(pageNumLockOff)<<16 | uint32(u.ID())), true
			}
		}
	}
	return 0, false
}

var WindowsDropsInStandIn = map[hidrd.Usage]bool{
	kbd(0x74): true, kbd(0x75): true, kbd(0x76): true, kbd(0x77): true, kbd(0x78): true, kbd(0x79): true,
	kbd(0x7A): true, kbd(0x7B): true, kbd(0x7C): true, kbd(0x7D): true, kbd(0x7E): true,
}

var WindowsDriverMap = map[hidrd.Usage]hidrd.Usage{
	kbd(0x74): kbd(0x87), kbd(0x75): kbd(0x88), kbd(0x76): kbd(0x89), kbd(0x77): kbd(0x8A),
	kbd(0x78): kbd(0x8B), kbd(0x79): kbd(0x8C), kbd(0x7A): kbd(0x92), kbd(0x7B): kbd(0x93),
	kbd(0x7C): kbd(0x70), kbd(0x7D): kbd(0x73), kbd(0x7E): kbd(0x85),
}

func ByStandIn(u hidrd.Usage) (Key, bool) {
	for _, k := range Keys {
		if k.StandIn == u && k.StandIn != k.Legends {
			return k, true
		}
	}
	return Key{}, false
}

func (k Key) Source() string {
	legend := strings.ReplaceAll(strings.ToLower(k.Legend), " ", "-")
	switch k.Area {
	case Pad:
		return "pad." + legend
	case Numpad:
		return "num." + legend
	}
	switch k.Legend {
	case "I":
		return "hotkey.1"
	case "II":
		return "hotkey.2"
	case "III":
		return "hotkey.3"
	case "Restore", "Maximize":
		return legend
	case "Backlight intensity":
		return "backlight.intensity"
	case "Backlight colour":
		return "backlight.colour"
	}
	return ""
}
