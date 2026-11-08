// Package db abre la base de datos SQLite y aplica las migraciones.
//
// Se usa modernc.org/sqlite (Go puro, sin CGO) para que el binario compile
// en cualquier sitio. La base trabaja en modo WAL para permitir lecturas
// concurrentes mientras se escribe.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite" // registra el driver "sqlite"
)

// Open abre (o crea) la base de datos en path.
func Open(path string) (*sql.DB, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("db: crear carpeta: %w", err)
		}
	}
	dsn := "file:" + path +
		"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)"
	d, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("db: abrir: %w", err)
	}
	d.SetMaxOpenConns(4)
	if err := d.Ping(); err != nil {
		return nil, fmt.Errorf("db: ping: %w", err)
	}
	return d, nil
}

// Migrate aplica en orden los archivos NNNN_nombre.sql de migrations que aún
// no se hayan aplicado. Cada migración corre en su propia transacción y se
// registra en la tabla schema_migrations. Nunca edite una migración ya
// aplicada en producción: cree una nueva.
func Migrate(ctx context.Context, d *sql.DB, migrations fs.FS) error {
	if _, err := d.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
	)`); err != nil {
		return fmt.Errorf("db: crear schema_migrations: %w", err)
	}
	names, err := fs.Glob(migrations, "*.sql")
	if err != nil {
		return fmt.Errorf("db: listar migraciones: %w", err)
	}
	sort.Strings(names)
	for _, name := range names {
		version := strings.TrimSuffix(name, ".sql")
		var exists int
		if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&exists); err != nil {
			return fmt.Errorf("db: consultar migración %s: %w", version, err)
		}
		if exists > 0 {
			continue
		}
		body, err := fs.ReadFile(migrations, name)
		if err != nil {
			return fmt.Errorf("db: leer %s: %w", name, err)
		}
		tx, err := d.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("db: aplicar %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version) VALUES (?)`, version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("db: registrar %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("db: commit %s: %w", name, err)
		}
		slog.Info("migración aplicada", "version", version)
	}
	return nil
}
