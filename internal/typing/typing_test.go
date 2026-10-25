package typing

import "testing"

func TestStrokes(t *testing.T) {
	s, err := Strokes("Hi, gg!\n")
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		name  string
		shift bool
	}{{"h", true}, {"i", false}, {"comma", false}, {"space", false}, {"g", false}, {"g", false}, {"1", true}, {"enter", false}}
	if len(s) != len(want) {
		t.Fatalf("%d strokes, want %d", len(s), len(want))
	}
	for i, w := range want {
		if s[i].Key.Name != w.name || s[i].Shift != w.shift {
			t.Errorf("%d: %s/%v, want %s/%v", i, s[i].Key.Name, s[i].Shift, w.name, w.shift)
		}
	}
	if _, err := Strokes("é"); err == nil {
		t.Error("expected an error for a character outside the layout")
	}
}
