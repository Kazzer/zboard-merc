//go:build windows

package platwin

import (
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	getLastInputInfo = user32.NewProc("GetLastInputInfo")
	getTickCount     = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetTickCount")
)

func IdleFor() time.Duration {
	var info struct{ Size, Time uint32 }
	info.Size = uint32(unsafe.Sizeof(info))
	if r, _, _ := getLastInputInfo.Call(uintptr(unsafe.Pointer(&info))); r == 0 {
		return 0
	}
	now, _, _ := getTickCount.Call()
	return time.Duration(uint32(now)-info.Time) * time.Millisecond
}
