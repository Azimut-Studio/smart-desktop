package automation

import (
	"errors"
	"testing"
	"time"

	"github.com/Azimut-Studio/smart-desktop/internal/model"
)

func cp(t *testing.T, width int32, at time.Time) model.Layout {
	t.Helper()
	id, err := model.NewID()
	if err != nil {
		t.Fatal(err)
	}
	return model.Layout{Version: 1, ID: id, Kind: "checkpoint", CapturedAt: at, Monitors: []model.Monitor{{Identity: "screen", Width: width, Height: 1080, DPIX: 96, DPIY: 96, Primary: true}}, Icons: []model.Icon{{Identity: "a", Position: model.Point{X: 42}}}}
}
func TestArchiveOldCheckpointOnceThenStabilize(t *testing.T) {
	now := time.Now().UTC()
	old := cp(t, 1920, now)
	next := cp(t, 1280, now.Add(time.Second))
	oldGrid, nextGrid := true, false
	old.SnapToGrid, next.SnapToGrid = &oldGrid, &nextGrid
	o := Observer{Checkpoint: &old}
	var archives []model.Layout
	captures := 0
	capture := func() (model.Layout, error) { captures++; return next, nil }
	persist := func(model.Layout) error { return nil }
	archive := func(l model.Layout) error { archives = append(archives, l); return nil }
	for _, delay := range []time.Duration{time.Second, 1500 * time.Millisecond, 2 * time.Second} {
		if err := o.Step(now.Add(delay), next.Monitors, capture, persist, archive); err != nil {
			t.Fatal(err)
		}
	}
	if len(archives) != 1 || captures != 0 {
		t.Fatal(len(archives), captures)
	}
	if archives[0].SnapToGrid == nil || !*archives[0].SnapToGrid {
		t.Fatal("archive lost original grid state")
	}
	if archives[0].Monitors[0].Width != 1920 || !archives[0].CapturedAt.Equal(old.CapturedAt) || archives[0].Kind != "automatic" || archives[0].DetectedAt == nil || archives[0].ID == old.ID {
		t.Fatal(archives[0])
	}
	if err := o.Step(now.Add(4*time.Second), next.Monitors, capture, persist, archive); err != nil {
		t.Fatal(err)
	}
	if captures != 1 || o.Checkpoint.Monitors[0].Width != 1280 {
		t.Fatal(captures, o)
	}
	if o.Checkpoint.SnapToGrid == nil || *o.Checkpoint.SnapToGrid {
		t.Fatal("new checkpoint grid not preserved")
	}
}
func TestFailuresPreserveCheckpoint(t *testing.T) {
	now := time.Now().UTC()
	old := cp(t, 1920, now)
	next := cp(t, 1280, now.Add(time.Second))
	o := Observer{Checkpoint: &old}
	boom := errors.New("disk full")
	capture := func() (model.Layout, error) { return next, nil }
	if err := o.Step(now.Add(time.Second), next.Monitors, capture, func(model.Layout) error { return nil }, func(model.Layout) error { return boom }); !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if o.Checkpoint.ID != old.ID || o.transition {
		t.Fatal("failed archive advanced observer")
	}
	o.Reset()
	if err := o.Step(now, next.Monitors, capture, func(model.Layout) error { return boom }, func(model.Layout) error { return nil }); !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if o.Checkpoint != nil {
		t.Fatal("failed checkpoint replaced last state")
	}
}
func TestCaptureConfigurationRace(t *testing.T) {
	now := time.Now().UTC()
	a, b := cp(t, 1920, now), cp(t, 1280, now)
	o := Observer{}
	writes := 0
	err := o.Step(now, a.Monitors, func() (model.Layout, error) { return b, nil }, func(model.Layout) error { writes++; return nil }, func(model.Layout) error { return nil })
	if err == nil || writes != 0 || o.Checkpoint != nil {
		t.Fatal(err, writes)
	}
}
func TestTopologyKeepsChangingAndReset(t *testing.T) {
	now := time.Now().UTC()
	a, b, c := cp(t, 1920, now), cp(t, 1280, now), cp(t, 800, now)
	o := Observer{Checkpoint: &a}
	captures := 0
	capture := func() (model.Layout, error) { captures++; return c, nil }
	persist := func(model.Layout) error { return nil }
	archives := 0
	archive := func(model.Layout) error { archives++; return nil }
	for i, ms := range [][]model.Monitor{b.Monitors, c.Monitors, c.Monitors} {
		if err := o.Step(now.Add(time.Duration(i+1)*time.Second), ms, capture, persist, archive); err != nil {
			t.Fatal(err)
		}
	}
	if captures != 0 || archives != 1 {
		t.Fatal(captures, archives)
	}
	o.Reset()
	if o.Checkpoint != nil || o.transition {
		t.Fatal(o)
	}
}
