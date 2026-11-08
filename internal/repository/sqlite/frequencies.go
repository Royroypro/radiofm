package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"radioclub25/internal/domain"
)

// FrequencyRepo implementa domain.FrequencyRepository.
type FrequencyRepo struct{ db *sql.DB }

// NewFrequencyRepo crea el repositorio de frecuencias.
func NewFrequencyRepo(db *sql.DB) *FrequencyRepo { return &FrequencyRepo{db: db} }

const frequencyCols = `id, city, frequency, band, sort_order, active, created_at`

func scanFrequency(r rowScanner) (*domain.Frequency, error) {
	var f domain.Frequency
	var active int
	var created string
	if err := r.Scan(&f.ID, &f.City, &f.Frequency, &f.Band, &f.SortOrder, &active, &created); err != nil {
		return nil, err
	}
	f.Active, f.CreatedAt = active == 1, parseTime(created)
	return &f, nil
}

// List devuelve las frecuencias en el orden de la web.
func (r *FrequencyRepo) List(ctx context.Context, onlyActive bool) ([]domain.Frequency, error) {
	q := `SELECT ` + frequencyCols + ` FROM frequencies`
	if onlyActive {
		q += ` WHERE active = 1`
	}
	rows, err := r.db.QueryContext(ctx, q+` ORDER BY sort_order, id`)
	if err != nil {
		return nil, fmt.Errorf("frequencies: list: %w", err)
	}
	defer rows.Close()
	var out []domain.Frequency
	for rows.Next() {
		f, err := scanFrequency(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *f)
	}
	return out, rows.Err()
}

// Get busca una frecuencia.
func (r *FrequencyRepo) Get(ctx context.Context, id int64) (*domain.Frequency, error) {
	f, err := scanFrequency(r.db.QueryRowContext(ctx, `SELECT `+frequencyCols+` FROM frequencies WHERE id=?`, id))
	return f, notFound(err)
}

// Create inserta una frecuencia.
func (r *FrequencyRepo) Create(ctx context.Context, f *domain.Frequency) error {
	f.CreatedAt = time.Now()
	res, err := r.db.ExecContext(ctx, `INSERT INTO frequencies (city, frequency, band, sort_order, active, created_at)
		VALUES (?,?,?,?,?,?)`, f.City, f.Frequency, f.Band, f.SortOrder, boolInt(f.Active), formatTime(f.CreatedAt))
	if err != nil {
		return fmt.Errorf("frequencies: create: %w", err)
	}
	f.ID, _ = res.LastInsertId()
	return nil
}

// Update modifica una frecuencia.
func (r *FrequencyRepo) Update(ctx context.Context, f *domain.Frequency) error {
	return checkAffected(r.db.ExecContext(ctx, `UPDATE frequencies SET city=?, frequency=?, band=?, sort_order=?, active=?
		WHERE id=?`, f.City, f.Frequency, f.Band, f.SortOrder, boolInt(f.Active), f.ID))
}

// Delete borra una frecuencia.
func (r *FrequencyRepo) Delete(ctx context.Context, id int64) error {
	return checkAffected(r.db.ExecContext(ctx, `DELETE FROM frequencies WHERE id=?`, id))
}
