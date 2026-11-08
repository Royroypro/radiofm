package admin

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"radioclub25/internal/domain"
	"radioclub25/internal/service/auth"
	"radioclub25/internal/transport/httpserver/form"
	"radioclub25/internal/transport/httpserver/middleware"
	"radioclub25/internal/transport/httpserver/views"
)

// UsersList lista los usuarios: GET /admin/users
func (h *Handler) UsersList(w http.ResponseWriter, r *http.Request) {
	list, err := h.Auth.ListUsers(r.Context(), middleware.ActorFrom(r))
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	h.Views.Render(w, r, http.StatusOK, "admin/users", "Usuarios", list)
}

// UserNew muestra el formulario de alta: GET /admin/users/new
func (h *Handler) UserNew(w http.ResponseWriter, r *http.Request) {
	h.Views.Render(w, r, http.StatusOK, "admin/user_form", "Nuevo usuario", &domain.User{Role: domain.RoleEditor, Active: true})
}

// UserEdit muestra el formulario de edición: GET /admin/users/{id}
func (h *Handler) UserEdit(w http.ResponseWriter, r *http.Request) {
	u, err := h.Auth.GetUser(r.Context(), middleware.ActorFrom(r), form.ID(chi.URLParam(r, "id")))
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	h.Views.Render(w, r, http.StatusOK, "admin/user_form", "Editar usuario", u)
}

// UserSave crea o edita un usuario: POST /admin/users y /admin/users/{id}
func (h *Handler) UserSave(w http.ResponseWriter, r *http.Request) {
	id := form.ID(chi.URLParam(r, "id"))
	in := auth.UserInput{
		Email: form.String(r, "email"), Name: form.String(r, "name"), Role: domain.Role(form.String(r, "role")),
		Active: form.Bool(r, "active"), Password: r.FormValue("password"),
	}
	actor := middleware.ActorFrom(r)
	var err error
	title := "Nuevo usuario"
	if id > 0 {
		title = "Editar usuario"
		err = h.Auth.UpdateUser(r.Context(), actor, id, in)
	} else {
		_, err = h.Auth.CreateUser(r.Context(), actor, in)
	}
	if err != nil {
		h.formError(w, r, "admin/user_form", title,
			&domain.User{ID: id, Email: in.Email, Name: in.Name, Role: in.Role, Active: in.Active}, err)
		return
	}
	views.Redirect(w, r, "/admin/users", "ok", "Usuario guardado.")
}

// UserDelete borra un usuario: POST /admin/users/{id}/delete
func (h *Handler) UserDelete(w http.ResponseWriter, r *http.Request) {
	err := h.Auth.DeleteUser(r.Context(), middleware.ActorFrom(r), form.ID(chi.URLParam(r, "id")))
	if ve := views.AsValidation(err); ve != nil {
		views.Redirect(w, r, "/admin/users", "error", ve.Msg)
		return
	}
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	views.Redirect(w, r, "/admin/users", "ok", "Usuario eliminado.")
}
