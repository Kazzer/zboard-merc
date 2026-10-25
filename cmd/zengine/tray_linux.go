//go:build linux

package main

import (
	"os"

	"github.com/godbus/dbus/v5"

	"github.com/Kazzer/zboard-merc/internal/tray"
)

func trayAvailable() bool {
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return false
	}
	_, err := dbus.SessionBus()
	return err == nil
}

func trayIcon(enabled, action, attached bool) []byte { return tray.PNG(enabled, action, attached) }
