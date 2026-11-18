// Package ui es el puente entre la ventana (frontend/dist, HTML + JS) y la
// lógica: los métodos exportados de *App los llama el frontend
// (window.go.ui.App.*) y el estado en vivo llega al frontend con eventos
// "estado" (10 por segundo) y "registro" (líneas del registro).
//
// Aquí se compone todo: configuración (config), dos supervisores de ffmpeg
// (runner): uno para la radio y otro para la TV, la importación desde el panel (panel).
package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/energye/systray"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"radioclub25/emisor/internal/config"
	"radioclub25/emisor/internal/ffmpeg"
	"radioclub25/emisor/internal/license"
	"radioclub25/emisor/internal/loopback"
	"radioclub25/emisor/internal/panel"
	"radioclub25/emisor/internal/runner"
	"radioclub25/emisor/internal/updater"
)

// Version del emisor (se muestra en la ventana).
//
// Es variable para poder fijarla al compilar:
// -ldflags "-X radioclub25/emisor/internal/ui.Version=1.5.1"
var Version = "1.6.0"

// App es el objeto que ve el frontend.
type App struct {
	ctx context.Context

	mu       sync.Mutex
	cfg      config.Config
	cfgPath  string
	exeDefs  string
	ffmpeg   string // ruta encontrada ("" si falta)
	ffErr    string
	encoders map[string]bool
	devices  []ffmpeg.Device

	radio, tv *runner.Runner
	fd        feeds // bus de audio de cada salida (la entrada se cambia al aire)
	dirty     chan struct{}

	logMu   sync.Mutex
	logs    []string
	logFile *os.File

	// Emit envía un evento a la ventana. Por defecto, los eventos de Wails; el
	// modo consola (cmd/emisor-consola) lo reemplaza para imprimir en pantalla.
	Emit func(ctx context.Context, name string, data any)
	// AutoLaunched: lo abrió Windows al iniciar sesión (--autoarranque).
	AutoLaunched bool
	// TrayIcon (.ico) activa el ícono junto al reloj y el segundo plano. Vacío en modo consola.
	TrayIcon []byte
	// RunKey es la clave del registro para «Iniciar con Windows» (vacía: no se toca el registro).
	RunKey string

	tray tray
	eq   eqState // ganancias del ecualizador en cada ffmpeg al aire
	tt   titles  // nombre de la canción en Icecast
	lic  licState
	// ResumeOutputs: salidas a reanudar al abrir (--reanudar, tras una actualización).
	ResumeOutputs []string
	quitting      atomic.Bool
	bgHintShown   atomic.Bool

	// DataDir es la carpeta de configuración y registro. Vacío: %APPDATA%\Club25 Emisor.
	DataDir string
}

// NewApp crea la aplicación (sin abrir nada todavía).
func NewApp() *App {
	return &App{dirty: make(chan struct{}, 1), Emit: func(ctx context.Context, name string, data any) {
		wruntime.EventsEmit(ctx, name, data)
	}}
}

// Startup lo llama Wails al abrir la ventana.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	exe, _ := os.Executable()
	exeDir := filepath.Dir(exe)
	a.exeDefs = filepath.Join(exeDir, "emisor.defaults.json")
	dir, err := a.DataDir, error(nil)
	if dir == "" {
		dir, err = config.Dir()
	}
	if err == nil {
		a.cfgPath = filepath.Join(dir, "emisor.json")
		_ = os.MkdirAll(dir, 0o700)
		a.openLog(filepath.Join(dir, "emisor.log"))
	}
	updater.CleanupOld(exe) // el .exe anterior que dejó una actualización
	cfg, err := config.Load(a.exeDefs, a.cfgPath)
	if err != nil {
		a.log("No se pudo leer la configuración (%v); se usan los valores por defecto.", err)
		cfg = config.Defaults()
	}
	a.cfg = cfg
	a.findFFmpeg(exeDir)

	a.radio = runner.New(runner.Options{
		Name: "radio", FFmpeg: a.ffmpeg, Args: a.radioArgs, Redact: a.redact,
		OnStatus: func(runner.Status) { a.markDirty() }, OnLog: a.addLog,
	})
	a.tv = runner.New(runner.Options{
		Name: "tv", FFmpeg: a.ffmpeg, Args: a.tvArgs, Redact: a.redact,
		OnStatus: func(runner.Status) { a.markDirty() }, OnLog: a.addLog,
	})
	a.initLicense(ctx, dir)
	go a.emitLoop(ctx)
	go a.refreshDevices()
	go a.titleLoop(ctx)
	a.log("Club25 Emisor %s listo. Configuración: %s", Version, a.cfgPath)
	a.log("Arranque: %s.", startupSummary(a.cfg, a.AutoLaunched))
	if err := a.syncAutostart(a.cfg); err != nil {
		a.log("%v", err)
	}
	if len(a.TrayIcon) > 0 { // solo la ventana: el modo consola decide él qué emitir
		a.startTray()
		go a.trayStatusLoop(ctx)
		if len(a.ResumeOutputs) > 0 {
			go a.resumeOutputs(ctx, a.ResumeOutputs) // tras una actualización: volver a emitir ya
		} else {
			go a.autoBroadcast(ctx)
		}
	}
}

// Shutdown detiene todo al cerrar.
func (a *App) Shutdown(context.Context) {
	a.stopAll()
	if a.tray.ready != nil {
		systray.Quit()
	}
	a.closeFeed("radio")
	a.closeFeed("tv")
	a.logMu.Lock()
	if a.logFile != nil {
		_ = a.logFile.Close()
	}
	a.logMu.Unlock()
}

// BeforeClose pregunta antes de cerrar si hay algo al aire. true = no cerrar.
func (a *App) BeforeClose(ctx context.Context) bool {
	if a.quitting.Load() { // «Salir» del ícono: ya se detuvo todo
		return false
	}
	if a.closeToBackground(ctx) { // «Seguir en segundo plano»: la X solo oculta la ventana
		return true
	}
	if !a.radio.Running() && !a.tv.Running() {
		return false
	}
	res, err := wruntime.MessageDialog(ctx, wruntime.MessageDialogOptions{
		Type: wruntime.QuestionDialog, Title: "Club25 Emisor",
		Message: "Hay una emisión en curso. ¿Detenerla y salir?", Buttons: []string{"Sí", "No"},
		DefaultButton: "No", CancelButton: "No",
	})
	return err == nil && res != "Sí" && res != "Yes"
}

// ---------- ffmpeg y dispositivos ----------

func (a *App) findFFmpeg(exeDir string) {
	p, err := ffmpeg.Locate(a.cfg.FFmpegPath, exeDir)
	if err != nil {
		a.ffErr = err.Error()
		a.log("%s", err)
		return
	}
	a.ffmpeg = p
	cmd := exec.Command(p, "-hide_banner", "-encoders")
	runner.HideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		a.ffErr = "ffmpeg no responde: " + err.Error()
		return
	}
	a.encoders = ffmpeg.ParseEncoders(string(out))
	a.log("ffmpeg: %s", p)
}

func (a *App) refreshDevices() {
	a.mu.Lock()
	ff := a.ffmpeg
	a.mu.Unlock()
	if ff == "" {
		return
	}
	cmd := exec.Command(ff, ffmpeg.ListDevicesArgs()...)
	runner.HideWindow(cmd)
	out, _ := cmd.CombinedOutput() // ffmpeg sale con error a propósito ("dummy")
	list := ffmpeg.ParseDevices(string(out))
	// Adaptadores de reproducción (altavoces…): van en su propia lista, no con los micrófonos.
	if names, err := loopback.ListPlayback(); err == nil {
		for _, n := range names {
			list = append(list, ffmpeg.Device{Name: n, Kind: "playback"})
		}
	} else if loopback.Available {
		a.log("No se pudieron listar las salidas de audio: %v", err)
	}
	a.mu.Lock()
	a.devices = list
	a.mu.Unlock()
	if a.ctx != nil {
		a.Emit(a.ctx, "dispositivos", list)
	}
}

// ---------- lo que llama el frontend ----------

// Option es una opción de una lista desplegable.
type Option struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Disabled bool   `json:"disabled"`
	Note     string `json:"note"`
}

// State es todo lo que necesita la ventana al abrir.
type State struct {
	Version     string             `json:"version"`
	Config      config.Config      `json:"config"`
	ConfigPath  string             `json:"config_path"`
	FFmpeg      string             `json:"ffmpeg"`
	FFmpegError string             `json:"ffmpeg_error"`
	Devices     []ffmpeg.Device    `json:"devices"`
	Codecs      []Option           `json:"codecs"`
	Encoders    []Option           `json:"encoders"`
	Logs        []string           `json:"logs"`
	MaxAudio    int                `json:"max_audio_kbps"`
	MaxVideo    int                `json:"max_video_kbps"`
	License     license.State      `json:"license"`
	Update      UpdateInfo         `json:"update"`
	EQProfiles  []config.EQProfile `json:"eq_profiles"`
	EQBands     [10]int            `json:"eq_bands"`
	NowPlaying  NowPlaying         `json:"now_playing"`
}

// GetState devuelve el estado inicial de la ventana.
func (a *App) GetState() State {
	a.mu.Lock()
	defer a.mu.Unlock()
	codecs := make([]Option, 0, len(config.Codecs))
	for _, c := range config.Codecs {
		o := Option{ID: c, Label: config.CodecLabel(c)}
		lo, hi, _ := config.AudioBitrateRange(c, 2)
		o.Note = fmt.Sprintf("%d–%d kbps", lo, hi)
		if need := requiredEncoder(c); a.encoders != nil && !a.encoders[need] {
			o.Disabled = true
			o.Note = "este ffmpeg no trae " + need + " (ver README)"
		}
		codecs = append(codecs, o)
	}
	encs := make([]Option, 0, len(config.Encoders))
	for _, e := range config.Encoders {
		o := Option{ID: e.ID, Label: e.Label}
		if a.encoders != nil && !a.encoders[e.ID] {
			o.Disabled, o.Note = true, "no disponible en este ffmpeg"
		}
		encs = append(encs, o)
	}
	a.logMu.Lock()
	logs := append([]string(nil), a.logs...)
	a.logMu.Unlock()
	return State{Version: Version, Config: a.cfg, ConfigPath: a.cfgPath, FFmpeg: a.ffmpeg, FFmpegError: a.ffErr,
		Devices: a.devices, Codecs: codecs, Encoders: encs, Logs: logs,
		MaxAudio: config.MaxAudioKbps, MaxVideo: config.MaxVideoKbps,
		EQProfiles: append(append([]config.EQProfile(nil), config.EQPresets...), a.cfg.EQProfiles...),
		EQBands:    config.EQBands, NowPlaying: a.NowPlaying(),
		License: a.LicenseState(), Update: a.updateInfo()}
}

func requiredEncoder(codec string) string {
	switch codec {
	case config.CodecAAC:
		return "aac"
	case config.CodecHEAAC, config.CodecHEAACv2:
		return "libfdk_aac"
	case config.CodecOpus:
		return "libopus"
	case config.CodecVorbis:
		return "libvorbis"
	}
	return "libmp3lame"
}

// RefreshDevices vuelve a listar micrófonos y cámaras (llega por el evento "dispositivos").
func (a *App) RefreshDevices() { go a.refreshDevices() }

// SaveConfig guarda la configuración (no valida: se valida al iniciar cada salida).
// Si una salida está al aire, el cambio se aplica en la próxima reconexión.
func (a *App) SaveConfig(c config.Config) error {
	a.mu.Lock()
	credsChanged := a.cfg.Panel.URL != c.Panel.URL || a.cfg.Panel.Token != c.Panel.Token
	a.mu.Unlock()
	if err := a.saveConfigQuiet(c); err != nil {
		return err
	}
	if credsChanged && a.lic.firstDone != nil {
		go a.CheckLicense()
	}
	a.scheduleBackup()
	return nil
}

// saveConfigQuiet guarda sin disparar la autenticación ni la copia en el servidor.
func (a *App) saveConfigQuiet(c config.Config) error {
	c.Normalize()
	a.mu.Lock()
	titlesChanged := a.cfg.Radio.Titles.Source != c.Radio.Titles.Source || a.cfg.Radio.Titles.File != c.Radio.Titles.File ||
		a.cfg.Radio.Titles.IdleText != c.Radio.Titles.IdleText
	a.cfg = c
	path := a.cfgPath
	a.mu.Unlock()
	if titlesChanged {
		a.resetTitleFile() // se vuelve a leer el archivo de lo que suena con la fuente nueva
	}
	if path == "" {
		return errors.New("No se encontró la carpeta de datos del usuario.")
	}
	if err := config.Save(path, c); err != nil {
		return fmt.Errorf("No se pudo guardar: %w", err)
	}
	return a.syncAutostart(c)
}

// ResetDefaults vuelve a los valores de fábrica conservando las claves ya escritas.
func (a *App) ResetDefaults() config.Config {
	a.mu.Lock()
	old := a.cfg
	a.mu.Unlock()
	c := config.Defaults()
	if b, err := config.Load(a.exeDefs, ""); err == nil {
		c = b
	}
	c.Radio.Password, c.TV.Password, c.Panel.Token = old.Radio.Password, old.TV.Password, old.Panel.Token
	c.Radio.Input, c.TV.Video, c.TV.Audio = old.Radio.Input, old.TV.Video, old.TV.Audio
	return c
}

// ImportFromPanel trae los datos desde la web con el token del estudio.
func (a *App) ImportFromPanel(c config.Config) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	res, err := panel.Import(ctx, &c)
	if err != nil {
		return nil, err
	}
	if err := a.SaveConfig(c); err != nil {
		return nil, err
	}
	a.log("Datos traídos del panel (%s).", res.Station)
	return map[string]any{"config": c, "result": res}, nil
}

// ParsePasted entiende datos pegados desde Admin → Streams y emisión.
func (a *App) ParsePasted(c config.Config, text string) (map[string]any, error) {
	notes, err := panel.ParsePasted(&c, text)
	if err != nil {
		return nil, err
	}
	if err := a.SaveConfig(c); err != nil {
		return nil, err
	}
	return map[string]any{"config": c, "notes": notes}, nil
}

// Start inicia una salida ("radio", "tv" o "todo" = las marcadas con «Emitir»)
// guardando antes la configuración. Ninguna es obligatoria.
func (a *App) Start(which string, c config.Config) error {
	if err := a.SaveConfig(c); err != nil {
		return err
	}
	if err := a.requireAuth(); err != nil {
		return err
	}
	if a.ffmpeg == "" {
		return errors.New(a.ffErr)
	}
	if which == "todo" && !c.Radio.Enabled && !c.TV.Enabled {
		return errors.New("Marque «Emitir» en la radio, en la TV o en ambas.")
	}
	var errs []string
	if which == "radio" || (which == "todo" && c.Radio.Enabled) {
		if err := a.radio.Start(); err != nil {
			errs = append(errs, "Radio: "+err.Error())
		}
	}
	if which == "tv" || (which == "todo" && c.TV.Enabled) {
		if err := a.tv.Start(); err != nil {
			errs = append(errs, "TV: "+err.Error())
		}
	}
	a.markDirty()
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "\n"))
	}
	return nil
}

// Stop detiene una salida ("radio", "tv" o "todo").
func (a *App) Stop(which string) {
	var wg sync.WaitGroup
	for _, x := range []struct {
		name string
		r    *runner.Runner
	}{{"radio", a.radio}, {"tv", a.tv}} {
		if which == x.name || which == "todo" {
			wg.Add(1)
			go func(r *runner.Runner) { defer wg.Done(); r.Stop() }(x.r)
		}
	}
	wg.Wait()
	// Sin salida al aire, se suelta la entrada (micrófono, captura de la PC).
	if !a.radio.Running() {
		a.closeFeed("radio")
	}
	if !a.tv.Running() {
		a.closeFeed("tv")
	}
	a.markDirty()
}

func (a *App) stopAll() {
	if a.radio != nil {
		a.Stop("todo")
	}
}

// Statuses devuelve el estado de las dos salidas.
func (a *App) Statuses() map[string]runner.Status {
	return map[string]runner.Status{"radio": a.radio.Status(), "tv": a.tv.Status()}
}

// ---------- argumentos (los usa runner en cada intento) ----------

func (a *App) radioArgs() ([]string, error) {
	a.mu.Lock()
	c := a.cfg
	a.mu.Unlock()
	if err := c.ValidateRadio(); err != nil {
		return nil, err
	}
	in, err := a.busInput("radio", c.Radio.Input)
	if err != nil {
		return nil, err
	}
	c.Radio.Input = in
	a.eq.set("radio", c.Radio.EQ.Effective())
	return ffmpeg.RadioArgs(c.Radio), nil
}

func (a *App) tvArgs() ([]string, error) {
	a.mu.Lock()
	c := a.cfg
	a.mu.Unlock()
	if err := c.ValidateTV(); err != nil {
		return nil, err
	}
	in, err := a.busInput("tv", c.TV.Audio)
	if err != nil {
		return nil, err
	}
	c.TV.Audio = in
	a.eq.set("tv", c.TV.EQ.Effective())
	return ffmpeg.TVArgs(c.TV), nil
}

func (a *App) redact(s string) string {
	a.mu.Lock()
	c := a.cfg
	a.mu.Unlock()
	return ffmpeg.Redact(s, c.Radio.Password, c.TV.Password, c.Panel.Token)
}

// ---------- eventos hacia la ventana ----------

func (a *App) markDirty() {
	select {
	case a.dirty <- struct{}{}:
	default:
	}
}

// emitLoop envía el estado como mucho 10 veces por segundo.
func (a *App) emitLoop(ctx context.Context) {
	t := time.NewTicker(100 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			select {
			case <-a.dirty:
				a.Emit(ctx, "estado", a.Statuses())
			default:
			}
		}
	}
}

// ---------- registro ----------

const maxLogs = 500

func (a *App) openLog(path string) {
	if st, err := os.Stat(path); err == nil && st.Size() > 2<<20 {
		_ = os.Rename(path, path+".1") // un archivo anterior como respaldo
	}
	if f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
		a.logFile = f
	}
}

func (a *App) log(format string, args ...any) { a.addLog(fmt.Sprintf(format, args...)) }

// addLog agrega una línea al registro visible y al archivo (ya sin claves).
func (a *App) addLog(line string) {
	line = time.Now().Format("15:04:05") + "  " + line
	a.logMu.Lock()
	a.logs = append(a.logs, line)
	if len(a.logs) > maxLogs {
		a.logs = a.logs[len(a.logs)-maxLogs:]
	}
	if a.logFile != nil {
		_, _ = a.logFile.WriteString(time.Now().Format("2006-01-02 ") + line + "\r\n")
	}
	a.logMu.Unlock()
	if a.ctx != nil {
		a.Emit(a.ctx, "registro", line)
	}
}
