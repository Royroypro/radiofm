package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"radioclub25/estudio/internal/platform/config"
	"radioclub25/estudio/internal/platform/winsys"
	"radioclub25/estudio/internal/version"
)

// Registro de caídas.
//
// Club25 Estudio es una app de ventana: si el programa se cae (un panic de Go
// o un error dentro de código nativo como miniaudio o WebView2), Windows no
// muestra nada y el detalle que Go escribe en la salida de errores se pierde.
// crashGuard lo manda a logs\crash.log y, si la vez anterior el programa se
// cayó, lo avisa al abrir de nuevo con la ruta del archivo para enviarlo.
//
// Detecta la caída con una marca: logs\running se crea al abrir y se borra al
// cerrar bien. Si al abrir la marca existe y crash.log es más nuevo que ella,
// la vez anterior terminó en una caída (un corte de luz no escribe crash.log,
// así que no avisa).

const crashMaxBytes = 1 << 20

// crashGuard prepara el registro de caídas. Devuelve la función que hay que
// llamar al cerrar bien el programa.
func crashGuard() (cleanExit func()) {
	dir := filepath.Join(config.DataDir(), "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return func() {}
	}
	crashPath := filepath.Join(dir, "crash.log")
	flag := filepath.Join(dir, "running")

	if fi, err := os.Stat(flag); err == nil {
		if ci, err := os.Stat(crashPath); err == nil && ci.ModTime().After(fi.ModTime()) {
			winsys.Alert("Club25 Estudio",
				"El programa se cerró por un error la última vez que se abrió.\n\n"+
					"El detalle está en:\n"+crashPath+"\n\n"+
					"Envíe ese archivo a soporte. Ahora se abrirá de nuevo.")
		}
	}
	if fi, err := os.Stat(crashPath); err == nil && fi.Size() > crashMaxBytes {
		_ = os.Remove(crashPath)
	}
	_ = os.WriteFile(flag, []byte(time.Now().Format(time.RFC3339)), 0o644)

	f, err := os.OpenFile(crashPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err == nil {
		if err := debug.SetCrashOutput(f, debug.CrashOptions{}); err != nil {
			_ = f.Close()
		}
	}
	// Escribir la marca después de abrir crash.log: así crash.log solo queda
	// más nuevo que la marca si de verdad se escribió una caída.
	now := time.Now()
	_ = os.Chtimes(flag, now, now)
	return func() { _ = os.Remove(flag) }
}

// crashNote deja en crash.log una línea con el contexto de una salida por
// error que no es un panic (por ejemplo, la ventana no pudo crearse).
func crashNote(format string, args ...any) {
	path := filepath.Join(config.DataDir(), "logs", "crash.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s Club25 Estudio %s: %s\n", time.Now().Format(time.RFC3339), version.Version, fmt.Sprintf(format, args...))
}
