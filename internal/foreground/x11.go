//go:build linux

package foreground

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

type x11 struct {
	conn         *xgb.Conn
	root         xproto.Window
	activeWindow xproto.Atom
	wmPID        xproto.Atom
}

func openX11() (*x11, error) {
	if os.Getenv("DISPLAY") == "" {
		return nil, fmt.Errorf("no DISPLAY")
	}
	conn, err := xgb.NewConn()
	if err != nil {
		return nil, err
	}
	x := &x11{conn: conn, root: xproto.Setup(conn).DefaultScreen(conn).Root}
	for name, dst := range map[string]*xproto.Atom{"_NET_ACTIVE_WINDOW": &x.activeWindow, "_NET_WM_PID": &x.wmPID} {
		r, err := xproto.InternAtom(conn, true, uint16(len(name)), name).Reply()
		if err != nil || r.Atom == 0 {
			conn.Close()
			return nil, fmt.Errorf("window manager does not provide %s", name)
		}
		*dst = r.Atom
	}
	return x, nil
}

func (x *x11) process() string {
	r, err := xproto.GetProperty(x.conn, false, x.root, x.activeWindow, xproto.AtomWindow, 0, 1).Reply()
	if err != nil || len(r.Value) < 4 {
		return ""
	}
	win := xproto.Window(xgb.Get32(r.Value))
	if win == 0 {
		return ""
	}
	p, err := xproto.GetProperty(x.conn, false, win, x.wmPID, xproto.AtomCardinal, 0, 1).Reply()
	if err != nil || len(p.Value) < 4 {
		return ""
	}
	return processName(int(xgb.Get32(p.Value)))
}

func processName(pid int) string {
	if exe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid)); err == nil {
		return filepath.Base(exe)
	}
	if comm, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid)); err == nil {
		return strings.TrimSpace(string(comm))
	}
	return ""
}

func Watch(stop <-chan struct{}, fn func(process string)) error {
	x, err := openX11()
	if err != nil {
		return err
	}
	go func() {
		defer x.conn.Close()
		last := ""
		t := time.NewTicker(300 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				if p := x.process(); p != last {
					last = p
					fn(p)
				}
			}
		}
	}()
	return nil
}
