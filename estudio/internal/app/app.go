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
	"sync"
	"time"

	"radioclub25/estudio/internal/platform/config"
	"radioclub25/estudio/internal/platform/ffmpeg"
	"radioclub25/estudio/internal/platform/logger"
	"radioclub25/estudio/internal/platform/sqlite"
	"radioclub25/estudio/internal/platform/weather"
	"radioclub25/estudio/internal/service/library"
	"radioclub25/estudio/internal/service/reports"
	"radioclub25/estudio/internal/service/rotation"
	"radioclub25/estudio/internal/service/scheduler"
	"radioclub25/estudio/internal/service/spots"
	"radioclub25/estudio/internal/service/timeannounce"
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
	a.Tools, a.FFmpeg = ffmpeg.Locate()
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
	a.Reports = reports.New(sqlite.NewPlayRepo(a.DB), sqlite.NewSpotRepo(a.DB), loc)
	a.Weather = weather.New()
	// Al cambiar una playlist se replanifican sus tandas. En otra goroutine: OnChange
	// se llama con el programador bloqueado y Replan necesita editar el día.
	a.Scheduler.OnChange = func(day string) {
		go func() {
			if _, err := a.Spots.Replan(context.Background(), day); err != nil {
				a.Log.Error("replanificar tandas", "day", day, "err", err)
			}
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
		TitleSeparation: time.Duration(s.Rotation.TitleSeparationHours) * time.Hour}
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
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		t := time.NewTicker(10 * time.Minute)
		defer t.Stop()
		for {
			if err := a.Scheduler.EnsureDays(ctx, a.Settings().Schedule.DaysAhead); err != nil && ctx.Err() == nil {
				a.Log.Error("generar playlist", "err", err)
			}
			// las cuñas pueden haber cambiado (sincronización, vigencias): replanificar hoy
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
	if a.DB != nil {
		a.DB.Close()
	}
	a.Log.Info("Club25 Estudio cerrado")
	if a.logFile != nil {
		a.logFile.Close()
	}
}
