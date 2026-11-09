package scheduler

import (
	"sort"
	"time"

	"radioclub25/estudio/internal/domain"
	"radioclub25/estudio/internal/service/rotation"
)

// Config son los parámetros de la generación.
type Config struct {
	Crossfade time.Duration // solape entre pistas: la duración prevista es hasta el punto de mezcla
	Tolerance time.Duration // margen para cuadrar la hora (no rellenar si falta menos que esto)
	Rules     rotation.Rules
	SpotsLen  time.Duration // duración estimada de una tanda (la real la fija el servicio de cuñas)
	VoiceLen  time.Duration // duración estimada de una locución de hora o temperatura
}

// DefaultConfig son los valores del plan.
func DefaultConfig() Config {
	return Config{Crossfade: 3 * time.Second, Tolerance: 30 * time.Second, SpotsLen: 2 * time.Minute,
		VoiceLen: 5 * time.Second, Rules: rotation.Rules{ArtistSeparation: 45 * time.Minute, TitleSeparation: 3 * time.Hour}}
}

// Plan son los datos de entrada para generar un día. Generate no hace E/S.
type Plan struct {
	Day        time.Time                // medianoche del día, en la zona de la radio
	ClockFor   func(hour int) int64     // reloj de cada hora (excepción o parrilla); 0 = sin reloj
	Clocks     map[int64]domain.Clock   // relojes por ID
	ByCategory map[int64][]domain.Track // pistas por categoría
	ByID       map[int64]domain.Track   // pistas fijas por ID
	Filler     int64                    // categoría de relleno (0 = usar la última categoría musical del reloj)
	History    []rotation.Use           // emisiones y elementos ya planificados (días previos)
	Locked     []domain.LogItem         // elementos bloqueados de la versión anterior del día
}

// generator lleva el estado mientras se arma el día.
type generator struct {
	p          Plan
	cfg        Config
	items      []domain.LogItem
	history    []rotation.Use
	prevArtist string
	lastTracks []domain.Track // últimas pistas programadas (para las reglas de flujo)
}

// Generate arma la playlist de un día hora por hora según los relojes.
//
//   - Cada hora se planifica desde su inicio. Los slots blandos van en orden; un slot
//     duro se fija al minuto indicado y, si antes queda hueco, se rellena.
//   - Al final de la hora se rellena hasta (fin − tolerancia) con la categoría de
//     relleno o la última categoría musical del reloj.
//   - Si la hora se pasa de largo, las últimas pistas musicales se marcan Optional
//     (el aire las salta si va tarde).
//   - Los elementos bloqueados de la versión anterior se conservan en su hora y posición.
func Generate(p Plan, cfg Config) []domain.LogItem {
	g := &generator{p: p, cfg: cfg, history: make([]rotation.Use, 0, len(p.History)+600)}
	for _, u := range p.History {
		g.history = append(g.history, rotation.NewUse(u.TrackID, u.Artist, u.Title, u.At))
	}
	sort.SliceStable(g.history, func(i, j int) bool { return g.history[i].At.Before(g.history[j].At) })
	for h := 0; h < 24; h++ {
		g.hour(h)
	}
	g.mergeLocked()
	Retime(g.items, p.Day)
	return g.items
}

func (g *generator) hour(h int) {
	start := g.p.Day.Add(time.Duration(h) * time.Hour)
	end := start.Add(time.Hour)
	cursor := start
	first := len(g.items)
	clock, ok := g.p.Clocks[g.p.ClockFor(h)]
	lastCat := g.p.Filler
	if ok {
		for _, s := range clock.Slots {
			if s.Kind == domain.SlotCategory && s.CategoryID > 0 && isMusic(g.p.ByCategory[s.CategoryID]) {
				lastCat = s.CategoryID
			}
		}
		for _, s := range clock.Slots {
			if s.Hard {
				at := start.Add(time.Duration(min(max(s.Minute, 0), 59)) * time.Minute)
				cursor = g.fill(h, cursor, at.Add(-g.cfg.Tolerance), g.fillCat(lastCat))
				it, ok := g.item(h, s, at)
				if ok {
					it.Hard, it.At = true, at
					g.items = append(g.items, it)
					cursor = at.Add(it.Length)
				}
				continue
			}
			if it, ok := g.item(h, s, cursor); ok {
				g.items = append(g.items, it)
				cursor = cursor.Add(it.Length)
			}
		}
	}
	cursor = g.fill(h, cursor, end.Add(-g.cfg.Tolerance), g.fillCat(lastCat))
	// hora larga: marcar como opcionales las últimas pistas musicales que se pasan
	over := cursor.Sub(end)
	for i := len(g.items) - 1; i >= first && over > g.cfg.Tolerance; i-- {
		it := &g.items[i]
		if it.Kind == domain.SlotCategory && !it.Hard && !it.Locked {
			it.Optional = true
			over -= it.Length
		}
	}
}

// fillCat devuelve la categoría para rellenar: la de relleno si tiene pistas, si no la musical.
func (g *generator) fillCat(lastMusic int64) int64 {
	if g.p.Filler > 0 && len(g.p.ByCategory[g.p.Filler]) > 0 {
		return g.p.Filler
	}
	return lastMusic
}

// fill agrega pistas de cat desde cursor hasta llegar a until. Devuelve el nuevo cursor.
func (g *generator) fill(h int, cursor, until time.Time, cat int64) time.Time {
	if cat == 0 {
		return cursor
	}
	// tope de seguridad: con la duración mínima de 5 s, una hora no pasa de 720 pistas
	for i := 0; i < 800 && cursor.Before(until); i++ {
		it, ok := g.item(h, domain.ClockSlot{Kind: domain.SlotCategory, CategoryID: cat}, cursor)
		if !ok {
			break
		}
		it.Note = joinNote("relleno", it.Note)
		g.items = append(g.items, it)
		cursor = cursor.Add(it.Length)
	}
	return cursor
}

// item resuelve un slot en un elemento de la playlist.
func (g *generator) item(h int, s domain.ClockSlot, at time.Time) (domain.LogItem, bool) {
	it := domain.LogItem{Hour: h, Kind: s.Kind, At: at, Status: domain.LogPending}
	switch s.Kind {
	case domain.SlotCategory:
		r := rotation.PickFlow(g.p.ByCategory[s.CategoryID], g.recent(at), g.recentTracks(), at, g.cfg.Rules)
		if !r.OK {
			return it, false
		}
		g.setTrack(&it, r.Track, at)
		it.CategoryID = s.CategoryID
		if r.Relaxed != "" {
			it.Note = "rotación: se relajó " + r.Relaxed
		}
	case domain.SlotTrack:
		t, ok := g.p.ByID[s.TrackID]
		if !ok || !t.Available(at) {
			return it, false
		}
		g.setTrack(&it, t, at)
	case domain.SlotSpots:
		it.Title, it.Length = "Tanda de cuñas", g.cfg.SpotsLen
	case domain.SlotTime:
		it.Title, it.Length = "Hora locutada", g.cfg.VoiceLen
	case domain.SlotTemp:
		it.Title, it.Length = "Temperatura", g.cfg.VoiceLen
	case domain.SlotMode:
		it.Title, it.Mode = "Cambio de modo: "+s.Mode, s.Mode
	case domain.SlotSilence:
		it.Title, it.Seconds, it.Length = "Silencio", s.Seconds, time.Duration(s.Seconds)*time.Second
	default:
		return it, false
	}
	return it, true
}

func (g *generator) setTrack(it *domain.LogItem, t domain.Track, at time.Time) {
	it.TrackID, it.Title, it.Artist = t.ID, t.Title, t.Artist
	if it.Title == "" {
		it.Title = t.Display()
	}
	it.Length = TrackLength(t, g.cfg.Crossfade)
	// historial ordenado por hora (un evento duro puede quedar antes que el cursor)
	u := rotation.NewUse(t.ID, t.Artist, t.Title, at)
	k := sort.Search(len(g.history), func(i int) bool { return g.history[i].At.After(at) })
	g.history = append(g.history, rotation.Use{})
	copy(g.history[k+1:], g.history[k:])
	g.history[k] = u
	// recordar el uso en la pista: el orden "menos reciente" vale también fuera de la ventana
	for i := range g.p.ByCategory[t.CategoryID] {
		if c := &g.p.ByCategory[t.CategoryID][i]; c.ID == t.ID {
			when := at
			c.LastPlayedAt = &when
		}
	}
	if t.Artist != "" {
		g.prevArtist = t.Artist
	}
	g.lastTracks = append(g.lastTracks, t)
	if len(g.lastTracks) > 8 {
		g.lastTracks = g.lastTracks[len(g.lastTracks)-8:]
	}
}

// recentTracks son las pistas anteriores para las reglas de flujo; la última
// lleva el intérprete anterior (aunque haya sido una pista fija sin artista,
// se mantiene el último intérprete conocido, como antes).
func (g *generator) recentTracks() []domain.Track {
	if len(g.lastTracks) == 0 {
		if g.prevArtist == "" {
			return nil
		}
		return []domain.Track{{Artist: g.prevArtist}}
	}
	out := append([]domain.Track(nil), g.lastTracks...)
	if out[len(out)-1].Artist == "" {
		out[len(out)-1].Artist = g.prevArtist
	}
	return out
}

// recent devuelve los usos que pueden afectar a una elección en at: los de la
// ventana de separación (más 1 h de margen para el orden de "menos reciente").
// El historial se mantiene ordenado por hora, así que basta una búsqueda binaria
// y el coste de cada elección no crece con el tamaño del día.
func (g *generator) recent(at time.Time) []rotation.Use {
	window := max(g.cfg.Rules.TitleSeparation, g.cfg.Rules.ArtistSeparation) + time.Hour
	from := at.Add(-window)
	i := sort.Search(len(g.history), func(i int) bool { return !g.history[i].At.Before(from) })
	j := sort.Search(len(g.history), func(i int) bool { return g.history[i].At.After(at) })
	return g.history[i:j]
}

// TrackLength es lo que ocupa una pista en la playlist: hasta su punto de mezcla.
func TrackLength(t domain.Track, crossfade time.Duration) time.Duration {
	l := t.Segue(crossfade)
	if l <= 0 {
		l = t.PlayLength()
	}
	return max(l, 5*time.Second)
}

func isMusic(tracks []domain.Track) bool { return len(tracks) > 0 }

func joinNote(a, b string) string {
	if b == "" {
		return a
	}
	return a + " · " + b
}

// mergeLocked reinserta los elementos bloqueados en su hora, en la misma posición relativa.
func (g *generator) mergeLocked() {
	if len(g.p.Locked) == 0 {
		return
	}
	locked := append([]domain.LogItem(nil), g.p.Locked...)
	sort.SliceStable(locked, func(i, j int) bool { return locked[i].Seq < locked[j].Seq })
	// posición relativa dentro de su hora en la versión anterior
	firstSeq := map[int]int{}
	for _, l := range locked {
		if s, ok := firstSeq[l.Hour]; !ok || l.Seq < s {
			firstSeq[l.Hour] = l.Seq
		}
	}
	for _, l := range locked {
		idx := len(g.items) // el bloqueado conserva su ID

		pos := 0
		for i, it := range g.items {
			if it.Hour == l.Hour {
				if pos == l.Seq-firstSeq[l.Hour] {
					idx = i
					break
				}
				pos++
			} else if it.Hour > l.Hour {
				idx = i
				break
			}
		}
		g.items = append(g.items[:idx], append([]domain.LogItem{l}, g.items[idx:]...)...)
	}
}

// Retime recalcula las horas previstas en orden: cada elemento empieza cuando
// termina el anterior; los duros mantienen su hora; si una hora empieza más
// tarde que el cursor (hueco), el cursor salta al inicio de esa hora.
func Retime(items []domain.LogItem, day time.Time) {
	cursor := day
	for i := range items {
		it := &items[i]
		hourStart := day.Add(time.Duration(it.Hour) * time.Hour)
		if cursor.Before(hourStart) {
			cursor = hourStart
		}
		if it.Hard {
			if it.At.IsZero() {
				it.At = hourStart
			}
		} else {
			it.At = cursor
		}
		cursor = it.At.Add(it.Length)
	}
}
