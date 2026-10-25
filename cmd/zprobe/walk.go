//go:build linux || windows

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Kazzer/zboard-merc/internal/merc"
)

type state struct {
	Done []string `json:"done"`
}

func (s *state) has(id string) bool {
	for _, d := range s.Done {
		if d == id {
			return true
		}
	}
	return false
}

func (s *state) redo(spec string) []string {
	if spec == "" {
		return nil
	}
	var dropped []string
	if n, err := strconv.Atoi(spec); err == nil {
		if n > len(s.Done) {
			n = len(s.Done)
		}
		dropped = append(dropped, s.Done[len(s.Done)-n:]...)
		s.Done = s.Done[:len(s.Done)-n]
		return dropped
	}
	want := map[string]bool{}
	for _, id := range strings.Split(spec, ",") {
		want[strings.TrimSpace(id)] = true
	}
	kept := s.Done[:0]
	for _, d := range s.Done {
		if want[d] {
			dropped = append(dropped, d)
		} else {
			kept = append(kept, d)
		}
	}
	s.Done = kept
	return dropped
}

func (s *state) save(out string) {
	b, _ := json.Marshal(s)
	os.WriteFile(filepath.Join(out, "state.json"), b, 0o644)
}

func keyWalk(r *recorder, out string, st *state, prefix string, main bool) {
	fmt.Println("\nKEY WALK. For each prompt, press the key once. Keys are blocked while a prompt is open, so they do")
	fmt.Printf("not reach the desktop. A prompt starts once every key is released and ends one second after the\n"+
		"last event, or after %s with no event. Ctrl+C stops; rerun with -resume to continue, with\n"+
		"-redo N (or -redo id,id) to repeat the last N prompts (or those ids).\n", firstEventTimeout)
	for _, p := range prompts(prefix, main) {
		if st.has(p.id) {
			continue
		}
		fmt.Printf("[%s] %s\n", p.id, p.text)
		r.settle()
		r.mark(map[string]any{"prompt": p.id, "text": p.text})
		n := r.waitForActivity(!strings.Contains(p.id, "numlock-"))
		r.mark(map[string]any{"prompt_end": p.id, "events": n})
		if n == 0 {
			fmt.Println("        no events")
		} else {
			fmt.Printf("        %d events\n", n)
		}
		if p.legend {
			r.mark(map[string]any{"legend": p.id, "text": ask("        legend(s) printed on that key (Enter to skip): ")})
		}
		if strings.Contains(p.id, "numlock-") {
			r.mark(map[string]any{"led_state": r.lockState()})
		}
		st.Done = append(st.Done, p.id)
		st.save(out)
	}
}

func ledTrials(colls []collection, out string) {
	fd := commandChannel(colls)
	if fd == nil {
		fmt.Println("\nno collection with the three-byte feature report; skipping the LED trials")
		return
	}
	fmt.Println("\nLED TRIALS. Each step sends one value of feature report 2; answer with the LED you see:")
	fmt.Println("  e=Enabled  a=Action  n/c/s=Num/Caps/Scroll  o=other  x=nothing")
	if strings.ToLower(ask("Run the LED trials? [y/N] ")) != "y" {
		return
	}
	var results []map[string]any
	for _, step := range []struct {
		enabled, action bool
		expect          string
	}{{true, false, "Enabled"}, {false, true, "Action"}, {true, true, "Enabled and Action"}} {
		fd.SetFeature(merc.LEDReport(false, false))
		report := merc.LEDReport(step.enabled, step.action)
		err := fd.SetFeature(report)
		seen := status(err)
		if err == nil {
			seen = ask(fmt.Sprintf("  feature [% X] (expected %s): which LED is lit? ", report, step.expect))
		}
		results = append(results, map[string]any{"method": "feature2", "node": fd.Info().Name, "payload": report, "expected": step.expect, "status": status(err), "seen": seen})
	}
	fd.SetFeature(merc.LEDReport(false, false))
	b, _ := json.MarshalIndent(results, "", "  ")
	os.WriteFile(filepath.Join(out, "led-trials.json"), b, 0o644)
}

func tableWalk(r *recorder, colls []collection, out string, st *state) {
	fd := commandChannel(colls)
	if fd == nil {
		return
	}
	fmt.Println("\nSTAND-IN TABLE. Feature report 2 selects the firmware's key matrix: table 1 is the power-on legend")
	fmt.Println("table, table 2 the stand-in table ZEngine's driver selected at start-up, where the pad sends usages of")
	fmt.Println("its own. The same report brings table 1 back; so does a replug.")
	if strings.ToLower(ask("Switch to table 2 and walk the pad, numpad, top row and chords again? [y/N] ")) != "y" {
		return
	}
	sendTable(r, fd, merc.StandIn)
	keyWalk(r, out, st, "m2-", false)
	sendTable(r, fd, merc.Legend)
	id, text := "m1-pad-01", "pad key #1 after switching back to table 1"
	fmt.Printf("[%s] %s\n", id, text)
	r.settle()
	r.mark(map[string]any{"prompt": id, "text": text})
	n := r.waitForActivity(true)
	r.mark(map[string]any{"prompt_end": id, "events": n})
	fmt.Printf("        %d events\n", n)
}

func sendTable(r *recorder, fd collection, t merc.Table) {
	report := merc.TableReport(t)
	err := fd.SetFeature(report)
	r.mark(map[string]any{"matrix_table": t, "feature": fmt.Sprintf("% X", report), "node": fd.Info().Name, "status": status(err)})
	fmt.Printf("  sent % X (table %d): %s\n", report, t, status(err))
}
