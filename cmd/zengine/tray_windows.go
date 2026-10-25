//go:build windows

package main

import "github.com/Kazzer/zboard-merc/internal/tray"

func trayAvailable() bool { return true }

func trayIcon(enabled, action, attached bool) []byte { return tray.ICO(enabled, action, attached) }
