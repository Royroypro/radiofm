package admin

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"radioclub25/internal/domain"
	"radioclub25/internal/platform/storage"
	"radioclub25/internal/transport/httpserver/form"
	"radioclub25/internal/transport/httpserver/middleware"
	"radioclub25/internal/transport/httpserver/views"
)

// ProgramsList lista la programación: GET /admin/programs
func (h *Handler) ProgramsList(w http.ResponseWriter, r *http.Request) {
	list, err := h.Programs.List(r.Context())
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	h.Views.Render(w, r, http.StatusOK, "admin/programs", "Programación", list)
}

// ProgramNew muestra el formulario de alta: GET /admin/programs/new
func (h *Handler) ProgramNew(w http.ResponseWriter, r *http.Request) {
	p := &domain.Program{Weekday: time.Monday, StartTime: "06:00", EndTime: "08:00", Active: true}
	h.Views.Render(w, r, http.StatusOK, "admin/program_form", "Nuevo programa", p)
}

// ProgramEdit muestra el formulario de edición: GET /admin/programs/{id}
func (h *Handler) ProgramEdit(w http.ResponseWriter, r *http.Request) {
	p, err := h.Programs.Get(r.Context(), form.ID(chi.URLParam(r, "id")))
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	h.Views.Render(w, r, http.StatusOK, "admin/program_form", "Editar programa", p)
}

// ProgramSave crea o modifica un programa: POST /admin/programs y /admin/programs/{id}
func (h *Handler) ProgramSave(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := form.ID(chi.URLParam(r, "id"))
	p := &domain.Program{}
	title := "Nuevo programa"
	if id > 0 {
		old, err := h.Programs.Get(ctx, id)
		if err != nil {
			h.Views.Error(w, r, err)
			return
		}
		p, title = old, "Editar programa"
	}
	p.Title = form.String(r, "title")
	p.Host = form.String(r, "host")
	p.Description = form.String(r, "description")
	p.Weekday = time.Weekday(form.Int(r, "weekday"))
	p.StartTime = form.String(r, "start_time")
	p.EndTime = form.String(r, "end_time")
	p.Active = form.Bool(r, "active")
	if form.Bool(r, "remove_image") {
		_ = h.Store.Delete(p.ImageURL)
		p.ImageURL = ""
	}
	url, err := h.upload(r, "image", storage.KindImage, "programs")
	if err != nil {
		h.formError(w, r, "admin/program_form", title, p, err)
		return
	}
	if url != "" {
		_ = h.Store.Delete(p.ImageURL)
		p.ImageURL = url
	}
	actor := middleware.ActorFrom(r)
	if id > 0 {
		err = h.Programs.Update(ctx, actor, p)
	} else {
		err = h.Programs.Create(ctx, actor, p)
	}
	if err != nil {
		h.formError(w, r, "admin/program_form", title, p, err)
		return
	}
	views.Redirect(w, r, "/admin/programs", "ok", "Programa guardado.")
}

// ProgramDelete borra un programa: POST /admin/programs/{id}/delete
func (h *Handler) ProgramDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := form.ID(chi.URLParam(r, "id"))
	p, err := h.Programs.Get(ctx, id)
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	if err := h.Programs.Delete(ctx, middleware.ActorFrom(r), id); err != nil {
		h.Views.Error(w, r, err)
		return
	}
	_ = h.Store.Delete(p.ImageURL)
	views.Redirect(w, r, "/admin/programs", "ok", "Programa eliminado.")
}
