package hidrd

import (
	"errors"
	"fmt"
)

type Kind uint8

const (
	Input Kind = iota
	Output
	Feature
)

func (k Kind) String() string {
	return [...]string{"input", "output", "feature"}[k]
}

type Usage uint32

func (u Usage) Page() uint16 { return uint16(u >> 16) }
func (u Usage) ID() uint16   { return uint16(u) }

func (u Usage) String() string {
	return fmt.Sprintf("%04X:%04X", u.Page(), u.ID())
}

type Field struct {
	Kind       Kind
	ReportID   uint8
	Offset     int
	Size       int
	Count      int
	Constant   bool
	Variable   bool
	LogicalMin int32
	LogicalMax int32
	Usages     []Usage
	UsageMin   Usage
	UsageMax   Usage
	Collection *Collection
}

type Collection struct {
	Type     uint8
	Usage    Usage
	Parent   *Collection
	Children []*Collection
}

type Descriptor struct {
	Fields      []Field
	Collections []*Collection
}

var ErrTruncated = errors.New("hidrd: truncated item")

const (
	maxDescriptor  = 4096
	maxReportSize  = 256
	maxReportCount = 12288
	maxUsages      = 12288
	maxReportBytes = 16384
)

func Parse(desc []byte) (*Descriptor, error) {
	if len(desc) > maxDescriptor {
		return nil, fmt.Errorf("hidrd: descriptor of %d bytes exceeds %d", len(desc), maxDescriptor)
	}
	d := &Descriptor{}
	var (
		page           uint16
		reportID       uint8
		size, count    int
		logMin, logMax int32
		usages         []Usage
		usageMin       Usage
		haveUsageMin   bool
		current        *Collection
		offsets        = map[[2]uint8]int{}
		depth          int
	)
	ext := func(data uint32, n int) Usage {
		if n == 4 {
			return Usage(data)
		}
		return Usage(uint32(page)<<16 | data)
	}
	for i := 0; i < len(desc); {
		head := desc[i]
		i++
		if head == 0xFE {
			if i+1 >= len(desc) {
				return nil, ErrTruncated
			}
			i += 2 + int(desc[i])
			continue
		}
		n := [4]int{0, 1, 2, 4}[head&3]
		if i+n > len(desc) {
			return nil, ErrTruncated
		}
		var data uint32
		for k := 0; k < n; k++ {
			data |= uint32(desc[i+k]) << (8 * k)
		}
		i += n
		switch head & 0xFC {
		case 0x04:
			page = uint16(data)
		case 0x84:
			reportID = uint8(data)
		case 0x74:
			if data > maxReportSize {
				return nil, fmt.Errorf("hidrd: report size %d exceeds %d bits", data, maxReportSize)
			}
			size = int(data)
		case 0x94:
			if data > maxReportCount {
				return nil, fmt.Errorf("hidrd: report count %d exceeds %d", data, maxReportCount)
			}
			count = int(data)
		case 0x14:
			logMin = signExtend(data, n)
		case 0x24:
			logMax = signExtend(data, n)
		case 0x08:
			if len(usages) >= maxUsages {
				return nil, fmt.Errorf("hidrd: more than %d usages for one item", maxUsages)
			}
			usages = append(usages, ext(data, n))
		case 0x18:
			usageMin, haveUsageMin = ext(data, n), true
		case 0x28:
			if haveUsageMin {
				max := ext(data, n)
				if max < usageMin || uint64(max-usageMin) >= uint64(maxUsages-len(usages)) {
					return nil, fmt.Errorf("hidrd: usage range %s..%s is empty or exceeds %d usages", usageMin, max, maxUsages)
				}
				for u := usageMin; ; u++ {
					usages = append(usages, u)
					if u == max {
						break
					}
				}
				haveUsageMin = false
			}
		case 0xA0:
			c := &Collection{Type: uint8(data), Parent: current}
			if len(usages) > 0 {
				c.Usage = usages[len(usages)-1]
			}
			if current == nil {
				d.Collections = append(d.Collections, c)
			} else {
				current.Children = append(current.Children, c)
			}
			current = c
			depth++
			usages = usages[:0]
		case 0xC0:
			if current == nil {
				return nil, errors.New("hidrd: end collection without start")
			}
			current = current.Parent
			depth--
			usages = usages[:0]
		case 0x80, 0x90, 0xB0:
			f := Field{
				Kind:       Kind(int(head&0xF0>>4) - 8),
				ReportID:   reportID,
				Size:       size,
				Count:      count,
				Constant:   data&1 != 0,
				Variable:   data&2 != 0,
				LogicalMin: logMin,
				LogicalMax: logMax,
				Usages:     append([]Usage(nil), usages...),
				Collection: current,
			}
			if head&0xF0 == 0xB0 {
				f.Kind = Feature
			}
			key := [2]uint8{uint8(f.Kind), reportID}
			f.Offset = offsets[key]
			offsets[key] += size * count
			if offsets[key] > maxReportBytes*8 {
				return nil, fmt.Errorf("hidrd: %s report %d exceeds %d bytes", f.Kind, reportID, maxReportBytes)
			}
			if len(f.Usages) > 0 {
				f.UsageMin, f.UsageMax = f.Usages[0], f.Usages[len(f.Usages)-1]
			}
			d.Fields = append(d.Fields, f)
			usages = usages[:0]
		}
	}
	if depth != 0 {
		return nil, errors.New("hidrd: unbalanced collections")
	}
	return d, nil
}

func signExtend(data uint32, n int) int32 {
	switch n {
	case 1:
		return int32(int8(data))
	case 2:
		return int32(int16(data))
	}
	return int32(data)
}

func (d *Descriptor) UsesReportIDs() bool {
	for _, f := range d.Fields {
		if f.ReportID != 0 {
			return true
		}
	}
	return false
}

func (d *Descriptor) ReportLength(kind Kind, id uint8) int {
	bits := 0
	for _, f := range d.Fields {
		if f.Kind == kind && f.ReportID == id {
			bits += f.Size * f.Count
		}
	}
	return (bits + 7) / 8
}

func (d *Descriptor) ReportIDs(kind Kind) []uint8 {
	var ids []uint8
	seen := map[uint8]bool{}
	for _, f := range d.Fields {
		if f.Kind == kind && !seen[f.ReportID] {
			seen[f.ReportID] = true
			ids = append(ids, f.ReportID)
		}
	}
	return ids
}

func (d *Descriptor) ActiveUsages(id uint8, body []byte) []Usage {
	var out []Usage
	for _, f := range d.Fields {
		if f.Kind != Input || f.ReportID != id || f.Constant {
			continue
		}
		for k := 0; k < f.Count; k++ {
			v := bits(body, f.Offset+k*f.Size, f.Size)
			if v == 0 {
				continue
			}
			switch {
			case f.Variable && k < len(f.Usages):
				out = append(out, f.Usages[k])
			case f.Variable:
				out = append(out, f.UsageMax)
			case len(f.Usages) == 1 && f.Usages[0] == f.UsageMax:
				out = append(out, f.Usages[0])
			default:
				idx := int(int32(v) - f.LogicalMin)
				if f.UsageMax-f.UsageMin+1 == Usage(len(f.Usages)) {
					out = append(out, f.UsageMin+Usage(idx))
				} else if idx >= 0 && idx < len(f.Usages) {
					out = append(out, f.Usages[idx])
				}
			}
		}
	}
	return out
}

func bits(body []byte, offset, size int) uint32 {
	var v uint32
	for b := 0; b < size; b++ {
		pos := offset + b
		if pos/8 >= len(body) {
			break
		}
		v |= uint32(body[pos/8]>>(pos%8)&1) << b
	}
	return v
}
