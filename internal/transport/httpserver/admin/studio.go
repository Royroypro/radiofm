package admin

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"radioclub25/internal/domain"
	"radioclub25/internal/platform/storage"
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
	Freq   domain.Frequency
	State  *domain.StudioState
	Online bool
	Ingest string // servidor:puerto/montaje al que envía su audio ("" si es externo)
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
		s := StudioStation{Freq: f, State: st, Online: st.Online(now, studioOnline)}
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

// ---------- Cuñas FM ----------

// FMSpotsList lista las cuñas FM: GET /admin/fm-spots
func (h *Handler) FMSpotsList(w http.ResponseWriter, r *http.Request) {
	list, err := h.Studio.ListSpots(r.Context())
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	today, err := h.Studio.TodayCounts(r.Context())
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	h.Views.Render(w, r, http.StatusOK, "admin/fm_spots", "Cuñas FM", map[string]any{
		"Spots": list, "Today": today, "Now": time.Now(),
	})
}

// FMSpotNew muestra el formulario de alta: GET /admin/fm-spots/new
func (h *Handler) FMSpotNew(w http.ResponseWriter, r *http.Request) {
	sp := &domain.FMSpot{PlaysPerDay: 6, HourFrom: 6, HourTo: 22, Active: true}
	h.Views.Render(w, r, http.StatusOK, "admin/fm_spot_form", "Nueva cuña FM", h.fmSpotFormData(sp))
}

// FMSpotEdit muestra el formulario de edición: GET /admin/fm-spots/{id}
func (h *Handler) FMSpotEdit(w http.ResponseWriter, r *http.Request) {
	sp, err := h.Studio.GetSpot(r.Context(), form.ID(chi.URLParam(r, "id")))
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	h.Views.Render(w, r, http.StatusOK, "admin/fm_spot_form", "Editar cuña FM", h.fmSpotFormData(sp))
}

func (h *Handler) fmSpotFormData(sp *domain.FMSpot) map[string]any {
	return map[string]any{"Spot": sp, "MaxUploadMB": h.maxUploadMB(), "DurationSec": sp.DurationMs / 1000}
}

// FMSpotSave crea o modifica una cuña: POST /admin/fm-spots y POST /admin/fm-spots/{id}
func (h *Handler) FMSpotSave(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := form.ID(chi.URLParam(r, "id"))
	sp := &domain.FMSpot{}
	if id > 0 {
		old, err := h.Studio.GetSpot(ctx, id)
		if err != nil {
			h.Views.Error(w, r, err)
			return
		}
		sp = old
	}
	sp.Advertiser = form.String(r, "advertiser")
	sp.Name = form.String(r, "name")
	sp.DurationMs = int64(form.Int(r, "duration")) * 1000
	sp.PlaysPerDay = form.Int(r, "plays_per_day")
	sp.HourFrom = form.Int(r, "hour_from")
	sp.HourTo = form.Int(r, "hour_to")
	sp.StartsAt = form.DateTimePtr(r, "starts_at", h.Cfg.Location)
	sp.EndsAt = form.DateTimePtr(r, "ends_at", h.Cfg.Location)
	sp.Active = form.Bool(r, "active")
	sp.FrequencyID = form.ID(form.String(r, "frequency_id"))

	title := "Nueva cuña FM"
	if id > 0 {
		title = "Editar cuña FM"
	}
	url, err := h.upload(r, "file", storage.KindAudio, "fm")
	if err != nil {
		h.formError(w, r, "admin/fm_spot_form", title, h.fmSpotFormData(sp), err)
		return
	}
	if url != "" {
		sp.FileURL = url
	}
	actor := middleware.ActorFrom(r)
	if id > 0 {
		err = h.Studio.UpdateSpot(ctx, actor, sp)
	} else {
		err = h.Studio.CreateSpot(ctx, actor, sp)
	}
	if err != nil {
		if url != "" {
			_ = h.Store.Delete(url) // no dejar archivos huérfanos
		}
		h.formError(w, r, "admin/fm_spot_form", title, h.fmSpotFormData(sp), err)
		return
	}
	views.Redirect(w, r, "/admin/fm-spots", "ok", "Cuña guardada. La PC la descargará en la próxima sincronización.")
}

// FMSpotDelete borra una cuña: POST /admin/fm-spots/{id}/delete
func (h *Handler) FMSpotDelete(w http.ResponseWriter, r *http.Request) {
	if err := h.Studio.DeleteSpot(r.Context(), middleware.ActorFrom(r), form.ID(chi.URLParam(r, "id"))); err != nil {
		h.Views.Error(w, r, err)
		return
	}
	views.Redirect(w, r, "/admin/fm-spots", "ok", "Cuña eliminada. Su historial de emisiones se conserva.")
}

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
	h.Views.Render(w, r, http.StatusOK, "admin/fm_spot_report", "Comprobante de emisión", map[string]any{
		"From": fromStr, "To": toStr, "Advertiser": f.Advertiser, "Advertisers": advertisers, "Freq": f.FrequencyID,
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
