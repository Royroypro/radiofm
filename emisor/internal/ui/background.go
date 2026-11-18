package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/energye/systray"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"radioclub25/emisor/internal/autostart"
	"radioclub25/emisor/internal/config"
	runnerpkg "radioclub25/emisor/internal/runner"
)

// AutoBroadcastDelay es la espera antes de emitir solo al abrir: tiempo para
// que Windows termine de cargar la red y los dispositivos de audio.
var AutoBroadcastDelay = 5 * time.Second

// ---------- arranque con Windows ----------

// autostartCommand es la línea para el registro: este .exe, con --autoarranque.
func (a *App) autostartCommand() string {
	exe, _ := os.Executable()
	exe, _ = filepath.Abs(exe)
	return autostart.Command(exe, a.DataDir)
}

// syncAutostart deja el registro de Windows como dice la configuración. Si el
// programa se movió de carpeta, la línea se actualiza sola al abrirlo.
func (a *App) syncAutostart(c config.Config) error {
	if a.RunKey == "" {
		return nil // modo consola o pruebas: no se toca el registro
	}
	if err := autostart.Set(a.RunKey, c.Startup.WithWindows, a.autostartCommand()); err != nil {
		return fmt.Errorf("No se pudo cambiar el arranque con Windows: %w", err)
	}
	return nil
}

// autoBroadcast empieza a emitir solo, si está configurado, al abrir el programa.
func (a *App) autoBroadcast(ctx context.Context) {
	a.mu.Lock()
	c := a.cfg
	a.mu.Unlock()
	if !c.Startup.AutoBroadcast {
		return
	}
	if !c.Radio.Enabled && !c.TV.Enabled {
		a.log("Emitir al abrir: no hay ninguna salida marcada con «Emitir».")
		return
	}
	a.log("Emitir al abrir: se empieza en %s…", AutoBroadcastDelay)
	select {
	case <-ctx.Done():
		return
	case <-time.After(AutoBroadcastDelay):
	}
	a.waitFirstCheck(ctx)
	if !a.authenticated() {
		a.log("Emitir al abrir: el programa no está autenticado (pestaña Ajustes → Datos del panel).")
		return
	}
	a.mu.Lock()
	c = a.cfg
	a.mu.Unlock()
	if err := a.Start("todo", c); err != nil {
		a.log("Emitir al abrir: %v", err)
	}
}

// ---------- ícono junto al reloj ----------

// tray es el ícono del área de notificación y su menú.
type tray struct {
	once    sync.Once
	ready   chan struct{}
	start   *systray.MenuItem
	stop    *systray.MenuItem
	lastTip string
}

// startTray muestra el ícono junto al reloj (hilo propio con su bucle de mensajes de Windows).
func (a *App) startTray() {
	if len(a.TrayIcon) == 0 {
		return
	}
	a.tray.ready = make(chan struct{})
	go func() {
		runtime.LockOSThread() // la ventana del ícono y su bucle deben quedar en el mismo hilo
		systray.Run(a.onTrayReady, func() {})
	}()
}

func (a *App) onTrayReady() {
	systray.SetIcon(a.TrayIcon)
	systray.SetTitle("Club25 Emisor")
	systray.SetTooltip("Club25 Emisor")
	systray.SetOnClick(func(systray.IMenu) { a.ShowWindow() })
	systray.SetOnDClick(func(systray.IMenu) { a.ShowWindow() })
	show := systray.AddMenuItem("Mostrar Club25 Emisor", "Abrir la ventana")
	show.Click(a.ShowWindow)
	systray.AddSeparator()
	a.tray.start = systray.AddMenuItem("▶ Iniciar (radio/TV marcadas)", "Empezar a emitir lo marcado con «Emitir»")
	a.tray.start.Click(func() {
		a.mu.Lock()
		c := a.cfg
		a.mu.Unlock()
		if err := a.Start("todo", c); err != nil {
			a.log("Iniciar desde el ícono: %v", err)
			a.ShowWindow()
		}
	})
	a.tray.stop = systray.AddMenuItem("■ Detener todo", "Detener la radio y la TV")
	a.tray.stop.Click(func() { a.Stop("todo") })
	systray.AddSeparator()
	quit := systray.AddMenuItem("Salir (deja de emitir)", "Detener todo y cerrar el programa")
	quit.Click(a.QuitApp)
	close(a.tray.ready)
	a.updateTray()
}

// updateTray pone el estado en el texto del ícono (al pasar el mouse) y habilita el menú.
func (a *App) updateTray() {
	if a.tray.ready == nil {
		return
	}
	select {
	case <-a.tray.ready:
	default:
		return
	}
	st := a.Statuses()
	label := map[runnerpkg.State]string{runnerpkg.Stopped: "detenida", runnerpkg.Connecting: "conectando",
		runnerpkg.Live: "EN VIVO", runnerpkg.Reconnecting: "reconectando"}
	tip := fmt.Sprintf("Club25 Emisor · Radio %s · TV %s", label[st["radio"].State], label[st["tv"].State])
	if tip != a.tray.lastTip {
		a.tray.lastTip = tip
		systray.SetTooltip(tip)
		running := st["radio"].State != runnerpkg.Stopped || st["tv"].State != runnerpkg.Stopped
		if running {
			a.tray.stop.Enable()
		} else {
			a.tray.stop.Disable()
		}
	}
}

// ---------- ventana ----------

// ShowWindow trae la ventana al frente (desde el ícono o al abrir el programa otra vez).
func (a *App) ShowWindow() {
	if a.ctx == nil {
		return
	}
	wruntime.WindowShow(a.ctx)
	wruntime.WindowUnminimise(a.ctx)
	// Breve «siempre visible» para que Windows la ponga delante de otras ventanas.
	wruntime.WindowSetAlwaysOnTop(a.ctx, true)
	wruntime.WindowSetAlwaysOnTop(a.ctx, false)
}

// QuitApp detiene todo y cierra el programa (menú «Salir» del ícono).
func (a *App) QuitApp() {
	a.quitting.Store(true)
	a.log("Saliendo: se detiene la emisión.")
	a.Stop("todo")
	if a.ctx != nil {
		wruntime.Quit(a.ctx)
	}
}

// closeToBackground decide qué hace la X. true = no cerrar (queda en segundo plano).
func (a *App) closeToBackground(ctx context.Context) bool {
	a.mu.Lock()
	bg := a.cfg.Startup.Background
	a.mu.Unlock()
	if !bg || len(a.TrayIcon) == 0 {
		return false
	}
	wruntime.WindowHide(ctx)
	running := a.radio.Running() || a.tv.Running()
	if running {
		a.log("Ventana cerrada: la emisión sigue en segundo plano (ícono junto al reloj).")
	} else {
		a.log("Ventana cerrada: el programa sigue en segundo plano (ícono junto al reloj).")
	}
	// La primera vez se explica dónde quedó.
	if !a.bgHintShown.Swap(true) {
		go func() {
			msg := "Club25 Emisor sigue funcionando en segundo plano"
			if running {
				msg += " y la emisión continúa"
			}
			msg += ".\n\nPara volver a la ventana: clic en el ícono junto al reloj (o abra el programa otra vez).\n" +
				"Para cerrarlo del todo: clic derecho en el ícono → Salir."
			_, _ = wruntime.MessageDialog(ctx, wruntime.MessageDialogOptions{
				Type: wruntime.InfoDialog, Title: "Club25 Emisor", Message: msg})
		}()
	}
	return true
}

// trayStatusLoop actualiza el texto del ícono cada segundo.
func (a *App) trayStatusLoop(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.updateTray()
		}
	}
}

// startupSummary describe el modo de arranque para el registro.
func startupSummary(c config.Config, autoLaunched bool) string {
	var p []string
	if autoLaunched {
		p = append(p, "abierto por Windows al iniciar sesión")
	}
	if c.Startup.WithWindows {
		p = append(p, "arranca con Windows")
	}
	if c.Startup.AutoBroadcast {
		p = append(p, "emite al abrir")
	}
	if c.Startup.Background {
		p = append(p, "sigue en segundo plano al cerrar la ventana")
	}
	if len(p) == 0 {
		return "arranque manual"
	}
	return strings.Join(p, ", ")
}
