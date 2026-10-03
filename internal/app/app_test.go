package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Azimut-Studio/smart-desktop/internal/model"
	"github.com/Azimut-Studio/smart-desktop/internal/storage"
)

type fakeDesktop struct {
	icons                []model.Icon
	readErr, positionErr error
	calls                int
	ignore               bool
	options              model.DesktopOptions
	savedGrid            *bool
	beforePosition       func()
}

func (d *fakeDesktop) Snapshot() (model.DesktopSnapshot, error) {
	icons, err := d.Icons()
	return model.DesktopSnapshot{Icons: icons, Options: d.options}, err
}

func (d *fakeDesktop) Icons() ([]model.Icon, error) {
	return append([]model.Icon(nil), d.icons...), d.readErr
}
func (d *fakeDesktop) Position(want []model.Icon, savedGrid *bool) error {
	d.calls++
	d.savedGrid = savedGrid
	if d.beforePosition != nil {
		d.beforePosition()
	}
	if err := d.options.CheckPosition(savedGrid); err != nil {
		return err
	}
	if !d.ignore {
		for i := range d.icons {
			for _, p := range want {
				if d.icons[i].Identity == p.Identity {
					d.icons[i].Position = p.Position
				}
			}
		}
	}
	return d.positionErr
}

type fakeDisplay struct {
	monitors []model.Monitor
	calls    int
	changeAt int
	err      error
}

func (d *fakeDisplay) Monitors() ([]model.Monitor, error) {
	d.calls++
	ms := append([]model.Monitor(nil), d.monitors...)
	if d.changeAt > 0 && d.calls >= d.changeAt {
		ms[0].Width++
	}
	return ms, d.err
}
func fixture(t *testing.T) (*App, *fakeDesktop, *fakeDisplay) {
	t.Helper()
	s, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	d := &fakeDesktop{icons: []model.Icon{{Identity: "a", Position: model.Point{X: 10, Y: 20}}, {Identity: "b", Position: model.Point{X: 30, Y: 40}}}}
	m := &fakeDisplay{monitors: []model.Monitor{{Identity: "screen", Width: 1920, Height: 1080, DPIX: 96, DPIY: 96, Primary: true}}}
	return &App{Desktop: d, Display: m, Store: s, Now: time.Now}, d, m
}
func TestRestoreRoundTripAndReport(t *testing.T) {
	a, d, _ := fixture(t)
	l, err := a.Save()
	if err != nil {
		t.Fatal(err)
	}
	d.icons = []model.Icon{{Identity: "a", Position: model.Point{X: 999}}, {Identity: "new", Position: model.Point{X: 55}}}
	r, err := a.Restore(l.ID)
	if err != nil || r != (Report{Restored: 1, Missing: 1, New: 1}) {
		t.Fatal(r, err)
	}
	if d.icons[1].Position.X != 55 {
		t.Fatal("new icon moved")
	}
	ls, err := a.Store.List()
	if err != nil || len(ls) != 2 || ls[0].Kind != "safety" {
		t.Fatal(ls, err)
	}
}
func TestRestoreRefusesMismatch(t *testing.T) {
	a, d, m := fixture(t)
	l, err := a.Save()
	if err != nil {
		t.Fatal(err)
	}
	m.monitors[0].DPIX = 144
	if _, err = a.Restore(l.ID); err == nil || d.calls != 0 {
		t.Fatal(err, d.calls)
	}
}
func TestCaptureRaceAndDesktopFailure(t *testing.T) {
	a, d, m := fixture(t)
	m.changeAt = 2
	if _, err := a.Save(); err == nil {
		t.Fatal("display race accepted")
	}
	m.changeAt = 0
	d.readErr = errors.New("Explorer unavailable")
	if _, err := a.Save(); err == nil {
		t.Fatal("desktop error hidden")
	}
}
func TestSafetyFailurePreventsRestore(t *testing.T) {
	a, d, _ := fixture(t)
	l, err := a.Save()
	if err != nil {
		t.Fatal(err)
	}
	a.Store.Root = filepath.Join(t.TempDir(), "missing")
	// Keep the target accessible, but make writing the safety backup fail.
	if err := os.MkdirAll(filepath.Join(a.Store.Root, "backups"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.Save(l); err != nil {
		t.Fatal(err)
	}
	calls := 0
	a.Now = func() time.Time {
		calls++
		if calls == 1 {
			os.Rename(filepath.Join(a.Store.Root, "backups"), filepath.Join(a.Store.Root, "moved"))
		}
		return time.Now()
	}
	if _, err := a.Restore(l.ID); err == nil || d.calls != 0 {
		t.Fatal(err, d.calls)
	}
}
func TestPartialRestoreIsNotSuccess(t *testing.T) {
	a, d, _ := fixture(t)
	l, err := a.Save()
	if err != nil {
		t.Fatal(err)
	}

	d.icons[0].Position.X = 100
	d.ignore = true
	r, err := a.Restore(l.ID)
	if err == nil || r.Failed != 1 || r.Restored != 1 {
		t.Fatal(r, err)
	}
	d.positionErr = errors.New("Shell rejected operation")
	if _, err := a.Restore(l.ID); err == nil {
		t.Fatal("HRESULT hidden")
	}
}

func TestRestoreGridMatrix(t *testing.T) {
	for _, auto := range []bool{false, true} {
		for _, grid := range []bool{false, true} {
			for _, state := range []string{"unknown", "off", "on"} {
				t.Run(fmt.Sprintf("auto=%t/grid=%t/saved=%s", auto, grid, state), func(t *testing.T) {
					a, d, _ := fixture(t)
					l, err := a.Save()
					if err != nil {
						t.Fatal(err)
					}
					l.SnapToGrid = nil
					if state != "unknown" {
						saved := state == "on"
						l.SnapToGrid = &saved
					}
					if err := a.Store.Save(l); err != nil {
						t.Fatal(err)
					}
					d.options = model.DesktopOptions{AutoArrange: auto, SnapToGrid: grid}
					d.icons[0].Position.X++
					allowed := !auto && (!grid || state == "on")
					r, err := a.Restore(l.ID)
					if (err == nil) != allowed {
						t.Fatal(r, err)
					}
					if !allowed {
						if d.calls != 0 || d.icons[0].Position.X != l.Icons[0].Position.X+1 {
							t.Fatal("blocked restore moved icons")
						}
						return
					}
					if r.Restored != 2 || d.calls != 1 {
						t.Fatal(r, d.calls)
					}
					if (d.savedGrid == nil) != (l.SnapToGrid == nil) || (d.savedGrid != nil && *d.savedGrid != *l.SnapToGrid) {
						t.Fatal("target grid provenance not forwarded")
					}
					layouts, err := a.Store.List()
					if err != nil || len(layouts) != 2 || layouts[0].SnapToGrid == nil || *layouts[0].SnapToGrid != grid {
						t.Fatal("safety grid not captured", layouts, err)
					}
				})
			}
		}
	}
}

func TestCaptureGridAndLateChanges(t *testing.T) {
	for _, grid := range []bool{false, true} {
		a, d, _ := fixture(t)
		d.options.SnapToGrid = grid
		for _, kind := range []string{"manual", "safety", "checkpoint"} {
			l, err := a.Capture(kind)
			if err != nil || l.SnapToGrid == nil || *l.SnapToGrid != grid {
				t.Fatal(l, err)
			}
		}
	}
	for _, change := range []model.DesktopOptions{{SnapToGrid: true}, {AutoArrange: true}} {
		a, d, _ := fixture(t)
		l, err := a.Save()
		if err != nil {
			t.Fatal(err)
		}
		d.icons[0].Position.X++
		d.beforePosition = func() { d.options = change }
		r, err := a.Restore(l.ID)
		if err == nil || r.Failed != 1 || d.icons[0].Position.X != l.Icons[0].Position.X+1 {
			t.Fatal("late option change accepted", r, err)
		}
	}
	a, d, _ := fixture(t)
	d.options.SnapToGrid = true
	l, err := a.Save()
	if err != nil {
		t.Fatal(err)
	}
	d.icons[0].Position.X++
	d.ignore = true
	if r, err := a.Restore(l.ID); err == nil || r.Failed != 1 {
		t.Fatal("grid correction accepted as success", r, err)
	}
}
