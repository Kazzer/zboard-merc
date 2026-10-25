package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/Kazzer/zboard-merc/internal/engine"
	"github.com/Kazzer/zboard-merc/internal/keys"
)

type recorder struct {
	mu    sync.Mutex
	eng   *engine.Engine
	on    bool
	last  time.Time
	steps []recStep
}

type recStep struct {
	Tap     string `json:"tap,omitempty" yaml:"tap,omitempty"`
	Press   string `json:"press,omitempty" yaml:"press,omitempty"`
	Release string `json:"release,omitempty" yaml:"release,omitempty"`
	Wait    string `json:"wait,omitempty" yaml:"wait,omitempty"`
}

func newRecorder(eng *engine.Engine) *recorder { return &recorder{eng: eng} }

func (r *recorder) start() {
	r.mu.Lock()
	r.on, r.steps, r.last = true, []recStep{}, time.Time{}
	r.mu.Unlock()
	r.eng.SetRecording(true)
}

func (r *recorder) stop() []recStep {
	r.mu.Lock()
	r.on = false
	steps := r.steps
	r.mu.Unlock()
	r.eng.SetRecording(false)
	return steps
}

func (r *recorder) state() (bool, []recStep) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.on, append([]recStep{}, r.steps...)
}

func (r *recorder) observe(ev engine.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.on {
		return
	}
	k, ok := keys.ByUsage(ev.Usage)
	if !ok {
		return
	}
	at := ev.Time
	if at.IsZero() {
		at = time.Now()
	}
	if n := len(r.steps); n > 0 && !ev.Down && r.steps[n-1].Press == k.Name {
		r.steps[n-1] = recStep{Tap: k.Name}
		r.last = at
		return
	}
	if !r.last.IsZero() {
		if gap := at.Sub(r.last).Round(time.Millisecond); gap > 0 {
			r.steps = append(r.steps, recStep{Wait: fmt.Sprintf("%dms", gap.Milliseconds())})
		}
	}
	r.last = at
	if ev.Down {
		r.steps = append(r.steps, recStep{Press: k.Name})
	} else {
		r.steps = append(r.steps, recStep{Release: k.Name})
	}
}
