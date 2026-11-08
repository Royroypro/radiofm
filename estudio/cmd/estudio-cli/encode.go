package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"radioclub25/estudio/internal/audio"
	"radioclub25/estudio/internal/platform/ffmpeg"
	"radioclub25/estudio/internal/platform/icecast"
)

// cmdEncode: reproduce una lista con crossfade y la emite a Icecast (con
// metadata). Sin -device no usa tarjeta: el motor avanza con el reloj de software.
//
//	estudio-cli encode -host radioclub25.sehuacho.com -port 8010 -mount /radio [-device X] a.mp3 b.mp3
//
// La contraseña de fuente se pasa con -password o con la variable CLUB25_ICECAST_PASSWORD.
func cmdEncode(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("encode", flag.ContinueOnError)
	fs.SetOutput(stderr)
	host := fs.String("host", "localhost", "servidor Icecast")
	port := fs.Int("port", 8010, "puerto de Icecast")
	mount := fs.String("mount", "/radio", "punto de montaje")
	user := fs.String("user", "source", "usuario de fuente")
	password := fs.String("password", os.Getenv("CLUB25_ICECAST_PASSWORD"), "contraseña de fuente")
	format := fs.String("format", "mp3", "mp3 o aac")
	bitrate := fs.Int("bitrate", 0, "kbps (128 mp3, 96 aac)")
	device := fs.String("device", "", "tarjeta de salida además de Icecast (vacío = sin tarjeta)")
	xfade := fs.Duration("xfade", 3*time.Second, "duración del crossfade")
	name := fs.String("name", "Radio Club 25", "nombre de la radio (ice_name)")
	if err := fs.Parse(args); err != nil || fs.NArg() == 0 || *password == "" {
		fmt.Fprintln(stderr, "uso: estudio-cli encode -host H -port 8010 -mount /radio -password CLAVE [-device X] archivo...")
		fmt.Fprintln(stderr, "     (o la variable CLUB25_ICECAST_PASSWORD en lugar de -password)")
		return 2
	}
	tl, err := ffmpeg.Locate()
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	e := audio.NewEngine(audio.DefaultConfig())
	if *device != "" {
		out, err := audio.OpenOutput(*device, e.Render)
		if err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
		defer out.Close()
		fmt.Fprintln(stdout, "Salida:", out.Device.Name)
	} else {
		go audio.RunClock(ctx, e)
	}

	enc := tl.NewEncoder(ffmpeg.EncoderConfig{Host: *host, Port: *port, Mount: *mount, User: *user, Password: *password,
		Format: *format, BitrateKbps: *bitrate, Name: *name}, e.EncoderOutput())
	go enc.Run(ctx)
	meta := icecast.New(*host, *port, *mount, *user, *password)

	seq := newSequencer(e, tl, fs.Args(), *xfade, stdout)
	seq.onStarted = func(song string) {
		go func() {
			c, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			// Icecast solo acepta metadata con la fuente ya conectada: se reintenta unos segundos.
			for i := 0; i < 10; i++ {
				if err := meta.UpdateSong(c, song); err == nil {
					fmt.Fprintf(stdout, "\nmetadata: %s\n", song)
					return
				}
				time.Sleep(500 * time.Millisecond)
			}
		}()
	}
	if err := seq.loadNext(); err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	// esperar a que el codificador se conecte para no perder el inicio
	for i := 0; i < 50 && !enc.Status().Connected && ctx.Err() == nil; i++ {
		time.Sleep(100 * time.Millisecond)
	}
	seq.startLoaded()

	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		select {
		case ev := <-e.Events():
			seq.handle(ev)
		case <-seq.done:
			fmt.Fprintln(stdout, "\nFin de la lista.")
			cancel()
			time.Sleep(500 * time.Millisecond)
			return 0
		case <-ctx.Done():
			return 0
		case <-tick.C:
			st := enc.Status()
			state := "desconectado"
			if st.Connected {
				state = "conectado"
			}
			fmt.Fprintf(stdout, "\r  Icecast: %s · %d kbps · %d KB enviados · reintentos %d %s   ", state, st.Kbps,
				st.BytesSent/1024, st.Retries, st.LastError)
		}
	}
}
