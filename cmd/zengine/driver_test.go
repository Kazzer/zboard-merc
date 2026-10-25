package main

import (
	"testing"

	"github.com/Kazzer/zboard-merc/internal/hidrd"
	"github.com/Kazzer/zboard-merc/internal/keys"
	"github.com/Kazzer/zboard-merc/internal/merc"
)

func TestWindowsDriverMap(t *testing.T) {
	if len(merc.WindowsDriverMap) != len(merc.WindowsDropsInStandIn) {
		t.Fatalf("%d mapped, %d dropped", len(merc.WindowsDriverMap), len(merc.WindowsDropsInStandIn))
	}
	seen := map[hidrd.Usage]bool{}
	for from, to := range merc.WindowsDriverMap {
		if !merc.WindowsDropsInStandIn[from] {
			t.Errorf("%v is mapped but not dropped", from)
		}
		k, ok := keys.ByUsage(to)
		if !ok || k.Scan == 0 {
			t.Errorf("%v maps to %v, which has no Windows scan code", from, to)
		}
		if seen[to] {
			t.Errorf("%v used twice", to)
		}
		seen[to] = true
		if _, clash := merc.WindowsDropsInStandIn[to]; clash {
			t.Errorf("%v maps onto a dropped usage", from)
		}
		for _, mk := range merc.Keys {
			if mk.StandIn == to || mk.Legends == to {
				t.Errorf("%v maps onto %v, which the %s key sends", from, to, mk.Legend)
			}
		}
	}
}
