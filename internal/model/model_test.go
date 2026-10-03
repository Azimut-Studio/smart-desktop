package model

import (
	"strings"
	"testing"
	"time"
)

func testMonitors() []Monitor {
	return []Monitor{
		{Identity: "a", Width: 1920, Height: 1080, DPIX: 96, DPIY: 96, Primary: true},
		{Identity: "b", Left: -2560, Width: 2560, Height: 1440, DPIX: 144, DPIY: 144},
	}
}
func TestCompatible(t *testing.T) {
	a := testMonitors()
	b := []Monitor{a[1], a[0]}
	b[0].Name = "renamed"
	if !Compatible(a, b) {
		t.Fatal("enumeration order and label must not matter")
	}
	for _, mutate := range []func(*Monitor){
		func(m *Monitor) { m.Left++ }, func(m *Monitor) { m.Width++ },
		func(m *Monitor) { m.DPIX++ }, func(m *Monitor) { m.Orientation++ },
		func(m *Monitor) { m.Identity = "other" }, func(m *Monitor) { m.Primary = false },
	} {
		b = append([]Monitor(nil), a...)
		mutate(&b[0])
		if Compatible(a, b) {
			t.Fatal("changed configuration accepted")
		}
	}
	if Compatible(nil, nil) {
		t.Fatal("empty configurations accepted")
	}
}
func TestSettingsBounds(t *testing.T) {
	s := Defaults()
	if s.Tray || s.Startup || s.Automatic || s.Background || s.IntervalSeconds != 5 || s.AutomaticLimit != 100 {
		t.Fatal(s)
	}
	for _, n := range []int{0, -1, 3601} {
		s.IntervalSeconds = n
		if s.Validate() == nil {
			t.Fatal(n)
		}
	}
	for _, n := range []int{1, 3600} {
		s.IntervalSeconds = n
		if s.Validate() != nil {
			t.Fatal(n)
		}
	}
	s = Defaults()
	for _, n := range []int{0, 10001} {
		s.AutomaticLimit = n
		if s.Validate() == nil {
			t.Fatal(n)
		}
	}
}
func TestLayoutValidationAndSorting(t *testing.T) {
	l := Layout{Version: Version, ID: strings.Repeat("a", 32), CapturedAt: time.Now().UTC(), Kind: "manual", Monitors: testMonitors(), Icons: []Icon{{Identity: "desktop-item", Position: Point{-10, 5}}}}
	if err := l.Validate(); err != nil {
		t.Fatal(err)
	}
	l.Icons = append(l.Icons, l.Icons[0])
	if l.Validate() == nil {
		t.Fatal("duplicate icon accepted")
	}
	l.Icons = l.Icons[:1]
	l.Kind = "automatic"
	if l.Validate() == nil {
		t.Fatal("automatic without detection accepted")
	}
	now := l.CapturedAt.Add(time.Second)
	l.DetectedAt = &now
	if l.Validate() != nil {
		t.Fatal("valid automatic rejected")
	}
	a, b := l, l
	b.ID = strings.Repeat("b", 32)
	ls := []Layout{a, b}
	SortLayouts(ls)
	if ls[0].ID != b.ID {
		t.Fatal("tie not deterministic")
	}
	a.CapturedAt = a.CapturedAt.Add(time.Hour)
	ls = []Layout{b, a}
	SortLayouts(ls)
	if ls[0].ID != a.ID {
		t.Fatal("not newest first")
	}
}
func TestIDs(t *testing.T) {
	id, err := NewID()
	if err != nil || !ValidID(id) {
		t.Fatal(id, err)
	}

	for _, id := range []string{"..\\settings", strings.Repeat("A", 32), "", strings.Repeat("z", 32)} {
		if ValidID(id) {
			t.Fatal(id)
		}
	}
}

func TestDesktopPositionPolicy(t *testing.T) {
	for _, auto := range []bool{false, true} {
		for _, grid := range []bool{false, true} {
			for _, saved := range []*bool{nil, boolValue(false), boolValue(true)} {
				options := DesktopOptions{AutoArrange: auto, SnapToGrid: grid}
				wantAllowed := !auto && (!grid || (saved != nil && *saved))
				if err := options.CheckPosition(saved); (err == nil) != wantAllowed {
					t.Fatalf("auto=%t grid=%t saved=%v: %v", auto, grid, saved, err)
				}
			}
		}
	}
}

func boolValue(value bool) *bool { return &value }

func TestPhysicalTargetsAndClones(t *testing.T) {
	a := testMonitors()
	a[0].Targets = []Target{
		{Identity: "hdmi", Name: "HDMI", Width: 1920, Height: 1080},
		{Identity: "displayport", Name: "DP", Width: 2560, Height: 1440},
	}
	b := append([]Monitor(nil), a...)
	b[0].Targets = []Target{a[0].Targets[1], a[0].Targets[0]}
	b[0].Targets[0].Name = "label"
	if !Compatible(a, b) {
		t.Fatal("physical target enumeration order matters")
	}
	b[0].Targets[0].Width++
	if Compatible(a, b) {
		t.Fatal("changed physical resolution accepted")
	}
	b[0].Targets = []Target{a[0].Targets[0]}
	if Compatible(a, b) {
		t.Fatal("disconnected cloned target accepted")
	}
}
