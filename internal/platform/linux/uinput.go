//go:build linux

package platlinux

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	uiSetEvBit  = 0x40045564
	uiSetKeyBit = 0x40045565
	uiSetMscBit = 0x40045568
	uiSetLEDBit = 0x40045569
	uiDevSetup  = 0x405C5503
	uiDevCreate = 0x5501
	uiDestroy   = 0x5502
)

type Virtual struct {
	f *os.File
}

func CreateVirtual(name string, vendor, product uint16, keyCodes []uint16) (*Virtual, error) {
	f, err := os.OpenFile("/dev/uinput", os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	v := &Virtual{f: f}
	set := func(req uintptr, value int) error {
		return unix.IoctlSetInt(int(f.Fd()), uint(req), value)
	}
	for _, ev := range []int{EvKey, EvLED, EvRep, EvMsc} {
		if err := set(uiSetEvBit, ev); err != nil {
			f.Close()
			return nil, fmt.Errorf("UI_SET_EVBIT %d: %w", ev, err)
		}
	}
	for _, c := range keyCodes {
		if err := set(uiSetKeyBit, int(c)); err != nil {
			f.Close()
			return nil, fmt.Errorf("UI_SET_KEYBIT %d: %w", c, err)
		}
	}
	for led := 0; led < 5; led++ {
		if err := set(uiSetLEDBit, led); err != nil {
			f.Close()
			return nil, fmt.Errorf("UI_SET_LEDBIT %d: %w", led, err)
		}
	}
	if err := set(uiSetMscBit, MscScan); err != nil {
		f.Close()
		return nil, fmt.Errorf("UI_SET_MSCBIT: %w", err)
	}
	var setup [92]byte
	binary.LittleEndian.PutUint16(setup[0:], 0x03)
	binary.LittleEndian.PutUint16(setup[2:], vendor)
	binary.LittleEndian.PutUint16(setup[4:], product)
	binary.LittleEndian.PutUint16(setup[6:], 1)
	copy(setup[8:8+79], name)
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), uiDevSetup, uintptr(unsafe.Pointer(&setup[0]))); errno != 0 {
		f.Close()
		return nil, fmt.Errorf("UI_DEV_SETUP: %w", errno)
	}
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), uiDevCreate, 0); errno != 0 {
		f.Close()
		return nil, fmt.Errorf("UI_DEV_CREATE: %w", errno)
	}
	return v, nil
}

func (v *Virtual) write(events ...InputEvent) error {
	_, err := v.f.Write(encodeEvents(events))
	return err
}

func (v *Virtual) Key(code uint16, down bool) error {
	value := int32(0)
	if down {
		value = 1
	}
	if debugOn() {
		slog.Debug("out", "code", code, "down", down)
	}
	return v.write(InputEvent{Type: EvKey, Code: code, Value: value}, InputEvent{Type: EvSyn})
}

func (v *Virtual) ReadLEDs(fn func(led uint16, on bool)) {
	var b [24]byte
	for {
		if _, err := v.f.Read(b[:]); err != nil {
			return
		}
		typ := binary.LittleEndian.Uint16(b[16:])
		if debugOn() && typ != EvSyn {
			slog.Debug("uinput readback", "type", typ, "code", binary.LittleEndian.Uint16(b[18:]), "value", int32(binary.LittleEndian.Uint32(b[20:])))
		}
		if typ == EvLED {
			fn(binary.LittleEndian.Uint16(b[18:]), binary.LittleEndian.Uint32(b[20:]) != 0)
		}
	}
}

func (v *Virtual) Close() error {
	unix.Syscall(unix.SYS_IOCTL, v.f.Fd(), uiDestroy, 0)
	return v.f.Close()
}
