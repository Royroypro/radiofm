package admin

import (
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"radioclub25/internal/domain"
	"radioclub25/internal/platform/storage"
	"radioclub25/internal/transport/httpserver/form"
	"radioclub25/internal/transport/httpserver/middleware"
	"radioclub25/internal/transport/httpserver/views"
)

// AdsList lista los anuncios: GET /admin/ads?type=banner
func (h *Handler) AdsList(w http.ResponseWriter, r *http.Request) {
	f := domain.AdFilter{Type: domain.AdType(r.URL.Query().Get("type"))}
	if !f.Type.Valid() {
		f.Type = ""
	}
	list, err := h.Ads.List(r.Context(), f)
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	h.Views.Render(w, r, http.StatusOK, "admin/ads", "Publicidad", map[string]any{
		"Ads": list, "Filter": f.Type, "Now": time.Now(), "Limits": h.License.Current().Limits,
		"MaxUploadMB": h.maxUploadMB(),
	})
}

// AdNew muestra el formulario de alta: GET /admin/ads/new?type=banner
func (h *Handler) AdNew(w http.ResponseWriter, r *http.Request) {
	t := domain.AdType(r.URL.Query().Get("type"))
	if !t.Valid() {
		t = domain.AdBanner
	}
	ad := &domain.Ad{Type: t, Position: domain.PosSidebar, Weight: 1, SkipAfterSec: 5, Active: true}
	h.Views.Render(w, r, http.StatusOK, "admin/ad_form", "Nuevo anuncio", h.adFormData(ad))
}

// AdEdit muestra el formulario de edición: GET /admin/ads/{id}
func (h *Handler) AdEdit(w http.ResponseWriter, r *http.Request) {
	ad, err := h.Ads.Get(r.Context(), form.ID(chi.URLParam(r, "id")))
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	h.Views.Render(w, r, http.StatusOK, "admin/ad_form", "Editar anuncio", h.adFormData(ad))
}

func (h *Handler) adFormData(ad *domain.Ad) map[string]any {
	return map[string]any{"Ad": ad, "MaxUploadMB": h.maxUploadMB()}
}

// AdSave crea o modifica un anuncio: POST /admin/ads y POST /admin/ads/{id}
func (h *Handler) AdSave(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := form.ID(chi.URLParam(r, "id"))
	ad := &domain.Ad{}
	if id > 0 {
		old, err := h.Ads.Get(ctx, id)
		if err != nil {
			h.Views.Error(w, r, err)
			return
		}
		ad = old
	}
	ad.Name = form.String(r, "name")
	ad.Type = domain.AdType(form.String(r, "type"))
	ad.Position = domain.AdPosition(form.String(r, "position"))
	ad.LinkURL = form.String(r, "link_url")
	ad.StartsAt = form.DateTimePtr(r, "starts_at", h.Cfg.Location)
	ad.EndsAt = form.DateTimePtr(r, "ends_at", h.Cfg.Location)
	ad.Weight = form.Int(r, "weight")
	ad.SkipAfterSec = form.Int(r, "skip_after_sec")
	ad.Active = form.Bool(r, "active")

	title := "Nuevo anuncio"
	if id > 0 {
		title = "Editar anuncio"
	}
	kind := storage.KindImage
	switch ad.Type {
	case domain.AdPrerollVideo:
		kind = storage.KindVideo
	case domain.AdAudioSpot:
		kind = storage.KindAudio
	}
	url, err := h.upload(r, "file", kind, "ads")
	if err != nil {
		h.formError(w, r, "admin/ad_form", title, h.adFormData(ad), err)
		return
	}
	if url != "" {
		ad.FileURL = url
	}

	actor := middleware.ActorFrom(r)
	if id > 0 {
		err = h.Ads.Update(ctx, actor, ad)
	} else {
		err = h.Ads.Create(ctx, actor, ad)
	}
	if err != nil {
		if url != "" && id == 0 {
			_ = h.Store.Delete(url) // no dejar archivos huérfanos
		}
		h.formError(w, r, "admin/ad_form", title, h.adFormData(ad), err)
		return
	}
	views.Redirect(w, r, "/admin/ads", "ok", "Anuncio guardado.")
}

// AdDelete borra un anuncio: POST /admin/ads/{id}/delete
func (h *Handler) AdDelete(w http.ResponseWriter, r *http.Request) {
	if err := h.Ads.Delete(r.Context(), middleware.ActorFrom(r), form.ID(chi.URLParam(r, "id"))); err != nil {
		h.Views.Error(w, r, err)
		return
	}
	views.Redirect(w, r, "/admin/ads", "ok", "Anuncio eliminado.")
}

// ---------- Estadísticas ----------

// statsRange lee ?from=YYYY-MM-DD&to=YYYY-MM-DD (por defecto, últimos 30 días).
// Devuelve el rango [from, to+1día) en la zona de la radio.
func (h *Handler) statsRange(r *http.Request) (from, to time.Time, fromStr, toStr string) {
	loc := h.Cfg.Location
	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	from, to = today.AddDate(0, 0, -29), today
	if t, err := time.ParseInLocation(domain.DateLayout, r.URL.Query().Get("from"), loc); err == nil {
		from = t
	}
	if t, err := time.ParseInLocation(domain.DateLayout, r.URL.Query().Get("to"), loc); err == nil {
		to = t
	}
	if to.Before(from) {
		from, to = to, from
	}
	return from, to.AddDate(0, 0, 1), from.Format(domain.DateLayout), to.Format(domain.DateLayout)
}

// Stats muestra las estadísticas de publicidad: GET /admin/stats
func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	from, to, fromStr, toStr := h.statsRange(r)
	stats, err := h.Ads.Stats(r.Context(), from, to)
	data := map[string]any{"From": fromStr, "To": toStr, "Stats": stats, "Disabled": false}
	if err != nil {
		if _, msg := views.ErrorMessage(err); errors.Is(err, domain.ErrPlanLimit) {
			data["Disabled"], data["Message"] = true, msg
		} else {
			h.Views.Error(w, r, err)
			return
		}
	}
	var tot domain.AdStats
	for _, s := range stats {
		tot.Impressions += s.Impressions
		tot.Clicks += s.Clicks
		tot.Completes += s.Completes
		tot.Skips += s.Skips
	}
	data["Total"] = tot
	h.Views.Render(w, r, http.StatusOK, "admin/stats", "Estadísticas", data)
}

// StatsCSV descarga las estadísticas en CSV: GET /admin/stats.csv
func (h *Handler) StatsCSV(w http.ResponseWriter, r *http.Request) {
	from, to, fromStr, toStr := h.statsRange(r)
	stats, err := h.Ads.Stats(r.Context(), from, to)
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="estadisticas_%s_%s.csv"`, fromStr, toStr))
	_, _ = w.Write([]byte("\xEF\xBB\xBF")) // BOM para que Excel lea bien las tildes
	cw := csv.NewWriter(w)
	cw.Comma = ';'
	_ = cw.Write([]string{"ID", "Anuncio", "Tipo", "Impresiones", "Clics", "CTR %", "Completados", "Saltados", "Desde", "Hasta"})
	for _, s := range stats {
		_ = cw.Write([]string{
			fmt.Sprint(s.AdID), s.Name, s.Type.Label(), fmt.Sprint(s.Impressions), fmt.Sprint(s.Clicks),
			fmt.Sprintf("%.2f", s.CTR()), fmt.Sprint(s.Completes), fmt.Sprint(s.Skips), fromStr, toStr,
		})
	}
	cw.Flush()
}
