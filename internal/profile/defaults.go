package profile

import (
	"github.com/Kazzer/zboard-merc/internal/keys"
	"github.com/Kazzer/zboard-merc/internal/merc"
)

var windowKeys = map[string][]string{"Restore": {"left-win", "down"}, "Maximize": {"left-win", "up"}}

func StandInDefault() (base, numLock Layer) {
	base, numLock = Layer{}, Layer{}
	for _, k := range merc.Keys {
		legend, ok := keys.ByUsage(k.Legends)
		if !ok {
			continue
		}
		if k.Area == merc.Pad {
			if k.StandIn != k.Legends {
				base[k.StandIn] = Binding{Source: k.Source(), Remap: &legend}
			}
			continue
		}
		if k.Area == merc.Top {
			if names, ok := windowKeys[k.Legend]; ok {
				off, _ := merc.NumLockOff(k.StandIn)
				var chord []keys.Key
				for _, n := range names {
					c, _ := keys.ByName(n)
					chord = append(chord, c)
				}
				base[off] = Binding{Source: k.Source(), Chord: chord}
			}
			continue
		}
		for _, s := range merc.Shortcuts {
			if s.Legend != k.Legend {
				continue
			}
			numLock[k.StandIn] = Binding{Source: k.Source(), Remap: &legend}
			key, _ := keys.ByUsage(s.Key)
			b := Binding{Source: k.Source()}
			if s.Modifier == 0 {
				b.Remap = &key
			} else {
				mod, _ := keys.ByUsage(s.Modifier)
				b.Chord = []keys.Key{mod, key}
			}
			base[k.StandIn] = b
		}
	}
	return base, numLock
}
