package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Kazzer/zboard-merc/internal/engine"
	"github.com/Kazzer/zboard-merc/internal/keys"
)

func TestRecorder(t *testing.T) {
	eng := engine.New(nopOut{}, scheduler{})
	r := newRecorder(eng)
	key := func(name string, down bool, at time.Duration) engine.Event {
		k, _ := keys.ByName(name)
		return engine.Event{Usage: k.Usage, Down: down, Time: time.Unix(0, 0).Add(at)}
	}
	r.observe(key("a", true, 0))
	r.start()
	r.observe(key("a", true, 0))
	r.observe(key("a", false, 60*time.Millisecond))
	r.observe(key("left-shift", true, 200*time.Millisecond))
	r.observe(key("b", true, 250*time.Millisecond))
	r.observe(key("b", false, 300*time.Millisecond))
	r.observe(key("left-shift", false, 320*time.Millisecond))
	r.observe(engine.Event{Usage: 0x00FF0001, Down: true, Time: time.Unix(0, 0).Add(400 * time.Millisecond)})
	on, sofar := r.state()
	if !on || len(sofar) != 7 {
		t.Errorf("state: %v %d", on, len(sofar))
	}
	steps := r.stop()
	b, _ := json.Marshal(steps)
	want := `[{"tap":"a"},{"wait":"140ms"},{"press":"left-shift"},{"wait":"50ms"},{"tap":"b"},{"wait":"20ms"},{"release":"left-shift"}]`
	if string(b) != want {
		t.Errorf("steps %s\nwant  %s", b, want)
	}
	if on, _ := r.state(); on {
		t.Error("still recording after stop")
	}
	r.observe(key("c", true, time.Second))
	if _, after := r.state(); len(after) != len(steps) {
		t.Error("recorded after stop")
	}
	r.start()
	if _, fresh := r.state(); len(fresh) != 0 {
		t.Error("a new recording kept old steps")
	}
	r.stop()
}
