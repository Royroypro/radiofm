// Comando emisor: «Club25 Emisor», un emisor en vivo sencillo (como BUTT) para
// enviar al servidor de Radio Club 25 audio a Icecast, video a MediaMTX, o ambos.
//
// Está en la raíz del módulo porque Wails v2 compila el paquete main de la
// raíz y embebe frontend/dist desde aquí. La lógica está en internal/.
package main

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"radioclub25/emisor/internal/autostart"
	"radioclub25/emisor/internal/ui"
	"radioclub25/emisor/internal/updater"
)

//go:embed all:frontend/dist
var assets embed.FS

// trayIcon es el ícono junto al reloj (el mismo del programa).
//
//go:embed build/windows/icon.ico
var trayIcon []byte

func main() {
	// -datos: otra carpeta de configuración (por ejemplo, para un segundo perfil o pruebas).
	dataDir := flag.String("datos", "", "carpeta de configuración (por defecto %APPDATA%\\Club25 Emisor)")
	// --autoarranque: lo abre Windows al iniciar sesión; arranca oculto junto al reloj.
	autoLaunch := flag.Bool(strings.TrimLeft(autostart.Flag, "-"), false, "abierto por Windows al iniciar sesión")
	// Tras una actualización: esperar a que cierre la versión anterior y volver a emitir.
	waitPID := flag.Int(updater.FlagWaitPID, 0, "esperar a que termine este proceso (actualización)")
	resume := flag.String(updater.FlagResume, "", "salidas a reanudar: radio, tv o radio,tv")
	flag.Parse()
	if *waitPID > 0 {
		updater.WaitPID(*waitPID, 30*time.Second)
	}
	app := ui.NewApp()
	app.DataDir = *dataDir
	app.AutoLaunched = *autoLaunch
	app.TrayIcon = trayIcon
	app.RunKey = autostart.RunKey
	app.ResumeOutputs = updater.ParseResume(*resume)
	err := wails.Run(&options.App{
		Title:     "Club25 Emisor",
		Width:     1040,
		Height:    760,
		MinWidth:  860,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 17, G: 20, B: 26, A: 1},
		OnStartup:        app.Startup,
		OnShutdown:       app.Shutdown,
		OnBeforeClose:    app.BeforeClose,
		// Una sola copia abierta: dos emisores pelearían por el mismo montaje.
		SingleInstanceLock: &options.SingleInstanceLock{UniqueId: instanceID(*dataDir),
			// Abrirlo otra vez (acceso directo, menú Inicio) muestra la ventana que ya está corriendo.
			OnSecondInstanceLaunch: func(options.SecondInstanceData) { app.ShowWindow() }},
		StartHidden: *autoLaunch, // al iniciar sesión: solo el ícono junto al reloj
		Bind:        []any{app},
		Windows: &windows.Options{
			Theme: windows.Dark,
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

// instanceID es la «instancia única» de Wails: una por carpeta de datos. La
// configuración normal (%APPDATA%) es una sola copia; con -datos se puede abrir
// otro perfil a la vez (por ejemplo, otra frecuencia en la misma PC).
func instanceID(dataDir string) string {
	const base = "club25-emisor-8f3c1d2e"
	if dataDir == "" {
		return base
	}
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		abs = dataDir
	}
	h := sha256.Sum256([]byte(strings.ToLower(abs)))
	return base + "-" + hex.EncodeToString(h[:6])
}
