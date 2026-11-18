// Package updater instala una versión nueva del emisor sin dejar a la radio
// fuera del aire más que unos segundos:
//
//  1. la versión nueva ya está descargada y verificada (internal/license.Download);
//  2. Swap: el .exe en uso se renombra a .old (Windows lo permite aunque esté
//     corriendo) y el nuevo ocupa su lugar; si algo falla, se deshace;
//  3. Relaunch: se abre el .exe nuevo con --esperar-pid (espera a que el viejo
//     cierre, por la instancia única) y --reanudar (vuelve a emitir lo que estaba al aire);
//  4. el programa viejo detiene ffmpeg y se cierra.
//
// Al abrir, CleanupOld borra el .old que haya quedado.
package updater

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Argumentos de la línea de comandos que usa el relanzamiento.
const (
	FlagWaitPID = "esperar-pid" // espera a que termine este proceso antes de abrir la ventana
	FlagResume  = "reanudar"    // salidas a reanudar: "radio", "tv" o "radio,tv"
	FlagData    = "datos"
)

func oldPath(exe string) string { return exe + ".old" }

// Swap pone newExe en el lugar de exe y deja el anterior como exe.old.
func Swap(exe, newExe string) error {
	if _, err := os.Stat(newExe); err != nil {
		return fmt.Errorf("no se encuentra la versión descargada: %w", err)
	}
	_ = os.Remove(oldPath(exe)) // un .old de una actualización anterior
	if err := os.Rename(exe, oldPath(exe)); err != nil {
		return fmt.Errorf("no se pudo apartar la versión actual (¿la carpeta del programa permite escribir?): %w", err)
	}
	if err := os.Rename(newExe, exe); err != nil {
		if rerr := os.Rename(oldPath(exe), exe); rerr != nil {
			return errors.Join(fmt.Errorf("no se pudo instalar la versión nueva: %w", err), rerr)
		}
		return fmt.Errorf("no se pudo instalar la versión nueva: %w", err)
	}
	return nil
}

// CleanupOld borra el .old que dejó una actualización (al abrir la versión nueva).
func CleanupOld(exe string) { _ = os.Remove(oldPath(exe)) }

// RelaunchArgs arma los argumentos del programa nuevo.
func RelaunchArgs(pid int, resume []string, dataDir string) []string {
	args := []string{"--" + FlagWaitPID, strconv.Itoa(pid)}
	if len(resume) > 0 {
		args = append(args, "--"+FlagResume, strings.Join(resume, ","))
	}
	if dataDir != "" {
		args = append(args, "-"+FlagData, dataDir)
	}
	return args
}

// ParseResume interpreta --reanudar ("radio,tv").
func ParseResume(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		switch p = strings.TrimSpace(strings.ToLower(p)); p {
		case "radio", "tv":
			out = append(out, p)
		}
	}
	return out
}

// Relaunch abre exe con args, separado de este proceso.
func Relaunch(exe string, args []string) error {
	cmd := exec.Command(exe, args...)
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("no se pudo abrir la versión nueva: %w", err)
	}
	return cmd.Process.Release()
}

// Newer dice si la versión a es mayor que b ("1.10.0" > "1.9.3").
func Newer(a, b string) bool {
	pa, pb := strings.Split(strings.TrimPrefix(a, "v"), "."), strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := 0; i < 3; i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			return x > y
		}
	}
	return false
}
