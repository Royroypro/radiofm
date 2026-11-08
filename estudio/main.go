// Comando estudio: la aplicación de escritorio "Club25 Estudio" (Wails v2).
//
// Este archivo está en la raíz del módulo (y no en cmd/estudio) porque Wails v2
// compila el paquete main de la raíz del proyecto y embebe frontend/dist desde
// aquí. Solo arranca: la composición de servicios está en internal/app y lo
// que ve el frontend en internal/ui.
//
// Argumentos:
//
//	--setup      revisar la PC y descargar del servidor lo que falte (lo usa el instalador) y salir
//	--autostart  arranque con Windows (igual que normal; se registra en el log)
package main

import (
	"context"
	"embed"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"radioclub25/estudio/internal/app"
	"radioclub25/estudio/internal/platform/winsys"
	"radioclub25/estudio/internal/ui"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if hasArg("--setup") {
		os.Exit(setup())
	}
	release, err := winsys.SingleInstance("Club25Estudio")
	if err != nil {
		winsys.Alert("Club25 Estudio", err.Error())
		os.Exit(1)
	}
	defer release()
	cleanExit := crashGuard() // si el programa se cae, el detalle queda en logs\crash.log
	winsys.KeepAwake()        // la PC de la radio no debe suspenderse mientras emite
	defer winsys.AllowSleep()

	core, err := app.New(context.Background(), "")
	if err != nil {
		crashNote("no se pudo iniciar: %v", err)
		winsys.Alert("Club25 Estudio", "No se pudo iniciar: "+err.Error())
		os.Exit(1)
	}
	if hasArg("--autostart") {
		core.Log.Info("arranque automático con Windows")
	}
	front := ui.NewApp(core)
	err = wails.Run(&options.App{
		Title:     "Club25 Estudio",
		Width:     1280,
		Height:    720,
		MinWidth:  1024, // la interfaz se adapta hasta 1024×600
		MinHeight: 600,
		// Arranca maximizada: así la barra de título (minimizar, maximizar, cerrar)
		// siempre queda visible aunque la pantalla sea pequeña.
		WindowStartState: options.Maximised,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 17, G: 19, B: 22, A: 1}, // fondo del tema oscuro
		OnStartup:        front.Startup,
		OnShutdown:       front.Shutdown,
		Bind:             []any{front},
	})
	if err != nil {
		// Normalmente: WebView2 no está instalado o no pudo arrancar.
		core.Log.Error("la ventana no pudo abrirse", "err", err)
		crashNote("la ventana no pudo abrirse: %v", err)
		winsys.Alert("Club25 Estudio", "No se pudo abrir la ventana del programa:\n"+err.Error()+
			"\n\nSuele deberse a que falta Microsoft Edge WebView2. Instálelo desde "+
			"https://go.microsoft.com/fwlink/p/?LinkId=2124703 y vuelva a abrir el programa.")
		os.Exit(1)
	}
	cleanExit()
}

// setup revisa la PC y descarga del servidor las herramientas que falten.
// Devuelve el código de salida (0 = todo listo). Lo llama el instalador.
func setup() int {
	core, err := app.New(context.Background(), "")
	if err != nil {
		winsys.Alert("Club25 Estudio — instalación", "No se pudo preparar la carpeta de datos: "+err.Error())
		return 1
	}
	defer core.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	core.Log.Info("instalación: revisando la PC")
	got, err := core.EnsureTools(ctx, func(name string, done, total int64) {
		if total > 0 && done == total {
			core.Log.Info("instalación: descargado", "tool", name, "bytes", total)
		}
	})
	var lines []string
	for _, c := range core.ToolsStatus() {
		mark := "OK"
		if !c.OK {
			mark = "FALTA"
		}
		lines = append(lines, fmt.Sprintf("%s: %s (%s)", c.Name, mark, c.Detail))
	}
	core.Log.Info("instalación: resultado", "descargado", got, "checks", strings.Join(lines, "; "))
	if err != nil {
		winsys.Alert("Club25 Estudio — instalación",
			"No se pudieron descargar las herramientas desde el servidor:\n"+err.Error()+
				"\n\nEl programa las volverá a intentar descargar al abrirse (necesita internet).")
		return 2
	}
	return 0
}

func hasArg(a string) bool {
	for _, x := range os.Args[1:] {
		if strings.EqualFold(x, a) {
			return true
		}
	}
	return false
}
