// Package frequencies gestiona las frecuencias de la radio por ciudad (93.3 FM
// en Huacho, 103.2 FM en Barranca, 105.4 FM en Huaral…): alta, edición, orden
// y baja desde el panel (con auditoría) y la lista que muestra la web.
//
// La primera frecuencia activa es la principal: la usan el título de la web,
// el logo y el reproductor (reemplaza al ajuste "Frecuencia" de Marca).
package frequencies

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"radioclub25/internal/domain"
	"radioclub25/internal/service/audit"
)

// Service implementa los casos de uso de frecuencias.
type Service struct {
	repo  domain.FrequencyRepository
	audit *audit.Service
}

// New crea el servicio.
func New(repo domain.FrequencyRepository, au *audit.Service) *Service {
	return &Service{repo: repo, audit: au}
}

// List devuelve todas las frecuencias en orden.
func (s *Service) List(ctx context.Context) ([]domain.Frequency, error) {
	return s.repo.List(ctx, false)
}

// Active devuelve las frecuencias activas en orden (la primera es la principal).
func (s *Service) Active(ctx context.Context) ([]domain.Frequency, error) {
	return s.repo.List(ctx, true)
}

// Get devuelve una frecuencia.
func (s *Service) Get(ctx context.Context, id int64) (*domain.Frequency, error) {
	return s.repo.Get(ctx, id)
}

// Save crea (ID = 0) o modifica una frecuencia.
func (s *Service) Save(ctx context.Context, actor domain.Actor, f *domain.Frequency) error {
	if err := validate(f); err != nil {
		return err
	}
	if f.ID == 0 {
		if err := s.repo.Create(ctx, f); err != nil {
			return err
		}
		s.audit.Log(ctx, actor, "create", "frequency", f.ID, "%s %s", f.Label(), f.City)
		return nil
	}
	if err := s.repo.Update(ctx, f); err != nil {
		return err
	}
	s.audit.Log(ctx, actor, "update", "frequency", f.ID, "%s %s (activa=%t)", f.Label(), f.City, f.Active)
	return nil
}

// Delete borra una frecuencia.
func (s *Service) Delete(ctx context.Context, actor domain.Actor, id int64) error {
	f, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.audit.Log(ctx, actor, "delete", "frequency", id, "%s %s", f.Label(), f.City)
	return nil
}

// Move sube (delta −1) o baja (+1) una frecuencia en el orden de la web.
func (s *Service) Move(ctx context.Context, actor domain.Actor, id int64, delta int) error {
	list, err := s.repo.List(ctx, false)
	if err != nil {
		return err
	}
	i := -1
	for k := range list {
		if list[k].ID == id {
			i = k
		}
	}
	j := i + delta
	if i < 0 || j < 0 || j >= len(list) {
		return nil
	}
	list[i], list[j] = list[j], list[i]
	for k := range list { // renumerar 0, 10, 20… para dejar huecos
		if list[k].SortOrder != k*10 {
			list[k].SortOrder = k * 10
			if err := s.repo.Update(ctx, &list[k]); err != nil {
				return err
			}
		}
	}
	s.audit.Log(ctx, actor, "update", "frequency", id, "orden")
	return nil
}

var freqPattern = regexp.MustCompile(`^\d{2,4}([.,]\d{1,2})?$`)

func validate(f *domain.Frequency) error {
	f.City = strings.TrimSpace(f.City)
	f.Frequency = strings.ReplaceAll(strings.TrimSpace(f.Frequency), ",", ".")
	f.Band = strings.ToUpper(strings.TrimSpace(f.Band))
	if f.Band == "" {
		f.Band = "FM"
	}
	switch {
	case f.City == "":
		return domain.Invalid("city", "La ciudad es obligatoria")
	case utf8.RuneCountInString(f.City) > 60:
		return domain.Invalid("city", "Máximo 60 caracteres")
	case f.Band != "FM" && f.Band != "AM":
		return domain.Invalid("band", "La banda debe ser FM o AM")
	case !freqPattern.MatchString(f.Frequency):
		return domain.Invalid("frequency", "Escriba solo el número, por ejemplo 93.3")
	}
	v, _ := strconv.ParseFloat(f.Frequency, 64)
	if f.Band == "FM" && (v < 76 || v > 108) {
		return domain.Invalid("frequency", "Una frecuencia FM va de 76.0 a 108.0")
	}
	if f.Band == "AM" && (v < 520 || v > 1710) {
		return domain.Invalid("frequency", "Una frecuencia AM va de 520 a 1710")
	}
	if f.SortOrder < 0 || f.SortOrder > 10000 {
		return domain.Invalid("sort_order", "Orden fuera de rango")
	}
	return nil
}
