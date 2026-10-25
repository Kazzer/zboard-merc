//go:build linux

package main

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Kazzer/zboard-merc/internal/hidrd"
	platlinux "github.com/Kazzer/zboard-merc/internal/platform/linux"
)

const (
	mercIf00 = "05010906a101050719e029e71500250175019508810295017508810195037501050819012903910295057501910195067508150026ff00050719002aff008100c0"
	mercIf01 = "060c000901a101850125011500750109b509b609b709cd09e209e909ea0a8301950881020a8a010a92010a94010a21020a23020a24020a25020a2602950881020a27020a2a029502810295068101c00600ff0901a1018502750895078101150026ff00092075089502b102c0"
)

func fakeInterface(t *testing.T, root string, iface int, hid, desc string) platlinux.HIDDevice {
	t.Helper()
	write := func(path string, content []byte) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	usb := filepath.Join(root, "3-1")
	write(filepath.Join(usb, "product"), []byte("Merc\n"))
	write(filepath.Join(usb, "bcdDevice"), []byte("0130\n"))
	hidDir := filepath.Join(usb, fmt.Sprintf("3-1:1.%d", iface), hid)
	raw, _ := hex.DecodeString(desc)
	write(filepath.Join(hidDir, "report_descriptor"), raw)
	node := filepath.Join(root, "hidraw"+strconv.Itoa(iface))
	write(node, nil)
	return platlinux.HIDDevice{SysPath: hidDir, Interface: iface, HasLEDs: iface == 0, Hidraw: node}
}

func TestOpenInterfaceReadsTheMercLayout(t *testing.T) {
	root := t.TempDir()
	kbd, err := openInterface(fakeInterface(t, root, 0, "0003:1038:0210.0001", mercIf00), 0x0210)
	if err != nil {
		t.Fatal(err)
	}
	defer kbd.Close()
	cons, err := openInterface(fakeInterface(t, root, 1, "0003:1038:0210.0002", mercIf01), 0x0210)
	if err != nil {
		t.Fatal(err)
	}
	defer cons.Close()
	if info := kbd.Info(); info.Name != "if00" || info.Product != "Merc" || info.Version != 0x0130 || info.UsagePage != 1 || info.Usage != 6 ||
		info.InputLen != 9 || info.OutputLen != 2 || info.FeatureLen != 0 || !info.Readable {
		t.Errorf("if00: %+v", info)
	}
	if info := cons.Info(); info.Name != "if01" || info.UsagePage != 0x0C || info.Usage != 1 || info.InputLen != 8 || info.OutputLen != 0 || info.FeatureLen != 3 {
		t.Errorf("if01: %+v", info)
	}
	if commandChannel([]collection{kbd, cons}) != cons {
		t.Error("the command channel is not the vendor interface")
	}
	for _, tc := range []struct {
		c         collection
		raw, want string
	}{
		{kbd, "0000040000000000", "0007:0004"},
		{kbd, "0200000000000000", "0007:00E1"},
		{kbd, "0000000000000000", ""},
		{cons, "01000200", "000C:0192"},
		{cons, "01800000", "000C:0183"},
		{cons, "01000001", "000C:0227"},
	} {
		b, _ := hex.DecodeString(tc.raw)
		if got := strings.Join(usageStrings(tc.c.Usages(b)), " "); got != tc.want {
			t.Errorf("%s %s: usages %q, want %q", tc.c.Info().Name, tc.raw, got, tc.want)
		}
	}
	if ids := reportIDs(cons, hidrd.Feature); len(ids) != 1 || ids[0] != 2 {
		t.Errorf("if01 feature report ids %v, want [2]", ids)
	}
	if ids := reportIDs(kbd, hidrd.Input); len(ids) != 1 || ids[0] != 0 {
		t.Errorf("if00 input report ids %v, want [0]", ids)
	}
}

type fakeSink struct {
	mu     sync.Mutex
	keys   []keyEvent
	events []map[string]any
}

func (s *fakeSink) key(ev keyEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keys = append(s.keys, ev)
}

func (s *fakeSink) event(r map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, r)
}

func (s *fakeSink) note(map[string]any) {}

func (s *fakeSink) names() (out []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ev := range s.keys {
		out = append(out, fmt.Sprintf("%s:%v:%v", ev.key.Name, ev.down, ev.repeat))
	}
	return out
}

func encodeEvents(events []platlinux.InputEvent) []byte {
	var buf []byte
	for _, e := range events {
		var b [24]byte
		binary.LittleEndian.PutUint16(b[16:], e.Type)
		binary.LittleEndian.PutUint16(b[18:], e.Code)
		binary.LittleEndian.PutUint32(b[20:], uint32(e.Value))
		buf = append(buf, b[:]...)
	}
	return buf
}

func TestEvdevKeysAreNamedByUsageThenByCode(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	s := &fakeSink{}
	ks := &linuxKeys{s: s}
	done := make(chan struct{})
	go func() {
		ks.read(r, "event2", false)
		close(done)
	}()
	w.Write(encodeEvents([]platlinux.InputEvent{
		{Type: platlinux.EvMsc, Code: platlinux.MscScan, Value: 0x70004},
		{Type: platlinux.EvKey, Code: 30, Value: 1},
		{Type: platlinux.EvSyn},
		{Type: platlinux.EvKey, Code: 30, Value: 2},
		{Type: platlinux.EvSyn},
		{Type: platlinux.EvKey, Code: 30, Value: 0},
		{Type: platlinux.EvSyn},
	}))
	w.Close()
	<-done
	if got := strings.Join(s.names(), " "); got != "a:true:false a:true:true a:false:false" {
		t.Errorf("keys %s", got)
	}
	if len(s.events) != 1 || s.events[0]["type"] != uint16(platlinux.EvMsc) {
		t.Errorf("non-key records %v", s.events)
	}
	if rec := s.keys[0].record; rec["src"] != "event2" || rec["code"] != uint16(30) || rec["value"] != int32(1) {
		t.Errorf("key record %v", rec)
	}
}

func TestReportsBecomeKeyTransitions(t *testing.T) {
	cons, err := openInterface(fakeInterface(t, t.TempDir(), 0, "0003:1038:0210.0001", mercIf00), 0x0210)
	if err != nil {
		t.Fatal(err)
	}
	defer cons.Close()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	s := &fakeSink{}
	ks := &linuxKeys{s: s}
	done := make(chan struct{})
	go func() {
		ks.readReports(r, cons)
		close(done)
	}()
	for i, raw := range []string{"0000040000000000", "0200040000000000", "0200000000000000", "0000000000000000"} {
		b, _ := hex.DecodeString(raw)
		w.Write(b)
		for deadline := time.Now().Add(time.Second); len(s.names()) <= i && time.Now().Before(deadline); {
			time.Sleep(time.Millisecond)
		}
	}
	w.Close()
	<-done
	if got := strings.Join(s.names(), " "); got != "a:true:false left-shift:true:false a:false:false left-shift:false:false" {
		t.Errorf("keys %s", got)
	}
	if len(s.events) != 0 || s.keys[0].record != nil {
		t.Error("report-derived keys must not log records of their own")
	}
}
