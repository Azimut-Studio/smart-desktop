package storage

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Azimut-Studio/smart-desktop/internal/model"
)

func TestLayoutGridCompatibility(t *testing.T) {
	for _, state := range []string{"unknown", "off", "on"} {
		t.Run(state, func(t *testing.T) {
			s := newStore(t)
			l := layout(t, "manual", time.Now().UTC())
			if state != "unknown" {
				grid := state == "on"
				l.SnapToGrid = &grid
			}
			if err := s.Save(l); err != nil {
				t.Fatal(err)
			}
			got, err := s.Load(l.ID)
			if err != nil || (got.SnapToGrid == nil) != (l.SnapToGrid == nil) || (got.SnapToGrid != nil && *got.SnapToGrid != *l.SnapToGrid) {
				t.Fatal(got, err)
			}
			raw, err := os.ReadFile(filepath.Join(s.Root, "backups", l.ID+".json"))
			if err != nil || bytes.Contains(raw, []byte(`"snap_to_grid"`)) != (state != "unknown") {
				t.Fatal(string(raw), err)
			}
			l.Kind = "checkpoint"
			if err := s.SaveCheckpoint(l); err != nil {
				t.Fatal(err)
			}
			cp, err := s.Checkpoint()
			if err != nil || (cp.SnapToGrid == nil) != (l.SnapToGrid == nil) || (cp.SnapToGrid != nil && *cp.SnapToGrid != *l.SnapToGrid) {
				t.Fatal(cp, err)
			}
		})
	}
}

func layout(t *testing.T, kind string, at time.Time) model.Layout {
	t.Helper()
	id, err := model.NewID()
	if err != nil {
		t.Fatal(err)
	}
	l := model.Layout{Version: 1, ID: id, Kind: kind, CapturedAt: at, Monitors: []model.Monitor{{Identity: "screen", Width: 1920, Height: 1080, DPIX: 96, DPIY: 96, Primary: true}}, Icons: []model.Icon{{Identity: "file", Name: "Fichier", Position: model.Point{X: 45, Y: 67}}}}
	if kind == "automatic" {
		detected := at.Add(time.Second)
		l.DetectedAt = &detected
	}
	return l
}
func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func TestSaveLoadRetention(t *testing.T) {
	s := newStore(t)
	at := time.Now().UTC()
	var manuals []string
	var newest string
	for i, kind := range []string{"manual", "automatic", "safety", "automatic", "automatic"} {
		l := layout(t, kind, at.Add(time.Duration(i)*time.Second))
		if err := s.Save(l); err != nil {
			t.Fatal(err)
		}
		got, err := s.Load(l.ID)
		if err != nil || got.Icons[0] != l.Icons[0] {
			t.Fatal(got, err)
		}
		if kind != "automatic" {
			manuals = append(manuals, l.ID)
		} else {
			newest = l.ID
		}
	}
	if err := s.Retain(1); err != nil {
		t.Fatal(err)
	}
	all, err := s.List()
	if err != nil || len(all) != 3 {
		t.Fatal(len(all), err)
	}
	for _, id := range append(manuals, newest) {
		if _, err := s.Load(id); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Delete("..\\settings"); err == nil {
		t.Fatal("unsafe ID accepted")
	}
}
func TestCorruptionIsReportedAndNotReplaced(t *testing.T) {
	s := newStore(t)
	path := filepath.Join(s.Root, "settings.json")
	for _, content := range []string{`{"version":1}`, `{"version":999}`, `not json`, `{} {}`, `{"unexpected":true}`} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Settings(); err == nil {
			t.Fatal("accepted", content)
		}
		got, _ := os.ReadFile(path)
		if string(got) != content {
			t.Fatal("corrupt settings overwritten")
		}
	}
}
func TestDefaultsAndAtomicReplacement(t *testing.T) {
	s := newStore(t)
	v, err := s.Settings()
	if err != nil || v != model.Defaults() {
		t.Fatal(v, err)
	}
	v.Tray = true
	if err := s.SaveSettings(v); err != nil {
		t.Fatal(err)
	}
	v.Tray = false
	if err := s.SaveSettings(v); err != nil {
		t.Fatal(err)
	}
	got, err := s.Settings()
	if err != nil || got.Tray {
		t.Fatal(got, err)
	}
	dir := filepath.Join(s.Root, "destination-directory")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSON(dir, v); err == nil {
		t.Fatal("directory replaced")
	}
	files, _ := filepath.Glob(filepath.Join(s.Root, ".smart-desktop-*"))
	if len(files) != 0 {
		t.Fatal("temporary files leaked")
	}
}
func TestCheckpointAndCorruptHistory(t *testing.T) {
	s := newStore(t)
	if cp, err := s.Checkpoint(); err != nil || cp != nil {
		t.Fatal(cp, err)
	}
	l := layout(t, "checkpoint", time.Now().UTC())
	if err := s.SaveCheckpoint(l); err != nil {
		t.Fatal(err)
	}
	cp, err := s.Checkpoint()
	if err != nil || cp.ID != l.ID {
		t.Fatal(cp, err)
	}
	path := filepath.Join(s.Root, "backups", strings.Repeat("a", 32)+".json")
	if err := os.WriteFile(path, []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.List(); err == nil {
		t.Fatal("corruption hidden")
	}
	if err := s.Retain(1); err == nil {
		t.Fatal("retention ignored corruption")
	}
}
func TestLogAndStatus(t *testing.T) {
	s := newStore(t)
	if err := s.Log("test diagnostic"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Status()
	if err != nil || got != "test diagnostic" {
		t.Fatal(got, err)
	}
}
