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
	defer view.release()
	snapshot, err := (Desktop{}).Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Options.AutoArrange {
		t.Skip("réorganisation automatique active : aucun réglage modifié, aucune icône déplacée")
	}
	icons := snapshot.Icons
	if len(icons) == 0 {
		t.Skip("no desktop icons")
	}
	ms, err := (Display{}).Monitors()
	if err != nil {
		t.Fatal(err)
	}
	original := icons[0]
	moved := original
	if snapshot.Options.SnapToGrid {
		var spacing model.Point
		if err := HResult("IFolderView.GetSpacing", view.call(12, unsafe.Pointer(&spacing))); err != nil {
			t.Fatal(err)
		}
		destination, ok := freeGridDestination(original.Position, spacing, icons, ms)
		if !ok {
			t.Skip("aucune destination de grille sûre disponible ; aucun déplacement")
		}
		moved.Position = destination
	} else {
		moved.Position.X += 37
		moved.Position.Y += 19
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
	safety := model.Layout{Version: model.Version, ID: id, Kind: "safety", CapturedAt: time.Now().UTC(), Monitors: ms, Icons: icons, SnapToGrid: &snapshot.Options.SnapToGrid}
	if err := store.Save(safety); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := (Desktop{}).Position([]model.Icon{original}, safety.SnapToGrid); err != nil {
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
		positions := make(map[string]model.Point, len(actual))
		for _, icon := range actual {
			positions[icon.Identity] = icon.Position
		}
		for _, icon := range safety.Icons {
			if got, ok := positions[icon.Identity]; !ok || got != icon.Position {
				t.Errorf("icon %s changed; safety backup ID %s", icon.Name, id)
			}
		}
		options, err := desktopOptions(view)
		if err != nil {
			t.Error(err)
		} else if options != snapshot.Options {
			t.Error("desktop settings changed during integration test")
		}
	}()
	if err := (Desktop{}).Position([]model.Icon{moved}, safety.SnapToGrid); err != nil {
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
	t.Logf("Moved one icon with grid=%t; restoration verified by deferred check; retained safety backup ID %s", snapshot.Options.SnapToGrid, id)
}

func freeGridDestination(origin, spacing model.Point, icons []model.Icon, monitors []model.Monitor) (model.Point, bool) {
	// Restrict the consented test to unambiguous Shell coordinates on one primary screen.
	if spacing.X <= 0 || spacing.Y <= 0 || len(monitors) != 1 || monitors[0].Left != 0 || monitors[0].Top != 0 || !monitors[0].Primary {
		return model.Point{}, false
	}

	for dy := int64(-2); dy <= 2; dy++ {
		for dx := int64(-2); dx <= 2; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			x, y := int64(origin.X)+dx*int64(spacing.X), int64(origin.Y)+dy*int64(spacing.Y)
			if x < int64(spacing.X) || y < int64(spacing.Y) || x+int64(spacing.X) >= int64(monitors[0].Width) || y+int64(spacing.Y) >= int64(monitors[0].Height) {
				continue
			}
			occupied := false
			for _, icon := range icons {
				diffX, diffY := x-int64(icon.Position.X), y-int64(icon.Position.Y)
				if diffX > -int64(spacing.X) && diffX < int64(spacing.X) && diffY > -int64(spacing.Y) && diffY < int64(spacing.Y) {
					occupied = true
					break
				}
			}
			if !occupied {
				return model.Point{X: int32(x), Y: int32(y)}, true
			}
		}
	}
	return model.Point{}, false
}
