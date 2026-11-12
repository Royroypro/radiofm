package admin

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"radioclub25/internal/service/studio"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"radioclub25/internal/domain"
	"radioclub25/internal/transport/httpserver/form"
	"radioclub25/internal/transport/httpserver/middleware"
	"radioclub25/internal/transport/httpserver/views"
)

// Pantallas del panel para "Club25 Estudio" (la PC de la radio):
//   - Estudio (PC), solo admin: una PC por frecuencia, cada una con su token y su estado.
//   - Cuñas FM, editor o superior: alta/edición de cuñas y comprobante de emisión.

// studioOnline es el tiempo sin heartbeat tras el que la PC se considera desconectada.
const studioOnline = 3 * time.Minute

// StudioStation es una frecuencia con el estado de su PC del estudio.
type StudioStation struct {
	Freq    domain.Frequency
	State   *domain.StudioState
	Online  bool
	Ingest  string // servidor:puerto/montaje al que envía su audio ("" si es externo)
	Report  domain.StudioReport
	Alarms  []domain.Alarm
	Backups []string // copias de la base de la PC guardadas en el servidor
}

// StudioAlarms es la lista de alarmas de una frecuencia (para el inicio del panel).
type StudioAlarms struct {
	Freq   domain.Frequency
	Alarms []domain.Alarm
}

// studioAlarms devuelve las frecuencias con alarmas (solo las que tienen PC).
func (h *Handler) studioAlarms(ctx context.Context) []StudioAlarms {
	freqs, err := h.Frequencies.List(ctx)
	if err != nil {
		return nil
	}
	states, err := h.Studio.States(ctx)
	if err != nil {
		return nil
	}
	now := time.Now()
	var out []StudioAlarms
	for _, f := range freqs {
		if st := states[f.ID]; st != nil {
			if al := st.Alarms(now, studioOnline); len(al) > 0 {
				out = append(out, StudioAlarms{Freq: f, Alarms: al})
			}
		}
	}
	return out
}

// StudioPage muestra la PC de cada frecuencia: GET /admin/studio
func (h *Handler) StudioPage(w http.ResponseWriter, r *http.Request) {
	h.renderStudio(w, r, 0, "")
}

func (h *Handler) renderStudio(w http.ResponseWriter, r *http.Request, tokenFor int64, newToken string) {
	ctx := r.Context()
	freqs, err := h.Frequencies.List(ctx)
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	states, err := h.Studio.States(ctx)
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	now := time.Now()
	list := make([]StudioStation, 0, len(freqs))
	for _, f := range freqs {
		st := states[f.ID]
		if st == nil {
			st = &domain.StudioState{FrequencyID: f.ID, EncoderJSON: "{}"}
		}
		s := StudioStation{Freq: f, State: st, Online: st.Online(now, studioOnline), Report: st.Report(),
			Alarms: st.Alarms(now, studioOnline), Backups: studio.Backups(h.Cfg.StudioBackupsDir(), f.ID)}
		if !f.IsExternal() {
			s.Ingest = fmt.Sprintf("%s:%d%s", h.Cfg.IngestHost, h.Cfg.IcecastPublicPt, f.Mount)
		}
		list = append(list, s)
	}
	h.Views.Render(w, r, http.StatusOK, "admin/studio", "Estudio (PC)", map[string]any{
		"Stations": list, "TokenFor": tokenFor, "NewToken": newToken, "APIBase": h.Cfg.BaseURL + "/api/studio",
	})
}

// StudioTokenNew genera un token nuevo para la PC de una frecuencia y lo
// muestra una sola vez: POST /admin/studio/{fid}/token
func (h *Handler) StudioTokenNew(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f, err := h.Frequencies.Get(ctx, form.ID(chi.URLParam(r, "fid")))
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	tok, err := h.Studio.GenerateToken(ctx, middleware.ActorFrom(r), f.ID)
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	h.renderStudio(w, r, f.ID, tok)
}

// StudioTokenRevoke revoca el token de una frecuencia: POST /admin/studio/{fid}/token/revoke
func (h *Handler) StudioTokenRevoke(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f, err := h.Frequencies.Get(ctx, form.ID(chi.URLParam(r, "fid")))
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	if err := h.Studio.RevokeToken(ctx, middleware.ActorFrom(r), f.ID); err != nil {
		h.Views.Error(w, r, err)
		return
	}
	views.Redirect(w, r, "/admin/studio", "ok", "Token de "+f.City+" revocado: esa PC ya no puede sincronizar.")
}

// Cuñas FM (campañas, versiones y anunciantes): ver campaigns.go.

// ---------- Comprobante de emisión ----------

// reportFilter lee ?from&to&advertiser (por defecto, últimos 30 días; ver statsRange).
func (h *Handler) reportFilter(r *http.Request) (domain.AirplayFilter, string, string) {
	from, to, fromStr, toStr := h.statsRange(r)
	return domain.AirplayFilter{From: from, To: to, Advertiser: form.String(r, "advertiser"),
		FrequencyID: form.ID(form.String(r, "f"))}, fromStr, toStr
}

// advertiserSummary es el total por anunciante del comprobante.
type advertiserSummary struct {
	Advertiser string
	Plays      int
	Complete   int
	Seconds    int64
}

// FMSpotReport muestra el comprobante de emisión: GET /admin/fm-spots/report
func (h *Handler) FMSpotReport(w http.ResponseWriter, r *http.Request) {
	f, fromStr, toStr := h.reportFilter(r)
	plays, err := h.Studio.Report(r.Context(), f)
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	spots, err := h.Studio.ListSpots(r.Context())
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	// anunciantes para el filtro: los de las cuñas actuales y los del comprobante
	seen := map[string]bool{}
	var advertisers []string
	add := func(a string) {
		if k := strings.ToLower(a); !seen[k] {
			seen[k] = true
			advertisers = append(advertisers, a)
		}
	}
	for _, s := range spots {
		add(s.Advertiser)
	}
	byAdv := map[string]*advertiserSummary{}
	var order []string
	for _, p := range plays {
		add(p.Advertiser)
		s, ok := byAdv[p.Advertiser]
		if !ok {
			s = &advertiserSummary{Advertiser: p.Advertiser}
			byAdv[p.Advertiser] = s
			order = append(order, p.Advertiser)
		}
		s.Plays++
		s.Seconds += p.DurationMs / 1000
		if p.Complete {
			s.Complete++
		}
	}
	sort.Strings(advertisers)
	sort.Strings(order)
	summary := make([]advertiserSummary, 0, len(order))
	for _, a := range order {
		summary = append(summary, *byAdv[a])
	}
	if len(plays) > 1000 {
		plays = plays[len(plays)-1000:] // en pantalla, las últimas 1000; el CSV trae todas
	}
	// cumplimiento por campaña en el mismo rango (filtrado por anunciante si se eligió uno)
	comp, err := h.Studio.Compliance(r.Context(), f.From, f.To)
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	if f.Advertiser != "" {
		kept := comp[:0]
		for _, c := range comp {
			if strings.EqualFold(c.Campaign.Advertiser, f.Advertiser) {
				kept = append(kept, c)
			}
		}
		comp = kept
	}
	h.Views.Render(w, r, http.StatusOK, "admin/fm_spot_report", "Comprobante de emisión", map[string]any{
		"Compliance": comp,
		"From":       fromStr, "To": toStr, "Advertiser": f.Advertiser, "Advertisers": advertisers, "Freq": f.FrequencyID,
		"Plays": plays, "Summary": summary,
	})
}

// FMSpotReportCSV descarga el comprobante en CSV: GET /admin/fm-spots/report.csv
func (h *Handler) FMSpotReportCSV(w http.ResponseWriter, r *http.Request) {
	f, fromStr, toStr := h.reportFilter(r)
	plays, err := h.Studio.Report(r.Context(), f)
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	name := "comprobante"
	if f.Advertiser != "" {
		name += "_" + slug(f.Advertiser)
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s_%s_%s.csv"`, name, fromStr, toStr))
	_, _ = w.Write([]byte("\xEF\xBB\xBF")) // BOM para que Excel lea bien las tildes
	cw := csv.NewWriter(w)
	cw.Comma = ';'
	cities := map[int64]string{}
	if list, err := h.Frequencies.List(r.Context()); err == nil {
		for _, fr := range list {
			cities[fr.ID] = fr.Label() + " " + fr.City
		}
	}
	_ = cw.Write([]string{"Fecha", "Hora", "Frecuencia", "Anunciante", "Cuña", "Duración (s)", "Completa", "ID de emisión"})
	for _, p := range plays {
		t := p.AiredAt.In(h.Cfg.Location)
		complete := "No"
		if p.Complete {
			complete = "Sí"
		}
		_ = cw.Write([]string{t.Format("02/01/2006"), t.Format("15:04:05"), cities[p.FrequencyID], p.Advertiser, p.SpotName,
			fmt.Sprintf("%.1f", float64(p.DurationMs)/1000), complete, p.PlayUUID})
	}
	cw.Flush()
}

// slug deja solo letras, números y guiones para usarlo en un nombre de archivo.
func slug(s string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(s) {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			b.WriteRune(c)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "-"):
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
