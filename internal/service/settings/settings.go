// Package settings gestiona los ajustes editables del sitio (marca, contacto,
// redes y streams). Los ajustes se leen en cada página pública, así que se
// guardan en una caché en memoria que se invalida al guardar.
package settings

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"sync"

	"radioclub25/internal/domain"
	"radioclub25/internal/service/audit"
)

var hexColor = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// Service lee y guarda los ajustes.
type Service struct {
	repo  domain.SettingsRepository
	audit *audit.Service

	mu     sync.RWMutex
	cached *domain.SiteSettings
}

// New crea el servicio.
func New(repo domain.SettingsRepository, au *audit.Service) *Service {
	return &Service{repo: repo, audit: au}
}

// Get devuelve los ajustes actuales (desde la caché si está disponible).
func (s *Service) Get(ctx context.Context) (domain.SiteSettings, error) {
	s.mu.RLock()
	if s.cached != nil {
		v := *s.cached
		s.mu.RUnlock()
		return v, nil
	}
	s.mu.RUnlock()

	m, err := s.repo.GetAll(ctx)
	if err != nil {
		return domain.DefaultSiteSettings(), fmt.Errorf("settings: get: %w", err)
	}
	v := domain.SiteSettingsFromMap(m)
	s.mu.Lock()
	s.cached = &v
	s.mu.Unlock()
	return v, nil
}

// Update valida y guarda todos los ajustes.
func (s *Service) Update(ctx context.Context, actor domain.Actor, v domain.SiteSettings) error {
	if err := validate(&v); err != nil {
		return err
	}
	if err := s.repo.SetMany(ctx, v.ToMap()); err != nil {
		return fmt.Errorf("settings: update: %w", err)
	}
	s.invalidate()
	s.audit.Log(ctx, actor, "update", "settings", 0, "")
	return nil
}

// EnsureDefaults guarda los valores por defecto la primera vez y genera una
// clave de publicación de TV si no existe. Se llama al arrancar.
func (s *Service) EnsureDefaults(ctx context.Context) error {
	m, err := s.repo.GetAll(ctx)
	if err != nil {
		return err
	}
	v := domain.SiteSettingsFromMap(m)
	missing := map[string]string{}
	for k, val := range v.ToMap() {
		if _, ok := m[k]; !ok {
			missing[k] = val
		}
	}
	if v.TVPublishPass == "" {
		missing["tv_publish_pass"] = RandomKey()
	}
	if len(missing) == 0 {
		return nil
	}
	if err := s.repo.SetMany(ctx, missing); err != nil {
		return err
	}
	s.invalidate()
	return nil
}

func (s *Service) invalidate() {
	s.mu.Lock()
	s.cached = nil
	s.mu.Unlock()
}

// RandomKey genera una clave aleatoria de 20 caracteres hexadecimales.
func RandomKey() string {
	b := make([]byte, 10)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func validate(v *domain.SiteSettings) error {
	trim := func(p *string) { *p = strings.TrimSpace(*p) }
	for _, p := range []*string{&v.SiteName, &v.Frequency, &v.Slogan, &v.Email, &v.Phone, &v.WhatsApp,
		&v.ColorNavy, &v.ColorRoyal, &v.ColorRed, &v.ColorOrange, &v.TVPublishUser, &v.TVPublishPass} {
		trim(p)
	}
	if v.SiteName == "" {
		return domain.Invalid("site_name", "El nombre de la radio es obligatorio")
	}
	for field, c := range map[string]string{"color_navy": v.ColorNavy, "color_royal": v.ColorRoyal,
		"color_red": v.ColorRed, "color_orange": v.ColorOrange} {
		if !hexColor.MatchString(c) {
			return domain.Invalid(field, "Color inválido, use formato #RRGGBB")
		}
	}
	if v.Email != "" {
		if _, err := mail.ParseAddress(v.Email); err != nil {
			return domain.Invalid("email", "Email inválido")
		}
	}
	for field, u := range map[string]string{"facebook": v.Facebook, "instagram": v.Instagram, "tiktok": v.TikTok,
		"youtube": v.YouTube, "x": v.X, "map_url": v.MapURL} {
		if u != "" && !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "http://") {
			return domain.Invalid(field, "El enlace debe empezar con https://")
		}
	}
	if v.TVPublishUser == "" || len(v.TVPublishPass) < 8 {
		return domain.Invalid("tv_publish_pass", "Usuario y clave de publicación de TV obligatorios (clave de 8+ caracteres)")
	}
	if strings.ContainsAny(v.TVPublishUser+v.TVPublishPass, " &?#/:@") {
		return domain.Invalid("tv_publish_pass", "Usuario y clave de TV no pueden tener espacios ni & ? # / : @")
	}
	if v.PrerollIntervalMin < 0 || v.PrerollIntervalMin > 1440 {
		return domain.Invalid("preroll_interval_min", "Debe estar entre 0 y 1440 minutos")
	}
	return nil
}
