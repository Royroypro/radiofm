package ffmpeg

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// SampleReader es de donde el codificador lee el audio (lo cumple *audio.RingBuffer).
// Read no debe bloquear: devuelve las muestras disponibles.
type SampleReader interface {
	Read(p []float32) int
	Len() int
	Discard(n int) int
}

// EncoderConfig son los datos de conexión a Icecast y el formato de salida.
type EncoderConfig struct {
	Host        string
	Port        int
	Mount       string // "/radio"
	User        string // "source"
	Password    string
	Format      string // "mp3" (por defecto) o "aac"
	BitrateKbps int    // 128 por defecto (mp3), 96 (aac)
	Name        string // nombre de la radio (ice_name)
	Description string
	Genre       string
	URL         string // web de la radio
}

func (c EncoderConfig) withDefaults() EncoderConfig {
	if c.Format == "" {
		c.Format = "mp3"
	}
	if c.BitrateKbps <= 0 {
		c.BitrateKbps = 128
		if c.Format == "aac" {
			c.BitrateKbps = 96
		}
	}
	if c.User == "" {
		c.User = "source"
	}
	if c.Mount == "" {
		c.Mount = "/radio"
	}
	if !strings.HasPrefix(c.Mount, "/") {
		c.Mount = "/" + c.Mount
	}
	return c
}

// iceURL arma icecast://usuario:clave@host:puerto/mount con la clave escapada.
func (c EncoderConfig) iceURL() string {
	u := url.URL{Scheme: "icecast", User: url.UserPassword(c.User, c.Password),
		Host: net.JoinHostPort(c.Host, strconv.Itoa(c.Port)), Path: c.Mount}
	return u.String()
}

// args devuelve los argumentos de ffmpeg para codificar PCM de stdin hacia Icecast.
func (c EncoderConfig) args() []string {
	a := []string{"-hide_banner", "-loglevel", "error", "-nostdin",
		"-f", "f32le", "-ar", "44100", "-ac", "2", "-i", "pipe:0"}
	switch c.Format {
	case "aac":
		a = append(a, "-c:a", "aac", "-b:a", fmt.Sprintf("%dk", c.BitrateKbps), "-content_type", "audio/aac", "-f", "adts")
	default:
		a = append(a, "-c:a", "libmp3lame", "-b:a", fmt.Sprintf("%dk", c.BitrateKbps), "-content_type", "audio/mpeg", "-f", "mp3")
	}
	for k, v := range map[string]string{"-ice_name": c.Name, "-ice_description": c.Description,
		"-ice_genre": c.Genre, "-ice_url": c.URL} {
		if v != "" {
			a = append(a, k, v)
		}
	}
	return append(a, c.iceURL())
}

// EncoderStatus es el estado del codificador para la interfaz y el heartbeat.
type EncoderStatus struct {
	Connected bool      `json:"connected"`
	Since     time.Time `json:"since,omitempty"` // conectado desde
	Retries   int       `json:"retries"`         // reintentos desde la última conexión estable
	LastError string    `json:"last_error,omitempty"`
	Kbps      int       `json:"kbps"`
	BytesSent int64     `json:"bytes_sent"`
	Format    string    `json:"format"`
}

// Encoder envía el audio a Icecast con ffmpeg y se reconecta solo con espera
// exponencial (1 s → 60 s) si se cae la red o el servidor rechaza la fuente.
type Encoder struct {
	tools Tools
	cfg   EncoderConfig
	src   SampleReader

	mu     sync.Mutex
	status EncoderStatus
	sent   atomic.Int64

	// MinBackoff y MaxBackoff se pueden cambiar antes de Run (los tests usan valores cortos).
	MinBackoff, MaxBackoff time.Duration
}

// NewEncoder crea el codificador. src suele ser Engine.EncoderOutput().
func (t Tools) NewEncoder(cfg EncoderConfig, src SampleReader) *Encoder {
	cfg = cfg.withDefaults()
	return &Encoder{tools: t, cfg: cfg, src: src, status: EncoderStatus{Format: cfg.Format, Kbps: cfg.BitrateKbps},
		MinBackoff: time.Second, MaxBackoff: time.Minute}
}

// Status devuelve una copia del estado.
func (e *Encoder) Status() EncoderStatus {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := e.status
	s.BytesSent = e.sent.Load()
	return s
}

func (e *Encoder) update(f func(*EncoderStatus)) {
	e.mu.Lock()
	f(&e.status)
	e.mu.Unlock()
}

// Run conecta y mantiene la emisión hasta que ctx se cancele.
func (e *Encoder) Run(ctx context.Context) {
	backoff := e.MinBackoff
	for ctx.Err() == nil {
		started := time.Now()
		err := e.session(ctx)
		if ctx.Err() != nil {
			break
		}
		stable := time.Since(started) > 30*time.Second
		e.update(func(s *EncoderStatus) {
			s.Connected = false
			s.Since = time.Time{}
			if err != nil {
				s.LastError = err.Error()
			}
			if stable {
				s.Retries = 0
			}
			s.Retries++
		})
		if stable {
			backoff = e.MinBackoff
		}
		select {
		case <-ctx.Done():
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, e.MaxBackoff)
	}
	e.update(func(s *EncoderStatus) { s.Connected, s.Since = false, time.Time{} })
}

// session ejecuta un proceso de ffmpeg hasta que termine o falle la escritura.
func (e *Encoder) session(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := command(ctx, e.tools.FFmpeg, e.cfg.args()...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stderr := &limitedBuffer{max: 4096}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("iniciar ffmpeg: %w", err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	// No enviar audio viejo acumulado mientras estábamos desconectados.
	e.src.Discard(e.src.Len())

	samples := make([]float32, 44100*2/5) // hasta 200 ms por escritura
	buf := make([]byte, len(samples)*4)
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	connectedAt := time.Now()
	marked := false
	fail := func(err error) error {
		cancel()
		<-exited
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("%s", lastLine(msg))
		}
		return err
	}
	for {
		select {
		case <-ctx.Done():
			_ = stdin.Close()
			<-exited
			return nil
		case werr := <-exited:
			msg := strings.TrimSpace(stderr.String())
			if msg == "" && werr != nil {
				msg = werr.Error()
			}
			if msg == "" {
				msg = "ffmpeg terminó"
			}
			return fmt.Errorf("%s", lastLine(msg))
		case <-tick.C:
			n := e.src.Read(samples)
			if n == 0 {
				continue
			}
			for i := 0; i < n; i++ {
				binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(samples[i]))
			}
			if _, err := stdin.Write(buf[:n*4]); err != nil {
				return fail(fmt.Errorf("escribir a ffmpeg: %w", err))
			}
			e.sent.Add(int64(n * 4))
			// Se considera conectado tras 3 s enviando sin que ffmpeg termine
			// (si Icecast rechaza la fuente, ffmpeg sale antes).
			if !marked && time.Since(connectedAt) > 3*time.Second {
				marked = true
				e.update(func(s *EncoderStatus) { s.Connected, s.Since, s.LastError = true, time.Now(), "" })
			}
		}
	}
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
