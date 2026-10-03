//go:build windows && amd64

package win32

import (
	"encoding/binary"
	"fmt"
	"sort"
	"unsafe"

	"github.com/Azimut-Studio/smart-desktop/internal/model"
	"golang.org/x/sys/windows"
)

type luid struct {
	Low  uint32
	High int32
}
type pathSource struct {
	Adapter         luid
	ID, Mode, Flags uint32
}
type rational struct{ Numerator, Denominator uint32 }
type pathTarget struct {
	Adapter                                 luid
	ID, Mode, Technology, Rotation, Scaling uint32
	Refresh                                 rational
	Scanline, Available, Flags              uint32
}
type displayPath struct {
	Source pathSource
	Target pathTarget
	Flags  uint32
}
type displayMode struct {
	Type, ID uint32
	Adapter  luid
	Value    [48]byte
}
type deviceHeader struct {
	Type, Size uint32
	Adapter    luid
	ID         uint32
}
type sourceName struct {
	Header deviceHeader
	Name   [32]uint16
}
type targetName struct {
	Header                deviceHeader
	Flags, Technology     uint32
	Manufacturer, Product uint16
	Connector             uint32
	Name                  [64]uint16
	Path                  [128]uint16
}

func configError(name string, code uintptr) error {
	if code == 0 {
		return nil
	}
	return fmt.Errorf("%s : %w", name, windows.Errno(code))
}

func activeTargets() (map[string][]model.Target, error) {
	for attempt := 0; attempt < 3; attempt++ {
		var pathCount, modeCount uint32
		code, _, _ := User32.NewProc("GetDisplayConfigBufferSizes").Call(2, uintptr(unsafe.Pointer(&pathCount)), uintptr(unsafe.Pointer(&modeCount)))
		if err := configError("GetDisplayConfigBufferSizes", code); err != nil {
			return nil, err
		}
		if pathCount == 0 || pathCount > 128 || modeCount == 0 || modeCount > 512 {
			return nil, fmt.Errorf("tailles de configuration d'affichage invalides")
		}
		paths := make([]displayPath, pathCount)
		modes := make([]displayMode, modeCount)
		code, _, _ = User32.NewProc("QueryDisplayConfig").Call(2, uintptr(unsafe.Pointer(&pathCount)), uintptr(unsafe.Pointer(&paths[0])), uintptr(unsafe.Pointer(&modeCount)), uintptr(unsafe.Pointer(&modes[0])), 0)
		if code == 122 {
			continue
		}
		if err := configError("QueryDisplayConfig", code); err != nil {
			return nil, err
		}
		out := make(map[string][]model.Target)
		for _, path := range paths[:pathCount] {
			source := sourceName{Header: deviceHeader{Type: 1, Size: uint32(unsafe.Sizeof(sourceName{})), Adapter: path.Source.Adapter, ID: path.Source.ID}}
			code, _, _ = User32.NewProc("DisplayConfigGetDeviceInfo").Call(uintptr(unsafe.Pointer(&source)))
			if err := configError("DisplayConfigGetDeviceInfo source", code); err != nil {
				return nil, err
			}
			target := targetName{Header: deviceHeader{Type: 2, Size: uint32(unsafe.Sizeof(targetName{})), Adapter: path.Target.Adapter, ID: path.Target.ID}}
			code, _, _ = User32.NewProc("DisplayConfigGetDeviceInfo").Call(uintptr(unsafe.Pointer(&target)))
			if err := configError("DisplayConfigGetDeviceInfo cible", code); err != nil {
				return nil, err
			}
			if path.Target.Mode >= modeCount {
				return nil, fmt.Errorf("mode physique d'écran indisponible")
			}
			mode := modes[path.Target.Mode]
			if mode.Type != 2 || mode.ID != path.Target.ID || mode.Adapter != path.Target.Adapter {
				return nil, fmt.Errorf("mode physique d'écran incohérent")
			}
			width := binary.LittleEndian.Uint32(mode.Value[24:28])
			height := binary.LittleEndian.Uint32(mode.Value[28:32])
			if path.Target.Rotation < 1 || path.Target.Rotation > 4 {
				return nil, fmt.Errorf("orientation physique d'écran indisponible")
			}
			if path.Target.Rotation == 2 || path.Target.Rotation == 4 {
				width, height = height, width
			}
			gdiName := windows.UTF16ToString(source.Name[:])
			identity := windows.UTF16ToString(target.Path[:])
			if identity == "" || gdiName == "" || width == 0 || height == 0 {
				return nil, fmt.Errorf("identité ou résolution physique d'écran indisponible")
			}
			out[gdiName] = append(out[gdiName], model.Target{Identity: identity, Name: windows.UTF16ToString(target.Name[:]), Width: width, Height: height, Orientation: path.Target.Rotation - 1})
		}
		for key := range out {
			sort.Slice(out[key], func(i, j int) bool { return out[key][i].Identity < out[key][j].Identity })
		}
		return out, nil
	}
	return nil, fmt.Errorf("configuration d'affichage instable ; réessayez")
}
