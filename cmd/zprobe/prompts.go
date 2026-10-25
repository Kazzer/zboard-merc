//go:build linux || windows

package main

import (
	"fmt"
	"strings"
)

const padKeys = 34

var chords = [][2]string{
	{"chord-wa", "hold pad W, then press pad A, then release both"},
	{"chord-sd", "hold pad S, then press pad D, then release both"},
	{"chord-5", "hold pad Shift, pad Ctrl, pad Space and pad W, then press pad 1, then release all"},
	{"chord-7", "press and hold seven pad keys at once (e.g. 1 2 3 4 5 6 7), then release all"},
	{"chord-cross", "hold main-keyboard A, then press pad W, then release both"},
	{"hold-w", "hold pad W for three seconds, then release"},
}
var mainRef = []string{
	"A",
	"Enter",
	"Left Shift",
	"Left Ctrl",
	"Space",
	"F1",
	"Esc",
	"Up arrow",
	"Caps Lock",
	"Left Windows key",
}
var models = map[uint16]string{
	0x0100: "Zboard",
	0x0210: "Merc",
	0x0310: "Fang",
	0x0510: "Merc Stealth",
}
var numpad = []string{
	"Scroll Lock",
	"Pause",
	"/",
	"7",
	"8",
	"9",
	"*",
	"4",
	"5",
	"6",
	"-",
	"1",
	"2",
	"3",
	"+",
	"0",
	".",
	"Enter",
}
var topRow = []string{
	"Z key (opens Z Engine)",
	"Volume up",
	"Volume down",
	"Mute",
	"Previous track",
	"Play/Pause",
	"Stop",
	"Next track",
	"Hotkey I",
	"Hotkey II",
	"Hotkey III",
	"backlight colour key",
	"backlight intensity key",
}

type prompt struct {
	id, text string
	legend   bool
}

func prompts(prefix string, main bool) []prompt {
	var out []prompt
	if main {
		for i, k := range mainRef {
			out = append(out, prompt{fmt.Sprintf("main-%02d", i), fmt.Sprintf("main keyboard: press and release [%s]", k), false})
		}
	}
	for i := 1; i <= padKeys; i++ {
		out = append(out, prompt{fmt.Sprintf("pad-%02d", i), fmt.Sprintf("gaming pad: press and release pad key #%d of %d (reading order: top row left to right, then the next row)", i, padKeys), true})
	}
	for _, state := range []string{"off", "on"} {
		out = append(out, prompt{"numlock-" + state, fmt.Sprintf("press Num Lock until its LED is %s (not blocked)", strings.ToUpper(state)), false})
		for i, k := range numpad {
			out = append(out, prompt{fmt.Sprintf("np-%s-%02d", state, i), fmt.Sprintf("numpad (Num Lock %s): press and release [%s]", state, k), false})
		}
	}
	for i, k := range topRow {
		out = append(out, prompt{fmt.Sprintf("top-%02d", i), fmt.Sprintf("top row: press and release [%s]", k), false})
	}
	for _, c := range chords {
		out = append(out, prompt{c[0], c[1], false})
	}
	for i := range out {
		out[i].id = prefix + out[i].id
	}
	return out
}
