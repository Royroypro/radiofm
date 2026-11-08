package admin

import (
	"errors"
	"net/http"
	"strings"

	"radioclub25/internal/domain"
	"radioclub25/internal/transport/httpserver/form"
	"radioclub25/internal/transport/httpserver/middleware"
	"radioclub25/internal/transport/httpserver/views"
)

// LoginForm muestra el formulario de acceso: GET /login
func (h *Handler) LoginForm(w http.ResponseWriter, r *http.Request) {
	if u := middleware.UserFrom(r.Context()); u != nil {
		http.Redirect(w, r, homeFor(u), http.StatusSeeOther)
		return
	}
	h.Views.Render(w, r, http.StatusOK, "auth/login", "Ingresar", map[string]string{"Next": safeNext(r.URL.Query().Get("next"))})
}

// Login procesa el acceso: POST /login
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	email, next := form.String(r, "email"), safeNext(form.String(r, "next"))
	token, u, err := h.Auth.Login(r.Context(), email, r.FormValue("password"), middleware.IPFrom(r.Context()), r.UserAgent())
	if err != nil {
		msg := "Email o contraseña incorrectos."
		if errors.Is(err, domain.ErrTooManyAttempts) {
			msg = err.Error()
		} else if !errors.Is(err, domain.ErrInvalidCredentials) {
			h.Views.Error(w, r, err)
			return
		}
		h.Views.RenderPage(w, r, http.StatusUnauthorized, "auth/login",
			&views.Page{Title: "Ingresar", Error: msg, Data: map[string]string{"Next": next, "Email": email}})
		return
	}
	http.SetCookie(w, &http.Cookie{Name: middleware.SessionCookie, Value: token, Path: "/", HttpOnly: true,
		Secure: h.Cfg.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: int(h.Cfg.SessionTTL.Seconds())})
	if next == "" {
		next = homeFor(u)
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

// Logout cierra la sesión: POST /logout
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(middleware.SessionCookie); err == nil {
		_ = h.Auth.Logout(r.Context(), c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: middleware.SessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true,
		Secure: h.Cfg.CookieSecure})
	views.Redirect(w, r, "/login", "ok", "Sesión cerrada.")
}

// PasswordForm muestra el cambio de contraseña: GET /admin/password
func (h *Handler) PasswordForm(w http.ResponseWriter, r *http.Request) {
	h.Views.Render(w, r, http.StatusOK, "admin/password", "Cambiar contraseña", nil)
}

// Password cambia la contraseña propia: POST /admin/password
func (h *Handler) Password(w http.ResponseWriter, r *http.Request) {
	next := r.FormValue("new_password")
	if next != r.FormValue("confirm_password") {
		h.formError(w, r, "admin/password", "Cambiar contraseña", nil, domain.Invalid("confirm_password", "Las contraseñas no coinciden"))
		return
	}
	if err := h.Auth.ChangePassword(r.Context(), middleware.ActorFrom(r), r.FormValue("current_password"), next); err != nil {
		h.formError(w, r, "admin/password", "Cambiar contraseña", nil, err)
		return
	}
	views.Redirect(w, r, "/admin", "ok", "Contraseña actualizada.")
}

func homeFor(u *domain.User) string {
	if u.Role == domain.RoleSuperAdmin {
		return "/superadmin"
	}
	return "/admin"
}

// safeNext evita redirecciones abiertas: solo rutas internas del panel.
func safeNext(next string) string {
	if strings.HasPrefix(next, "/admin") || strings.HasPrefix(next, "/superadmin") {
		if !strings.HasPrefix(next, "//") {
			return next
		}
	}
	return ""
}
