//go:build linux

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/Kazzer/zboard-merc/internal/hidrd"
	"github.com/Kazzer/zboard-merc/internal/keys"
	"github.com/Kazzer/zboard-merc/internal/merc"
	platlinux "github.com/Kazzer/zboard-merc/internal/platform/linux"
)

type linuxCollection struct {
	hid  platlinux.HIDDevice
	f    *os.File
	desc *hidrd.Descriptor
	raw  []byte
	info collectionInfo
	usb  string
}

func enumerate(pid uint16) ([]collection, error) {
	pids := []uint16{pid}
	if pid == 0 {
		pids = pids[:0]
		for p := range models {
			pids = append(pids, p)
		}
		sort.Slice(pids, func(i, j int) bool { return pids[i] < pids[j] })
	}
	var colls []collection
	for _, p := range pids {
		hids, err := platlinux.FindHID(merc.VendorID, p)
		if err != nil {
			continue
		}
		for _, h := range hids {
			c, err := openInterface(h, p)
			if err != nil {
				for _, c := range colls {
					c.Close()
				}
				if errors.Is(err, os.ErrPermission) {
					err = fmt.Errorf("%w (run as root: hidraw and /dev/input need it)", err)
				}
				return nil, err
			}
			colls = append(colls, c)
		}
	}
	return colls, nil
}

func openInterface(h platlinux.HIDDevice, pid uint16) (*linuxCollection, error) {
	raw, err := platlinux.ReportDescriptor(h)
	if err != nil {
		return nil, err
	}
	desc, err := hidrd.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", h.SysPath, err)
	}
	if h.Hidraw == "" {
		return nil, fmt.Errorf("%s: no hidraw node", h.SysPath)
	}
	f, err := os.OpenFile(h.Hidraw, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	usb := filepath.Dir(filepath.Dir(h.SysPath))
	c := &linuxCollection{hid: h, f: f, desc: desc, raw: raw, usb: usb}
	c.info = collectionInfo{
		Name: filepath.Base(h.Hidraw), Product: sysfsText(usb, "product"),
		VendorID: merc.VendorID, ProductID: pid, Version: uint16(sysfsHex(usb, "bcdDevice")),
		InputLen: reportLen(desc, hidrd.Input), OutputLen: reportLen(desc, hidrd.Output), FeatureLen: reportLen(desc, hidrd.Feature),
		Readable: true,
	}
	if h.Interface >= 0 {
		c.info.Name = fmt.Sprintf("if%02d", h.Interface)
	}
	if len(desc.Collections) > 0 {
		c.info.UsagePage, c.info.Usage = desc.Collections[0].Usage.Page(), desc.Collections[0].Usage.ID()
	}
	return c, nil
}

func reportLen(desc *hidrd.Descriptor, kind hidrd.Kind) int {
	n := 0
	for _, id := range desc.ReportIDs(kind) {
		if l := desc.ReportLength(kind, id) + 1; l > n {
			n = l
		}
	}
	return n
}

func sysfsText(dir, name string) string {
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func sysfsHex(dir, name string) uint64 {
	v, _ := strconv.ParseUint(sysfsText(dir, name), 16, 16)
	return v
}

func (c *linuxCollection) Info() collectionInfo { return c.info }
func (c *linuxCollection) Descriptor() []byte   { return c.raw }
func (c *linuxCollection) Details() map[string]any {
	driver, _ := filepath.EvalSymlinks(filepath.Join(c.hid.SysPath, "driver"))
	inputs := map[string]any{}
	dirs, _ := filepath.Glob(filepath.Join(c.hid.SysPath, "input", "input*"))
	for _, dir := range dirs {
		leds, _ := filepath.Glob("/sys/class/leds/" + filepath.Base(dir) + "::*")
		for i, led := range leds {
			leds[i] = filepath.Base(led)
		}
		inputs[filepath.Base(dir)] = map[string]any{"name": sysfsText(dir, "name"), "phys": sysfsText(dir, "phys"), "leds": leds}
	}
	return map[string]any{
		"sysfs": c.hid.SysPath, "interface": c.hid.Interface, "driver": filepath.Base(driver),
		"hidraw": c.hid.Hidraw, "events": c.hid.Events, "inputs": inputs, "uevent": sysfsText(c.hid.SysPath, "uevent"),
	}
}
func (c *linuxCollection) Read() ([]byte, error) {
	buf := make([]byte, 256)
	n, err := c.f.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}
func (c *linuxCollection) Usages(report []byte) []hidrd.Usage {
	id, body := byte(0), report
	if c.desc.UsesReportIDs() && len(report) > 0 {
		id, body = report[0], report[1:]
	}
	return c.desc.ActiveUsages(id, body)
}
func (c *linuxCollection) GetFeature(id byte) ([]byte, error) {
	return platlinux.GetFeature(c.hid.Hidraw, id, c.desc.ReportLength(hidrd.Feature, id))
}
func (c *linuxCollection) GetInput(id byte) ([]byte, error) {
	return platlinux.GetInputReport(c.hid.Hidraw, id, c.desc.ReportLength(hidrd.Input, id))
}
func (c *linuxCollection) SetFeature(report []byte) error {
	return platlinux.SetFeature(c.hid.Hidraw, report)
}
func (c *linuxCollection) SetOutput(report []byte) error {
	return platlinux.WriteOutput(c.hid.Hidraw, report)
}
func (c *linuxCollection) Close() { c.f.Close() }

type linuxKeys struct {
	s        sink
	keyboard []*os.File
	own      []*os.File
	reports  []*os.File
	leds     *os.File
}

func startKeys(colls []collection, s sink) (keySource, error) {
	ks := &linuxKeys{s: s}
	for _, c := range colls {
		lc := c.(*linuxCollection)
		for _, path := range lc.hid.Events {
			f, err := os.OpenFile(path, os.O_RDWR, 0)
			if err != nil {
				ks.Stop()
				return nil, err
			}
			ks.keyboard = append(ks.keyboard, f)
			if lc.hid.HasLEDs && ks.leds == nil {
				ks.leds = f
			}
			go ks.read(f, filepath.Base(path), false)
		}
	}
	for _, path := range platlinux.FindInput(platlinux.VirtualName) {
		f, err := os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			continue
		}
		ks.own = append(ks.own, f)
		go ks.read(f, filepath.Base(path), true)
	}
	if ks.grabbedElsewhere() {
		fmt.Println("the keyboard's event nodes are grabbed (zengine?): its keys are read from the HID reports instead")
		for _, c := range colls {
			lc := c.(*linuxCollection)
			if lc.info.InputLen == 0 {
				continue
			}
			f, err := os.OpenFile(lc.hid.Hidraw, os.O_RDONLY, 0)
			if err != nil {
				continue
			}
			ks.reports = append(ks.reports, f)
			go ks.readReports(f, lc)
		}
	}
	return ks, nil
}

func (ks *linuxKeys) grabbedElsewhere() bool {
	for _, f := range ks.keyboard {
		if err := platlinux.Grab(f, true); err != nil {
			return errors.Is(err, unix.EBUSY)
		}
		platlinux.Grab(f, false)
	}
	return false
}

func (ks *linuxKeys) read(f *os.File, name string, own bool) {
	var scan uint32
	platlinux.ReadInputEvents(f, func(e platlinux.InputEvent) {
		switch e.Type {
		case platlinux.EvSyn:
			scan = 0
			return
		case platlinux.EvMsc:
			if e.Code == platlinux.MscScan {
				scan = uint32(e.Value)
			}
		}
		record := map[string]any{"src": name, "type": e.Type, "code": e.Code, "value": e.Value}
		if e.Type != platlinux.EvKey {
			ks.s.event(record)
			return
		}
		ev := keyEvent{down: e.Value != 0, repeat: e.Value == 2, own: own, record: record}
		if scan != 0 {
			ev.key, ev.known = keys.ByUsage(hidrd.Usage(scan))
		} else {
			ev.key, ev.known = keys.ByLinuxCode(e.Code)
		}
		ks.s.key(ev)
	})
}

func (ks *linuxKeys) readReports(f *os.File, c *linuxCollection) {
	var held []hidrd.Usage
	buf := make([]byte, 256)
	for {
		n, err := f.Read(buf)
		if err != nil {
			return
		}
		now := c.Usages(buf[:n])
		for _, u := range now {
			if !containsUsage(held, u) {
				ks.emit(u, true)
			}
		}
		for _, u := range held {
			if !containsUsage(now, u) {
				ks.emit(u, false)
			}
		}
		held = now
	}
}

func (ks *linuxKeys) emit(u hidrd.Usage, down bool) {
	ev := keyEvent{down: down}
	ev.key, ev.known = keys.ByUsage(u)
	ks.s.key(ev)
}

func containsUsage(list []hidrd.Usage, u hidrd.Usage) bool {
	for _, v := range list {
		if v == u {
			return true
		}
	}
	return false
}

func (ks *linuxKeys) Block(on bool)    { ks.grab(ks.keyboard, on) }
func (ks *linuxKeys) BlockOwn(on bool) { ks.grab(ks.own, on) }

func (ks *linuxKeys) grab(files []*os.File, on bool) {
	for _, f := range files {
		if err := platlinux.Grab(f, on); err != nil {
			ks.s.note(map[string]any{"src": filepath.Base(f.Name()), "grab_error": err.Error()})
		}
	}
}

func (ks *linuxKeys) KeysDown() int {
	n := 0
	for _, f := range ks.keyboard {
		held, _ := platlinux.HeldKeys(f)
		n += held
	}
	return n
}

func (ks *linuxKeys) Locks() (num, caps, scroll bool) {
	if ks.leds == nil {
		return
	}
	num, caps, scroll, _ = platlinux.LockLEDs(ks.leds)
	return
}

func (ks *linuxKeys) Stop() {
	for _, list := range [][]*os.File{ks.keyboard, ks.own, ks.reports} {
		for _, f := range list {
			f.Close()
		}
	}
}

func setLockLEDs(colls []collection, num, caps, scroll bool) error {
	for _, c := range colls {
		lc := c.(*linuxCollection)
		if !lc.hid.HasLEDs || len(lc.hid.Events) == 0 {
			continue
		}
		f, err := os.OpenFile(lc.hid.Events[0], os.O_RDWR, 0)
		if err != nil {
			return err
		}
		defer f.Close()
		return platlinux.WriteLockLEDs(f, num, caps, scroll)
	}
	return errors.New("no keyboard interface with lock LEDs")
}

func usbInfo(colls []collection) (map[string]any, []byte) {
	if len(colls) == 0 {
		return nil, nil
	}
	usb := colls[0].(*linuxCollection).usb
	info := map[string]any{"sysfs": usb}
	for _, name := range []string{"bcdDevice", "bMaxPower", "bNumInterfaces", "manufacturer", "product", "serial", "speed", "version"} {
		info[name] = sysfsText(usb, name)
	}
	descriptors, _ := os.ReadFile(filepath.Join(usb, "descriptors"))
	return info, descriptors
}

func platformFlags() func() (bool, int) { return nil }
