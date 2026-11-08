// Package streaming gestiona la emisión a Icecast: arranca el codificador
// (platform/ffmpeg.Encoder) con los ajustes y la contraseña guardada, elige la
// fuente (mezcla de la PC o entrada de línea con la salida de la consola) y
// actualiza la metadata ("lo que suena") al empezar cada segmento.
package streaming

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"radioclub25/estudio/internal/audio"
	"radioclub25/estudio/internal/platform/config"
	"radioclub25/estudio/internal/platform/ffmpeg"
	"radioclub25/estudio/internal/platform/icecast"
)

// Status es el estado de la emisión para la interfaz y el heartbeat.
type Status struct {
	Enabled     bool                 `json:"enabled"`
	HasPassword bool                 `json:"has_password"`
	Source      string               `json:"source"`
	Target      string               `json:"target"` // host:puerto/mount
	Encoder     ffmpeg.EncoderStatus `json:"encoder"`
	Error       string               `json:"error"`
	Song        string               `json:"song"`
}

// Service es la emisión a Icecast.
type Service struct {
	tools    ffmpeg.Tools
	engine   *audio.Engine
	settings func() config.Settings
	password func() (string, error)
	log      *slog.Logger

	mu      sync.Mutex
	cancel  context.CancelFunc
	enc     *ffmpeg.Encoder
	capture *audio.Stream
	meta    *icecast.Client
	err     string
	song    string
}

// New crea el servicio.
func New(tools ffmpeg.Tools, engine *audio.Engine, settings func() config.Settings,
	password func() (string, error), log *slog.Logger) *Service {
	return &Service{tools: tools, engine: engine, settings: settings, password: password, log: log}
}

// Restart detiene la emisión y la vuelve a arrancar con los ajustes actuales.
func (s *Service) Restart(ctx context.Context) {
	s.Stop()
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg := s.settings().Icecast
	s.err = ""
	if !cfg.Enabled {
		s.engine.SetEncoderTap(false)
		return
	}
	pw, err := s.password()
	if err != nil || pw == "" {
		s.err = "falta la contraseña de Icecast (Ajustes → Emisión)"
		s.engine.SetEncoderTap(false)
		return
	}
	var src ffmpeg.SampleReader = s.engine.EncoderOutput()
	s.engine.SetEncoderTap(true)
	if cfg.Source == "capture" {
		ring := audio.NewRingBuffer(int(audio.DurationToFrames(2*time.Second)) * audio.Channels)
		st, err := audio.OpenCapture(cfg.CaptureDevice, ring)
		if err != nil {
			s.err = "entrada de línea: " + err.Error()
			s.engine.SetEncoderTap(false)
			return
		}
		s.capture, src = st, ring
		s.engine.SetEncoderTap(false)
	}
	station := s.settings().Station.Name
	s.enc = s.tools.NewEncoder(ffmpeg.EncoderConfig{Host: cfg.Host, Port: cfg.Port, Mount: cfg.Mount, User: cfg.User,
		Password: pw, Format: cfg.Format, BitrateKbps: cfg.BitrateKbps, Name: station, Genre: "Variado"}, src)
	s.meta = icecast.New(cfg.Host, cfg.Port, cfg.Mount, cfg.User, pw)
	ctx, s.cancel = context.WithCancel(ctx)
	go s.enc.Run(ctx)
	s.log.Info("emisión a Icecast iniciada", "destino", cfg.Host, "mount", cfg.Mount, "fuente", cfg.Source)
}

// Stop detiene la emisión.
func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	if s.capture != nil {
		_ = s.capture.Close()
		s.capture = nil
	}
	s.enc = nil
}

// Status devuelve el estado.
func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg := s.settings().Icecast
	st := Status{Enabled: cfg.Enabled, Source: cfg.Source, Error: s.err, Song: s.song,
		Target: cfg.Host + ":" + strconv.Itoa(cfg.Port) + cfg.Mount}
	if pw, err := s.password(); err == nil && pw != "" {
		st.HasPassword = true
	}
	if s.enc != nil {
		st.Encoder = s.enc.Status()
	}
	return st
}

// UpdateSong envía la metadata a Icecast. music = false (cuña, jingle, locución)
// envía el nombre de la radio si así se configuró.
func (s *Service) UpdateSong(song string, music bool) {
	cfg := s.settings()
	if !music && cfg.Icecast.MetadataMusic {
		song = cfg.Station.Name
	}
	s.mu.Lock()
	meta := s.meta
	if song == s.song || meta == nil {
		s.mu.Unlock()
		return
	}
	s.song = song
	s.mu.Unlock()
	go func() {
		// Icecast solo acepta metadata con la fuente conectada: unos reintentos.
		for i := 0; i < 6; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := meta.UpdateSong(ctx, song)
			cancel()
			if err == nil {
				return
			}
			time.Sleep(2 * time.Second)
		}
	}()
}
