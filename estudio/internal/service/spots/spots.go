// Package spots gestiona las cuñas de la emisión y arma las tandas: reparte
// las veces por día de cada cuña entre las tandas de su franja (plan.go) y
// replanifica las tandas futuras cuando cambian las cuñas.
//
// Una tanda al aire es: cortina de entrada + cuñas planificadas + cortina de salida.
// Las cortinas salen de la categoría "Cortinas" (si tiene pistas).
package spots

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"radioclub25/estudio/internal/domain"
	"radioclub25/estudio/internal/platform/ffmpeg"
)

// DayEditor aplica un cambio a la playlist de un día (lo implementa scheduler.Service.Apply).
type DayEditor interface {
	Apply(ctx context.Context, day string, f func(items []domain.LogItem) []domain.LogItem, notify bool) error
}

// Prober mide la duración de un archivo (ffmpeg.Tools).
type Prober interface {
	Probe(ctx context.Context, path string) (ffmpeg.Info, error)
}

// Service gestiona cuñas y tandas.
type Service struct {
	spots  domain.SpotRepository
	tandas domain.TandaRepository
	plays  domain.PlayRepository
	cats   domain.CategoryRepository
	tracks domain.TrackRepository
	days   DayEditor
	media  Prober
	loc    func() *time.Location
	log    *slog.Logger
	now    func() time.Time
}

// New crea el servicio.
func New(spots domain.SpotRepository, tandas domain.TandaRepository, plays domain.PlayRepository,
	cats domain.CategoryRepository, tracks domain.TrackRepository, days DayEditor, media Prober,
	loc func() *time.Location, log *slog.Logger) *Service {
	return &Service{spots: spots, tandas: tandas, plays: plays, cats: cats, tracks: tracks, days: days, media: media,
		loc: loc, log: log, now: time.Now}
}

// List devuelve todas las cuñas.
func (s *Service) List(ctx context.Context) ([]domain.Spot, error) { return s.spots.List(ctx) }

// Get devuelve una cuña.
func (s *Service) Get(ctx context.Context, id int64) (*domain.Spot, error) {
	return s.spots.Get(ctx, id)
}

// SaveLocal valida y guarda una cuña local (sin panel web). Mide la duración del archivo.
func (s *Service) SaveLocal(ctx context.Context, sp *domain.Spot) error {
	sp.Advertiser, sp.Name = strings.TrimSpace(sp.Advertiser), strings.TrimSpace(sp.Name)
	switch {
	case sp.Advertiser == "":
		return domain.Invalid("advertiser", "El anunciante es obligatorio")
	case sp.Name == "":
		return domain.Invalid("name", "El nombre es obligatorio")
	case sp.Path == "":
		return domain.Invalid("path", "Elija el archivo de audio")
	case sp.PlaysPerDay < 1 || sp.PlaysPerDay > 96:
		return domain.Invalid("plays_per_day", "Entre 1 y 96 veces por día")
	case sp.HourFrom < 0 || sp.HourTo > 24 || sp.HourFrom >= sp.HourTo:
		return domain.Invalid("hour_to", "Franja horaria inválida")
	case sp.StartsAt != nil && sp.EndsAt != nil && !sp.EndsAt.After(*sp.StartsAt):
		return domain.Invalid("ends_at", "La fecha de fin debe ser posterior al inicio")
	}
	if sp.ID != 0 {
		old, err := s.spots.Get(ctx, sp.ID)
		if err != nil {
			return err
		}
		if old.RemoteID != 0 {
			return domain.Invalid("spot", "Esta cuña viene del panel web: edítela allí")
		}
	}
	info, err := s.media.Probe(ctx, sp.Path)
	if err != nil {
		return domain.Invalid("path", "No se pudo leer el archivo: "+err.Error())
	}
	sp.Duration, sp.RemoteID = info.Duration, 0
	return s.spots.Save(ctx, sp)
}

// Delete borra una cuña local.
func (s *Service) Delete(ctx context.Context, id int64) error {
	sp, err := s.spots.Get(ctx, id)
	if err != nil {
		return err
	}
	if sp.RemoteID != 0 {
		return domain.Invalid("spot", "Esta cuña viene del panel web: desactívela o bórrela allí")
	}
	return s.spots.Delete(ctx, id)
}

// AiredToday cuenta las emisiones de cada cuña hoy (hora de la radio).
func (s *Service) AiredToday(ctx context.Context) (map[int64]int, error) {
	loc := s.loc()
	now := s.now().In(loc)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	plays, err := s.plays.Between(ctx, start, start.AddDate(0, 0, 1))
	if err != nil {
		return nil, err
	}
	out := map[int64]int{}
	for _, p := range plays {
		if p.SpotID > 0 {
			out[p.SpotID]++
		}
	}
	return out, nil
}

// cortinaLen estima la duración de una cortina (promedio de la categoría "Cortinas"; 0 si no hay).
func (s *Service) cortinaLen(ctx context.Context) time.Duration {
	id := s.CortinaCategory(ctx)
	if id == 0 {
		return 0
	}
	list, err := s.tracks.ByCategory(ctx, id)
	if err != nil || len(list) == 0 {
		return 0
	}
	var total time.Duration
	for _, t := range list {
		total += t.PlayLength()
	}
	return total / time.Duration(len(list))
}

// CortinaCategory devuelve la categoría de cortinas de tanda (la llamada "Cortinas"), o 0.
func (s *Service) CortinaCategory(ctx context.Context) int64 {
	cats, err := s.cats.List(ctx)
	if err != nil {
		return 0
	}
	for _, c := range cats {
		if strings.EqualFold(c.Name, "Cortinas") {
			return c.ID
		}
	}
	return 0
}

// Replan vuelve a repartir las cuñas en las tandas pendientes de un día y
// actualiza la duración y el título de cada tanda en la playlist.
func (s *Service) Replan(ctx context.Context, day string) (map[int64]int, error) {
	loc := s.loc()
	list, err := s.spots.List(ctx)
	if err != nil {
		return nil, err
	}
	aired := map[int64]int{}
	now := s.now()
	if day == now.In(loc).Format(domain.DayLayout) {
		if aired, err = s.AiredToday(ctx); err != nil {
			return nil, err
		}
	}
	byID := map[int64]domain.Spot{}
	var needs []Need
	for _, sp := range list {
		byID[sp.ID] = sp
		needs = append(needs, Need{Spot: sp, Count: sp.PlaysPerDay - aired[sp.ID]})
	}
	cortinas := s.cortinaLen(ctx)
	var missing map[int64]int
	err = s.days.Apply(ctx, day, func(items []domain.LogItem) []domain.LogItem {
		var tandas []Tanda
		for _, it := range items {
			if it.Kind == domain.SlotSpots && it.Status == domain.LogPending && it.At.After(now) {
				tandas = append(tandas, Tanda{ID: it.ID, At: it.At})
			}
		}
		var plan map[int64][]int64
		plan, missing = Plan(tandas, needs, loc)
		for i := range items {
			it := &items[i]
			if it.Kind != domain.SlotSpots || it.Status != domain.LogPending || !it.At.After(now) {
				continue
			}
			ids := plan[it.ID]
			if err := s.tandas.Set(ctx, it.ID, ids); err != nil {
				s.log.Error("guardar tanda", "item", it.ID, "err", err)
			}
			var total time.Duration
			for _, id := range ids {
				total += byID[id].Duration
			}
			if len(ids) > 0 {
				total += 2 * cortinas
				it.Title = fmt.Sprintf("Tanda · %d cuñas", len(ids))
			} else {
				it.Title = "Tanda (sin cuñas)"
			}
			it.Length = total
		}
		return items
	}, false)
	return missing, err
}

// TandaSpots devuelve las cuñas planificadas de una tanda (en orden; omite las que ya no existen).
func (s *Service) TandaSpots(ctx context.Context, logItemID int64) ([]domain.Spot, error) {
	m, err := s.tandas.ForItems(ctx, []int64{logItemID})
	if err != nil {
		return nil, err
	}
	var out []domain.Spot
	for _, id := range m[logItemID] {
		if sp, err := s.spots.Get(ctx, id); err == nil {
			out = append(out, *sp)
		}
	}
	return out, nil
}

// TandaView es una tanda del día con sus cuñas, para la interfaz.
type TandaView struct {
	LogItemID int64         `json:"log_item_id"`
	At        time.Time     `json:"at"`
	Status    string        `json:"status"`
	Spots     []domain.Spot `json:"spots"`
}

// DayTandas devuelve las tandas de un día con su contenido.
func (s *Service) DayTandas(ctx context.Context, items []domain.LogItem) ([]TandaView, error) {
	var ids []int64
	for _, it := range items {
		if it.Kind == domain.SlotSpots {
			ids = append(ids, it.ID)
		}
	}
	m, err := s.tandas.ForItems(ctx, ids)
	if err != nil {
		return nil, err
	}
	all, err := s.spots.List(ctx)
	if err != nil {
		return nil, err
	}
	byID := map[int64]domain.Spot{}
	for _, sp := range all {
		byID[sp.ID] = sp
	}
	var out []TandaView
	for _, it := range items {
		if it.Kind != domain.SlotSpots {
			continue
		}
		v := TandaView{LogItemID: it.ID, At: it.At, Status: string(it.Status), Spots: []domain.Spot{}}
		for _, id := range m[it.ID] {
			if sp, ok := byID[id]; ok {
				v.Spots = append(v.Spots, sp)
			}
		}
		out = append(out, v)
	}
	return out, nil
}
