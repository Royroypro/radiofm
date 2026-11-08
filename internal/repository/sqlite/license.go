package sqlite

import (
	"context"
	"database/sql"

	"radioclub25/internal/domain"
)

// LicenseRepo implementa domain.LicenseRepository (fila única id=1).
type LicenseRepo struct{ db *sql.DB }

// NewLicenseRepo crea el repositorio de licencia.
func NewLicenseRepo(db *sql.DB) *LicenseRepo { return &LicenseRepo{db: db} }

// GetPolicy devuelve la política de bloqueo.
func (r *LicenseRepo) GetPolicy(ctx context.Context) (domain.LicensePolicy, error) {
	var p domain.LicensePolicy
	var auto, manual string
	err := r.db.QueryRowContext(ctx, `SELECT grace_days, auto_block_level, manual_level, manual_reason, suspended_message FROM license WHERE id=1`).
		Scan(&p.GraceDays, &auto, &manual, &p.ManualReason, &p.SuspendedMessage)
	p.AutoBlockLevel, p.ManualLevel = domain.LicenseLevel(auto), domain.LicenseLevel(manual)
	return p, notFound(err)
}

// SavePolicy guarda la política de bloqueo.
func (r *LicenseRepo) SavePolicy(ctx context.Context, p domain.LicensePolicy) error {
	_, err := r.db.ExecContext(ctx, `UPDATE license SET grace_days=?, auto_block_level=?, manual_level=?, manual_reason=?, suspended_message=? WHERE id=1`,
		p.GraceDays, string(p.AutoBlockLevel), string(p.ManualLevel), p.ManualReason, p.SuspendedMessage)
	return err
}

// LastLevel devuelve el último nivel aplicado.
func (r *LicenseRepo) LastLevel(ctx context.Context) (domain.LicenseLevel, error) {
	var l string
	err := r.db.QueryRowContext(ctx, `SELECT last_level FROM license WHERE id=1`).Scan(&l)
	return domain.LicenseLevel(l), notFound(err)
}

// SaveLastLevel guarda el último nivel aplicado.
func (r *LicenseRepo) SaveLastLevel(ctx context.Context, l domain.LicenseLevel) error {
	_, err := r.db.ExecContext(ctx, `UPDATE license SET last_level=? WHERE id=1`, string(l))
	return err
}
