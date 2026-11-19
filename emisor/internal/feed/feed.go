// Package feed separa la CAPTURA del audio de su ENVÍO al servidor.
//
// Cada salida (la radio y el audio de la TV) tiene un Feed: un bus de audio
// interno (loopback.Server: PCM s16le estéreo 48 kHz a ritmo de reloj, con
// silencio si no llega nada) que lee el ffmpeg que codifica y mantiene la
// conexión con Icecast o MediaMTX. Lo que alimenta el bus es intercambiable
// AL AIRE con Switch: micrófono o línea (un ffmpeg de captura DirectShow), audio
// de la PC (WASAPI loopback), tono de prueba o silencio. Cambiar de entrada no
// toca la conexión: los oyentes no se cortan.
//
// No sabe de configuración guardada ni de la ventana: recibe la fuente a usar.
package feed

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os/exec"
	"strings"
	"sync"
	"time"

	"radioclub25/emisor/internal/config"
	"radioclub25/emisor/internal/ffmpeg"
	"radioclub25/emisor/internal/loopback"
	"radioclub25/emisor/internal/runner"
)

// Options configura un Feed.
type Options struct {
	Name   string // "radio" o "tv", para el registro
	FFmpeg string // ruta de ffmpeg (para capturar micrófono o línea)
	OnLog  func(string)
	// NewCmd crea el proceso de captura (reemplazable en tests); por defecto exec.CommandContext.
	NewCmd func(ctx context.Context, name string, args ...string) *exec.Cmd
	// Hide oculta la ventana del proceso (Windows).
	Hide func(*exec.Cmd)
	// Capture abre la captura de una salida de reproducción (por defecto loopback.Capture).
	Capture func(name string, onData func([]byte)) (func(), error)
}

// Feed es el bus de audio de una salida y lo que lo alimenta ahora.
type Feed struct {
	opt Options
	bus *loopback.Server

	mu     sync.Mutex
	src    config.Source
	stopFn func()
	closed bool
}

// New abre el bus (sin fuente: silencio).
func New(opt Options) (*Feed, error) {
	if opt.OnLog == nil {
		opt.OnLog = func(string) {}
	}
	if opt.NewCmd == nil {
		opt.NewCmd = exec.CommandContext
	}
	if opt.Capture == nil {
		opt.Capture = loopback.Capture
	}
	bus, err := loopback.NewServer()
	if err != nil {
		return nil, err
	}
	return &Feed{opt: opt, bus: bus, src: config.Source{Kind: config.SourceNone}}, nil
}

// Addr es la entrada para el ffmpeg que codifica: tcp://127.0.0.1:PUERTO.
func (f *Feed) Addr() string { return f.bus.Addr() }

// Source es la fuente que alimenta el bus ahora.
func (f *Feed) Source() config.Source {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.src
}

// Supports dice si una fuente se puede usar en el bus (todas menos «el audio del video»).
func Supports(s config.Source) bool { return s.Kind != config.SourceFromVideo }

// Switch cambia lo que alimenta el bus, sin tocar la conexión del codificador.
// Si es la misma fuente, no hace nada. Si la nueva no abre (micrófono
// desconectado, por ejemplo), devuelve el error y el bus queda en silencio.
func (f *Feed) Switch(s config.Source) error {
	if !Supports(s) {
		return errors.New("esa fuente no pasa por el bus de audio")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return errors.New("bus cerrado")
	}
	if f.stopFn != nil && sameSource(f.src, s) {
		return nil
	}
	if f.stopFn != nil {
		f.stopFn()
		f.stopFn = nil
	}
	f.src = config.Source{Kind: config.SourceNone}
	stop, err := f.start(s)
	if err != nil {
		return err
	}
	f.src, f.stopFn = s, stop
	f.opt.OnLog(fmt.Sprintf("[%s] entrada de audio: %s (sin reconectar)", f.opt.Name, Describe(s)))
	return nil
}

// Close detiene la fuente y cierra el bus (el codificador lee fin de archivo).
func (f *Feed) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return
	}
	f.closed = true
	if f.stopFn != nil {
		f.stopFn()
		f.stopFn = nil
	}
	f.bus.Close()
}

func sameSource(a, b config.Source) bool { return a.Kind == b.Kind && a.Name == b.Name }

// Describe es el nombre de la fuente para el registro y la ventana.
func Describe(s config.Source) string {
	switch s.Kind {
	case config.SourceDShow:
		return s.Name
	case config.SourceLoopback:
		if s.Name == "" {
			return "audio de la PC (salida predeterminada)"
		}
		return "audio de la PC · " + s.Name
	case config.SourceTest:
		return "tono de prueba"
	case config.SourceIcecast:
		return "radio desde el servidor · " + s.Name
	case config.SourceNone:
		return "silencio"
	}
	return s.Kind
}

// start arranca lo que alimenta el bus. Devuelve la función que lo detiene.
func (f *Feed) start(s config.Source) (func(), error) {
	switch s.Kind {
	case config.SourceNone:
		return func() {}, nil // el bus ya envía silencio
	case config.SourceTest:
		return Tone(f.bus.Write, 1000, -18), nil
	case config.SourceLoopback:
		return f.opt.Capture(s.Name, f.bus.Write)
	case config.SourceDShow:
		if s.Name == "" {
			return nil, errors.New("Elija el micrófono o la entrada de línea.")
		}
		return f.captureDShow(s.Name), nil
	case config.SourceIcecast:
		if s.Name == "" {
			return nil, errors.New("Falta la dirección de la radio en el servidor.")
		}
		return f.capture("la radio desde el servidor", NetCaptureArgs(s.Name)), nil
	}
	return nil, fmt.Errorf("fuente desconocida: %q", s.Kind)
}

// Tone genera un tono (Hz, dBFS) en tiempo real hacia write. Devuelve cómo detenerlo.
func Tone(write func([]byte), freq, dbfs float64) func() {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		const chunk = loopback.SampleRate / 50 // 20 ms
		amp := 32767 * math.Pow(10, dbfs/20)
		var phase float64
		step := 2 * math.Pi * freq / loopback.SampleRate
		t := time.NewTicker(20 * time.Millisecond)
		defer t.Stop()
		buf := make([]byte, chunk*loopback.Channels*2)
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
			for i := 0; i < chunk; i++ {
				v := uint16(int16(amp * math.Sin(phase)))
				phase += step
				binary.LittleEndian.PutUint16(buf[i*4:], v)
				binary.LittleEndian.PutUint16(buf[i*4+2:], v)
			}
			write(append([]byte(nil), buf...))
		}
	}()
	return cancel
}

// captureDShow captura un micrófono o una entrada de línea con un ffmpeg propio
// (PCM hacia el bus). Si el proceso se cae (dispositivo desconectado), lo vuelve
// a abrir cada 3 s mientras siga siendo la fuente elegida; el envío no se corta.
func (f *Feed) captureDShow(device string) func() {
	return f.capture("«"+device+"»", CaptureArgs(device))
}

// capture corre un ffmpeg que entrega PCM al bus (args) y lo reabre cada 3 s si
// se corta, mientras siga siendo la fuente elegida. label es para el registro.
func (f *Feed) capture(label string, args []string) func() {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		lastErr := ""
		for ctx.Err() == nil {
			cmd := f.opt.NewCmd(ctx, f.opt.FFmpeg, args...)
			if f.opt.Hide != nil {
				f.opt.Hide(cmd)
			}
			// Al cambiar de entrada se mata el árbol completo (el ffmpeg de Chocolatey
			// es un lanzador con el ffmpeg real como hijo) y Wait no espera de más.
			cmd.Cancel = func() error { return runner.KillTree(cmd) }
			cmd.WaitDelay = 3 * time.Second
			out, err1 := cmd.StdoutPipe()
			errOut, err2 := cmd.StderrPipe()
			if err := errors.Join(err1, err2); err == nil {
				err = cmd.Start()
				if err == nil {
					var msg strings.Builder
					go func() {
						sc := bufio.NewScanner(errOut)
						for sc.Scan() {
							msg.Reset()
							msg.WriteString(sc.Text())
						}
					}()
					pump(out, f.bus.Write)
					_ = cmd.Wait()
					if ctx.Err() == nil {
						m := strings.TrimSpace(msg.String())
						if h := ffmpeg.Hint(m); h != "" {
							m = h
						}
						if m == "" {
							m = "el dispositivo dejó de entregar audio"
						}
						if m != lastErr {
							f.opt.OnLog(fmt.Sprintf("[%s] la captura de %s se cortó: %s · se reintenta (la emisión sigue con silencio)", f.opt.Name, label, m))
							lastErr = m
						}
					}
				} else if err.Error() != lastErr {
					f.opt.OnLog(fmt.Sprintf("[%s] no se pudo abrir %s: %v", f.opt.Name, label, err))
					lastErr = err.Error()
				}
			}
			select {
			case <-ctx.Done():
			case <-time.After(3 * time.Second):
			}
		}
	}()
	return func() { cancel(); <-done }
}

// CaptureArgs es el ffmpeg que captura un dispositivo DirectShow como PCM
// s16le estéreo 48 kHz por la salida estándar (lo que espera el bus).
func CaptureArgs(device string) []string {
	return []string{"-hide_banner", "-nostdin", "-loglevel", "error",
		"-f", "dshow", "-audio_buffer_size", "50", "-i", "audio=" + device,
		"-f", "s16le", "-ar", fmt.Sprint(loopback.SampleRate), "-ac", fmt.Sprint(loopback.Channels), "pipe:1"}
}

// NetCaptureArgs escucha un stream de audio por http (la radio en Icecast, como
// un oyente) y lo entrega al bus como PCM s16le estéreo 48 kHz. Se reconecta si
// el servidor corta; si la radio no está al aire, Icecast responde 404 y el
// bus sigue en silencio hasta que vuelva (se reintenta cada 3 s).
func NetCaptureArgs(rawURL string) []string {
	return []string{"-hide_banner", "-nostdin", "-loglevel", "error",
		"-reconnect", "1", "-reconnect_streamed", "1", "-reconnect_on_network_error", "1", "-reconnect_delay_max", "5",
		"-rw_timeout", "15000000", "-user_agent", "Club25Emisor",
		"-i", rawURL, "-vn",
		"-f", "s16le", "-ar", fmt.Sprint(loopback.SampleRate), "-ac", fmt.Sprint(loopback.Channels), "pipe:1"}
}

// pump copia la salida del ffmpeg de captura al bus, en trozos de 20 ms.
func pump(r io.Reader, write func([]byte)) {
	buf := make([]byte, loopback.SampleRate/50*loopback.Channels*2)
	for {
		n, err := io.ReadFull(r, buf)
		if n > 0 {
			write(append([]byte(nil), buf[:n]...))
		}
		if err != nil {
			return
		}
	}
}
