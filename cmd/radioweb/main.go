// Comando radioweb: servidor web de RadioClub25.
//
// Uso:
//
//	radioweb serve                               arranca el servidor (por defecto)
//	radioweb migrate                             solo aplica migraciones
//	radioweb create-superadmin EMAIL PASSWORD    crea o restablece un superadmin
//	radioweb backup ARCHIVO.db                   copia consistente de la base (VACUUM INTO)
//
// La construcción de la aplicación (repositorios, servicios, router) está en
// internal/app. Este archivo solo interpreta la línea de comandos y gestiona
// el ciclo de vida del servidor HTTP.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"radioclub25/internal/app"
	"radioclub25/internal/platform/config"
	"radioclub25/internal/platform/db"
	"radioclub25/internal/platform/logger"
	"radioclub25/internal/repository/sqlite"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := logger.New(cfg.IsProduction())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	database, err := db.Open(cfg.DBPath())
	if err != nil {
		return err
	}
	defer database.Close()
	if err := db.Migrate(ctx, database, sqlite.Migrations()); err != nil {
		return err
	}

	a, err := app.New(cfg, log, database, app.Options{})
	if err != nil {
		return err
	}

	cmd := "serve"
	if len(args) > 0 {
		cmd = args[0]
	}
	switch cmd {
	case "migrate":
		log.Info("migraciones al día")
		return nil

	case "create-superadmin":
		if len(args) != 3 {
			return errors.New("uso: radioweb create-superadmin EMAIL PASSWORD")
		}
		u, created, err := a.Auth.EnsureSuperAdmin(ctx, args[1], args[2])
		if err != nil {
			return err
		}
		if created {
			fmt.Println("Superadmin creado:", u.Email)
		} else {
			fmt.Println("Superadmin actualizado (contraseña restablecida):", u.Email)
		}
		return nil

	case "backup":
		if len(args) != 2 {
			return errors.New("uso: radioweb backup ARCHIVO.db")
		}
		if _, err := os.Stat(args[1]); err == nil {
			return fmt.Errorf("%s ya existe", args[1])
		}
		if _, err := database.ExecContext(ctx, `VACUUM INTO ?`, args[1]); err != nil {
			return fmt.Errorf("backup: %w", err)
		}
		fmt.Println("Copia creada:", args[1])
		return nil

	case "serve":
		return serve(ctx, cfg, log, a)

	default:
		return fmt.Errorf("comando desconocido %q (use serve, migrate, create-superadmin o backup)", cmd)
	}
}

// serve prepara los datos iniciales, lanza las tareas periódicas y el servidor HTTP
// hasta recibir SIGINT/SIGTERM.
func serve(ctx context.Context, cfg config.Config, log *slog.Logger, a *app.App) error {
	if err := a.Prepare(ctx); err != nil {
		return err
	}
	sched := a.Scheduler()
	sched.Start(ctx)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           a.Router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       10 * time.Minute, // subidas de video grandes
		IdleTimeout:       2 * time.Minute,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Info("servidor escuchando", "addr", cfg.HTTPAddr, "base_url", cfg.BaseURL, "env", cfg.Env)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Info("apagando...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Error("apagado", "err", err)
		}
	}
	sched.Wait()
	return nil
}
