package keys

import (
	"encoding/json"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/Kazzer/zboard-merc/internal/hidrd"
)

func TestTablesAreConsistent(t *testing.T) {
	seenName, seenUsage := map[string]bool{}, map[hidrd.Usage]bool{}
	for _, k := range All {
		if seenName[k.Name] {
			t.Errorf("duplicate name %q", k.Name)
		}
		if seenUsage[k.Usage] {
			t.Errorf("duplicate usage %v (%s)", k.Usage, k.Name)
		}
		seenName[k.Name], seenUsage[k.Usage] = true, true
		if k.Linux != "" {
			if _, ok := LinuxCodes[k.Linux]; !ok {
				t.Errorf("%s: unknown Linux name %s", k.Name, k.Linux)
			}
		}
		if k.Name != strings.ToLower(k.Name) {
			t.Errorf("%s: names are lower-case", k.Name)
		}
	}
}

func TestLinuxCapture(t *testing.T) {
	data, err := os.ReadFile("../../merc-capture/events.jsonl")
	if err != nil {
		t.Skip("capture not present:", err)
	}
	hwdbRemapped := map[uint32]bool{0x70046: true, 0x70076: true, 0x70079: true, 0x70077: true, 0xC0183: true}
	var scan uint32
	checked := 0
	for _, line := range strings.Split(string(data), "\n") {
		var e struct {
			Type, Code, Value int
			Src               string
		}
		if json.Unmarshal([]byte(line), &e) != nil || e.Src == "" || !strings.HasPrefix(e.Src, "event") {
			continue
		}
		switch {
		case e.Type == 4 && e.Code == 4:
			scan = uint32(e.Value)
		case e.Type == 1 && e.Value == 1 && scan != 0:
			if hwdbRemapped[scan] || scan>>16 != pageKeyboard && scan>>16 != pageConsumer {
				continue
			}
			k, ok := ByUsage(hidrd.Usage(scan))
			if !ok {
				t.Errorf("usage %v has no key", hidrd.Usage(scan))
				continue
			}
			want, _ := k.LinuxCode()
			if got, ok := ByLinuxCode(uint16(e.Code)); ok && got.Usage != k.Usage {
				if k.Usage.Page() == pageConsumer || hwdbLegend(k) {
					continue
				}
				t.Errorf("usage %v (%s): kernel code %d (%s), table says %d", k.Usage, k.Name, e.Code, got.Name, want)
			}
			checked++
		}
	}
	if checked < 50 {
		t.Errorf("only %d events checked", checked)
	}
}

func hwdbLegend(k Key) bool {
	u := k.Usage.ID()
	return u >= 0x54 && u <= 0x6F || u == 0x74
}

func TestWindowsCapture(t *testing.T) {
	data, err := os.ReadFile("../../docs/capture-stealth.md")
	if err != nil {
		t.Skip("capture not present:", err)
	}
	re := regexp.MustCompile(`vk([0-9A-F]{2})/sc([0-9A-F]{2})(E?)=1`)
	checked, unknown := 0, map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "| ") || strings.Contains(line, "| m2-") {
			continue
		}
		for _, m := range re.FindAllStringSubmatch(line, -1) {
			vk, _ := strconv.ParseUint(m[1], 16, 8)
			scan, _ := strconv.ParseUint(m[2], 16, 8)
			k, ok := ByScan(uint16(scan), m[3] == "E", uint8(vk))
			if !ok {
				unknown[m[0]] = true
				continue
			}
			if k.VK != uint8(vk) && !(scan == 0x1C && m[3] == "E") {
				t.Errorf("%s: table VK %02X for %s", m[0], k.VK, k.Name)
			}
			checked++
		}
	}
	for u := range unknown {
		t.Errorf("no key for %s", u)
	}
	if checked < 100 {
		t.Errorf("only %d scan codes checked", checked)
	}
}
