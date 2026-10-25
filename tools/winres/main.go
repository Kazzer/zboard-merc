package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/Kazzer/zboard-merc/internal/tray"
	"github.com/Kazzer/zboard-merc/internal/version"
)

type machine struct {
	code  uint16
	reloc uint16
}

var machines = map[string]machine{
	"386":   {0x014c, 7},
	"amd64": {0x8664, 3},
	"arm64": {0xaa64, 2},
}

type icon struct {
	size int
	data []byte
}

type resource struct {
	kind uint32
	id   uint32
	data []byte
}

func main() {
	err := run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "winres:", err)
		os.Exit(1)
	}
}

func run() error {
	arch := flag.String("arch", "amd64", "the GOARCH the object is for: 386, amd64 or arm64")
	name := flag.String("name", "", "the command, zengine for instance")
	desc := flag.String("desc", "", "the file description Explorer shows")
	out := flag.String("o", "", "the object to write (default rsrc_windows_ARCH.syso)")
	flag.Parse()
	if *name == "" {
		return errors.New("-name is required")
	}
	m, ok := machines[*arch]
	if !ok {
		return fmt.Errorf("-arch %s: want 386, amd64 or arm64", *arch)
	}
	if *out == "" {
		*out = "rsrc_windows_" + *arch + ".syso"
	}
	images, err := icons()
	if err != nil {
		return err
	}
	var resources []resource
	for i, img := range images {
		resources = append(resources, resource{3, uint32(i + 1), img.data})
	}
	resources = append(resources, resource{14, 1, group(images)}, resource{16, 1, versionInfo(*name, *desc)})
	return os.WriteFile(*out, object(m, resources), 0o644)
}

func object(m machine, resources []resource) []byte {
	var kinds []uint32
	count := map[uint32]int{}
	for _, r := range resources {
		if count[r.kind] == 0 {
			kinds = append(kinds, r.kind)
		}
		count[r.kind]++
	}
	n := len(resources)
	typeDir := 16 + 8*len(kinds)
	idDir := typeDir + 16*len(kinds) + 8*n
	entry := idDir + 24*n
	blob := entry + 16*n
	var section, relocs bytes.Buffer
	directory := func(entries int) {
		binary.Write(&section, binary.LittleEndian, []uint32{0, 0, 0, uint32(entries) << 16})
	}
	directory(len(kinds))
	for _, kind := range kinds {
		binary.Write(&section, binary.LittleEndian, []uint32{kind, 0x80000000 | uint32(typeDir)})
		typeDir += 16 + 8*count[kind]
	}
	i := 0
	for _, kind := range kinds {
		directory(count[kind])
		for ; i < n && resources[i].kind == kind; i++ {
			binary.Write(&section, binary.LittleEndian, []uint32{resources[i].id, 0x80000000 | uint32(idDir+24*i)})
		}
	}
	for i := range resources {
		directory(1)
		binary.Write(&section, binary.LittleEndian, []uint32{0x0409, uint32(entry + 16*i)})
	}
	for i, r := range resources {
		binary.Write(&section, binary.LittleEndian, []uint32{uint32(blob), uint32(len(r.data)), 0, 0})
		binary.Write(&relocs, binary.LittleEndian, []uint32{uint32(entry + 16*i), 0})
		binary.Write(&relocs, binary.LittleEndian, m.reloc)
		blob += (len(r.data) + 7) &^ 7
	}
	for _, r := range resources {
		section.Write(r.data)
		section.Write(make([]byte, (8-len(r.data)%8)%8))
	}
	var out bytes.Buffer
	binary.Write(&out, binary.LittleEndian, []uint16{m.code, 1})
	binary.Write(&out, binary.LittleEndian, []uint32{0, uint32(60 + section.Len() + relocs.Len()), 1})
	binary.Write(&out, binary.LittleEndian, []uint16{0, 0})
	out.WriteString(".rsrc\x00\x00\x00")
	binary.Write(&out, binary.LittleEndian, []uint32{0, 0, uint32(section.Len()), 60, uint32(60 + section.Len()), 0})
	binary.Write(&out, binary.LittleEndian, []uint16{uint16(n), 0})
	binary.Write(&out, binary.LittleEndian, uint32(0x40000040))
	out.Write(section.Bytes())
	out.Write(relocs.Bytes())
	out.WriteString(".rsrc\x00\x00\x00")
	binary.Write(&out, binary.LittleEndian, []uint16{0, 0, 1, 0})
	out.Write([]byte{3, 0})
	binary.Write(&out, binary.LittleEndian, uint32(4))
	return out.Bytes()
}

func versionInfo(name, desc string) []byte {
	var parts [4]uint32
	for i, p := range strings.SplitN(version.String, ".", 4) {
		n, _ := strconv.Atoi(p)
		parts[i] = uint32(n)
	}
	ms, ls := parts[0]<<16|parts[1], parts[2]<<16|parts[3]
	var fixed bytes.Buffer
	binary.Write(&fixed, binary.LittleEndian, []uint32{0xfeef04bd, 0x00010000, ms, ls, ms, ls, 0x3f, 0, 0x40004, 1, 0, 0, 0})
	var table [][]byte
	for _, kv := range [][2]string{
		{"CompanyName", "Aether Software"},
		{"FileDescription", desc},
		{"FileVersion", version.String},
		{"InternalName", name},
		{"LegalCopyright", "Copyright (c) 2026 Aether Software"},
		{"OriginalFilename", name + ".exe"},
		{"ProductName", "zboard"},
		{"ProductVersion", version.String},
	} {
		value := wide(kv[1])
		table = append(table, block(kv[0], 1, len(value)/2, value))
	}
	return block("VS_VERSION_INFO", 0, fixed.Len(), fixed.Bytes(),
		block("StringFileInfo", 1, 0, nil, block("040904B0", 1, 0, nil, table...)),
		block("VarFileInfo", 1, 0, nil, block("Translation", 0, 4, []byte{0x09, 0x04, 0xb0, 0x04})),
	)
}

func block(key string, kind uint16, valueLength int, value []byte, children ...[]byte) []byte {
	var b bytes.Buffer
	binary.Write(&b, binary.LittleEndian, []uint16{0, uint16(valueLength), kind})
	b.Write(wide(key))
	b.Write(make([]byte, (4-b.Len()%4)%4))
	b.Write(value)
	for _, child := range children {
		b.Write(make([]byte, (4-b.Len()%4)%4))
		b.Write(child)
	}
	out := b.Bytes()
	binary.LittleEndian.PutUint16(out, uint16(len(out)))
	return out
}

func wide(s string) []byte {
	var b bytes.Buffer
	binary.Write(&b, binary.LittleEndian, append(utf16.Encode([]rune(s)), 0))
	return b.Bytes()
}

func group(images []icon) []byte {
	var b bytes.Buffer
	binary.Write(&b, binary.LittleEndian, []uint16{0, 1, uint16(len(images))})
	for i, img := range images {
		b.Write([]byte{byte(img.size), byte(img.size), 0, 0})
		binary.Write(&b, binary.LittleEndian, []uint16{1, 32})
		binary.Write(&b, binary.LittleEndian, uint32(len(img.data)))
		binary.Write(&b, binary.LittleEndian, uint16(i+1))
	}
	return b.Bytes()
}

func icons() (out []icon, err error) {
	src, err := png.Decode(bytes.NewReader(tray.PNG(true, false, true)))
	if err != nil {
		return
	}
	for _, n := range []int{16, 32, 48} {
		img := scale(src, n)
		var b bytes.Buffer
		binary.Write(&b, binary.LittleEndian, []uint32{40, uint32(n), uint32(2 * n)})
		binary.Write(&b, binary.LittleEndian, []uint16{1, 32})
		binary.Write(&b, binary.LittleEndian, make([]uint32, 6))
		for y := n - 1; y >= 0; y-- {
			for x := 0; x < n; x++ {
				c := img.NRGBAAt(x, y)
				b.Write([]byte{c.B, c.G, c.R, c.A})
			}
		}
		stride := (n + 31) / 32 * 4
		for y := n - 1; y >= 0; y-- {
			row := make([]byte, stride)
			for x := 0; x < n; x++ {
				if img.NRGBAAt(x, y).A == 0 {
					row[x/8] |= 0x80 >> (x % 8)
				}
			}
			b.Write(row)
		}
		out = append(out, icon{n, b.Bytes()})
	}
	return
}

func scale(src image.Image, n int) (out *image.NRGBA) {
	out = image.NewNRGBA(image.Rect(0, 0, n, n))
	b := src.Bounds()
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			out.Set(x, y, src.At(b.Min.X+x*b.Dx()/n, b.Min.Y+y*b.Dy()/n))
		}
	}
	return
}
