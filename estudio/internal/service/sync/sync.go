// Package sync mantiene la PC sincronizada con el panel web (/api/studio/*):
//
//   - configuración de la radio (nombre, zona horaria, coordenadas) cada 10 min;
//   - cuñas cada 2 min (con ETag): descarga los audios verificando el SHA-256,
//     actualiza las locales y desactiva las que el panel ya no envía;
//   - heartbeat cada 30 s (versión, modo, estado del codificador y estado completo);
//   - órdenes del panel (control remoto) cada 5 s;
//   - "lo que suena" al cambiar;
//   - emisiones de cuñas: cola local (SQLite) que se envía cada 30 s o al
//     momento; si no hay red se acumula y se envía al volver (sin duplicar).
//
// Si el panel responde 503 (licencia suspendida) o 401 (token revocado), la
// emisión sigue normal: solo se muestra el aviso.
package sync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	stdsync "sync"
	"time"

	"radioclub25/estudio/internal/domain"
	"radioclub25/estudio/internal/platform/apiclient"
	"radioclub25/estudio/internal/platform/sqlite"
)

// Queue es la cola local de emisiones (sqlite.AirplayQueue).
type Queue interface {
	Enqueue(ctx context.Context, a sqlite.QueuedAirplay) error
	Batch(ctx context.Context, limit int) ([]sqlite.QueuedAirplay, error)
	Remove(ctx context.Context, uuids []string) error
	Count(ctx context.Context) (int, error)
}

// KV guarda el ETag de cuñas (sqlite.KV).
type KV interface {
	Get(ctx context.Context, key string) string
	Set(ctx context.Context, key, value string) error
}

// Deps son las dependencias.
type Deps struct {
	ServerURL    func() string
	Token        func() (string, error)
	Version      string
	Spots        domain.SpotRepository
	Queue        Queue
	KV           KV
	SpotsDir     string
	Probe        func(ctx context.Context, path string) (time.Duration, error)
	ApplyConfig  func(apiclient.Config) // guarda nombre, zona horaria, coordenadas, Icecast
	SpotsChanged func()                 // replanificar tandas
	Mode         func() string
	Encoder      func() any
	Status       func() any // estado completo para el panel (puede ser nil)
	// Command ejecuta una orden del panel (control remoto) y devuelve el
	// resultado; nil = la PC no acepta control remoto.
	Command func(kind, arg string) (string, error)
	Log     *slog.Logger
}

// Status es el estado de la sincronización para la interfaz.
type Status struct {
	Configured   bool      `json:"configured"`
	Online       bool      `json:"online"`
	Suspended    bool      `json:"suspended"`
	Unauthorized bool      `json:"unauthorized"`
	LastOK       time.Time `json:"last_ok"`
	LastError    string    `json:"last_error"`
	Queue        int       `json:"queue"`
	Spots        int       `json:"spots"`
	LastSpots    time.Time `json:"last_spots"`
	ClockOffset  float64   `json:"clock_offset"` // segundos que el reloj de la PC va adelantado (+) o atrasado (−) respecto al servidor
}

// Service es la sincronización.
type Service struct {
	d    Deps
	wake chan struct{}

	mu       stdsync.Mutex
	status   Status
	np       *apiclient.NowPlaying
	npSent   string
	force    bool
	lastFull time.Time
}

// New crea el servicio.
func New(d Deps) *Service { return &Service{d: d, wake: make(chan struct{}, 1)} }

func (s *Service) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// SyncNow pide sincronizar todo ahora (botón "Sincronizar").
func (s *Service) SyncNow() {
	s.mu.Lock()
	s.force = true
	s.mu.Unlock()
	s.signal()
}

// NowPlaying registra lo que suena; se envía en segundo plano.
func (s *Service) NowPlaying(artist, title, kind string) {
	s.mu.Lock()
	s.np = &apiclient.NowPlaying{Artist: artist, Title: title, Kind: kind}
	s.mu.Unlock()
	s.signal()
}

// EnqueueAirplay guarda una emisión de cuña del panel para enviarla.
func (s *Service) EnqueueAirplay(ctx context.Context, remoteSpotID int64, uuid string, at time.Time, d time.Duration, complete bool) {
	if remoteSpotID <= 0 {
		return
	}
	if err := s.d.Queue.Enqueue(ctx, sqlite.QueuedAirplay{PlayUUID: uuid, SpotID: remoteSpotID, AiredAt: at, Duration: d,
		Complete: complete}); err != nil {
		s.d.Log.Error("encolar emisión", "err", err)
	}
	s.signal()
}

// Status devuelve el estado.
func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.status
	if n, err := s.d.Queue.Count(context.Background()); err == nil {
		st.Queue = n
	}
	return st
}

func (s *Service) client() (*apiclient.Client, error) {
	url := s.d.ServerURL()
	tok, err := s.d.Token()
	if url == "" || tok == "" || err != nil {
		return nil, errors.New("falta configurar la URL del panel y el token (Ajustes → Sincronización)")
	}
	return apiclient.New(url, tok, s.d.Version), nil
}

// Run sincroniza hasta que ctx se cancele.
func (s *Service) Run(ctx context.Context) {
	var lastConfig, lastSpots, lastHeartbeat, lastFlush time.Time
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		now := time.Now()
		s.mu.Lock()
		force := s.force
		s.force = false
		s.mu.Unlock()
		c, err := s.client()
		s.mu.Lock()
		s.status.Configured = err == nil
		s.mu.Unlock()
		if err == nil {
			if force || now.Sub(lastConfig) >= 10*time.Minute {
				if s.run(ctx, func() error { return s.syncConfig(ctx, c) }) {
					lastConfig = now
				}
			}
			if force || now.Sub(lastSpots) >= 2*time.Minute {
				if s.run(ctx, func() error { return s.syncSpots(ctx, c, force) }) {
					lastSpots = now
				}
			}
			// control remoto: órdenes del panel (cada 5 s); después de ejecutar alguna,
			// el heartbeat sale enseguida para que el panel vea el cambio
			if s.d.Command != nil {
				if n := s.commands(ctx, c); n > 0 {
					lastHeartbeat = time.Time{}
				}
			}
			if force || now.Sub(lastHeartbeat) >= 30*time.Second {
				if s.run(ctx, func() error { return s.heartbeat(ctx, c) }) {
					lastHeartbeat = now
				}
			}
			s.run(ctx, func() error { return s.sendNowPlaying(ctx, c) })
			// la cola se envía en cuanto hay algo; si falla (sin red), se reintenta cada 15 s
			if n, _ := s.d.Queue.Count(ctx); n > 0 && (force || now.Sub(lastFlush) >= 15*time.Second) {
				if !s.run(ctx, func() error { return s.flush(ctx, c) }) {
					lastFlush = now
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		case <-s.wake:
		}
	}
}

// run ejecuta f y actualiza el estado de conexión. Devuelve true si fue bien.
func (s *Service) run(ctx context.Context, f func() error) bool {
	err := f()
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil {
		s.status.Online, s.status.Suspended, s.status.Unauthorized = true, false, false
		s.status.LastOK, s.status.LastError = time.Now(), ""
		return true
	}
	if ctx.Err() != nil {
		return false
	}
	s.status.Online = false
	s.status.Suspended = errors.Is(err, apiclient.ErrSuspended)
	s.status.Unauthorized = errors.Is(err, apiclient.ErrUnauthorized)
	if s.status.LastError != err.Error() {
		s.d.Log.Warn("sincronización", "err", err)
	}
	s.status.LastError = err.Error()
	return false
}

func (s *Service) syncConfig(ctx context.Context, c *apiclient.Client) error {
	sent := time.Now()
	cfg, err := c.GetConfig(ctx)
	if err != nil {
		return err
	}
	if !cfg.ServerTime.IsZero() {
		// el servidor responde en segundos enteros: se compara con el punto medio de la petición
		mid := sent.Add(time.Since(sent) / 2)
		off := mid.Sub(cfg.ServerTime).Seconds()
		s.mu.Lock()
		s.status.ClockOffset = off
		s.mu.Unlock()
		if off > 5 || off < -5 {
			s.d.Log.Warn("el reloj de la PC no coincide con el del servidor", "segundos", off)
		}
	}
	if s.d.ApplyConfig != nil {
		s.d.ApplyConfig(cfg)
	}
	return nil
}

func (s *Service) heartbeat(ctx context.Context, c *apiclient.Client) error {
	hb := apiclient.Heartbeat{Version: s.d.Version, Mode: s.d.Mode(), Encoder: s.d.Encoder()}
	if s.d.Status != nil {
		hb.Status = s.d.Status()
	}
	return c.SendHeartbeat(ctx, hb)
}

func (s *Service) sendNowPlaying(ctx context.Context, c *apiclient.Client) error {
	s.mu.Lock()
	np := s.np
	sent := s.npSent
	s.mu.Unlock()
	if np == nil {
		return nil
	}
	key := np.Kind + "|" + np.Artist + "|" + np.Title
	if key == sent {
		return nil
	}
	if err := c.SendNowPlaying(ctx, *np); err != nil {
		return err
	}
	s.mu.Lock()
	s.npSent = key
	s.mu.Unlock()
	return nil
}

// flush envía la cola de emisiones en lotes de 200.
func (s *Service) flush(ctx context.Context, c *apiclient.Client) error {
	for i := 0; i < 20; i++ {
		batch, err := s.d.Queue.Batch(ctx, 200)
		if err != nil || len(batch) == 0 {
			return err
		}
		req := make([]apiclient.Airplay, len(batch))
		for j, a := range batch {
			req[j] = apiclient.Airplay{SpotID: a.SpotID, AiredAt: a.AiredAt.UTC(), DurationMs: a.Duration.Milliseconds(),
				Complete: a.Complete, PlayUUID: a.PlayUUID}
		}
		res, err := c.SendAirplays(ctx, req)
		if err != nil {
			return err
		}
		// insertadas, ignoradas (ya estaban o la cuña no existe) y rechazadas: ninguna se reenvía
		uuids := make([]string, len(batch))
		for j, a := range batch {
			uuids[j] = a.PlayUUID
		}
		if err := s.d.Queue.Remove(ctx, uuids); err != nil {
			return err
		}
		if len(res.Rejected) > 0 {
			s.d.Log.Warn("emisiones rechazadas por el panel", "uuids", res.Rejected)
		}
	}
	return nil
}

// syncSpots descarga la lista de cuñas y deja las locales iguales al panel.
func (s *Service) syncSpots(ctx context.Context, c *apiclient.Client, force bool) error {
	etag := s.d.KV.Get(ctx, "spots_etag")
	full := force || time.Since(s.lastFull) > 30*time.Minute
	if full {
		etag = "" // cada tanto se pide todo, por si se borró un archivo local
	}
	remote, newTag, notModified, err := c.GetSpots(ctx, etag)
	if err != nil {
		return err
	}
	if notModified {
		return nil
	}
	if full {
		s.lastFull = time.Now()
	}
	changed := false
	seen := map[int64]bool{}
	var firstErr error
	for _, r := range remote {
		seen[r.ID] = true
		ch, err := s.syncSpot(ctx, c, r)
		if err != nil && firstErr == nil {
			firstErr = fmt.Errorf("cuña %q: %w", r.Name, err)
		}
		changed = changed || ch
	}
	local, err := s.d.Spots.List(ctx)
	if err != nil {
		return err
	}
	count := 0
	for _, l := range local {
		if l.RemoteID == 0 {
			continue
		}
		if !seen[l.RemoteID] && l.Active {
			l.Active = false // el panel ya no la envía (vencida, pausada o borrada)
			if err := s.d.Spots.Save(ctx, &l); err != nil {
				return err
			}
			changed = true
		}
		if l.Active || seen[l.RemoteID] {
			count++
		}
	}
	s.mu.Lock()
	s.status.Spots, s.status.LastSpots = count, time.Now()
	s.mu.Unlock()
	if changed && s.d.SpotsChanged != nil {
		s.d.SpotsChanged()
	}
	if firstErr != nil {
		return firstErr // no se guarda el ETag: se reintenta la próxima vez
	}
	return s.d.KV.Set(ctx, "spots_etag", newTag)
}

func (s *Service) syncSpot(ctx context.Context, c *apiclient.Client, r apiclient.Spot) (bool, error) {
	local, err := s.d.Spots.ByRemoteID(ctx, r.ID)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return false, err
	}
	if local == nil || errors.Is(err, domain.ErrNotFound) {
		local = &domain.Spot{RemoteID: r.ID}
	}
	ext := path.Ext(r.URL)
	if ext == "" || len(ext) > 5 {
		ext = ".mp3"
	}
	dst := filepath.Join(s.d.SpotsDir, fmt.Sprintf("%d-%s%s", r.ID, short(r.SHA256), ext))
	_, statErr := os.Stat(dst)
	needFile := local.Path != dst || statErr != nil || local.SHA256 != r.SHA256
	if needFile {
		if err := c.Download(ctx, r.URL, dst, r.SHA256); err != nil {
			return false, err
		}
		if local.Path != "" && local.Path != dst {
			_ = os.Remove(local.Path) // archivo de la versión anterior
		}
	}
	duration := time.Duration(r.DurationMs) * time.Millisecond
	if duration <= 0 && s.d.Probe != nil {
		if d, err := s.d.Probe(ctx, dst); err == nil {
			duration = d
		}
	}
	upd := *local
	upd.Advertiser, upd.Name, upd.Path, upd.SHA256 = r.Advertiser, r.Name, dst, r.SHA256
	upd.Duration, upd.StartsAt, upd.EndsAt = duration, r.StartsAt, r.EndsAt
	upd.PlaysPerDay, upd.HourFrom, upd.HourTo, upd.Active = r.PlaysPerDay, r.HourFrom, r.HourTo, true
	upd.CampaignID, upd.Weight, upd.DaysMask, upd.Position = r.CampaignID, max(r.Weight, 1), r.Days, r.Position
	upd.Priority, upd.Sector = r.Priority, r.Sector
	if !needFile && same(*local, upd) {
		return false, nil
	}
	return true, s.d.Spots.Save(ctx, &upd)
}

func same(a, b domain.Spot) bool {
	eqT := func(x, y *time.Time) bool { return (x == nil && y == nil) || (x != nil && y != nil && x.Equal(*y)) }
	return a.Advertiser == b.Advertiser && a.Name == b.Name && a.Path == b.Path && a.SHA256 == b.SHA256 &&
		a.Duration == b.Duration && eqT(a.StartsAt, b.StartsAt) && eqT(a.EndsAt, b.EndsAt) &&
		a.PlaysPerDay == b.PlaysPerDay && a.HourFrom == b.HourFrom && a.HourTo == b.HourTo && a.Active == b.Active &&
		a.CampaignID == b.CampaignID && a.Weight == b.Weight && a.DaysMask == b.DaysMask && a.Position == b.Position &&
		a.Priority == b.Priority && a.Sector == b.Sector
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	if sha == "" {
		return "sinhash"
	}
	return sha
}

// commands pide las órdenes del panel, las ejecuta y responde. Devuelve cuántas.
func (s *Service) commands(ctx context.Context, c *apiclient.Client) int {
	var list []apiclient.Command
	if !s.run(ctx, func() error {
		var err error
		list, err = c.GetCommands(ctx)
		return err
	}) {
		return 0
	}
	for _, cmd := range list {
		res, err := s.d.Command(cmd.Kind, cmd.Arg)
		ok := err == nil
		if err != nil {
			res = err.Error()
		}
		s.d.Log.Info("orden del panel", "orden", cmd.Kind, "arg", cmd.Arg, "ok", ok, "resultado", res)
		_ = c.CommandDone(ctx, cmd.ID, ok, res)
	}
	return len(list)
}
