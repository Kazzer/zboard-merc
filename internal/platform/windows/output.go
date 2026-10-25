//go:build windows

package platwin

import (
	"fmt"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/Kazzer/zboard-merc/internal/keys"
)

var (
	sendInput     = user32.NewProc("SendInput")
	shellExecute  = windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteW")
	getForeground = user32.NewProc("GetForegroundWindow")
	getWindowTID  = user32.NewProc("GetWindowThreadProcessId")
)

const (
	keyeventfExtendedKey = 0x0001
	keyeventfKeyUp       = 0x0002
	keyeventfScanCode    = 0x0008
)

type keybdInput struct {
	Type      uint32
	_         uint32
	VK        uint16
	Scan      uint16
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
	_         [8]byte
}

func inputFor(k keys.Key, down bool) (keybdInput, error) {
	in := keybdInput{Type: 1, ExtraInfo: InjectTag}
	switch {
	case k.Scan != 0 && !k.ScanShared:
		in.Scan = k.Scan
		in.Flags = keyeventfScanCode
	case k.VK != 0:
		in.VK = uint16(k.VK)
		in.Scan = k.Scan
	default:
		return in, fmt.Errorf("%s has no Windows form", k.Name)
	}
	if k.Extended {
		in.Flags |= keyeventfExtendedKey
	}
	if !down {
		in.Flags |= keyeventfKeyUp
	}
	return in, nil
}

func SendKey(k keys.Key, down bool) error {
	in, err := inputFor(k, down)
	if err != nil {
		return err
	}
	if n, _, e := sendInput.Call(1, uintptr(unsafe.Pointer(&in)), unsafe.Sizeof(in)); n != 1 {
		return fmt.Errorf("SendInput: %w", e)
	}
	return nil
}

func Launch(target string) error {
	verb, _ := windows.UTF16PtrFromString("open")
	t, _ := windows.UTF16PtrFromString(target)
	r, _, _ := shellExecute.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(t)), 0, 0, 1 /*SW_SHOWNORMAL*/)
	if r <= 32 {
		return fmt.Errorf("ShellExecute %q: code %d", target, r)
	}
	return nil
}

func ForegroundProcess() string {
	hwnd, _, _ := getForeground.Call()
	if hwnd == 0 {
		return ""
	}
	var pid uint32
	getWindowTID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return ""
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	var buf [windows.MAX_LONG_PATH]uint16
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return ""
	}
	return strings.ToLower(filepath.Base(windows.UTF16ToString(buf[:n])))
}
