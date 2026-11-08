// Package middleware contiene los middlewares HTTP: recuperación de pánicos,
// log de peticiones, IP real, cabeceras de seguridad, CSRF, sesión de usuario,
// control de roles y control de licencia.
//
// Orden en el router (ver httpserver/router.go):
//
//	Recover → RealIP → Logger → SecurityHeaders → CSRF → LoadUser → (por grupo) RequireRole / LicenseGuard
package middleware

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"radioclub25/internal/domain"
)

type ctxKey int

const (
	keyUser ctxKey = iota
	keyCSRF
	keyIP
)

// UserFrom devuelve el usuario con sesión iniciada o nil.
func UserFrom(ctx context.Context) *domain.User {
	u, _ := ctx.Value(keyUser).(*domain.User)
	return u
}

// CSRFTokenFrom devuelve el token CSRF de la petición (para ponerlo en los formularios).
func CSRFTokenFrom(ctx context.Context) string {
	t, _ := ctx.Value(keyCSRF).(string)
	return t
}

// IPFrom devuelve la IP del cliente.
func IPFrom(ctx context.Context) string {
	ip, _ := ctx.Value(keyIP).(string)
	return ip
}

// ActorFrom construye el actor de auditoría a partir de la petición.
func ActorFrom(r *http.Request) domain.Actor {
	a := domain.Actor{IP: IPFrom(r.Context())}
	if u := UserFrom(r.Context()); u != nil {
		a.UserID, a.Email, a.Role = u.ID, u.Email, u.Role
	}
	return a
}

// Recover convierte un pánico en un 500 y lo registra con la traza.
func Recover(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					if rec == http.ErrAbortHandler {
						panic(rec)
					}
					log.Error("pánico en handler", "panic", rec, "path", r.URL.Path, "stack", string(debug.Stack()))
					http.Error(w, "Error interno", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// RealIP toma la IP del cliente de X-Real-IP / X-Forwarded-For (puestos por
// nginx). La app solo escucha en 127.0.0.1, así que esas cabeceras siempre
// vienen de nginx.
func RealIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.Header.Get("X-Real-IP")
		if ip == "" {
			if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
				ip = strings.TrimSpace(strings.Split(xff, ",")[0])
			}
		}
		if ip == "" {
			ip, _, _ = net.SplitHostPort(r.RemoteAddr)
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), keyIP, ip)))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (s *statusRecorder) WriteHeader(code int) { s.status = code; s.ResponseWriter.WriteHeader(code) }
func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

// Flush permite el streaming a través del recorder.
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap permite a http.ResponseController llegar al writer original.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// Logger registra cada petición (excepto estáticos y health check).
func Logger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)
			p := r.URL.Path
			if strings.HasPrefix(p, "/static/") || p == "/healthz" || strings.HasPrefix(p, "/internal/") {
				return
			}
			log.Info("http", "method", r.Method, "path", p, "status", rec.status, "bytes", rec.bytes,
				"dur_ms", time.Since(start).Milliseconds(), "ip", IPFrom(r.Context()))
		})
	}
}

// SecurityHeaders añade cabeceras de seguridad básicas.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		next.ServeHTTP(w, r)
	})
}

// BodyLimit limita el tamaño del cuerpo de las peticiones (subidas incluidas).
func BodyLimit(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}
