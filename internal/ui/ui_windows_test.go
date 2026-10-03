//go:build windows && amd64

package ui

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/Azimut-Studio/smart-desktop/internal/app"
	"github.com/Azimut-Studio/smart-desktop/internal/model"
	"github.com/Azimut-Studio/smart-desktop/internal/storage"
)

func TestGridDescription(t *testing.T) {
	on, off := true, false
	for _, tc := range []struct {
		grid *bool
		want string
	}{
		{nil, "état non enregistré"},
		{&off, "désactivé ; restauration avec grille désactivée uniquement"},
		{&on, "activé ; restauration autorisée avec ou sans grille"},
	} {
		if text := gridDescription(tc.grid); !strings.Contains(text, tc.want) {
			t.Fatal(text)
		}
	}
}

func TestNativeABISizes(t *testing.T) {
	if unsafe.Sizeof(windowClass{}) != 80 {
		t.Fatal("WNDCLASSEXW size")
	}
	if unsafe.Sizeof(notificationIcon{}) != 976 {
		t.Fatalf("NOTIFYICONDATAW size: %d", unsafe.Sizeof(notificationIcon{}))
	}
}

func TestNativeWindowSmoke(t *testing.T) {
	if os.Getenv("SMART_DESKTOP_INTEGRATION") != "1" {
		t.Skip("requires a Windows desktop; hidden window, no icon positions modified")
	}
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	settings := model.Defaults()
	worker := app.NewWorker(store, settings)
	if err := worker.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := worker.Close(); err != nil {
			t.Error(err)
		}
	}()
	className := "SmartDesktop.Integration." + time.Now().Format("150405.000000000")
	result := make(chan error, 1)
	go func() {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			h := call("FindWindowW", str(className), 0)
			if h != 0 && call("GetDlgItem", h, idLimit) != 0 {
				defer call("PostMessageW", h, 0x111, idQuit, 0)
				for _, id := range []int{idSave, idHistory, idTray, idApply, idAutomatic, idInterval, idLimit} {
					if call("GetDlgItem", h, id) == 0 {
						result <- errors.New("missing native control")
						return
					}
				}
				var text [128]uint16
				call("GetWindowTextW", h, unsafe.Pointer(&text[0]), len(text))
				if text[0] == 0 {
					result <- errors.New("missing window title")
					return
				}
				result <- nil
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		result <- errors.New("native window not created")
	}()
	if err := Run(worker, settings, className, true); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}
