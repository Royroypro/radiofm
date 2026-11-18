// Package autostart pone o quita a Club25 Emisor del arranque de Windows del
// usuario actual (HKCU\Software\Microsoft\Windows\CurrentVersion\Run): no
// necesita permisos de administrador y no toca a otros usuarios.
//
// Windows ejecuta la línea guardada al iniciar sesión. Lleva --autoarranque
// para que el programa arranque oculto junto al reloj (y empiece a emitir si
// así está configurado).
package autostart

import (
	"strings"
)

// RunKey es la clave del registro (bajo HKEY_CURRENT_USER) de los programas que arrancan con Windows.
const RunKey = `Software\Microsoft\Windows\CurrentVersion\Run`

// ValueName es el nombre de la entrada del emisor en RunKey.
const ValueName = "Club25 Emisor"

// Flag es el argumento que indica que lo abrió Windows al iniciar sesión.
const Flag = "--autoarranque"

// Command arma la línea que ejecutará Windows: el .exe entre comillas, el
// indicador de arranque automático y, si se usa otra carpeta de datos, -datos.
func Command(exe, dataDir string) string {
	cmd := quote(exe) + " " + Flag
	if dataDir != "" {
		cmd += " -datos " + quote(dataDir)
	}
	return cmd
}

func quote(s string) string { return `"` + strings.ReplaceAll(s, `"`, ``) + `"` }
