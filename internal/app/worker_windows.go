//go:build windows && amd64

package app

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/Azimut-Studio/smart-desktop/internal/automation"
	"github.com/Azimut-Studio/smart-desktop/internal/model"
	"github.com/Azimut-Studio/smart-desktop/internal/startup"
	"github.com/Azimut-Studio/smart-desktop/internal/storage"
	"github.com/Azimut-Studio/smart-desktop/internal/win32"
)

type Job func(*Worker)

type Worker struct {
	App            *App
	Settings       model.Settings
	observer       automation.Observer
	jobs           chan Job
	display        chan struct{}
	stop           chan struct{}
	done           chan struct{}
	next           time.Time
	lastError      string
	Notify         func(string)
	historyChanged func()
	notifyMu       sync.Mutex
	once           sync.Once
}

func NewWorker(store *storage.Store, settings model.Settings) *Worker {
	return &Worker{
		App:      &App{Desktop: win32.Desktop{}, Display: win32.Display{}, Store: store, Now: time.Now},
		Settings: settings, jobs: make(chan Job, 16), display: make(chan struct{}, 1),
		stop: make(chan struct{}), done: make(chan struct{}),
	}
}

func (w *Worker) Start() error {
	ready := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(w.done)
		if err := win32.InitializeCOM(); err != nil {
			ready <- err
			return
		}
		defer win32.UninitializeCOM()
		if w.Settings.Automatic {
			cp, err := w.App.Store.Checkpoint()
			if err != nil {
				ready <- err
				return
			}
			w.observer.Checkpoint = cp
		}
		ready <- nil
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-w.stop:
				return
			case job := <-w.jobs:
				job(w)
			case <-w.display:
				if w.Settings.Automatic && w.observer.Checkpoint == nil {
					w.Error(errors.New("changement d'affichage : aucun checkpoint antérieur disponible ; impossible de sauvegarder l'ancien état"))
				}
				w.next = time.Time{}
			case <-ticker.C:
				win32.Pump()
				if w.Settings.Automatic && !time.Now().Before(w.next) {
					w.observe()
					w.next = time.Now().Add(time.Duration(w.Settings.IntervalSeconds) * time.Second)
				}
			}
		}
	}()
	return <-ready
}

func (w *Worker) Submit(job Job) error {
	select {
	case <-w.done:
		return errors.New("agent arrêté")
	default:
	}
	select {
	case w.jobs <- job:
		return nil
	default:
		return errors.New("agent occupé ; réessayez")
	}
}

func (w *Worker) DisplayChanged() {
	select {
	case w.display <- struct{}{}:
	default:
	}
}

func (w *Worker) Close() error {
	w.once.Do(func() { close(w.stop) })
	select {
	case <-w.done:
		return nil
	case <-time.After(3 * time.Second):
		return errors.New("arrêt de l'agent : opération Windows toujours en cours")
	}
}

func (w *Worker) Error(err error) {
	if err == nil {
		return
	}
	message := err.Error()
	if message == w.lastError {
		return
	}
	w.lastError = message
	if logErr := w.App.Store.Log(message); logErr != nil {
		message += "\nJournalisation impossible : " + logErr.Error()
	}
	w.notifyMu.Lock()
	notify := w.Notify
	w.notifyMu.Unlock()
	if notify != nil {
		notify(message)
	}
}

func (w *Worker) SetNotify(notify func(string)) {
	w.notifyMu.Lock()
	w.Notify = notify
	w.notifyMu.Unlock()
}

func (w *Worker) SetHistoryChanged(notify func()) {
	w.notifyMu.Lock()
	w.historyChanged = notify
	w.notifyMu.Unlock()
}

func (w *Worker) changed() {
	w.notifyMu.Lock()
	notify := w.historyChanged
	w.notifyMu.Unlock()
	if notify != nil {
		notify()
	}
}
func (w *Worker) SetSettings(next model.Settings) error {
	if err := next.Validate(); err != nil {
		return err
	}
	old := w.Settings
	previousValue, previousPresent, err := startup.ReadValue()
	if err != nil {
		return err
	}
	if err = startup.Set(next.Startup); err != nil {
		return fmt.Errorf("démarrage Windows : %w", err)
	}
	if err = w.App.Store.SaveSettings(next); err != nil {
		return errors.Join(err, startup.RestoreValue(previousValue, previousPresent))
	}
	w.Settings = next
	if old.Automatic != next.Automatic {
		w.observer.Reset()
		w.next = time.Time{}
	}
	if old.IntervalSeconds != next.IntervalSeconds {
		w.next = time.Time{}
	}
	return nil
}

func (w *Worker) observe() {
	if err := win32.Interactive(); err != nil {
		w.Error(err)
		return
	}
	ms, err := w.App.Display.Monitors()
	if err != nil {
		w.Error(err)
		return
	}
	err = w.observer.Step(time.Now(), ms,
		func() (model.Layout, error) { return w.App.Capture("checkpoint") },
		w.App.Store.SaveCheckpoint,
		func(l model.Layout) error {
			if err := w.App.Store.Save(l); err != nil {
				return err
			}
			// Retention failure must not cause the same transition to be archived again.
			if err := w.App.Store.Retain(w.Settings.AutomaticLimit); err != nil {
				w.Error(fmt.Errorf("conservation : %w", err))
			}
			w.changed()
			return nil
		})
	if err != nil {
		w.Error(err)
	}
}
