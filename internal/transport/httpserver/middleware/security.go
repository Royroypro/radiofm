package middleware

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"

	"radioclub25/internal/domain"
)

// Nombres de las cookies.
const (
	SessionCookie = "rc_session"
	csrfCookie    = "rc_csrf"
)

// CSRF protege los formularios con el patrón "double submit cookie":
// se guarda un token aleatorio en una cookie HttpOnly y cada POST debe enviar
// el mismo valor en el campo csrf_token (o la cabecera X-CSRF-Token).
// Las rutas con prefijo en exempt (beacons públicos e internas) no se validan.
func CSRF(secure bool, exempt ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, p := range exempt {
				if strings.HasPrefix(r.URL.Path, p) {
					next.ServeHTTP(w, r)
					return
				}
			}
			token := ""
			if c, err := r.Cookie(csrfCookie); err == nil && len(c.Value) == 32 {
				token = c.Value
			} else {
				b := make([]byte, 16)
				_, _ = rand.Read(b)
				token = hex.EncodeToString(b)
				http.SetCookie(w, &http.Cookie{Name: csrfCookie, Value: token, Path: "/", HttpOnly: true,
					Secure: secure, SameSite: http.SameSiteLaxMode})
			}
			if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete {
				sent := r.Header.Get("X-CSRF-Token")
				if sent == "" {
					sent = r.PostFormValue("csrf_token")
				}
				if subtle.ConstantTimeCompare([]byte(sent), []byte(token)) != 1 {
					http.Error(w, "Formulario vencido o inválido. Recargue la página e inténtelo de nuevo.", http.StatusForbidden)
					return
				}
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), keyCSRF, token)))
		})
	}
}

// Authenticator valida un token de sesión (lo implementa service/auth).
type Authenticator interface {
	Authenticate(ctx context.Context, token string) (*domain.User, error)
}

// LoadUser lee la cookie de sesión y, si es válida, pone el usuario en el contexto.
func LoadUser(auth Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if c, err := r.Cookie(SessionCookie); err == nil && c.Value != "" {
				if u, err := auth.Authenticate(r.Context(), c.Value); err == nil {
					r = r.WithContext(context.WithValue(r.Context(), keyUser, u))
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireRole exige sesión y un rol mínimo. Sin sesión redirige a /login.
func RequireRole(min domain.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u := UserFrom(r.Context())
			if u == nil {
				http.Redirect(w, r, "/login?next="+r.URL.Path, http.StatusSeeOther)
				return
			}
			if !u.Role.AtLeast(min) {
				http.Error(w, "No tiene permiso para ver esta página.", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// LicenseReader da el estado de licencia.
type LicenseReader interface {
	Current() domain.LicenseState
}

// PublicLicenseGuard muestra la página de "servicio suspendido" en la web
// pública cuando la licencia está en bloqueo total. suspended dibuja esa página.
func PublicLicenseGuard(lic LicenseReader, suspended http.HandlerFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if lic.Current().Level.PublicBlocked() {
				suspended(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// AdminWriteGuard impide cambios en el panel admin (POST) cuando la licencia
// lo pone en solo lectura. El superadmin nunca queda bloqueado. Las rutas en
// allow (p. ej. cambio de contraseña) siempre se permiten. blocked responde
// al usuario (normalmente un mensaje y redirección).
func AdminWriteGuard(lic LicenseReader, blocked http.HandlerFunc, allow ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && lic.Current().Level.AdminReadOnly() {
				u := UserFrom(r.Context())
				allowed := u != nil && u.Role == domain.RoleSuperAdmin
				for _, p := range allow {
					if r.URL.Path == p {
						allowed = true
					}
				}
				if !allowed {
					blocked(w, r)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// StudioAuthenticator valida el token de la PC del estudio (lo implementa service/studio).
type StudioAuthenticator interface {
	Authenticate(ctx context.Context, token string) error
}

// StudioToken exige "Authorization: Bearer <token>" con el token del estudio.
// Responde 401 en JSON si falta o no es válido. La comparación en tiempo
// constante la hace el servicio sobre el hash guardado.
func StudioToken(auth StudioAuthenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok || auth.Authenticate(r.Context(), strings.TrimSpace(token)) != nil {
				w.Header().Set("WWW-Authenticate", `Bearer realm="studio"`)
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"unauthorized"}` + "\n"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
