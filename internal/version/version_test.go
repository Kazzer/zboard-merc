package version

import (
	"bytes"
	"os"
	"regexp"
	"testing"
	"unicode/utf16"
)

func TestFormat(t *testing.T) {
	if !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`).MatchString(String) {
		t.Fatalf("VERSION %q: want major.minor.patch", String)
	}
}

func TestResourcesCurrent(t *testing.T) {
	var want []byte
	for _, u := range utf16.Encode([]rune(String)) {
		want = append(want, byte(u), byte(u>>8))
	}
	checked := 0
	for _, cmd := range []string{"zengine", "zboard", "zprobe"} {
		for _, arch := range []string{"amd64", "arm64"} {
			object := "cmd/" + cmd + "/rsrc_windows_" + arch + ".syso"
			b, err := os.ReadFile("../../" + object)
			if err != nil {
				continue
			}
			checked++
			if !bytes.Contains(b, want) {
				t.Errorf("%s does not carry %s; run make", object, String)
			}
		}
	}
	if checked == 0 {
		t.Skip("no resources built")
	}
}
