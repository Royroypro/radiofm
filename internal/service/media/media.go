// Package media consulta el estado en vivo de la radio (Icecast) y de la TV
// (MediaMTX) y corta las emisiones cuando la licencia lo exige.
//
// El estado se guarda en caché unos segundos: la web pública lo consulta
// cada 15 s por cada visitante y no queremos multiplicar las llamadas.
package media

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"radioclub25/internal/platform/icecast"
	"radioclub25/internal/platform/mediamtx"
)

// RadioSource es lo que se necesita de Icecast.
type RadioSource interface {
	Status(ctx context.Context) (icecast.MountStatus, error)
	KillSource(ctx context.Context) error
}

// TVSource es lo que se necesita de MediaMTX.
type TVSource interface {
	Status(ctx context.Context) (mediamtx.PathStatus, error)
	KickPublisher(ctx context.Context) error
}

// Status es el estado combinado que se muestra en la web y en los paneles.
type Status struct {
	Radio     RadioStatus `json:"radio"`
	TV        TVStatus    `json:"tv"`
	CheckedAt time.Time   `json:"checked_at"`
}

// RadioStatus es el estado de la radio.
type RadioStatus struct {
	Online    bool   `json:"online"`
	Listeners int    `json:"listeners"`
	Title     string `json:"title"`
	Bitrate   int    `json:"bitrate"`
	Error     string `json:"-"`
}

// TVStatus es el estado de la TV.
type TVStatus struct {
	Online  bool     `json:"online"`
	Viewers int      `json:"viewers"`
	Tracks  []string `json:"-"`
	Error   string   `json:"-"`
}

// Service consulta y cachea el estado de los streams.
type Service struct {
	radio RadioSource
	tv    TVSource
	log   *slog.Logger
	ttl   time.Duration

	mu     sync.Mutex
	cached Status
}

// New crea el servicio.
func New(radio RadioSource, tv TVSource, log *slog.Logger) *Service {
	return &Service{radio: radio, tv: tv, log: log, ttl: 5 * time.Second}
}

// Status devuelve el estado (cacheado ttl segundos).
func (s *Service) Status(ctx context.Context) Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Since(s.cached.CheckedAt) < s.ttl {
		return s.cached
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	var st Status
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		r, err := s.radio.Status(ctx)
		if err != nil {
			s.log.Debug("estado de radio no disponible", "err", err)
			st.Radio.Error = err.Error()
			return
		}
		st.Radio = RadioStatus{Online: r.Online, Listeners: r.Listeners, Title: r.Title, Bitrate: r.Bitrate}
	}()
	go func() {
		defer wg.Done()
		t, err := s.tv.Status(ctx)
		if err != nil {
			s.log.Debug("estado de TV no disponible", "err", err)
			st.TV.Error = err.Error()
			return
		}
		st.TV = TVStatus{Online: t.Online, Viewers: t.Viewers, Tracks: t.Tracks}
	}()
	wg.Wait()
	st.CheckedAt = time.Now()
	s.cached = st
	return st
}

// CutStreams desconecta la PC emisora de Icecast y de MediaMTX.
// Implementa license.StreamCutter.
func (s *Service) CutStreams(ctx context.Context) error {
	err := errors.Join(s.radio.KillSource(ctx), s.tv.KickPublisher(ctx))
	s.mu.Lock()
	s.cached = Status{}
	s.mu.Unlock()
	return err
}
