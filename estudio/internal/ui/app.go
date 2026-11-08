// Package ui contiene los structs que Wails expone al frontend (Svelte).
// Cada método público de App es invocable desde TypeScript (Wails genera
// frontend/wailsjs/go/ui/App.{js,d.ts} al ejecutar `wails dev` o `wails build`).
//
// Regla: aquí no hay lógica de negocio. Los métodos solo traducen entre la
// interfaz y los servicios (internal/service/...) o el motor de audio.
package ui

import (
	"context"

	"radioclub25/estudio/internal/audio"
	"radioclub25/estudio/internal/version"
)

// App es el punto de entrada del frontend hacia Go.
type App struct {
	ctx context.Context
}

// NewApp crea la App. En próximas fases recibirá los servicios por parámetro.
func NewApp() *App { return &App{} }

// Startup lo llama Wails al abrir la ventana; guarda el contexto del runtime.
func (a *App) Startup(ctx context.Context) { a.ctx = ctx }

// AppInfo es lo que muestra la pantalla de inicio.
type AppInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Info devuelve el nombre y la versión del programa.
func (a *App) Info() AppInfo { return AppInfo{Name: version.Name, Version: version.Version} }

// Devices lista las tarjetas de sonido (salida y captura) para la pantalla de ajustes.
func (a *App) Devices() ([]audio.Device, error) { return audio.ListDevices() }
