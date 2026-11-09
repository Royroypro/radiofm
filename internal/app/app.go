// Package app es la "raíz de composición": construye todas las piezas
// (repositorios → servicios → router HTTP) e inyecta las dependencias.
// Es el único paquete que conoce todas las capas a la vez.
//
// Lo usan cmd/radioweb (el binario) y los tests de integración, para que
// ambos arranquen exactamente la misma aplicación.
package app

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"time"

	"radioclub25/internal/platform/config"
	"radioclub25/internal/platform/icecast"
	"radioclub25/internal/platform/mediamtx"
	"radioclub25/internal/platform/scheduler"
	"radioclub25/internal/platform/storage"
	"radioclub25/internal/repository/sqlite"
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
	"radioclub25/internal/transport/httpserver"
	"radioclub25/internal/transport/httpserver/views"
	"radioclub25/web"
)

// App contiene los servicios construidos y el router HTTP.
type App struct {
	Cfg         config.Config
	Log         *slog.Logger
	DB          *sql.DB
	Auth        *auth.Service
	Settings    *settings.Service
	License     *license.Service
	Billing     *billing.Service
	Ads         *ads.Service
	Media       *media.Service
	Studio      *studio.Service
	Frequencies *frequencies.Service
	Router      http.Handler
}

// Options permite sustituir piezas externas en los tests.
type Options struct {
	Radio media.RadioSource // por defecto, cliente real de Icecast
	TV    media.TVSource    // por defecto, cliente real de MediaMTX
}

// New construye la aplicación completa sobre una base ya migrada.
func New(cfg config.Config, log *slog.Logger, database *sql.DB, opt Options) (*App, error) {
	// Repositorios (SQLite)
	userRepo := sqlite.NewUserRepo(database)
	sessionRepo := sqlite.NewSessionRepo(database)
	settingsRepo := sqlite.NewSettingsRepo(database)
	programRepo := sqlite.NewProgramRepo(database)
	frequencyRepo := sqlite.NewFrequencyRepo(database)
	adRepo := sqlite.NewAdRepo(database)
	adEventRepo := sqlite.NewAdEventRepo(database)
	billingRepo := sqlite.NewBillingRepo(database)
	licenseRepo := sqlite.NewLicenseRepo(database)
	auditRepo := sqlite.NewAuditRepo(database)
	fmSpotRepo := sqlite.NewFMSpotRepo(database)
	airplayRepo := sqlite.NewAirplayRepo(database)
	studioRepo := sqlite.NewStudioRepo(database)

	// Plataforma
	store, err := storage.New(cfg.UploadsDir(), "/uploads")
	if err != nil {
		return nil, err
	}
	if opt.Radio == nil {
		opt.Radio = icecast.New(cfg.IcecastURL, cfg.IcecastMount, cfg.IcecastAdminUser, cfg.IcecastAdminPass)
	}
	if opt.TV == nil {
		opt.TV = mediamtx.New(cfg.MediaMTXAPIURL, cfg.MediaMTXPath)
	}

	// Servicios
	auditSvc := audit.New(auditRepo, log)
	authSvc := auth.New(userRepo, sessionRepo, auditSvc, cfg.SessionTTL)
	settingsSvc := settings.New(settingsRepo, auditSvc)
	programsSvc := programs.New(programRepo, auditSvc, cfg.Location)
	frequenciesSvc := frequencies.New(frequencyRepo, auditSvc, cfg.IcecastMount)
	mediaSvc := media.New(opt.Radio, opt.TV, log)
	licenseSvc := license.New(licenseRepo, billingRepo, mediaSvc, auditSvc, log, cfg.Location)
	billingSvc := billing.New(billingRepo, auditSvc, cfg.Location)
	billingSvc.OnChange(func(ctx context.Context) {
		if _, err := licenseSvc.Refresh(ctx); err != nil {
			log.Error("recalcular licencia", "err", err)
		}
	})
	adsSvc := ads.New(adRepo, adEventRepo, licenseSvc, store, auditSvc, cfg.IPHashSalt)
	studioSvc := studio.New(fmSpotRepo, airplayRepo, studioRepo, store, auditSvc, cfg.Location)
	gateSvc := gate.New(licenseSvc, settingsSvc, cfg.MediaMTXPath, cfg.IcecastSourcePass)
	gateSvc.SetMounts(frequenciesSvc)

	// Vistas: embebidas en el binario o leídas del disco (WEB_DIR) en desarrollo
	var webFS fs.FS = web.FS
	if cfg.WebDir != "" {
		webFS = os.DirFS(cfg.WebDir)
	}
	renderer, err := views.New(webFS, cfg.WebDir != "", cfg.Location, log)
	if err != nil {
		return nil, err
	}
	staticFS, err := fs.Sub(webFS, "static")
	if err != nil {
		return nil, err
	}

	router := httpserver.NewRouter(httpserver.Deps{
		Cfg: cfg, Log: log, Views: renderer, StaticFS: staticFS, Store: store,
		Auth: authSvc, Settings: settingsSvc, Programs: programsSvc, Ads: adsSvc, Media: mediaSvc,
		Frequencies: frequenciesSvc, License: licenseSvc, Gate: gateSvc, Billing: billingSvc, Audit: auditSvc, Studio: studioSvc,
	})
	return &App{Cfg: cfg, Log: log, DB: database, Auth: authSvc, Settings: settingsSvc, License: licenseSvc,
		Billing: billingSvc, Ads: adsSvc, Media: mediaSvc, Studio: studioSvc, Frequencies: frequenciesSvc, Router: router}, nil
}

// Prepare deja los datos iniciales listos: ajustes por defecto, clave de TV,
// superadmin inicial (si se configuró y no existe) y primer cálculo de licencia.
func (a *App) Prepare(ctx context.Context) error {
	if err := a.Settings.EnsureDefaults(ctx); err != nil {
		return fmt.Errorf("ajustes iniciales: %w", err)
	}
	// Siempre hay al menos una frecuencia (la principal, con el montaje de siempre).
	if set, err := a.Settings.Get(ctx); err == nil {
		if err := a.Frequencies.Ensure(ctx, set.Frequency); err != nil {
			return fmt.Errorf("frecuencia inicial: %w", err)
		}
	}
	if a.Cfg.BootstrapSuperEmail != "" && a.Cfg.BootstrapSuperPass != "" {
		has, err := a.Auth.HasSuperAdmin(ctx)
		if err != nil {
			return err
		}
		if !has {
			if _, _, err := a.Auth.EnsureSuperAdmin(ctx, a.Cfg.BootstrapSuperEmail, a.Cfg.BootstrapSuperPass); err != nil {
				return fmt.Errorf("crear superadmin inicial: %w", err)
			}
			a.Log.Info("superadmin inicial creado", "email", a.Cfg.BootstrapSuperEmail)
		}
	}
	_, err := a.License.Refresh(ctx)
	return err
}

// Scheduler devuelve las tareas periódicas de la aplicación.
func (a *App) Scheduler() *scheduler.Scheduler {
	s := scheduler.New(a.Log)
	s.Add(scheduler.Task{Name: "licencia", Every: time.Hour, SkipFirst: true, Run: func(ctx context.Context) error {
		_, err := a.License.Refresh(ctx)
		return err
	}})
	s.Add(scheduler.Task{Name: "limpiar-sesiones", Every: 6 * time.Hour, Run: a.Auth.CleanupSessions})
	return s
}
