package hidrd

import (
	"encoding/hex"
	"testing"
)

const (
	mercIF0 = "05010906a101050719e029e71500250175019508810295017508810195037501050819012903910295057501910195067508150026ff00050719002aff008100c0"
	mercIF1 = "060c000901a101850125011500750109b509b609b709cd09e209e909ea0a8301950881020a8a010a92010a94010a21020a23020a24020a25020a2602950881020a27020a2a029502810295068101c00600ff0901a1018502750895078101150026ff00092075089502b102c0"
)

func parse(t *testing.T, h string) *Descriptor {
	t.Helper()
	b, err := hex.DecodeString(h)
	if err != nil {
		t.Fatal(err)
	}
	d, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestBootKeyboard(t *testing.T) {
	d := parse(t, mercIF0)
	if d.UsesReportIDs() {
		t.Error("boot keyboard must not use report IDs")
	}
	if got := d.ReportLength(Input, 0); got != 8 {
		t.Errorf("input length %d, want 8", got)
	}
	if got := d.ReportLength(Output, 0); got != 1 {
		t.Errorf("output length %d, want 1", got)
	}
	if len(d.Collections) != 1 || d.Collections[0].Usage != 0x00010006 {
		t.Errorf("collections %+v", d.Collections)
	}
	got := d.ActiveUsages(0, []byte{0, 0, 0x54, 0x04, 0, 0, 0, 0})
	want := []Usage{0x00070054, 0x00070004}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("active %v, want %v", got, want)
	}
	got = d.ActiveUsages(0, []byte{0x02, 0, 0, 0, 0, 0, 0, 0})
	if len(got) != 1 || got[0] != 0x000700E1 {
		t.Errorf("modifier %v, want 0007:00E1", got)
	}
	var leds []Usage
	for _, f := range d.Fields {
		if f.Kind == Output && !f.Constant {
			leds = f.Usages
		}
	}
	if len(leds) != 3 || leds[0] != 0x00080001 || leds[2] != 0x00080003 {
		t.Errorf("LED usages %v", leds)
	}
}

func TestMercConsumerAndVendor(t *testing.T) {
	d := parse(t, mercIF1)
	if !d.UsesReportIDs() {
		t.Error("expected report IDs")
	}
	for _, c := range []struct {
		kind Kind
		id   uint8
		want int
	}{{Input, 1, 3}, {Input, 2, 7}, {Feature, 2, 2}} {
		if got := d.ReportLength(c.kind, c.id); got != c.want {
			t.Errorf("%s %d length %d, want %d", c.kind, c.id, got, c.want)
		}
	}
	if ids := d.ReportIDs(Feature); len(ids) != 1 || ids[0] != 2 {
		t.Errorf("feature ids %v", ids)
	}
	if len(d.Collections) != 2 || d.Collections[1].Usage.Page() != 0xFF00 {
		t.Errorf("collections %+v", d.Collections)
	}
	var slots []Usage
	for _, f := range d.Fields {
		if f.Kind == Input && f.ReportID == 1 && !f.Constant {
			slots = append(slots, f.Usages...)
		}
	}
	want := []Usage{0x000C00B5, 0x000C00B6, 0x000C00B7, 0x000C00CD, 0x000C00E2, 0x000C00E9, 0x000C00EA, 0x000C0183,
		0x000C018A, 0x000C0192, 0x000C0194, 0x000C0221, 0x000C0223, 0x000C0224, 0x000C0225, 0x000C0226, 0x000C0227, 0x000C022A}
	if len(slots) != len(want) {
		t.Fatalf("slots %v", slots)
	}
	for i := range want {
		if slots[i] != want[i] {
			t.Errorf("slot %d = %v, want %v", i, slots[i], want[i])
		}
	}
	for _, c := range []struct {
		body string
		want Usage
	}{{"001000", 0x000C0223}, {"800000", 0x000C0183}, {"000400", 0x000C0194}, {"000002", 0x000C022A}} {
		b, _ := hex.DecodeString(c.body)
		got := d.ActiveUsages(1, b)
		if len(got) != 1 || got[0] != c.want {
			t.Errorf("report 1 %s -> %v, want %v", c.body, got, c.want)
		}
	}
}

func TestTruncated(t *testing.T) {
	if _, err := Parse([]byte{0x05, 0x01, 0x09, 0x06, 0xA1}); err == nil {
		t.Error("expected an error for a truncated item")
	}
	if _, err := Parse([]byte{0x05, 0x01, 0x09, 0x06, 0xA1, 0x01}); err == nil {
		t.Error("expected an error for an unbalanced collection")
	}
}

func TestLimits(t *testing.T) {
	for _, c := range []struct {
		name string
		hex  string
	}{
		{"usage range over the whole 32 bits", "050109061b000000002bffffffff750195018100"},
		{"usage maximum below minimum", "0501090619e729e0750195088100"},
		{"usage range of 12289", "05011900002a0030750195018100"},
		{"report size 257", "0501090676010195018100"},
		{"report count 12289", "0501090675019601308100"},
		{"report count times report size found by FuzzParse", "9730c03c66769db8823030"},
		{"input report over 16384 bytes", "0501090615002501750896003081028102"},
	} {
		b, err := hex.DecodeString(c.hex)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Parse(b); err == nil {
			t.Errorf("%s: parsed without error", c.name)
		}
	}
	if _, err := Parse(make([]byte, maxDescriptor+1)); err == nil {
		t.Error("a descriptor over the maximum parsed without error")
	}
	d := parse(t, "05011bf0ffffff2bffffffff750195108100")
	if n := len(d.Fields[0].Usages); n != 16 || d.Fields[0].UsageMax != 0xFFFFFFFF {
		t.Errorf("range ending at the top of the usage space: %d usages, max %v", n, d.Fields[0].UsageMax)
	}
	d = parse(t, "050119002aff2f75019600308100")
	if n := len(d.Fields[0].Usages); n != maxUsages || d.ReportLength(Input, 0) != maxUsages/8 {
		t.Errorf("range of %d usages: %d usages, %d bytes", maxUsages, n, d.ReportLength(Input, 0))
	}
}

func FuzzParse(f *testing.F) {
	for _, h := range []string{mercIF0, mercIF1} {
		b, _ := hex.DecodeString(h)
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		d, err := Parse(b)
		if err != nil {
			return
		}
		for _, id := range d.ReportIDs(Input) {
			d.ActiveUsages(id, make([]byte, d.ReportLength(Input, id)))
		}
	})
}
