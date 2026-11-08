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

	"radioclub25/estudio/internal/platform/config"
	"radioclub25/estudio/internal/platform/ffmpeg"
	"radioclub25/estudio/internal/platform/logger"
	"radioclub25/estudio/internal/platform/sqlite"
	"radioclub25/estudio/internal/service/library"
)

// App reúne las piezas construidas.
type App struct {
	DataDir string
	Log     *slog.Logger
	DB      *sql.DB
	Config  *config.Store
	Tools   ffmpeg.Tools
	FFmpeg  error // error al localizar ffmpeg (la app arranca igual y lo muestra)

	Library *library.Service

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

// Start arranca las tareas en segundo plano (analizador de la biblioteca…).
func (a *App) Start(ctx context.Context) {
	ctx, a.cancel = context.WithCancel(ctx)
	if a.FFmpeg == nil {
		a.wg.Add(1)
		go func() { defer a.wg.Done(); a.Library.RunAnalyzer(ctx) }()
	}
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
