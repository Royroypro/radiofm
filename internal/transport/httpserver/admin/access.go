package admin

import (
	"net/http"

	"radioclub25/internal/service/settings"
	"radioclub25/internal/transport/httpserver/form"
	"radioclub25/internal/transport/httpserver/middleware"
	"radioclub25/internal/transport/httpserver/views"
)

// Accesos y seguridad → Tokens y claves (solo admin): en un solo lugar todo lo
// que da acceso a la emisión desde fuera del panel.
//   - Token de la PC del estudio de cada frecuencia (Club25 Estudio).
//   - Usuario y clave de publicación de la TV (OBS).
//   - Contraseña de fuente de Icecast (solo lectura: se define en el servidor).
// Los usuarios del panel están en Accesos y seguridad → Usuarios.

// AccessPage muestra los tokens y claves: GET /admin/access
func (h *Handler) AccessPage(w http.ResponseWriter, r *http.Request) {
	h.renderAccess(w, r, 0, "")
}

func (h *Handler) renderAccess(w http.ResponseWriter, r *http.Request, tokenFor int64, newToken string) {
	ctx := r.Context()
	stations, err := h.studioStations(r)
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	s, err := h.Settings.Get(ctx)
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	h.Views.Render(w, r, http.StatusOK, "admin/access", "Tokens y claves", map[string]any{
		"Stations": stations, "TokenFor": tokenFor, "NewToken": newToken, "APIBase": h.Cfg.BaseURL + "/api/studio",
		"S": s, "Info": h.streamInfo(s), "TVPlanEnabled": h.License.Current().Limits.TVEnabled,
	})
}

// AccessTVSave cambia el usuario de publicación de la TV y, si se pide, genera
// una clave nueva: POST /admin/access/tv
func (h *Handler) AccessTVSave(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s, err := h.Settings.Get(ctx)
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	s.TVPublishUser = form.String(r, "tv_publish_user")
	regen := form.Bool(r, "regenerate_key")
	if regen {
		s.TVPublishPass = settings.RandomKey()
	}
	if err := h.Settings.Update(ctx, middleware.ActorFrom(r), s); err != nil {
		if ve := views.AsValidation(err); ve != nil {
			views.Redirect(w, r, "/admin/access", "error", ve.Msg)
			return
		}
		h.Views.Error(w, r, err)
		return
	}
	msg := "Datos de publicación de la TV guardados."
	if regen {
		msg += " Se generó una clave nueva: actualícela en OBS."
	}
	views.Redirect(w, r, "/admin/access", "ok", msg)
}
