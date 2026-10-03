//go:build windows && amd64

package win32

import (
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/Azimut-Studio/smart-desktop/internal/model"
	"golang.org/x/sys/windows"
)

var (
	clsShellWindows = windows.GUID{Data1: 0x9ba05972, Data2: 0xf6a8, Data3: 0x11cf, Data4: [8]byte{0xa4, 0x42, 0, 0xa0, 0xc9, 0x0a, 0x8f, 0x39}}
	iidShellWindows = windows.GUID{Data1: 0x85cb6900, Data2: 0x4d95, Data3: 0x11cf, Data4: [8]byte{0x96, 0x0c, 0, 0x80, 0xc7, 0xf4, 0xee, 0x85}}
	iidProvider     = windows.GUID{Data1: 0x6d5140c1, Data2: 0x7436, Data3: 0x11ce, Data4: [8]byte{0x80, 0x34, 0, 0xaa, 0, 0x60, 0x09, 0xfa}}
	sidBrowser      = windows.GUID{Data1: 0x4c96be40, Data2: 0x915c, Data3: 0x11cf, Data4: [8]byte{0x99, 0xd3, 0, 0xaa, 0, 0x4a, 0xe8, 0x37}}
	iidBrowser      = windows.GUID{Data1: 0x000214e2, Data2: 0, Data3: 0, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidFolderView2  = windows.GUID{Data1: 0x1af3a467, Data2: 0x214f, Data3: 0x4298, Data4: [8]byte{0x90, 0x8e, 0x06, 0xb0, 0x3e, 0x0b, 0x39, 0xf9}}
	iidShellItem    = windows.GUID{Data1: 0x43826d1e, Data2: 0xe718, Data3: 0x42ee, Data4: [8]byte{0xbc, 0x55, 0xa1, 0xe2, 0x61, 0xc3, 0x7b, 0xfe}}
	iidShellFolder  = windows.GUID{Data1: 0x000214e6, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
)

type comObject struct{ vtable *[64]uintptr }

func (o *comObject) call(index int, args ...interface{}) uintptr {
	all := make([]uintptr, len(args)+1)
	all[0] = uintptr(unsafe.Pointer(o))
	for i, arg := range args {
		switch v := arg.(type) {
		case int:
			all[i+1] = uintptr(v)
		case uintptr:
			all[i+1] = v
		case unsafe.Pointer:
			all[i+1] = uintptr(v)
		default:
			panic(fmt.Sprintf("unsupported COM argument %T", arg))
		}
	}
	hr, _, _ := syscall.SyscallN(o.vtable[index], all...)
	runtime.KeepAlive(args)
	runtime.KeepAlive(o)
	return hr
}
func (o *comObject) release() {
	if o != nil {
		o.call(2)
	}
}
func (o *comObject) query(iid *windows.GUID) (*comObject, error) {
	var out *comObject
	hr := o.call(0, unsafe.Pointer(iid), unsafe.Pointer(&out))
	runtime.KeepAlive(iid)
	return out, HResult("QueryInterface", hr)
}

// VARIANT occupies 24 bytes on Windows x64.
type variant struct {
	Type, Reserved1, Reserved2, Reserved3 uint16
	Value                                 uint64
	Extra                                 uint64
}

func desktopView() (*comObject, error) {
	var shell *comObject
	hr, _, _ := Ole32.NewProc("CoCreateInstance").Call(uintptr(unsafe.Pointer(&clsShellWindows)), 0, 5, uintptr(unsafe.Pointer(&iidShellWindows)), uintptr(unsafe.Pointer(&shell)))
	if err := HResult("CoCreateInstance ShellWindows", hr); err != nil {
		return nil, err
	}
	defer shell.release()
	loc := variant{Type: 3, Value: 0}
	empty := variant{}
	var hwnd int32
	var dispatch *comObject
	hr = shell.call(15, unsafe.Pointer(&loc), unsafe.Pointer(&empty), 8, unsafe.Pointer(&hwnd), 1, unsafe.Pointer(&dispatch))
	runtime.KeepAlive(&loc)
	runtime.KeepAlive(&empty)
	runtime.KeepAlive(&hwnd)
	if err := HResult("FindWindowSW bureau", hr); err != nil {
		return nil, err
	}
	if dispatch == nil {
		return nil, errors.New("Explorer : aucune vue de bureau disponible")
	}
	defer dispatch.release()
	provider, err := dispatch.query(&iidProvider)
	if err != nil {
		return nil, err
	}
	defer provider.release()
	var browser *comObject
	hr = provider.call(3, unsafe.Pointer(&sidBrowser), unsafe.Pointer(&iidBrowser), unsafe.Pointer(&browser))
	if err = HResult("QueryService ShellBrowser", hr); err != nil {
		return nil, err
	}
	defer browser.release()
	var view *comObject
	hr = browser.call(15, unsafe.Pointer(&view))
	if err = HResult("QueryActiveShellView", hr); err != nil {
		return nil, err
	}
	defer view.release()
	return view.query(&iidFolderView2)
}

func itemName(item *comObject, mode uintptr) (string, error) {
	var p *uint16
	hr := item.call(5, mode, unsafe.Pointer(&p))
	if err := HResult("IShellItem.GetDisplayName", hr); err != nil {
		return "", err
	}
	if p == nil {
		return "", errors.New("nom Shell vide")
	}
	defer Ole32.NewProc("CoTaskMemFree").Call(uintptr(unsafe.Pointer(p)))
	buf := unsafe.Slice(p, 32768)
	for n, ch := range buf {
		if ch == 0 {
			return windows.UTF16ToString(buf[:n]), nil
		}
	}
	return "", errors.New("nom Shell trop long")
}

type desktopEntry struct {
	icon model.Icon
	pidl unsafe.Pointer
}

func entries(view *comObject) ([]desktopEntry, error) {
	var folder *comObject
	hr := view.call(5, unsafe.Pointer(&iidShellFolder), unsafe.Pointer(&folder))
	if err := HResult("IFolderView.GetFolder", hr); err != nil {
		return nil, err
	}
	defer folder.release()
	var count int32
	hr = view.call(7, 2, unsafe.Pointer(&count))
	if err := HResult("IFolderView.ItemCount", hr); err != nil {
		return nil, err
	}
	if count < 0 || count > 100000 {
		return nil, errors.New("nombre d'icônes invalide")
	}
	out := make([]desktopEntry, 0, count)
	success := false
	defer func() {
		if !success {
			freeEntries(out)
		}
	}()
	for i := int32(0); i < count; i++ {
		var pidl unsafe.Pointer
		hr = view.call(6, uintptr(i), unsafe.Pointer(&pidl))
		if err := HResult("IFolderView.Item", hr); err != nil {
			return nil, err
		}
		out = append(out, desktopEntry{pidl: pidl})
		var item *comObject
		hr, _, _ = Shell32.NewProc("SHCreateItemWithParent").Call(0, uintptr(unsafe.Pointer(folder)), uintptr(pidl), uintptr(unsafe.Pointer(&iidShellItem)), uintptr(unsafe.Pointer(&item)))
		if err := HResult("SHCreateItemWithParent", hr); err != nil {
			return nil, err
		}
		id, idErr := itemName(item, 0x80028000)
		name, nameErr := itemName(item, 0)
		item.release()
		if err := errors.Join(idErr, nameErr); err != nil {
			return nil, err
		}
		out[len(out)-1].icon = model.Icon{Identity: id, Name: name}
		hr = view.call(11, pidl, unsafe.Pointer(&out[len(out)-1].icon.Position))
		if err := HResult("IFolderView.GetItemPosition", hr); err != nil {
			return nil, err
		}
	}
	runtime.KeepAlive(out)
	success = true
	return out, nil
}

func freeEntries(es []desktopEntry) {
	for _, e := range es {
		if e.pidl != nil {
			Ole32.NewProc("CoTaskMemFree").Call(uintptr(e.pidl))
		}
	}
}

type Desktop struct{}

const (
	fwfAutoArrange = 0x1
	fwfSnapToGrid  = 0x4
)

func desktopOptions(view *comObject) (model.DesktopOptions, error) {
	var flags uint32
	hr := view.call(25, unsafe.Pointer(&flags))
	if err := HResult("GetCurrentFolderFlags", hr); err != nil {
		return model.DesktopOptions{}, err
	}
	return model.DesktopOptions{AutoArrange: flags&fwfAutoArrange != 0, SnapToGrid: flags&fwfSnapToGrid != 0}, nil
}

func desktopIcons(view *comObject) ([]model.Icon, error) {
	es, err := entries(view)
	if err != nil {
		return nil, err
	}
	defer freeEntries(es)
	icons := make([]model.Icon, len(es))
	for i, e := range es {
		icons[i] = e.icon
	}
	return icons, nil
}

func captureDesktop(readOptions func() (model.DesktopOptions, error), readIcons func() ([]model.Icon, error)) (model.DesktopSnapshot, error) {
	before, err := readOptions()
	if err != nil {
		return model.DesktopSnapshot{}, err
	}
	icons, err := readIcons()
	if err != nil {
		return model.DesktopSnapshot{}, err
	}
	after, err := readOptions()
	if err != nil {
		return model.DesktopSnapshot{}, err
	}
	if before != after {
		return model.DesktopSnapshot{}, errors.New("réglages de disposition du bureau modifiés pendant la capture ; réessayez")
	}
	return model.DesktopSnapshot{Icons: icons, Options: before}, nil
}

func (Desktop) Snapshot() (model.DesktopSnapshot, error) {
	if err := Interactive(); err != nil {
		return model.DesktopSnapshot{}, err
	}
	view, err := desktopView()
	if err != nil {
		return model.DesktopSnapshot{}, err
	}
	defer view.release()
	return captureDesktop(
		func() (model.DesktopOptions, error) { return desktopOptions(view) },
		func() ([]model.Icon, error) { return desktopIcons(view) },
	)
}

func (Desktop) Icons() ([]model.Icon, error) {
	if err := Interactive(); err != nil {
		return nil, err
	}
	view, err := desktopView()
	if err != nil {
		return nil, err
	}
	defer view.release()
	return desktopIcons(view)
}

func (Desktop) Position(icons []model.Icon, savedGrid *bool) error {
	if err := Interactive(); err != nil {
		return err
	}
	view, err := desktopView()
	if err != nil {
		return err
	}
	defer view.release()
	options, err := desktopOptions(view)
	if err != nil {
		return err
	}
	if err = options.CheckPosition(savedGrid); err != nil {
		return err
	}
	es, err := entries(view)
	if err != nil {
		return err
	}
	defer freeEntries(es)
	byID := make(map[string]unsafe.Pointer)
	for _, e := range es {
		byID[e.icon.Identity] = e.pidl
	}
	return positionDesktop(icons, savedGrid, options,
		func() (model.DesktopOptions, error) { return desktopOptions(view) },
		func(icon model.Icon) error {
			pidl, ok := byID[icon.Identity]
			if !ok {
				return fmt.Errorf("icône disparue : %s", icon.Name)
			}
			p := icon.Position
			hr := view.call(16, 1, unsafe.Pointer(&pidl), unsafe.Pointer(&p), 0x80)
			runtime.KeepAlive(&pidl)
			runtime.KeepAlive(&p)
			return HResult("SelectAndPositionItems "+icon.Name, hr)
		},
	)
}

func positionDesktop(icons []model.Icon, savedGrid *bool, options model.DesktopOptions, readOptions func() (model.DesktopOptions, error), move func(model.Icon) error) error {
	if err := options.CheckPosition(savedGrid); err != nil {
		return err
	}
	var failures []error
	for _, icon := range icons {
		currentOptions, readErr := readOptions()
		if readErr != nil {
			return errors.Join(errors.Join(failures...), readErr)
		}
		if currentOptions != options {
			return errors.Join(errors.Join(failures...), errors.New("réglages de disposition du bureau modifiés pendant la restauration ; opération interrompue"))
		}
		if err := move(icon); err != nil {
			failures = append(failures, err)
		}
	}
	after, readErr := readOptions()
	if readErr != nil {
		failures = append(failures, readErr)
	} else if after != options {
		failures = append(failures, errors.New("réglages de disposition du bureau modifiés pendant la restauration"))
	}
	return errors.Join(failures...)
}
