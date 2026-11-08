package admin

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"radioclub25/internal/domain"
	"radioclub25/internal/transport/httpserver/form"
	"radioclub25/internal/transport/httpserver/middleware"
	"radioclub25/internal/transport/httpserver/views"
)

// FrequenciesList lista las frecuencias por ciudad: GET /admin/frequencies
func (h *Handler) FrequenciesList(w http.ResponseWriter, r *http.Request) {
	list, err := h.Frequencies.List(r.Context())
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	h.Views.Render(w, r, http.StatusOK, "admin/frequencies", "Frecuencias", list)
}

// FrequencyNew muestra el formulario de alta: GET /admin/frequencies/new
func (h *Handler) FrequencyNew(w http.ResponseWriter, r *http.Request) {
	list, _ := h.Frequencies.List(r.Context())
	f := &domain.Frequency{Band: "FM", Active: true, SortOrder: len(list) * 10}
	h.Views.Render(w, r, http.StatusOK, "admin/frequency_form", "Nueva frecuencia", f)
}

// FrequencyEdit muestra el formulario de edición: GET /admin/frequencies/{id}
func (h *Handler) FrequencyEdit(w http.ResponseWriter, r *http.Request) {
	f, err := h.Frequencies.Get(r.Context(), form.ID(chi.URLParam(r, "id")))
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	h.Views.Render(w, r, http.StatusOK, "admin/frequency_form", "Editar frecuencia", f)
}

// FrequencySave crea o modifica: POST /admin/frequencies y /admin/frequencies/{id}
func (h *Handler) FrequencySave(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := form.ID(chi.URLParam(r, "id"))
	f, title := &domain.Frequency{}, "Nueva frecuencia"
	if id > 0 {
		old, err := h.Frequencies.Get(ctx, id)
		if err != nil {
			h.Views.Error(w, r, err)
			return
		}
		f, title = old, "Editar frecuencia"
	}
	f.City = form.String(r, "city")
	f.Frequency = form.String(r, "frequency")
	f.Band = form.String(r, "band")
	f.SortOrder = form.Int(r, "sort_order")
	f.Active = form.Bool(r, "active")
	if err := h.Frequencies.Save(ctx, middleware.ActorFrom(r), f); err != nil {
		h.formError(w, r, "admin/frequency_form", title, f, err)
		return
	}
	views.Redirect(w, r, "/admin/frequencies", "ok", "Frecuencia guardada.")
}

// FrequencyMove cambia el orden: POST /admin/frequencies/{id}/up (delta −1) y /down (+1)
func (h *Handler) FrequencyMove(delta int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := form.ID(chi.URLParam(r, "id"))
		if err := h.Frequencies.Move(r.Context(), middleware.ActorFrom(r), id, delta); err != nil {
			h.Views.Error(w, r, err)
			return
		}
		views.Redirect(w, r, "/admin/frequencies", "ok", "Orden actualizado.")
	}
}

// FrequencyDelete borra: POST /admin/frequencies/{id}/delete
func (h *Handler) FrequencyDelete(w http.ResponseWriter, r *http.Request) {
	if err := h.Frequencies.Delete(r.Context(), middleware.ActorFrom(r), form.ID(chi.URLParam(r, "id"))); err != nil {
		h.Views.Error(w, r, err)
		return
	}
	views.Redirect(w, r, "/admin/frequencies", "ok", "Frecuencia eliminada.")
}
