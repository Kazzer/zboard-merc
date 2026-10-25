//go:build linux || windows

package main

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/Kazzer/zboard-merc/internal/hidrd"
	"github.com/Kazzer/zboard-merc/internal/keys"
	"github.com/Kazzer/zboard-merc/internal/merc"
)

func keyName(u hidrd.Usage) string {
	if k, ok := keys.ByUsage(u); ok {
		return k.Name
	}
	return fmt.Sprintf("%v", u)
}

func joinNames(names []string, empty string) string {
	if len(names) == 0 {
		return empty
	}
	return strings.Join(names, " ")
}

func agentRunning(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, 300*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

func expectRun(r *recorder, control string) {
	agent := agentRunning(control)
	fmt.Println("\nEXPECTED VERSUS ACTUAL. Keys do not reach the desktop while this runs, except during the Num")
	fmt.Println("Lock prompts, so Ctrl+C from the numpad cannot stop it either: to abort, close this window.")
	if agent {
		fmt.Printf("zengine answers on %s: what it types is shown beside the keyboard's keys and kept from the desktop.\n", control)
	} else {
		fmt.Printf("zengine not found on %s (-control sets the address): keys are hidden here.\n", control)
	}
	guard := func(on bool) {
		if agent {
			r.keys.BlockOwn(on)
		} else {
			r.keys.Block(on)
		}
	}
	guard(true)
	defer guard(false)
	type check struct {
		id, text, numLock string
		want              []string
	}
	var checks []check
	for _, state := range []string{"off", "on"} {
		checks = append(checks, check{"ex-numlock-" + state, "press Num Lock until its LED is " + strings.ToUpper(state), state, nil})
		n := 0
		for _, k := range merc.Keys {
			if k.Area != merc.Numpad {
				continue
			}
			want := []string{keyName(k.Legends)}
			if state == "off" {
				for _, sc := range merc.Shortcuts {
					if sc.Legend == k.Legend {
						want = nil
						if sc.Modifier != 0 {
							want = append(want, keyName(sc.Modifier))
						}
						want = append(want, keyName(sc.Key))
					}
				}
			}
			checks = append(checks, check{fmt.Sprintf("ex-np-%s-%02d", state, n), fmt.Sprintf("numpad (Num Lock %s): press [%s]", state, k.Legend), state, want})
			n++
		}
	}
	checks = append(checks, check{"ex-restore", "press the key labelled Restore (Scroll Lock; Num Lock does not matter)", "", []string{"scroll-lock"}},
		check{"ex-maximize", "press the key labelled Maximize (Pause; Num Lock does not matter)", "", []string{"pause"}})
	total, bad := 0, 0
	for _, c := range checks {
		fmt.Printf("[%s] %s\n", c.id, c.text)
		r.settle()
		start := r.keyCount()
		r.mark(map[string]any{"prompt": c.id, "text": c.text})
		if c.want == nil {
			guard(false)
		}
		n := r.waitForActivity(false)
		if c.want == nil {
			guard(true)
		}
		r.mark(map[string]any{"prompt_end": c.id, "events": n})
		if c.want == nil {
			continue
		}
		total++
		keyboard, agent := r.downsSince(start)
		ok := joinNames(keyboard, "") == joinNames(c.want, "")
		verdict := "OK"
		if !ok {
			verdict = "MISMATCH"
			bad++
		}
		fmt.Printf("        keyboard: %-20s agent: %-20s expected: %-20s %s\n", joinNames(keyboard, "(nothing)"), joinNames(agent, "-"), joinNames(c.want, ""), verdict)
		keypad := len(keyboard) == 1 && strings.HasPrefix(keyboard[0], "kp-")
		num, _, _ := r.keys.Locks()
		desktop := map[bool]string{true: "on", false: "off"}[num]
		switch {
		case ok:
		case len(keyboard) == 0 && len(agent) > 0:
			fmt.Println("        (nothing seen from the keyboard, only what zengine typed: on Windows, start zengine before zprobe)")
		case c.numLock == "off" && keypad:
			fmt.Printf("        (the keyboard is in keypad mode, so its Num Lock LED is on; the desktop has Num Lock %s)\n", desktop)
		case c.numLock == "on" && !keypad:
			fmt.Printf("        (the keyboard is in shortcut mode, so its Num Lock LED is off; the desktop has Num Lock %s)\n", desktop)
		}
		r.mark(map[string]any{"expect": c.id, "keyboard": keyboard, "agent": agent, "want": c.want, "ok": ok})
	}
	fmt.Printf("\n%d of %d keys as expected\n", total-bad, total)
}
