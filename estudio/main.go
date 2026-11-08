// Comando estudio: la aplicación de escritorio "Club25 Estudio" (Wails v2).
//
// Este archivo está en la raíz del módulo (y no en cmd/estudio) porque Wails v2
// compila el paquete main de la raíz del proyecto y embebe frontend/dist desde
// aquí. Por eso es solo la "raíz de composición": construye las piezas, las
// inyecta y arranca la ventana. Nada de lógica de negocio aquí.
package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"radioclub25/estudio/internal/ui"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := ui.NewApp()
	err := wails.Run(&options.App{
		Title:     "Club25 Estudio",
		Width:     1280,
		Height:    800,
		MinWidth:  1024,
		MinHeight: 680,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 10, G: 26, B: 107, A: 1}, // azul marino de la marca (#0A1A6B)
		OnStartup:        app.Startup,
		Bind:             []any{app},
	})
	if err != nil {
		log.Fatal("Club25 Estudio: ", err)
	}
}
