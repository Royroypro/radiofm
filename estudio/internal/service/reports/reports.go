// Package reports arma los reportes locales a partir del historial de
// emisiones (as-run): todo lo que sonó y el comprobante de cuñas por
// anunciante, en pantalla y en CSV (separador ";" y BOM para que Excel lea
// bien las tildes, igual que el panel web). El comprobante oficial para el
// anunciante es el del panel web (se arma con las emisiones sincronizadas).
package reports

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
	"time"

	"radioclub25/estudio/internal/domain"
)

// Service genera reportes.
type Service struct {
	plays domain.PlayRepository
	spots domain.SpotRepository
	loc   func() *time.Location
}

// New crea el servicio.
func New(plays domain.PlayRepository, spots domain.SpotRepository, loc func() *time.Location) *Service {
	return &Service{plays: plays, spots: spots, loc: loc}
}

// Range convierte fechas YYYY-MM-DD (inclusive) al rango [desde, hasta+1 día) en la zona de la radio.
func (s *Service) Range(from, to string) (time.Time, time.Time, error) {
	loc := s.loc()
	f, err := time.ParseInLocation(domain.DayLayout, from, loc)
	if err != nil {
		return f, f, domain.Invalid("from", "Fecha de inicio inválida")
	}
	t, err := time.ParseInLocation(domain.DayLayout, to, loc)
	if err != nil {
		return f, f, domain.Invalid("to", "Fecha de fin inválida")
	}
	if t.Before(f) {
		f, t = t, f
	}
	return f, t.AddDate(0, 0, 1), nil
}

// AsRun devuelve todo lo que sonó en el rango (fechas inclusive).
func (s *Service) AsRun(ctx context.Context, from, to string) ([]domain.Play, error) {
	f, t, err := s.Range(from, to)
	if err != nil {
		return nil, err
	}
	return s.plays.Between(ctx, f, t)
}

// ProofRow es una emisión de cuña para el comprobante.
type ProofRow struct {
	At         time.Time     `json:"at"`
	Advertiser string        `json:"advertiser"`
	Spot       string        `json:"spot"`
	Duration   time.Duration `json:"duration"`
	Complete   bool          `json:"complete"`
	UUID       string        `json:"uuid"`
}

// SpotProof devuelve las emisiones de cuñas del rango, opcionalmente de un anunciante.
func (s *Service) SpotProof(ctx context.Context, from, to, advertiser string) ([]ProofRow, error) {
	plays, err := s.AsRun(ctx, from, to)
	if err != nil {
		return nil, err
	}
	spots, err := s.spots.List(ctx)
	if err != nil {
		return nil, err
	}
	byID := map[int64]domain.Spot{}
	for _, sp := range spots {
		byID[sp.ID] = sp
	}
	adv := strings.ToLower(strings.TrimSpace(advertiser))
	out := []ProofRow{}
	for _, p := range plays {
		if p.Kind != domain.PlaySpot {
			continue
		}
		row := ProofRow{At: p.StartedAt, Advertiser: p.Artist, Spot: p.Title, Duration: p.Duration, Complete: p.Complete, UUID: p.UUID}
		if sp, ok := byID[p.SpotID]; ok {
			row.Advertiser, row.Spot = sp.Advertiser, sp.Name
		}
		if adv != "" && strings.ToLower(row.Advertiser) != adv {
			continue
		}
		out = append(out, row)
	}
	return out, nil
}

func newCSV(w io.Writer) *csv.Writer {
	_, _ = w.Write([]byte("\xEF\xBB\xBF"))
	cw := csv.NewWriter(w)
	cw.Comma = ';'
	return cw
}

func yesNo(b bool) string {
	if b {
		return "Sí"
	}
	return "No"
}

var kindLabel = map[domain.PlayKind]string{domain.PlayMusic: "Música", domain.PlayJingle: "Jingle", domain.PlaySpot: "Cuña",
	domain.PlayVoice: "Locución", domain.PlayCart: "Botonera"}

// WriteAsRunCSV escribe el as-run en CSV.
func (s *Service) WriteAsRunCSV(w io.Writer, plays []domain.Play) error {
	loc := s.loc()
	cw := newCSV(w)
	_ = cw.Write([]string{"Fecha", "Hora", "Tipo", "Artista / Anunciante", "Título", "Duración (s)", "Completa"})
	for _, p := range plays {
		t := p.StartedAt.In(loc)
		_ = cw.Write([]string{t.Format("02/01/2006"), t.Format("15:04:05"), kindLabel[p.Kind], p.Artist, p.Title,
			fmt.Sprintf("%.1f", p.Duration.Seconds()), yesNo(p.Complete)})
	}
	cw.Flush()
	return cw.Error()
}

// WriteProofCSV escribe el comprobante de cuñas en CSV.
func (s *Service) WriteProofCSV(w io.Writer, rows []ProofRow) error {
	loc := s.loc()
	cw := newCSV(w)
	_ = cw.Write([]string{"Fecha", "Hora", "Anunciante", "Cuña", "Duración (s)", "Completa", "ID de emisión"})
	for _, r := range rows {
		t := r.At.In(loc)
		_ = cw.Write([]string{t.Format("02/01/2006"), t.Format("15:04:05"), r.Advertiser, r.Spot,
			fmt.Sprintf("%.1f", r.Duration.Seconds()), yesNo(r.Complete), r.UUID})
	}
	cw.Flush()
	return cw.Error()
}
