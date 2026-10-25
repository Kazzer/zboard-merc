package merc

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/Kazzer/zboard-merc/internal/hidrd"
)

func TestStandInMatchesCapture(t *testing.T) {
	var want struct {
		Pad, Numpad, Top map[string]uint32
	}
	b, err := os.ReadFile("testdata/table2-usages.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	areas := map[Area]map[string]uint32{Pad: want.Pad, Numpad: want.Numpad}
	for _, k := range Keys {
		captured, ok := areas[k.Area]
		if !ok {
			continue
		}
		u, ok := captured[k.Legend]
		if !ok {
			t.Errorf("%s %q not in the capture", areaName(k.Area), k.Legend)
			continue
		}
		if hidrd.Usage(u) != k.StandIn {
			t.Errorf("%s %q stand-in %v, capture %v", areaName(k.Area), k.Legend, k.StandIn, hidrd.Usage(u))
		}
	}
	for legend, u := range want.Pad {
		if _, ok := ByStandIn(hidrd.Usage(u)); !ok && u != uint32(kbd(0x46)) {
			t.Errorf("captured pad %q usage %v has no model entry", legend, hidrd.Usage(u))
		}
	}
}

func TestStandInsAreDistinctPerInterface(t *testing.T) {
	seen := map[hidrd.Usage]string{}
	for _, k := range Keys {
		if k.Area == Top || k.StandIn == k.Legends {
			continue
		}
		if prev, dup := seen[k.StandIn]; dup {
			t.Errorf("stand-in %v used by %q and %q", k.StandIn, prev, k.Legend)
		}
		seen[k.StandIn] = k.Legend
	}
}

func TestReports(t *testing.T) {
	if got := LEDReport(true, false); got[0] != 2 || got[1] != 1 || got[2] != 0 {
		t.Errorf("enabled report %x", got)
	}
	if got := LEDReport(true, true); got[1] != 3 {
		t.Errorf("both report %x", got)
	}
	if got := TableReport(StandIn); got[1] != 0x0B || got[2] != 1 {
		t.Errorf("table report %x", got)
	}
	drops := 0
	for _, k := range Keys {
		if WindowsDropsInStandIn[k.StandIn] {
			drops++
		}
	}
	if drops != 11 {
		t.Errorf("%d keys dropped on Windows, capture says 11", drops)
	}
}

func areaName(a Area) string { return [...]string{"main", "pad", "numpad", "top"}[a] }

func TestSourceNames(t *testing.T) {
	seen := map[string]bool{}
	for _, k := range Keys {
		s := k.Source()
		if s == "" {
			continue
		}
		if strings.ContainsAny(s, " \t") || s != strings.ToLower(s) {
			t.Errorf("%s: source %q is not a plain lower-case name", k.Legend, s)
		}
		if seen[s] {
			t.Errorf("%s: source %q names two keys", k.Legend, s)
		}
		seen[s] = true
	}
	for legend, want := range map[string]string{"PRT SCR": "pad.prt-scr", "I": "hotkey.1", "Restore": "restore", "Z": "", "Mute": ""} {
		for _, k := range Keys {
			if k.Legend == legend && k.Source() != want {
				t.Errorf("%s: source %q, want %q", legend, k.Source(), want)
			}
		}
	}
}
