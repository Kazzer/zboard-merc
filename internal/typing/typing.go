package typing

import (
	"fmt"
	"strings"

	"github.com/Kazzer/zboard-merc/internal/keys"
)

type Stroke struct {
	Key   keys.Key
	Shift bool
}

var plain = map[rune]string{
	' ': "space", '\n': "enter", '\t': "tab", '-': "minus", '=': "equal", '[': "left-bracket", ']': "right-bracket",
	'\\': "backslash", ';': "semicolon", '\'': "apostrophe", '`': "grave", ',': "comma", '.': "period", '/': "slash",
}

var shifted = map[rune]string{
	'!': "1", '@': "2", '#': "3", '$': "4", '%': "5", '^': "6", '&': "7", '*': "8", '(': "9", ')': "0",
	'_': "minus", '+': "equal", '{': "left-bracket", '}': "right-bracket", '|': "backslash", ':': "semicolon",
	'"': "apostrophe", '~': "grave", '<': "comma", '>': "period", '?': "slash",
}

func Strokes(text string) ([]Stroke, error) {
	var out []Stroke
	var missing []string
	for _, r := range text {
		name, shift := "", false
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			name = string(r)
		case r >= 'A' && r <= 'Z':
			name, shift = strings.ToLower(string(r)), true
		default:
			if n, ok := plain[r]; ok {
				name = n
			} else if n, ok := shifted[r]; ok {
				name, shift = n, true
			}
		}
		k, ok := keys.ByName(name)
		if name == "" || !ok {
			missing = append(missing, string(r))
			continue
		}
		out = append(out, Stroke{Key: k, Shift: shift})
	}
	if len(missing) > 0 {
		return out, fmt.Errorf("typing: no key for %q on the US layout", strings.Join(missing, ""))
	}
	return out, nil
}
