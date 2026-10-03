//go:build windows && amd64

package win32

import (
	"testing"
	"unsafe"
)

func TestWindowsABISizes(t *testing.T) {
	for name, got := range map[string]uintptr{"VARIANT": unsafe.Sizeof(variant{}), "MONITORINFOEXW": unsafe.Sizeof(monitorInfo{}), "DISPLAY_DEVICEW": unsafe.Sizeof(displayDevice{}), "DEVMODEW": unsafe.Sizeof(devMode{}), "MSG": unsafe.Sizeof(Message{})} {
		want := map[string]uintptr{"VARIANT": 24, "MONITORINFOEXW": 104, "DISPLAY_DEVICEW": 840, "DEVMODEW": 220, "MSG": 48}[name]
		if got != want {
			t.Fatalf("%s: got %d want %d", name, got, want)
		}
		for name, got := range map[string]uintptr{"PATH": unsafe.Sizeof(displayPath{}), "MODE": unsafe.Sizeof(displayMode{}), "SOURCE_NAME": unsafe.Sizeof(sourceName{}), "TARGET_NAME": unsafe.Sizeof(targetName{})} {
			want := map[string]uintptr{"PATH": 72, "MODE": 64, "SOURCE_NAME": 84, "TARGET_NAME": 420}[name]
			if got != want {
				t.Fatalf("%s: got %d want %d", name, got, want)
			}
		}
	}
}
