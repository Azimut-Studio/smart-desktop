//go:build windows && amd64

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Azimut-Studio/smart-desktop/internal/app"
	"github.com/Azimut-Studio/smart-desktop/internal/storage"
	"github.com/Azimut-Studio/smart-desktop/internal/ui"
	"golang.org/x/sys/windows"
)

func run() error {
	background := flag.Bool("background", false, "Démarrer sans fenêtre")
	flag.Parse()
	if flag.NArg() != 0 {
		return errors.New("arguments inconnus")
	}
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return err
	}
	user, err := token.GetTokenUser()
	token.Close()
	if err != nil {
		return err
	}
	className := "SmartDesktop." + user.User.Sid.String()
	mutex, err := windows.CreateMutex(nil, false, windows.StringToUTF16Ptr(`Local\`+className))
	if mutex != 0 {
		defer windows.CloseHandle(mutex)
	}
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return ui.Activate(className)
	}
	if err != nil {
		return err
	}
	root := os.Getenv("LOCALAPPDATA")
	if root == "" {
		return errors.New("LOCALAPPDATA est indisponible")
	}
	store, err := storage.Open(filepath.Join(root, "SmartDesktop"))
	if err != nil {
		return err
	}
	settings, err := store.Settings()
	if err != nil {
		return fmt.Errorf("paramètres invalides, fichier conservé : %w", err)
	}
	worker := app.NewWorker(store, settings)
	if err = worker.Start(); err != nil {
		return err
	}
	uiErr := ui.Run(worker, settings, className, *background)
	closeErr := worker.Close()
	if err = errors.Join(uiErr, closeErr); err != nil {
		if logErr := store.Log(err.Error()); logErr != nil {
			return errors.Join(err, logErr)
		}
	}
	return err
}

func main() {
	if err := run(); err != nil {
		ui.Message(err.Error(), true)
		os.Exit(1)
	}
}
