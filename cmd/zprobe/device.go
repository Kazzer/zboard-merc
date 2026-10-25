//go:build linux || windows

package main

import (
	"github.com/Kazzer/zboard-merc/internal/hidrd"
	"github.com/Kazzer/zboard-merc/internal/keys"
)

type collectionInfo struct {
	Name                            string
	Product                         string
	VendorID, ProductID, Version    uint16
	UsagePage, Usage                uint16
	InputLen, OutputLen, FeatureLen int
	Readable                        bool
}

type collection interface {
	Info() collectionInfo
	Descriptor() []byte
	Details() map[string]any
	Read() ([]byte, error)
	Usages(report []byte) []hidrd.Usage
	GetFeature(id byte) ([]byte, error)
	GetInput(id byte) ([]byte, error)
	SetFeature(report []byte) error
	SetOutput(report []byte) error
	Close()
}

type keyEvent struct {
	key    keys.Key
	known  bool
	down   bool
	repeat bool
	own    bool
	record map[string]any
}

type keySource interface {
	Block(on bool)
	BlockOwn(on bool)
	KeysDown() int
	Locks() (num, caps, scroll bool)
	Stop()
}

type sink interface {
	key(ev keyEvent)
	event(record map[string]any)
	note(record map[string]any)
}

func commandChannel(colls []collection) collection {
	for _, c := range colls {
		if c.Info().FeatureLen == 3 {
			return c
		}
	}
	return nil
}

func usageStrings(usages []hidrd.Usage) []string {
	out := make([]string, 0, len(usages))
	for _, u := range usages {
		out = append(out, u.String())
	}
	return out
}

func status(err error) string {
	if err == nil {
		return "ok"
	}
	return err.Error()
}
