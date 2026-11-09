package studio

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"radioclub25/internal/domain"
)

// Tráfico publicitario FM: anunciantes (con rubro), campañas (reglas) y sus
// versiones (los audios, filas de fm_spots). La PC del estudio aplica las
// reglas al armar las tandas (estudio/internal/service/spots).

// WithTraffic conecta los repositorios de anunciantes y campañas.
func (s *Service) WithTraffic(adv domain.FMAdvertiserRepository, camp domain.FMCampaignRepository) *Service {
	s.adv, s.camp = adv, camp
	return s
}

// WithClock cambia el reloj (para tests).
func (s *Service) WithClock(now func() time.Time) *Service {
	s.now = now
	return s
}

// ---------- Anunciantes ----------

// Advertisers devuelve los anunciantes.
func (s *Service) Advertisers(ctx context.Context) ([]domain.FMAdvertiser, error) {
	return s.adv.List(ctx)
}

// Advertiser devuelve un anunciante.
func (s *Service) Advertiser(ctx context.Context, id int64) (*domain.FMAdvertiser, error) {
	return s.adv.Get(ctx, id)
}

// SaveAdvertiser crea o modifica un anunciante (el nombre y el rubro se copian a sus cuñas).
func (s *Service) SaveAdvertiser(ctx context.Context, actor domain.Actor, a *domain.FMAdvertiser) error {
	a.Name, a.Sector = strings.TrimSpace(a.Name), normalizeSector(a.Sector)
	switch {
	case a.Name == "":
		return domain.Invalid("name", "El nombre es obligatorio")
	case utf8.RuneCountInString(a.Name) > 120:
		return domain.Invalid("name", "Máximo 120 caracteres")
	case utf8.RuneCountInString(a.Sector) > 60:
		return domain.Invalid("sector", "Máximo 60 caracteres")
	}
	isNew := a.ID == 0
	if err := s.adv.Save(ctx, a); err != nil {
		return err
	}
	action := "update"
	if isNew {
		action = "create"
	}
	s.audit.Log(ctx, actor, action, "fm_advertiser", a.ID, "%s (rubro %q)", a.Name, a.Sector)
	return nil
}

// DeleteAdvertiser borra un anunciante sin campañas.
func (s *Service) DeleteAdvertiser(ctx context.Context, actor domain.Actor, id int64) error {
	a, err := s.adv.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := s.adv.Delete(ctx, id); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return domain.Invalid("advertiser", "Tiene campañas: bórrelas primero")
		}
		return err
	}
	s.audit.Log(ctx, actor, "delete", "fm_advertiser", id, "%s", a.Name)
	return nil
}

// normalizeSector deja el rubro en minúsculas y sin espacios de más ("Banca " = "banca").
func normalizeSector(s string) string { return strings.Join(strings.Fields(strings.ToLower(s)), " ") }

// ---------- Campañas ----------

// Campaigns devuelve las campañas (sin versiones).
func (s *Service) Campaigns(ctx context.Context) ([]domain.FMCampaign, error) {
	return s.camp.List(ctx)
}

// Campaign devuelve una campaña con sus versiones.
func (s *Service) Campaign(ctx context.Context, id int64) (*domain.FMCampaign, error) {
	c, err := s.camp.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if c.Versions, err = s.spots.ByCampaign(ctx, id); err != nil {
		return nil, err
	}
	return c, nil
}

// SaveCampaign crea o modifica una campaña. Si AdvertiserID es 0 se usa (o se
// crea) el anunciante advertiserName. Las reglas se copian a sus versiones.
func (s *Service) SaveCampaign(ctx context.Context, actor domain.Actor, c *domain.FMCampaign, advertiserName string) error {
	c.Name = strings.TrimSpace(c.Name)
	if c.AdvertiserID == 0 {
		name := strings.TrimSpace(advertiserName)
		if name == "" {
			return domain.Invalid("advertiser", "Elija o escriba el anunciante")
		}
		a, err := s.adv.ByName(ctx, name)
		if errors.Is(err, domain.ErrNotFound) {
			a = &domain.FMAdvertiser{Name: name}
			err = s.SaveAdvertiser(ctx, actor, a)
		}
		if err != nil {
			return err
		}
		c.AdvertiserID = a.ID
	}
	if c.DaysMask == 0 {
		c.DaysMask = domain.AllDays
	}
	if c.Priority == 0 {
		c.Priority = 2
	}
	if c.Position == "" {
		c.Position = domain.PositionAny
	}
	if err := validateCampaign(c); err != nil {
		return err
	}
	isNew := c.ID == 0
	if err := s.camp.Save(ctx, c); err != nil {
		return err
	}
	saved, err := s.camp.Get(ctx, c.ID) // completa anunciante y rubro
	if err != nil {
		return err
	}
	c.Advertiser, c.Sector = saved.Advertiser, saved.Sector
	if err := s.camp.SyncVersions(ctx, *saved); err != nil {
		return err
	}
	action := "update"
	if isNew {
		action = "create"
	}
	s.audit.Log(ctx, actor, action, "fm_campaign", c.ID, "%s — %s (%d/día, %s, %s)", c.Advertiser, c.Name,
		c.PlaysPerDay, c.HoursLabel(), c.DaysLabel())
	return nil
}

func validateCampaign(c *domain.FMCampaign) error {
	switch {
	case c.Name == "":
		return domain.Invalid("name", "El nombre de la campaña es obligatorio")
	case utf8.RuneCountInString(c.Name) > 120:
		return domain.Invalid("name", "Máximo 120 caracteres")
	case c.PlaysPerDay < 1 || c.PlaysPerDay > 96:
		return domain.Invalid("plays_per_day", "Debe estar entre 1 y 96 veces por día")
	case c.HourFrom < 0 || c.HourFrom > 23 || c.HourTo < 1 || c.HourTo > 24 || c.HourFrom >= c.HourTo:
		return domain.Invalid("hour_to", "La franja debe ir de 00 a 24 h y terminar después de empezar")
	case c.DaysMask < 1 || c.DaysMask > domain.AllDays:
		return domain.Invalid("days", "Elija al menos un día")
	case c.Position != domain.PositionAny && c.Position != domain.PositionFirst && c.Position != domain.PositionLast:
		return domain.Invalid("position", "Posición inválida")
	case c.Priority < 1 || c.Priority > 3:
		return domain.Invalid("priority", "Prioridad inválida")
	case c.StartsAt != nil && c.EndsAt != nil && !c.EndsAt.After(*c.StartsAt):
		return domain.Invalid("ends_at", "La fecha de fin debe ser posterior al inicio")
	}
	return nil
}

// DeleteCampaign borra una campaña, sus versiones y sus archivos. Las
// emisiones ya registradas se conservan en el comprobante.
func (s *Service) DeleteCampaign(ctx context.Context, actor domain.Actor, id int64) error {
	c, err := s.Campaign(ctx, id)
	if err != nil {
		return err
	}
	if err := s.camp.Delete(ctx, id); err != nil {
		return err
	}
	for _, v := range c.Versions {
		_ = s.files.Delete(v.FileURL)
	}
	s.audit.Log(ctx, actor, "delete", "fm_campaign", id, "%s — %s", c.Advertiser, c.Name)
	return nil
}

// SaveVersion crea (ID = 0) o modifica una versión de una campaña: recibe las
// reglas de la campaña; el archivo nuevo se verifica con su SHA-256.
func (s *Service) SaveVersion(ctx context.Context, actor domain.Actor, campaignID int64, v *domain.FMSpot) error {
	c, err := s.camp.Get(ctx, campaignID)
	if err != nil {
		return err
	}
	if v.Weight < 1 || v.Weight > 10 {
		return domain.Invalid("weight", "El peso debe estar entre 1 y 10")
	}
	if strings.TrimSpace(v.Name) == "" {
		v.Name = c.Name
	}
	if v.ID == 0 {
		v.Active = true
	}
	c.Apply(v)
	if v.ID == 0 {
		return s.CreateSpot(ctx, actor, v)
	}
	return s.UpdateSpot(ctx, actor, v)
}

// ---------- Cumplimiento ----------

// CampaignCompliance es el cumplimiento de una campaña en un rango de fechas.
type CampaignCompliance struct {
	Campaign  domain.FMCampaign
	Scheduled int // veces que debía sonar (veces/día × días en que aplica)
	Aired     int // emisiones registradas
	Complete  int // emisiones completas
}

// Missing son las emisiones que faltan (nunca negativo).
func (c CampaignCompliance) Missing() int { return max(c.Scheduled-c.Aired, 0) }

// Percent es el porcentaje cumplido (0–100).
func (c CampaignCompliance) Percent() int {
	if c.Scheduled == 0 {
		return 100
	}
	return min(100, c.Aired*100/c.Scheduled)
}

// Compliance calcula, por campaña, lo programado y lo emitido entre from y to
// (fechas locales de la radio, to exclusivo). Para el día en curso cuenta solo
// lo que ya debía haber sonado: la parte de la franja que pasó.
func (s *Service) Compliance(ctx context.Context, from, to time.Time) ([]CampaignCompliance, error) {
	camps, err := s.camp.List(ctx)
	if err != nil {
		return nil, err
	}
	spots, err := s.spots.List(ctx, false)
	if err != nil {
		return nil, err
	}
	campOf := map[int64]int64{}
	for _, sp := range spots {
		campOf[sp.ID] = sp.CampaignID
	}
	plays, err := s.plays.List(ctx, domain.AirplayFilter{From: from, To: to})
	if err != nil {
		return nil, err
	}
	now := s.now().In(s.loc)
	out := make([]CampaignCompliance, 0, len(camps))
	index := map[int64]int{}
	for _, c := range camps {
		cc := CampaignCompliance{Campaign: c}
		for d := from.In(s.loc); d.Before(to); d = d.AddDate(0, 0, 1) {
			if !c.PlaysOn(d) {
				continue
			}
			cc.Scheduled += expectedOn(c, d, now)
		}
		index[c.ID] = len(out)
		out = append(out, cc)
	}
	for _, p := range plays {
		if i, ok := index[campOf[p.SpotID]]; ok {
			out[i].Aired++
			if p.Complete {
				out[i].Complete++
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Missing() > out[j].Missing() })
	return out, nil
}

// expectedOn es cuántas veces debía haber sonado la campaña el día d hasta now:
// todas si el día ya pasó; proporcional a la franja transcurrida si es hoy; 0 si es futuro.
func expectedOn(c domain.FMCampaign, d, now time.Time) int {
	day := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, d.Location())
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch {
	case day.Before(today):
		return c.PlaysPerDay
	case day.After(today):
		return 0
	}
	from, to := day.Add(time.Duration(c.HourFrom)*time.Hour), day.Add(time.Duration(c.HourTo)*time.Hour)
	switch {
	case !now.After(from):
		return 0
	case !now.Before(to):
		return c.PlaysPerDay
	}
	return int(float64(c.PlaysPerDay) * float64(now.Sub(from)) / float64(to.Sub(from)))
}

// TodayShortfall son las campañas que hoy van atrasadas (para el aviso del panel).
func (s *Service) TodayShortfall(ctx context.Context) ([]CampaignCompliance, error) {
	now := s.now().In(s.loc)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.loc)
	all, err := s.Compliance(ctx, start, start.AddDate(0, 0, 1))
	if err != nil {
		return nil, err
	}
	var out []CampaignCompliance
	for _, c := range all {
		if c.Missing() > 0 {
			out = append(out, c)
		}
	}
	return out, nil
}
