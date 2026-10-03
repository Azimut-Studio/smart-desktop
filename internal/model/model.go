package model

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
)

const Version = 1

type Point struct {
	X int32 `json:"x"`
	Y int32 `json:"y"`
}

type Icon struct {
	Identity string `json:"identity"`
	Name     string `json:"name"`
	Position Point  `json:"position"`
}

type Monitor struct {
	Identity    string   `json:"identity"`
	Name        string   `json:"name"`
	Left        int32    `json:"left"`
	Top         int32    `json:"top"`
	Width       int32    `json:"width"`
	Height      int32    `json:"height"`
	Orientation uint32   `json:"orientation"`
	DPIX        uint32   `json:"dpi_x"`
	DPIY        uint32   `json:"dpi_y"`
	Primary     bool     `json:"primary"`
	Targets     []Target `json:"targets,omitempty"`
}

type Target struct {
	Identity    string `json:"identity"`
	Name        string `json:"name"`
	Width       uint32 `json:"width"`
	Height      uint32 `json:"height"`
	Orientation uint32 `json:"orientation"`
}
type Layout struct {
	Version    int        `json:"version"`
	ID         string     `json:"id"`
	CapturedAt time.Time  `json:"captured_at"`
	DetectedAt *time.Time `json:"detected_at,omitempty"`
	Kind       string     `json:"kind"`
	Monitors   []Monitor  `json:"monitors"`
	Icons      []Icon     `json:"icons"`
	SnapToGrid *bool      `json:"snap_to_grid,omitempty"`
}

type DesktopOptions struct {
	SnapToGrid  bool
	AutoArrange bool
}

type DesktopSnapshot struct {
	Icons   []Icon
	Options DesktopOptions
}

func (o DesktopOptions) CheckPosition(savedGrid *bool) error {
	if o.AutoArrange {
		return errors.New("désactivez « Réorganiser automatiquement les icônes » dans le menu Affichage du bureau, puis réessayez")
	}
	if o.SnapToGrid && (savedGrid == nil || !*savedGrid) {
		if savedGrid == nil {
			return errors.New("état de la grille non enregistré dans cette sauvegarde : désactivez « Aligner les icônes sur la grille » sur le bureau, puis réessayez")
		}
		return errors.New("cette sauvegarde a été capturée sans alignement sur la grille : désactivez « Aligner les icônes sur la grille » sur le bureau, puis réessayez")
	}
	return nil
}

type Settings struct {
	Version         int  `json:"version"`
	Tray            bool `json:"tray"`
	Startup         bool `json:"startup"`
	Background      bool `json:"background"`
	Automatic       bool `json:"automatic"`
	IntervalSeconds int  `json:"interval_seconds"`
	AutomaticLimit  int  `json:"automatic_limit"`
}

func Defaults() Settings {
	return Settings{Version: Version, IntervalSeconds: 5, AutomaticLimit: 100}
}

func (s Settings) Validate() error {
	if s.Version != Version {
		return errors.New("version des paramètres non prise en charge")
	}
	if s.IntervalSeconds < 1 || s.IntervalSeconds > 3600 {
		return errors.New("intervalle : entre 1 et 3600 secondes")
	}
	if s.AutomaticLimit < 1 || s.AutomaticLimit > 10000 {
		return errors.New("conservation : entre 1 et 10000 sauvegardes automatiques")
	}
	return nil
}

func NewID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func ValidID(id string) bool {
	if len(id) != 32 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil && strings.ToLower(id) == id
}

func ValidateMonitors(ms []Monitor) error {
	if len(ms) == 0 || len(ms) > 64 {
		return errors.New("configuration d'écrans vide ou trop grande")
	}
	seen := make(map[string]bool)
	primary := 0
	for _, m := range ms {
		if !validText(m.Identity, false) || !validText(m.Name, true) || seen[m.Identity] || m.Width <= 0 || m.Height <= 0 || m.DPIX == 0 || m.DPIY == 0 || m.Orientation > 3 {
			return errors.New("configuration d'écrans invalide")
		}
		seen[m.Identity] = true
		targets := make(map[string]bool)
		for _, target := range m.Targets {
			if !validText(target.Identity, false) || !validText(target.Name, true) || targets[target.Identity] || target.Width == 0 || target.Height == 0 || target.Orientation > 3 {
				return errors.New("cible d'affichage invalide")
			}
			targets[target.Identity] = true
		}
		if m.Primary {
			primary++
		}
	}
	if primary != 1 {
		return errors.New("la configuration doit contenir un seul écran principal")
	}
	return nil
}

func (l Layout) Validate() error {
	if l.Version != Version || !ValidID(l.ID) || l.CapturedAt.IsZero() {
		return errors.New("en-tête de sauvegarde invalide")
	}
	if l.Kind != "manual" && l.Kind != "automatic" && l.Kind != "safety" && l.Kind != "checkpoint" {
		return errors.New("type de sauvegarde invalide")
	}
	if l.Kind == "automatic" && (l.DetectedAt == nil || l.DetectedAt.Before(l.CapturedAt)) {
		return errors.New("date de détection automatique invalide")
	}
	if err := ValidateMonitors(l.Monitors); err != nil {
		return err
	}
	if len(l.Icons) > 100000 {
		return errors.New("trop d'icônes")
	}
	seen := make(map[string]bool)
	for _, i := range l.Icons {
		if !validText(i.Identity, false) || !validText(i.Name, true) || seen[i.Identity] {
			return errors.New("identité d'icône vide, dupliquée ou trop longue")
		}
		seen[i.Identity] = true
	}
	return nil
}

func validText(s string, allowEmpty bool) bool {
	return (allowEmpty || s != "") && len(s) <= 32768 && !strings.ContainsRune(s, 0)
}

func Compatible(a, b []Monitor) bool {
	if ValidateMonitors(a) != nil || ValidateMonitors(b) != nil || len(a) != len(b) {
		return false
	}
	byID := make(map[string]Monitor, len(a))
	for _, m := range a {
		m = normalizedMonitor(m)
		byID[m.Identity] = m
	}
	for _, m := range b {
		m = normalizedMonitor(m)
		if old, ok := byID[m.Identity]; !ok || !reflect.DeepEqual(old, m) {
			return false
		}
	}
	return true
}

func normalizedMonitor(m Monitor) Monitor {
	m.Name = ""
	m.Targets = append([]Target(nil), m.Targets...)
	for i := range m.Targets {
		m.Targets[i].Name = ""
	}
	sort.Slice(m.Targets, func(i, j int) bool { return m.Targets[i].Identity < m.Targets[j].Identity })
	return m
}

func SortLayouts(ls []Layout) {
	sort.Slice(ls, func(i, j int) bool {
		if ls[i].CapturedAt.Equal(ls[j].CapturedAt) {
			return ls[i].ID > ls[j].ID
		}
		return ls[i].CapturedAt.After(ls[j].CapturedAt)
	})
}

func DisplaySummary(ms []Monitor) string {
	parts := make([]string, 0, len(ms))
	for _, m := range ms {
		part := fmt.Sprintf("%dx%d (%d DPI)", m.Width, m.Height, m.DPIX)
		if len(m.Targets) > 1 {
			var physical []string
			for _, target := range m.Targets {
				physical = append(physical, fmt.Sprintf("%dx%d", target.Width, target.Height))
			}
			part += " [duplication : " + strings.Join(physical, ", ") + "]"
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, " + ")
}
