//go:build linux

package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Kazzer/zboard-merc/internal/browser"
	"github.com/Kazzer/zboard-merc/internal/engine"
	"github.com/Kazzer/zboard-merc/internal/foreground"
	"github.com/Kazzer/zboard-merc/internal/keys"
	"github.com/Kazzer/zboard-merc/internal/merc"
	platlinux "github.com/Kazzer/zboard-merc/internal/platform/linux"
	"github.com/Kazzer/zboard-merc/internal/profile"
	"github.com/Kazzer/zboard-merc/internal/typing"
)

type device struct {
	mu  sync.Mutex
	kbd *platlinux.Keyboard
}

func (d *device) feature(report []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.kbd == nil {
		return errors.New("keyboard not attached")
	}
	return d.kbd.Feature(report)
}

func (d *device) setLocks(num, caps, scroll bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.kbd != nil {
		d.kbd.SetLocks(num, caps, scroll)
	}
}

type output struct {
	virt      *platlinux.Virtual
	dev       *device
	hasTables bool
	sel       *selector
	uiLink    func() string
}

func (o *output) Key(k keys.Key, down bool) {
	code, err := k.LinuxCode()
	if err != nil {
		slog.Warn("key", "error", err)
		return
	}
	if err := o.virt.Key(code, down); err != nil {
		slog.Warn("uinput", "error", err)
	}
}

func (o *output) Type(text string) {
	strokes, err := typing.Strokes(text)
	if err != nil {
		slog.Warn("chat", "error", err)
	}
	shift, _ := keys.ByName("left-shift")
	for _, s := range strokes {
		if s.Shift {
			o.Key(shift, true)
		}
		o.Key(s.Key, true)
		o.Key(s.Key, false)
		if s.Shift {
			o.Key(shift, false)
		}
	}
}

func (o *output) Launch(target string) {
	cmd := exec.Command("xdg-open", target)
	if !strings.Contains(target, "://") {
		if info, err := os.Stat(target); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			cmd = exec.Command(target)
		} else if _, err := exec.LookPath(target); err == nil {
			cmd = exec.Command(target)
		}
	}
	if err := cmd.Start(); err != nil {
		slog.Warn("launch", "target", target, "error", err)
		return
	}
	go cmd.Wait()
}

func (o *output) LEDs(enabled, action bool) {
	if err := o.dev.feature(merc.LEDReport(enabled, action)); err != nil {
		slog.Warn("leds", "error", err)
	}
}

func (o *output) Table(t merc.Table) {
	if !o.hasTables {
		return
	}
	if err := o.dev.feature(merc.TableReport(t)); err != nil {
		slog.Warn("table", "error", err)
	}
}

func (o *output) ModRequest(which string) { go o.sel.request(which) }

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

func openKeyboard(pid uint16) (*platlinux.Keyboard, uint16, error) {
	var last error
	for _, candidate := range []uint16{pid, merc.ProductID, 0x0210} {
		if candidate == 0 {
			continue
		}
		kbd, err := platlinux.OpenKeyboard(merc.VendorID, candidate)
		if err == nil {
			return kbd, candidate, nil
		}
		last = err
	}
	return nil, 0, last
}

func run(cfg Config, mods []*profile.Mod, stop <-chan struct{}, ready func(*session)) error {
	pid := uint16(0)
	if cfg.Device != "" {
		v, err := strconv.ParseUint(cfg.Device, 16, 16)
		if err != nil {
			return fmt.Errorf("device %q: %w", cfg.Device, err)
		}
		pid = uint16(v)
	}
	kbd, pid, err := openKeyboard(pid)
	if err != nil {
		return fmt.Errorf("keyboard: %w", err)
	}
	dev := &device{kbd: kbd}
	defer func() {
		dev.mu.Lock()
		if dev.kbd != nil {
			dev.kbd.Close()
		}
		dev.mu.Unlock()
	}()
	for _, d := range kbd.HID {
		slog.Info("interface", "number", d.Interface, "leds", d.HasLEDs, "hidraw", d.Hidraw, "events", d.Events)
	}
	slog.Info("keyboard", "pid", fmt.Sprintf("%04x", pid), "feature node", kbd.FeatureNode())

	var codes []uint16
	for _, k := range keys.All {
		if c, err := k.LinuxCode(); err == nil {
			codes = append(codes, c)
		}
	}
	virt, err := platlinux.CreateVirtual(platlinux.VirtualName, 0x5A42, 0x0001, codes)
	if err != nil {
		return fmt.Errorf("uinput: %w", err)
	}
	defer virt.Close()

	out := &output{virt: virt, dev: dev, hasTables: pid == merc.ProductID}
	eng := engine.New(out, scheduler{})
	if !out.hasTables {
		eng.DeviceTable(merc.StandIn, false)
	}
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
	s := &session{eng: eng, sel: sel, ctl: ctl, rec: rec, reload: reload, attached: func() bool {
		dev.mu.Lock()
		defer dev.mu.Unlock()
		return dev.kbd != nil
	}}
	if ctl != nil {
		ctl.s = s
	}
	out.uiLink = s.uiLink
	if ready != nil {
		ready(s)
	}
	sel.foreground("")

	var locks atomic.Uint32
	applyLocks := func() {
		v := locks.Load()
		dev.setLocks(v&1 != 0, v&2 != 0, v&4 != 0)
		eng.SetLocks(v&1 != 0, v&2 != 0, v&4 != 0)
	}
	if num, caps, scroll := kbd.Locks(); num || caps || scroll {
		var v uint32
		for i, on := range []bool{num, caps, scroll} {
			if on {
				v |= 1 << i
			}
		}
		locks.Store(v)
		slog.Info("locks at start", "num", num, "caps", caps, "scroll", scroll)
	}
	applyLocks()
	go virt.ReadLEDs(func(led uint16, on bool) {
		if led < 3 {
			for {
				old := locks.Load()
				next := old &^ (1 << led)
				if on {
					next |= 1 << led
				}
				if locks.CompareAndSwap(old, next) {
					break
				}
			}
			applyLocks()
		}
	})
	done := make(chan struct{})
	if err := foreground.Watch(done, sel.foreground); err != nil {
		slog.Warn("foreground tracking off", "reason", err)
	}
	go func() {
		for {
			dev.mu.Lock()
			k := dev.kbd
			dev.mu.Unlock()
			if k != nil {
				k.Read(s.handle)
				slog.Warn("keyboard gone; waiting for it")
				dev.mu.Lock()
				k.Close()
				dev.kbd = nil
				dev.mu.Unlock()
			}
			select {
			case <-done:
				return
			case <-time.After(2 * time.Second):
			}
			k, _, err := openKeyboard(pid)
			if err != nil {
				continue
			}
			dev.mu.Lock()
			dev.kbd = k
			dev.mu.Unlock()
			slog.Info("keyboard back")
			eng.DeviceTable(merc.Legend, true)
			if !out.hasTables {
				eng.DeviceTable(merc.StandIn, false)
			}
			applyLocks()
			cur := eng.Active()
			eng.Activate(nil, false)
			eng.Activate(cur, cur != nil && len(cur.Triggers) > 0)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sg := <-sig:
		slog.Info("stopping", "signal", sg)
	case <-stop:
		slog.Info("stopping", "reason", "quit")
	}
	close(done)
	eng.ReleaseAll()
	eng.Activate(nil, false)
	out.LEDs(false, false)
	return nil
}
