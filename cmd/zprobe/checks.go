//go:build linux || windows

package main

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Kazzer/zboard-merc/internal/merc"
)

func setTable(colls []collection, t merc.Table) {
	fd := commandChannel(colls)
	if fd == nil {
		fmt.Println("no collection with the three-byte feature report")
		os.Exit(1)
	}
	report := merc.TableReport(t)
	if err := fd.SetFeature(report); err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
	fmt.Printf("sent % X: the keyboard is in table %d until -table %d, zengine, or a replug\n", report, t, 3-t)
}

func ledTest(colls []collection) {
	for _, step := range []struct {
		num, caps, scroll bool
		text              string
	}{{true, true, true, "all three on"}, {false, false, false, "all off"}, {true, false, false, "Num Lock on"}} {
		err := setLockLEDs(colls, step.num, step.caps, step.scroll)
		fmt.Printf("lock LEDs %s: %s\n", step.text, status(err))
		time.Sleep(time.Second)
	}
	fmt.Println("press Num Lock twice to put the LEDs back in step with the desktop")
}

func watchReports(colls []collection) {
	for _, c := range colls {
		info := c.Info()
		if !info.Readable || info.InputLen == 0 {
			continue
		}
		fmt.Println("watching", info.Name)
		go func(c collection, name string) {
			for {
				rep, err := c.Read()
				if err != nil {
					fmt.Printf("%s: %v\n", name, err)
					return
				}
				fmt.Printf("%s %s %s %s\n", time.Now().Format("15:04:05.000"), name, hex.EncodeToString(rep), strings.Join(usageStrings(c.Usages(rep)), " "))
			}
		}(c, info.Name)
	}
	fmt.Println("press Enter to stop")
	bufio.NewReader(os.Stdin).ReadString('\n')
}
