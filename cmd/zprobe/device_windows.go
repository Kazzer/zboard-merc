//go:build windows

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Kazzer/zboard-merc/internal/hidrd"
	"github.com/Kazzer/zboard-merc/internal/keys"
	"github.com/Kazzer/zboard-merc/internal/merc"
	platwin "github.com/Kazzer/zboard-merc/internal/platform/windows"
)

type winCollection struct {
	d    *platwin.Device
	info collectionInfo
}

func enumerate(pid uint16) ([]collection, error) {
	devs, err := platwin.Enumerate(merc.VendorID, pid)
	var colls []collection
	for _, d := range devs {
		colls = append(colls, &winCollection{d: d, info: collectionInfo{
			Name: collectionName(d.Path), Product: d.Product,
			VendorID: d.VendorID, ProductID: d.ProductID, Version: d.Version,
			UsagePage: d.Caps.UsagePage, Usage: d.Caps.Usage,
			InputLen: int(d.Caps.InputLen), OutputLen: int(d.Caps.OutputLen), FeatureLen: int(d.Caps.FeatureLen),
			Readable: !d.ReadOnly,
		}})
	}
	return colls, err
}

func collectionName(path string) string {
	up := strings.ToUpper(path)
	suffix := ""
	if i := strings.Index(up, "&COL"); i >= 0 {
		suffix = "-" + strings.ToLower(up[i+1:i+6])
	}
	switch {
	case strings.Contains(up, "MI_00"):
		return "if00" + suffix
	case strings.Contains(up, "MI_01"):
		return "if01" + suffix
	}
	return "hid" + suffix
}

func (c *winCollection) Info() collectionInfo { return c.info }
func (c *winCollection) Descriptor() []byte   { return nil }
func (c *winCollection) Details() map[string]any {
	return map[string]any{"path": c.d.Path, "caps": c.d.Caps}
}
func (c *winCollection) Read() ([]byte, error) { return c.d.Read() }
func (c *winCollection) Usages(report []byte) []hidrd.Usage {
	var out []hidrd.Usage
	for _, u := range c.d.Usages(report) {
		out = append(out, hidrd.Usage(uint32(u.Page)<<16|uint32(u.Usage)))
	}
	return out
}
func (c *winCollection) GetFeature(id byte) ([]byte, error) { return c.d.GetFeature(id) }
func (c *winCollection) GetInput(id byte) ([]byte, error)   { return c.d.GetInputReport(id) }
func (c *winCollection) SetFeature(report []byte) error     { return c.d.SetFeature(report) }
func (c *winCollection) SetOutput(report []byte) error      { return c.d.SetOutputReport(report) }
func (c *winCollection) Close()                             { c.d.Close() }

type winKeys struct {
	h *platwin.Hook
}

func startKeys(colls []collection, s sink) (keySource, error) {
	events := make(chan platwin.KeyEvent, 256)
	h, err := platwin.StartHook(events)
	if err != nil {
		return nil, err
	}
	go func() {
		for ev := range events {
			k, ok := keys.ByScan(uint16(ev.Scan), ev.Extended, uint8(ev.VK))
			s.key(keyEvent{key: k, known: ok, down: !ev.Up, repeat: ev.Repeat, own: ev.Own(),
				record: map[string]any{"src": "llhook", "vk": ev.VK, "scan": ev.Scan, "flags": ev.Flags,
					"up": ev.Up, "ext": ev.Extended, "injected": ev.Injected, "extra": ev.Extra}})
		}
	}()
	return &winKeys{h: h}, nil
}

func (k *winKeys) Block(on bool)    { k.h.Block(on) }
func (k *winKeys) BlockOwn(on bool) { k.h.BlockOwn(on) }
func (k *winKeys) KeysDown() int    { return k.h.KeysDown() }
func (k *winKeys) Locks() (num, caps, scroll bool) {
	l := k.h.Locks()
	return l.Num, l.Caps, l.Scroll
}
func (k *winKeys) Stop() { k.h.Stop() }

func setLockLEDs(colls []collection, num, caps, scroll bool) error {
	for _, c := range colls {
		info := c.Info()
		if info.UsagePage != 1 || info.Usage != 6 || info.OutputLen == 0 {
			continue
		}
		report := make([]byte, info.OutputLen)
		for i, on := range []bool{num, caps, scroll} {
			if on {
				report[1] |= 1 << i
			}
		}
		return c.SetOutput(report)
	}
	return errors.New("no keyboard collection with an output report")
}

func usbInfo(colls []collection) (map[string]any, []byte) { return nil, nil }

func platformFlags() func() (bool, int) {
	selfTest := flag.Bool("selftest-hook", false, "install the keyboard hook, inject one key, and check it arrives")
	lockLeds := flag.Int("lock-leds", -1, "set the keyboard's lock LEDs once through driver/umdf's channel (1 Num, 2 Caps, 4 Scroll, added up) and exit; stop zengine first")
	return func() (bool, int) {
		if *selfTest {
			return true, hookSelfTest()
		}
		if *lockLeds >= 0 {
			if err := platwin.WriteLockLEDs(byte(*lockLeds)); err != nil {
				fmt.Fprintln(os.Stderr, "lock LEDs:", err)
				return true, 1
			}
			fmt.Printf("lock LEDs set to %#x\n", *lockLeds)
			return true, 0
		}
		return false, 0
	}
}

func hookSelfTest() int {
	events := make(chan platwin.KeyEvent, 16)
	h, err := platwin.StartHook(events)
	if err != nil {
		fmt.Println("hook:", err)
		return 1
	}
	if os.Getenv("ZPROBE_SELFTEST_WINDOW") != "" {
		platwin.ShowWindow("zprobe self-test")
		time.Sleep(300 * time.Millisecond)
	}
	platwin.InjectKey(0x41, false)
	platwin.InjectKey(0x41, true)
	got := 0
	timeout := time.After(3 * time.Second)
	for got < 2 {
		select {
		case ev := <-events:
			fmt.Printf("hook event: vk 0x%02X scan 0x%02X up %v injected %v\n", ev.VK, ev.Scan, ev.Up, ev.Injected)
			if ev.VK == 0x41 && ev.Own() {
				got++
			}
		case <-timeout:
			fmt.Printf("hook: injected key never arrived (callback invoked %d times)\n", h.Calls.Load())
			h.Stop()
			return 1
		}
	}
	h.Stop()
	fmt.Println("hook self-test passed")
	return 0
}
