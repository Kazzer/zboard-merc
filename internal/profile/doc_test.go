package profile

import (
	"strings"
	"testing"
)

const docSample = `# a comment the form loses
name: sample
table: standin
triggers: [game.exe, Other.exe]
leds: { enabled: mod, action: flag:streaming }
keys:
  pad.w: up
  pad.2: { chord: [left-ctrl, s] }
  pad.3: { macro: [ {tap: a}, {wait: 200ms} ], mode: repeat }
  hotkey.1: { toggle: sharing }
  pad.tab: disable
layers:
  alt:
    pad.1: f11
  num-lock:
    num.8: f12
`

func TestDocumentRoundTrip(t *testing.T) {
	d, err := DecodeDocument([]byte(docSample))
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "" || d.Table != "standin" || len(d.Triggers) != 2 || d.LEDs == nil || d.LEDs.Action != "flag:streaming" {
		t.Errorf("header: %+v", d)
	}
	var order []string
	for _, b := range d.Keys {
		order = append(order, b.Source)
	}
	if got := strings.Join(order, " "); got != "pad.w pad.2 pad.3 hotkey.1 pad.tab" {
		t.Errorf("key order lost: %s", got)
	}
	if d.Keys[0].Action != "up" || d.Keys[4].Action != "disable" {
		t.Errorf("scalar actions: %v %v", d.Keys[0].Action, d.Keys[4].Action)
	}
	if m, ok := d.Keys[2].Action.(map[string]any); !ok || m["mode"] != "repeat" {
		t.Errorf("macro action: %#v", d.Keys[2].Action)
	}
	if len(d.Layers) != 2 || d.Layers[0].Name != "alt" || d.Layers[1].Keys[0].Source != "num.8" {
		t.Errorf("layers: %+v", d.Layers)
	}
	out, err := d.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "pad.2:\n    chord:\n      - left-ctrl\n      - s\n") || !strings.Contains(string(out), "      - wait: 200ms\n    mode: repeat\n") {
		t.Errorf("encoded bindings are not on one line:\n%s", out)
	}
	m, err := Parse(out)
	if err != nil {
		t.Fatalf("encoded file does not parse: %v\n%s", err, out)
	}
	orig, _ := Parse([]byte(docSample))
	if len(m.Base) != len(orig.Base) || len(m.Layers) != len(orig.Layers) || m.Table != orig.Table || m.LEDs.Action != orig.LEDs.Action {
		t.Errorf("encoded Mod differs from the original")
	}
	again, err := DecodeDocument(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Keys) != len(d.Keys) || again.Keys[2].Source != "pad.3" {
		t.Errorf("second decode: %+v", again.Keys)
	}
}

func TestDocumentEmptyAndInvalid(t *testing.T) {
	d, err := DecodeDocument(nil)
	if err != nil || d.Name != "" || d.Keys == nil || d.Triggers == nil {
		t.Errorf("empty file: %+v %v", d, err)
	}
	if _, err := DecodeDocument([]byte("- a\n- b\n")); err == nil {
		t.Error("a list was accepted as a Mod")
	}
	d = &Document{Name: "x", Keys: []DocBinding{{Source: "pad.1", Action: map[string]any{"tap": "no-such-key"}}}}
	out, _ := d.Encode()
	if _, err := Parse(out); err == nil || !strings.Contains(err.Error(), "no-such-key") {
		t.Errorf("validation through Parse: %v", err)
	}
}
