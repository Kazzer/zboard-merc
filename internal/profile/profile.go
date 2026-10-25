package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Kazzer/zboard-merc/internal/hidrd"
	"github.com/Kazzer/zboard-merc/internal/keys"
	"github.com/Kazzer/zboard-merc/internal/merc"
)

type Mod struct {
	Name     string
	Table    merc.Table
	Triggers []string
	Menu     bool
	LEDs     *LEDs
	Base     Layer
	Layers   map[string]Layer
}

type LEDKind uint8

const (
	LEDMod LEDKind = iota
	LEDOn
	LEDOff
	LEDFlag
)

type LEDSource struct {
	Kind LEDKind
	Flag string
}

type LEDs struct {
	Enabled, Action LEDSource
}

func ParseLEDSource(s string) (LEDSource, error) {
	v := strings.ToLower(strings.TrimSpace(s))
	switch {
	case v == "" || v == "mod":
		return LEDSource{Kind: LEDMod}, nil
	case v == "on":
		return LEDSource{Kind: LEDOn}, nil
	case v == "off":
		return LEDSource{Kind: LEDOff}, nil
	case strings.HasPrefix(v, "flag:") && len(v) > 5:
		return LEDSource{Kind: LEDFlag, Flag: v[5:]}, nil
	}
	return LEDSource{}, fmt.Errorf("LED source %q is not mod, on, off or flag:<name>", s)
}

func ParseLEDs(enabled, action string) (LEDs, error) {
	e, err := ParseLEDSource(enabled)
	if err != nil {
		return LEDs{}, fmt.Errorf("enabled: %w", err)
	}
	a, err := ParseLEDSource(action)
	if err != nil {
		return LEDs{}, fmt.Errorf("action: %w", err)
	}
	return LEDs{Enabled: e, Action: a}, nil
}

type Layer map[hidrd.Usage]Binding

type Binding struct {
	Source  string
	Toggle  string
	Remap   *keys.Key
	Chord   []keys.Key
	Macro   *Macro
	Chat    *Chat
	Launch  string
	Layer   string
	Mod     string
	Disable bool
	UI      bool
}

type MacroMode uint8

const (
	Once MacroMode = iota
	Repeat
	Toggle
)

type Macro struct {
	Mode  MacroMode
	Steps []Step
	Hold  time.Duration
}

var DefaultTapHold = 40 * time.Millisecond

type Step struct {
	Key   keys.Key
	Press bool
	Wait  time.Duration
}

type Chat struct {
	Open keys.Key
	Text string
	Send keys.Key
}

type file struct {
	Name     string   `yaml:"name"`
	Table    string   `yaml:"table"`
	Triggers []string `yaml:"triggers"`
	Menu     *bool    `yaml:"menu"`
	LEDs     *struct {
		Enabled string `yaml:"enabled"`
		Action  string `yaml:"action"`
	} `yaml:"leds"`
	Keys   map[string]yaml.Node            `yaml:"keys"`
	Layers map[string]map[string]yaml.Node `yaml:"layers"`
}

func Load(path string) (*Mod, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m, err := Parse(b)
	if err != nil {
		return nil, err
	}
	m.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return m, nil
}

func Parse(b []byte) (*Mod, error) {
	var f file
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	m := &Mod{Name: f.Name, Layers: map[string]Layer{}}
	switch strings.ToLower(f.Table) {
	case "", "legend":
		m.Table = merc.Legend
	case "standin", "stand-in":
		m.Table = merc.StandIn
	default:
		return nil, fmt.Errorf("profile: table %q is not legend or standin", f.Table)
	}
	for _, t := range f.Triggers {
		m.Triggers = append(m.Triggers, strings.ToLower(strings.TrimSpace(t)))
	}
	m.Menu = len(m.Triggers) == 0
	if f.Menu != nil {
		m.Menu = *f.Menu
	}
	if f.LEDs != nil {
		l, err := ParseLEDs(f.LEDs.Enabled, f.LEDs.Action)
		if err != nil {
			return nil, fmt.Errorf("profile: leds: %w", err)
		}
		m.LEDs = &l
	}
	var err error
	if m.Base, err = parseLayer(m, "keys", f.Keys); err != nil {
		return nil, err
	}
	for name, raw := range f.Layers {
		if m.Layers[name], err = parseLayer(m, "layers."+name, raw); err != nil {
			return nil, err
		}
	}
	return m, nil
}

func parseLayer(m *Mod, where string, raw map[string]yaml.Node) (Layer, error) {
	layer := Layer{}
	names := make([]string, 0, len(raw))
	for n := range raw {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		u, err := Source(name, m.Table)
		if err != nil {
			return nil, fmt.Errorf("profile %s, %s: %w", m.Name, where, err)
		}
		if _, dup := layer[u]; dup {
			return nil, fmt.Errorf("profile %s, %s: %q binds the same key as an earlier entry", m.Name, where, name)
		}
		b, err := parseBinding(raw[name])
		if err != nil {
			return nil, fmt.Errorf("profile %s, %s, %s: %w", m.Name, where, name, err)
		}
		b.Source = name
		layer[u] = b
	}
	return layer, nil
}

func Source(name string, table merc.Table) (hidrd.Usage, error) {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "backlight.color" {
		n = "backlight.colour"
	}
	for _, k := range merc.Keys {
		if s := k.Source(); s == "" || s != n {
			continue
		}
		if off, ok := merc.NumLockOff(k.StandIn); ok {
			if table != merc.StandIn {
				plain, _ := keys.ByUsage(k.Legends)
				return 0, fmt.Errorf("%s is only reachable in the stand-in table (the firmware runs its own macro in the legend table); bind %s instead", n, plain.Name)
			}
			return off, nil
		}
		if table == merc.StandIn {
			return k.StandIn, nil
		}
		return k.Legends, nil
	}
	if strings.HasPrefix(n, "pad.") || strings.HasPrefix(n, "num.") {
		return 0, fmt.Errorf("unknown board key %q", name)
	}
	return keyUsage(n)
}

func keyUsage(name string) (hidrd.Usage, error) {
	k, ok := keys.ByName(name)
	if !ok {
		return 0, fmt.Errorf("unknown key %q", name)
	}
	return k.Usage, nil
}

func parseBinding(n yaml.Node) (Binding, error) {
	var b Binding
	if n.Kind == yaml.ScalarNode {
		if strings.EqualFold(n.Value, "disable") {
			b.Disable = true
			return b, nil
		}
		if strings.EqualFold(n.Value, "ui") {
			b.UI = true
			return b, nil
		}
		k, err := target(n.Value)
		if err != nil {
			return b, err
		}
		b.Remap = &k
		return b, nil
	}
	var spec struct {
		Toggle string                             `yaml:"toggle"`
		Tap    string                             `yaml:"tap"`
		Chord  []string                           `yaml:"chord"`
		Macro  []map[string]string                `yaml:"macro"`
		Mode   string                             `yaml:"mode"`
		Hold   string                             `yaml:"hold"`
		Chat   *struct{ Open, Text, Send string } `yaml:"chat"`
		Launch string                             `yaml:"launch"`
		Layer  string                             `yaml:"layer"`
		Mod    string                             `yaml:"mod"`
	}
	if err := n.Decode(&spec); err != nil {
		return b, err
	}
	b.Toggle = strings.ToLower(strings.TrimSpace(spec.Toggle))
	set := 0
	if spec.Tap != "" {
		k, err := target(spec.Tap)
		if err != nil {
			return b, err
		}
		b.Remap = &k
		set++
	}
	if len(spec.Chord) > 0 {
		for _, name := range spec.Chord {
			k, err := target(name)
			if err != nil {
				return b, err
			}
			b.Chord = append(b.Chord, k)
		}
		set++
	}
	if len(spec.Macro) > 0 {
		mac := &Macro{}
		switch strings.ToLower(spec.Mode) {
		case "", "once":
			mac.Mode = Once
		case "repeat":
			mac.Mode = Repeat
		case "toggle":
			mac.Mode = Toggle
		default:
			return b, fmt.Errorf("macro mode %q is not once, repeat or toggle", spec.Mode)
		}
		mac.Hold = DefaultTapHold
		if spec.Hold != "" {
			d, err := time.ParseDuration(spec.Hold)
			if err != nil || d < 0 {
				return b, fmt.Errorf("macro hold %q is not a duration", spec.Hold)
			}
			mac.Hold = d
		}
		for i, raw := range spec.Macro {
			step, err := parseStep(raw)
			if err != nil {
				return b, fmt.Errorf("macro step %d: %w", i+1, err)
			}
			mac.Steps = append(mac.Steps, step...)
		}
		mac.Steps = withHold(mac.Steps, mac.Hold)
		b.Macro = mac
		set++
	}
	if spec.Chat != nil {
		open, err := target(spec.Chat.Open)
		if err != nil {
			return b, fmt.Errorf("chat open: %w", err)
		}
		send, err := target(spec.Chat.Send)
		if err != nil {
			return b, fmt.Errorf("chat send: %w", err)
		}
		b.Chat = &Chat{Open: open, Text: spec.Chat.Text, Send: send}
		set++
	}
	if spec.Launch != "" {
		b.Launch = spec.Launch
		set++
	}
	if spec.Layer != "" {
		b.Layer = spec.Layer
		set++
	}
	if spec.Mod != "" {
		b.Mod = spec.Mod
		set++
	}
	if set > 1 || set == 0 && b.Toggle == "" {
		return b, fmt.Errorf("a binding needs exactly one of tap, chord, macro, chat, launch, layer or mod, or a toggle")
	}
	return b, nil
}

func withHold(steps []Step, hold time.Duration) []Step {
	if hold <= 0 {
		return steps
	}
	var out []Step
	for i, s := range steps {
		out = append(out, s)
		if s.Press && s.Wait == 0 && i+1 < len(steps) && !steps[i+1].Press && steps[i+1].Wait == 0 && steps[i+1].Key == s.Key {
			out = append(out, Step{Wait: hold})
		}
	}
	return out
}

func parseStep(raw map[string]string) ([]Step, error) {
	var hold time.Duration
	if h, ok := raw["hold"]; ok {
		d, err := time.ParseDuration(h)
		if err != nil || d < 0 {
			return nil, fmt.Errorf("hold %q is not a duration", h)
		}
		hold = d
		rest := make(map[string]string, len(raw))
		for k, v := range raw {
			if k != "hold" {
				rest[k] = v
			}
		}
		raw = rest
	}
	if len(raw) != 1 {
		return nil, fmt.Errorf("one of tap, press, release or wait, with hold only beside tap")
	}
	for kind, value := range raw {
		switch strings.ToLower(kind) {
		case "wait":
			d, err := time.ParseDuration(value)
			if err != nil {
				return nil, err
			}
			return []Step{{Wait: d}}, nil
		case "tap", "press", "release":
			k, err := target(value)
			if err != nil {
				return nil, err
			}
			switch strings.ToLower(kind) {
			case "tap":
				if hold > 0 {
					return []Step{{Key: k, Press: true}, {Wait: hold}, {Key: k}}, nil
				}
				return []Step{{Key: k, Press: true}, {Key: k}}, nil
			case "press":
				return []Step{{Key: k, Press: true}}, nil
			}
			return []Step{{Key: k}}, nil
		}
		return nil, fmt.Errorf("unknown step %q", kind)
	}
	return nil, nil
}

func target(name string) (keys.Key, error) {
	k, ok := keys.ByName(strings.TrimSpace(name))
	if !ok {
		return keys.Key{}, fmt.Errorf("unknown key %q", name)
	}
	return k, nil
}
