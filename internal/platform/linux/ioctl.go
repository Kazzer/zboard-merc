//go:build linux

package platlinux

import (
	"os"

	"golang.org/x/sys/unix"
)

func ioctlInt(f *os.File, req uint, value int) error {
	return unix.IoctlSetInt(int(f.Fd()), req, value)
}
