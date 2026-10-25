//go:build windows

package platwin

import (
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"golang.org/x/sys/windows"
)

const lockLEDPipe = `\\.\pipe\zboardhid`

const (
	ledNum    = 0x01
	ledCaps   = 0x02
	ledScroll = 0x04
)

const lockLEDPeriod = 5 * time.Second

type LockLEDs struct {
	want      chan byte
	hold      chan bool
	stop      chan struct{}
	done      chan struct{}
	connected atomic.Bool
}

func (l *LockLEDs) Suspend() { l.hold <- true }
func (l *LockLEDs) Resume()  { l.hold <- false }

func (l *LockLEDs) Connected() bool { return l.connected.Load() }

func OpenLockLEDs() *LockLEDs {
	l := &LockLEDs{want: make(chan byte, 1), hold: make(chan bool), stop: make(chan struct{}), done: make(chan struct{})}
	go l.run()
	return l
}

func (l *LockLEDs) Set(s LockState) {
	select {
	case <-l.want:
	default:
	}
	select {
	case l.want <- lockMask(s):
	default:
	}
}

func (l *LockLEDs) Close() {
	close(l.stop)
	<-l.done
}

func lockMask(s LockState) byte {
	var m byte
	if s.Num {
		m |= ledNum
	}
	if s.Caps {
		m |= ledCaps
	}
	if s.Scroll {
		m |= ledScroll
	}
	return m
}

func (l *LockLEDs) run() {
	defer close(l.done)
	h := windows.InvalidHandle
	var mask byte
	have, told := false, false
	connect := func() {
		if h != windows.InvalidHandle {
			return
		}
		var err error
		if h, err = openLockLEDPipe(); err != nil {
			h = windows.InvalidHandle
			if !told {
				slog.Info("lock LEDs: no driver channel; the keyboard's own lock LEDs apply", "error", err)
				told = true
			}
			return
		}
		slog.Info("lock LEDs: following Windows through the driver")
		told = false
		l.connected.Store(true)
	}
	write := func() {
		if h == windows.InvalidHandle || !have {
			return
		}
		var n uint32
		if err := windows.WriteFile(h, []byte{mask}, &n, nil); err != nil {
			slog.Warn("lock LEDs: driver channel lost", "error", err)
			windows.CloseHandle(h)
			h = windows.InvalidHandle
			l.connected.Store(false)
		}
	}
	release := func() {
		if h != windows.InvalidHandle {
			windows.CloseHandle(h)
			h = windows.InvalidHandle
			l.connected.Store(false)
		}
	}
	tick := time.NewTicker(lockLEDPeriod)
	defer tick.Stop()
	suspended := false
	connect()
	for {
		select {
		case mask = <-l.want:
			have = true
			if !suspended {
				connect()
				write()
			}
		case <-tick.C:
			if !suspended {
				connect()
				write()
			}
		case suspended = <-l.hold:
			if suspended {
				release()
				slog.Info("lock LEDs: left to the driver while the secure desktop is up")
			} else {
				connect()
				write()
			}
		case <-l.stop:
			release()
			return
		}
	}
}

func openLockLEDPipe() (windows.Handle, error) {
	return windows.CreateFile(windows.StringToUTF16Ptr(lockLEDPipe), windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, 0, 0)
}

func WriteLockLEDs(mask byte) error {
	h, err := openLockLEDPipe()
	if err != nil {
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
			return errors.New("no driver channel: driver/umdf is not running on the keyboard")
		}
		if errors.Is(err, windows.ERROR_PIPE_BUSY) {
			return errors.New("the channel is held by zengine; stop it first")
		}
		return err
	}
	defer windows.CloseHandle(h)
	var n uint32
	return windows.WriteFile(h, []byte{mask}, &n, nil)
}
