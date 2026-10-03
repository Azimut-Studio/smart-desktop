package automation

import (
	"fmt"
	"time"

	"github.com/Azimut-Studio/smart-desktop/internal/model"
)

type Observer struct {
	Checkpoint *model.Layout
	pending    []model.Monitor
	since      time.Time
	transition bool
}

// Step never captures the new layout into the old checkpoint during a transition.
func (o *Observer) Step(now time.Time, monitors []model.Monitor, capture func() (model.Layout, error), persist func(model.Layout) error, archive func(model.Layout) error) error {
	if err := model.ValidateMonitors(monitors); err != nil {
		return err
	}
	if o.Checkpoint != nil && !model.Compatible(o.Checkpoint.Monitors, monitors) && !o.transition {
		old := *o.Checkpoint
		id, err := model.NewID()
		if err != nil {
			return err
		}
		old.ID = id
		old.Kind = "automatic"
		t := now.UTC()
		old.DetectedAt = &t
		if err := archive(old); err != nil {
			return fmt.Errorf("archivage de l'ancien état : %w", err)
		}
		o.transition = true
		o.pending = append([]model.Monitor(nil), monitors...)
		o.since = now
	}
	if o.transition {
		if !model.Compatible(o.pending, monitors) {
			o.pending = append([]model.Monitor(nil), monitors...)
			o.since = now
			return nil
		}
		if now.Sub(o.since) < 2*time.Second {
			return nil
		}
	}
	l, err := capture()
	if err != nil {
		return err
	}
	if !model.Compatible(monitors, l.Monitors) {
		return fmt.Errorf("affichage modifié pendant l'observation")
	}
	if err := persist(l); err != nil {
		return err
	}
	o.Checkpoint = &l
	o.transition = false
	return nil
}

func (o *Observer) Reset() { *o = Observer{} }
