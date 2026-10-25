//go:build linux

package platlinux

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/unix"
)

func hidiocSetFeature(n int) uintptr {
	return uintptr((3 << 30) | (n << 16) | ('H' << 8) | 0x06)
}

type HIDDevice struct {
	SysPath   string
	Interface int
	HasLEDs   bool
	Hidraw    string
	Events    []string
}

var sysfsRoot = "/sys"

func FindHID(vid, pid uint16) ([]HIDDevice, error) {
	paths, _ := filepath.Glob(fmt.Sprintf("%s/bus/hid/devices/0003:%04X:%04X.*", sysfsRoot, vid, pid))
	var devs []HIDDevice
	for _, link := range paths {
		p, err := filepath.EvalSymlinks(link)
		if err != nil {
			p = link
		}
		d := HIDDevice{SysPath: p, Interface: -1}
		if b, err := os.ReadFile(filepath.Join(filepath.Dir(p), "bInterfaceNumber")); err == nil {
			d.Interface, _ = strconv.Atoi(strings.TrimSpace(string(b)))
		}
		if raws, _ := filepath.Glob(filepath.Join(p, "hidraw", "hidraw*")); len(raws) > 0 {
			d.Hidraw = "/dev/" + filepath.Base(raws[0])
		}
		inputs, _ := filepath.Glob(filepath.Join(p, "input", "input*"))
		for _, in := range inputs {
			if b, err := os.ReadFile(filepath.Join(in, "capabilities", "led")); err == nil && strings.Trim(strings.TrimSpace(string(b)), "0") != "" {
				d.HasLEDs = true
			}
			evs, _ := filepath.Glob(filepath.Join(in, "event*"))
			for _, ev := range evs {
				d.Events = append(d.Events, "/dev/input/"+filepath.Base(ev))
			}
		}
		devs = append(devs, d)
	}
	sort.SliceStable(devs, func(i, j int) bool { return devs[i].Interface < devs[j].Interface })
	if len(devs) == 0 {
		return nil, fmt.Errorf("no HID device %04x:%04x under /sys/bus/hid/devices", vid, pid)
	}
	return devs, nil
}

func SetFeature(hidraw string, report []byte) error {
	fd, err := unix.Open(hidraw, unix.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("%s: %w", hidraw, err)
	}
	defer unix.Close(fd)
	buf := append([]byte(nil), report...)
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), hidiocSetFeature(len(buf)), uintptr(unsafe.Pointer(&buf[0]))); errno != 0 {
		return fmt.Errorf("%s: HIDIOCSFEATURE: %w", hidraw, errno)
	}
	return nil
}

func hidiocGetFeature(n int) uintptr {
	return uintptr((3 << 30) | (n << 16) | ('H' << 8) | 0x07)
}

func hidiocGetInput(n int) uintptr {
	return uintptr((3 << 30) | (n << 16) | ('H' << 8) | 0x0A)
}

func GetFeature(hidraw string, id byte, n int) ([]byte, error) {
	return getReport(hidraw, hidiocGetFeature, id, n)
}

func GetInputReport(hidraw string, id byte, n int) ([]byte, error) {
	return getReport(hidraw, hidiocGetInput, id, n)
}

func getReport(hidraw string, req func(int) uintptr, id byte, n int) ([]byte, error) {
	fd, err := unix.Open(hidraw, unix.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", hidraw, err)
	}
	defer unix.Close(fd)
	buf := make([]byte, n+1)
	buf[0] = id
	got, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), req(len(buf)), uintptr(unsafe.Pointer(&buf[0])))
	if errno != 0 {
		return nil, fmt.Errorf("%s: report %d: %w", hidraw, id, errno)
	}
	return buf[:got], nil
}

func WriteOutput(hidraw string, report []byte) error {
	fd, err := unix.Open(hidraw, unix.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("%s: %w", hidraw, err)
	}
	defer unix.Close(fd)
	if _, err := unix.Write(fd, report); err != nil {
		return fmt.Errorf("%s: write: %w", hidraw, err)
	}
	return nil
}

func ReportDescriptor(d HIDDevice) ([]byte, error) {
	return os.ReadFile(filepath.Join(d.SysPath, "report_descriptor"))
}
