//go:build windows && amd64

package ui

import (
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/Azimut-Studio/smart-desktop/internal/app"
	"github.com/Azimut-Studio/smart-desktop/internal/model"
	"github.com/Azimut-Studio/smart-desktop/internal/startup"
	"github.com/Azimut-Studio/smart-desktop/internal/win32"
	"golang.org/x/sys/windows"
)

const (
	wmResult     = 0x8001
	wmTray       = 0x8002
	idSave       = 100
	idRestore    = 101
	idDelete     = 102
	idRefresh    = 103
	idApply      = 104
	idOpen       = 105
	idQuit       = 106
	idHistory    = 200
	idTray       = 300
	idStartup    = 301
	idBackground = 302
	idAutomatic  = 303
	idInterval   = 304
	idLimit      = 305
)

type windowClass struct {
	Size, Style                        uint32
	Proc                               uintptr
	ClassExtra, WindowExtra            int32
	Instance, Icon, Cursor, Background uintptr
	Menu, Name                         *uint16
	SmallIcon                          uintptr
}

type notificationIcon struct {
	Size                uint32
	HWND                uintptr
	ID, Flags, Callback uint32
	Icon                uintptr
	Tip                 [128]uint16
	State, StateMask    uint32
	Info                [256]uint16
	Version             uint32
	InfoTitle           [64]uint16
	InfoFlags           uint32
	GUID                windows.GUID
	BalloonIcon         uintptr
}

type Window struct {
	worker                           *app.Worker
	hwnd, instance, icon, font, menu uintptr
	className                        string
	controls                         map[int]uintptr
	labels                           []uintptr
	settings                         model.Settings
	history                          []model.Layout
	busy                             bool
	tray                             bool
	closing                          bool
	taskbarMessage                   uint32
	queueMu                          sync.Mutex
	queue                            []func()
	createErr                        error
	dpi                              uint32
	scroll                           int
	layouting                        bool
	menuErr                          error
}

type scrollInfo struct {
	Size, Mask      uint32
	Min, Max        int32
	Page            uint32
	Position, Track int32
}

var current *Window
var callback = windows.NewCallback(windowProc)

func invoke(proc *windows.LazyProc, args ...interface{}) (uintptr, uintptr, error) {
	raw := make([]uintptr, len(args))
	for i, arg := range args {
		switch v := arg.(type) {
		case uintptr:
			raw[i] = v
		case int:
			raw[i] = uintptr(v)
		case uint32:
			raw[i] = uintptr(v)
		case *uint16:
			raw[i] = uintptr(unsafe.Pointer(v))
		case unsafe.Pointer:
			raw[i] = uintptr(v)
		default:
			panic(fmt.Sprintf("unsupported Win32 argument %T", arg))
		}
	}
	a, b, err := proc.Call(raw...)
	runtime.KeepAlive(args)
	return a, b, err
}

func call(name string, args ...interface{}) uintptr {
	value, _, _ := invoke(win32.User32.NewProc(name), args...)
	return value
}
func str(s string) *uint16 { return win32.Text(s) }

func Message(text string, errorBox bool) {
	flags := uintptr(0x40)
	if errorBox {
		flags = 0x10
	}
	call("MessageBoxW", 0, str(text), str("Smart Desktop"), flags)
}

func (w *Window) message(text string, errorBox bool) {
	flags := uintptr(0x40)
	if errorBox {
		flags = 0x10
	}
	call("MessageBoxW", w.hwnd, str(text), str("Smart Desktop"), flags)
}
func (w *Window) confirm(text string) bool {
	return call("MessageBoxW", w.hwnd, str(text), str("Smart Desktop"), 0x124) == 6
}

func Run(worker *app.Worker, settings model.Settings, className string, background bool) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	w := &Window{worker: worker, settings: settings, className: className, controls: make(map[int]uintptr), dpi: 96}
	if dpi := call("GetDpiForSystem"); dpi != 0 {
		w.dpi = uint32(dpi)
	}
	current = w
	defer func() { current = nil }()
	w.instance, _, _ = win32.Kernel32.NewProc("GetModuleHandleW").Call(0)
	w.icon = call("LoadIconW", w.instance, 1)
	if w.icon == 0 {
		w.icon = call("LoadIconW", 0, 32512)
	}
	if w.icon == 0 {
		return errors.New("icône de l'application indisponible")
	}
	class := windowClass{Size: uint32(unsafe.Sizeof(windowClass{})), Proc: callback,
		Instance: w.instance, Icon: w.icon, SmallIcon: w.icon, Cursor: call("LoadCursorW", 0, 32512),
		Background: 6, Name: win32.Text(className)}
	r, _, e := win32.User32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&class)))
	if err := win32.CheckBOOL("RegisterClassExW", r, e); err != nil {
		return err
	}
	defer call("UnregisterClassW", str(className), w.instance)
	width, height := 1080*int(w.dpi)/96, 820*int(w.dpi)/96
	if max := int(call("GetSystemMetrics", 0)) - 40; width > max {
		width = max
	}
	if max := int(call("GetSystemMetrics", 1)) - 80; height > max {
		height = max
	}
	w.hwnd, _, e = invoke(win32.User32.NewProc("CreateWindowExW"), 0, str(className), str("Smart Desktop — positions des icônes"), 0x00ef0000,
		20, 20, width, height, 0, 0, w.instance, 0)
	if err := win32.CheckBOOL("CreateWindowExW", w.hwnd, e); err != nil {
		return err
	}
	if w.createErr != nil {
		call("DestroyWindow", w.hwnd)
		return w.createErr
	}
	w.taskbarMessage = uint32(call("RegisterWindowMessageW", str("TaskbarCreated")))
	worker.SetNotify(func(text string) {
		w.post(func() {
			w.status(text)
			if w.tray {
				if err := w.balloon(text); err != nil {
					w.status(text + "\r\n" + err.Error())
				}
			}
		})
		worker.SetHistoryChanged(func() { w.post(w.refresh) })
	})
	if err := w.setTray(settings.Tray); err != nil {
		call("DestroyWindow", w.hwnd)
		return err
	}
	if !background {
		w.open()
	} else {
		w.refresh()
	}
	w.submit(func(a *app.Worker) {
		status, err := a.App.Store.Status()
		if err != nil {
			a.Error(err)
		} else if status != "" {
			w.post(func() { w.status("Dernier diagnostic : " + status) })
		}
		enabled, err := startup.Enabled()
		if err != nil {
			a.Error(err)
			return
		}
		if enabled != a.Settings.Startup {
			w.post(func() {
				w.settings.Startup = enabled
				w.check(idStartup, enabled)
				w.status("L'inscription au démarrage ne correspond pas aux paramètres enregistrés. Vérifiez le chemin de l'exécutable et appliquez vos paramètres.")
			})
		}
	})
	var m win32.Message
	for {
		result, _, e := win32.User32.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(result) == -1 {
			return fmt.Errorf("GetMessageW : %w", e)
		}
		if result == 0 {
			break
		}
		if call("IsDialogMessageW", w.hwnd, unsafe.Pointer(&m)) == 0 {
			call("TranslateMessage", unsafe.Pointer(&m))
			call("DispatchMessageW", unsafe.Pointer(&m))
		}
	}
	return nil
}

func (w *Window) post(f func()) {
	w.queueMu.Lock()
	if w.closing {
		w.queueMu.Unlock()
		return
	}
	w.queue = append(w.queue, f)
	w.queueMu.Unlock()
	ok, _, err := win32.User32.NewProc("PostMessageW").Call(w.hwnd, wmResult, 0, 0)
	if ok == 0 {
		message := fmt.Sprintf("PostMessageW : %v", err)
		if logErr := w.worker.App.Store.Log(message); logErr != nil {
			message += "\n" + logErr.Error()
		}
		Message(message, true)
	}
}

func (w *Window) submit(job app.Job) {
	if err := w.worker.Submit(job); err != nil {
		w.message(err.Error(), true)
	}
}

func windowProc(hwnd uintptr, message uint32, wp, lp uintptr) uintptr {
	w := current
	if w == nil {
		return call("DefWindowProcW", hwnd, uintptr(message), wp, lp)
	}
	if message == w.taskbarMessage && w.taskbarMessage != 0 {
		w.tray = false
		if err := w.setTray(w.settings.Tray); err != nil {
			w.status(err.Error())
			w.message(err.Error(), true)
		}
		return 0
	}
	switch message {
	case 1:
		w.hwnd = hwnd
		w.create()
		return 0
	case 5:
		w.layout()
		return 0
	case 0x115:
		switch wp & 0xffff {
		case 0:
			w.scroll -= 27
		case 1:
			w.scroll += 27
		case 2:
			w.scroll -= 200
		case 3:
			w.scroll += 200
		case 4, 5:
			w.scroll = int((wp >> 16) & 0xffff)
		case 6:
			w.scroll = 0
		case 7:
			w.scroll = 10000
		}
		w.layout()
		return 0
	case 0x20a:
		w.scroll -= int(int16(wp>>16)) / 120 * 81
		w.layout()
		return 0
	case 0x2e0:
		w.dpi = uint32(wp & 0xffff)
		var r win32.Rect
		var read uintptr
		if err := windows.ReadProcessMemory(windows.CurrentProcess(), lp, (*byte)(unsafe.Pointer(&r)), unsafe.Sizeof(r), &read); err != nil {
			w.status("Lecture du changement DPI impossible : " + err.Error())
			return 0
		}
		call("SetWindowPos", hwnd, 0, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top), 0x14)
		w.updateFont()
		w.layout()
		return 0
	case 0x111:
		id := int(wp & 0xffff)
		if id == idHistory && (wp>>16) == 1 {
			w.details()
			return 0
		}
		w.command(id)
		return 0
	case 0x7e, 0x219, 0x1a:
		w.worker.DisplayChanged()
	case wmResult:
		w.queueMu.Lock()
		jobs := w.queue
		w.queue = nil
		w.queueMu.Unlock()
		for _, f := range jobs {
			f()
		}
		return 0
	case wmTray:
		switch uint32(lp) {
		case 0x203:
			w.open()
		case 0x205, 0x7b:
			w.trayMenu()
		}
		return 0
	case 0x10:
		if w.settings.Background {
			call("ShowWindow", hwnd, 0)
			return 0
		}
		if w.busy {
			w.message("Une opération est en cours. Attendez sa fin avant de quitter.", true)
			return 0
		}
		call("DestroyWindow", hwnd)
		return 0
	case 2:
		w.queueMu.Lock()
		w.closing = true
		w.queueMu.Unlock()
		if err := w.setTray(false); err != nil {
			Message(err.Error(), true)
		}
		if w.font != 0 {
			windows.NewLazySystemDLL("gdi32.dll").NewProc("DeleteObject").Call(w.font)
		}
		if w.menu != 0 {
			call("DestroyMenu", w.menu)
			w.menu = 0
		}
		call("PostQuitMessage", 0)
		return 0
	case 0x8003:
		w.open()
		return 0
	}
	return call("DefWindowProcW", hwnd, uintptr(message), wp, lp)
}

func (w *Window) control(class, text string, style uintptr, id int) uintptr {
	h, _, e := invoke(win32.User32.NewProc("CreateWindowExW"), 0, str(class), str(text), style|0x50000000,
		0, 0, 10, 10, w.hwnd, uintptr(id), w.instance, 0)
	if err := win32.CheckBOOL("Créer le contrôle "+text, h, e); err != nil {
		w.createErr = errors.Join(w.createErr, err)
	}
	w.controls[id] = h
	return h
}

func (w *Window) create() {
	dpi := call("GetDpiForWindow", w.hwnd)
	if dpi != 0 {
		w.dpi = uint32(dpi)
	}
	for _, b := range []struct {
		id   int
		text string
	}{
		{idSave, "Sauvegarder"}, {idRestore, "Restaurer"}, {idDelete, "Supprimer"}, {idRefresh, "Actualiser"}, {idApply, "Appliquer les paramètres"}, {idQuit, "Quitter"},
	} {
		w.control("BUTTON", b.text, 0x10000, b.id)
	}
	w.control("LISTBOX", "", 0x00a10001, idHistory)
	w.control("EDIT", "", 0x00a00844, 201)
	w.control("STATIC", "Sauvegardes : date locale | résolutions et DPI | origine | icônes", 0, 202)
	for _, c := range []struct {
		id   int
		text string
	}{
		{idTray, "Afficher une icône dans la barre système (tray)"},
		{idStartup, "Démarrer en arrière-plan à l'ouverture de session Windows"},
		{idBackground, "Continuer en arrière-plan après fermeture de la fenêtre"},
		{idAutomatic, "Conserver le dernier état observé lors d'un changement d'affichage"},
	} {
		w.control("BUTTON", c.text, 0x10003, c.id)
	}
	w.control("STATIC", "Intervalle (1–3600 s) :", 0, 306)
	w.control("EDIT", strconv.Itoa(w.settings.IntervalSeconds), 0x00812000, idInterval)
	w.control("STATIC", "Limite automatique (1–10000) :", 0, 307)
	w.control("EDIT", strconv.Itoa(w.settings.AutomaticLimit), 0x00812000, idLimit)
	w.control("STATIC", "L'automatisation utilise une capture périodique : elle ne garantit pas une sauvegarde juste avant le changement.\r\nFermer quitte par défaut. Un agent sans tray se retrouve en relançant l'application.", 0, 308)
	w.control("EDIT", "Prêt.", 0x00a00844, 309)
	w.check(idTray, w.settings.Tray)
	w.check(idStartup, w.settings.Startup)
	w.check(idBackground, w.settings.Background)
	w.check(idAutomatic, w.settings.Automatic)
	w.updateFont()
	w.rebuildMenu()
	w.layout()
}

func (w *Window) updateFont() {
	gdi := windows.NewLazySystemDLL("gdi32.dll")
	height := -int32(15 * w.dpi / 96)
	newFont, _, _ := invoke(gdi.NewProc("CreateFontW"), uintptr(height), 0, 0, 0, 400, 0, 0, 0, 1, 0, 0, 0, 0, str("Segoe UI"))
	if newFont == 0 {
		w.status("Création de police impossible.")
		return
	}
	old := w.font
	w.font = newFont
	for _, h := range w.controls {
		call("SendMessageW", h, 0x30, w.font, 1)
	}
	if old != 0 {
		gdi.NewProc("DeleteObject").Call(old)
	}
}

func (w *Window) layout() {
	if len(w.controls) == 0 || w.layouting {
		return
	}
	w.layouting = true
	defer func() { w.layouting = false }()
	var r win32.Rect
	call("GetClientRect", w.hwnd, unsafe.Pointer(&r))
	scale := float64(w.dpi) / 96
	width := int(float64(r.Right) / scale)
	height := int(float64(r.Bottom) / scale)
	virtualHeight := height
	if virtualHeight < 600 {
		virtualHeight = 600
	}
	maxScroll := virtualHeight - height
	if w.scroll < 0 {
		w.scroll = 0
	}
	if w.scroll > maxScroll {
		w.scroll = maxScroll
	}
	info := scrollInfo{Size: uint32(unsafe.Sizeof(scrollInfo{})), Mask: 0x17, Max: int32(virtualHeight - 1), Page: uint32(height), Position: int32(w.scroll)}
	call("SetScrollInfo", w.hwnd, 1, unsafe.Pointer(&info), 1)
	move := func(id, x, y, dx, dy int) {
		y -= w.scroll
		call("MoveWindow", w.controls[id], uintptr(int(float64(x)*scale)), uintptr(int(float64(y)*scale)), uintptr(int(float64(dx)*scale)), uintptr(int(float64(dy)*scale)), 1)
	}
	for i, id := range []int{idSave, idRestore, idDelete, idRefresh} {
		move(id, 16+i*138, 12, 128, 32)
	}
	move(idQuit, width-110, 12, 94, 32)
	move(202, 16, 54, width-32, 24)
	listHeight := virtualHeight - 486
	if listHeight < 80 {
		listHeight = 80
	}
	move(idHistory, 16, 80, width-32, listHeight)
	y := 88 + listHeight
	move(201, 16, y, width-32, 90)
	y += 100
	for _, id := range []int{idTray, idStartup, idBackground, idAutomatic} {
		move(id, 16, y, width-32, 24)
		y += 27
	}
	move(306, 16, y, 158, 24)
	move(idInterval, 176, y, 65, 24)
	move(307, 270, y, 224, 24)
	move(idLimit, 500, y, 80, 24)
	move(idApply, 16, y+32, 240, 28)
	y += 68
	move(308, 16, y, width-32, 44)
	move(309, 16, y+50, width-32, 56)
}

func (w *Window) check(id int, yes bool) {
	var n uintptr
	if yes {
		n = 1
	}
	call("SendMessageW", w.controls[id], 0xf1, n, 0)
}
func (w *Window) checked(id int) bool { return call("SendMessageW", w.controls[id], 0xf0, 0, 0) == 1 }
func (w *Window) text(id int) string {
	buf := make([]uint16, 256)
	call("GetWindowTextW", w.controls[id], unsafe.Pointer(&buf[0]), uintptr(len(buf)))
	return windows.UTF16ToString(buf)
}
func (w *Window) status(s string) {
	call("SetWindowTextW", w.controls[309], str(strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n")))
}

func kind(k string) string {
	switch k {
	case "manual":
		return "manuelle"
	case "automatic":
		return "automatique"
	case "safety":
		return "sécurité"
	}
	return k
}
func label(l model.Layout) string {
	return fmt.Sprintf("%s | %s | %s | %d icônes", l.CapturedAt.Local().Format("02/01/2006 15:04:05"), model.DisplaySummary(l.Monitors), kind(l.Kind), len(l.Icons))
}

func (w *Window) refresh() {
	w.submit(func(a *app.Worker) {
		ls, err := a.App.Store.List()
		if err != nil {
			a.Error(err)
		}
		w.post(func() {
			w.history = ls
			call("SendMessageW", w.controls[idHistory], 0x184, 0, 0)
			for _, l := range ls {
				result := int32(call("SendMessageW", w.controls[idHistory], 0x180, 0, str(label(l))))
				if result < 0 {
					call("SendMessageW", w.controls[idHistory], 0x184, 0, 0)
					w.history = nil
					w.status("Affichage de l'historique impossible : mémoire ou contrôle Windows indisponible.")
					break
				}
			}
			w.rebuildMenu()
			w.details()
		})
	})
}

func (w *Window) selected() *model.Layout {
	index := int(int32(call("SendMessageW", w.controls[idHistory], 0x188, 0, 0)))
	if index < 0 || index >= len(w.history) {
		return nil
	}
	return &w.history[index]
}
func (w *Window) details() {
	l := w.selected()
	text := "Sélectionnez une sauvegarde pour consulter sa configuration."
	if l != nil {
		var parts []string
		for _, m := range l.Monitors {
			parts = append(parts, fmt.Sprintf("%s : %dx%d, origine (%d,%d), DPI %d/%d, orientation %d, principal %t", m.Name, m.Width, m.Height, m.Left, m.Top, m.DPIX, m.DPIY, m.Orientation, m.Primary))
			for _, target := range m.Targets {
				parts = append(parts, fmt.Sprintf("  Cible : %s — %dx%d, orientation %d", target.Name, target.Width, target.Height, target.Orientation))
			}
		}
		if l.DetectedAt != nil {
			parts = append(parts, "Changement détecté : "+l.DetectedAt.Local().Format("02/01/2006 15:04:05")+" ; capture périodique antérieure, non garantie immédiatement avant.")
		}
		text = strings.Join(parts, "\r\n")
	}
	call("SetWindowTextW", w.controls[201], str(text))
}

func (w *Window) operation(job func(*app.Worker) (string, error)) {
	if w.busy {
		w.message("Une opération est déjà en cours.", true)
		return
	}
	w.busy = true
	w.status("Opération en cours…")
	for _, id := range []int{idSave, idRestore, idDelete, idApply} {
		call("EnableWindow", w.controls[id], 0)
	}
	err := w.worker.Submit(func(a *app.Worker) {
		text, err := job(a)
		if err != nil {
			a.Error(err)
		}
		w.post(func() {
			w.busy = false
			for _, id := range []int{idSave, idRestore, idDelete, idApply} {
				call("EnableWindow", w.controls[id], 1)
			}
			if err != nil {
				text += "\n" + err.Error()
			}
			w.status(text)
			if err != nil {
				w.message(text, true)
			}
			w.refresh()
		})
	})
	if err != nil {
		w.busy = false
		for _, id := range []int{idSave, idRestore, idDelete, idApply} {
			call("EnableWindow", w.controls[id], 1)
		}
		w.message(err.Error(), true)
	}
}

func (w *Window) command(id int) {
	if id >= 4000 && id < 4000+len(w.history) {
		call("SendMessageW", w.controls[idHistory], 0x186, uintptr(id-4000), 0)
		w.details()
		w.open()
		w.command(idRestore)
		return
	}
	switch id {
	case idOpen:
		w.open()
	case idQuit:
		if w.busy {
			w.message("Attendez la fin de l'opération.", true)
			return
		}
		call("DestroyWindow", w.hwnd)
	case idSave:
		w.operation(func(a *app.Worker) (string, error) {
			l, err := a.App.Save()
			if err != nil {
				return "Sauvegarde impossible.", err
			}
			return "Sauvegarde créée : " + label(l), nil
		})
	case idRefresh:
		w.refresh()
	case idRestore:
		l := w.selected()
		if l == nil {
			w.message("Sélectionnez une sauvegarde.", true)
			return
		}
		id := l.ID
		if !w.confirm("Restaurer cette disposition ?\n" + label(*l) + "\nUne sauvegarde de sécurité sera créée. Désactivez l'arrangement automatique et l'alignement sur la grille sur le bureau.") {
			return
		}
		w.operation(func(a *app.Worker) (string, error) { r, err := a.App.Restore(id); return r.String(), err })
	case idDelete:
		l := w.selected()
		if l == nil {
			w.message("Sélectionnez une sauvegarde.", true)
			return
		}
		id := l.ID
		if !w.confirm("Supprimer définitivement cette sauvegarde ?\n" + label(*l)) {
			return
		}
		w.operation(func(a *app.Worker) (string, error) {
			err := a.App.Store.Delete(id)
			if err != nil {
				return "Suppression impossible.", err
			}
			return "Sauvegarde supprimée.", nil
		})
	case idApply:
		w.apply()
	}
}

func (w *Window) apply() {
	next := w.settings
	next.Tray = w.checked(idTray)
	next.Startup = w.checked(idStartup)
	next.Background = w.checked(idBackground)
	next.Automatic = w.checked(idAutomatic)
	interval, err := strconv.Atoi(w.text(idInterval))
	if err != nil {
		w.message("Intervalle invalide.", true)
		return
	}
	limit, err := strconv.Atoi(w.text(idLimit))
	if err != nil {
		w.message("Limite invalide.", true)
		return
	}
	next.IntervalSeconds = interval
	next.AutomaticLimit = limit
	if err := next.Validate(); err != nil {
		w.message(err.Error(), true)
		return
	}
	oldTray := w.tray
	if err := w.setTray(next.Tray); err != nil {
		w.message(err.Error(), true)
		return
	}
	w.operation(func(a *app.Worker) (string, error) {
		err := a.SetSettings(next)
		w.post(func() {
			if err == nil {
				w.settings = next
			} else if trayErr := w.setTray(oldTray); trayErr != nil {
				w.message(trayErr.Error(), true)
			}
		})
		if err != nil {
			return "Paramètres non enregistrés.", err
		}
		return "Paramètres appliqués.", nil
	})
}

func appendMenu(menu uintptr, flags uintptr, id uintptr, text string) {
	ok, _, err := invoke(win32.User32.NewProc("AppendMenuW"), menu, flags, id, str(text))
	if err := win32.CheckBOOL("AppendMenuW", ok, err); err != nil && current != nil {
		current.menuErr = errors.Join(current.menuErr, err)
	}
}

func (w *Window) historyMenu() uintptr {
	m := call("CreatePopupMenu")
	for i, l := range w.history {
		if i == 50 {
			break
		}
		appendMenu(m, 0, uintptr(4000+i), label(l))
	}
	if len(w.history) == 0 {
		appendMenu(m, 2, 0, "Aucune sauvegarde")
	}
	return m
}
func (w *Window) rebuildMenu() {
	w.menuErr = nil
	m := call("CreateMenu")
	file := call("CreatePopupMenu")
	appendMenu(file, 0, idSave, "Sauvegarder")
	appendMenu(file, 0, idRefresh, "Actualiser")
	appendMenu(file, 0, idQuit, "Quitter")
	appendMenu(m, 0x10, file, "Application")
	appendMenu(m, 0x10, w.historyMenu(), "Sauvegardes (50 dernières)")
	if w.menuErr != nil {
		call("DestroyMenu", m)
		w.createErr = errors.Join(w.createErr, w.menuErr)
		w.status(w.menuErr.Error())
		return
	}
	ok, _, err := invoke(win32.User32.NewProc("SetMenu"), w.hwnd, m)
	if err := win32.CheckBOOL("SetMenu", ok, err); err != nil {
		w.menuErr = errors.Join(w.menuErr, err)
	}
	if w.menuErr != nil {
		call("DestroyMenu", m)
		w.createErr = errors.Join(w.createErr, w.menuErr)
		w.status(w.menuErr.Error())
		return
	}
	if w.menu != 0 {
		call("DestroyMenu", w.menu)
	}
	w.menu = m
	call("DrawMenuBar", w.hwnd)
}

func (w *Window) open() {
	call("ShowWindow", w.hwnd, 9)
	call("SetForegroundWindow", w.hwnd)
	w.refresh()
}

func (w *Window) trayData() notificationIcon {
	n := notificationIcon{Size: uint32(unsafe.Sizeof(notificationIcon{})), HWND: w.hwnd, ID: 1, Flags: 7, Callback: wmTray, Icon: w.icon}
	copy(n.Tip[:], windows.StringToUTF16("Smart Desktop"))
	return n
}

func (w *Window) setTray(enabled bool) error {
	if w.tray == enabled {
		return nil
	}
	n := w.trayData()
	op := uintptr(0)
	if !enabled {
		op = 2
	}
	ok, _, e := win32.Shell32.NewProc("Shell_NotifyIconW").Call(op, uintptr(unsafe.Pointer(&n)))
	if err := win32.CheckBOOL("Shell_NotifyIconW", ok, e); err != nil {
		return err
	}
	w.tray = enabled
	return nil
}
func (w *Window) balloon(text string) error {
	n := w.trayData()
	n.Flags = 0x10
	n.InfoFlags = 3
	copy(n.Info[:255], windows.StringToUTF16(text))
	copy(n.InfoTitle[:], windows.StringToUTF16("Smart Desktop : diagnostic"))
	ok, _, e := win32.Shell32.NewProc("Shell_NotifyIconW").Call(1, uintptr(unsafe.Pointer(&n)))
	return win32.CheckBOOL("Notification tray", ok, e)
}
func (w *Window) trayMenu() {
	w.menuErr = nil
	m := call("CreatePopupMenu")
	defer call("DestroyMenu", m)
	appendMenu(m, 0, idOpen, "Ouvrir")
	appendMenu(m, 0, idSave, "Sauvegarder")
	appendMenu(m, 0x10, w.historyMenu(), "Restaurer une sauvegarde")
	appendMenu(m, 0, idQuit, "Quitter")
	if w.menuErr != nil {
		w.message(w.menuErr.Error(), true)
		return
	}
	var p model.Point
	call("GetCursorPos", unsafe.Pointer(&p))
	call("SetForegroundWindow", w.hwnd)
	id := call("TrackPopupMenu", m, 0x100|2, uintptr(p.X), uintptr(p.Y), 0, w.hwnd, 0)
	if id != 0 {
		w.command(int(id))
	}
	call("PostMessageW", w.hwnd, 0, 0, 0)
}

func Activate(className string) error {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		h := call("FindWindowW", str(className), 0)
		if h != 0 {
			ok, _, e := win32.User32.NewProc("PostMessageW").Call(h, 0x8003, 0, 0)
			return win32.CheckBOOL("Ouvrir l'agent existant", ok, e)
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("l'agent existe mais sa fenêtre ne répond pas")
}
