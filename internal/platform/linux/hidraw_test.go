//go:build linux

package platlinux

import (
	"os"
	"path/filepath"
	"testing"
)

func fakeSysfs(t *testing.T) {
	root := t.TempDir()
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	usb := filepath.Join(root, "devices", "pci0000:00", "usb3", "3-1")
	for _, c := range []struct {
		iface, hid, hidraw, input, event, leds string
	}{
		{"1", "0003:1038:0210.0002", "hidraw1", "input3", "event3", "0"},
		{"0", "0003:1038:0210.0001", "hidraw0", "input2", "event2", "7"},
	} {
		ifaceDir := filepath.Join(usb, "3-1:1."+c.iface)
		write(filepath.Join(ifaceDir, "bInterfaceNumber"), "0"+c.iface+"\n")
		hidDir := filepath.Join(ifaceDir, c.hid)
		write(filepath.Join(hidDir, "hidraw", c.hidraw, "dev"), "243:0\n")
		write(filepath.Join(hidDir, "input", c.input, "capabilities", "led"), c.leds+"\n")
		write(filepath.Join(hidDir, "input", c.input, c.event, "dev"), "13:66\n")
		link := filepath.Join(root, "bus", "hid", "devices", c.hid)
		os.MkdirAll(filepath.Dir(link), 0o755)
		if err := os.Symlink(hidDir, link); err != nil {
			t.Fatal(err)
		}
	}
	sysfsRoot = root
	t.Cleanup(func() { sysfsRoot = "/sys" })
}

func TestFindHIDFollowsSymlinks(t *testing.T) {
	fakeSysfs(t)
	devs, err := FindHID(0x1038, 0x0210)
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 2 {
		t.Fatalf("%d devices", len(devs))
	}
	if devs[0].Interface != 0 || !devs[0].HasLEDs || devs[0].Hidraw != "/dev/hidraw0" || len(devs[0].Events) != 1 || devs[0].Events[0] != "/dev/input/event2" {
		t.Errorf("interface 0: %+v", devs[0])
	}
	if devs[1].Interface != 1 || devs[1].HasLEDs || devs[1].Hidraw != "/dev/hidraw1" {
		t.Errorf("interface 1: %+v", devs[1])
	}
	k := &Keyboard{HID: devs}
	if n := k.FeatureNode(); n != "/dev/hidraw1" {
		t.Errorf("feature node %s, want /dev/hidraw1", n)
	}
	for i := range devs {
		devs[i].Interface = -1
	}
	if n := k.FeatureNode(); n != "/dev/hidraw1" {
		t.Errorf("feature node by capability %s, want /dev/hidraw1", n)
	}
}
