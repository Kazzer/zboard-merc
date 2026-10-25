//go:build !linux && !windows

package main

import (
	"errors"

	"github.com/Kazzer/zboard-merc/internal/profile"
)

func run(cfg Config, mods []*profile.Mod, stop <-chan struct{}, ready func(*session)) error {
	return errors.New("the agent runs on Linux and Windows")
}
