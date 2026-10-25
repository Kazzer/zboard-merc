//go:build windows

package platwin

import "testing"

func TestSecureDesktop(t *testing.T) {
	if SecureDesktop() {
		t.Error("the test's own desktop counts as secure")
	}
}
