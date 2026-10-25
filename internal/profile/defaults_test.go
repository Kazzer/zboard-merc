package profile

import (
	"testing"

	"github.com/Kazzer/zboard-merc/internal/merc"
)

func TestStandInDefault(t *testing.T) {
	base, numLock := StandInDefault()
	w, _ := Source("pad.w", merc.StandIn)
	if b := base[w]; b.Remap == nil || b.Remap.Name != "w" {
		t.Errorf("pad.w → %+v", b)
	}
	seven, _ := Source("num.7", merc.StandIn)
	if b := base[seven]; len(b.Chord) != 2 || b.Chord[0].Name != "left-ctrl" || b.Chord[1].Name != "f" {
		t.Errorf("num.7 with Num Lock off → %+v", b)
	}
	if b := numLock[seven]; b.Remap == nil || b.Remap.Name != "kp-7" {
		t.Errorf("num.7 with Num Lock on → %+v", b)
	}
	eight, _ := Source("num.8", merc.StandIn)
	if b := base[eight]; b.Remap == nil || b.Remap.Name != "home" {
		t.Errorf("num.8 with Num Lock off → %+v", b)
	}
	minus, _ := Source("num.-", merc.StandIn)
	if b := base[minus]; len(b.Chord) != 2 || b.Chord[0].Name != "left-win" || b.Chord[1].Name != "d" {
		t.Errorf("num.- with Num Lock off → %+v", b)
	}
	if b := numLock[minus]; b.Remap == nil || b.Remap.Name != "kp-minus" {
		t.Errorf("num.- with Num Lock on → %+v", b)
	}
	restore, _ := Source("restore", merc.StandIn)
	if b := base[restore]; len(b.Chord) != 2 || b.Chord[0].Name != "left-win" || b.Chord[1].Name != "down" {
		t.Errorf("restore with Num Lock off → %+v", b)
	}
	if plain, _ := Source("scroll-lock", merc.StandIn); restore == plain {
		t.Error("restore should not be the plain Scroll Lock usage")
	}
	if _, err := Source("restore", merc.Legend); err == nil {
		t.Error("restore in a legend Mod should be refused")
	}
	if len(base) != 33+15+2 || len(numLock) != 15 {
		t.Errorf("base %d numLock %d bindings", len(base), len(numLock))
	}
}
