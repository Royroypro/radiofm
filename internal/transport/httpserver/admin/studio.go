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
//   - Estudio (PC), solo admin: token de acceso y estado de la PC.
//   - Cuñas FM, editor o superior: alta/edición de cuñas y comprobante de emisión.

// studioOnline es el tiempo sin heartbeat tras el que la PC se considera desconectada.
const studioOnline = 3 * time.Minute

// StudioPage muestra el estado de la PC y el token: GET /admin/studio
func (h *Handler) StudioPage(w http.ResponseWriter, r *http.Request) {
	h.renderStudio(w, r, "")
}

func (h *Handler) renderStudio(w http.ResponseWriter, r *http.Request, newToken string) {
	st, err := h.Studio.State(r.Context())
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	h.Views.Render(w, r, http.StatusOK, "admin/studio", "Estudio (PC)", map[string]any{
		"State": st, "Online": st.Online(time.Now(), studioOnline), "NewToken": newToken,
		"APIBase": h.Cfg.BaseURL + "/api/studio",
		"Ingest":  fmt.Sprintf("%s:%d%s", h.Cfg.IngestHost, h.Cfg.IcecastPublicPt, h.Cfg.IcecastMount),
	})
}

// StudioTokenNew genera un token nuevo y lo muestra una sola vez: POST /admin/studio/token
func (h *Handler) StudioTokenNew(w http.ResponseWriter, r *http.Request) {
	tok, err := h.Studio.GenerateToken(r.Context(), middleware.ActorFrom(r))
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	h.renderStudio(w, r, tok)
}

// StudioTokenRevoke revoca el token: POST /admin/studio/token/revoke
func (h *Handler) StudioTokenRevoke(w http.ResponseWriter, r *http.Request) {
	if err := h.Studio.RevokeToken(r.Context(), middleware.ActorFrom(r)); err != nil {
		h.Views.Error(w, r, err)
		return
	}
	views.Redirect(w, r, "/admin/studio", "ok", "Token revocado: la PC ya no puede sincronizar.")
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
	return domain.AirplayFilter{From: from, To: to, Advertiser: form.String(r, "advertiser")}, fromStr, toStr
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
		"From": fromStr, "To": toStr, "Advertiser": f.Advertiser, "Advertisers": advertisers,
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
	_ = cw.Write([]string{"Fecha", "Hora", "Anunciante", "Cuña", "Duración (s)", "Completa", "ID de emisión"})
	for _, p := range plays {
		t := p.AiredAt.In(h.Cfg.Location)
		complete := "No"
		if p.Complete {
			complete = "Sí"
		}
		_ = cw.Write([]string{t.Format("02/01/2006"), t.Format("15:04:05"), p.Advertiser, p.SpotName,
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
