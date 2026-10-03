package app

import (
	"errors"
	"fmt"
	"time"

	"github.com/Azimut-Studio/smart-desktop/internal/model"
	"github.com/Azimut-Studio/smart-desktop/internal/storage"
)

type Desktop interface {
	Snapshot() (model.DesktopSnapshot, error)
	Icons() ([]model.Icon, error)
	Position([]model.Icon, *bool) error
}
type Display interface {
	Monitors() ([]model.Monitor, error)
}

type App struct {
	Desktop Desktop
	Display Display
	Store   *storage.Store
	Now     func() time.Time
}

type Report struct{ Restored, Missing, New, Failed int }

func (r Report) String() string {
	return fmt.Sprintf("Restaurées : %d ; absentes : %d ; nouvelles inchangées : %d ; échecs : %d.", r.Restored, r.Missing, r.New, r.Failed)
}

func (a *App) Capture(kind string) (model.Layout, error) {
	l, _, err := a.capture(kind)
	return l, err
}

func (a *App) capture(kind string) (model.Layout, model.DesktopOptions, error) {
	var l model.Layout
	var options model.DesktopOptions
	before, err := a.Display.Monitors()
	if err != nil {
		return l, options, err
	}
	snapshot, err := a.Desktop.Snapshot()
	if err != nil {
		return l, options, err
	}
	options = snapshot.Options
	after, err := a.Display.Monitors()
	if err != nil {
		return l, options, err
	}
	if !model.Compatible(before, after) {
		return l, options, errors.New("configuration d'affichage modifiée pendant la capture ; réessayez")
	}
	id, err := model.NewID()
	if err != nil {
		return l, options, err
	}
	l = model.Layout{Version: model.Version, ID: id, Kind: kind, CapturedAt: a.Now().UTC(), Monitors: before, Icons: snapshot.Icons, SnapToGrid: &snapshot.Options.SnapToGrid}
	return l, options, l.Validate()
}

func (a *App) Save() (model.Layout, error) {
	l, err := a.Capture("manual")
	if err != nil {
		return l, err
	}
	return l, a.Store.Save(l)
}

func (a *App) Restore(id string) (Report, error) {
	var r Report
	target, err := a.Store.Load(id)
	if err != nil {
		return r, err
	}
	current, options, err := a.capture("safety")
	if err != nil {
		return r, err
	}
	if !model.Compatible(target.Monitors, current.Monitors) {
		return r, fmt.Errorf("restauration refusée : écrans, géométrie, écran principal, orientation ou DPI différents.\nSauvegarde : %s\nActuellement : %s", model.DisplaySummary(target.Monitors), model.DisplaySummary(current.Monitors))
	}
	if err = options.CheckPosition(target.SnapToGrid); err != nil {
		return r, err
	}
	if err = a.Store.Save(current); err != nil {
		return r, fmt.Errorf("sauvegarde de sécurité impossible : %w", err)
	}
	present := make(map[string]model.Icon)
	for _, i := range current.Icons {
		present[i.Identity] = i
	}
	want := make(map[string]model.Point)
	var positions []model.Icon
	for _, i := range target.Icons {
		if _, ok := present[i.Identity]; !ok {
			r.Missing++
			continue
		}
		want[i.Identity] = i.Position
		positions = append(positions, i)
	}
	for _, i := range current.Icons {
		if _, ok := want[i.Identity]; !ok {
			r.New++
		}
	}
	positionErr := a.Desktop.Position(positions, target.SnapToGrid)
	after, err := a.Desktop.Icons()
	if err != nil {
		return r, errors.Join(positionErr, fmt.Errorf("vérification impossible : %w", err))
	}
	actual := make(map[string]model.Point)
	for _, i := range after {
		actual[i.Identity] = i.Position
	}
	for identity, p := range want {
		if got, ok := actual[identity]; ok && got == p {
			r.Restored++
		} else {
			r.Failed++
		}
	}
	monitors, monitorErr := a.Display.Monitors()
	if monitorErr == nil && !model.Compatible(target.Monitors, monitors) {
		monitorErr = errors.New("affichage modifié pendant la restauration")
	}
	if r.Failed > 0 {
		positionErr = errors.Join(positionErr, errors.New("certaines positions n'ont pas été restaurées exactement"))
	}
	return r, errors.Join(positionErr, monitorErr)
}
