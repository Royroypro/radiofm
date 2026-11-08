package sqlite

import (
	"context"
	"database/sql"

	"radioclub25/estudio/internal/domain"
)

// CartRepo implementa domain.CartRepository.
type CartRepo struct{ db *sql.DB }

// NewCartRepo crea el repositorio.
func NewCartRepo(db *sql.DB) *CartRepo { return &CartRepo{db: db} }

// Page devuelve los botones configurados de una página.
func (r *CartRepo) Page(ctx context.Context, page int) ([]domain.Cart, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT page, slot, label, color, path, stop_on_push FROM carts WHERE page=? ORDER BY slot`, page)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Cart
	for rows.Next() {
		var c domain.Cart
		var stop int
		if err := rows.Scan(&c.Page, &c.Slot, &c.Label, &c.Color, &c.Path, &stop); err != nil {
			return nil, err
		}
		c.StopOnPush = stop == 1
		out = append(out, c)
	}
	return out, rows.Err()
}

// Save guarda un botón; con Path vacío lo borra.
func (r *CartRepo) Save(ctx context.Context, c domain.Cart) error {
	if c.Path == "" {
		_, err := r.db.ExecContext(ctx, `DELETE FROM carts WHERE page=? AND slot=?`, c.Page, c.Slot)
		return err
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO carts (page, slot, label, color, path, stop_on_push) VALUES (?,?,?,?,?,?)
		ON CONFLICT(page, slot) DO UPDATE SET label=excluded.label, color=excluded.color, path=excluded.path,
		stop_on_push=excluded.stop_on_push`, c.Page, c.Slot, c.Label, c.Color, c.Path, b2i(c.StopOnPush))
	return err
}
