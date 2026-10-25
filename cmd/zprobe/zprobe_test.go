//go:build linux || windows

package main

import (
	"strings"
	"testing"

	"github.com/Kazzer/zboard-merc/internal/keys"
)

func TestPromptsAreUniqueAndPrefixed(t *testing.T) {
	all := prompts("", true)
	if want := len(mainRef) + padKeys + 2*(1+len(numpad)) + len(topRow) + len(chords); len(all) != want {
		t.Fatalf("%d prompts, want %d", len(all), want)
	}
	seen := map[string]bool{}
	for _, p := range all {
		if seen[p.id] {
			t.Errorf("duplicate prompt id %s", p.id)
		}
		seen[p.id] = true
	}
	for _, p := range prompts("m2-", false) {
		if !strings.HasPrefix(p.id, "m2-") || strings.HasPrefix(p.id, "m2-main-") {
			t.Errorf("unexpected prompt %s in the table walk", p.id)
		}
	}
}

func TestRedo(t *testing.T) {
	s := &state{Done: []string{"a", "b", "c", "d"}}
	if got := s.redo("2"); strings.Join(got, " ") != "c d" || strings.Join(s.Done, " ") != "a b" {
		t.Fatalf("redo 2: dropped %v, left %v", got, s.Done)
	}
	s = &state{Done: []string{"a", "b", "c", "d"}}
	if got := s.redo("b, d"); strings.Join(got, " ") != "b d" || strings.Join(s.Done, " ") != "a c" {
		t.Fatalf("redo b,d: dropped %v, left %v", got, s.Done)
	}
	if got := s.redo("9"); len(got) != 2 || len(s.Done) != 0 {
		t.Fatalf("redo past the start: dropped %v, left %v", got, s.Done)
	}
}

func TestDownsSinceCollapsesRepeatsAndSplitsOwners(t *testing.T) {
	w, _ := keys.ByName("w")
	a, _ := keys.ByName("a")
	r := &recorder{seen: []keyEvent{
		{key: w, known: true, down: true},
		{key: w, known: true, down: true, repeat: true},
		{key: w, known: true, down: false},
		{known: false, down: true},
		{key: a, known: true, down: true, own: true},
	}}
	keyboard, agent := r.downsSince(0)
	if strings.Join(keyboard, " ") != "w ?" || strings.Join(agent, " ") != "a" {
		t.Fatalf("keyboard %v, agent %v", keyboard, agent)
	}
	if keyboard, _ := r.downsSince(3); strings.Join(keyboard, " ") != "?" {
		t.Fatalf("from 3: %v", keyboard)
	}
}
