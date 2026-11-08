package sqlite

import (
	"context"
	"database/sql"

	"radioclub25/internal/domain"
)

// AuditRepo implementa domain.AuditRepository.
type AuditRepo struct{ db *sql.DB }

// NewAuditRepo crea el repositorio de auditoría.
func NewAuditRepo(db *sql.DB) *AuditRepo { return &AuditRepo{db: db} }

// Add guarda una entrada de auditoría.
func (r *AuditRepo) Add(ctx context.Context, e *domain.AuditEntry) error {
	res, err := r.db.ExecContext(ctx, `INSERT INTO audit_log (user_id, user_email, action, entity, entity_id, details, ip, created_at)
		VALUES (?,?,?,?,?,?,?,?)`, e.UserID, e.UserEmail, e.Action, e.Entity, e.EntityID, e.Details, e.IP, formatTime(e.CreatedAt))
	if err != nil {
		return err
	}
	e.ID, _ = res.LastInsertId()
	return nil
}

// List devuelve las entradas más recientes primero.
func (r *AuditRepo) List(ctx context.Context, limit, offset int) ([]domain.AuditEntry, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, user_id, user_email, action, entity, entity_id, details, ip, created_at
		FROM audit_log ORDER BY id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.AuditEntry
	for rows.Next() {
		var e domain.AuditEntry
		var created string
		if err := rows.Scan(&e.ID, &e.UserID, &e.UserEmail, &e.Action, &e.Entity, &e.EntityID, &e.Details, &e.IP, &created); err != nil {
			return nil, err
		}
		e.CreatedAt = parseTime(created)
		out = append(out, e)
	}
	return out, rows.Err()
}
