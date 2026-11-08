// Package ui contiene los structs que Wails expone al frontend (Svelte).
// Cada método público de App es invocable desde TypeScript (Wails genera
// frontend/wailsjs/go/ui/App.{js,d.ts} al ejecutar `wails dev` o `wails build`).
//
// Regla: aquí no hay lógica de negocio. Los métodos solo traducen entre la
// interfaz y los servicios (internal/service/...) o el motor de audio.
// Los errores de validación llegan al frontend como texto (la promesa se rechaza).
package ui

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"radioclub25/estudio/internal/app"
	"radioclub25/estudio/internal/audio"
	"radioclub25/estudio/internal/platform/config"
	"radioclub25/estudio/internal/version"
)

// App es el punto de entrada del frontend hacia Go.
type App struct {
	ctx  context.Context
	core *app.App
}

// NewApp crea la App sobre la aplicación ya construida.
func NewApp(core *app.App) *App { return &App{core: core} }

// Startup lo llama Wails al abrir la ventana; arranca las tareas en segundo plano.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	a.core.OnLogChange = func(day string) { a.emit("log:changed", day) }
	a.core.Start(ctx)
}

// Shutdown lo llama Wails al cerrar la ventana.
func (a *App) Shutdown(context.Context) { a.core.Close() }

// emit envía un evento al frontend (si la ventana ya arrancó).
func (a *App) emit(name string, data any) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, name, data)
	}
}

// AppInfo es lo que muestra la cabecera y la pantalla de ajustes.
type AppInfo struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	DataDir  string `json:"data_dir"`
	FFmpeg   string `json:"ffmpeg"`
	FFmpegOK bool   `json:"ffmpeg_ok"`
}

// Info devuelve nombre, versión, carpeta de datos y estado de ffmpeg.
func (a *App) Info() AppInfo {
	info := AppInfo{Name: version.Name, Version: version.Version, DataDir: a.core.DataDir, FFmpegOK: a.core.FFmpeg == nil}
	if a.core.FFmpeg != nil {
		info.FFmpeg = a.core.FFmpeg.Error()
	} else {
		info.FFmpeg = a.core.Tools.FFmpeg
	}
	return info
}

// Devices lista las tarjetas de sonido (salida y captura).
func (a *App) Devices() ([]audio.Device, error) { return audio.ListDevices() }

// Settings devuelve la configuración.
func (a *App) Settings() config.Settings { return a.core.Settings() }

// SaveSettings guarda la configuración.
func (a *App) SaveSettings(s config.Settings) error { return a.core.SaveSettings(s) }

// PickFolder abre el diálogo de Windows para elegir una carpeta.
func (a *App) PickFolder(title string) (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: title})
}

// PickFile abre el diálogo para elegir un archivo de audio.
func (a *App) PickFile(title string) (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: title, Filters: []runtime.FileFilter{
		{DisplayName: "Audio", Pattern: "*.mp3;*.wav;*.flac;*.ogg;*.m4a;*.aac;*.wma;*.opus"}}})
}
