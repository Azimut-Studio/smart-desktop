//go:build windows && amd64

package win32

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"unsafe"

	"github.com/Azimut-Studio/smart-desktop/internal/model"
	"golang.org/x/sys/windows"
)

type monitorInfo struct {
	Size          uint32
	Monitor, Work Rect
	Flags         uint32
	Device        [32]uint16
}
type displayDevice struct {
	Size        uint32
	Name        [32]uint16
	Description [128]uint16
	Flags       uint32
	ID          [128]uint16
	Key         [128]uint16
}
type devMode struct {
	Name                                                                                   [32]uint16
	Spec, Driver, Size, Extra                                                              uint16
	Fields                                                                                 uint32
	X, Y                                                                                   int32
	Orientation, Fixed                                                                     uint32
	Color, Duplex, YResolution, TTOption, Collate                                          int16
	Form                                                                                   [32]uint16
	LogPixels                                                                              uint16
	Bits, Width, Height, Flags, Frequency                                                  uint32
	ICMMethod, ICMIntent, Media, Dither, Reserved1, Reserved2, PanningWidth, PanningHeight uint32
}

type Display struct{}

var monitorMutex sync.Mutex
var monitorOutput []model.Monitor
var monitorFailure error
var monitorTargets map[string][]model.Target
var monitorCallback = windows.NewCallback(enumerateMonitor)

func (Display) Monitors() ([]model.Monitor, error) {
	monitorMutex.Lock()
	defer monitorMutex.Unlock()
	monitorOutput = nil
	monitorFailure = nil
	var err error
	monitorTargets, err = activeTargets()
	if err != nil {
		return nil, err
	}
	ok, _, e := User32.NewProc("EnumDisplayMonitors").Call(0, 0, monitorCallback, 0)
	if monitorFailure != nil {
		return nil, monitorFailure
	}
	if err := CheckBOOL("EnumDisplayMonitors", ok, e); err != nil {
		return nil, err
	}
	out := monitorOutput
	monitorOutput = nil
	sort.Slice(out, func(i, j int) bool { return out[i].Identity < out[j].Identity })
	return out, model.ValidateMonitors(out)
}

func enumerateMonitor(h, dc, rect, data uintptr) uintptr {
	var info monitorInfo
	info.Size = uint32(unsafe.Sizeof(info))
	ok, _, e := User32.NewProc("GetMonitorInfoW").Call(h, uintptr(unsafe.Pointer(&info)))
	if monitorFailure = CheckBOOL("GetMonitorInfoW", ok, e); monitorFailure != nil {
		return 0
	}
	var mode devMode
	mode.Size = uint16(unsafe.Sizeof(mode))
	ok, _, e = User32.NewProc("EnumDisplaySettingsW").Call(uintptr(unsafe.Pointer(&info.Device[0])), uintptr(^uint32(0)), uintptr(unsafe.Pointer(&mode)))
	if monitorFailure = CheckBOOL("EnumDisplaySettingsW", ok, e); monitorFailure != nil {
		return 0
	}
	var x, y uint32
	hr, _, _ := windows.NewLazySystemDLL("shcore.dll").NewProc("GetDpiForMonitor").Call(h, 0, uintptr(unsafe.Pointer(&x)), uintptr(unsafe.Pointer(&y)))
	if monitorFailure = HResult("GetDpiForMonitor", hr); monitorFailure != nil {
		return 0
	}
	targets := monitorTargets[windows.UTF16ToString(info.Device[:])]
	if len(targets) == 0 {
		monitorFailure = errors.New("identifiant matériel d'écran indisponible")
		return 0
	}
	var ids, names []string
	for _, target := range targets {
		ids = append(ids, target.Identity)
		names = append(names, target.Name)
	}
	id := strings.Join(ids, "|")
	monitorOutput = append(monitorOutput, model.Monitor{
		Identity: id, Name: strings.Join(names, " + "), Targets: targets,
		Left: info.Monitor.Left, Top: info.Monitor.Top,
		Width: info.Monitor.Right - info.Monitor.Left, Height: info.Monitor.Bottom - info.Monitor.Top,
		Orientation: mode.Orientation, DPIX: x, DPIY: y, Primary: info.Flags&1 != 0,
	})
	return 1
}
