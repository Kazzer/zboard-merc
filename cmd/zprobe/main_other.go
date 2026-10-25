//go:build !linux && !windows

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "zprobe runs on Linux and Windows")
	os.Exit(2)
}
