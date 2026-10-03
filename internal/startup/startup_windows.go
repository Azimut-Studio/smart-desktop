package startup

import (
	"errors"
	"os"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const keyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
const valueName = "SmartDesktop"

func Command(path string) (string, error) {
	if path == "" || strings.ContainsAny(path, "\"\x00\r\n") {
		return "", errors.New("chemin de démarrage invalide")
	}
	return `"` + path + `" --background`, nil
}

func ReadValue() (string, bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, keyPath, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	defer k.Close()
	v, _, err := k.GetStringValue(valueName)
	if errors.Is(err, registry.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

func Enabled() (bool, error) {
	v, present, err := ReadValue()
	if err != nil || !present {
		return false, err
	}
	exe, err := os.Executable()
	if err != nil {
		return false, err
	}
	expected, err := Command(exe)
	return v == expected, err
}

func Set(enabled bool) error {
	if !enabled {
		return RestoreValue("", false)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd, err := Command(exe)
	if err != nil {
		return err
	}
	return RestoreValue(cmd, true)
}

func RestoreValue(value string, present bool) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, keyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if !present {
		err = k.DeleteValue(valueName)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	}
	return k.SetStringValue(valueName, value)
}
