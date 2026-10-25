//go:build windows

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Kazzer/zboard-merc/internal/browser"
	"github.com/Kazzer/zboard-merc/internal/engine"
	"github.com/Kazzer/zboard-merc/internal/hidrd"
	"github.com/Kazzer/zboard-merc/internal/keys"
	"github.com/Kazzer/zboard-merc/internal/merc"
	platwin "github.com/Kazzer/zboard-merc/internal/platform/windows"
	"github.com/Kazzer/zboard-merc/internal/profile"
)

type output struct {
	feature  *platwin.Device
	features chan []byte
	drained  chan struct{}
	sel      *selector
	uiLink   func() string
	debug    bool
}

func newOutput(feature *platwin.Device) *output {
	o := &output{feature: feature, features: make(chan []byte, 32), drained: make(chan struct{}), debug: debugOn()}
	go func() {
		defer close(o.drained)
		for r := range o.features {
			if r == nil {
				return
			}
			o.featureWrite(r)
		}
	}()
	return o
}

func (o *output) close() {
	o.features <- nil
	<-o.drained
	if o.feature != nil {
		o.feature.Close()
	}
}

func (o *output) Key(k keys.Key, down bool) {
	if o.debug {
		slog.Debug("out", "name", k.Name, "down", down)
	}
	if err := platwin.SendKey(k, down); err != nil {
		slog.Warn("key", "error", err)
	}
}

func (o *output) Type(text string) {
	if err := platwin.TypeText(text); err != nil {
		slog.Warn("chat", "error", err)
	}
}

func (o *output) Launch(target string) {
	go func() {
		if err := platwin.Launch(target); err != nil {
			slog.Warn("launch", "error", err)
		}
	}()
}

func (o *output) featureWrite(report []byte) {
	if debugOn() {
		slog.Debug("feature report", "bytes", fmt.Sprintf("% X", report))
	}
	if o.feature == nil || o.feature.SetFeature(report) != nil {
		if o.feature != nil {
			o.feature.Close()
			o.feature = nil
		}
		dev, err := vendorCollection()
		if err != nil {
			slog.Warn("feature report", "error", err)
			return
		}
		o.feature = dev
		if err := dev.SetFeature(report); err != nil {
			slog.Warn("feature report", "error", err)
		}
	}
}

func (o *output) enqueue(report []byte) {
	select {
	case o.features <- report:
	default:
		slog.Warn("feature report dropped", "bytes", fmt.Sprintf("% X", report))
	}
}

func (o *output) LEDs(enabled, action bool) { o.enqueue(merc.LEDReport(enabled, action)) }
func (o *output) Table(t merc.Table)        { o.enqueue(merc.TableReport(t)) }
func (o *output) ModRequest(which string)   { go o.sel.request(which) }

func (o *output) OpenUI() {
	link := o.uiLink()
	if link == "" {
		slog.Warn("ui", "error", "the control API is off, so there is no UI")
		return
	}
	if err := browser.Open(link); err != nil {
		slog.Warn("ui", "error", err)
	}
}

func debugOn() bool { return slog.Default().Enabled(context.Background(), slog.LevelDebug) }

func vendorCollection() (*platwin.Device, error) {
	devs, err := platwin.Enumerate(merc.VendorID, merc.ProductID)
	if err != nil {
		return nil, err
	}
	var found *platwin.Device
	for _, d := range devs {
		if found == nil && d.Caps.FeatureLen == 3 && d.Caps.UsagePage == 0xFF00 {
			found = d
			continue
		}
		d.Close()
	}
	if found == nil {
		return nil, fmt.Errorf("no Merc Stealth vendor collection found")
	}
	return found, nil
}

func consumerCollection() (*platwin.Device, error) {
	devs, err := platwin.Enumerate(merc.VendorID, merc.ProductID)
	if err != nil {
		return nil, err
	}
	var found *platwin.Device
	for _, d := range devs {
		if found == nil && d.Caps.UsagePage == 0x0C && d.Caps.Usage == 0x01 && !d.ReadOnly && d.Caps.InputLen > 0 {
			found = d
			continue
		}
		d.Close()
	}
	if found == nil {
		return nil, fmt.Errorf("no Merc Stealth consumer collection found")
	}
	return found, nil
}

type rawKey struct {
	usage hidrd.Usage
	down  bool
}

func run(cfg Config, mods []*profile.Mod, stop <-chan struct{}, ready func(*session)) error {
	feature, err := vendorCollection()
	if err != nil {
		return err
	}
	out := newOutput(feature)
	eng := engine.New(out, scheduler{})
	sel := &selector{eng: eng, mods: mods}
	out.sel = sel
	if cfg.Default != "" {
		if sel.def = sel.byName(cfg.Default); sel.def == nil {
			return fmt.Errorf("default Mod %q is not loaded", cfg.Default)
		}
	}
	out.LEDs(false, false)
	eng.SetLEDDefaults(ledDefaults(cfg))
	declareFlags(eng, cfg, mods)
	if cfg.pin != "" {
		sel.pinned = sel.byName(cfg.pin)
	}
	rec := newRecorder(eng)
	ctl, reload, err := startControl(cfg, sel, rec)
	if err != nil {
		return err
	}
	var attached atomic.Bool
	attached.Store(true)
	lockLEDs := platwin.OpenLockLEDs()
	s := &session{eng: eng, sel: sel, ctl: ctl, rec: rec, reload: reload, attached: attached.Load, driver: lockLEDs.Connected}
	if ctl != nil {
		ctl.s = s
	}
	out.uiLink = s.uiLink
	if ready != nil {
		ready(s)
	}
	sel.foreground(platwin.ForegroundProcess())

	type queued struct {
		ev        platwin.KeyEvent
		raw       *rawKey
		at        time.Time
		synthetic bool
		reset     bool
	}
	events := make(chan queued, 1024)
	var dropped atomic.Int64
	filter := func(ev platwin.KeyEvent) bool {
		if _, ok := keys.ByScan(uint16(ev.Scan), ev.Extended, uint8(ev.VK)); !ok {
			return false
		}
		select {
		case events <- queued{ev: ev, at: time.Now()}:
		default:
			dropped.Add(1)
		}
		return true
	}
	hook, err := platwin.StartFilterHook(filter)
	if err != nil {
		out.close()
		return fmt.Errorf("keyboard hook: %w", err)
	}
	var hookMu sync.Mutex
	stopping := false
	locks := hook.Locks()
	eng.SetLocks(locks.Num, locks.Caps, locks.Scroll)
	slog.Info("hook installed", "num", locks.Num, "caps", locks.Caps, "scroll", locks.Scroll)
	lockLEDs.Set(locks)
	hook.SetLockListener(lockLEDs.Set)

	done := make(chan struct{})

	driverBack := map[hidrd.Usage]hidrd.Usage{}
	for from, to := range merc.WindowsDriverMap {
		driverBack[to] = from
	}
	numLockKey, _ := keys.ByName("num-lock")
	var rawConsumer atomic.Bool
	delay, interval := platwin.Typematic()
	var eventsMu sync.Mutex
	eventsClosed := false
	send := func(q queued) {
		eventsMu.Lock()
		defer eventsMu.Unlock()
		if !eventsClosed {
			select {
			case events <- q:
			default:
			}
		}
	}
	closeEvents := func() {
		eventsMu.Lock()
		eventsClosed = true
		close(events)
		eventsMu.Unlock()
	}
	engineDone := make(chan struct{})
	go func() {
		defer close(engineDone)
		debug := debugOn()
		repeating := map[hidrd.Usage]*time.Timer{}
		native := map[hidrd.Usage]bool{}
		stopRepeat := func(u hidrd.Usage) {
			if t := repeating[u]; t != nil {
				t.Stop()
				delete(repeating, u)
			}
		}
		stopRepeats := func() {
			for u := range repeating {
				stopRepeat(u)
			}
		}
		defer stopRepeats()
		for q := range events {
			if q.reset {
				stopRepeats()
				continue
			}
			if q.raw != nil {
				usage := q.raw.usage
				if _, standIn := merc.ByStandIn(usage); !standIn {
					continue
				}
				if q.synthetic {
					if repeating[usage] == nil {
						continue
					}
					s.handle(engine.Event{Usage: usage, Down: true, Time: q.at})
					repeating[usage] = time.AfterFunc(interval, func() { send(queued{raw: q.raw, at: time.Now(), synthetic: true}) })
					continue
				}
				if debug {
					k, _ := keys.ByUsage(usage)
					slog.Debug("key", "name", k.Name, "from", "consumer collection", "up", !q.raw.down)
				}
				s.handle(engine.Event{Usage: usage, Down: q.raw.down, Time: q.at})
				stopRepeat(usage)
				if q.raw.down {
					raw := q.raw
					repeating[usage] = time.AfterFunc(delay+interval, func() { send(queued{raw: raw, at: time.Now(), synthetic: true}) })
				}
				continue
			}
			ev := q.ev
			k, _ := keys.ByScan(uint16(ev.Scan), ev.Extended, uint8(ev.VK))
			usage := k.Usage
			if original, ok := driverBack[usage]; ok {
				usage = original
			}
			if q.synthetic {
				if repeating[usage] == nil {
					continue
				}
				s.handle(engine.Event{Usage: usage, Down: true, Time: q.at})
				repeating[usage] = time.AfterFunc(interval, func() { send(queued{ev: ev, at: time.Now(), synthetic: true}) })
				continue
			}
			eng.SetLocks(ev.Locks.Num, ev.Locks.Caps, ev.Locks.Scroll)
			if _, standIn := merc.ByStandIn(usage); standIn && usage.Page() == 0x0C && rawConsumer.Load() {
				continue
			}
			if debug {
				slog.Debug("key", "name", k.Name, "scan", fmt.Sprintf("%#x", ev.Scan), "ext", ev.Extended, "vk", fmt.Sprintf("%#x", ev.VK),
					"up", ev.Up, "repeat", ev.Repeat, "injected", ev.Injected, "num", ev.Locks.Num, "caps", ev.Locks.Caps, "scroll", ev.Locks.Scroll)
			}
			if !ev.Up && !ev.Repeat && ev.KeypadNavigation() && eng.Table() == merc.Legend {
				platwin.SendKey(numLockKey, true)
				platwin.SendKey(numLockKey, false)
				ev.Locks.Num = true
				eng.SetLocks(true, ev.Locks.Caps, ev.Locks.Scroll)
				slog.Info("num lock: Windows had it off while the keyboard is in keypad mode; turned it on")
			}
			s.handle(engine.Event{Usage: usage, Down: !ev.Up, Time: q.at})
			if _, standIn := merc.ByStandIn(usage); standIn && usage.Page() == 0x0C {
				stopRepeat(usage)
				if ev.Repeat {
					native[usage] = true
				} else if !ev.Up && !native[usage] {
					rep := ev
					rep.Repeat = true
					repeating[usage] = time.AfterFunc(delay+interval, func() { send(queued{ev: rep, at: time.Now(), synthetic: true}) })
				}
			}
			if n := dropped.Swap(0); n > 0 {
				slog.Warn("key events dropped: the engine fell behind", "count", n)
			}
		}
	}()

	var secure atomic.Bool
	go func() {
		for {
			d, err := consumerCollection()
			if err != nil {
				select {
				case <-done:
					return
				case <-time.After(2 * time.Second):
				}
				continue
			}
			rawConsumer.Store(true)
			slog.Info("consumer collection: reading the pad's consumer keys directly")
			closer := make(chan struct{})
			go func() {
				select {
				case <-done:
				case <-closer:
				}
				d.Close()
			}()
			held := map[hidrd.Usage]bool{}
			for {
				rep, err := d.Read()
				if err != nil {
					break
				}
				now := map[hidrd.Usage]bool{}
				for _, u := range d.Usages(rep) {
					if u.Page == 0x0C {
						now[hidrd.Usage(uint32(u.Page)<<16|uint32(u.Usage))] = true
					}
				}
				for u := range held {
					if !now[u] {
						delete(held, u)
						send(queued{raw: &rawKey{u, false}, at: time.Now()})
					}
				}
				for u := range now {
					if !held[u] {
						held[u] = true
						send(queued{raw: &rawKey{u, true}, at: time.Now()})
					}
				}
			}
			rawConsumer.Store(false)
			for u := range held {
				send(queued{raw: &rawKey{u, false}, at: time.Now()})
			}
			close(closer)
			select {
			case <-done:
				return
			case <-time.After(2 * time.Second):
			}
		}
	}()

	const probeIdle = 30 * time.Second
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		var lastProbe time.Time
		lastReal := time.Now()
		for {
			select {
			case <-done:
				return
			case <-t.C:
			}
			idle := platwin.IdleFor()
			if lastProbe.IsZero() || idle+time.Second < time.Since(lastProbe) {
				lastReal = time.Now().Add(-idle)
			}
			if secure.Load() || time.Since(lastReal) > probeIdle {
				continue
			}
			lastProbe = time.Now()
			hookMu.Lock()
			if !stopping && !hook.Probe(500*time.Millisecond) {
				slog.Warn("keyboard hook lost; reinstalling")
				hook.Stop()
				if nh, err := platwin.StartFilterHook(filter); err != nil {
					slog.Error("keyboard hook", "error", err)
				} else {
					hook = nh
					nh.SetLockListener(lockLEDs.Set)
					lockLEDs.Set(nh.Locks())
					send(queued{reset: true})
				}
			}
			hookMu.Unlock()
		}
	}()
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
			}
			devs, err := platwin.Enumerate(merc.VendorID, merc.ProductID)
			for _, d := range devs {
				d.Close()
			}
			if was := attached.Swap(err == nil && len(devs) > 0); was != attached.Load() {
				slog.Info("keyboard", "attached", attached.Load())
			}
			if attached.Load() && !secure.Load() {
				out.Table(eng.Table())
				out.LEDs(eng.LEDState())
			}
		}
	}()
	go func() {
		t := time.NewTicker(300 * time.Millisecond)
		defer t.Stop()
		last := ""
		for {
			select {
			case <-done:
				return
			case <-t.C:
			}
			if s := platwin.SecureDesktop(); s != secure.Load() {
				secure.Store(s)
				if s {
					out.Table(merc.Legend)
					lockLEDs.Suspend()
					slog.Info("secure desktop: keyboard in its power-on table meanwhile")
				} else {
					if m := eng.Active(); m != nil && m.Table == merc.StandIn {
						out.Table(merc.StandIn)
					}
					hookMu.Lock()
					if !stopping {
						hook.Reseed()
					}
					hookMu.Unlock()
					lockLEDs.Resume()
					slog.Info("desktop back: table restored")
				}
			}
			if secure.Load() {
				continue
			}
			if p := platwin.ForegroundProcess(); p != last {
				last = p
				sel.foreground(p)
			}
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	select {
	case sg := <-sig:
		slog.Info("stopping", "signal", sg)
	case <-stop:
		slog.Info("stopping", "reason", "quit")
	}
	close(done)
	hookMu.Lock()
	stopping = true
	hook.Stop()
	hookMu.Unlock()
	lockLEDs.Close()
	closeEvents()
	<-engineDone
	eng.ReleaseAll()
	eng.Activate(nil, false)
	out.LEDs(false, false)
	out.close()
	return nil
}
