package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/Azimut-Studio/smart-desktop/internal/model"
)

type Store struct {
	Root  string
	logMu sync.Mutex
}

func Open(root string) (*Store, error) {
	for _, dir := range []string{root, filepath.Join(root, "backups"), filepath.Join(root, "logs")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
	}
	return &Store{Root: root}, nil
}

func ReadJSON(path string, dst interface{}) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 32*1024*1024+1))
	if err != nil {
		return err
	}
	if len(b) > 32*1024*1024 {
		return errors.New("fichier JSON trop volumineux")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err = d.Decode(dst); err != nil {
		return fmt.Errorf("%s : %w", path, err)
	}
	var extra interface{}
	if err = d.Decode(&extra); err != io.EOF {
		return fmt.Errorf("%s : données supplémentaires ou invalides", path)
	}
	return nil
}

func WriteJSON(path string, v interface{}) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if len(b) > 32*1024*1024 {
		return errors.New("fichier JSON trop volumineux")
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".smart-desktop-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return replace(name, path)
}

func (s *Store) Settings() (model.Settings, error) {
	var v model.Settings
	err := ReadJSON(filepath.Join(s.Root, "settings.json"), &v)
	if errors.Is(err, os.ErrNotExist) {
		return model.Defaults(), nil
	}
	if err != nil {
		return v, err
	}
	return v, v.Validate()
}

func (s *Store) SaveSettings(v model.Settings) error {
	if err := v.Validate(); err != nil {
		return err
	}
	return WriteJSON(filepath.Join(s.Root, "settings.json"), v)
}

func (s *Store) Save(l model.Layout) error {
	if err := l.Validate(); err != nil {
		return err
	}
	return WriteJSON(filepath.Join(s.Root, "backups", l.ID+".json"), l)
}

func (s *Store) Load(id string) (model.Layout, error) {
	var l model.Layout
	if !model.ValidID(id) {
		return l, errors.New("identifiant de sauvegarde invalide")
	}
	if err := ReadJSON(filepath.Join(s.Root, "backups", id+".json"), &l); err != nil {
		return l, err
	}
	if l.ID != id {
		return l, errors.New("identifiant interne de sauvegarde incohérent")
	}
	return l, l.Validate()
}

func (s *Store) List() ([]model.Layout, error) {
	files, err := os.ReadDir(filepath.Join(s.Root, "backups"))
	if err != nil {
		return nil, err
	}
	var out []model.Layout
	var errs []error
	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".json" {
			continue
		}
		id := f.Name()[:len(f.Name())-5]
		l, e := s.Load(id)
		if e != nil {
			errs = append(errs, fmt.Errorf("%s : %w", f.Name(), e))
			continue
		}
		out = append(out, l)
	}
	model.SortLayouts(out)
	return out, errors.Join(errs...)
}

func (s *Store) Delete(id string) error {
	if !model.ValidID(id) {
		return errors.New("identifiant de sauvegarde invalide")
	}
	return os.Remove(filepath.Join(s.Root, "backups", id+".json"))
}

func (s *Store) Retain(limit int) error {
	if limit < 1 {
		return errors.New("limite de conservation invalide")
	}
	ls, err := s.List()
	if err != nil {
		return err
	}
	n := 0
	for _, l := range ls {
		if l.Kind != "automatic" {
			continue
		}
		n++
		if n > limit {
			if err := s.Delete(l.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) Checkpoint() (*model.Layout, error) {
	var l model.Layout
	err := ReadJSON(filepath.Join(s.Root, "checkpoint.json"), &l)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err = l.Validate(); err != nil {
		return nil, err
	}
	if l.Kind != "checkpoint" {
		return nil, errors.New("type de checkpoint invalide")
	}
	return &l, nil
}

func (s *Store) SaveCheckpoint(l model.Layout) error {
	if err := l.Validate(); err != nil {
		return err
	}
	if l.Kind != "checkpoint" {
		return errors.New("type de checkpoint invalide")
	}
	return WriteJSON(filepath.Join(s.Root, "checkpoint.json"), l)
}
