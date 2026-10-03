//go:build windows && amd64

package win32

import (
	"errors"
	"testing"

	"github.com/Azimut-Studio/smart-desktop/internal/model"
)

func TestCaptureDesktop(t *testing.T) {
	boom := errors.New("Shell unavailable")
	for _, tc := range []struct {
		name      string
		before    model.DesktopOptions
		after     model.DesktopOptions
		optionErr int
		iconErr   error
		wantErr   bool
	}{
		{name: "free"},
		{name: "grid", before: model.DesktopOptions{SnapToGrid: true}, after: model.DesktopOptions{SnapToGrid: true}},
		{name: "grid changed", after: model.DesktopOptions{SnapToGrid: true}, wantErr: true},
		{name: "arrangement changed", after: model.DesktopOptions{AutoArrange: true}, wantErr: true},
		{name: "first flags failure", optionErr: 1, wantErr: true},
		{name: "last flags failure", optionErr: 2, wantErr: true},
		{name: "icons failure", iconErr: boom, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			got, err := captureDesktop(func() (model.DesktopOptions, error) {
				calls++
				if calls == tc.optionErr {
					return model.DesktopOptions{}, boom
				}

				if calls == 1 {
					return tc.before, nil
				}
				return tc.after, nil
			}, func() ([]model.Icon, error) {
				return []model.Icon{{Identity: "icon"}}, tc.iconErr
			})
			if (err != nil) != tc.wantErr {
				t.Fatal(got, err)
			}
			if err == nil && (got.Options != tc.before || len(got.Icons) != 1) {
				t.Fatal(got)
			}
			if (tc.optionErr > 0 || tc.iconErr != nil) && !errors.Is(err, boom) {
				t.Fatal("Shell error hidden", err)
			}
			if err != nil && len(got.Icons) != 0 {
				t.Fatal("failed capture returned icons")
			}
		})
	}
}

func TestPositionDesktopGuards(t *testing.T) {
	on, off := true, false
	boom := errors.New("Shell rejected operation")
	for _, tc := range []struct {
		name      string
		options   model.DesktopOptions
		savedGrid *bool
		changeAt  int
		readErrAt int
		moveErr   error
		wantMoves int
		wantErr   bool
	}{
		{name: "free", wantMoves: 2},
		{name: "grid allowed", options: model.DesktopOptions{SnapToGrid: true}, savedGrid: &on, wantMoves: 2},
		{name: "grid unknown", options: model.DesktopOptions{SnapToGrid: true}, wantErr: true},
		{name: "grid not captured", options: model.DesktopOptions{SnapToGrid: true}, savedGrid: &off, wantErr: true},
		{name: "arrangement blocked", options: model.DesktopOptions{AutoArrange: true}, savedGrid: &on, wantErr: true},
		{name: "changed before first move", changeAt: 1, wantErr: true},
		{name: "changed between moves", changeAt: 2, wantMoves: 1, wantErr: true},
		{name: "changed after moves", changeAt: 3, wantMoves: 2, wantErr: true},
		{name: "flags unavailable", readErrAt: 1, wantErr: true},
		{name: "final flags unavailable", readErrAt: 3, wantMoves: 2, wantErr: true},
		{name: "move failure", moveErr: boom, wantMoves: 2, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reads, moves := 0, 0
			err := positionDesktop([]model.Icon{{Identity: "a"}, {Identity: "b"}}, tc.savedGrid, tc.options,
				func() (model.DesktopOptions, error) {
					reads++
					if reads == tc.readErrAt {
						return model.DesktopOptions{}, boom
					}
					options := tc.options
					if reads == tc.changeAt {
						options.AutoArrange = !options.AutoArrange
					}
					return options, nil
				},
				func(model.Icon) error { moves++; return tc.moveErr },
			)
			if (err != nil) != tc.wantErr || moves != tc.wantMoves {
				t.Fatal(moves, err)
			}
			if (tc.readErrAt > 0 || tc.moveErr != nil) && !errors.Is(err, boom) {
				t.Fatal("Shell error hidden", err)
			}
		})
	}
}
