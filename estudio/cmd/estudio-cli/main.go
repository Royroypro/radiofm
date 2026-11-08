// Comando estudio-cli: herramientas de línea de comandos de Club25 Estudio
// para probar piezas sin la interfaz gráfica (útil para programadores, agentes
// de IA y diagnóstico en la PC de la radio).
//
// Uso:
//
//	estudio-cli version   muestra la versión
//	estudio-cli devices   lista las tarjetas de sonido (salida y captura)
//	estudio-cli play [-device X] [-xfade 3s] a.mp3 b.mp3 ...   reproduce con crossfade por la tarjeta
//	estudio-cli render [-xfade 3s] salida.wav a.mp3 b.mp3 ...   mezcla sin tarjeta y escribe un WAV
//	estudio-cli encode -host H -port 8010 -mount /radio -password X a.mp3 ...   emite a Icecast con metadata
//
// Próximas fases: analyze (biblioteca), generate-log (playlist del día).
package main

import (
	"fmt"
	"io"
	"os"

	"radioclub25/estudio/internal/audio"
	"radioclub25/estudio/internal/version"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run ejecuta un subcomando y devuelve el código de salida (separado de main para testearlo).
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}
	switch args[0] {
	case "version":
		fmt.Fprintf(stdout, "%s %s\n", version.Name, version.Version)
	case "devices":
		return devices(stdout, stderr)
	case "play":
		return cmdPlay(args[1:], stdout, stderr)
	case "render":
		return cmdRender(args[1:], stdout, stderr)
	case "encode":
		return cmdEncode(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		usage(stdout)
	default:
		fmt.Fprintf(stderr, "comando desconocido: %q\n", args[0])
		usage(stderr)
		return 2
	}
	return 0
}

func devices(stdout, stderr io.Writer) int {
	devs, err := audio.ListDevices()
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	if len(devs) == 0 {
		fmt.Fprintln(stdout, "No se encontraron dispositivos de audio.")
		return 0
	}
	for _, d := range devs {
		mark := " "
		if d.IsDefault {
			mark = "*"
		}
		fmt.Fprintf(stdout, "%s %-8s %s\n", mark, d.Kind, d.Name)
	}
	fmt.Fprintln(stdout, "(* = predeterminado)")
	return 0
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "Uso: estudio-cli <comando>")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Comandos:")
	fmt.Fprintln(w, "  version   muestra la versión")
	fmt.Fprintln(w, "  devices   lista las tarjetas de sonido (salida y captura)")
	fmt.Fprintln(w, "  play      reproduce archivos con crossfade por la tarjeta de sonido")
	fmt.Fprintln(w, "  render    mezcla archivos sin tarjeta y escribe un WAV")
	fmt.Fprintln(w, "  encode    emite archivos a Icecast (con metadata), con o sin tarjeta")
}
