package profile

import (
	"strings"
	"testing"
	"time"

	"github.com/Kazzer/zboard-merc/internal/keys"
	"github.com/Kazzer/zboard-merc/internal/merc"
)

const sample = `
name: Doom
table: standin
triggers: [Doom.exe, doom]
keys:
  pad.1: f1
  pad.load: { chord: [left-ctrl, s] }
  pad.save: { macro: [ {tap: f5}, {wait: 200ms}, {press: enter}, {release: enter} ], mode: toggle, hold: 0 }
  pad.p: { chat: { open: t, text: "gg", send: enter } }
  hotkey.1: { launch: "C:\\Games\\doom.exe" }
  pad.caps: { layer: alt }
  backlight.colour: { mod: next }
  pad.tab: disable
layers:
  alt:
    pad.1: { tap: f11 }
  num-lock:
    num.7: kp-7
`

func TestParse(t *testing.T) {
	m, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if m.Table != merc.StandIn || len(m.Triggers) != 2 || m.Triggers[0] != "doom.exe" {
		t.Errorf("header %+v", m)
	}
	one, _ := Source("pad.1", merc.StandIn)
	if b := m.Base[one]; b.Remap == nil || b.Remap.Name != "f1" {
		t.Errorf("pad.1 → %+v", b)
	}
	load, _ := Source("pad.load", merc.StandIn)
	if b := m.Base[load]; len(b.Chord) != 2 || b.Chord[1].Name != "s" {
		t.Errorf("pad.load → %+v", b)
	}
	save, _ := Source("pad.save", merc.StandIn)
	if b := m.Base[save]; b.Macro == nil || b.Macro.Mode != Toggle || len(b.Macro.Steps) != 5 || b.Macro.Steps[2].Wait != 200*time.Millisecond {
		t.Errorf("pad.save → %+v", b.Macro)
	}
	p, _ := Source("pad.p", merc.StandIn)
	if b := m.Base[p]; b.Chat == nil || b.Chat.Text != "gg" || b.Chat.Send.Name != "enter" {
		t.Errorf("pad.p → %+v", b)
	}
	f13, _ := keys.ByName("f13")
	if b := m.Base[f13.Usage]; b.Launch == "" {
		t.Errorf("hotkey.1 → %+v", b)
	}
	tab, _ := Source("pad.tab", merc.StandIn)
	if !m.Base[tab].Disable {
		t.Error("pad.tab should be disabled")
	}
	if _, ok := m.Layers["alt"]; !ok {
		t.Error("layer alt missing")
	}
	seven, _ := Source("num.7", merc.StandIn)
	if b := m.Layers["num-lock"][seven]; b.Remap == nil || b.Remap.Name != "kp-7" {
		t.Errorf("num-lock num.7 → %+v", b)
	}
}

func TestSourceFollowsTable(t *testing.T) {
	w, _ := keys.ByName("w")
	home, _ := keys.ByName("browser-home")
	if u, _ := Source("pad.w", merc.Legend); u != w.Usage {
		t.Errorf("legend pad.w = %v", u)
	}
	if u, _ := Source("pad.w", merc.StandIn); u != home.Usage {
		t.Errorf("standin pad.w = %v", u)
	}
	if u, _ := Source("W", merc.StandIn); u != w.Usage {
		t.Errorf("plain w = %v", u)
	}
	if _, err := Source("pad.nope", merc.Legend); err == nil {
		t.Error("expected an error for an unknown board key")
	}
}

func TestErrors(t *testing.T) {
	for _, c := range []struct{ body, want string }{
		{"name: x\nkeys:\n  pad.1: { tap: f1, chord: [a] }\n", "exactly one"},
		{"name: x\ntable: three\n", "not legend"},
		{"name: x\nkeys:\n  pad.1: { macro: [ {wait: soon} ] }\n", "invalid duration"},
		{"name: x\nkeys:\n  pad.1: { macro: [ {tap: f1} ], mode: forever }\n", "once, repeat or toggle"},
		{"name: x\nkeys:\n  w: a\n  pad.w: b\n", "same key"},
		{"name: x\nleds: { enabled: blink }\n", "not mod, on, off"},
		{"name: x\nkeys:\n  f13: { }\n", "or a toggle"},
	} {
		_, err := Parse([]byte(c.body))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: got %v, want %q", c.body, err, c.want)
		}
	}
}

func TestMacroHold(t *testing.T) {
	for _, c := range []struct {
		binding string
		hold    time.Duration
	}{
		{"{ macro: [ {tap: a} ] }", DefaultTapHold},
		{"{ macro: [ {tap: a} ], hold: 100ms }", 100 * time.Millisecond},
		{"{ macro: [ {press: a}, {release: a} ], hold: 5ms }", 5 * time.Millisecond},
		{"{ macro: [ {tap: a, hold: 7ms} ], hold: 0 }", 7 * time.Millisecond},
	} {
		m, err := Parse([]byte("name: h\nkeys:\n  pad.1: " + c.binding + "\n"))
		if err != nil {
			t.Fatal(err)
		}
		u, _ := Source("pad.1", merc.Legend)
		steps := m.Base[u].Macro.Steps
		if len(steps) != 3 || !steps[0].Press || steps[1].Wait != c.hold || steps[2].Press {
			t.Errorf("%s → %+v", c.binding, steps)
		}
	}
	m, err := Parse([]byte("name: h\nkeys:\n  pad.1: { macro: [ {tap: a} ], hold: 0 }\n"))
	if err != nil {
		t.Fatal(err)
	}
	u, _ := Source("pad.1", merc.Legend)
	if steps := m.Base[u].Macro.Steps; len(steps) != 2 {
		t.Errorf("hold 0 → %+v", steps)
	}
	if _, err := Parse([]byte("name: h\nkeys:\n  pad.1: { macro: [ {tap: a} ], hold: soon }\n")); err == nil {
		t.Error("a bad hold should be refused")
	}
}

func TestMenu(t *testing.T) {
	for _, c := range []struct {
		body string
		menu bool
	}{
		{"keys: {}\n", true},
		{"triggers: [game.exe]\n", false},
		{"triggers: [game.exe]\nmenu: true\n", true},
		{"menu: false\n", false},
	} {
		m, err := Parse([]byte(c.body))
		if err != nil {
			t.Fatal(err)
		}
		if m.Menu != c.menu {
			t.Errorf("%q: menu %v", c.body, m.Menu)
		}
	}
}
