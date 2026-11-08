// Package ads gestiona la publicidad: alta y edición de anuncios, selección
// del anuncio a mostrar en cada hueco y medición (impresiones, clics...).
//
// Selección: entre los anuncios vigentes de un hueco (tipo + posición) se
// elige uno al azar ponderado por Weight. Con pesos 3 y 1, el primero sale
// el 75 % de las veces.
//
// Licencia: si el nivel de licencia desactiva la publicidad, Pick no devuelve
// nada. Los límites del plan (anuncios activos, estadísticas) se aplican aquí.
package ads

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"math/rand/v2"
	"net/url"
	"strings"
	"time"

	"radioclub25/internal/domain"
	"radioclub25/internal/service/audit"
)

// LicenseReader da acceso al estado de licencia actual.
type LicenseReader interface {
	Current() domain.LicenseState
}

// FileRemover borra archivos subidos (lo implementa platform/storage).
type FileRemover interface {
	Delete(publicURL string) error
}

// Service implementa los casos de uso de publicidad.
type Service struct {
	ads     domain.AdRepository
	events  domain.AdEventRepository
	license LicenseReader
	files   FileRemover
	audit   *audit.Service
	salt    string
	now     func() time.Time
}

// New crea el servicio. salt se usa para anonimizar las IPs de los eventos.
func New(ads domain.AdRepository, events domain.AdEventRepository, lic LicenseReader, files FileRemover,
	au *audit.Service, salt string) *Service {
	return &Service{ads: ads, events: events, license: lic, files: files, audit: au, salt: salt, now: time.Now}
}

// List devuelve los anuncios según el filtro.
func (s *Service) List(ctx context.Context, f domain.AdFilter) ([]domain.Ad, error) {
	return s.ads.List(ctx, f)
}

// Get devuelve un anuncio.
func (s *Service) Get(ctx context.Context, id int64) (*domain.Ad, error) { return s.ads.Get(ctx, id) }

// Create valida y crea un anuncio respetando el límite de anuncios activos del plan.
func (s *Service) Create(ctx context.Context, actor domain.Actor, a *domain.Ad) error {
	if err := validate(a); err != nil {
		return err
	}
	if a.Active {
		if err := s.checkActiveLimit(ctx); err != nil {
			return err
		}
	}
	if err := s.ads.Create(ctx, a); err != nil {
		return err
	}
	s.audit.Log(ctx, actor, "create", "ad", a.ID, "%s (%s)", a.Name, a.Type)
	return nil
}

// Update valida y modifica un anuncio. Si se reemplazó el archivo, borra el anterior.
func (s *Service) Update(ctx context.Context, actor domain.Actor, a *domain.Ad) error {
	if err := validate(a); err != nil {
		return err
	}
	old, err := s.ads.Get(ctx, a.ID)
	if err != nil {
		return err
	}
	if a.Active && !old.Active {
		if err := s.checkActiveLimit(ctx); err != nil {
			return err
		}
	}
	if err := s.ads.Update(ctx, a); err != nil {
		return err
	}
	if old.FileURL != a.FileURL {
		_ = s.files.Delete(old.FileURL)
	}
	s.audit.Log(ctx, actor, "update", "ad", a.ID, "%s (activo=%t)", a.Name, a.Active)
	return nil
}

// Delete borra un anuncio, sus eventos y su archivo.
func (s *Service) Delete(ctx context.Context, actor domain.Actor, id int64) error {
	a, err := s.ads.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := s.ads.Delete(ctx, id); err != nil {
		return err
	}
	_ = s.files.Delete(a.FileURL)
	s.audit.Log(ctx, actor, "delete", "ad", id, a.Name)
	return nil
}

// checkActiveLimit devuelve ErrPlanLimit si ya se alcanzó el máximo de anuncios activos del plan.
func (s *Service) checkActiveLimit(ctx context.Context) error {
	max := s.license.Current().Limits.MaxActiveAds
	if max <= 0 {
		return nil
	}
	n, err := s.ads.CountActive(ctx)
	if err != nil {
		return err
	}
	if n >= max {
		return domain.ErrPlanLimit
	}
	return nil
}

// Enabled indica si se debe servir publicidad según la licencia.
func (s *Service) Enabled() bool { return !s.license.Current().Level.AdsDisabled() }

// PickBanners elige un banner vigente por posición. Las posiciones sin anuncio no aparecen en el mapa.
func (s *Service) PickBanners(ctx context.Context) (map[domain.AdPosition]*domain.Ad, error) {
	out := map[domain.AdPosition]*domain.Ad{}
	if !s.Enabled() {
		return out, nil
	}
	list, err := s.ads.List(ctx, domain.AdFilter{Type: domain.AdBanner, OnlyActive: true})
	if err != nil {
		return out, err
	}
	byPos := map[domain.AdPosition][]domain.Ad{}
	now := s.now()
	for _, a := range list {
		if a.IsLive(now) {
			byPos[a.Position] = append(byPos[a.Position], a)
		}
	}
	for pos, cands := range byPos {
		out[pos] = weightedPick(cands)
	}
	return out, nil
}

// Pick elige un anuncio vigente del tipo indicado (para pre-roll y cuñas). Devuelve nil si no hay.
func (s *Service) Pick(ctx context.Context, t domain.AdType) (*domain.Ad, error) {
	if !s.Enabled() {
		return nil, nil
	}
	list, err := s.ads.List(ctx, domain.AdFilter{Type: t, OnlyActive: true})
	if err != nil {
		return nil, err
	}
	now := s.now()
	var live []domain.Ad
	for _, a := range list {
		if a.IsLive(now) {
			live = append(live, a)
		}
	}
	return weightedPick(live), nil
}

// RecordEvent guarda un evento de medición. ip se anonimiza con un hash diario.
func (s *Service) RecordEvent(ctx context.Context, adID int64, t domain.AdEventType, ip string) error {
	if !t.Valid() {
		return domain.Invalid("type", "Tipo de evento inválido")
	}
	now := s.now()
	return s.events.Record(ctx, domain.AdEvent{AdID: adID, Type: t, IPHash: s.hashIP(ip, now), CreatedAt: now})
}

// Stats devuelve las estadísticas por anuncio en [from, to). Requiere un plan con estadísticas.
func (s *Service) Stats(ctx context.Context, from, to time.Time) ([]domain.AdStats, error) {
	if !s.license.Current().Limits.StatsEnabled {
		return nil, domain.ErrPlanLimit
	}
	return s.events.Stats(ctx, from, to)
}

// hashIP devuelve un hash que cambia cada día: permite contar visitantes
// únicos por día sin guardar la IP.
func (s *Service) hashIP(ip string, t time.Time) string {
	sum := sha256.Sum256([]byte(s.salt + "|" + t.UTC().Format("2006-01-02") + "|" + ip))
	return hex.EncodeToString(sum[:8])
}

func weightedPick(cands []domain.Ad) *domain.Ad {
	if len(cands) == 0 {
		return nil
	}
	total := 0
	for _, a := range cands {
		total += max(a.Weight, 1)
	}
	r := rand.IntN(total)
	for i := range cands {
		r -= max(cands[i].Weight, 1)
		if r < 0 {
			return &cands[i]
		}
	}
	return &cands[len(cands)-1]
}

func validate(a *domain.Ad) error {
	a.Name = strings.TrimSpace(a.Name)
	a.LinkURL = strings.TrimSpace(a.LinkURL)
	if a.Name == "" {
		return domain.Invalid("name", "El nombre es obligatorio")
	}
	if !a.Type.Valid() {
		return domain.Invalid("type", "Tipo de anuncio inválido")
	}
	if a.Type == domain.AdBanner {
		if !a.Position.Valid() {
			return domain.Invalid("position", "Seleccione la posición del banner")
		}
	} else {
		a.Position = ""
	}
	if a.FileURL == "" {
		return domain.Invalid("file", "Suba el archivo del anuncio")
	}
	if a.LinkURL != "" {
		u, err := url.Parse(a.LinkURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return domain.Invalid("link_url", "El enlace debe ser una URL completa (https://...)")
		}
	}
	if a.Weight < 1 || a.Weight > 100 {
		return domain.Invalid("weight", "El peso debe estar entre 1 y 100")
	}
	if a.SkipAfterSec < 0 || a.SkipAfterSec > 120 {
		return domain.Invalid("skip_after_sec", "Debe estar entre 0 y 120 segundos")
	}
	if a.StartsAt != nil && a.EndsAt != nil && !a.EndsAt.After(*a.StartsAt) {
		return domain.Invalid("ends_at", "La fecha de fin debe ser posterior al inicio")
	}
	return nil
}
