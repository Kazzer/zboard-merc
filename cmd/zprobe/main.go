//go:build linux || windows

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Kazzer/zboard-merc/internal/merc"
	"github.com/Kazzer/zboard-merc/internal/version"
)

func main() {
	out := flag.String("out", "", "output directory (default <home>/capture-<model>)")
	pid := flag.String("pid", "", "USB product id when several Ideazon devices are attached, e.g. 0510")
	resume := flag.Bool("resume", false, "continue a walk that was interrupted")
	redo := flag.String("redo", "", "with -resume: a number of most recent prompts to repeat, or prompt ids separated by commas")
	timeout := flag.Duration("timeout", firstEventTimeout, "how long a prompt waits for its first event")
	skipWalk := flag.Bool("skip-walk", false, "skip the key walk")
	skipLeds := flag.Bool("skip-leds", false, "skip the LED trials")
	list := flag.Bool("list-prompts", false, "print the walk plan and exit")
	expect := flag.Bool("expect", false, "walk the numpad with Num Lock off and on and compare each key with what the legend table should send (zengine may run; nothing reaches the desktop)")
	control := flag.String("control", "127.0.0.1:47319", "zengine's control address, which -expect uses to tell whether it runs")
	table := flag.Int("table", 0, "select matrix table 1 or 2, leave the keyboard there, and exit")
	ledTestFlag := flag.Bool("led-test", false, "set the lock LEDs from user mode (all on, all off, Num Lock) and exit; stop zengine first")
	watch := flag.Bool("watch", false, "print every input report the readable collections send, with its usages, until Enter")
	showVersion := flag.Bool("version", false, "print the version and exit")
	platform := platformFlags()
	flag.Parse()
	firstEventTimeout = *timeout
	if *showVersion {
		fmt.Println("zprobe", version.String)
		return
	}
	if *list {
		for _, p := range prompts("", true) {
			fmt.Println(p.id, "|", p.text)
		}
		return
	}
	if platform != nil {
		if done, code := platform(); done {
			os.Exit(code)
		}
	}
	if *table != 0 && *table != 1 && *table != 2 {
		fmt.Fprintln(os.Stderr, "-table takes 1 or 2")
		os.Exit(2)
	}
	want := uint16(0)
	if *pid != "" {
		v, err := strconv.ParseUint(*pid, 16, 16)
		if err != nil {
			fmt.Fprintln(os.Stderr, "bad -pid:", err)
			os.Exit(2)
		}
		want = uint16(v)
	}
	colls, err := enumerate(want)
	if err != nil {
		fmt.Fprintln(os.Stderr, "enumerate:", err)
	}
	if *watch {
		watchReports(colls)
		return
	}
	if len(colls) == 0 {
		fmt.Fprintln(os.Stderr, "no Ideazon HID device found")
		os.Exit(1)
	}
	seen := map[uint16]bool{}
	for _, c := range colls {
		seen[c.Info().ProductID] = true
	}
	if len(seen) > 1 {
		fmt.Fprintln(os.Stderr, "several Ideazon devices attached; pass -pid")
		os.Exit(1)
	}
	first := colls[0].Info()
	model := models[first.ProductID]
	if model == "" {
		model = fmt.Sprintf("ideazon-%04x", first.ProductID)
	}
	if *out == "" {
		home, _ := os.UserHomeDir()
		*out = filepath.Join(home, "capture-"+strings.ToLower(strings.ReplaceAll(model, " ", "-")))
	}
	os.MkdirAll(*out, 0o755)
	st := &state{}
	if *resume {
		if b, err := os.ReadFile(filepath.Join(*out, "state.json")); err == nil {
			json.Unmarshal(b, st)
		}
		if dropped := st.redo(*redo); len(dropped) > 0 {
			fmt.Println("repeating:", strings.Join(dropped, " "))
		}
	}
	fmt.Printf("%s (%04x:%04x): %d HID collections\n", model, first.VendorID, first.ProductID, len(colls))
	if *table != 0 {
		setTable(colls, merc.Table(*table))
		return
	}
	if *ledTestFlag {
		ledTest(colls)
		return
	}
	staticCapture(colls, *out, model)
	f, err := os.OpenFile(filepath.Join(*out, "events.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	r := &recorder{f: f, colls: colls}
	if err := r.start(); err != nil {
		fmt.Fprintln(os.Stderr, "keys:", err)
		os.Exit(1)
	}
	r.mark(map[string]any{"session_start": float64(time.Now().Unix()), "redo": *redo, "led_state": r.lockState()})
	var once sync.Once
	finish := func(code int, tarball bool) {
		once.Do(func() {
			r.mark(map[string]any{"session_end": float64(time.Now().Unix())})
			r.keys.Stop()
			for _, c := range colls {
				c.Close()
			}
			f.Close()
			if !tarball {
				os.Exit(code)
			}
			path, err := writeTar(*out)
			if err != nil {
				fmt.Fprintln(os.Stderr, "tar:", err)
				os.Exit(1)
			}
			fmt.Printf("\nwrote %s; copy that file back\n", path)
			os.Exit(code)
		})
	}
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	go func() {
		<-interrupt
		fmt.Println("\ninterrupted; rerun with -resume to continue")
		finish(130, true)
	}()
	if *expect {
		expectRun(r, *control)
		finish(0, false)
	}
	if !*skipWalk {
		keyWalk(r, *out, st, "", true)
	}
	if !*skipLeds {
		ledTrials(colls, *out)
	}
	if !*skipWalk {
		tableWalk(r, colls, *out, st)
	}
	finish(0, true)
}
