// Package onair orquesta la emisión: toma la playlist del día, decide qué
// suena (service/events), expande cada elemento en segmentos (segment.go),
// los carga en los decks del motor de audio con crossfade, registra cada
// emisión y aplica los modos Automático, Asistido y En vivo. También maneja la
// botonera (carts.go), el micrófono y la playlist de emergencia por silencio.
//
// Un solo goroutine (Run) es dueño del estado: los métodos públicos le envían
// órdenes por un canal y leen una foto del estado protegida por un mutex.
package onair

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"radioclub25/estudio/internal/audio"
	"radioclub25/estudio/internal/domain"
	"radioclub25/estudio/internal/service/events"
	"radioclub25/estudio/internal/service/timeannounce"
)

// Deps son las dependencias del aire (interfaces pequeñas para poder probarlo).
type Deps struct {
	Engine     *audio.Engine
	Log        LogSource
	Tracks     TrackSource
	Categories interface {
		List(context.Context) ([]domain.Category, error)
	}
	Spots       SpotSource
	Plays       domain.PlayRepository
	Decode      func(ctx context.Context, path string, start time.Duration) (audio.Source, error)
	Voice       func() timeannounce.Files
	Temperature func(ctx context.Context) (float64, error)
	Location    func() *time.Location
	Options     func() Options
	Logger      *slog.Logger
	Now         func() time.Time // nil = time.Now
}

// LogSource es la playlist (scheduler.Service).
type LogSource interface {
	Upcoming(ctx context.Context, now time.Time, limit int) ([]domain.LogItem, error)
	SetStatus(ctx context.Context, id int64, s domain.LogStatus) error
}

// TrackSource son las pistas (domain.TrackRepository).
type TrackSource interface {
	Get(ctx context.Context, id int64) (*domain.Track, error)
	ByCategory(ctx context.Context, categoryID int64) ([]domain.Track, error)
}

// SpotSource son las tandas (spots.Service).
type SpotSource interface {
	TandaSpots(ctx context.Context, logItemID int64) ([]domain.Spot, error)
	CortinaCategory(ctx context.Context) int64
}

// Options son los ajustes que el aire lee en cada decisión.
type Options struct {
	Crossfade time.Duration
	HardFade  time.Duration
	Emergency int64 // categoría de emergencia (0 = ninguna)
}

// Hooks avisan a otros servicios (sincronización, metadata de Icecast).
type Hooks struct {
	OnSegment   func(seg Segment)                // empezó a sonar un segmento
	OnPlayEnded func(p domain.Play, seg Segment) // terminó (con duración real y si fue completa)
	OnMode      func(m domain.StudioMode)        // cambió el modo
	OnWarning   func(msg string)                 // aviso para la interfaz y el log
}

// active es un segmento sonando en un deck.
type active struct {
	seg     Segment
	playID  int64
	uuid    string
	started time.Time
	slot    int
}

type cmd struct {
	kind  string
	mode  domain.StudioMode
	cart  int
	reply chan error
}

// Service es el aire.
type Service struct {
	d     Deps
	hooks Hooks
	cmds  chan cmd

	// estado del goroutine Run
	running    bool
	mode       domain.StudioMode
	current    *active   // lo que suena en un deck (el último arrancado)
	fading     []*active // lo que se está fundiendo
	queue      []Segment // segmentos restantes del elemento en curso
	waiting    bool      // asistido: terminó y espera al operador
	stopAfter  bool      // parar al terminar lo que suena (botón "Parar tras actual")
	silenceAt  time.Time // fin de un segmento de silencio
	silenceSeg Segment
	upcoming   []domain.LogItem
	upAt       time.Time
	itemID     int64 // elemento de la playlist en curso
	carts      *cartDeck

	mu   sync.Mutex
	snap Snapshot
}

// New crea el aire (detenido, en modo automático).
func New(d Deps, h Hooks) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	s := &Service{d: d, hooks: h, cmds: make(chan cmd, 32), mode: domain.ModeAuto}
	s.carts = newCartDeck(d)
	return s
}

// ---------- órdenes (desde la interfaz) ----------

func (s *Service) send(c cmd) error {
	c.reply = make(chan error, 1)
	select {
	case s.cmds <- c:
	case <-time.After(2 * time.Second):
		return fmt.Errorf("el aire no responde")
	}
	select {
	case err := <-c.reply:
		return err
	case <-time.After(5 * time.Second):
		return fmt.Errorf("el aire no responde")
	}
}

// Start arranca la automatización.
func (s *Service) Start() error { return s.send(cmd{kind: "start"}) }

// Stop detiene la automatización (funde lo que suena).
func (s *Service) Stop() error { return s.send(cmd{kind: "stop"}) }

// Next pasa al siguiente elemento ahora (con crossfade). En asistido es el botón "Siguiente".
func (s *Service) Next() error { return s.send(cmd{kind: "next"}) }

// SetMode cambia el modo.
func (s *Service) SetMode(m domain.StudioMode) error {
	if !m.Valid() {
		return domain.Invalid("mode", "Modo inválido")
	}
	return s.send(cmd{kind: "mode", mode: m})
}

// TriggerCart dispara el botón slot de la página actual de la botonera.
func (s *Service) TriggerCart(page, slot int) error {
	return s.send(cmd{kind: "cart", cart: page*domain.CartsPerPage + slot})
}

// StopAfterCurrent alterna "parar al terminar lo que suena": la playlist queda
// en espera hasta pulsar Siguiente (o Iniciar).
func (s *Service) StopAfterCurrent() error { return s.send(cmd{kind: "stop_after"}) }

// StopCarts detiene todos los carts.
func (s *Service) StopCarts() error { return s.send(cmd{kind: "carts_off"}) }

// ---------- bucle ----------

// Run atiende al motor, a las órdenes y al reloj hasta que ctx se cancele.
func (s *Service) Run(ctx context.Context) {
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case c := <-s.cmds:
			c.reply <- s.handleCmd(ctx, c)
		case ev := <-s.d.Engine.Events():
			s.handleEvent(ctx, ev)
		case <-tick.C:
			s.tick(ctx)
		}
		s.publish()
	}
}

func (s *Service) handleCmd(ctx context.Context, c cmd) error {
	switch c.kind {
	case "start":
		if !s.running {
			s.running, s.waiting = true, false
			s.refreshUpcoming(ctx, true)
			s.advance(ctx, false)
		}
	case "stop":
		s.running = false
		s.fadeOutCurrent(2 * time.Second)
		s.queue = nil
	case "next":
		s.waiting, s.stopAfter = false, false
		if !s.running {
			s.running = true
		}
		s.queue = nil // saltar el resto del elemento en curso
		s.advanceOpt(ctx, true, true)
	case "mode":
		return s.setMode(ctx, c.mode)
	case "cart":
		return s.carts.trigger(ctx, c.cart/domain.CartsPerPage, c.cart%domain.CartsPerPage)
	case "carts_off":
		s.carts.stopAll()
	case "stop_after":
		s.stopAfter = !s.stopAfter
	case "refresh":
		s.upAt = time.Time{}
	}
	return nil
}

func (s *Service) setMode(ctx context.Context, m domain.StudioMode) error {
	if m == s.mode {
		return nil
	}
	prev := s.mode
	s.mode = m
	switch {
	case m == domain.ModeLive:
		s.fadeOutCurrent(2 * time.Second) // la música se pausa: queda mic + botonera
		s.queue = nil
		s.waiting = false
	case prev == domain.ModeLive && s.running:
		s.refreshUpcoming(ctx, true)
		s.advance(ctx, false)
	}
	if s.hooks.OnMode != nil {
		s.hooks.OnMode(m)
	}
	return nil
}

func (s *Service) handleEvent(ctx context.Context, ev audio.Event) {
	if ev.Type == audio.EventSilence {
		if s.running && s.mode != domain.ModeLive {
			if s.playEmergency(ctx) {
				s.warn("silencio en el aire: se puso una pista de emergencia")
			} else {
				s.warn("silencio en el aire y no hay pistas para cubrirlo")
			}
		} else {
			s.warn("silencio en el aire")
		}
		return
	}
	if ev.Slot >= audio.NumDecks {
		s.carts.event(ev)
		return
	}
	switch ev.Type {
	case audio.EventSegue:
		if s.current != nil && ev.Slot == s.current.slot && uint64(s.current.playID) == ev.Tag &&
			s.running && s.mode == domain.ModeAuto && !(s.stopAfter && len(s.queue) == 0) {
			s.advance(ctx, true)
		}
	case audio.EventEnded:
		s.finish(ctx, ev)
		if s.current != nil && ev.Slot == s.current.slot && uint64(s.current.playID) == ev.Tag {
			s.current = nil
			if !s.running || s.mode == domain.ModeLive {
				return
			}
			if s.stopAfter && len(s.queue) == 0 {
				s.stopAfter, s.waiting = false, true // se pidió parar tras este elemento
				return
			}
			if s.mode == domain.ModeAssisted && len(s.queue) == 0 {
				s.waiting = true // asistido: espera al operador
				return
			}
			s.advance(ctx, false)
		}
	}
}

func (s *Service) tick(ctx context.Context) {
	if !s.running || s.mode == domain.ModeLive {
		return
	}
	now := s.d.Now()
	// fin de un segmento de silencio
	if !s.silenceAt.IsZero() && !now.Before(s.silenceAt) {
		s.silenceAt = time.Time{}
		if s.silenceSeg.Last {
			s.itemDone(ctx, s.silenceSeg.ItemID)
		}
		s.advance(ctx, false)
		return
	}
	s.refreshUpcoming(ctx, false)
	// evento duro en hora
	d := events.Next(now, false, s.pending(), events.DefaultRules())
	if d.Action == events.HardCut {
		s.markSkipped(ctx, d.Skip)
		s.queue = nil
		s.waiting = false
		s.startItem(ctx, d.Item, s.d.Options().HardFade)
		return
	}
	// automático sin nada sonando (arranque, lista vacía que se llenó, error de un segmento)
	if s.mode == domain.ModeAuto && s.current == nil && s.silenceAt.IsZero() {
		s.advance(ctx, false)
	}
}

// pending devuelve los elementos pendientes (sin el que está sonando).
func (s *Service) pending() []domain.LogItem {
	out := make([]domain.LogItem, 0, len(s.upcoming))
	for _, it := range s.upcoming {
		if it.Status == domain.LogPending && it.ID != s.itemID {
			out = append(out, it)
		}
	}
	return out
}

func (s *Service) refreshUpcoming(ctx context.Context, force bool) {
	now := s.d.Now()
	if !force && now.Sub(s.upAt) < 2*time.Second {
		return
	}
	list, err := s.d.Log.Upcoming(ctx, now.Add(-10*time.Minute), 400)
	if err != nil {
		s.warn("leer la playlist: " + err.Error())
		return
	}
	s.upcoming, s.upAt = list, now
}

// Refresh pide releer la playlist (tras editarla). No espera respuesta.
func (s *Service) Refresh() {
	select {
	case s.cmds <- cmd{kind: "refresh", reply: make(chan error, 1)}:
	default:
	}
}

// advance arranca lo siguiente: el resto del elemento en curso o el próximo elemento.
// crossfade = fundir lo que suena (punto de mezcla o "Siguiente").
func (s *Service) advance(ctx context.Context, crossfade bool) { s.advanceOpt(ctx, crossfade, false) }

// aheadWindow es hasta dónde se adelanta la playlist cuando el aire quedaría en
// silencio esperando un evento duro.
const aheadWindow = 3 * time.Hour

// advanceOpt es advance; force = el operador pulsó "Siguiente" (se adelanta
// la playlist aunque algo esté sonando).
func (s *Service) advanceOpt(ctx context.Context, crossfade, force bool) {
	fade := time.Duration(0)
	if crossfade {
		fade = s.d.Options().Crossfade
	}
	for tries := 0; tries < 20; tries++ {
		if len(s.queue) > 0 {
			seg := s.queue[0]
			s.queue = s.queue[1:]
			if s.startSegment(ctx, seg, fade) {
				return
			}
			continue
		}
		s.itemID = 0 // el elemento anterior se marca emitido cuando termina su último segmento
		s.refreshUpcoming(ctx, false)
		d := events.Next(s.d.Now(), true, s.pending(), events.DefaultRules())
		s.markSkipped(ctx, d.Skip)
		switch d.Action {
		case events.HardCut:
			s.startItem(ctx, d.Item, s.d.Options().HardFade)
			return
		case events.Start:
			if s.startItem(ctx, d.Item, fade) {
				return
			}
			continue
		}
		// Nada que arrancar ahora: lo siguiente es un evento duro que todavía no
		// llega (p. ej. la tanda de las 19:30 y no queda nada antes). En vez de dejar
		// el aire en silencio hasta esa hora, se adelanta lo que viene después: el
		// evento duro cortará a su hora. Si no hay nada, emergencia o relleno.
		if s.current == nil || force {
			if it, ok := s.aheadItem(); ok {
				if s.startItem(ctx, it, fade) {
					return
				}
				continue
			}
			if s.current == nil {
				s.playEmergency(ctx)
			}
		}
		return
	}
}

// aheadItem es el primer elemento blando pendiente de las próximas horas que
// está después de un evento duro futuro (el que se puede adelantar).
func (s *Service) aheadItem() (domain.LogItem, bool) {
	limit := s.d.Now().Add(aheadWindow)
	for _, it := range s.pending() {
		if it.Hard || it.Optional {
			continue
		}
		if it.At.After(limit) {
			break
		}
		return it, true
	}
	return domain.LogItem{}, false
}

// startItem expande un elemento de la playlist y arranca su primer segmento.
func (s *Service) startItem(ctx context.Context, it domain.LogItem, fade time.Duration) bool {
	x := s.expander(ctx)
	segs, warns := x.expand(ctx, it)
	for _, w := range warns {
		s.warn(w)
	}
	if len(segs) == 0 {
		_ = s.d.Log.SetStatus(ctx, it.ID, domain.LogSkipped)
		s.markLocal(it.ID, domain.LogSkipped)
		s.itemID = 0
		return false
	}
	s.itemID = it.ID
	_ = s.d.Log.SetStatus(ctx, it.ID, domain.LogPlaying)
	s.markLocal(it.ID, domain.LogPlaying)
	s.queue = segs
	for len(s.queue) > 0 {
		seg := s.queue[0]
		s.queue = s.queue[1:]
		if s.startSegment(ctx, seg, fade) {
			return true
		}
	}
	return false
}

// startSegment carga un segmento en el deck libre y lo hace sonar; funde lo anterior.
func (s *Service) startSegment(ctx context.Context, seg Segment, fade time.Duration) bool {
	if seg.Mode != "" {
		_ = s.setMode(ctx, domain.StudioMode(seg.Mode))
		if seg.Last {
			s.itemDone(ctx, seg.ItemID)
		}
		return false
	}
	if seg.Silence > 0 {
		s.fadeOutCurrent(fade)
		s.silenceAt, s.silenceSeg = s.d.Now().Add(seg.Silence), seg
		return true
	}
	src, err := s.d.Decode(ctx, seg.Path, seg.CueIn)
	if err != nil {
		s.warn(fmt.Sprintf("no se pudo abrir %s: %v", seg.Path, err))
		return false
	}
	slot := audio.DeckA
	if s.current != nil && s.current.slot == audio.DeckA {
		slot = audio.DeckB
	}
	a := &active{seg: seg, uuid: newUUID(), started: s.d.Now(), slot: slot}
	p := &domain.Play{TrackID: seg.TrackID, SpotID: seg.SpotID, Kind: seg.Kind, Artist: seg.Artist, Title: seg.Title,
		StartedAt: a.started, UUID: a.uuid}
	if err := s.d.Plays.Add(ctx, p); err != nil {
		s.warn("registrar emisión: " + err.Error())
	}
	a.playID = p.ID
	if a.playID == 0 {
		a.playID = time.Now().UnixNano() // sin base: el tag sigue siendo único
	}
	if err := s.d.Engine.Load(slot, src, audio.LoadOptions{Tag: uint64(a.playID), GainDB: seg.GainDB,
		Length: seg.Length, SegueAt: seg.Segue}); err != nil {
		s.warn("cargar en el motor: " + err.Error())
		return false
	}
	if s.current != nil {
		s.fading = append(s.fading, s.current)
		_ = s.d.Engine.Fade(s.current.slot, 0, max(fade, 50*time.Millisecond), true)
	}
	_ = s.d.Engine.Play(slot, 0)
	s.current = a
	s.silenceAt = time.Time{}
	if s.hooks.OnSegment != nil {
		s.hooks.OnSegment(seg)
	}
	return true
}

// finish cierra el registro de una emisión al terminar.
func (s *Service) finish(ctx context.Context, ev audio.Event) {
	var a *active
	if s.current != nil && uint64(s.current.playID) == ev.Tag {
		a = s.current
	}
	for i, f := range s.fading {
		if uint64(f.playID) == ev.Tag {
			a = f
			s.fading = append(s.fading[:i], s.fading[i+1:]...)
			break
		}
	}
	if a == nil {
		return
	}
	played := s.d.Engine.Slot(ev.Slot).Position
	if st := s.d.Engine.Slot(ev.Slot); st.Tag != ev.Tag {
		played = s.d.Now().Sub(a.started) // el slot ya tiene otra cosa: usar el reloj
	}
	complete := a.seg.Length == 0 || played >= a.seg.Length-500*time.Millisecond
	if a.seg.Segue > 0 && played >= a.seg.Segue {
		complete = true // fundida en su punto de mezcla: sonó lo previsto
	}
	_ = s.d.Plays.Finish(ctx, a.playID, played, complete)
	if a.seg.Last {
		s.itemDone(ctx, a.seg.ItemID)
	}
	if s.hooks.OnPlayEnded != nil {
		s.hooks.OnPlayEnded(domain.Play{ID: a.playID, TrackID: a.seg.TrackID, SpotID: a.seg.SpotID, Kind: a.seg.Kind,
			Artist: a.seg.Artist, Title: a.seg.Title, StartedAt: a.started, Duration: played, Complete: complete, UUID: a.uuid}, a.seg)
	}
}

func (s *Service) fadeOutCurrent(d time.Duration) {
	if s.current != nil {
		s.fading = append(s.fading, s.current)
		_ = s.d.Engine.Fade(s.current.slot, 0, max(d, 50*time.Millisecond), true)
		s.current = nil
	}
	s.silenceAt = time.Time{}
}

// playEmergency hace sonar una pista de la categoría de emergencia (o de
// relleno). Si esa categoría no tiene pistas, una de cualquier categoría de
// música: el aire nunca debe quedar en silencio. Devuelve si sonó algo.
func (s *Service) playEmergency(ctx context.Context) bool {
	now := s.d.Now()
	pick := func(cat int64) (domain.Track, bool) {
		list, err := s.d.Tracks.ByCategory(ctx, cat)
		if err != nil {
			return domain.Track{}, false
		}
		var ok []domain.Track
		for _, t := range list {
			if t.Available(now) {
				ok = append(ok, t)
			}
		}
		if len(ok) == 0 {
			return domain.Track{}, false
		}
		return ok[int(now.UnixNano()/1e6)%len(ok)], true
	}
	t, found := domain.Track{}, false
	if cat := s.d.Options().Emergency; cat != 0 {
		t, found = pick(cat)
	}
	if !found && s.d.Categories != nil {
		if cats, err := s.d.Categories.List(ctx); err == nil {
			for _, c := range cats {
				if c.Kind == domain.KindMusic {
					if t, found = pick(c.ID); found {
						break
					}
				}
			}
		}
	}
	if !found {
		return false
	}
	seg := s.expander(ctx).trackSegment(t)
	seg.Last = true
	s.queue = nil
	return s.startSegment(ctx, seg, s.d.Options().Crossfade)
}

func (s *Service) expander(ctx context.Context) *expander {
	kinds := map[int64]domain.CategoryKind{}
	if cats, err := s.d.Categories.List(ctx); err == nil {
		for _, c := range cats {
			kinds[c.ID] = c.Kind
		}
	}
	return &expander{d: s.d, crossfade: s.d.Options().Crossfade, kinds: kinds}
}

// itemDone marca un elemento de la playlist como emitido (al terminar su último segmento).
func (s *Service) itemDone(ctx context.Context, id int64) {
	if id == 0 {
		return
	}
	_ = s.d.Log.SetStatus(ctx, id, domain.LogPlayed)
	s.markLocal(id, domain.LogPlayed)
}

func (s *Service) markSkipped(ctx context.Context, ids []int64) {
	for _, id := range ids {
		_ = s.d.Log.SetStatus(ctx, id, domain.LogSkipped)
		s.markLocal(id, domain.LogSkipped)
	}
}

func (s *Service) markLocal(id int64, st domain.LogStatus) {
	for i := range s.upcoming {
		if s.upcoming[i].ID == id {
			s.upcoming[i].Status = st
		}
	}
}

func (s *Service) warn(msg string) {
	if s.d.Logger != nil {
		s.d.Logger.Warn("aire", "msg", msg)
	}
	s.mu.Lock()
	s.snap.Warnings = append([]Warning{{At: s.d.Now(), Message: msg}}, s.snap.Warnings...)
	if len(s.snap.Warnings) > 20 {
		s.snap.Warnings = s.snap.Warnings[:20]
	}
	s.mu.Unlock()
	if s.hooks.OnWarning != nil {
		s.hooks.OnWarning(msg)
	}
}

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
