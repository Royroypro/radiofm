// Package sqlite implementa las interfaces de repositorio de internal/domain
// sobre SQLite. Es el ÚNICO paquete que contiene SQL.
//
// Organización: un archivo por agregado (users.go, ads.go, billing.go...).
// Las migraciones están en migrations/ y se embeben en el binario.
//
// Para añadir una tabla nueva:
//  1. Crear migrations/NNNN_descripcion.sql (número siguiente).
//  2. Definir la entidad y la interfaz del repositorio en internal/domain.
//  3. Implementarla aquí en un archivo nuevo, con un test en *_test.go.
//  4. Construirla en cmd/radioweb/main.go e inyectarla en el servicio.
package sqlite

import (
	"database/sql"
	"embed"
	"errors"
	"io/fs"
	"strings"
	"time"

	"radioclub25/internal/domain"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrations devuelve las migraciones embebidas para db.Migrate.
func Migrations() fs.FS {
	sub, _ := fs.Sub(migrationsFS, "migrations")
	return sub
}

// --- utilidades de conversión compartidas por todos los repositorios ---

func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func formatTimePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return formatTime(*t)
}

func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

func parseTimePtr(ns sql.NullString) *time.Time {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	t := parseTime(ns.String)
	return &t
}

func formatDate(t time.Time) string { return t.Format(domain.DateLayout) }

func formatDatePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return formatDate(*t)
}

func parseDate(s string) time.Time {
	t, _ := time.Parse(domain.DateLayout, s)
	return t
}

func parseDatePtr(ns sql.NullString) *time.Time {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	t := parseDate(ns.String)
	return &t
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// notFound traduce sql.ErrNoRows a domain.ErrNotFound.
func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}

// isUnique detecta violaciones de restricción UNIQUE.
func isUnique(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// checkAffected devuelve ErrNotFound si una actualización/borrado no afectó filas.
func checkAffected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// rowScanner permite reutilizar funciones scan con *sql.Row y *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}
