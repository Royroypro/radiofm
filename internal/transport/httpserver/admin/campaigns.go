package admin

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"radioclub25/internal/domain"
	"radioclub25/internal/platform/storage"
	"radioclub25/internal/service/studio"
	"radioclub25/internal/transport/httpserver/form"
	"radioclub25/internal/transport/httpserver/middleware"
	"radioclub25/internal/transport/httpserver/views"
)

// Publicidad FM (tráfico): campañas con sus versiones y anunciantes con su
// rubro. La PC del estudio aplica las reglas al armar las tandas.
//
//	GET  /admin/fm-spots                       campañas (con lo emitido hoy)
//	GET  /admin/fm-spots/new                   nueva campaña (+ primera versión opcional)
//	POST /admin/fm-spots, /admin/fm-spots/{id} guardar campaña
//	GET  /admin/fm-spots/{id}                  campaña con sus versiones
//	POST /admin/fm-spots/{id}/delete           borrar campaña
//	POST /admin/fm-spots/{id}/versions[/{vid}] subir o editar versión; …/{vid}/delete borrarla
//	GET/POST /admin/fm-advertisers[/{id}]      anunciantes; …/{id}/delete

// CampaignRow es una fila de la lista de campañas.
type CampaignRow struct {
	C        domain.FMCampaign
	Versions int
	Today    studio.CampaignCompliance
}

// FMSpotsList lista las campañas: GET /admin/fm-spots
func (h *Handler) FMSpotsList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	camps, err := h.Studio.Campaigns(ctx)
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	spots, err := h.Studio.ListSpots(ctx)
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	versions := map[int64]int{}
	for _, sp := range spots {
		versions[sp.CampaignID]++
	}
	now := time.Now().In(h.Cfg.Location)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, h.Cfg.Location)
	comp, err := h.Studio.Compliance(ctx, start, start.AddDate(0, 0, 1))
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	today := map[int64]studio.CampaignCompliance{}
	for _, c := range comp {
		today[c.Campaign.ID] = c
	}
	rows := make([]CampaignRow, 0, len(camps))
	for _, c := range camps {
		rows = append(rows, CampaignRow{C: c, Versions: versions[c.ID], Today: today[c.ID]})
	}
	h.Views.Render(w, r, http.StatusOK, "admin/fm_spots", "Publicidad FM", map[string]any{"Rows": rows, "Now": now})
}

func (h *Handler) campaignForm(c *domain.FMCampaign) map[string]any {
	advs, _ := h.Studio.Advertisers(context.Background())
	return map[string]any{"C": c, "Advertisers": advs, "MaxUploadMB": h.maxUploadMB(), "Weekdays": weekdayChecks(c.DaysMask)}
}

// FMSpotNew muestra el formulario de campaña nueva: GET /admin/fm-spots/new
func (h *Handler) FMSpotNew(w http.ResponseWriter, r *http.Request) {
	c := &domain.FMCampaign{PlaysPerDay: 6, HourFrom: 6, HourTo: 22, DaysMask: domain.AllDays,
		Position: domain.PositionAny, Priority: 2, Active: true}
	h.Views.Render(w, r, http.StatusOK, "admin/fm_spot_form", "Nueva campaña FM", h.campaignForm(c))
}

// FMSpotEdit muestra una campaña con sus versiones: GET /admin/fm-spots/{id}
func (h *Handler) FMSpotEdit(w http.ResponseWriter, r *http.Request) {
	c, err := h.Studio.Campaign(r.Context(), form.ID(chi.URLParam(r, "id")))
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	h.Views.Render(w, r, http.StatusOK, "admin/fm_spot_form", "Campaña FM", h.campaignForm(c))
}

// FMSpotSave crea o modifica una campaña: POST /admin/fm-spots y /admin/fm-spots/{id}.
// Al crearla se puede subir la primera versión (campo file).
func (h *Handler) FMSpotSave(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := form.ID(chi.URLParam(r, "id"))
	c := &domain.FMCampaign{}
	title := "Nueva campaña FM"
	if id > 0 {
		old, err := h.Studio.Campaign(ctx, id)
		if err != nil {
			h.Views.Error(w, r, err)
			return
		}
		c, title = old, "Campaña FM"
	}
	c.AdvertiserID = form.ID(form.String(r, "advertiser_id"))
	c.Name = form.String(r, "name")
	c.PlaysPerDay = form.Int(r, "plays_per_day")
	c.HourFrom = form.Int(r, "hour_from")
	c.HourTo = form.Int(r, "hour_to")
	c.StartsAt = form.DateTimePtr(r, "starts_at", h.Cfg.Location)
	c.EndsAt = form.DateTimePtr(r, "ends_at", h.Cfg.Location)
	c.DaysMask = daysFromForm(r)
	c.Position = domain.SpotPosition(form.String(r, "position"))
	c.Priority = form.Int(r, "priority")
	c.FrequencyID = form.ID(form.String(r, "frequency_id"))
	c.Active = form.Bool(r, "active")
	actor := middleware.ActorFrom(r)

	url, err := h.upload(r, "file", storage.KindAudio, "fm")
	if err != nil {
		h.formError(w, r, "admin/fm_spot_form", title, h.campaignForm(c), err)
		return
	}
	if err := h.Studio.SaveCampaign(ctx, actor, c, form.String(r, "advertiser")); err != nil {
		if url != "" {
			_ = h.Store.Delete(url)
		}
		h.formError(w, r, "admin/fm_spot_form", title, h.campaignForm(c), err)
		return
	}
	if url != "" { // primera versión
		v := &domain.FMSpot{Name: form.String(r, "version_name"), FileURL: url,
			DurationMs: int64(form.Int(r, "duration")) * 1000, Weight: 1}
		if err := h.Studio.SaveVersion(ctx, actor, c.ID, v); err != nil {
			_ = h.Store.Delete(url)
			views.Redirect(w, r, "/admin/fm-spots/"+strconv.FormatInt(c.ID, 10), "error",
				"Campaña guardada, pero el audio no: "+errText(err))
			return
		}
	}
	views.Redirect(w, r, "/admin/fm-spots/"+strconv.FormatInt(c.ID, 10), "ok",
		"Campaña guardada. La PC la recibirá en la próxima sincronización.")
}

// FMSpotDelete borra una campaña: POST /admin/fm-spots/{id}/delete
func (h *Handler) FMSpotDelete(w http.ResponseWriter, r *http.Request) {
	if err := h.Studio.DeleteCampaign(r.Context(), middleware.ActorFrom(r), form.ID(chi.URLParam(r, "id"))); err != nil {
		h.Views.Error(w, r, err)
		return
	}
	views.Redirect(w, r, "/admin/fm-spots", "ok", "Campaña eliminada. Su historial de emisiones se conserva.")
}

// FMVersionSave sube o edita una versión: POST /admin/fm-spots/{id}/versions[/{vid}]
func (h *Handler) FMVersionSave(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cid := form.ID(chi.URLParam(r, "id"))
	back := "/admin/fm-spots/" + strconv.FormatInt(cid, 10)
	v := &domain.FMSpot{Weight: 1}
	if vid := form.ID(chi.URLParam(r, "vid")); vid > 0 {
		old, err := h.Studio.GetSpot(ctx, vid)
		if err != nil || old.CampaignID != cid {
			h.Views.Error(w, r, domain.ErrNotFound)
			return
		}
		v = old
		v.Active = form.Bool(r, "active")
	}
	v.Name = form.String(r, "name")
	if wgt := form.Int(r, "weight"); wgt > 0 {
		v.Weight = wgt
	}
	if d := form.Int(r, "duration"); d > 0 || v.ID == 0 {
		v.DurationMs = int64(d) * 1000
	}
	url, err := h.upload(r, "file", storage.KindAudio, "fm")
	if err != nil {
		views.Redirect(w, r, back, "error", errText(err))
		return
	}
	if url != "" {
		v.FileURL = url
	}
	if err := h.Studio.SaveVersion(ctx, middleware.ActorFrom(r), cid, v); err != nil {
		if url != "" {
			_ = h.Store.Delete(url)
		}
		views.Redirect(w, r, back, "error", errText(err))
		return
	}
	views.Redirect(w, r, back, "ok", "Versión guardada.")
}

// FMVersionDelete borra una versión: POST /admin/fm-spots/{id}/versions/{vid}/delete
func (h *Handler) FMVersionDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cid, vid := form.ID(chi.URLParam(r, "id")), form.ID(chi.URLParam(r, "vid"))
	back := "/admin/fm-spots/" + strconv.FormatInt(cid, 10)
	if v, err := h.Studio.GetSpot(ctx, vid); err != nil || v.CampaignID != cid {
		h.Views.Error(w, r, domain.ErrNotFound)
		return
	}
	if err := h.Studio.DeleteSpot(ctx, middleware.ActorFrom(r), vid); err != nil {
		h.Views.Error(w, r, err)
		return
	}
	views.Redirect(w, r, back, "ok", "Versión eliminada. Su historial de emisiones se conserva.")
}

// ---------- Anunciantes ----------

// FMAdvertisersList lista los anunciantes: GET /admin/fm-advertisers
func (h *Handler) FMAdvertisersList(w http.ResponseWriter, r *http.Request) {
	list, err := h.Studio.Advertisers(r.Context())
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	h.Views.Render(w, r, http.StatusOK, "admin/fm_advertisers", "Anunciantes", list)
}

// FMAdvertiserSave crea o modifica: POST /admin/fm-advertisers y /admin/fm-advertisers/{id}
func (h *Handler) FMAdvertiserSave(w http.ResponseWriter, r *http.Request) {
	a := &domain.FMAdvertiser{ID: form.ID(chi.URLParam(r, "id")), Name: form.String(r, "name"), Sector: form.String(r, "sector")}
	if err := h.Studio.SaveAdvertiser(r.Context(), middleware.ActorFrom(r), a); err != nil {
		views.Redirect(w, r, "/admin/fm-advertisers", "error", errText(err))
		return
	}
	views.Redirect(w, r, "/admin/fm-advertisers", "ok", "Anunciante guardado.")
}

// FMAdvertiserDelete borra un anunciante sin campañas: POST /admin/fm-advertisers/{id}/delete
func (h *Handler) FMAdvertiserDelete(w http.ResponseWriter, r *http.Request) {
	if err := h.Studio.DeleteAdvertiser(r.Context(), middleware.ActorFrom(r), form.ID(chi.URLParam(r, "id"))); err != nil {
		views.Redirect(w, r, "/admin/fm-advertisers", "error", errText(err))
		return
	}
	views.Redirect(w, r, "/admin/fm-advertisers", "ok", "Anunciante eliminado.")
}

// ---------- días de la semana ----------

// WeekdayCheck es una casilla de día para el formulario (lunes primero).
type WeekdayCheck struct {
	Value   int
	Label   string
	Checked bool
}

func weekdayChecks(mask int) []WeekdayCheck {
	if mask == 0 {
		mask = domain.AllDays
	}
	out := make([]WeekdayCheck, 0, 7)
	for _, d := range []int{1, 2, 3, 4, 5, 6, 0} {
		out = append(out, WeekdayCheck{Value: d, Label: domain.WeekdayNames[d], Checked: mask&(1<<d) != 0})
	}
	return out
}

func daysFromForm(r *http.Request) int {
	_ = r.ParseMultipartForm(1 << 20)
	mask := 0
	for _, v := range r.Form["days"] {
		if d, err := strconv.Atoi(v); err == nil && d >= 0 && d <= 6 {
			mask |= 1 << d
		}
	}
	return mask
}

// errText es el mensaje de un error para un aviso del panel.
func errText(err error) string {
	_, msg := views.ErrorMessage(err)
	return msg
}
