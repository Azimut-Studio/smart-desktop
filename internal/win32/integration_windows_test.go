//go:build windows && amd64

package win32

import (
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/Azimut-Studio/smart-desktop/internal/model"
)

func TestDesktopReadOnlyIntegration(t *testing.T) {
	if os.Getenv("SMART_DESKTOP_INTEGRATION") != "1" {
		t.Skip("requires an interactive Windows desktop; read-only")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := InitializeCOM(); err != nil {
		t.Fatal(err)
	}
	defer UninitializeCOM()
	ms, err := (Display{}).Monitors()
	if err != nil {
		t.Fatal(err)
	}
	icons, err := (Desktop{}).Icons()
	if err != nil {
		t.Fatal(err)
	}
	id, err := model.NewID()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Read %d monitors and %d icons via Shell; no positions changed", len(ms), len(icons))
	for _, icon := range icons {
		if icon.Identity == "" {
			t.Fatal("missing identity")
		}
	}
	l := model.Layout{Version: model.Version, ID: id, Kind: "manual", CapturedAt: time.Now().UTC(), Monitors: ms, Icons: icons}
	if err := l.Validate(); err != nil {
		t.Fatal(err)
	}
}
