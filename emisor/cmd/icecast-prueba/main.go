// Comando icecast-prueba: un Icecast mínimo para probar el emisor sin instalar
// Icecast (ver internal/icefake). Acepta fuentes con usuario y clave, muestra
// cada segundo cuántos datos llegan por montaje y, al salir, guarda lo recibido
// en archivos (para revisarlo con ffprobe o escucharlo).
//
//	icecast-prueba -puerto 8010 -clave CLAVE [-guardar carpeta]
package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"radioclub25/emisor/internal/icefake"
)

func main() {
	port := flag.Int("puerto", 8010, "puerto donde escuchar")
	user := flag.String("usuario", "source", "usuario de fuente")
	pass := flag.String("clave", "", "contraseña de fuente (obligatoria)")
	save := flag.String("guardar", "", "carpeta donde guardar lo recibido al salir")
	flag.Parse()
	if *pass == "" {
		fmt.Fprintln(os.Stderr, "Falta -clave.")
		os.Exit(2)
	}
	srv := icefake.New(*user, *pass)
	var mu sync.Mutex
	totals := map[string]int{}
	srv.OnChunk = func(mount string, total int) { mu.Lock(); totals[mount] = total; mu.Unlock() }
	srv.OnMeta = func(mount, song string) {
		fmt.Printf("%s %s: título «%s»\n", time.Now().Format("15:04:05"), mount, song)
	}
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		fmt.Fprintln(os.Stderr, "No se pudo escuchar:", err)
		os.Exit(1)
	}
	go srv.Serve(ln)
	fmt.Printf("Icecast de prueba escuchando en :%d (usuario %s)\n", *port, *user)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	tick := time.NewTicker(time.Second)
	last := map[string]int{}
	for {
		select {
		case <-stop:
			ln.Close()
			mu.Lock()
			for m := range totals {
				ctype, data, _, _ := srv.Get(m)
				fmt.Printf("%s: %d bytes (%s)\n", m, len(data), ctype)
				if *save != "" {
					_ = os.MkdirAll(*save, 0o755)
					f := filepath.Join(*save, strings.Trim(strings.ReplaceAll(m, "/", "_"), "_")+ext(ctype))
					if err := os.WriteFile(f, data, 0o644); err == nil {
						fmt.Println("  guardado en", f)
					}
				}
			}
			mu.Unlock()
			return
		case <-tick.C:
			mu.Lock()
			for m, t := range totals {
				if t != last[m] {
					fmt.Printf("%s %s: %d bytes (+%.0f kbps)\n", time.Now().Format("15:04:05"), m, t, float64(t-last[m])*8/1000)
					last[m] = t
				}
			}
			if r := srv.Rejected(); r > 0 && r != last["\x00rej"] {
				fmt.Printf("%s rechazos por clave: %d\n", time.Now().Format("15:04:05"), r)
				last["\x00rej"] = r
			}
			mu.Unlock()
		}
	}
}

func ext(ctype string) string {
	switch ctype {
	case "audio/mpeg":
		return ".mp3"
	case "audio/aac", "audio/aacp":
		return ".aac"
	case "audio/ogg", "application/ogg":
		return ".ogg"
	}
	return ".bin"
}
