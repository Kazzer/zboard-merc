package engine

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Kazzer/zboard-merc/internal/keys"
	"github.com/Kazzer/zboard-merc/internal/merc"
	"github.com/Kazzer/zboard-merc/internal/profile"
)

type fakeOut struct{ log []string }

func (f *fakeOut) Key(k keys.Key, down bool) {
	f.log = append(f.log, fmt.Sprintf("%s%s", map[bool]string{true: "+", false: "-"}[down], k.Name))
}
func (f *fakeOut) Type(text string)     { f.log = append(f.log, "type:"+text) }
func (f *fakeOut) Launch(target string) { f.log = append(f.log, "launch:"+target) }
func (f *fakeOut) LEDs(enabled, action bool) {
	f.log = append(f.log, fmt.Sprintf("leds:%v,%v", enabled, action))
}
func (f *fakeOut) Table(t merc.Table)      { f.log = append(f.log, fmt.Sprintf("table:%d", t)) }
func (f *fakeOut) ModRequest(which string) { f.log = append(f.log, "mod:"+which) }
func (f *fakeOut) OpenUI()                 { f.log = append(f.log, "ui") }
func (f *fakeOut) take() string            { s := strings.Join(f.log, " "); f.log = nil; return s }

type fakeTimer struct {
	d  time.Duration
	fn func()
}

type fakeSched struct{ timers []*fakeTimer }

func (s *fakeSched) After(d time.Duration, fn func()) func() {
	tm := &fakeTimer{d, fn}
	s.timers = append(s.timers, tm)
	return func() { tm.fn = nil }
}

func (s *fakeSched) fire(t *testing.T) time.Duration {
	for i, tm := range s.timers {
		if tm.fn != nil {
			s.timers = append(s.timers[:i], s.timers[i+1:]...)
			tm.fn()
			return tm.d
		}
	}
	t.Fatal("no timer to fire")
	return 0
}

func (s *fakeSched) live() int {
	n := 0
	for _, tm := range s.timers {
		if tm.fn != nil {
			n++
		}
	}
	return n
}

func usage(t *testing.T, name string, table merc.Table) Event {
	u, err := profile.Source(name, table)
	if err != nil {
		t.Fatal(err)
	}
	return Event{Usage: u, Down: true}
}

func up(ev Event) Event { ev.Down = false; return ev }

func mod(t *testing.T, body string) *profile.Mod {
	m, err := profile.Parse([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestPassthroughAndRemap(t *testing.T) {
	out, sched := &fakeOut{}, &fakeSched{}
	e := New(out, sched)
	e.Handle(usage(t, "a", merc.Legend))
	e.Handle(up(usage(t, "a", merc.Legend)))
	if got := out.take(); got != "+a -a" {
		t.Errorf("bare passthrough: %s", got)
	}
	e.Activate(mod(t, "name: m\nkeys:\n  pad.1: f1\n  pad.tab: disable\n"), true)
	if got := out.take(); got != "leds:true,false" {
		t.Errorf("activate: %s", got)
	}
	e.Handle(usage(t, "pad.1", merc.Legend))
	e.Handle(usage(t, "pad.1", merc.Legend))
	e.Handle(up(usage(t, "pad.1", merc.Legend)))
	e.Handle(usage(t, "pad.tab", merc.Legend))
	e.Handle(up(usage(t, "pad.tab", merc.Legend)))
	if got := out.take(); got != "+f1 +f1 -f1" {
		t.Errorf("remap and disable: %s", got)
	}
}

func TestReleaseUsesBindingFromPress(t *testing.T) {
	out, sched := &fakeOut{}, &fakeSched{}
	e := New(out, sched)
	e.Activate(mod(t, "name: m\nkeys:\n  pad.1: f1\n"), true)
	e.Handle(usage(t, "pad.1", merc.Legend))
	e.Activate(nil, false)
	e.Handle(up(usage(t, "pad.1", merc.Legend)))
	if got := out.take(); got != "leds:true,false +f1 leds:false,false -f1" {
		t.Errorf("%s", got)
	}
}

func TestChordLayerAndLocks(t *testing.T) {
	out, sched := &fakeOut{}, &fakeSched{}
	e := New(out, sched)
	e.Activate(mod(t, `
name: m
keys:
  pad.load: { chord: [left-ctrl, s] }
  pad.caps: { layer: alt }
layers:
  alt:
    pad.load: f11
  num-lock:
    pad.load: f12
`), true)
	out.take()
	load := usage(t, "pad.load", merc.Legend)
	e.Handle(load)
	e.Handle(up(load))
	e.Handle(usage(t, "pad.caps", merc.Legend))
	e.Handle(load)
	e.Handle(up(load))
	e.Handle(up(usage(t, "pad.caps", merc.Legend)))
	e.SetLocks(true, false, false)
	e.Handle(load)
	e.Handle(up(load))
	if got := out.take(); got != "+left-ctrl +s -s -left-ctrl +f11 -f11 +f12 -f12" {
		t.Errorf("%s", got)
	}
}

func TestMacroModes(t *testing.T) {
	out, sched := &fakeOut{}, &fakeSched{}
	e := New(out, sched)
	e.Activate(mod(t, `
name: m
keys:
  pad.1: { macro: [ {tap: f1}, {wait: 30ms}, {tap: f2} ], hold: 0 }
  pad.2: { macro: [ {tap: x} ], mode: repeat, hold: 0 }
  pad.3: { macro: [ {tap: y} ], mode: toggle, hold: 0 }
`), true)
	out.take()
	one := usage(t, "pad.1", merc.Legend)
	e.Handle(one)
	if got := out.take(); got != "leds:true,true +f1 -f1" {
		t.Errorf("once, before the wait: %s", got)
	}
	if d := sched.fire(t); d != 30*time.Millisecond {
		t.Errorf("wait %v", d)
	}
	e.Handle(up(one))
	if got := out.take(); got != "+f2 -f2 leds:true,false" {
		t.Errorf("once, after the wait: %s", got)
	}
	two := usage(t, "pad.2", merc.Legend)
	e.Handle(two)
	sched.fire(t)
	sched.fire(t)
	e.Handle(up(two))
	sched.fire(t)
	if got := out.take(); got != "leds:true,true +x -x +x -x +x -x leds:true,false" {
		t.Errorf("repeat: %s", got)
	}
	three := usage(t, "pad.3", merc.Legend)
	e.Handle(three)
	e.Handle(up(three))
	sched.fire(t)
	e.Handle(three)
	e.Handle(up(three))
	if got := out.take(); got != "leds:true,true +y -y +y -y leds:true,false" {
		t.Errorf("toggle: %s", got)
	}
	if sched.live() != 0 {
		t.Error("toggle left a live timer")
	}
}

func TestChatLaunchAndMod(t *testing.T) {
	out, sched := &fakeOut{}, &fakeSched{}
	e := New(out, sched)
	e.Activate(mod(t, `
name: m
keys:
  pad.p: { chat: { open: t, text: "gg", send: enter } }
  hotkey.1: { launch: "notepad" }
  backlight.colour: { mod: next }
  media-select: ui
`), true)
	out.take()
	for _, name := range []string{"pad.p", "hotkey.1", "backlight.colour", "media-select"} {
		e.Handle(usage(t, name, merc.Legend))
		e.Handle(usage(t, name, merc.Legend))
		e.Handle(up(usage(t, name, merc.Legend)))
	}
	if got := out.take(); got != "+t -t type:gg +enter -enter launch:notepad mod:next ui" {
		t.Errorf("%s", got)
	}
}

func TestStandInTableAndEmulation(t *testing.T) {
	out, sched := &fakeOut{}, &fakeSched{}
	e := New(out, sched)
	e.Activate(mod(t, "name: s\ntable: standin\nkeys:\n  pad.w: up\n"), true)
	if got := out.take(); got != "table:2 leds:true,false" {
		t.Errorf("activate stand-in: %s", got)
	}
	for _, name := range []string{"pad.w", "pad.a", "num.7"} {
		e.Handle(usage(t, name, merc.StandIn))
		e.Handle(up(usage(t, name, merc.StandIn)))
	}
	e.SetLocks(true, false, false)
	e.Handle(usage(t, "num.7", merc.StandIn))
	e.Handle(up(usage(t, "num.7", merc.StandIn)))
	if got := out.take(); got != "+up -up +a -a +left-ctrl +f -f -left-ctrl +kp-7 -kp-7" {
		t.Errorf("%s", got)
	}
	e.Activate(nil, false)
	if got := out.take(); got != "table:1 leds:false,false" {
		t.Errorf("deactivate: %s", got)
	}
	e.DeviceTable(merc.StandIn, true)
	e.Handle(usage(t, "pad.w", merc.StandIn))
	e.Handle(up(usage(t, "pad.w", merc.StandIn)))
	if got := out.take(); got != "+w -w" {
		t.Errorf("stuck in table 2: %s", got)
	}
}

func TestFixedStandInDevice(t *testing.T) {
	out, sched := &fakeOut{}, &fakeSched{}
	e := New(out, sched)
	e.DeviceTable(merc.StandIn, false)
	e.Activate(mod(t, "name: desk\ntable: legend\nkeys:\n  hotkey.1: { toggle: x }\n"), false)
	if got := out.take(); got != "leds:true,false" {
		t.Errorf("activate: %s", got)
	}
	for _, name := range []string{"pad.w", "pad.-", "pad.t", "num.7"} {
		e.Handle(usage(t, name, merc.StandIn))
		e.Handle(up(usage(t, name, merc.StandIn)))
	}
	if got := out.take(); got != "+w -w +minus -minus +t -t +left-ctrl +f -f -left-ctrl" {
		t.Errorf("emulation on a fixed device: %s", got)
	}
	e.Activate(mod(t, "name: s\ntable: standin\n"), true)
	if got := out.take(); got != "" && got != "leds:true,false" {
		t.Errorf("no table request expected: %s", got)
	}
}

func TestTogglesAndLEDSources(t *testing.T) {
	out, sched := &fakeOut{}, &fakeSched{}
	e := New(out, sched)
	leds, _ := profile.ParseLEDs("flag:sharing", "flag:streaming")
	e.SetLEDDefaults(leds)
	e.Activate(mod(t, `
name: desk
keys:
  hotkey.1: { toggle: sharing }
  hotkey.3: { toggle: streaming, tap: f15 }
`), false)
	out.take()
	one, three := usage(t, "hotkey.1", merc.Legend), usage(t, "hotkey.3", merc.Legend)
	e.Handle(one)
	e.Handle(up(one))
	e.Handle(three)
	e.Handle(up(three))
	e.Handle(one)
	e.Handle(up(one))
	if got := out.take(); got != "leds:true,false +f13 -f13 leds:true,true +f15 -f15 leds:false,true +f13 -f13" {
		t.Errorf("%s", got)
	}
	if f := e.Flags(); f["sharing"] || !f["streaming"] {
		t.Errorf("flags %v", f)
	}
	e.Activate(mod(t, "name: game\ntriggers: [x]\nleds: { enabled: mod, action: on }\n"), true)
	if got := out.take(); got != "leds:true,true" {
		t.Errorf("mod leds: %s", got)
	}
	e.Activate(nil, false)
	if got := out.take(); got != "leds:false,true" {
		t.Errorf("back to defaults: %s", got)
	}
	e.SetFlag("streaming", false)
	if got := out.take(); got != "leds:false,false" {
		t.Errorf("set flag: %s", got)
	}
}

func TestEnabledFollowsActivation(t *testing.T) {
	out, sched := &fakeOut{}, &fakeSched{}
	e := New(out, sched)
	e.Activate(mod(t, "name: g\ntriggers: [game.exe]\n"), false)
	if got := out.take(); got != "leds:true,false" {
		t.Errorf("pinned: %s", got)
	}
	e.Activate(nil, false)
	if got := out.take(); got != "leds:false,false" {
		t.Errorf("bare: %s", got)
	}
}

func TestSelect(t *testing.T) {
	a := mod(t, "name: a\ntriggers: [Doom.exe]\n")
	b := mod(t, "name: b\ntriggers: [quake]\n")
	if Select([]*profile.Mod{a, b}, "DOOM.EXE") != a || Select([]*profile.Mod{a, b}, "quake.exe") != b || Select([]*profile.Mod{a, b}, "explorer.exe") != nil {
		t.Error("trigger matching")
	}
}

func TestRepeatFollowsBinding(t *testing.T) {
	out, sched := &fakeOut{}, &fakeSched{}
	e := New(out, sched)
	e.Activate(mod(t, `
name: r
keys:
  pad.1: f1
  pad.2: { chord: [left-ctrl, s] }
  pad.3: { macro: [ {tap: a} ], hold: 0 }
  pad.4: { layer: alt }
  hotkey.1: { toggle: sharing }
layers:
  alt:
    pad.1: f11
`), true)
	out.take()
	for _, c := range []struct{ key, want string }{
		{"pad.1", "+f1 +f1 -f1"},
		{"pad.2", "+left-ctrl +s +s -s -left-ctrl"},
		{"pad.5", "+5 +5 -5"},
		{"hotkey.1", "+f13 +f13 -f13"},
	} {
		ev := usage(t, c.key, merc.Legend)
		e.Handle(ev)
		e.Handle(ev)
		e.Handle(up(ev))
		if got := out.take(); got != c.want {
			t.Errorf("%s: %s, want %s", c.key, got, c.want)
		}
	}
	if !e.Flags()["sharing"] {
		t.Error("the repeat flipped the toggle back")
	}
	three := usage(t, "pad.3", merc.Legend)
	e.Handle(three)
	out.take()
	e.Handle(three)
	if got := out.take(); got != "" {
		t.Errorf("macro repeated: %s", got)
	}
	e.Handle(up(three))
	four, one := usage(t, "pad.4", merc.Legend), usage(t, "pad.1", merc.Legend)
	e.Handle(four)
	e.Handle(four)
	e.Handle(one)
	e.Handle(up(one))
	e.Handle(up(four))
	if got := out.take(); got != "+f11 -f11" {
		t.Errorf("layer under repeat: %s", got)
	}
}

func TestReleaseAll(t *testing.T) {
	out, sched := &fakeOut{}, &fakeSched{}
	e := New(out, sched)
	e.Activate(mod(t, `
name: r
keys:
  pad.1: f1
  pad.2: { chord: [left-ctrl, s] }
  pad.3: { macro: [ {tap: y}, {wait: 1s} ], mode: toggle, hold: 0 }
`), true)
	one, two, three := usage(t, "pad.1", merc.Legend), usage(t, "pad.2", merc.Legend), usage(t, "pad.3", merc.Legend)
	e.Handle(one)
	e.Handle(two)
	e.Handle(three)
	e.Handle(up(three))
	out.take()
	e.ReleaseAll()
	got := out.take()
	for _, want := range []string{"-f1", "-s -left-ctrl", "leds:true,false"} {
		if !strings.Contains(got, want) {
			t.Errorf("release-all output %q lacks %q", got, want)
		}
	}
	if sched.live() != 0 {
		t.Error("the toggle macro is still scheduled")
	}
	e.Handle(one)
	if got := out.take(); got != "+f1" {
		t.Errorf("press after release-all: %s", got)
	}
}

func TestMacroLoopFloor(t *testing.T) {
	out, sched := &fakeOut{}, &fakeSched{}
	e := New(out, sched)
	e.Activate(mod(t, `
name: m
keys:
  pad.1: { macro: [ {tap: x} ], mode: repeat, hold: 0 }
  pad.2: { macro: [ {tap: y}, {wait: 500ms} ], mode: toggle, hold: 0 }
  pad.3: { macro: [ {tap: z}, {wait: 10ms} ], mode: repeat, hold: 0 }
`), true)
	one, two, three := usage(t, "pad.1", merc.Legend), usage(t, "pad.2", merc.Legend), usage(t, "pad.3", merc.Legend)
	e.Handle(one)
	if d := sched.fire(t); d != minLoop {
		t.Errorf("no waits: next pass after %v, want %v", d, minLoop)
	}
	e.Handle(up(one))
	sched.fire(t)
	e.Handle(two)
	if d := sched.fire(t); d != 500*time.Millisecond {
		t.Errorf("wait step: %v", d)
	}
	if d := sched.fire(t); d != 0 {
		t.Errorf("500 ms of waits: next pass after %v, want 0", d)
	}
	e.Handle(up(two))
	e.Handle(two)
	e.Handle(up(two))
	e.Handle(three)
	sched.fire(t)
	if d := sched.fire(t); d != minLoop-10*time.Millisecond {
		t.Errorf("10 ms of waits: next pass after %v, want %v", d, minLoop-10*time.Millisecond)
	}
	e.Handle(up(three))
}

func TestRecordingBlinksAction(t *testing.T) {
	out, sched := &fakeOut{}, &fakeSched{}
	e := New(out, sched)
	e.SetLEDDefaults(profile.LEDs{Action: profile.LEDSource{Kind: profile.LEDOn}})
	if got := out.take(); got != "leds:false,true" {
		t.Fatalf("action on: %s", got)
	}
	e.SetRecording(true)
	if got := out.take(); got != "" {
		t.Errorf("start: %s", got)
	}
	if d := sched.fire(t); d != blinkPeriod {
		t.Errorf("period %v", d)
	}
	sched.fire(t)
	if got := out.take(); got != "leds:false,false leds:false,true" {
		t.Errorf("blinking: %s", got)
	}
	e.SetRecording(false)
	if sched.live() != 0 {
		t.Error("blink timer left running")
	}
	e.SetRecording(false)
	if got := out.take(); got != "" {
		t.Errorf("after stop: %s", got)
	}
}

func TestRestoreIsScrollLockWithNumLockOff(t *testing.T) {
	out, sched := &fakeOut{}, &fakeSched{}
	e := New(out, sched)
	key := usage(t, "scroll-lock", merc.StandIn)
	e.Activate(mod(t, "name: s\ntable: standin\nkeys:\n  restore: f1\n  scroll-lock: f2\n"), true)
	out.take()
	e.SetLocks(false, false, false)
	e.Handle(key)
	e.Handle(up(key))
	e.SetLocks(true, false, false)
	e.Handle(key)
	e.Handle(up(key))
	if got := out.take(); got != "+f1 -f1 +f2 -f2" {
		t.Errorf("restore and scroll-lock bound: %s", got)
	}
	e.Activate(mod(t, "name: s\ntable: standin\nkeys:\n  restore: f1\n"), true)
	out.take()
	e.Handle(key)
	e.Handle(up(key))
	if got := out.take(); got != "+scroll-lock -scroll-lock" {
		t.Errorf("only restore bound, Num Lock on: %s", got)
	}
	e.Activate(mod(t, "name: s\ntable: standin\n"), true)
	out.take()
	e.SetLocks(false, false, false)
	e.Handle(key)
	e.Handle(up(key))
	if got := out.take(); got != "+left-win +down -down -left-win" {
		t.Errorf("unbound, Num Lock off: %s", got)
	}
}

func TestDeclare(t *testing.T) {
	e := New(&fakeOut{}, &fakeSched{})
	e.Declare("Sharing", "streaming", "")
	if f := e.Flags(); len(f) != 2 || f["sharing"] || f["streaming"] {
		t.Errorf("declared: %v", f)
	}
	e.SetFlag("sharing", true)
	e.Declare("sharing")
	if !e.Flags()["sharing"] {
		t.Error("declaring again reset the flag")
	}
}
