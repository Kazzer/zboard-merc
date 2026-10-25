//go:build windows

package platwin

import (
	"testing"
	"time"
)

func TestTypematic(t *testing.T) {
	delay, interval := Typematic()
	if delay < 250*time.Millisecond || delay > time.Second {
		t.Errorf("delay %v", delay)
	}
	if interval < 33*time.Millisecond || interval > 400*time.Millisecond {
		t.Errorf("interval %v", interval)
	}
}
