// Package httpserver arma el servidor HTTP: middlewares y TODAS las rutas de
// la aplicación en un solo lugar (este archivo). Los handlers viven en los
// subpaquetes public, admin y superadmin.
//
// Para añadir una ruta:
//  1. Escribir el método handler en el subpaquete del área.
//  2. Registrarlo aquí, dentro del grupo con el rol adecuado.
//  3. Si es una página, crear su plantilla en web/templates/<área>/.
package httpserver

import (
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"radioclub25/internal/domain"
	"radioclub25/internal/platform/config"
	"radioclub25/internal/platform/storage"
	"radioclub25/internal/service/ads"
	"radioclub25/internal/service/audit"
	"radioclub25/internal/service/auth"
	"radioclub25/internal/service/billing"
	"radioclub25/internal/service/frequencies"
	"radioclub25/internal/service/gate"
	"radioclub25/internal/service/license"
	"radioclub25/internal/service/media"
	"radioclub25/internal/service/programs"
	"radioclub25/internal/service/settings"
	"radioclub25/internal/service/studio"
	"radioclub25/internal/transport/httpserver/admin"
	mw "radioclub25/internal/transport/httpserver/middleware"
	"radioclub25/internal/transport/httpserver/public"
	"radioclub25/internal/transport/httpserver/studioapi"
	"radioclub25/internal/transport/httpserver/superadmin"
	"radioclub25/internal/transport/httpserver/views"
)

// Deps son las dependencias del servidor HTTP (se construyen en cmd/radioweb).
type Deps struct {
	Cfg      config.Config
	Log      *slog.Logger
	Views    *views.Renderer
	StaticFS fs.FS // contenido de web/static
	Store    *storage.Store

	Auth        *auth.Service
	Settings    *settings.Service
	Programs    *programs.Service
	Frequencies *frequencies.Service
	Ads         *ads.Service
	Media       *media.Service
	License     *license.Service
	Gate        *gate.Service
	Billing     *billing.Service
	Audit       *audit.Service
	Studio      *studio.Service
}

// NewRouter devuelve el http.Handler con todas las rutas.
func NewRouter(d Deps) http.Handler {
	pub := &public.Handler{Views: d.Views, Settings: d.Settings, Programs: d.Programs, Frequencies: d.Frequencies,
		Ads: d.Ads, Media: d.Media,
		License: d.License, Gate: d.Gate, Studio: d.Studio, Log: d.Log, InternalToken: d.Cfg.InternalToken}
	adm := &admin.Handler{Views: d.Views, Auth: d.Auth, Settings: d.Settings, Programs: d.Programs, Ads: d.Ads,
		Frequencies: d.Frequencies, Media: d.Media, License: d.License, Studio: d.Studio, Store: d.Store, Cfg: d.Cfg, Log: d.Log}
	stu := &studioapi.Handler{Studio: d.Studio, Settings: d.Settings, Frequencies: d.Frequencies, Programs: d.Programs, Cfg: d.Cfg, Log: d.Log}
	sup := &superadmin.Handler{Views: d.Views, Billing: d.Billing, License: d.License, Audit: d.Audit, Media: d.Media,
		Store: d.Store, Cfg: d.Cfg, Log: d.Log}

	// Datos comunes de todas las páginas.
	d.Views.SetCommon(func(w http.ResponseWriter, r *http.Request, p *views.Page) {
		p.Site, _ = d.Settings.Get(r.Context())
		// Frecuencias por ciudad: la primera activa es la principal (títulos, logo, reproductor).
		// En el panel se listan todas (también las ocultas) para los selectores.
		if strings.HasPrefix(r.URL.Path, "/admin") {
			p.Frequencies, _ = d.Frequencies.List(r.Context())
		} else {
			p.Frequencies, _ = d.Frequencies.Active(r.Context())
		}
		for _, f := range p.Frequencies {
			if f.Active {
				p.Site.Frequency = f.Label()
				break
			}
		}
		p.User = mw.UserFrom(r.Context())
		p.License = d.License.Current()
		p.CSRF = mw.CSRFTokenFrom(r.Context())
		p.Flash = views.PopFlash(w, r)
		p.BaseURL = d.Cfg.BaseURL
	})

	r := chi.NewRouter()
	r.Use(mw.Recover(d.Log), mw.RealIP, mw.Logger(d.Log), mw.SecurityHeaders,
		mw.BodyLimit(int64(d.Cfg.MaxUploadMB+2)<<20),
		mw.CSRF(d.Cfg.CookieSecure, "/api/", "/internal/"),
		mw.LoadUser(d.Auth))
	r.NotFound(pub.NotFound)

	// --- Sin restricciones de licencia ---
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	r.Handle("/static/*", http.StripPrefix("/static/", cacheStatic(http.FileServerFS(d.StaticFS))))
	r.Handle("/uploads/*", http.StripPrefix("/uploads/", cacheStatic(noDirListing(http.FileServer(http.Dir(d.Store.Root()))))))
	r.Post("/internal/mediamtx/auth", pub.MediaMTXAuth)
	r.Post("/internal/icecast/source", pub.IcecastSourceAuth)
	r.Get("/internal/gate/radio", pub.RadioGate)
	r.Get("/login", adm.LoginForm)
	r.Post("/login", adm.Login)
	r.Post("/logout", adm.Logout)

	// --- Web pública: con bloqueo total muestra "servicio suspendido" ---
	r.Group(func(r chi.Router) {
		r.Use(mw.PublicLicenseGuard(d.License, pub.Suspended))
		r.Get("/", pub.Home)
		r.Get("/programacion", pub.Schedule)
		r.Get("/contacto", pub.Contact)
		r.Get("/api/status", pub.APIStatus)
		r.Get("/api/ads/preroll", pub.APIPreroll)
		r.Post("/api/ads/event", pub.APIEvent)
		r.Get("/ad/click/{id}", pub.AdClick)
	})

	// --- API de la PC del estudio (Club25 Estudio): token Bearer; con bloqueo total, 503 ---
	r.Route("/api/studio", func(r chi.Router) {
		r.Use(mw.PublicLicenseGuard(d.License, pub.Suspended), mw.StudioToken(d.Studio))
		r.Get("/config", stu.Config)
		r.Get("/spots", stu.Spots)
		r.Get("/programs", stu.Schedule)
		r.Post("/heartbeat", stu.Heartbeat)
		r.Post("/nowplaying", stu.NowPlaying)
		r.Post("/airplays", stu.Airplays)
	})

	// --- Panel admin (editor o superior) ---
	r.Route("/admin", func(r chi.Router) {
		r.Use(mw.RequireRole(domain.RoleEditor))
		r.Use(mw.AdminWriteGuard(d.License, adm.LicenseBlocked, "/admin/password"))
		r.Get("/", adm.Dashboard)
		r.Get("/password", adm.PasswordForm)
		r.Post("/password", adm.Password)

		r.Get("/ads", adm.AdsList)
		r.Get("/ads/new", adm.AdNew)
		r.Post("/ads", adm.AdSave)
		r.Get("/ads/{id}", adm.AdEdit)
		r.Post("/ads/{id}", adm.AdSave)
		r.Post("/ads/{id}/delete", adm.AdDelete)
		r.Get("/stats", adm.Stats)
		r.Get("/stats.csv", adm.StatsCSV)

		r.Get("/fm-spots", adm.FMSpotsList)
		r.Get("/fm-spots/new", adm.FMSpotNew)
		r.Post("/fm-spots", adm.FMSpotSave)
		r.Get("/fm-spots/report", adm.FMSpotReport)
		r.Get("/fm-spots/report.csv", adm.FMSpotReportCSV)
		r.Get("/fm-spots/{id}", adm.FMSpotEdit)
		r.Post("/fm-spots/{id}", adm.FMSpotSave)
		r.Post("/fm-spots/{id}/delete", adm.FMSpotDelete)

		r.Get("/frequencies", adm.FrequenciesList)
		r.Get("/frequencies/new", adm.FrequencyNew)
		r.Post("/frequencies", adm.FrequencySave)
		r.Get("/frequencies/{id}", adm.FrequencyEdit)
		r.Post("/frequencies/{id}", adm.FrequencySave)
		r.Post("/frequencies/{id}/up", adm.FrequencyMove(-1))
		r.Post("/frequencies/{id}/down", adm.FrequencyMove(+1))
		r.Post("/frequencies/{id}/delete", adm.FrequencyDelete)

		r.Get("/programs", adm.ProgramsList)
		r.Get("/programs/new", adm.ProgramNew)
		r.Post("/programs", adm.ProgramSave)
		r.Get("/programs/{id}", adm.ProgramEdit)
		r.Post("/programs/{id}", adm.ProgramSave)
		r.Post("/programs/{id}/delete", adm.ProgramDelete)

		// Solo admin o superadmin
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireRole(domain.RoleAdmin))
			r.Get("/settings", adm.SettingsForm)
			r.Post("/settings", adm.SettingsSave)
			r.Get("/streams", adm.Streams)
			r.Post("/streams", adm.StreamsSave)
			r.Get("/studio", adm.StudioPage)
			r.Post("/studio/{fid}/token", adm.StudioTokenNew)
			r.Post("/studio/{fid}/token/revoke", adm.StudioTokenRevoke)
			r.Get("/users", adm.UsersList)
			r.Get("/users/new", adm.UserNew)
			r.Post("/users", adm.UserSave)
			r.Get("/users/{id}", adm.UserEdit)
			r.Post("/users/{id}", adm.UserSave)
			r.Post("/users/{id}/delete", adm.UserDelete)
		})
	})

	// --- Panel superadmin ---
	r.Route("/superadmin", func(r chi.Router) {
		r.Use(mw.RequireRole(domain.RoleSuperAdmin))
		r.Get("/", sup.Dashboard)
		r.Get("/client", sup.ClientForm)
		r.Post("/client", sup.ClientSave)

		r.Get("/plans", sup.Plans)
		r.Get("/plans/new", sup.PlanForm)
		r.Post("/plans", sup.PlanSave)
		r.Get("/plans/{id}", sup.PlanForm)
		r.Post("/plans/{id}", sup.PlanSave)
		r.Post("/plans/{id}/delete", sup.PlanDelete)

		r.Get("/contracts", sup.Contracts)
		r.Get("/contracts/new", sup.ContractForm)
		r.Post("/contracts", sup.ContractSave)
		r.Get("/contracts/{id}", sup.Contract)
		r.Get("/contracts/{id}/edit", sup.ContractForm)
		r.Post("/contracts/{id}/edit", sup.ContractSave)
		r.Post("/contracts/{id}/delete", sup.ContractDelete)

		r.Get("/payments", sup.Payments)
		r.Get("/payments/new", sup.PaymentForm)
		r.Post("/payments", sup.PaymentSave)
		r.Get("/payments/{id}", sup.PaymentForm)
		r.Post("/payments/{id}", sup.PaymentSave)
		r.Post("/payments/{id}/delete", sup.PaymentDelete)

		r.Get("/license", sup.LicensePage)
		r.Post("/license/policy", sup.LicensePolicy)
		r.Post("/license/block", sup.LicenseBlock)
		r.Post("/license/unblock", sup.LicenseUnblock)
		r.Post("/license/refresh", sup.LicenseRefresh)

		r.Get("/audit", sup.AuditLog)
	})
	return r
}

// cacheStatic añade caché de navegador a los archivos estáticos y subidos.
func cacheStatic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		next.ServeHTTP(w, r)
	})
}

// noDirListing evita que se listen las carpetas de uploads.
func noDirListing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "" || strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
