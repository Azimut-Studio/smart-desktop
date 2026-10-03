//go:build windows && amd64

package win32

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	User32   = windows.NewLazySystemDLL("user32.dll")
	Shell32  = windows.NewLazySystemDLL("shell32.dll")
	Ole32    = windows.NewLazySystemDLL("ole32.dll")
	Kernel32 = windows.NewLazySystemDLL("kernel32.dll")
)

func Text(s string) *uint16 { return windows.StringToUTF16Ptr(s) }

func CheckBOOL(name string, value uintptr, err error) error {
	if value != 0 {
		return nil
	}
	if err == nil || err == syscall.Errno(0) {
		return fmt.Errorf("%s a échoué", name)
	}
	return fmt.Errorf("%s : %w", name, err)
}

func HResult(name string, hr uintptr) error {
	if int32(hr) < 0 {
		return fmt.Errorf("%s : HRESULT 0x%08X", name, uint32(hr))
	}
	return nil
}

type Rect struct{ Left, Top, Right, Bottom int32 }
type Message struct {
	HWND           uintptr
	ID             uint32
	WParam, LParam uintptr
	Time           uint32
	Point          struct{ X, Y int32 }
	Private        uint32
}

func Pump() {
	var m Message
	for {
		ok, _, _ := User32.NewProc("PeekMessageW").Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, 1)
		if ok == 0 {
			break
		}
		User32.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&m)))
		User32.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&m)))
	}
}

func InitializeCOM() error {
	context, _, e := User32.NewProc("SetThreadDpiAwarenessContext").Call(^uintptr(3))
	if err := CheckBOOL("SetThreadDpiAwarenessContext", context, e); err != nil {
		return err
	}
	hr, _, _ := Ole32.NewProc("CoInitializeEx").Call(0, 2)
	return HResult("CoInitializeEx", hr)
}

func UninitializeCOM() { Ole32.NewProc("CoUninitialize").Call() }

func Interactive() error {
	h, _, e := User32.NewProc("OpenInputDesktop").Call(0, 0, 0x100)
	if h == 0 {
		return CheckBOOL("Bureau interactif indisponible (session verrouillée ?)", h, e)
	}
	ok, _, e := User32.NewProc("CloseDesktop").Call(h)
	return CheckBOOL("CloseDesktop", ok, e)
}
