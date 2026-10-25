//go:build linux || windows

package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	idleAfterEvent = time.Second
	settleQuiet    = 600 * time.Millisecond
	settleTimeout  = 15 * time.Second
)

var firstEventTimeout = 20 * time.Second

type recorder struct {
	mu        sync.Mutex
	f         *os.File
	count     int
	lastEvent time.Time
	colls     []collection
	keys      keySource
	seen      []keyEvent
}

func (r *recorder) mark(fields map[string]any) {
	fields["t"] = float64(time.Now().UnixNano()) / 1e9
	b, _ := json.Marshal(fields)
	r.mu.Lock()
	r.f.Write(append(b, '\n'))
	r.mu.Unlock()
}

func (r *recorder) note(record map[string]any) { r.mark(record) }

func (r *recorder) event(record map[string]any) {
	r.mark(record)
	r.mu.Lock()
	r.count++
	r.lastEvent = time.Now()
	r.mu.Unlock()
}

func (r *recorder) key(ev keyEvent) {
	if ev.record != nil {
		r.event(ev.record)
	}
	r.mu.Lock()
	r.seen = append(r.seen, ev)
	r.mu.Unlock()
}

func (r *recorder) snapshot() (int, time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.count, r.lastEvent
}

func (r *recorder) keyCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.seen)
}

func (r *recorder) downsSince(start int) (keyboard, agent []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, ev := range r.seen[start:] {
		if !ev.down || ev.repeat {
			continue
		}
		name := "?"
		if ev.known {
			name = ev.key.Name
		}
		list := &keyboard
		if ev.own {
			list = &agent
		}
		if n := len(*list); n == 0 || (*list)[n-1] != name {
			*list = append(*list, name)
		}
	}
	return keyboard, agent
}

func (r *recorder) lockState() map[string]bool {
	num, caps, scroll := r.keys.Locks()
	return map[string]bool{"numlock": num, "capslock": caps, "scrolllock": scroll}
}

func (r *recorder) start() error {
	ks, err := startKeys(r.colls, r)
	if err != nil {
		return err
	}
	r.keys = ks
	for _, c := range r.colls {
		info := c.Info()
		if !info.Readable || info.InputLen == 0 {
			continue
		}
		go func(c collection, name string) {
			for {
				rep, err := c.Read()
				if err != nil {
					r.mark(map[string]any{"src": name, "error": err.Error()})
					return
				}
				r.event(map[string]any{"src": name, "raw": hex.EncodeToString(rep), "usages": usageStrings(c.Usages(rep))})
			}
		}(c, info.Name)
	}
	return nil
}

func (r *recorder) settle() {
	deadline := time.Now().Add(settleTimeout)
	for time.Now().Before(deadline) {
		_, last := r.snapshot()
		if r.keys.KeysDown() == 0 && time.Since(last) > settleQuiet {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	r.mark(map[string]any{"settle_timeout": r.keys.KeysDown()})
	fmt.Printf("        (a key still looks held after %s; carrying on)\n", settleTimeout)
}

func (r *recorder) waitForActivity(block bool) int {
	start, _ := r.snapshot()
	if block {
		r.keys.Block(true)
		defer r.keys.Block(false)
	}
	deadline := time.Now().Add(firstEventTimeout)
	for {
		n, last := r.snapshot()
		if n == start {
			if time.Now().After(deadline) {
				return 0
			}
		} else if time.Since(last) > idleAfterEvent {
			return n - start
		}
		time.Sleep(20 * time.Millisecond)
	}
}

var stdin = bufio.NewScanner(os.Stdin)

func ask(text string) string {
	fmt.Print(text)
	if stdin.Scan() {
		return strings.TrimSpace(stdin.Text())
	}
	return ""
}
