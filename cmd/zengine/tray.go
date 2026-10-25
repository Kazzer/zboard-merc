package main

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"fyne.io/systray"

	"github.com/Kazzer/zboard-merc/internal/browser"
)

type trayUI struct {
	mu      sync.Mutex
	ready   bool
	s       *session
	readyCh chan struct{}
	stop    <-chan struct{}
	quit    func()
	once    sync.Once

	gen      chan struct{}
	title    *systray.MenuItem
	auto     *systray.MenuItem
	modItems map[string]*systray.MenuItem
	flags    map[string]*systray.MenuItem
	last     traySnapshot
	icon     string
}

type traySnapshot struct {
	mods            []string
	active, pinned  string
	flags           map[string]bool
	enabled, action bool
	attached        bool
}

func newTrayUI(stop <-chan struct{}, quit func()) *trayUI {
	return &trayUI{readyCh: make(chan struct{}), stop: stop, quit: quit}
}

func (t *trayUI) onReady() {
	systray.SetTooltip("zboard")
	systray.SetTitle("zboard")
	t.setIcon(false, false, true)
	t.mu.Lock()
	t.ready = true
	s := t.s
	t.mu.Unlock()
	close(t.readyCh)
	if s != nil {
		t.start(s)
	}
}

func (t *trayUI) attach(s *session) {
	t.mu.Lock()
	t.s = s
	ready := t.ready
	t.mu.Unlock()
	if ready {
		t.start(s)
	}
}

func (t *trayUI) start(s *session) {
	t.once.Do(func() { go t.follow(s) })
}

func (t *trayUI) setIcon(enabled, action, attached bool) {
	key := fmt.Sprintf("%v/%v/%v", enabled, action, attached)
	if key == t.icon {
		return
	}
	t.icon = key
	systray.SetIcon(trayIcon(enabled, action, attached))
}

func snapshotOf(s *session) traySnapshot {
	s.sel.mu.Lock()
	snap := traySnapshot{flags: s.eng.Flags()}
	for _, m := range s.sel.mods {
		if m.Menu {
			snap.mods = append(snap.mods, m.Name)
		}
	}
	if s.sel.pinned != nil {
		snap.pinned = s.sel.pinned.Name
	}
	s.sel.mu.Unlock()
	if m := s.eng.Active(); m != nil {
		snap.active = m.Name
	}
	snap.enabled, snap.action = s.eng.LEDState()
	snap.attached = s.keyboardAttached()
	return snap
}

func flagNames(flags map[string]bool) []string {
	names := make([]string, 0, len(flags))
	for n := range flags {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func (t *trayUI) follow(s *session) {
	t.rebuild(s, snapshotOf(s))
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-t.stop:
			return
		case <-tick.C:
		}
		snap := snapshotOf(s)
		if strings.Join(snap.mods, "\x00") != strings.Join(t.last.mods, "\x00") ||
			strings.Join(flagNames(snap.flags), "\x00") != strings.Join(flagNames(t.last.flags), "\x00") {
			t.rebuild(s, snap)
			continue
		}
		t.update(snap)
	}
}

func (t *trayUI) rebuild(s *session, snap traySnapshot) {
	if t.gen != nil {
		close(t.gen)
	}
	gen := make(chan struct{})
	t.gen = gen
	systray.ResetMenu()
	t.title = systray.AddMenuItem("zboard", "")
	t.title.Disable()
	t.auto = systray.AddMenuItemCheckbox("Automatic", "follow the window in front", snap.pinned == "")
	t.watch(gen, t.auto, func() { s.sel.pin("") })
	t.modItems = map[string]*systray.MenuItem{}
	for _, name := range snap.mods {
		item := systray.AddMenuItemCheckbox(name, "pin this Mod; a matching trigger still wins", snap.pinned == name)
		t.modItems[name] = item
		t.watch(gen, item, func() {
			if err := s.sel.pin(name); err != nil {
				slog.Warn("tray", "error", err)
			}
		})
	}
	systray.AddSeparator()
	t.flags = map[string]*systray.MenuItem{}
	for _, name := range flagNames(snap.flags) {
		item := systray.AddMenuItemCheckbox("flag "+name, "", snap.flags[name])
		t.flags[name] = item
		t.watch(gen, item, func() { s.eng.SetFlag(name, !s.eng.Flags()[name]) })
	}
	if len(snap.flags) > 0 {
		systray.AddSeparator()
	}
	t.watch(gen, systray.AddMenuItem("Open settings…", "the web UI in the browser"), func() {
		link := s.uiLink()
		if link == "" {
			slog.Warn("tray", "error", "the control API is off, so there is no UI")
			return
		}
		if err := browser.Open(link); err != nil {
			slog.Warn("tray", "error", err)
		}
	})
	t.watch(gen, systray.AddMenuItem("Reload files", "re-read config.yaml and the Mods"), func() {
		if _, err := s.reload(); err != nil {
			slog.Warn("reload", "error", err)
		}
	})
	systray.AddSeparator()
	t.watch(gen, systray.AddMenuItem("Quit zboard", ""), t.quit)
	t.last = traySnapshot{active: "\x00"}
	t.update(snap)
}

func (t *trayUI) watch(gen chan struct{}, item *systray.MenuItem, fn func()) {
	go func() {
		for {
			select {
			case <-gen:
				return
			case <-t.stop:
				return
			case _, ok := <-item.ClickedCh:
				if !ok {
					return
				}
				fn()
			}
		}
	}()
}

func (t *trayUI) update(snap traySnapshot) {
	if snap.active != t.last.active || snap.attached != t.last.attached {
		switch {
		case !snap.attached:
			t.title.SetTitle("zboard: keyboard not found")
		case snap.active == "":
			t.title.SetTitle("zboard: bare keyboard")
		default:
			t.title.SetTitle("zboard: " + snap.active)
		}
	}
	if snap.pinned != t.last.pinned || snap.active != t.last.active {
		check(t.auto, snap.pinned == "")
		for name, item := range t.modItems {
			check(item, snap.pinned == name)
			if name == snap.active {
				item.SetTitle(name + "  ●")
			} else {
				item.SetTitle(name)
			}
		}
	}
	for name, item := range t.flags {
		if snap.flags[name] != t.last.flags[name] {
			check(item, snap.flags[name])
		}
	}
	t.setIcon(snap.enabled, snap.action, snap.attached)
	t.last = snap
}

func check(item *systray.MenuItem, on bool) {
	if on {
		item.Check()
	} else {
		item.Uncheck()
	}
}
