// Comando emisor-consola: el mismo emisor, sin ventana. Sirve para diagnosticar
// la conexión (qué responde el servidor, a qué bitrate sale) y para emitir
// desde una tarea programada. Usa la misma lógica y la misma configuración que
// la ventana (internal/ui): lo que se guarde aquí lo ve la ventana y al revés.
//
// Ejemplos:
//
//	emisor-consola -radio                         # emite la radio con la configuración guardada
//	emisor-consola -radio -tv -segundos 60        # las dos salidas durante un minuto
//	emisor-consola -panel https://radioclub25peru.com -token XXXX -importar
//	emisor-consola -tv -prueba                    # barras y tono de prueba (sin cámara ni micrófono)
//	emisor-consola -dispositivos                  # lista micrófonos y cámaras
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"radioclub25/emisor/internal/config"
	"radioclub25/emisor/internal/ffmpeg"
	"radioclub25/emisor/internal/runner"
	"radioclub25/emisor/internal/ui"
)

func main() {
	var (
		radio    = flag.Bool("radio", false, "emitir la radio (audio → Icecast)")
		tv       = flag.Bool("tv", false, "emitir la TV (video → MediaMTX)")
		secs     = flag.Int("segundos", 0, "detener después de N segundos (0 = hasta Ctrl+C)")
		dataDir  = flag.String("datos", "", "carpeta de configuración (por defecto la de la ventana: %APPDATA%\\Club25 Emisor)")
		panelURL = flag.String("panel", "", "dirección de la web para -importar")
		token    = flag.String("token", "", "token de Admin → Estudio (PC) para -importar")
		importar = flag.Bool("importar", false, "traer servidor, puertos, montaje y clave de la TV desde el panel")
		radioPw  = flag.String("clave-radio", "", "contraseña de fuente de Icecast (se guarda cifrada)")
		prueba   = flag.Bool("prueba", false, "usar tono y barras de prueba en lugar de micrófono y cámara")
		proto    = flag.String("protocolo", "", "rtmp o srt para la TV (vacío: el guardado)")
		codec    = flag.String("formato", "", "formato de la radio: mp3, aac, heaac, heaacv2, opus, vorbis")
		kbps     = flag.Int("kbps", 0, "bitrate de la radio en kbps (0: el guardado)")
		vkbps    = flag.Int("video-kbps", 0, "bitrate de video en kbps (0: el guardado)")
		listDev  = flag.Bool("dispositivos", false, "listar micrófonos y cámaras y salir")
	)
	flag.Parse()

	app := ui.NewApp()
	app.DataDir = *dataDir
	var devOnce sync.Once
	devCh := make(chan []ffmpeg.Device, 1)
	app.Emit = func(_ context.Context, name string, data any) {
		switch name {
		case "registro":
			fmt.Println(data)
		case "dispositivos":
			devOnce.Do(func() { devCh <- data.([]ffmpeg.Device) })
		}
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	app.Startup(ctx)
	defer app.Shutdown(ctx)

	if *listDev {
		select {
		case list := <-devCh:
			for _, d := range list {
				fmt.Printf("%-6s %s\n", d.Kind, d.Name)
			}
		case <-time.After(15 * time.Second):
			fmt.Println("No se pudieron listar los dispositivos.")
		}
		return
	}

	st := app.GetState()
	c := st.Config
	if *panelURL != "" {
		c.Panel.URL = *panelURL
	}
	if *token != "" {
		c.Panel.Token = *token
	}
	if *radioPw != "" {
		c.Radio.Password = *radioPw
	}
	if *importar {
		res, err := app.ImportFromPanel(c)
		if err != nil {
			fail(err)
		}
		c = res["config"].(config.Config)
		r := res["result"]
		fmt.Printf("Datos del panel: %+v\n", r)
	}
	if *prueba {
		c.Radio.Input = config.Source{Kind: config.SourceTest}
		c.TV.Video, c.TV.Audio = config.Source{Kind: config.SourceTest}, config.Source{Kind: config.SourceTest}
	}
	if *proto != "" {
		c.TV.Protocol = *proto
	}
	if *codec != "" {
		c.Radio.Codec = *codec
	}
	if *kbps > 0 {
		c.Radio.BitrateKbps = *kbps
	}
	if *vkbps > 0 {
		c.TV.VideoKbps = *vkbps
	}
	c.Radio.Enabled, c.TV.Enabled = *radio, *tv
	if !*radio && !*tv {
		if err := app.SaveConfig(c); err != nil {
			fail(err)
		}
		fmt.Println("Configuración guardada. Use -radio, -tv o ambos para emitir.")
		return
	}
	// Como la ventana: solo emite autenticado con el dominio y el token de la radio.
	if err := app.SaveConfig(c); err != nil {
		fail(err)
	}
	if st := app.CheckLicense(); !st.Authenticated {
		fail(fmt.Errorf("%s\nContacto del proveedor: %s · %s", ui.ErrLocked, st.Provider.Phone, st.Provider.URL))
	}
	if err := app.Start("todo", c); err != nil {
		fail(err)
	}
	fmt.Printf("Radio → %s:%d%s (%s %d kbps)   TV → %s://%s (%dx%d %d kbps)\n", c.Radio.Host, c.Radio.Port, c.Radio.Mount,
		config.CodecLabel(c.Radio.Codec), c.Radio.BitrateKbps, c.TV.Protocol, c.TV.Host, c.TV.Width, c.TV.Height, c.TV.VideoKbps)

	var deadline <-chan time.Time
	if *secs > 0 {
		deadline = time.After(time.Duration(*secs) * time.Second)
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			fmt.Println("Deteniendo…")
			app.Stop("todo")
			return
		case <-deadline:
			fmt.Println("Tiempo cumplido; deteniendo…")
			app.Stop("todo")
			printStatus(app.Statuses(), *radio, *tv)
			return
		case <-tick.C:
			printStatus(app.Statuses(), *radio, *tv)
		}
	}
}

// printStatus imprime una línea por salida activa: estado, bitrate y nivel.
func printStatus(s map[string]runner.Status, radio, tv bool) {
	var parts []string
	for _, name := range []string{"radio", "tv"} {
		if (name == "radio" && !radio) || (name == "tv" && !tv) {
			continue
		}
		x := s[name]
		p := fmt.Sprintf("%-5s %-12s", strings.ToUpper(name), x.State)
		if x.State == runner.Live {
			p += fmt.Sprintf(" %5.0f kbps %7.1f KB", x.BitrateKbps, float64(x.SentBytes)/1024)
			if name == "tv" {
				p += fmt.Sprintf(" %3.0f fps", x.FPS)
			}
			for _, l := range x.Levels {
				p += fmt.Sprintf(" [%5.1f dB]", l.PeakDB)
			}
		} else if x.Message != "" {
			p += " " + x.Message
			if x.RetryInSec > 0 {
				p += fmt.Sprintf(" (reintento en %d s)", x.RetryInSec)
			}
		}
		parts = append(parts, p)
	}
	fmt.Println(time.Now().Format("15:04:05"), strings.Join(parts, "  |  "))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "Error:", err)
	os.Exit(1)
}
