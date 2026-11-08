package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

// SettingsRepo implementa domain.SettingsRepository (tabla clave/valor).
type SettingsRepo struct{ db *sql.DB }

// NewSettingsRepo crea el repositorio de ajustes.
func NewSettingsRepo(db *sql.DB) *SettingsRepo { return &SettingsRepo{db: db} }

// GetAll devuelve todos los ajustes guardados.
func (r *SettingsRepo) GetAll(ctx context.Context) (map[string]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, fmt.Errorf("settings: get: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// SetMany guarda (inserta o reemplaza) varios ajustes en una transacción.
func (r *SettingsRepo) SetMany(ctx context.Context, values map[string]string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // tras Commit no hace nada
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for k, v := range values {
		if _, err := stmt.ExecContext(ctx, k, v); err != nil {
			return fmt.Errorf("settings: set %s: %w", k, err)
		}
	}
	return tx.Commit()
}
