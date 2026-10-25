//go:build linux

package platlinux

import (
	"encoding/binary"
	"math/bits"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/unix"
)

const VirtualName = "Zboard virtual keyboard"

const (
	EvSyn = 0x00
	EvKey = 0x01
	EvMsc = 0x04
	EvLED = 0x11
	EvRep = 0x14

	MscScan = 0x04
)

const (
	eviocgrab = 0x40044590
	eviocgkey = 0x80604518
	eviocgled = 0x80084519
)

type InputEvent struct {
	Sec, Usec int64
	Type      uint16
	Code      uint16
	Value     int32
}

func encodeEvents(events []InputEvent) []byte {
	buf := make([]byte, 0, 24*len(events))
	for _, e := range events {
		var b [24]byte
		binary.LittleEndian.PutUint16(b[16:], e.Type)
		binary.LittleEndian.PutUint16(b[18:], e.Code)
		binary.LittleEndian.PutUint32(b[20:], uint32(e.Value))
		buf = append(buf, b[:]...)
	}
	return buf
}

func decodeEvent(b [24]byte) InputEvent {
	return InputEvent{
		Sec:   int64(binary.LittleEndian.Uint64(b[0:])),
		Usec:  int64(binary.LittleEndian.Uint64(b[8:])),
		Type:  binary.LittleEndian.Uint16(b[16:]),
		Code:  binary.LittleEndian.Uint16(b[18:]),
		Value: int32(binary.LittleEndian.Uint32(b[20:])),
	}
}

func Grab(f *os.File, on bool) error {
	v := 0
	if on {
		v = 1
	}
	return ioctlInt(f, eviocgrab, v)
}

func HeldKeys(f *os.File) (int, error) {
	var state [96]byte
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), eviocgkey, uintptr(unsafe.Pointer(&state[0]))); errno != 0 {
		return 0, errno
	}
	n := 0
	for _, b := range state {
		n += bits.OnesCount8(b)
	}
	return n, nil
}

func LockLEDs(f *os.File) (num, caps, scroll bool, err error) {
	var state [8]byte
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), eviocgled, uintptr(unsafe.Pointer(&state[0]))); errno != 0 {
		return false, false, false, errno
	}
	return state[0]&1 != 0, state[0]&2 != 0, state[0]&4 != 0, nil
}

func WriteLockLEDs(f *os.File, num, caps, scroll bool) error {
	on := func(b bool) int32 {
		if b {
			return 1
		}
		return 0
	}
	_, err := f.Write(encodeEvents([]InputEvent{
		{Type: EvLED, Code: 0, Value: on(num)},
		{Type: EvLED, Code: 1, Value: on(caps)},
		{Type: EvLED, Code: 2, Value: on(scroll)},
		{Type: EvSyn},
	}))
	return err
}

func ReadInputEvents(f *os.File, fn func(InputEvent)) error {
	var b [24]byte
	for {
		if _, err := f.Read(b[:]); err != nil {
			return err
		}
		fn(decodeEvent(b))
	}
}

func FindInput(name string) []string {
	var nodes []string
	dirs, _ := filepath.Glob(sysfsRoot + "/class/input/input*")
	for _, dir := range dirs {
		b, err := os.ReadFile(filepath.Join(dir, "name"))
		if err != nil || strings.TrimSpace(string(b)) != name {
			continue
		}
		evs, _ := filepath.Glob(filepath.Join(dir, "event*"))
		for _, ev := range evs {
			nodes = append(nodes, "/dev/input/"+filepath.Base(ev))
		}
	}
	return nodes
}
