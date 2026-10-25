//go:build windows

package platwin

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func press(vk, scan uint32) (down, up KeyEvent) {
	return KeyEvent{VK: vk, Scan: scan}, KeyEvent{VK: vk, Scan: scan, Flags: 0x80, Up: true}
}

func TestSwallowHidesKeysAsAWhole(t *testing.T) {
	h := newHook(nil, nil)
	enterDown, enterUp := press(0x0D, 0x1C)
	wDown, wUp := press(0x57, 0x11)

	if h.swallow(enterDown) {
		t.Fatal("unblocked press swallowed")
	}
	h.Block(true)
	if h.swallow(enterUp) {
		t.Fatal("release of a visible press swallowed")
	}
	if !h.swallow(wDown) {
		t.Fatal("blocked press not swallowed")
	}
	if h.KeysDown() != 1 {
		t.Fatalf("keys down %d, want 1", h.KeysDown())
	}
	h.Block(false)
	if !h.swallow(wUp) {
		t.Fatal("release of a hidden press not swallowed")
	}
	if h.KeysDown() != 0 {
		t.Fatalf("keys down %d, want 0", h.KeysDown())
	}
	h.Block(true)
	if h.swallow(KeyEvent{VK: 0x41, Scan: 0x1E, Flags: 0x10, Injected: true, Extra: InjectTag}) {
		t.Fatal("own injected event swallowed")
	}
	calc := KeyEvent{VK: 0xB7, Scan: 0, Flags: 0x11, Injected: true, Extended: true}
	if !h.swallow(calc) {
		t.Fatal("HID-service launch key not swallowed while blocking")
	}
	calc.Up, calc.Flags = true, 0x91
	if !h.swallow(calc) {
		t.Fatal("release of a hidden launch key not swallowed")
	}
	kpDown := KeyEvent{VK: 0x0D, Scan: 0x1C, Flags: 1, Extended: true}
	h.swallow(kpDown)
	if h.KeysDown() != 1 {
		t.Fatalf("keys down %d, want 1 after keypad Enter", h.KeysDown())
	}
}

func TestRepeatFollowsItsPress(t *testing.T) {
	h := newHook(nil, nil)
	enterDown, enterUp := press(0x0D, 0x1C)
	wDown, wUp := press(0x57, 0x11)
	h.swallow(enterDown)
	h.Block(true)
	if h.swallow(enterDown) {
		t.Fatal("repeat of a visible press swallowed")
	}
	if h.swallow(enterUp) {
		t.Fatal("release of a visible press swallowed")
	}
	h.swallow(wDown)
	h.Block(false)
	if !h.swallow(wDown) {
		t.Fatal("repeat of a hidden press let through")
	}
	if !h.swallow(wUp) {
		t.Fatal("release of a hidden press let through")
	}
	if h.KeysDown() != 0 {
		t.Fatalf("keys down %d, want 0", h.KeysDown())
	}
}

func TestFilterSeesEveryTransition(t *testing.T) {
	var seen []string
	h := newHook(nil, func(ev KeyEvent) bool {
		seen = append(seen, fmt.Sprintf("%#x/%v/%v", ev.Scan, ev.Repeat, ev.Up))
		return ev.Scan == 0x11 && !ev.Repeat && !ev.Up
	})
	wDown, wUp := press(0x57, 0x11)
	enterDown, enterUp := press(0x0D, 0x1C)
	got := []bool{h.handle(wDown), h.handle(wDown), h.handle(wUp), h.handle(enterDown), h.handle(enterDown), h.handle(enterUp)}
	if want := []bool{true, true, true, false, false, false}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("hidden %v, want %v", got, want)
	}
	want := "0x11/false/false 0x11/true/false 0x11/false/true 0x1c/false/false 0x1c/true/false 0x1c/false/true"
	if s := strings.Join(seen, " "); s != want {
		t.Errorf("filter saw %s\nwant       %s", s, want)
	}
	if h.KeysDown() != 0 {
		t.Fatalf("keys down %d, want 0", h.KeysDown())
	}
}

func TestLocksFollowVisiblePresses(t *testing.T) {
	h := newHook(nil, nil)
	capsDown, capsUp := press(vkCapital, 0x3A)
	for i, step := range []struct {
		ev   KeyEvent
		want bool
	}{
		{capsDown, true},
		{capsDown, true},
		{capsUp, true},
		{capsDown, false},
		{capsUp, false},
	} {
		h.handle(step.ev)
		if got := h.Locks().Caps; got != step.want {
			t.Fatalf("step %d: caps %v, want %v", i, got, step.want)
		}
	}
	h.Block(true)
	h.handle(capsDown)
	h.handle(capsUp)
	if h.Locks().Caps {
		t.Fatal("a hidden press toggled the lock")
	}
	own := KeyEvent{VK: vkCapital, Scan: 0x3A, Flags: 0x10, Injected: true, Extra: InjectTag}
	h.handle(own)
	if !h.Locks().Caps {
		t.Fatal("the agent's own press did not toggle the lock")
	}
	var before []bool
	h = newHook(nil, func(ev KeyEvent) bool { before = append(before, ev.Locks.Caps); return false })
	h.handle(capsDown)
	h.handle(capsUp)
	h.handle(capsDown)
	if fmt.Sprint(before) != "[false true true]" {
		t.Errorf("lock state seen by the filter: %v", before)
	}
}

func TestBlockOwnHidesTaggedKeys(t *testing.T) {
	h := newHook(nil, nil)
	own := KeyEvent{VK: 0x43, Scan: 0x2E, Flags: 0x10, Injected: true, Extra: InjectTag}
	ownUp := own
	ownUp.Up, ownUp.Flags = true, 0x90
	cDown, cUp := press(0x43, 0x2E)
	if h.handle(own) || h.handle(ownUp) {
		t.Fatal("tagged event hidden while BlockOwn is off")
	}
	h.BlockOwn(true)
	if !h.handle(own) {
		t.Fatal("tagged press not hidden")
	}
	if h.handle(cDown) {
		t.Fatal("physical press hidden by BlockOwn")
	}
	h.BlockOwn(false)
	if !h.handle(ownUp) {
		t.Fatal("release of a hidden tagged press let through")
	}
	if h.handle(cUp) {
		t.Fatal("physical release hidden")
	}
	if h.handle(own) {
		t.Fatal("tagged press hidden after BlockOwn went off")
	}
	if h.KeysDown() != 0 {
		t.Fatalf("keys down %d, want 0", h.KeysDown())
	}
}

func TestLockListener(t *testing.T) {
	h := newHook(nil, nil)
	var heard []LockState
	h.SetLockListener(func(s LockState) { heard = append(heard, s) })
	capsDown, capsUp := press(vkCapital, 0x3A)
	h.handle(capsDown)
	h.handle(capsDown)
	h.handle(capsUp)
	h.handle(capsDown)
	h.handle(capsUp)
	if fmt.Sprint(heard) != "[{false true false} {false false false}]" {
		t.Errorf("heard %v", heard)
	}
}

func TestNumLockInferredFromKeypad(t *testing.T) {
	h := newHook(nil, nil)
	var heard []LockState
	h.SetLockListener(func(s LockState) { heard = append(heard, s) })
	kp1Digit := KeyEvent{VK: 0x61, Scan: 0x4F}
	kp1End := KeyEvent{VK: 0x23, Scan: 0x4F}
	home := KeyEvent{VK: 0x24, Scan: 0x47, Flags: 1, Extended: true}
	h.handle(kp1Digit)
	if !h.Locks().Num || len(heard) != 1 {
		t.Fatalf("digit: num %v, heard %v", h.Locks().Num, heard)
	}
	h.handle(home)
	h.handle(kp1End)
	if h.Locks().Num || len(heard) != 2 {
		t.Fatalf("navigation: num %v, heard %v", h.Locks().Num, heard)
	}
	shiftDown, shiftUp := press(0xA0, 0x2A)
	h.handle(kp1Digit)
	h.handle(shiftDown)
	h.handle(kp1End)
	if !h.Locks().Num {
		t.Fatal("shifted navigation changed Num Lock")
	}
	h.handle(shiftUp)
	ownShiftDown := KeyEvent{VK: 0xA0, Scan: 0x2A, Flags: 0x10, Injected: true, Extra: InjectTag}
	ownShiftUp := ownShiftDown
	ownShiftUp.Up, ownShiftUp.Flags = true, 0x90
	h.handle(ownShiftDown)
	h.handle(kp1End)
	if !h.Locks().Num {
		t.Fatal("navigation under the agent's own Shift changed Num Lock")
	}
	h.handle(ownShiftUp)
	h.handle(kp1End)
	if h.Locks().Num {
		t.Fatal("navigation after the agent released Shift kept Num Lock")
	}
	h.handle(kp1Digit)
	var seen []bool
	h = newHook(nil, func(ev KeyEvent) bool { seen = append(seen, ev.KeypadNavigation()); return false })
	h.handle(kp1End)
	h.handle(shiftDown)
	h.handle(kp1End)
	h.handle(shiftUp)
	h.handle(kp1Digit)
	if fmt.Sprint(seen) != "[true false false false false]" {
		t.Errorf("keypad navigation seen: %v", seen)
	}
}

func TestReseed(t *testing.T) {
	h, err := StartFilterHook(func(KeyEvent) bool { return false })
	if err != nil {
		t.Skip(err)
	}
	defer h.Stop()
	heard := make(chan LockState, 1)
	h.SetLockListener(func(s LockState) {
		select {
		case heard <- s:
		default:
		}
	})
	h.Reseed()
	select {
	case <-heard:
	case <-time.After(2 * time.Second):
		t.Fatal("no lock state after Reseed")
	}
}
