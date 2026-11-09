// Package frequencies gestiona las frecuencias de la radio por ciudad (93.3 FM
// en Huacho, 103.2 FM en Barranca, 105.4 FM en Huaral…).
//
// Cada frecuencia es una emisora: tiene su reproductor en la web, su audio
// (un montaje propio en nuestro Icecast o la URL de un servidor externo), su
// propia PC del estudio (token en Admin → Estudio) y puede tener programación
// y cuñas FM propias (las que no tienen frecuencia valen para todas).
//
// La primera frecuencia activa es la principal: la usan el título de la web,
// el logo y el reproductor fijo. Siempre queda al menos una (Ensure la crea
// al arrancar y Delete no deja borrar la última).
package frequencies

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"radioclub25/internal/domain"
	"radioclub25/internal/service/audit"
)

// Service implementa los casos de uso de frecuencias.
type Service struct {
	repo      domain.FrequencyRepository
	audit     *audit.Service
	mainMount string // montaje de siempre (ICECAST_MOUNT, /radio)
}

// New crea el servicio. mainMount es el montaje principal de Icecast (/radio).
func New(repo domain.FrequencyRepository, au *audit.Service, mainMount string) *Service {
	return &Service{repo: repo, audit: au, mainMount: mainMount}
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

// MountAllowed indica si Icecast debe aceptar una fuente en ese montaje:
// tiene que ser el montaje de una frecuencia activa con audio local.
func (s *Service) MountAllowed(ctx context.Context, mount string) bool {
	list, err := s.repo.List(ctx, true)
	if err != nil {
		return false
	}
	for _, f := range list {
		if !f.IsExternal() && f.Mount == mount {
			return true
		}
	}
	return false
}

// Ensure crea la primera frecuencia si no hay ninguna (instalación nueva),
// con la frecuencia de Ajustes y el montaje principal.
func (s *Service) Ensure(ctx context.Context, settingsFrequency string) error {
	list, err := s.repo.List(ctx, false)
	if err != nil || len(list) > 0 {
		return err
	}
	band := "FM"
	if strings.Contains(strings.ToUpper(settingsFrequency), "AM") {
		band = "AM"
	}
	num := strings.TrimSpace(strings.NewReplacer("FM", "", "fm", "", "AM", "", "am", "").Replace(settingsFrequency))
	f := &domain.Frequency{City: "Huacho", Frequency: num, Band: band, Active: true,
		Source: domain.SourceLocal, Mount: s.mainMount}
	if validate(f) != nil {
		f.Frequency, f.Band = "93.3", "FM"
	}
	return s.repo.Create(ctx, f)
}

// Save crea (ID = 0) o modifica una frecuencia. Si el audio es local y no se
// escribió montaje, propone /radio-<ciudad>.
func (s *Service) Save(ctx context.Context, actor domain.Actor, f *domain.Frequency) error {
	if f.Source == "" {
		f.Source = domain.SourceLocal
	}
	if f.Source == domain.SourceLocal && strings.TrimSpace(f.Mount) == "" {
		f.Mount = s.suggestMount(ctx, f)
	}
	if err := validate(f); err != nil {
		return err
	}
	if f.ID == 0 {
		if err := s.repo.Create(ctx, f); err != nil {
			return err
		}
		s.audit.Log(ctx, actor, "create", "frequency", f.ID, "%s %s (%s)", f.Label(), f.City, f.ListenURL())
		return nil
	}
	if err := s.repo.Update(ctx, f); err != nil {
		return err
	}
	s.audit.Log(ctx, actor, "update", "frequency", f.ID, "%s %s (%s, activa=%t)", f.Label(), f.City, f.ListenURL(), f.Active)
	return nil
}

// suggestMount propone /radio si está libre o /radio-<ciudad>.
func (s *Service) suggestMount(ctx context.Context, f *domain.Frequency) string {
	list, _ := s.repo.List(ctx, false)
	used := map[string]bool{}
	for _, o := range list {
		if o.ID != f.ID && !o.IsExternal() {
			used[o.Mount] = true
		}
	}
	if !used[s.mainMount] {
		return s.mainMount
	}
	slug := Slug(f.City)
	if slug == "" {
		slug = strconv.Itoa(len(list) + 1)
	}
	base := s.mainMount + "-" + slug
	m := base
	for i := 2; used[m]; i++ {
		m = fmt.Sprintf("%s-%d", base, i)
	}
	return m
}

// Delete borra una frecuencia. No deja borrar la última ni una que tenga
// programas o cuñas propias (primero hay que reasignarlos).
func (s *Service) Delete(ctx context.Context, actor domain.Actor, id int64) error {
	f, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	list, err := s.repo.List(ctx, false)
	if err != nil {
		return err
	}
	if len(list) <= 1 {
		return domain.Invalid("frequency", "Debe quedar al menos una frecuencia")
	}
	u, err := s.repo.Usage(ctx, id)
	if err != nil {
		return err
	}
	if u.Programs > 0 || u.Spots > 0 {
		return domain.Invalid("frequency", fmt.Sprintf("%s tiene %d programa(s) y %d cuña(s) propias: "+
			"páselos a otra frecuencia o a «Todas» antes de borrarla", f.City, u.Programs, u.Spots))
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

var (
	freqPattern = regexp.MustCompile(`^\d{2,4}([.,]\d{1,2})?$`)
	// Debe coincidir con la location de nginx (deploy/nginx): /radio o /radio-<nombre>.
	mountPattern = regexp.MustCompile(`^/radio(-[a-z0-9-]{1,40})?$`)
)

func validate(f *domain.Frequency) error {
	f.City = strings.TrimSpace(f.City)
	f.Frequency = strings.ReplaceAll(strings.TrimSpace(f.Frequency), ",", ".")
	f.Band = strings.ToUpper(strings.TrimSpace(f.Band))
	f.Mount = strings.ToLower(strings.TrimSpace(f.Mount))
	f.StreamURL = strings.TrimSpace(f.StreamURL)
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
	case f.SortOrder < 0 || f.SortOrder > 10000:
		return domain.Invalid("sort_order", "Orden fuera de rango")
	}
	v, _ := strconv.ParseFloat(f.Frequency, 64)
	if f.Band == "FM" && (v < 76 || v > 108) {
		return domain.Invalid("frequency", "Una frecuencia FM va de 76.0 a 108.0")
	}
	if f.Band == "AM" && (v < 520 || v > 1710) {
		return domain.Invalid("frequency", "Una frecuencia AM va de 520 a 1710")
	}
	switch f.Source {
	case domain.SourceLocal:
		if !mountPattern.MatchString(f.Mount) {
			return domain.Invalid("mount", "El montaje debe ser /radio o /radio-nombre (minúsculas, números y guiones)")
		}
	case domain.SourceExternal:
		u, err := url.Parse(f.StreamURL)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || len(f.StreamURL) > 300 {
			return domain.Invalid("stream_url", "Escriba la URL completa del stream, por ejemplo https://servidor.com:8000/stream")
		}
	default:
		return domain.Invalid("source", "Elija de dónde sale el audio")
	}
	return nil
}

// Slug pasa "Barranca" o "San José" a "barranca" / "san-jose" (para el montaje).
func Slug(s string) string {
	r := strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n")
	s = r.Replace(strings.ToLower(strings.TrimSpace(s)))
	var b strings.Builder
	dash := false
	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z' || c >= '0' && c <= '9':
			b.WriteRune(c)
			dash = false
		case b.Len() > 0 && !dash:
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.TrimSuffix(b.String(), "-")
	if len(out) > 30 {
		out = strings.TrimSuffix(out[:30], "-")
	}
	return out
}
