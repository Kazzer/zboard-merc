//go:build windows

package platwin

import (
	"errors"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestLockLEDs(t *testing.T) {
	srv, err := windows.CreateNamedPipe(windows.StringToUTF16Ptr(lockLEDPipe), windows.PIPE_ACCESS_INBOUND,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT, 1, 16, 16, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(srv)
	got := make(chan byte, 8)
	go func() {
		defer close(got)
		buf := make([]byte, 1)
		for {
			if err := windows.ConnectNamedPipe(srv, nil); err != nil && !errors.Is(err, windows.ERROR_PIPE_CONNECTED) {
				return
			}
			for {
				var n uint32
				if windows.ReadFile(srv, buf, &n, nil) != nil || n != 1 {
					break
				}
				got <- buf[0]
			}
			windows.DisconnectNamedPipe(srv)
		}
	}()
	next := func() byte {
		select {
		case b, ok := <-got:
			if !ok {
				t.Fatal("server closed")
			}
			return b
		case <-time.After(5 * time.Second):
			t.Fatal("nothing arrived")
		}
		return 0
	}
	l := OpenLockLEDs()
	defer l.Close()
	l.Set(LockState{Num: true, Caps: true})
	if b := next(); b != ledNum|ledCaps {
		t.Fatalf("got %#x, want num+caps", b)
	}
	l.Set(LockState{Scroll: true})
	if b := next(); b != ledScroll {
		t.Fatalf("got %#x, want scroll", b)
	}
	l.Suspend()
	l.Set(LockState{Num: true})
	l.Resume()
	if b := next(); b != ledNum {
		t.Fatalf("after resume got %#x, want num", b)
	}
}
