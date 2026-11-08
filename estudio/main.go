// Comando estudio: la aplicación de escritorio "Club25 Estudio" (Wails v2).
//
// Este archivo está en la raíz del módulo (y no en cmd/estudio) porque Wails v2
// compila el paquete main de la raíz del proyecto y embebe frontend/dist desde
// aquí. Solo arranca: la composición de servicios está en internal/app y lo
// que ve el frontend en internal/ui.
package main

import (
	"context"
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"radioclub25/estudio/internal/app"
	"radioclub25/estudio/internal/ui"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	core, err := app.New(context.Background(), "")
	if err != nil {
		log.Fatal("Club25 Estudio: ", err)
	}
	front := ui.NewApp(core)
	err = wails.Run(&options.App{
		Title:     "Club25 Estudio",
		Width:     1366,
		Height:    820,
		MinWidth:  1100,
		MinHeight: 680,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 11, G: 18, B: 51, A: 1},
		OnStartup:        front.Startup,
		OnShutdown:       front.Shutdown,
		Bind:             []any{front},
	})
	if err != nil {
		log.Fatal("Club25 Estudio: ", err)
	}
}
