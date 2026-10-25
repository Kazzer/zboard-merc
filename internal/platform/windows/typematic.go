//go:build windows

package platwin

import (
	"time"
	"unsafe"
)

var systemParametersInfo = user32.NewProc("SystemParametersInfoW")

const (
	spiGetKeyboardSpeed = 0x000A
	spiGetKeyboardDelay = 0x0016
)

func Typematic() (delay, interval time.Duration) {
	var d, s uint32
	systemParametersInfo.Call(spiGetKeyboardDelay, 0, uintptr(unsafe.Pointer(&d)), 0)
	systemParametersInfo.Call(spiGetKeyboardSpeed, 0, uintptr(unsafe.Pointer(&s)), 0)
	if d > 3 {
		d = 3
	}
	if s > 31 {
		s = 31
	}
	delay = time.Duration(d+1) * 250 * time.Millisecond
	perSecond := 2.5 + float64(s)*27.5/31
	interval = time.Duration(float64(time.Second) / perSecond)
	return delay, interval
}
