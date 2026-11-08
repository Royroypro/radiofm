// Package scheduler genera y edita la playlist del día (log) a partir de la
// parrilla semanal y los relojes, usando service/rotation para elegir cada
// canción. generate.go tiene la lógica pura; este archivo la conecta con los
// repositorios (relojes, parrilla, pistas, historial, playlist).
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"radioclub25/estudio/internal/domain"
	"radioclub25/estudio/internal/service/rotation"
)

// Service gestiona relojes, parrilla y playlist.
type Service struct {
	clocks domain.ClockRepository
	grid   domain.GridRepository
	logs   domain.LogRepository
	tracks domain.TrackRepository
	cats   domain.CategoryRepository
	plays  domain.PlayRepository

	mu  sync.Mutex // serializa generación y edición
	cfg func() (Config, *time.Location)
	now func() time.Time

	// OnChange se llama tras cambiar la playlist de un día (para refrescar el aire y la interfaz).
	OnChange func(day string)
}

// New crea el servicio. cfg devuelve la configuración vigente (crossfade, separaciones, zona horaria).
func New(clocks domain.ClockRepository, grid domain.GridRepository, logs domain.LogRepository,
	tracks domain.TrackRepository, cats domain.CategoryRepository, plays domain.PlayRepository,
	cfg func() (Config, *time.Location)) *Service {
	return &Service{clocks: clocks, grid: grid, logs: logs, tracks: tracks, cats: cats, plays: plays, cfg: cfg, now: time.Now}
}

// DayOf devuelve el día (YYYY-MM-DD) de t en la zona de la radio.
func (s *Service) DayOf(t time.Time) string {
	_, loc := s.cfg()
	return t.In(loc).Format(domain.DayLayout)
}

func (s *Service) midnight(day string) (time.Time, error) {
	_, loc := s.cfg()
	t, err := time.ParseInLocation(domain.DayLayout, day, loc)
	if err != nil {
		return t, domain.Invalid("day", "Fecha inválida")
	}
	return t, nil
}

func (s *Service) changed(day string) {
	if s.OnChange != nil {
		s.OnChange(day)
	}
}

// ---------- Relojes ----------

// Clocks devuelve los relojes.
func (s *Service) Clocks(ctx context.Context) ([]domain.Clock, error) { return s.clocks.List(ctx) }

// SaveClock valida y guarda un reloj.
func (s *Service) SaveClock(ctx context.Context, c *domain.Clock) error {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return domain.Invalid("name", "El nombre es obligatorio")
	}
	if c.Color == "" {
		c.Color = "#223CB8"
	}
	for i, sl := range c.Slots {
		n := i + 1
		switch {
		case !sl.Kind.Valid():
			return domain.Invalid("slots", fmt.Sprintf("Slot %d: tipo inválido", n))
		case sl.Kind == domain.SlotCategory && sl.CategoryID == 0:
			return domain.Invalid("slots", fmt.Sprintf("Slot %d: elija una categoría", n))
		case sl.Kind == domain.SlotTrack && sl.TrackID == 0:
			return domain.Invalid("slots", fmt.Sprintf("Slot %d: elija una pista", n))
		case sl.Hard && (sl.Minute < 0 || sl.Minute > 59):
			return domain.Invalid("slots", fmt.Sprintf("Slot %d: el minuto debe estar entre 0 y 59", n))
		case sl.Kind == domain.SlotMode && !domain.StudioMode(sl.Mode).Valid():
			return domain.Invalid("slots", fmt.Sprintf("Slot %d: modo inválido", n))
		case sl.Kind == domain.SlotSilence && (sl.Seconds <= 0 || sl.Seconds > 600):
			return domain.Invalid("slots", fmt.Sprintf("Slot %d: el silencio debe durar entre 1 y 600 s", n))
		}
	}
	// los eventos duros deben estar en orden de minuto
	last := -1
	for i, sl := range c.Slots {
		if sl.Hard {
			if sl.Minute < last {
				return domain.Invalid("slots", fmt.Sprintf("Slot %d: los eventos duros deben ir en orden de minuto", i+1))
			}
			last = sl.Minute
		}
	}
	err := s.clocks.Save(ctx, c)
	if errors.Is(err, domain.ErrConflict) {
		return domain.Invalid("name", "Ya existe un reloj con ese nombre")
	}
	return err
}

// DeleteClock borra un reloj que no se use en la parrilla.
func (s *Service) DeleteClock(ctx context.Context, id int64) error {
	err := s.clocks.Delete(ctx, id)
	if errors.Is(err, domain.ErrConflict) {
		return domain.Invalid("clock", "El reloj está en la parrilla o en una excepción: quítelo antes")
	}
	return err
}

// ---------- Parrilla ----------

// Grid devuelve las celdas asignadas.
func (s *Service) Grid(ctx context.Context) ([]domain.GridCell, error) { return s.grid.Cells(ctx) }

// SetGrid asigna relojes a celdas (ClockID 0 = vaciar).
func (s *Service) SetGrid(ctx context.Context, cells []domain.GridCell) error {
	for _, c := range cells {
		if c.Weekday < 0 || c.Weekday > 6 || c.Hour < 0 || c.Hour > 23 {
			return domain.Invalid("grid", "Celda fuera de rango")
		}
		if c.ClockID != 0 {
			if _, err := s.clocks.Get(ctx, c.ClockID); err != nil {
				return domain.Invalid("grid", "Reloj inexistente")
			}
		}
	}
	return s.grid.SetCells(ctx, cells)
}

// Exceptions devuelve las excepciones desde hoy.
func (s *Service) Exceptions(ctx context.Context) ([]domain.GridException, error) {
	return s.grid.Exceptions(ctx, s.DayOf(s.now()))
}

// SetException crea o borra (ClockID 0) una excepción.
func (s *Service) SetException(ctx context.Context, e domain.GridException) error {
	if _, err := s.midnight(e.Date); err != nil {
		return err
	}
	if e.Hour < 0 || e.Hour > 23 {
		return domain.Invalid("hour", "Hora fuera de rango")
	}
	return s.grid.SetException(ctx, e)
}

// EnsureDefaults crea un reloj básico y llena la parrilla si no hay ningún reloj,
// para que el programa suene desde el primer arranque.
func (s *Service) EnsureDefaults(ctx context.Context) error {
	list, err := s.clocks.List(ctx)
	if err != nil || len(list) > 0 {
		return err
	}
	cats, err := s.cats.List(ctx)
	if err != nil {
		return err
	}
	byName := map[string]int64{}
	for _, c := range cats {
		byName[c.Name] = c.ID
	}
	var slots []domain.ClockSlot
	add := func(sl domain.ClockSlot) {
		if (sl.Kind != domain.SlotCategory) || sl.CategoryID != 0 {
			slots = append(slots, sl)
		}
	}
	add(domain.ClockSlot{Kind: domain.SlotTime, Hard: true, Minute: 0})
	add(domain.ClockSlot{Kind: domain.SlotCategory, CategoryID: byName["IDs"]})
	for _, n := range []string{"Éxitos", "Cumbia", "Salsa", "Baladas", "Retro"} {
		add(domain.ClockSlot{Kind: domain.SlotCategory, CategoryID: byName[n]})
	}
	add(domain.ClockSlot{Kind: domain.SlotSpots, Hard: true, Minute: 30})
	add(domain.ClockSlot{Kind: domain.SlotCategory, CategoryID: byName["Jingles"]})
	for _, n := range []string{"Éxitos", "Cumbia", "Salsa", "Retro"} {
		add(domain.ClockSlot{Kind: domain.SlotCategory, CategoryID: byName[n]})
	}
	c := &domain.Clock{Name: "Musical básico", Color: "#223CB8", Slots: slots}
	if err := s.clocks.Save(ctx, c); err != nil {
		return err
	}
	var cells []domain.GridCell
	for d := 0; d < 7; d++ {
		for h := 0; h < 24; h++ {
			cells = append(cells, domain.GridCell{Weekday: d, Hour: h, ClockID: c.ID})
		}
	}
	return s.grid.SetCells(ctx, cells)
}

// ---------- Playlist ----------

// Day devuelve la playlist de un día.
func (s *Service) Day(ctx context.Context, day string) ([]domain.LogItem, error) {
	return s.logs.Day(ctx, day)
}

// Generate (re)genera la playlist de un día conservando los elementos bloqueados
// y los que ya sonaron o están sonando.
func (s *Service) Generate(ctx context.Context, day string) ([]domain.LogItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := s.generate(ctx, day)
	if err == nil {
		s.changed(day)
	}
	return items, err
}

func (s *Service) generate(ctx context.Context, day string) ([]domain.LogItem, error) {
	cfg, _ := s.cfg()
	midnight, err := s.midnight(day)
	if err != nil {
		return nil, err
	}
	clocks, err := s.clocks.List(ctx)
	if err != nil {
		return nil, err
	}
	cells, err := s.grid.Cells(ctx)
	if err != nil {
		return nil, err
	}
	exc, err := s.grid.Exceptions(ctx, day)
	if err != nil {
		return nil, err
	}
	cats, err := s.cats.List(ctx)
	if err != nil {
		return nil, err
	}
	p := Plan{Day: midnight, Clocks: map[int64]domain.Clock{}, ByCategory: map[int64][]domain.Track{},
		ByID: map[int64]domain.Track{}}
	for _, c := range clocks {
		p.Clocks[c.ID] = c
	}
	weekday := int(midnight.Weekday())
	gridMap := map[int]int64{}
	for _, c := range cells {
		if c.Weekday == weekday {
			gridMap[c.Hour] = c.ClockID
		}
	}
	for _, e := range exc {
		if e.Date == day {
			gridMap[e.Hour] = e.ClockID
		}
	}
	p.ClockFor = func(h int) int64 { return gridMap[h] }

	// pistas de las categorías usadas por los relojes y de relleno
	need := map[int64]bool{}
	for _, c := range clocks {
		for _, sl := range c.Slots {
			if sl.Kind == domain.SlotCategory {
				need[sl.CategoryID] = true
			}
			if sl.Kind == domain.SlotTrack {
				if t, err := s.tracks.Get(ctx, sl.TrackID); err == nil {
					p.ByID[t.ID] = *t
				}
			}
		}
	}
	for _, c := range cats {
		if c.Kind == domain.KindFiller && p.Filler == 0 {
			p.Filler = c.ID
			need[c.ID] = true
		}
	}
	for id := range need {
		list, err := s.tracks.ByCategory(ctx, id)
		if err != nil {
			return nil, err
		}
		p.ByCategory[id] = list
		for _, t := range list {
			p.ByID[t.ID] = t
		}
	}

	// historial: emisiones de las últimas 24 h y la playlist del día anterior
	sep := max(cfg.Rules.TitleSeparation, cfg.Rules.ArtistSeparation, 3*time.Hour)
	plays, err := s.plays.Since(ctx, midnight.Add(-sep))
	if err != nil {
		return nil, err
	}
	for _, pl := range plays {
		if pl.TrackID > 0 {
			p.History = append(p.History, rotation.Use{TrackID: pl.TrackID, Artist: pl.Artist, Title: pl.Title, At: pl.StartedAt})
		}
	}
	prev, _ := s.logs.Day(ctx, midnight.AddDate(0, 0, -1).Format(domain.DayLayout))
	for _, it := range prev {
		if it.TrackID > 0 && it.Status == domain.LogPending {
			p.History = append(p.History, rotation.Use{TrackID: it.TrackID, Artist: it.Artist, Title: it.Title, At: it.At})
		}
	}

	// conservar bloqueados; si el día ya empezó, conservar también lo que ya sonó
	old, err := s.logs.Day(ctx, day)
	if err != nil {
		return nil, err
	}
	var kept []domain.LogItem
	for _, it := range old {
		if it.Status != domain.LogPending {
			kept = append(kept, it)
		} else if it.Locked {
			p.Locked = append(p.Locked, it)
		}
	}
	items := Generate(p, cfg)
	if len(kept) > 0 {
		// lo ya emitido queda al principio; lo nuevo empieza después de la última hora emitida
		lastHour := kept[len(kept)-1].Hour
		var rest []domain.LogItem
		for _, it := range items {
			if it.Hour > lastHour || (it.Hour == lastHour && it.Locked) {
				rest = append(rest, it)
			}
		}
		items = append(kept, rest...)
		Retime(items, midnight)
	}
	if err := s.logs.ReplaceDay(ctx, day, items); err != nil {
		return nil, err
	}
	return items, nil
}

// EnsureDays genera los días que falten desde hoy hasta hoy + ahead.
func (s *Service) EnsureDays(ctx context.Context, ahead int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, loc := s.cfg()
	today := s.now().In(loc)
	for i := 0; i <= ahead; i++ {
		day := today.AddDate(0, 0, i).Format(domain.DayLayout)
		items, err := s.logs.Day(ctx, day)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			if _, err := s.generate(ctx, day); err != nil {
				return err
			}
			s.changed(day)
		}
	}
	return nil
}

// Apply carga la playlist de un día, aplica f, recalcula las horas y guarda.
// notify = false no llama a OnChange (lo usa el servicio de cuñas, que se
// ejecuta precisamente al cambiar la playlist: así no hay bucles).
func (s *Service) Apply(ctx context.Context, day string, f func(items []domain.LogItem) []domain.LogItem, notify bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	midnight, err := s.midnight(day)
	if err != nil {
		return err
	}
	items, err := s.logs.Day(ctx, day)
	if err != nil {
		return err
	}
	items = f(items)
	Retime(items, midnight)
	if err := s.logs.ReplaceDay(ctx, day, items); err != nil {
		return err
	}
	if notify {
		s.changed(day)
	}
	return nil
}

// edit carga el día de un elemento, aplica f y guarda con horas recalculadas.
func (s *Service) edit(ctx context.Context, id int64, f func(items []domain.LogItem, i int) ([]domain.LogItem, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, err := s.logs.Get(ctx, id)
	if err != nil {
		return err
	}
	items, err := s.logs.Day(ctx, it.Day)
	if err != nil {
		return err
	}
	idx := -1
	for i := range items {
		if items[i].ID == id {
			idx = i
		}
	}
	if idx < 0 {
		return domain.ErrNotFound
	}
	items, err = f(items, idx)
	if err != nil {
		return err
	}
	midnight, _ := s.midnight(it.Day)
	Retime(items, midnight)
	if err := s.logs.ReplaceDay(ctx, it.Day, items); err != nil {
		return err
	}
	s.changed(it.Day)
	return nil
}

func editable(it domain.LogItem) error {
	if it.Status != domain.LogPending {
		return domain.Invalid("item", "Ese elemento ya sonó o está sonando")
	}
	return nil
}

// Move mueve un elemento delta posiciones (−1 arriba, +1 abajo) dentro del día.
func (s *Service) Move(ctx context.Context, id int64, delta int) error {
	return s.edit(ctx, id, func(items []domain.LogItem, i int) ([]domain.LogItem, error) {
		j := i + delta
		if j < 0 || j >= len(items) {
			return items, nil
		}
		if err := editable(items[i]); err != nil {
			return nil, err
		}
		if err := editable(items[j]); err != nil {
			return nil, err
		}
		items[i], items[j] = items[j], items[i]
		items[i].Hour, items[j].Hour = items[j].Hour, items[i].Hour
		return items, nil
	})
}

// Remove quita un elemento pendiente.
func (s *Service) Remove(ctx context.Context, id int64) error {
	return s.edit(ctx, id, func(items []domain.LogItem, i int) ([]domain.LogItem, error) {
		if err := editable(items[i]); err != nil {
			return nil, err
		}
		return append(items[:i], items[i+1:]...), nil
	})
}

// ToggleLock bloquea o desbloquea un elemento (los bloqueados sobreviven a la regeneración).
func (s *Service) ToggleLock(ctx context.Context, id int64) error {
	return s.edit(ctx, id, func(items []domain.LogItem, i int) ([]domain.LogItem, error) {
		items[i].Locked = !items[i].Locked
		return items, nil
	})
}

// InsertTrack inserta una pista de la biblioteca después del elemento afterID (bloqueada).
func (s *Service) InsertTrack(ctx context.Context, afterID, trackID int64) error {
	t, err := s.tracks.Get(ctx, trackID)
	if err != nil {
		return err
	}
	cfg, _ := s.cfg()
	return s.edit(ctx, afterID, func(items []domain.LogItem, i int) ([]domain.LogItem, error) {
		ref := items[i]
		it := domain.LogItem{Hour: ref.Hour, Kind: domain.SlotTrack, TrackID: t.ID, CategoryID: t.CategoryID,
			Title: t.Title, Artist: t.Artist, Length: TrackLength(*t, cfg.Crossfade), Locked: true,
			Status: domain.LogPending, Note: "insertada a mano"}
		if it.Title == "" {
			it.Title = t.Display()
		}
		return append(items[:i+1], append([]domain.LogItem{it}, items[i+1:]...)...), nil
	})
}

// Upcoming devuelve los elementos pendientes desde now (hoy y mañana), en orden.
func (s *Service) Upcoming(ctx context.Context, now time.Time, limit int) ([]domain.LogItem, error) {
	_, loc := s.cfg()
	var out []domain.LogItem
	for i := 0; i < 2 && len(out) < limit; i++ {
		day := now.In(loc).AddDate(0, 0, i).Format(domain.DayLayout)
		items, err := s.logs.Day(ctx, day)
		if err != nil {
			return nil, err
		}
		for _, it := range items {
			if it.Status == domain.LogPending || it.Status == domain.LogPlaying {
				out = append(out, it)
				if len(out) >= limit {
					break
				}
			}
		}
	}
	return out, nil
}

// SetStatus cambia el estado de un elemento (lo usa el aire).
func (s *Service) SetStatus(ctx context.Context, id int64, st domain.LogStatus) error {
	return s.logs.SetStatus(ctx, id, st)
}
