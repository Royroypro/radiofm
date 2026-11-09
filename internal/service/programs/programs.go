// Package programs gestiona la programación semanal de la radio y calcula
// qué programa está al aire.
package programs

import (
	"context"
	"strings"
	"time"

	"radioclub25/internal/domain"
	"radioclub25/internal/service/audit"
)

// Service implementa los casos de uso de programación.
type Service struct {
	repo  domain.ProgramRepository
	audit *audit.Service
	loc   *time.Location
}

// New crea el servicio. loc es la zona horaria de la radio.
func New(repo domain.ProgramRepository, au *audit.Service, loc *time.Location) *Service {
	return &Service{repo: repo, audit: au, loc: loc}
}

// List devuelve todos los programas (lunes primero).
func (s *Service) List(ctx context.Context) ([]domain.Program, error) { return s.repo.List(ctx) }

// Get devuelve un programa.
func (s *Service) Get(ctx context.Context, id int64) (*domain.Program, error) {
	return s.repo.Get(ctx, id)
}

// Create valida y crea un programa.
func (s *Service) Create(ctx context.Context, actor domain.Actor, p *domain.Program) error {
	if err := validate(p); err != nil {
		return err
	}
	if err := s.repo.Create(ctx, p); err != nil {
		return err
	}
	s.audit.Log(ctx, actor, "create", "program", p.ID, p.Title)
	return nil
}

// Update valida y modifica un programa.
func (s *Service) Update(ctx context.Context, actor domain.Actor, p *domain.Program) error {
	if err := validate(p); err != nil {
		return err
	}
	if err := s.repo.Update(ctx, p); err != nil {
		return err
	}
	s.audit.Log(ctx, actor, "update", "program", p.ID, p.Title)
	return nil
}

// Delete borra un programa.
func (s *Service) Delete(ctx context.Context, actor domain.Actor, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.audit.Log(ctx, actor, "delete", "program", id, "")
	return nil
}

// OnAir devuelve el programa al aire en el instante now en la frecuencia fid
// (0 = cualquiera), o nil.
func (s *Service) OnAir(ctx context.Context, now time.Time, fid int64) (*domain.Program, error) {
	list, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	local := now.In(s.loc)
	for i := range list {
		if list[i].AppliesTo(fid) && list[i].IsOnAir(local) {
			return &list[i], nil
		}
	}
	return nil, nil
}

// DaySchedule son los programas activos de un día.
type DaySchedule struct {
	Weekday  time.Weekday
	Name     string
	Programs []domain.Program
	IsToday  bool
}

// Week devuelve la programación activa de la frecuencia fid (0 = todas)
// agrupada por día, de lunes a domingo.
func (s *Service) Week(ctx context.Context, now time.Time, fid int64) ([]DaySchedule, error) {
	list, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	today := now.In(s.loc).Weekday()
	days := make([]DaySchedule, 7)
	for i := range days {
		wd := time.Weekday((i + 1) % 7) // lunes=1 ... domingo=0
		days[i] = DaySchedule{Weekday: wd, Name: domain.WeekdayNames[wd], IsToday: wd == today}
	}
	for _, p := range list {
		if p.Active && p.AppliesTo(fid) {
			idx := (int(p.Weekday) + 6) % 7
			days[idx].Programs = append(days[idx].Programs, p)
		}
	}
	return days, nil
}

func validate(p *domain.Program) error {
	p.Title = strings.TrimSpace(p.Title)
	p.Host = strings.TrimSpace(p.Host)
	if p.Title == "" {
		return domain.Invalid("title", "El título es obligatorio")
	}
	if p.Weekday < 0 || p.Weekday > 6 {
		return domain.Invalid("weekday", "Día inválido")
	}
	if !domain.ValidTime(p.StartTime) || !domain.ValidTime(p.EndTime) {
		return domain.Invalid("start_time", "Use horas en formato HH:MM")
	}
	if p.StartTime == p.EndTime {
		return domain.Invalid("end_time", "La hora de fin debe ser distinta a la de inicio")
	}
	return nil
}
