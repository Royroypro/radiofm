// Package runner mantiene vivo un proceso de ffmpeg por salida (radio o TV):
// lo arranca, lee su progreso y sus niveles, y si se cae lo vuelve a lanzar
// con espera creciente (reconexión automática), hasta que se pida detenerlo.
//
// Estados: Detenido → Conectando → En vivo; si el proceso termina o se cuelga
// (sin progreso durante StallTimeout) pasa a Reconectando y vuelve a Conectando.
//
// No sabe nada de radios ni de códecs: recibe los argumentos ya armados
// (paquete ffmpeg). Para los tests, el comando se crea con NewCmd, que se
// puede reemplazar por un proceso falso.
package runner

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"radioclub25/emisor/internal/ffmpeg"
)

// State es el estado visible de una salida.
type State string

// Estados posibles.
const (
	Stopped      State = "detenido"
	Connecting   State = "conectando"
	Live         State = "en_vivo"
	Reconnecting State = "reconectando"
)

// MaxAuthFails es cuántos rechazos de clave seguidos se aceptan antes de dejar de reintentar.
const MaxAuthFails = 3

// Status es lo que la interfaz muestra de una salida.
type Status struct {
	State       State          `json:"state"`
	Message     string         `json:"message"`      // última explicación (error o aviso)
	Attempt     int            `json:"attempt"`      // reintento actual (0 = primera conexión)
	RetryInSec  int            `json:"retry_in_sec"` // segundos para el próximo intento
	LiveSince   time.Time      `json:"live_since"`
	BitrateKbps float64        `json:"bitrate_kbps"`
	SentBytes   int64          `json:"sent_bytes"`
	Speed       float64        `json:"speed"`
	FPS         float64        `json:"fps"`
	DropFrames  int64          `json:"drop_frames"`
	Levels      []ffmpeg.Level `json:"levels"`
}

// Options configura un Runner.
type Options struct {
	Name   string                   // "radio" o "tv", para el registro
	FFmpeg string                   // ruta de ffmpeg.exe
	Args   func() ([]string, error) // se llama en cada intento (toma la configuración actual)
	Redact func(string) string      // oculta claves en el registro
	// OnStatus se llama en cada cambio de estado o de progreso (puede ser frecuente).
	OnStatus func(Status)
	// OnLog recibe líneas para el registro visible.
	OnLog func(string)

	// Ajustes de reconexión (cero = valores por defecto).
	MinBackoff, MaxBackoff time.Duration
	StallTimeout           time.Duration // sin progreso tanto tiempo = colgado
	StableAfter            time.Duration // en vivo tanto tiempo = se reinicia la espera
	StopGrace              time.Duration // tiempo para cerrar ffmpeg con «q» antes de matarlo

	// NewCmd crea el proceso; por defecto exec.CommandContext con la ventana oculta.
	NewCmd func(ctx context.Context, name string, args ...string) *exec.Cmd
}

// Runner supervisa una salida.
type Runner struct {
	opt Options

	mu      sync.Mutex
	status  Status
	cancel  context.CancelFunc
	done    chan struct{}
	stdin   io.Writer     // entrada del ffmpeg que está corriendo (nil si no hay)
	restart chan struct{} // Restart: reconectar ya, con la configuración nueva
}

// restartReason marca que el proceso terminó porque se pidió Restart (no es un error).
const restartReason = "\x00reinicio"

// ErrNotRunning: Send sin un ffmpeg corriendo (el cambio se usará al iniciar).
var ErrNotRunning = errors.New("la salida no está emitiendo")

// Send escribe líneas en la entrada del ffmpeg que está corriendo: comandos
// para cambiar filtros en caliente (ver ffmpeg.EnhanceCommands).
func (r *Runner) Send(lines []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stdin == nil {
		return ErrNotRunning
	}
	for _, l := range lines {
		if _, err := io.WriteString(r.stdin, l); err != nil {
			return err
		}
	}
	return nil
}

// Restart reconecta en seguida (cierra ffmpeg con «q» y lo vuelve a lanzar con
// la configuración actual), sin la espera de una reconexión por error. Sirve
// para aplicar cambios con la salida al aire. Si la salida está detenida, no hace nada.
func (r *Runner) Restart() {
	if !r.Running() {
		return
	}
	select {
	case r.restart <- struct{}{}:
	default: // ya había un reinicio pedido
	}
}

// New crea un Runner detenido.
func New(opt Options) *Runner {
	if opt.MinBackoff == 0 {
		opt.MinBackoff = time.Second
	}
	if opt.MaxBackoff == 0 {
		opt.MaxBackoff = 30 * time.Second
	}
	if opt.StallTimeout == 0 {
		opt.StallTimeout = 20 * time.Second
	}
	if opt.StableAfter == 0 {
		opt.StableAfter = 30 * time.Second
	}
	if opt.StopGrace == 0 {
		opt.StopGrace = 3 * time.Second
	}
	if opt.NewCmd == nil {
		opt.NewCmd = func(ctx context.Context, name string, args ...string) *exec.Cmd {
			cmd := exec.CommandContext(ctx, name, args...)
			hideWindow(cmd)
			return cmd
		}
	}
	if opt.Redact == nil {
		opt.Redact = func(s string) string { return s }
	}
	if opt.OnStatus == nil {
		opt.OnStatus = func(Status) {}
	}
	if opt.OnLog == nil {
		opt.OnLog = func(string) {}
	}
	return &Runner{opt: opt, status: Status{State: Stopped}, restart: make(chan struct{}, 1)}
}

// Status devuelve una copia del estado actual.
func (r *Runner) Status() Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status
}

// Running dice si la salida está activa (en cualquier estado salvo Detenido).
func (r *Runner) Running() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cancel != nil
}

// Start arranca la salida. Si ya estaba activa no hace nada.
// Devuelve el error de Args si la configuración no es válida (no arranca).
func (r *Runner) Start() error {
	if r.Running() {
		return nil
	}
	// Se valida fuera del candado: Args puede consultar el estado de la otra salida.
	if _, err := r.opt.Args(); err != nil {
		return err
	}
	r.mu.Lock()
	if r.cancel != nil {
		r.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel, r.done = cancel, make(chan struct{})
	done := r.done
	r.mu.Unlock()
	go func() {
		r.loop(ctx)
		// Si el bucle terminó solo (clave rechazada), la salida queda detenida y se puede volver a iniciar.
		r.mu.Lock()
		if r.done == done {
			r.cancel, r.done = nil, nil
		}
		r.mu.Unlock()
		close(done)
	}()
	return nil
}

// Stop detiene la salida y espera a que ffmpeg termine.
func (r *Runner) Stop() {
	r.mu.Lock()
	cancel, done := r.cancel, r.done
	r.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	<-done
	r.mu.Lock()
	r.cancel, r.done = nil, nil
	r.mu.Unlock()
	r.update(func(s *Status) { *s = Status{State: Stopped, Message: "Detenido."} })
}

func (r *Runner) update(f func(*Status)) {
	r.mu.Lock()
	f(&r.status)
	s := r.status
	r.mu.Unlock()
	r.opt.OnStatus(s)
}

func (r *Runner) logf(format string, a ...any) {
	r.opt.OnLog(r.opt.Redact(fmt.Sprintf("[%s] ", r.opt.Name) + fmt.Sprintf(format, a...)))
}

// loop es la reconexión: corre ffmpeg, y mientras no se pida parar, lo relanza.
func (r *Runner) loop(ctx context.Context) {
	backoff := r.opt.MinBackoff
	authFails := 0
	for attempt := 0; ; attempt++ {
		r.update(func(s *Status) {
			*s = Status{State: Connecting, Attempt: attempt, Message: s.Message}
			if attempt == 0 {
				s.Message = "Conectando…"
			}
		})
		liveFor, msg := r.runOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		if msg == restartReason { // se pidió aplicar cambios: reconectar ya, sin espera
			r.logf("aplicando cambios: se reconecta con la configuración nueva")
			backoff, authFails = r.opt.MinBackoff, 0
			r.update(func(s *Status) { s.Message = "Aplicando cambios…" })
			attempt = -1
			continue
		}
		if liveFor >= r.opt.StableAfter {
			backoff = r.opt.MinBackoff // estuvo bien un buen rato: reintento rápido
		}
		if msg == "" {
			msg = "ffmpeg terminó sin explicar el motivo."
		}
		// Con la clave rechazada, reintentar no sirve: tras MaxAuthFails seguidos se detiene.
		if ffmpeg.IsAuthHint(msg) && liveFor == 0 {
			authFails++
		} else {
			authFails = 0
		}
		if authFails >= MaxAuthFails {
			r.logf("se detiene: %s (rechazada %d veces seguidas)", msg, authFails)
			r.update(func(s *Status) {
				*s = Status{State: Stopped, Message: fmt.Sprintf("Detenido: %s Corríjala y vuelva a iniciar.", msg)}
			})
			return
		}
		r.logf("se cortó: %s · reintento en %s", msg, backoff.Round(time.Second))
		// Cuenta regresiva visible.
		deadline := time.Now().Add(backoff)
		for left := time.Until(deadline); left > 0; left = time.Until(deadline) {
			secs := int((left + time.Second - 1) / time.Second)
			r.update(func(s *Status) {
				*s = Status{State: Reconnecting, Attempt: attempt + 1, RetryInSec: secs, Message: msg}
			})
			select {
			case <-ctx.Done():
				return
			case <-r.restart: // Aplicar durante la espera: se reintenta ya
				deadline = time.Now()
			case <-time.After(min(left, time.Second)):
			}
		}
		backoff = min(backoff*2, r.opt.MaxBackoff)
	}
}

// runOnce corre un ffmpeg hasta que termina. Devuelve cuánto estuvo en vivo y
// la mejor explicación disponible de por qué terminó.
func (r *Runner) runOnce(ctx context.Context) (liveFor time.Duration, reason string) {
	args, err := r.opt.Args()
	if err != nil {
		return 0, err.Error()
	}
	pctx, kill := context.WithCancel(context.Background()) // se mata a mano tras el cierre amable
	defer kill()
	cmd := r.opt.NewCmd(pctx, r.opt.FFmpeg, args...)
	// Al matar se mata el árbol completo: el ffmpeg de Chocolatey (y otros
	// lanzadores) es un intermediario que abre el ffmpeg real como hijo; si solo
	// muere el intermediario, el hijo sigue emitiendo y con las salidas abiertas.
	cmd.Cancel = func() error { return killTree(cmd) }
	stdin, err1 := cmd.StdinPipe()
	stdout, err2 := cmd.StdoutPipe()
	stderr, err3 := cmd.StderrPipe()
	if err := errors.Join(err1, err2, err3); err != nil {
		return 0, err.Error()
	}
	r.logf("ffmpeg %s", strings.Join(quoteArgs(args), " "))
	if err := cmd.Start(); err != nil {
		return 0, "No se pudo iniciar ffmpeg: " + err.Error()
	}
	// Un Restart pedido antes de este proceso ya quedó cumplido al lanzarlo.
	select {
	case <-r.restart:
	default:
	}
	r.mu.Lock()
	r.stdin = stdin
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.stdin = nil
		r.mu.Unlock()
	}()
	// quit pide a ffmpeg que cierre limpio con «q».
	quit := func() {
		r.mu.Lock()
		r.stdin = nil
		_, _ = io.WriteString(stdin, "q\n")
		_ = stdin.Close()
		r.mu.Unlock()
	}

	var (
		mu         sync.Mutex
		lastMsg    string // último mensaje de ffmpeg
		hint       string // explicación del mensaje más útil
		liveAt     time.Time
		lastSignal = time.Now() // último progreso (para detectar cuelgues)
		lastBytes  int64
	)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { // niveles
		defer wg.Done()
		var lp ffmpeg.LevelParser
		sc := bufio.NewScanner(stdout)
		sc.Split(ffmpeg.SplitLines)
		for sc.Scan() {
			if lv, ok := lp.Feed(sc.Text()); ok {
				r.update(func(s *Status) { s.Levels = lv })
			}
		}
		_, _ = io.Copy(io.Discard, stdout)
	}()
	go func() { // progreso y mensajes
		defer wg.Done()
		var pp ffmpeg.ProgressParser
		sc := bufio.NewScanner(stderr)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		sc.Split(ffmpeg.SplitLines)
		for sc.Scan() {
			line := sc.Text()
			pr, done, isLog := pp.Feed(line)
			if isLog {
				// Ecos de los comandos de MEJORA: solo se anotan si fallan.
				if strings.Contains(line, "Enter command:") ||
					(strings.Contains(line, "Command reply") && strings.Contains(line, "ret:0 ")) {
					continue
				}
				r.logf("%s", strings.TrimSpace(line))
				mu.Lock()
				lastMsg = strings.TrimSpace(line)
				if h := ffmpeg.Hint(line); h != "" {
					hint = h
				}
				mu.Unlock()
				continue
			}
			if !done {
				continue
			}
			mu.Lock()
			// En vivo = ya salieron datos hacia el servidor.
			grew := pr.TotalBytes > lastBytes
			if grew {
				lastSignal = time.Now()
				lastBytes = pr.TotalBytes
			}
			becameLive := grew && liveAt.IsZero()
			if becameLive {
				liveAt = time.Now()
			}
			since := liveAt
			mu.Unlock()
			if becameLive {
				r.logf("en vivo")
			}
			r.update(func(s *Status) {
				if !since.IsZero() {
					s.State, s.LiveSince, s.Message, s.RetryInSec = Live, since, "", 0
				}
				s.BitrateKbps, s.SentBytes, s.Speed, s.FPS, s.DropFrames = pr.BitrateKbps, pr.TotalBytes, pr.Speed, pr.FPS, pr.DropFrames
			})
		}
		_, _ = io.Copy(io.Discard, stderr)
	}()

	exited := make(chan error, 1)
	go func() {
		wg.Wait() // leer todo antes de Wait (regla de os/exec)
		exited <- cmd.Wait()
	}()

	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	var stalled bool
	var waitErr error
loop:
	for {
		select {
		case waitErr = <-exited:
			break loop
		case <-ctx.Done():
			// Cierre amable: «q» hace que ffmpeg cierre la conexión limpia.
			quit()
			select {
			case <-exited:
			case <-time.After(r.opt.StopGrace):
				kill()
				r.waitExit(exited)
			}
			return 0, ""
		case <-r.restart:
			quit()
			select {
			case <-exited:
			case <-time.After(r.opt.StopGrace):
				kill()
				r.waitExit(exited)
			}
			return 0, restartReason
		case <-tick.C:
			mu.Lock()
			quiet := time.Since(lastSignal)
			mu.Unlock()
			if quiet > r.opt.StallTimeout {
				stalled = true
				r.logf("sin datos hace %s: se reinicia", quiet.Round(time.Second))
				kill()
				waitErr = r.waitExit(exited)
				break loop
			}
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if !liveAt.IsZero() {
		liveFor = time.Since(liveAt)
	}
	switch {
	case stalled && hint == "":
		return liveFor, "La conexión se quedó sin avanzar; se reinició."
	case hint != "":
		return liveFor, hint
	case lastMsg != "":
		return liveFor, lastMsg
	case waitErr != nil:
		return liveFor, "ffmpeg terminó: " + waitErr.Error()
	}
	return liveFor, "La conexión terminó."
}

// waitExit espera el final del proceso ya matado, pero no para siempre: si algo
// lo retiene (un proceso hijo que no se pudo cerrar) sigue adelante y lo anota.
func (r *Runner) waitExit(exited <-chan error) error {
	select {
	case err := <-exited:
		return err
	case <-time.After(10 * time.Second):
		r.logf("ffmpeg no terminó tras cerrarlo; se continúa igual")
		return errors.New("ffmpeg no terminó")
	}
}

// quoteArgs pone comillas a los argumentos con espacios (solo para el registro).
func quoteArgs(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		if strings.ContainsAny(a, " \t\"") {
			a = `"` + strings.ReplaceAll(a, `"`, `\"`) + `"`
		}
		out[i] = a
	}
	return out
}
