//go:build windows

package platwin

import (
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32            = windows.NewLazySystemDLL("user32.dll")
	setWindowsHookEx  = user32.NewProc("SetWindowsHookExW")
	unhookWindowsHook = user32.NewProc("UnhookWindowsHookEx")
	callNextHookEx    = user32.NewProc("CallNextHookEx")
	getMessage        = user32.NewProc("GetMessageW")
	translateMessage  = user32.NewProc("TranslateMessage")
	dispatchMessage   = user32.NewProc("DispatchMessageW")
	postThreadMessage = user32.NewProc("PostThreadMessageW")
	getKeyState       = user32.NewProc("GetKeyState")
)

var (
	keybdEvent          = user32.NewProc("keybd_event")
	registerClassEx     = user32.NewProc("RegisterClassExW")
	createWindowEx      = user32.NewProc("CreateWindowExW")
	defWindowProc       = user32.NewProc("DefWindowProcW")
	setForegroundWindow = user32.NewProc("SetForegroundWindow")
	getModuleHandle     = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetModuleHandleW")
)

const (
	whKeyboardLL = 13
	wmQuit       = 0x0012
	wmReseed     = 0x8001

	vkCapital = 0x14
	vkNumLock = 0x90
	vkScroll  = 0x91
	vkNoName  = 0xFC
)

type kbdllhookstruct struct {
	VK, Scan, Flags, Time uint32
	ExtraInfo             uintptr
}

const InjectTag = 0x5A424F41

type LockState struct {
	Num, Caps, Scroll bool
}

type KeyEvent struct {
	VK, Scan, Flags, Time  uint32
	Extra                  uintptr
	Up, Extended, Injected bool
	Repeat                 bool
	Locks                  LockState
	ShiftHeld              bool
}

func (ev KeyEvent) KeypadNavigation() bool {
	if ev.Extended || ev.ShiftHeld || ev.Scan < 0x47 || ev.Scan > 0x53 || ev.Scan == 0x4A || ev.Scan == 0x4E {
		return false
	}
	return ev.VK >= 0x21 && ev.VK <= 0x28 || ev.VK == 0x2D || ev.VK == 0x2E || ev.VK == 0x0C
}

func (ev KeyEvent) KeypadDigit() bool {
	if ev.Extended || ev.Scan < 0x47 || ev.Scan > 0x53 || ev.Scan == 0x4A || ev.Scan == 0x4E {
		return false
	}
	return ev.VK >= 0x60 && ev.VK <= 0x69 || ev.VK == 0x6E
}

func (ev KeyEvent) Own() bool { return ev.Injected && ev.Extra == InjectTag }

type Hook struct {
	events   chan<- KeyEvent
	filter   func(KeyEvent) bool
	block    atomic.Bool
	blockOwn atomic.Bool
	tid      uint32
	done     chan struct{}
	Calls    atomic.Int64
	reseed   atomic.Bool

	mu        sync.Mutex
	down      map[uint32]bool
	swallowed map[uint32]bool
	locks     LockState
	lockDown  map[uint32]bool
	onLocks   func(LockState)
	ownHidden map[uint32]bool
	ownShift  map[uint32]bool
}

func (h *Hook) SetLockListener(fn func(LockState)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onLocks = fn
}

func newHook(events chan<- KeyEvent, filter func(KeyEvent) bool) *Hook {
	return &Hook{events: events, filter: filter, done: make(chan struct{}),
		down: map[uint32]bool{}, swallowed: map[uint32]bool{}, lockDown: map[uint32]bool{}, ownHidden: map[uint32]bool{}, ownShift: map[uint32]bool{}}
}

func (h *Hook) KeysDown() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.down)
}

func (h *Hook) Locks() LockState {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.locks
}

func StartHook(events chan<- KeyEvent) (*Hook, error) {
	h := newHook(events, nil)
	ready := make(chan error, 1)
	go h.run(ready)
	return h, <-ready
}

func StartFilterHook(filter func(KeyEvent) bool) (*Hook, error) {
	h := newHook(nil, filter)
	ready := make(chan error, 1)
	go h.run(ready)
	return h, <-ready
}

func (h *Hook) Block(on bool) { h.block.Store(on) }

func (h *Hook) BlockOwn(on bool) { h.blockOwn.Store(on) }

func (h *Hook) hideOwn(ev KeyEvent) bool {
	id := ev.Scan | uint32(ev.Flags&1)<<16
	h.mu.Lock()
	defer h.mu.Unlock()
	if ev.Up {
		hidden := h.ownHidden[id]
		delete(h.ownHidden, id)
		return hidden
	}
	if h.ownHidden[id] {
		return true
	}
	if h.blockOwn.Load() {
		h.ownHidden[id] = true
		return true
	}
	return false
}

func (h *Hook) inferNumLock(ev KeyEvent) {
	var on bool
	switch {
	case ev.KeypadDigit():
		on = true
	case ev.KeypadNavigation():
		on = false
	default:
		return
	}
	h.mu.Lock()
	changed := h.locks.Num != on
	h.locks.Num = on
	state, fn := h.locks, h.onLocks
	h.mu.Unlock()
	if changed && fn != nil {
		fn(state)
	}
}

func (h *Hook) handle(ev KeyEvent) bool {
	if ev.Own() && ev.VK == vkNoName && h.reseed.Swap(false) {
		h.seedLocks()
	}
	h.mu.Lock()
	if ev.Own() && (ev.Scan == 0x2A || ev.Scan == 0x36) {
		h.ownShift[ev.Scan] = !ev.Up
	}
	ev.ShiftHeld = h.down[0x2A] || h.down[0x36] || h.ownShift[0x2A] || h.ownShift[0x36]
	h.mu.Unlock()
	if !ev.Up {
		h.inferNumLock(ev)
	}
	ev.Locks = h.Locks()
	if h.events != nil {
		select {
		case h.events <- ev:
		default:
		}
	}
	hidden := h.swallow(ev)
	if !hidden {
		h.trackLocks(ev)
	}
	return hidden
}

func (h *Hook) swallow(ev KeyEvent) bool {
	if ev.Own() {
		return h.hideOwn(ev)
	}
	id := ev.Scan | uint32(ev.Flags&1)<<16
	h.mu.Lock()
	defer h.mu.Unlock()
	switch {
	case ev.Up:
		delete(h.down, id)
		hidden := h.swallowed[id]
		delete(h.swallowed, id)
		h.callFilter(ev)
		return hidden
	case h.down[id]:
		ev.Repeat = true
		h.callFilter(ev)
		return h.swallowed[id]
	}
	h.down[id] = true
	hide := h.block.Load()
	if h.filter != nil {
		hide = h.callFilter(ev)
	}
	if hide {
		h.swallowed[id] = true
	}
	return hide
}

func (h *Hook) callFilter(ev KeyEvent) bool {
	if h.filter == nil {
		return false
	}
	h.mu.Unlock()
	defer h.mu.Lock()
	return h.filter(ev)
}

func (h *Hook) trackLocks(ev KeyEvent) {
	h.mu.Lock()
	var bit *bool
	switch ev.VK {
	case vkNumLock:
		bit = &h.locks.Num
	case vkCapital:
		bit = &h.locks.Caps
	case vkScroll:
		bit = &h.locks.Scroll
	default:
		h.mu.Unlock()
		return
	}
	if ev.Up {
		delete(h.lockDown, ev.VK)
		h.mu.Unlock()
		return
	}
	if h.lockDown[ev.VK] {
		h.mu.Unlock()
		return
	}
	h.lockDown[ev.VK] = true
	*bit = !*bit
	state, fn := h.locks, h.onLocks
	h.mu.Unlock()
	if fn != nil {
		fn(state)
	}
}

func (h *Hook) seedLocks() {
	toggled := func(vk uintptr) bool {
		r, _, _ := getKeyState.Call(vk)
		return r&1 != 0
	}
	h.mu.Lock()
	h.locks = LockState{Num: toggled(vkNumLock), Caps: toggled(vkCapital), Scroll: toggled(vkScroll)}
	state, fn := h.locks, h.onLocks
	h.mu.Unlock()
	if fn != nil {
		fn(state)
	}
}

func (h *Hook) Reseed() {
	h.reseed.Store(true)
	if h.Probe(500 * time.Millisecond) {
		for deadline := time.Now().Add(200 * time.Millisecond); h.reseed.Load() && time.Now().Before(deadline); {
			time.Sleep(time.Millisecond)
		}
		return
	}
	postThreadMessage.Call(uintptr(h.tid), wmReseed, 0, 0)
}

func (h *Hook) Probe(timeout time.Duration) bool {
	before := h.Calls.Load()
	keybdEvent.Call(vkNoName, 0, 2 /*KEYEVENTF_KEYUP*/, InjectTag)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if h.Calls.Load() > before {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

func InjectKey(vk byte, up bool) {
	flags := uintptr(0)
	if up {
		flags = 2
	}
	keybdEvent.Call(uintptr(vk), 0, flags, InjectTag)
}

func ShowWindow(title string) {
	ready := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		class, _ := windows.UTF16PtrFromString("zboard-window")
		wndProc := windows.NewCallback(func(hwnd, msg, wParam, lParam uintptr) uintptr {
			r, _, _ := defWindowProc.Call(hwnd, msg, wParam, lParam)
			return r
		})
		module, _, _ := getModuleHandle.Call(0)
		wc := struct {
			Size, Style                        uint32
			WndProc                            uintptr
			ClsExtra, WndExtra                 int32
			Instance, Icon, Cursor, Background uintptr
			MenuName, ClassName                *uint16
			IconSm                             uintptr
		}{WndProc: wndProc, Instance: module, ClassName: class}
		wc.Size = uint32(unsafe.Sizeof(wc))
		registerClassEx.Call(uintptr(unsafe.Pointer(&wc)))
		t16, _ := windows.UTF16PtrFromString(title)
		const wsOverlappedWindow, wsVisible = 0x00CF0000, 0x10000000
		hwnd, _, _ := createWindowEx.Call(0, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(t16)), wsOverlappedWindow|wsVisible,
			100, 100, 300, 120, 0, 0, module, 0)
		setForegroundWindow.Call(hwnd)
		close(ready)
		var msg [48]byte
		for {
			r, _, _ := getMessage.Call(uintptr(unsafe.Pointer(&msg[0])), 0, 0, 0)
			if int32(r) <= 0 {
				return
			}
			translateMessage.Call(uintptr(unsafe.Pointer(&msg[0])))
			dispatchMessage.Call(uintptr(unsafe.Pointer(&msg[0])))
		}
	}()
	<-ready
}

func (h *Hook) Stop() {
	postThreadMessage.Call(uintptr(h.tid), wmQuit, 0, 0)
	<-h.done
}

func (h *Hook) run(ready chan<- error) {
	runtime.LockOSThread()
	defer close(h.done)
	h.tid = windows.GetCurrentThreadId()
	cb := windows.NewCallback(func(code int32, wParam uintptr, k *kbdllhookstruct) uintptr {
		h.Calls.Add(1)
		if code == 0 {
			ev := KeyEvent{VK: k.VK, Scan: k.Scan, Flags: k.Flags, Time: k.Time, Extra: k.ExtraInfo,
				Up: k.Flags&0x80 != 0, Extended: k.Flags&0x01 != 0, Injected: k.Flags&0x10 != 0}
			if h.handle(ev) {
				return 1
			}
		}
		r, _, _ := callNextHookEx.Call(0, uintptr(code), wParam, uintptr(unsafe.Pointer(k)))
		return r
	})
	module, _, _ := getModuleHandle.Call(0)
	hook, _, e := setWindowsHookEx.Call(whKeyboardLL, cb, module, 0)
	if hook == 0 {
		ready <- errors.New("SetWindowsHookEx: " + e.Error())
		return
	}
	h.seedLocks()
	ready <- nil
	var msg [48]byte
	for {
		r, _, _ := getMessage.Call(uintptr(unsafe.Pointer(&msg[0])), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		if *(*uint32)(unsafe.Pointer(&msg[8])) == wmReseed {
			h.reseed.Store(false)
			h.seedLocks()
			continue
		}
		translateMessage.Call(uintptr(unsafe.Pointer(&msg[0])))
		dispatchMessage.Call(uintptr(unsafe.Pointer(&msg[0])))
	}
	unhookWindowsHook.Call(hook)
}
