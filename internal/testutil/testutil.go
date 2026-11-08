// Package testutil reúne ayudas para los tests: base SQLite temporal ya
// migrada, configuración de prueba y fuentes de medios falsas.
// Solo debe importarse desde archivos *_test.go.
package testutil

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"radioclub25/internal/platform/config"
	"radioclub25/internal/platform/db"
	"radioclub25/internal/platform/icecast"
	"radioclub25/internal/platform/mediamtx"
	"radioclub25/internal/repository/sqlite"
)

// DB abre una base SQLite nueva en una carpeta temporal y aplica las migraciones.
func DB(t testing.TB) *sql.DB {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if err := db.Migrate(context.Background(), d, sqlite.Migrations()); err != nil {
		t.Fatal(err)
	}
	return d
}

// Logger descarta los logs.
func Logger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// Lima es la zona horaria de la radio.
func Lima(t testing.TB) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Lima")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

// Config devuelve una configuración de desarrollo con datos en una carpeta temporal.
func Config(t testing.TB) config.Config {
	t.Helper()
	return config.Config{
		Env: "development", HTTPAddr: ":0", BaseURL: "http://test.local", DataDir: t.TempDir(),
		Location: Lima(t), SessionTTL: time.Hour, IPHashSalt: "sal-de-prueba", InternalToken: "token-interno",
		MaxUploadMB: 5, IcecastMount: "/radio", IcecastSourcePass: "fuente123", MediaMTXPath: "tv",
		IngestHost: "test.local", IcecastPublicPt: 8010, RTMPPublicPort: 1935, SRTPublicPort: 8890,
	}
}

// FakeRadio simula Icecast.
type FakeRadio struct {
	Online    bool
	Listeners int
	Killed    int
}

// Status implementa media.RadioSource.
func (f *FakeRadio) Status(context.Context) (icecast.MountStatus, error) {
	return icecast.MountStatus{Online: f.Online, Listeners: f.Listeners, Title: "Artista - Canción"}, nil
}

// KillSource implementa media.RadioSource.
func (f *FakeRadio) KillSource(context.Context) error { f.Killed++; f.Online = false; return nil }

// FakeTV simula MediaMTX.
type FakeTV struct {
	Online  bool
	Viewers int
	Kicked  int
}

// Status implementa media.TVSource.
func (f *FakeTV) Status(context.Context) (mediamtx.PathStatus, error) {
	return mediamtx.PathStatus{Online: f.Online, Viewers: f.Viewers}, nil
}

// KickPublisher implementa media.TVSource.
func (f *FakeTV) KickPublisher(context.Context) error { f.Kicked++; f.Online = false; return nil }
