package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"radioclub25/internal/domain"
)

// FrequencyRepo implementa domain.FrequencyRepository.
type FrequencyRepo struct{ db *sql.DB }

// NewFrequencyRepo crea el repositorio de frecuencias.
func NewFrequencyRepo(db *sql.DB) *FrequencyRepo { return &FrequencyRepo{db: db} }

const frequencyCols = `id, city, frequency, band, sort_order, active, source, mount, stream_url, created_at`

func scanFrequency(r rowScanner) (*domain.Frequency, error) {
	var f domain.Frequency
	var active int
	var created, source string
	if err := r.Scan(&f.ID, &f.City, &f.Frequency, &f.Band, &f.SortOrder, &active, &source, &f.Mount,
		&f.StreamURL, &created); err != nil {
		return nil, err
	}
	f.Active, f.Source, f.CreatedAt = active == 1, domain.StreamSource(source), parseTime(created)
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
	res, err := r.db.ExecContext(ctx, `INSERT INTO frequencies (city, frequency, band, sort_order, active, source,
		mount, stream_url, created_at) VALUES (?,?,?,?,?,?,?,?,?)`, f.City, f.Frequency, f.Band, f.SortOrder,
		boolInt(f.Active), string(f.Source), f.Mount, f.StreamURL, formatTime(f.CreatedAt))
	if err != nil {
		return mountTaken(fmt.Errorf("frequencies: create: %w", err))
	}
	f.ID, _ = res.LastInsertId()
	return nil
}

// Update modifica una frecuencia.
func (r *FrequencyRepo) Update(ctx context.Context, f *domain.Frequency) error {
	return mountTaken(checkAffected(r.db.ExecContext(ctx, `UPDATE frequencies SET city=?, frequency=?, band=?,
		sort_order=?, active=?, source=?, mount=?, stream_url=? WHERE id=?`, f.City, f.Frequency, f.Band, f.SortOrder,
		boolInt(f.Active), string(f.Source), f.Mount, f.StreamURL, f.ID)))
}

// Delete borra una frecuencia (su PC del estudio se borra en cascada).
func (r *FrequencyRepo) Delete(ctx context.Context, id int64) error {
	return checkAffected(r.db.ExecContext(ctx, `DELETE FROM frequencies WHERE id=?`, id))
}

// Usage cuenta los programas y cuñas asignados solo a esta frecuencia.
func (r *FrequencyRepo) Usage(ctx context.Context, id int64) (domain.FrequencyUsage, error) {
	var u domain.FrequencyUsage
	err := r.db.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM programs WHERE frequency_id = ?),
		(SELECT COUNT(*) FROM fm_spots WHERE frequency_id = ?)`, id, id).Scan(&u.Programs, &u.Spots)
	return u, err
}

// mountTaken traduce el índice único de montajes a un error de validación.
func mountTaken(err error) error {
	if err != nil && strings.Contains(err.Error(), "UNIQUE") && strings.Contains(err.Error(), "mount") {
		return domain.Invalid("mount", "Ese montaje ya lo usa otra frecuencia")
	}
	return err
}
