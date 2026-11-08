package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite" // driver "sqlite" (Go puro, sin CGO)

	"radioclub25/estudio/internal/domain"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Open abre (o crea) la base en path y aplica las migraciones pendientes.
func Open(ctx context.Context, path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("sqlite: crear carpeta: %w", err)
	}
	dsn := "file:" + filepath.ToSlash(path) +
		"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite: abrir: %w", err)
	}
	db.SetMaxOpenConns(4)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite: %w", err)
	}
	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// migrate aplica en orden las migraciones NNNN_*.sql no aplicadas, cada una en su transacción.
func migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("sqlite: schema_migrations: %w", err)
	}
	names, err := fs.Glob(migrationsFS, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		version := strings.TrimSuffix(filepath.Base(name), ".sql")
		var n int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version=?`, version).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		body, err := migrationsFS.ReadFile(name)
		if err != nil {
			return err
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("sqlite: migración %s: %w", version, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES (?,?)`,
			version, fmtTime(time.Now())); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// Backup copia la base a dst de forma consistente (VACUUM INTO).
func Backup(ctx context.Context, db *sql.DB, dst string) error {
	_ = os.Remove(dst)
	_, err := db.ExecContext(ctx, `VACUUM INTO ?`, dst)
	return err
}

// --- utilidades de conversión ---

func fmtTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func fmtTimePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return fmtTime(*t)
}

func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}

func parseTimePtr(ns sql.NullString) *time.Time {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	t := parseTime(ns.String)
	return &t
}

func ms(d time.Duration) int64 { return d.Milliseconds() }

func dur(ms int64) time.Duration { return time.Duration(ms) * time.Millisecond }

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}

func isUnique(err error) bool { return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") }

func checkAffected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

type rowScanner interface{ Scan(dest ...any) error }
