// Package app es la raíz de composición de Club25 Estudio: abre la carpeta de
// datos, la configuración, el log y la base; construye los servicios y el
// motor de audio, y arranca/detiene las tareas en segundo plano.
//
// Lo usan main.go (la ventana Wails) y, si hace falta, estudio-cli, para que
// ambos arranquen exactamente lo mismo. Es el único paquete que conoce todas
// las capas a la vez; no contiene reglas de negocio.
package app

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"radioclub25/estudio/internal/audio"
	"radioclub25/estudio/internal/domain"
	"radioclub25/estudio/internal/platform/apiclient"
	"radioclub25/estudio/internal/platform/config"
	"radioclub25/estudio/internal/platform/ffmpeg"
	"radioclub25/estudio/internal/platform/logger"
	"radioclub25/estudio/internal/platform/secrets"
	"radioclub25/estudio/internal/platform/sqlite"
	"radioclub25/estudio/internal/platform/tools"
	"radioclub25/estudio/internal/platform/weather"
	"radioclub25/estudio/internal/platform/winsys"
	"radioclub25/estudio/internal/service/library"
	"radioclub25/estudio/internal/service/onair"
	"radioclub25/estudio/internal/service/reports"
	"radioclub25/estudio/internal/service/rotation"
	"radioclub25/estudio/internal/service/scheduler"
	"radioclub25/estudio/internal/service/spots"
	"radioclub25/estudio/internal/service/streaming"
	syncsvc "radioclub25/estudio/internal/service/sync"
	"radioclub25/estudio/internal/service/timeannounce"
	"radioclub25/estudio/internal/service/voicetrack"
	"radioclub25/estudio/internal/version"
)

// App reúne las piezas construidas.
type App struct {
	DataDir string
	Log     *slog.Logger
	DB      *sql.DB
	Config  *config.Store
	Tools   ffmpeg.Tools
	FFmpeg  error // error al localizar ffmpeg (la app arranca igual y lo muestra)

	Library   *library.Service
	Scheduler *scheduler.Service
	Spots     *spots.Service
	Reports   *reports.Service
	Weather   *weather.Client
	Secrets   *secrets.Store
	Engine    *audio.Engine
	OnAir     *onair.Service
	Voice     *voicetrack.Service // voice tracking (grabar intervenciones entre canciones)
	Streaming *streaming.Service
	Sync      *syncsvc.Service

	audioMu   sync.Mutex
	output    *audio.Stream
	mic       *audio.Stream
	clockStop context.CancelFunc
	audioErr  string

	// OnLogChange se llama (en otra goroutine) cuando cambia la playlist de un día
	// y sus tandas ya se replanificaron. La usa la interfaz para refrescarse.
	OnLogChange func(day string)

	mu       sync.RWMutex
	settings config.Settings

	logFile io.Closer
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// New construye la aplicación. dataDir vacío = config.DataDir().
func New(ctx context.Context, dataDir string) (*App, error) {
	if dataDir == "" {
		dataDir = config.DataDir()
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("carpeta de datos %s: %w", dataDir, err)
	}
	log, logFile, err := logger.New(dataDir+string(os.PathSeparator)+"logs", nil)
	if err != nil {
		return nil, fmt.Errorf("log: %w", err)
	}
	a := &App{DataDir: dataDir, Log: log, logFile: logFile, Config: config.NewStore(dataDir)}
	a.settings, err = a.Config.Load()
	if err != nil {
		log.Error("configuración inválida, se usan los valores por defecto", "err", err)
	}
	a.DB, err = sqlite.Open(ctx, dataDir+string(os.PathSeparator)+"estudio.db")
	if err != nil {
		logFile.Close()
		return nil, err
	}
	a.Tools, a.FFmpeg = ffmpeg.LocateIn(a.ToolsDir("ffmpeg"))
	if a.FFmpeg != nil {
		log.Error("ffmpeg no encontrado", "err", a.FFmpeg)
	}

	a.Library = library.New(sqlite.NewTrackRepo(a.DB), sqlite.NewCategoryRepo(a.DB), a.Tools, log)
	if err := a.Library.EnsureDefaultCategories(ctx); err != nil {
		return nil, err
	}
	a.Scheduler = scheduler.New(sqlite.NewClockRepo(a.DB), sqlite.NewGridRepo(a.DB), sqlite.NewLogRepo(a.DB),
		sqlite.NewTrackRepo(a.DB), sqlite.NewCategoryRepo(a.DB), sqlite.NewPlayRepo(a.DB), a.schedulerConfig)
	if err := a.Scheduler.EnsureDefaults(ctx); err != nil {
		return nil, err
	}
	loc := func() *time.Location { return a.Settings().Location() }
	a.Spots = spots.New(sqlite.NewSpotRepo(a.DB), sqlite.NewTandaRepo(a.DB), sqlite.NewPlayRepo(a.DB),
		sqlite.NewCategoryRepo(a.DB), sqlite.NewTrackRepo(a.DB), a.Scheduler, a.Tools, loc, log)
	a.Spots.SetLimits(func() spots.Limits {
		sc := a.Settings().Schedule
		return spots.Limits{MaxSpots: sc.BreakMaxSpots, MaxLength: time.Duration(sc.BreakMaxSeconds) * time.Second}
	})
	a.Reports = reports.New(sqlite.NewPlayRepo(a.DB), sqlite.NewSpotRepo(a.DB), loc)
	a.Weather = weather.New()
	a.Secrets = secrets.NewStore(dataDir)

	// Motor de audio, aire y emisión
	s := a.Settings()
	ecfg := audio.DefaultConfig()
	ecfg.DuckDepthDB, ecfg.DuckThresholdDB = s.Audio.DuckDepthDB, s.Audio.DuckThresholdDB
	ecfg.DuckAttack = time.Duration(s.Audio.DuckAttackMs) * time.Millisecond
	ecfg.DuckRelease = time.Duration(s.Audio.DuckReleaseMs) * time.Millisecond
	ecfg.SilenceAfter = time.Duration(s.Safety.SilenceSeconds) * time.Second
	a.Engine = audio.NewEngine(ecfg)
	a.Streaming = streaming.New(a.Tools, a.Engine, a.Settings,
		func() (string, error) { return a.Secrets.Get(secrets.IcecastPassword) }, log)
	a.Voice = voicetrack.New(a.Engine, a.Library, a.Scheduler, filepath.Join(dataDir, "voicetracks"),
		func() bool { a.audioMu.Lock(); defer a.audioMu.Unlock(); return a.mic != nil })
	a.OnAir = onair.New(onair.Deps{
		Engine: a.Engine, Log: a.Scheduler, Tracks: sqlite.NewTrackRepo(a.DB), Categories: sqlite.NewCategoryRepo(a.DB),
		Spots: a.Spots, Plays: sqlite.NewPlayRepo(a.DB), Decode: a.decode, Voice: a.Locutions,
		Temperature: func(ctx context.Context) (float64, error) {
			st := a.Settings().Station
			return a.Weather.Current(ctx, st.Latitude, st.Longitude)
		},
		Location: loc, Options: a.onairOptions, Logger: log, Exists: fileExists,
	}, onair.Hooks{
		OnSegment: func(seg onair.Segment) {
			if seg.Path != "" {
				a.Streaming.UpdateSong(seg.Display(), seg.Music)
				a.Sync.NowPlaying(seg.Artist, seg.Title, string(seg.Kind))
			}
		},
		OnPlayEnded: func(p domain.Play, seg onair.Segment) {
			if seg.RemoteID > 0 { // cuña del panel web: emisión para el comprobante
				a.Sync.EnqueueAirplay(context.Background(), seg.RemoteID, p.UUID, p.StartedAt, p.Duration, p.Complete)
			}
		},
	})
	a.Sync = syncsvc.New(syncsvc.Deps{
		ServerURL: func() string { return a.Settings().Server.URL },
		Token:     func() (string, error) { return a.Secrets.Get(secrets.ServerToken) },
		Version:   version.Version,
		Spots:     sqlite.NewSpotRepo(a.DB), Queue: sqlite.NewAirplayQueue(a.DB), KV: sqlite.NewKV(a.DB),
		SpotsDir: dataDir + string(os.PathSeparator) + "spots",
		Probe: func(ctx context.Context, path string) (time.Duration, error) {
			info, err := a.Tools.Probe(ctx, path)
			return info.Duration, err
		},
		ApplyConfig:  a.applyServerConfig,
		SpotsChanged: a.ReplanSoon,
		Mode:         func() string { return string(a.OnAir.Snapshot().Mode) },
		Encoder:      func() any { return a.Streaming.Status().Encoder },
		Log:          log,
	})
	a.OnAir.SetCartRepository(sqlite.NewCartRepo(a.DB))
	// Al cambiar una playlist se replanifican sus tandas. En otra goroutine: OnChange
	// se llama con el programador bloqueado y Replan necesita editar el día.
	a.Scheduler.OnChange = func(day string) {
		go func() {
			if _, err := a.Spots.Replan(context.Background(), day); err != nil {
				a.Log.Error("replanificar tandas", "day", day, "err", err)
			}
			a.OnAir.Refresh()
			if a.OnLogChange != nil {
				a.OnLogChange(day)
			}
		}()
	}
	log.Info("Club25 Estudio iniciado", "datos", dataDir)
	return a, nil
}

// Settings devuelve una copia de la configuración actual.
func (a *App) Settings() config.Settings {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.settings
}

// SaveSettings valida, guarda y aplica la configuración.
func (a *App) SaveSettings(s config.Settings) error {
	if err := a.Config.Save(s); err != nil {
		return err
	}
	if s.Safety.AutoStart != winsys.AutoStartEnabled(AutoStartName) {
		if err := winsys.SetAutoStart(AutoStartName, s.Safety.AutoStart); err != nil {
			a.Log.Error("arranque con Windows", "err", err)
		}
	}
	a.mu.Lock()
	a.settings = s
	a.mu.Unlock()
	return nil
}

// schedulerConfig traduce los ajustes a la configuración del generador de playlist.
func (a *App) schedulerConfig() (scheduler.Config, *time.Location) {
	s := a.Settings()
	c := scheduler.DefaultConfig()
	c.Crossfade = s.Audio.Crossfade()
	c.Tolerance = time.Duration(s.Schedule.FillToleranceMs) * time.Millisecond
	c.Rules = rotation.Rules{ArtistSeparation: time.Duration(s.Rotation.ArtistSeparationMin) * time.Minute,
		TitleSeparation: time.Duration(s.Rotation.TitleSeparationHours) * time.Hour,
		NoSameVoice:     s.Rotation.NoSameVoice, MaxEnergyJump: s.Rotation.MaxEnergyJump,
		MaxTempoJump: float64(s.Rotation.MaxTempoJumpPct), MaxGenreRun: s.Rotation.MaxGenreRun, MaxEraRun: s.Rotation.MaxEraRun}
	return c, s.Location()
}

// Start arranca las tareas en segundo plano: analizador de la biblioteca y
// generación de la playlist de hoy y los próximos días (al arrancar y cada 10 min).
func (a *App) Start(ctx context.Context) {
	ctx, a.cancel = context.WithCancel(ctx)
	if a.FFmpeg == nil {
		a.wg.Add(1)
		go func() { defer a.wg.Done(); a.Library.RunAnalyzer(ctx) }()
	}
	// Elementos que quedaron "sonando" porque el programa se cerró de golpe: ya sonaron.
	if items, err := a.Scheduler.Upcoming(ctx, time.Now().Add(-24*time.Hour), 100000); err == nil {
		for _, it := range items {
			if it.Status == domain.LogPlaying {
				_ = a.Scheduler.SetStatus(ctx, it.ID, domain.LogPlayed)
			}
		}
	}
	// audio, aire y emisión
	a.RestartAudio()
	a.wg.Add(1)
	go func() { defer a.wg.Done(); a.OnAir.Run(ctx) }()
	a.Streaming.Restart(ctx)
	a.wg.Add(1)
	go func() { defer a.wg.Done(); a.Sync.Run(ctx) }()
	// si falta ffmpeg, descargarlo del servidor en segundo plano
	if a.FFmpeg != nil {
		go func() {
			if got, err := a.EnsureTools(ctx, nil); err != nil {
				a.Log.Error("descargar herramientas", "err", err)
			} else if got {
				a.Log.Info("ffmpeg descargado: se usará al reiniciar el programa")
			}
		}()
	}
	if a.Settings().Safety.StartPlaying {
		go func() {
			// esperar a que la playlist de hoy exista
			time.Sleep(2 * time.Second)
			if err := a.OnAir.Start(); err != nil {
				a.Log.Error("arrancar el aire", "err", err)
			}
		}()
	}
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		t := time.NewTicker(10 * time.Minute)
		defer t.Stop()
		for {
			if err := a.Scheduler.EnsureDays(ctx, a.Settings().Schedule.DaysAhead); err != nil && ctx.Err() == nil {
				a.Log.Error("generar playlist", "err", err)
			}
			a.backupDaily(ctx)
			// las cuñas pueden haber cambiado (sincronización, vigencias): replanificar hoy.
			// Es también la reposición: Replan cuenta lo ya emitido, así que lo que no
			// salió en una tanda saltada se repone en las siguientes de su franja.
			if _, err := a.Spots.Replan(ctx, a.Scheduler.DayOf(time.Now())); err != nil && ctx.Err() == nil {
				a.Log.Error("replanificar tandas", "err", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}

// AutoStartName es el nombre del valor de arranque con Windows (clave Run del usuario).
const AutoStartName = "Club25 Estudio"

// ToolsDir es la carpeta donde se guardan las herramientas descargadas del servidor.
func (a *App) ToolsDir(name string) string {
	return a.DataDir + string(os.PathSeparator) + "tools" + string(os.PathSeparator) + name
}

// ToolsStatus revisa la PC y las herramientas.
func (a *App) ToolsStatus() []tools.Check {
	checks := tools.System(a.DataDir)
	ff := tools.Check{Name: "ffmpeg", OK: a.FFmpeg == nil}
	if a.FFmpeg == nil {
		ff.Detail = a.Tools.FFmpeg
	} else {
		ff.Detail = "no encontrado: se descarga del servidor"
	}
	if _, err := ffmpeg.LocateIn(a.ToolsDir("ffmpeg")); err == nil && a.FFmpeg != nil {
		ff.Detail = "descargado: reinicie el programa para usarlo"
	}
	return append(checks, ff)
}

// EnsureTools descarga del servidor las herramientas que falten (ffmpeg).
// Devuelve true si descargó algo (hay que reiniciar el programa para usarlo).
func (a *App) EnsureTools(ctx context.Context, progress func(name string, done, total int64)) (bool, error) {
	if _, err := ffmpeg.LocateIn(a.ToolsDir("ffmpeg")); err == nil {
		return false, nil
	}
	server := a.Settings().Server.URL
	m, err := tools.Fetch(ctx, server)
	if err != nil {
		return false, err
	}
	t, ok := m.Find("ffmpeg")
	if !ok {
		return false, fmt.Errorf("el servidor no publica ffmpeg")
	}
	a.Log.Info("descargando herramienta", "tool", t.Name, "version", t.Version)
	if err := tools.Install(ctx, server, t, a.ToolsDir("ffmpeg"), func(done, total int64) {
		if progress != nil {
			progress(t.Name, done, total)
		}
	}); err != nil {
		return false, err
	}
	return true, nil
}

// backupDaily guarda una copia de la base por día en <datos>ackups (conserva 14).
func (a *App) backupDaily(ctx context.Context) {
	dir := a.DataDir + string(os.PathSeparator) + "backups"
	_ = os.MkdirAll(dir, 0o755)
	name := dir + string(os.PathSeparator) + "estudio-" + time.Now().Format("20060102") + ".db"
	if _, err := os.Stat(name); err == nil {
		return
	}
	if err := sqlite.Backup(ctx, a.DB, name); err != nil {
		a.Log.Error("copia de la base", "err", err)
		return
	}
	a.Log.Info("copia de la base", "archivo", name)
	entries, _ := os.ReadDir(dir)
	var copies []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "estudio-") && strings.HasSuffix(e.Name(), ".db") {
			copies = append(copies, e.Name())
		}
	}
	sort.Strings(copies)
	for len(copies) > 14 {
		_ = os.Remove(dir + string(os.PathSeparator) + copies[0])
		copies = copies[1:]
	}
}

// applyServerConfig guarda los datos de la radio que envía el panel web.
func (a *App) applyServerConfig(c apiclient.Config) {
	s := a.Settings()
	before := s
	if c.Station.Name != "" {
		s.Station.Name = c.Station.Name
	}
	if c.Station.Timezone != "" {
		s.Station.Timezone = c.Station.Timezone
	}
	if c.Station.Latitude != 0 || c.Station.Longitude != 0 {
		s.Station.Latitude, s.Station.Longitude = c.Station.Latitude, c.Station.Longitude
	}
	if c.Icecast.Host != "" {
		s.Icecast.Host, s.Icecast.Port, s.Icecast.Mount = c.Icecast.Host, c.Icecast.Port, c.Icecast.Mount
	}
	if s != before {
		if err := a.SaveSettings(s); err != nil {
			a.Log.Error("guardar configuración del panel", "err", err)
		}
	}
}

// ReplanSoon replanifica las tandas de hoy y mañana en segundo plano.
func (a *App) ReplanSoon() {
	go func() {
		ctx := context.Background()
		today := time.Now().In(a.Settings().Location())
		for i := 0; i < 2; i++ {
			day := today.AddDate(0, 0, i).Format(domain.DayLayout)
			if _, err := a.Spots.Replan(ctx, day); err != nil {
				a.Log.Error("replanificar tandas", "day", day, "err", err)
			}
		}
		a.OnAir.Refresh()
		if a.OnLogChange != nil {
			a.OnLogChange(today.Format(domain.DayLayout))
		}
	}()
}

// decode abre un archivo con ffmpeg como fuente del motor.
func (a *App) decode(ctx context.Context, path string, start time.Duration) (audio.Source, error) {
	if a.FFmpeg != nil {
		return nil, a.FFmpeg
	}
	r, err := a.Tools.Decode(ctx, path, start)
	if err != nil {
		return nil, err
	}
	return audio.NewPCMSource(r), nil
}

// onairOptions traduce los ajustes a las opciones del aire.
func (a *App) onairOptions() onair.Options {
	s := a.Settings()
	o := onair.Options{Crossfade: s.Audio.Crossfade(), HardFade: time.Duration(s.Audio.HardFadeMs) * time.Millisecond,
		BreakFade:    time.Duration(s.Audio.BreakFadeMs) * time.Millisecond,
		TimeAnnounce: !s.Locutions.TimeOff}
	if cats, err := a.Library.Categories(context.Background()); err == nil {
		for _, c := range cats {
			if c.Kind == domain.KindEmergency && o.Emergency == 0 {
				o.Emergency = c.ID
			}
		}
		if o.Emergency == 0 {
			for _, c := range cats {
				if c.Kind == domain.KindFiller {
					o.Emergency = c.ID
				}
			}
		}
	}
	return o
}

// RestartAudio (re)abre la tarjeta de salida y el micrófono según los ajustes. Si la
// salida falla, el motor sigue con el reloj de software (Icecast sigue sonando).
func (a *App) RestartAudio() {
	a.audioMu.Lock()
	defer a.audioMu.Unlock()
	if a.output != nil {
		_ = a.output.Close()
		a.output = nil
	}
	if a.mic != nil {
		_ = a.mic.Close()
		a.mic = nil
	}
	if a.clockStop != nil {
		a.clockStop()
		a.clockStop = nil
	}
	s := a.Settings()
	a.audioErr = ""
	out, err := audio.OpenOutput(s.Audio.OutputDevice, a.Engine.Render)
	if err != nil {
		a.audioErr = "salida de audio: " + err.Error() + " (se emite solo a Icecast)"
		a.Log.Error("abrir salida de audio", "err", err)
		ctx, cancel := context.WithCancel(context.Background())
		a.clockStop = cancel
		go audio.RunClock(ctx, a.Engine)
	} else {
		a.output = out
		a.Log.Info("salida de audio", "dispositivo", out.Device.Name)
	}
	_ = a.Engine.SetMicGain(audio.DBToGain(s.Audio.MicGainDB))
	a.ApplyProcessor()
	if s.Audio.MicDevice != "" {
		mic, err := audio.OpenCapture(s.Audio.MicDevice, a.Engine.MicInput())
		if err != nil {
			a.audioErr += " · micrófono: " + err.Error()
			a.Log.Error("abrir micrófono", "err", err)
		} else {
			a.mic = mic
		}
	}
}

// AudioState describe los dispositivos abiertos.
type AudioState struct {
	Output string `json:"output"`
	Mic    string `json:"mic"`
	Error  string `json:"error"`
}

// Audio devuelve qué dispositivos están abiertos y el último error.
func (a *App) Audio() AudioState {
	a.audioMu.Lock()
	defer a.audioMu.Unlock()
	st := AudioState{Error: a.audioErr}
	if a.output != nil {
		st.Output = a.output.Device.Name
	} else {
		st.Output = "(sin tarjeta: reloj de software)"
	}
	if a.mic != nil {
		st.Mic = a.mic.Device.Name
	}
	return st
}

// Locutions devuelve la ubicación de las locuciones de hora y temperatura según los ajustes.
func (a *App) Locutions() timeannounce.Files {
	l := a.Settings().Locutions
	return timeannounce.Files{Dir: l.Dir, HourPrefix: l.HourPrefix, MinutePrefix: l.MinutePrefix, TempPrefix: l.TempPrefix}
}

// Close detiene todo y cierra la base y el log.
func (a *App) Close() {
	if a.cancel != nil {
		a.cancel()
	}
	a.wg.Wait()
	a.Streaming.Stop()
	a.audioMu.Lock()
	if a.output != nil {
		_ = a.output.Close()
	}
	if a.mic != nil {
		_ = a.mic.Close()
	}
	if a.clockStop != nil {
		a.clockStop()
	}
	a.audioMu.Unlock()
	if a.DB != nil {
		a.DB.Close()
	}
	a.Log.Info("Club25 Estudio cerrado")
	if a.logFile != nil {
		a.logFile.Close()
	}
}

// fileExists dice si un archivo de audio está en el disco (cuñas sin descargar o borradas).
func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

// ApplyProcessor aplica al motor el procesador de audio de los ajustes (preset y
// salida). Se puede llamar en cualquier momento: el cambio entra en el próximo bloque.
func (a *App) ApplyProcessor() {
	s := a.Settings().Audio
	scope := audio.ProcBoth
	if s.ProcScope == "stream" {
		scope = audio.ProcStream
	}
	var p *audio.Processor
	if ps, ok := audio.PresetSettings(audio.ProcPreset(s.ProcPreset)); ok {
		p = audio.NewProcessor(ps)
	}
	_ = a.Engine.SetProcessor(p, scope)
}
