package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func (s *Store) Log(message string) error {
	s.logMu.Lock()
	defer s.logMu.Unlock()
	path := filepath.Join(s.Root, "logs", "agent.log")
	if info, err := os.Stat(path); err == nil && info.Size() >= 1024*1024 {
		old := path + ".1"
		if err = os.Remove(old); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err = replace(path, old); err != nil {
			return err
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	_, writeErr := fmt.Fprintf(f, "%s %s\n", time.Now().UTC().Format(time.RFC3339), message)
	closeErr := f.Close()
	statusErr := WriteJSON(filepath.Join(s.Root, "status.json"), struct {
		Message string `json:"message"`
	}{message})
	return errors.Join(writeErr, closeErr, statusErr)
}

func (s *Store) Status() (string, error) {
	var v struct {
		Message string `json:"message"`
	}
	err := ReadJSON(filepath.Join(s.Root, "status.json"), &v)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return v.Message, err
}
