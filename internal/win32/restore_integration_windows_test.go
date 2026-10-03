//go:build windows && amd64

package win32

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/Azimut-Studio/smart-desktop/internal/model"
	"github.com/Azimut-Studio/smart-desktop/internal/storage"
)

func TestDesktopRestoreConsentedIntegration(t *testing.T) {
	if os.Getenv("SMART_DESKTOP_ALLOW_MOVE") != "1" {
		t.Skip("requires explicit consent to move one desktop icon")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := InitializeCOM(); err != nil {
		t.Fatal(err)
	}
	defer UninitializeCOM()
	view, err := desktopView()
	if err != nil {
		t.Fatal(err)
	}
	var flags uint32
	hr := view.call(25, unsafe.Pointer(&flags))
	view.release()
	if err := HResult("GetCurrentFolderFlags", hr); err != nil {
		t.Fatal(err)
	}
	if flags&5 != 0 {
		t.Skip("arrangement automatique ou alignement sur grille actif : aucun réglage modifié, aucune icône déplacée")
	}
	icons, err := (Desktop{}).Icons()
	if err != nil {
		t.Fatal(err)
	}
	if len(icons) == 0 {
		t.Skip("no desktop icons")
	}
	ms, err := (Display{}).Monitors()
	if err != nil {
		t.Fatal(err)
	}
	id, err := model.NewID()
	if err != nil {
		t.Fatal(err)
	}
	root := os.Getenv("LOCALAPPDATA")
	if root == "" {
		t.Fatal("LOCALAPPDATA unavailable")
	}
	store, err := storage.Open(filepath.Join(root, "SmartDesktop"))
	if err != nil {
		t.Fatal(err)
	}
	safety := model.Layout{Version: 1, ID: id, Kind: "safety", CapturedAt: time.Now().UTC(), Monitors: ms, Icons: icons}
	if err := store.Save(safety); err != nil {
		t.Fatal(err)
	}
	original := icons[0]
	moved := original
	moved.Position.X += 37
	moved.Position.Y += 19
	defer func() {
		if err := (Desktop{}).Position([]model.Icon{original}); err != nil {
			t.Errorf("restore original icon failed; safety backup ID %s: %v", id, err)
		}
		actual, err := (Desktop{}).Icons()
		if err != nil {
			t.Error(err)
			return
		}
		found := false
		for _, icon := range actual {
			if icon.Identity == original.Identity {
				found = true
				if icon.Position != original.Position {
					t.Errorf("original position not restored; safety backup ID %s", id)
				}
			}
		}
		if !found {
			t.Error("original icon no longer present")
		}
	}()
	if err := (Desktop{}).Position([]model.Icon{moved}); err != nil {
		t.Fatal(err)
	}
	actual, err := (Desktop{}).Icons()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, icon := range actual {
		if icon.Identity == moved.Identity {
			found = true
			if icon.Position != moved.Position {
				t.Fatal("temporary position not applied exactly")
			}
		}
	}
	if !found {
		t.Fatal("icon no longer present")
	}
	t.Logf("Moved one icon; restoration verified by deferred check; retained safety backup ID %s", id)
}
