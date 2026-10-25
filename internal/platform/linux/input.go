//go:build linux

package platlinux

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/Kazzer/zboard-merc/internal/engine"
	"github.com/Kazzer/zboard-merc/internal/hidrd"
	"github.com/Kazzer/zboard-merc/internal/keys"
)

func debugOn() bool { return slog.Default().Enabled(context.Background(), slog.LevelDebug) }

type Keyboard struct {
	HID    []HIDDevice
	nodes  []*os.File
	ledOut *os.File
	mu     sync.Mutex
}

func OpenKeyboard(vid, pid uint16) (*Keyboard, error) {
	hid, err := FindHID(vid, pid)
	if err != nil {
		return nil, err
	}
	k := &Keyboard{HID: hid}
	for _, d := range hid {
		for _, path := range d.Events {
			f, err := os.OpenFile(path, os.O_RDWR, 0)
			if err != nil {
				k.Close()
				return nil, fmt.Errorf("%s: %w", path, err)
			}
			if err := Grab(f, true); err != nil {
				k.Close()
				return nil, fmt.Errorf("%s: grab: %w", path, err)
			}
			k.nodes = append(k.nodes, f)
			if (d.Interface == 0 || d.HasLEDs) && k.ledOut == nil {
				k.ledOut = f
			}
		}
	}
	if len(k.nodes) == 0 {
		return nil, fmt.Errorf("device %04x:%04x has no event nodes", vid, pid)
	}
	return k, nil
}

func (k *Keyboard) Read(fn func(engine.Event)) {
	var wg sync.WaitGroup
	for _, f := range k.nodes {
		wg.Add(1)
		go func(f *os.File) {
			defer wg.Done()
			var scan uint32
			var b [24]byte
			for {
				if _, err := f.Read(b[:]); err != nil {
					return
				}
				typ, code, value := binary.LittleEndian.Uint16(b[16:]), binary.LittleEndian.Uint16(b[18:]), int32(binary.LittleEndian.Uint32(b[20:]))
				switch {
				case typ == EvMsc && code == MscScan:
					scan = uint32(value)
				case typ == EvKey && value != 2:
					u := hidrd.Usage(scan)
					if scan == 0 {
						key, ok := keys.ByLinuxCode(code)
						if !ok {
							slog.Warn("key without usage", "code", code)
							continue
						}
						u = key.Usage
					}
					if debugOn() {
						slog.Debug("in", "usage", u, "down", value == 1)
					}
					fn(engine.Event{Usage: u, Down: value == 1, Time: time.Now()})
				case typ == EvSyn:
					scan = 0
				}
			}
		}(f)
	}
	wg.Wait()
}

func (k *Keyboard) Locks() (num, caps, scroll bool) {
	if k.ledOut == nil {
		return
	}
	num, caps, scroll, _ = LockLEDs(k.ledOut)
	return
}

func (k *Keyboard) SetLocks(num, caps, scroll bool) {
	if k.ledOut == nil {
		return
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if err := WriteLockLEDs(k.ledOut, num, caps, scroll); err != nil {
		slog.Warn("lock LEDs", "error", err)
	}
	if debugOn() {
		slog.Debug("locks written", "num", num, "caps", caps, "scroll", scroll)
	}
}

func (k *Keyboard) FeatureNode() string {
	node := ""
	for _, d := range k.HID {
		if d.Hidraw == "" {
			continue
		}
		if d.Interface == 1 || node == "" && !d.HasLEDs {
			node = d.Hidraw
		}
	}
	if node == "" {
		for _, d := range k.HID {
			if d.Hidraw != "" {
				return d.Hidraw
			}
		}
	}
	return node
}

func (k *Keyboard) Feature(report []byte) error {
	node := k.FeatureNode()
	if node == "" {
		return fmt.Errorf("no hidraw node")
	}
	return SetFeature(node, report)
}

func (k *Keyboard) Close() {
	for _, f := range k.nodes {
		Grab(f, false)
		f.Close()
	}
	k.nodes = nil
}
