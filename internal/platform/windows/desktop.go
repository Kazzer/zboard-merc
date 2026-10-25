//go:build windows

package platwin

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	openInputDesktop     = user32.NewProc("OpenInputDesktop")
	closeDesktop         = user32.NewProc("CloseDesktop")
	getUserObjectInfoW   = user32.NewProc("GetUserObjectInformationW")
	defaultDesktopName16 = windows.StringToUTF16("Default")
)

func SecureDesktop() bool {
	const genericRead = 0x80000000
	h, _, _ := openInputDesktop.Call(0, 0, genericRead)
	if h == 0 {
		return true
	}
	defer closeDesktop.Call(h)
	const uoiName = 2
	var name [64]uint16
	var got uint32
	r, _, _ := getUserObjectInfoW.Call(h, uoiName, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)*2), uintptr(unsafe.Pointer(&got)))
	if r == 0 {
		return false
	}
	return windows.UTF16ToString(name[:]) != windows.UTF16ToString(defaultDesktopName16)
}
