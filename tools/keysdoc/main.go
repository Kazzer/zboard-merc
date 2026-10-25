package main

import (
	"bytes"
	_ "embed"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/Kazzer/zboard-merc/internal/hidrd"
	"github.com/Kazzer/zboard-merc/internal/keys"
	"github.com/Kazzer/zboard-merc/internal/merc"
	"github.com/Kazzer/zboard-merc/internal/profile"
)

//go:embed template.md
var markdown string

type page struct {
	Keys   []row
	Groups []group
}

type row struct {
	Source, Key, Table1, Table2, Unbound, Note string
}

type group struct {
	Title string
	Keys  []mark
}

type mark struct {
	Name, Mark string
}

func main() {
	out := flag.String("o", "docs/keys.md", "the file to write")
	flag.Parse()
	w := new(bytes.Buffer)
	err := template.Must(template.New("keys").Parse(markdown)).Execute(w, page{rows(), groups()})
	if err == nil {
		err = os.WriteFile(*out, w.Bytes(), 0o644)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "keysdoc:", err)
		os.Exit(1)
	}
}

func rows() []row {
	base, numLock := profile.StandInDefault()
	var rows []row
	for _, k := range merc.Keys {
		rows = append(rows, row{source(k), label(k), table1(k), name(k.StandIn), fallback(k, base, numLock), note(k)})
	}
	return rows
}

func groups() []group {
	var groups []group
	for _, g := range []struct {
		title string
		pick  func(keys.Key) bool
	}{
		{"Keyboard keys", func(k keys.Key) bool { return k.Usage.Page() == 0x07 && !strings.HasPrefix(k.Name, "kp-") }},
		{"Keypad keys", func(k keys.Key) bool { return strings.HasPrefix(k.Name, "kp-") }},
		{"Consumer (media and launch) keys", func(k keys.Key) bool { return k.Usage.Page() == 0x0C }},
	} {
		names := group{Title: g.title}
		for _, k := range keys.All {
			if g.pick(k) {
				names.Keys = append(names.Keys, mark{k.Name, marker(k)})
			}
		}
		groups = append(groups, names)
	}
	return groups
}

func marker(k keys.Key) string {
	switch {
	case merc.WindowsDropsInStandIn[k.Usage]:
		return "†"
	case !k.WindowsVisible():
		return "‡"
	}
	return ""
}

func source(k merc.Key) string {
	if _, ok := merc.NumLockOff(k.StandIn); ok {
		return name(k.Legends) + ", with Num Lock off " + code(k.Source())
	}
	if s := k.Source(); s != "" {
		return code(s)
	}
	return name(k.Legends)
}

func label(k merc.Key) string {
	switch k.Area {
	case merc.Pad:
		return "pad " + k.Legend
	case merc.Numpad:
		return "numpad " + k.Legend
	}
	return k.Legend
}

func keyName(u hidrd.Usage) string {
	if k, ok := keys.ByUsage(u); ok {
		return k.Name
	}
	return fmt.Sprintf("usage %s", u)
}

func code(s string) string { return "`" + s + "`" }

func name(u hidrd.Usage) string { return code(keyName(u)) }

func table1(k merc.Key) string {
	for _, q := range merc.Sequences {
		if q.Legend == k.Legend {
			var chords []string
			for _, c := range q.Chords {
				var names []string
				for _, u := range c {
					names = append(names, name(u))
				}
				chords = append(chords, strings.Join(names, "+"))
			}
			return fmt.Sprintf("Num Lock off: %s (firmware macro); on: %s", strings.Join(chords, ", "), name(k.Legends))
		}
	}
	if k.Area != merc.Numpad {
		return name(k.Legends)
	}
	for _, s := range merc.Shortcuts {
		if s.Legend == k.Legend {
			return fmt.Sprintf("Num Lock off: %s (%s); on: %s", chord(s.Modifier, s.Key), s.Name, name(k.Legends))
		}
	}
	return name(k.Legends)
}

func chord(mod, key hidrd.Usage) string {
	if mod == 0 {
		return name(key)
	}
	return name(mod) + "+" + name(key)
}

func binding(b profile.Binding) string {
	if b.Remap != nil {
		return code(b.Remap.Name)
	}
	if b.Macro != nil {
		var chords, open []string
		for _, st := range b.Macro.Steps {
			switch {
			case st.Wait != 0:
			case st.Press:
				open = append(open, code(st.Key.Name))
			case open != nil:
				chords = append(chords, strings.Join(open, "+"))
				open = nil
			}
		}
		return strings.Join(chords, ", ")
	}
	var parts []string
	for _, k := range b.Chord {
		parts = append(parts, code(k.Name))
	}
	return strings.Join(parts, "+")
}

func fallback(k merc.Key, base, numLock profile.Layer) string {
	if u, ok := merc.NumLockOff(k.StandIn); ok {
		return "Num Lock off: " + binding(base[u]) + "; on: passes through"
	}
	off, inBase := base[k.StandIn]
	on, inNumLock := numLock[k.StandIn]
	switch {
	case inBase && inNumLock:
		return fmt.Sprintf("Num Lock off: %s; on: %s", binding(off), binding(on))
	case inBase:
		return binding(off)
	case inNumLock:
		return "Num Lock off: passes through; on: " + binding(on)
	}
	return "passes through"
}

func note(k merc.Key) string {
	var notes []string
	if k.StandIn == k.Legends {
		notes = append(notes, "same usage in both tables")
		if k.Area == merc.Pad {
			notes = append(notes, "the board's only Print Screen key, so nothing to tell apart")
		}
	}
	if merc.WindowsDropsInStandIn[k.StandIn] {
		notes = append(notes, "needs the driver on Windows")
	}
	if k.StandIn.Page() == 0x0C && k.Legends.Page() != 0x0C {
		notes = append(notes, "consumer usage in table 2")
	}
	return strings.Join(notes, "; ")
}
