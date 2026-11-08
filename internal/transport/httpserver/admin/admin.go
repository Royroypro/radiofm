// Package admin contiene los handlers del panel de la radio (/admin) y del
// inicio/cierre de sesión (/login, /logout).
//
// Permisos (aplicados en router.go con middleware.RequireRole):
//   - editor: dashboard, publicidad, estadísticas, programación, su contraseña
//   - admin:  además ajustes de marca, streams y usuarios
//   - superadmin: todo lo anterior y además /superadmin
//
// Patrón de cada formulario: GET muestra el formulario; POST valida a través
// del servicio; si hay error de validación se vuelve a dibujar el formulario
// con el mensaje; si va bien se redirige con un mensaje flash (POST→redirect→GET).
package admin

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"radioclub25/internal/domain"
	"radioclub25/internal/platform/config"
	"radioclub25/internal/platform/storage"
	"radioclub25/internal/service/ads"
	"radioclub25/internal/service/auth"
	"radioclub25/internal/service/license"
	"radioclub25/internal/service/media"
	"radioclub25/internal/service/programs"
	"radioclub25/internal/service/settings"
	"radioclub25/internal/service/studio"
	"radioclub25/internal/transport/httpserver/views"
)

// Handler agrupa las dependencias del panel admin.
type Handler struct {
	Views    *views.Renderer
	Auth     *auth.Service
	Settings *settings.Service
	Programs *programs.Service
	Ads      *ads.Service
	Media    *media.Service
	License  *license.Service
	Studio   *studio.Service
	Store    *storage.Store
	Cfg      config.Config
	Log      *slog.Logger
}

// Dashboard es la portada del panel: estado en vivo, licencia y resumen.
func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	all, err := h.Ads.List(ctx, domain.AdFilter{})
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	now := time.Now()
	live := 0
	for _, a := range all {
		if a.IsLive(now) {
			live++
		}
	}
	onAir, _ := h.Programs.OnAir(ctx, now)
	progs, _ := h.Programs.List(ctx)
	var top []domain.AdStats
	if stats, err := h.Ads.Stats(ctx, now.AddDate(0, 0, -7), now.Add(time.Minute)); err == nil {
		for _, s := range stats {
			if len(top) < 5 && s.Impressions > 0 {
				top = append(top, s)
			}
		}
	}
	h.Views.Render(w, r, http.StatusOK, "admin/dashboard", "Panel", map[string]any{
		"Status":    h.Media.Status(ctx),
		"AdsTotal":  len(all),
		"AdsLive":   live,
		"Programs":  len(progs),
		"OnAir":     onAir,
		"TopAds":    top,
		"AdsActive": h.Ads.Enabled(),
	})
}

// LicenseBlocked responde cuando la licencia impide cambios en el panel.
func (h *Handler) LicenseBlocked(w http.ResponseWriter, r *http.Request) {
	back := r.Referer()
	if back == "" {
		back = "/admin"
	}
	views.Redirect(w, r, back, "error",
		"El panel está en modo solo lectura por el estado del servicio. Contacte al proveedor para regularizarlo.")
}

// formError vuelve a dibujar un formulario con el error de validación o de
// permiso. Para cualquier otro error muestra la página de error.
func (h *Handler) formError(w http.ResponseWriter, r *http.Request, page, title string, data any, err error) {
	p := &views.Page{Title: title, Data: data}
	switch {
	case views.AsValidation(err) != nil:
		ve := views.AsValidation(err)
		p.Error, p.FieldErrors = ve.Msg, map[string]string{ve.Field: ve.Msg}
	case errors.Is(err, domain.ErrPlanLimit), errors.Is(err, domain.ErrForbidden):
		_, p.Error = views.ErrorMessage(err)
	default:
		h.Views.Error(w, r, err)
		return
	}
	h.Views.RenderPage(w, r, http.StatusUnprocessableEntity, page, p)
}

// upload guarda el archivo del campo field si se envió. Devuelve "" si no se subió nada.
// El tamaño máximo es el menor entre MAX_UPLOAD_MB y el límite del plan.
func (h *Handler) upload(r *http.Request, field string, kind storage.Kind, category string) (string, error) {
	return Upload(r, field, kind, category, h.Store, h.maxUploadMB())
}

func (h *Handler) maxUploadMB() int {
	max := h.Cfg.MaxUploadMB
	if p := h.License.Current().Limits.MaxUploadMB; p > 0 && p < max {
		max = p
	}
	return max
}

// Upload es la lógica de subida compartida con el panel superadmin.
func Upload(r *http.Request, field string, kind storage.Kind, category string, store *storage.Store, maxMB int) (string, error) {
	_, fh, err := r.FormFile(field)
	if errors.Is(err, http.ErrMissingFile) || errors.Is(err, http.ErrNotMultipart) {
		return "", nil // no se envió archivo
	}
	if err != nil {
		return "", domain.Invalid(field, "No se pudo leer el archivo (¿supera el tamaño máximo?)")
	}
	url, err := store.Save(fh, kind, category, int64(maxMB)<<20)
	switch {
	case errors.Is(err, storage.ErrFileTooLarge):
		return "", domain.Invalid(field, "El archivo supera el tamaño máximo permitido")
	case errors.Is(err, storage.ErrFileType):
		return "", domain.Invalid(field, "Tipo de archivo no permitido: "+err.Error())
	case err != nil:
		return "", err
	}
	return url, nil
}
