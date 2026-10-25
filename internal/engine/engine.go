package engine

import (
	"strings"
	"sync"
	"time"

	"github.com/Kazzer/zboard-merc/internal/hidrd"
	"github.com/Kazzer/zboard-merc/internal/keys"
	"github.com/Kazzer/zboard-merc/internal/merc"
	"github.com/Kazzer/zboard-merc/internal/profile"
)

type Event struct {
	Usage hidrd.Usage
	Down  bool
	Time  time.Time
}

type Output interface {
	Key(k keys.Key, down bool)
	Type(text string)
	Launch(target string)
	LEDs(enabled, action bool)
	Table(t merc.Table)
	ModRequest(which string)
	OpenUI()
}

type Scheduler interface {
	After(d time.Duration, fn func()) (cancel func())
}

type held struct {
	binding *profile.Binding
	key     keys.Key
	chord   []keys.Key
	layer   string
	macro   *macroRun
}

type macroRun struct {
	cancel  func()
	held    bool
	stopped bool
}

type Engine struct {
	mu          sync.Mutex
	out         Output
	sched       Scheduler
	mod         *profile.Mod
	byTrigger   bool
	deviceTable merc.Table
	switchable  bool
	numLock     bool
	capsLock    bool
	scrollLock  bool
	heldKeys    map[hidrd.Usage]*held
	heldLayers  []string
	macros      map[hidrd.Usage]*macroRun
	defBase     profile.Layer
	defNumLock  profile.Layer
	ledDefaults profile.LEDs
	flags       map[string]bool
	ledEnabled  bool
	ledAction   bool
	recording   bool
	blinkOn     bool
	blinkCancel func()
}

const blinkPeriod = 300 * time.Millisecond

func New(out Output, sched Scheduler) *Engine {
	e := &Engine{out: out, sched: sched, deviceTable: merc.Legend, switchable: true,
		heldKeys: map[hidrd.Usage]*held{}, macros: map[hidrd.Usage]*macroRun{}, flags: map[string]bool{}}
	e.defBase, e.defNumLock = profile.StandInDefault()
	return e
}

func (e *Engine) Table() merc.Table {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.deviceTable
}

func (e *Engine) DeviceTable(t merc.Table, switchable bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.deviceTable, e.switchable = t, switchable
}

func (e *Engine) SetLEDDefaults(l profile.LEDs) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ledDefaults = l
	e.updateLEDs()
}

func (e *Engine) SetFlag(name string, on bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.flags[strings.ToLower(name)] = on
	e.updateLEDs()
}

func (e *Engine) Declare(names ...string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, n := range names {
		n = strings.ToLower(n)
		if _, known := e.flags[n]; n != "" && !known {
			e.flags[n] = false
		}
	}
}

func (e *Engine) Flags() map[string]bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make(map[string]bool, len(e.flags))
	for k, v := range e.flags {
		out[k] = v
	}
	return out
}

func (e *Engine) SetLocks(num, caps, scroll bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.numLock, e.capsLock, e.scrollLock = num, caps, scroll
}

func (e *Engine) Activate(mod *profile.Mod, byTrigger bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.mod, e.byTrigger = mod, byTrigger
	want := merc.Legend
	if mod != nil {
		want = mod.Table
	}
	if e.switchable && want != e.deviceTable {
		e.deviceTable = want
		e.out.Table(want)
	}
	e.updateLEDs()
}

func (e *Engine) LEDState() (enabled, action bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.ledEnabled, e.ledAction
}

func (e *Engine) Active() *profile.Mod {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.mod
}

func (e *Engine) SetRecording(on bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if on == e.recording {
		return
	}
	e.recording = on
	if on {
		e.blinkOn = false
		e.blink()
		return
	}
	if e.blinkCancel != nil {
		e.blinkCancel()
		e.blinkCancel = nil
	}
	e.updateLEDs()
}

func (e *Engine) blink() {
	e.blinkOn = !e.blinkOn
	e.updateLEDs()
	e.blinkCancel = e.sched.After(blinkPeriod, func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		if e.recording {
			e.blink()
		}
	})
}

func (e *Engine) updateLEDs() {
	modEnabled := e.mod != nil
	macroRunning := false
	for _, m := range e.macros {
		if !m.stopped {
			macroRunning = true
		}
	}
	sources := e.ledDefaults
	if e.mod != nil && e.mod.LEDs != nil {
		sources = *e.mod.LEDs
	}
	enabled := e.ledValue(sources.Enabled, modEnabled)
	action := e.ledValue(sources.Action, macroRunning)
	if e.recording {
		action = e.blinkOn
	}
	if enabled != e.ledEnabled || action != e.ledAction {
		e.ledEnabled, e.ledAction = enabled, action
		e.out.LEDs(enabled, action)
	}
}

func (e *Engine) ledValue(s profile.LEDSource, modMeaning bool) bool {
	switch s.Kind {
	case profile.LEDOn:
		return true
	case profile.LEDOff:
		return false
	case profile.LEDFlag:
		return e.flags[s.Flag]
	}
	return modMeaning
}

func (e *Engine) lookup(u hidrd.Usage) *profile.Binding {
	usages := []hidrd.Usage{u}
	if off, ok := merc.NumLockOff(u); ok && !e.numLock {
		usages = []hidrd.Usage{off, u}
	}
	find := func(l profile.Layer) *profile.Binding {
		for _, c := range usages {
			if b, ok := l[c]; ok {
				return &b
			}
		}
		return nil
	}
	if e.mod != nil {
		for i := len(e.heldLayers) - 1; i >= 0; i-- {
			if b := find(e.mod.Layers[e.heldLayers[i]]); b != nil {
				return b
			}
		}
		for name, on := range map[string]bool{"num-lock": e.numLock, "caps-lock": e.capsLock, "scroll-lock": e.scrollLock} {
			if on {
				if b := find(e.mod.Layers[name]); b != nil {
					return b
				}
			}
		}
		if b := find(e.mod.Base); b != nil {
			return b
		}
	}
	if e.deviceTable == merc.StandIn {
		if e.numLock {
			if b := find(e.defNumLock); b != nil {
				return b
			}
		}
		if b := find(e.defBase); b != nil {
			return b
		}
	}
	return nil
}

func (e *Engine) Handle(ev Event) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !ev.Down {
		e.release(ev.Usage)
		return
	}
	if h, repeat := e.heldKeys[ev.Usage]; repeat {
		e.repeat(h)
		return
	}
	h := &held{binding: e.lookup(ev.Usage)}
	e.heldKeys[ev.Usage] = h
	b := h.binding
	if b != nil && b.Toggle != "" {
		e.flags[b.Toggle] = !e.flags[b.Toggle]
		e.updateLEDs()
		if b.Remap == nil && b.Chord == nil && b.Macro == nil && b.Chat == nil && b.Launch == "" && b.Layer == "" && b.Mod == "" && !b.Disable {
			b = nil
		}
	}
	switch {
	case b == nil:
		if k, ok := keys.ByUsage(ev.Usage); ok {
			h.key = k
			e.out.Key(k, true)
		}
	case b.Disable:
	case b.Remap != nil:
		h.key = *b.Remap
		e.out.Key(h.key, true)
	case len(b.Chord) > 0:
		h.chord = b.Chord
		for _, k := range b.Chord {
			e.out.Key(k, true)
		}
	case b.Macro != nil:
		e.startMacro(ev.Usage, h, b.Macro)
	case b.Chat != nil:
		e.out.Key(b.Chat.Open, true)
		e.out.Key(b.Chat.Open, false)
		e.out.Type(b.Chat.Text)
		e.out.Key(b.Chat.Send, true)
		e.out.Key(b.Chat.Send, false)
	case b.Launch != "":
		e.out.Launch(b.Launch)
	case b.Layer != "":
		h.layer = b.Layer
		e.heldLayers = append(e.heldLayers, b.Layer)
	case b.Mod != "":
		e.out.ModRequest(b.Mod)
	case b.UI:
		e.out.OpenUI()
	}
}

func (e *Engine) repeat(h *held) {
	switch {
	case h.key.Name != "":
		e.out.Key(h.key, true)
	case len(h.chord) > 0:
		e.out.Key(h.chord[len(h.chord)-1], true)
	}
}

func (e *Engine) ReleaseAll() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for u := range e.heldKeys {
		e.release(u)
	}
	for u := range e.macros {
		e.stopMacro(u)
	}
}

func (e *Engine) release(u hidrd.Usage) {
	h, ok := e.heldKeys[u]
	delete(e.heldKeys, u)
	if !ok {
		if k, found := keys.ByUsage(u); found {
			e.out.Key(k, false)
		}
		return
	}
	switch {
	case h.chord != nil:
		for i := len(h.chord) - 1; i >= 0; i-- {
			e.out.Key(h.chord[i], false)
		}
	case h.layer != "":
		for i := len(e.heldLayers) - 1; i >= 0; i-- {
			if e.heldLayers[i] == h.layer {
				e.heldLayers = append(e.heldLayers[:i], e.heldLayers[i+1:]...)
				break
			}
		}
	case h.macro != nil:
		h.macro.held = false
	case h.key.Name != "":
		e.out.Key(h.key, false)
	}
}

func (e *Engine) startMacro(u hidrd.Usage, h *held, m *profile.Macro) {
	if running, ok := e.macros[u]; ok && !running.stopped {
		if m.Mode == profile.Toggle {
			e.stopMacro(u)
			return
		}
		return
	}
	run := &macroRun{held: true}
	h.macro = run
	e.macros[u] = run
	e.updateLEDs()
	e.step(u, run, m, 0)
}

func (e *Engine) step(u hidrd.Usage, run *macroRun, m *profile.Macro, i int) {
	for ; i < len(m.Steps); i++ {
		if run.stopped {
			return
		}
		s := m.Steps[i]
		if s.Wait > 0 {
			next := i + 1
			run.cancel = e.sched.After(s.Wait, func() {
				e.mu.Lock()
				defer e.mu.Unlock()
				e.step(u, run, m, next)
			})
			return
		}
		e.out.Key(s.Key, s.Press)
	}
	again := m.Mode == profile.Repeat && run.held || m.Mode == profile.Toggle
	if again {
		run.cancel = e.sched.After(loopGap(m), func() {
			e.mu.Lock()
			defer e.mu.Unlock()
			if run.stopped {
				return
			}
			if m.Mode == profile.Repeat && !run.held {
				e.stopMacro(u)
				return
			}
			e.step(u, run, m, 0)
		})
		return
	}
	e.stopMacro(u)
}

const minLoop = 30 * time.Millisecond

func loopGap(m *profile.Macro) time.Duration {
	var waits time.Duration
	for _, s := range m.Steps {
		waits += s.Wait
	}
	if waits >= minLoop {
		return 0
	}
	return minLoop - waits
}

func (e *Engine) stopMacro(u hidrd.Usage) {
	run, ok := e.macros[u]
	if !ok {
		return
	}
	run.stopped = true
	if run.cancel != nil {
		run.cancel()
	}
	delete(e.macros, u)
	e.updateLEDs()
}

func Select(mods []*profile.Mod, process string) *profile.Mod {
	p := strings.ToLower(process)
	bare := strings.TrimSuffix(p, ".exe")
	for _, m := range mods {
		for _, t := range m.Triggers {
			if t == p || t == bare || strings.TrimSuffix(t, ".exe") == bare {
				return m
			}
		}
	}
	return nil
}
