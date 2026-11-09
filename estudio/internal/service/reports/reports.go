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
	"sort"
	"strings"
	"time"

	"radioclub25/estudio/internal/domain"
)

// Service genera reportes.
type Service struct {
	plays  domain.PlayRepository
	spots  domain.SpotRepository
	tracks domain.TrackRepository // opcional (WithTracks): datos de la canción en el informe musical
	loc    func() *time.Location
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
	domain.PlayVoice: "Locución", domain.PlayCart: "Botonera", domain.PlayRelay: "Enlace a cadena"}

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

// Recent devuelve las últimas limit emisiones de las 24 h anteriores, la más reciente primero.
func (s *Service) Recent(ctx context.Context, limit int) ([]domain.Play, error) {
	plays, err := s.plays.Since(ctx, time.Now().Add(-24*time.Hour))
	if err != nil {
		return nil, err
	}
	out := make([]domain.Play, 0, limit)
	for i := len(plays) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, plays[i])
	}
	return out, nil
}

// ---------- Informe musical (derechos de autor: APDAYC) ----------

// WithTracks conecta la biblioteca (compositor, álbum y año del informe musical).
func (s *Service) WithTracks(tracks domain.TrackRepository) *Service {
	s.tracks = tracks
	return s
}

// MusicRow es una canción del informe musical: cuántas veces sonó y cuánto tiempo.
type MusicRow struct {
	TrackID  int64         `json:"track_id"`
	Artist   string        `json:"artist"`
	Title    string        `json:"title"`
	Composer string        `json:"composer"`
	Album    string        `json:"album"`
	Year     string        `json:"year"`
	Plays    int           `json:"plays"`
	Aired    time.Duration `json:"aired"`  // tiempo total al aire
	Length   time.Duration `json:"length"` // duración de la canción
}

// Music agrupa por canción la música emitida entre dos fechas (inclusive), de
// más a menos emitida. Solo música (no jingles, cuñas ni locuciones).
func (s *Service) Music(ctx context.Context, from, to string) ([]MusicRow, error) {
	f, t, err := s.Range(from, to)
	if err != nil {
		return nil, err
	}
	plays, err := s.plays.Between(ctx, f, t)
	if err != nil {
		return nil, err
	}
	byKey := map[string]*MusicRow{}
	var order []string
	for _, p := range plays {
		if p.Kind != domain.PlayMusic {
			continue
		}
		key := fmt.Sprint(p.TrackID)
		if p.TrackID == 0 {
			key = strings.ToLower(p.Artist + "|" + p.Title)
		}
		r, ok := byKey[key]
		if !ok {
			r = &MusicRow{TrackID: p.TrackID, Artist: p.Artist, Title: p.Title}
			if s.tracks != nil && p.TrackID > 0 {
				if tr, err := s.tracks.Get(ctx, p.TrackID); err == nil {
					r.Composer, r.Album, r.Year, r.Length = tr.Composer, tr.Album, tr.Year, tr.PlayLength()
				}
			}
			byKey[key] = r
			order = append(order, key)
		}
		r.Plays++
		r.Aired += p.Duration
	}
	out := make([]MusicRow, 0, len(order))
	for _, k := range order {
		out = append(out, *byKey[k])
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Plays != out[j].Plays {
			return out[i].Plays > out[j].Plays
		}
		return out[i].Artist+out[i].Title < out[j].Artist+out[j].Title
	})
	return out, nil
}

// WriteMusicCSV escribe el informe musical en CSV (para la sociedad de autores).
func (s *Service) WriteMusicCSV(w io.Writer, rows []MusicRow, from, to string) error {
	cw := newCSV(w)
	_ = cw.Write([]string{"Periodo", from + " a " + to})
	_ = cw.Write([]string{"Título", "Intérprete", "Compositor / autor", "Álbum", "Año", "Veces emitida",
		"Duración (s)", "Tiempo total al aire (s)"})
	for _, r := range rows {
		_ = cw.Write([]string{r.Title, r.Artist, r.Composer, r.Album, r.Year, fmt.Sprint(r.Plays),
			fmt.Sprintf("%.0f", r.Length.Seconds()), fmt.Sprintf("%.0f", r.Aired.Seconds())})
	}
	cw.Flush()
	return cw.Error()
}
