//go:build windows && amd64

package win32

import (
	"testing"

	"github.com/Azimut-Studio/smart-desktop/internal/model"
)

func TestFreeGridDestination(t *testing.T) {
	origin, spacing := model.Point{X: 200, Y: 200}, model.Point{X: 80, Y: 90}
	monitors := []model.Monitor{{Width: 1920, Height: 1080, Primary: true}}
	icons := []model.Icon{{Identity: "original", Position: origin}}
	got, ok := freeGridDestination(origin, spacing, icons, monitors)
	if !ok || got == origin || (got.X-origin.X)%spacing.X != 0 || (got.Y-origin.Y)%spacing.Y != 0 {
		t.Fatal("destination not on original grid", got, ok)
	}
	icons = append(icons, model.Icon{Identity: "occupied", Position: got})
	next, ok := freeGridDestination(origin, spacing, icons, monitors)
	if !ok || next == got {
		t.Fatal("occupied destination reused", next, ok)
	}
	for _, ms := range [][]model.Monitor{
		nil, {monitors[0], monitors[0]}, {{Width: 1920, Height: 1080, Left: -1920, Primary: true}},
		{{Width: 1920, Height: 1080, Primary: false}}, {{Width: 80, Height: 90, Primary: true}},
	} {
		if _, ok := freeGridDestination(origin, spacing, icons, ms); ok {
			t.Fatal("unsafe geometry accepted", ms)
		}
	}
	if _, ok := freeGridDestination(origin, model.Point{}, icons, monitors); ok {
		t.Fatal("invalid spacing accepted")
	}
	var filled []model.Icon
	for dy := int32(-2); dy <= 2; dy++ {
		for dx := int32(-2); dx <= 2; dx++ {
			filled = append(filled, model.Icon{Position: model.Point{X: origin.X + dx*spacing.X, Y: origin.Y + dy*spacing.Y}})
		}
	}
	if _, ok := freeGridDestination(origin, spacing, filled, monitors); ok {
		t.Fatal("occupied grid accepted")
	}
}
