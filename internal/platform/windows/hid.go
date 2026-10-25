//go:build windows

package platwin

import (
	"errors"
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	hid    = windows.NewLazySystemDLL("hid.dll")
	cfgmgr = windows.NewLazySystemDLL("cfgmgr32.dll")

	hidGetAttributes       = hid.NewProc("HidD_GetAttributes")
	hidGetPreparsedData    = hid.NewProc("HidD_GetPreparsedData")
	hidFreePreparsedData   = hid.NewProc("HidD_FreePreparsedData")
	hidGetCaps             = hid.NewProc("HidP_GetCaps")
	hidSetFeature          = hid.NewProc("HidD_SetFeature")
	hidSetOutputReport     = hid.NewProc("HidD_SetOutputReport")
	hidGetPhysical         = hid.NewProc("HidD_GetPhysicalDescriptor")
	hidGetFeature          = hid.NewProc("HidD_GetFeature")
	hidGetInputReport      = hid.NewProc("HidD_GetInputReport")
	hidGetProductString    = hid.NewProc("HidD_GetProductString")
	hidGetUsagesEx         = hid.NewProc("HidP_GetUsagesEx")
	cmGetInterfaceListSize = cfgmgr.NewProc("CM_Get_Device_Interface_List_SizeW")
	cmGetInterfaceList     = cfgmgr.NewProc("CM_Get_Device_Interface_ListW")
)

const hidpStatusSuccess = 0x00110000

var hidInterfaceGUID = windows.GUID{Data1: 0x4D1E55B2, Data2: 0xF16F, Data3: 0x11CF, Data4: [8]byte{0x88, 0xCB, 0x00, 0x11, 0x11, 0x00, 0x00, 0x30}}

type Caps struct {
	Usage, UsagePage                                        uint16
	InputLen, OutputLen, FeatureLen                         uint16
	_                                                       [17]uint16
	LinkCollectionNodes                                     uint16
	InputButtonCaps, InputValueCaps, InputDataIndices       uint16
	OutputButtonCaps, OutputValueCaps, OutputDataIndices    uint16
	FeatureButtonCaps, FeatureValueCaps, FeatureDataIndices uint16
}

type UsageAndPage struct {
	Usage, Page uint16
}

type Device struct {
	Path                         string
	Product                      string
	VendorID, ProductID, Version uint16
	Caps                         Caps
	ReadOnly                     bool
	handle, readHandle           windows.Handle
	preparsed                    uintptr
}

func interfacePaths() ([]string, error) { return interfacePathsFor(&hidInterfaceGUID) }

func interfacePathsFor(guid *windows.GUID) ([]string, error) {
	var n uint32
	if r, _, _ := cmGetInterfaceListSize.Call(uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(guid)), 0, 0); r != 0 {
		return nil, fmt.Errorf("CM_Get_Device_Interface_List_Size: CR 0x%X", r)
	}
	buf := make([]uint16, n+1)
	if r, _, _ := cmGetInterfaceList.Call(uintptr(unsafe.Pointer(guid)), 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(n), 0); r != 0 {
		return nil, fmt.Errorf("CM_Get_Device_Interface_List: CR 0x%X", r)
	}
	var paths []string
	start := 0
	for i, c := range buf {
		if c == 0 {
			if i > start {
				paths = append(paths, windows.UTF16ToString(buf[start:i]))
			}
			start = i + 1
		}
	}
	return paths, nil
}

func Enumerate(vid, pid uint16) ([]*Device, error) {
	paths, err := interfacePaths()
	if err != nil {
		return nil, err
	}
	want := fmt.Sprintf("VID_%04X&PID_%04X", vid, pid)
	if pid == 0 {
		want = fmt.Sprintf("VID_%04X&PID_", vid)
	}
	var devs []*Device
	for _, p := range paths {
		if !strings.Contains(strings.ToUpper(p), want) {
			continue
		}
		d, err := Open(p)
		if err != nil {
			return devs, fmt.Errorf("%s: %w", p, err)
		}
		devs = append(devs, d)
	}
	return devs, nil
}

func Open(path string) (*Device, error) {
	p16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	share := uint32(windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE)
	d := &Device{Path: path}
	d.handle, err = windows.CreateFile(p16, windows.GENERIC_READ|windows.GENERIC_WRITE, share, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		d.handle, err = windows.CreateFile(p16, 0, share, nil, windows.OPEN_EXISTING, 0, 0)
		if err != nil {
			return nil, err
		}
		d.ReadOnly = true
	} else if d.readHandle, err = windows.CreateFile(p16, windows.GENERIC_READ|windows.GENERIC_WRITE, share, nil, windows.OPEN_EXISTING, 0, 0); err != nil {
		windows.CloseHandle(d.handle)
		return nil, fmt.Errorf("second handle: %w", err)
	}
	var attr struct {
		Size                     uint32
		Vendor, Product, Version uint16
	}
	attr.Size = uint32(unsafe.Sizeof(attr))
	if r, _, e := hidGetAttributes.Call(uintptr(d.handle), uintptr(unsafe.Pointer(&attr))); r == 0 {
		d.Close()
		return nil, fmt.Errorf("HidD_GetAttributes: %w", e)
	}
	d.VendorID, d.ProductID, d.Version = attr.Vendor, attr.Product, attr.Version
	if r, _, e := hidGetPreparsedData.Call(uintptr(d.handle), uintptr(unsafe.Pointer(&d.preparsed))); r == 0 {
		d.Close()
		return nil, fmt.Errorf("HidD_GetPreparsedData: %w", e)
	}
	if r, _, _ := hidGetCaps.Call(d.preparsed, uintptr(unsafe.Pointer(&d.Caps))); r != hidpStatusSuccess {
		d.Close()
		return nil, fmt.Errorf("HidP_GetCaps: status 0x%X", r)
	}
	var name [128]uint16
	if r, _, _ := hidGetProductString.Call(uintptr(d.handle), uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)*2)); r != 0 {
		d.Product = windows.UTF16ToString(name[:])
	}
	return d, nil
}

func (d *Device) Close() {
	if d.preparsed != 0 {
		hidFreePreparsedData.Call(d.preparsed)
		d.preparsed = 0
	}
	for _, h := range []*windows.Handle{&d.readHandle, &d.handle} {
		if *h != 0 {
			windows.CancelIoEx(*h, nil)
			windows.CloseHandle(*h)
			*h = 0
		}
	}
}

func (d *Device) SetFeature(report []byte) error {
	if r, _, e := hidSetFeature.Call(uintptr(d.handle), uintptr(unsafe.Pointer(&report[0])), uintptr(len(report))); r == 0 {
		return fmt.Errorf("HidD_SetFeature: %w", e)
	}
	return nil
}

func (d *Device) SetOutputReport(report []byte) error {
	if r, _, e := hidSetOutputReport.Call(uintptr(d.handle), uintptr(unsafe.Pointer(&report[0])), uintptr(len(report))); r == 0 {
		return fmt.Errorf("HidD_SetOutputReport: %w", e)
	}
	return nil
}

func (d *Device) GetPhysicalDescriptor() ([]byte, error) {
	buf := make([]byte, 64)
	if r, _, e := hidGetPhysical.Call(uintptr(d.handle), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf))); r == 0 {
		return nil, fmt.Errorf("HidD_GetPhysicalDescriptor: %w", e)
	}
	return buf, nil
}

func (d *Device) GetFeature(id byte) ([]byte, error) {
	buf := make([]byte, d.Caps.FeatureLen)
	if len(buf) == 0 {
		return nil, errors.New("no feature report declared")
	}
	buf[0] = id
	if r, _, e := hidGetFeature.Call(uintptr(d.handle), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf))); r == 0 {
		return nil, fmt.Errorf("HidD_GetFeature: %w", e)
	}
	return buf, nil
}

func (d *Device) GetInputReport(id byte) ([]byte, error) {
	buf := make([]byte, d.Caps.InputLen)
	if len(buf) == 0 {
		return nil, errors.New("no input report declared")
	}
	buf[0] = id
	if r, _, e := hidGetInputReport.Call(uintptr(d.handle), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf))); r == 0 {
		return nil, fmt.Errorf("HidD_GetInputReport: %w", e)
	}
	return buf, nil
}

func (d *Device) Read() ([]byte, error) {
	if d.readHandle == 0 {
		return nil, errors.New("collection opened without read access")
	}
	buf := make([]byte, d.Caps.InputLen)
	var n uint32
	if err := windows.ReadFile(d.readHandle, buf, &n, nil); err != nil {
		return nil, err
	}
	return buf[:n], nil
}

func (d *Device) Usages(report []byte) []UsageAndPage {
	list := make([]UsageAndPage, 64)
	n := uint32(len(list))
	r, _, _ := hidGetUsagesEx.Call(0, 0, uintptr(unsafe.Pointer(&list[0])), uintptr(unsafe.Pointer(&n)), d.preparsed,
		uintptr(unsafe.Pointer(&report[0])), uintptr(len(report)))
	if r != hidpStatusSuccess {
		return nil
	}
	return list[:n]
}
